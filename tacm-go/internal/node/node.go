// Package node 實現 TAC 自主智能鏈的單節點：持續出塊、交易驗證與內存池、
// 動態難度，以及輕客戶端所需的查詢能力。M2 為單節點垂直切片，P2P 網絡與
// BFT 最終性在後續里程碑接入。
package node

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tacm/internal/chaindb"
	"tacm/internal/config"
	"tacm/internal/consensus/bft"
	"tacm/internal/consensus/difficulty"
	bridgepkg "tacm/internal/bridge"
	"tacm/internal/c2c"
	"tacm/internal/community"
	"tacm/internal/defi"
	"tacm/internal/exchange"
	"tacm/internal/wallet"
	"tacm/internal/crypto"
	"tacm/internal/p2p"
	"tacm/internal/rollup"
	"tacm/internal/vm"
)

const (
	maxTxPerBlock = 500
	fastBlockTime = 1 * time.Second
)

// errNotProposer 表示當前高度不由本節點提議（分佈式模式下靜默等待網絡塊）。
var errNotProposer = errors.New("not my turn to propose")

// Node 為 TAC 自主智能鏈節點。
type Node struct {
	cfg    *config.Config
	db     *chaindb.ChainDB
	nodeID string

	keypair     *crypto.KeyPair
	nodeAddress string

	blockTime time.Duration
	onlineSince time.Time

	mu       sync.Mutex // 保護以下共識狀態
	baseDiff int
	adjDiff  int

	running     bool
	distributed bool // 分佈式 BFT：僅輪值 proposer 出塊，其餘投票
	stopCh      chan struct{}
	wg          sync.WaitGroup

	// consensusWake 喚醒 consensusLoop（收到 view-change / 新提案時立即重試）。
	consensusWake chan struct{}

	// BFT 即時最終性
	vset          *bft.ValidatorSet
	bftNode       *bft.BFTNode
	finalityLog   *bft.FinalityLog
	finalizedH    int64
	precommitMu   sync.Mutex
	precommitVotes map[int64]map[string]bft.Vote

	// M13 錢包帳本（多資產：TACm/TiUSD/USDT；隨鏈上區塊同步）。
	walletSvc   *wallet.Service
	exchangeSvc *exchange.Service
	communitySvc *community.Store
	defiSvc      *defi.Store
	c2cSvc       *c2c.Store
	authSvc      *AuthService

	// M12 view-change 硬化：多數認證票集（>2/3 驗證人簽名才切輪）。
	vcMu          sync.Mutex
	viewChangeTk  *viewChangeTickets

	// P2P 網絡（可選；nil 時本節點不廣播，作為孤立節點運行）
	p2pNet *p2p.Network

	// 智能合約執行層（TAC VM）
	contracts *vm.ContractManager

	// Layer2 Optimistic Rollup（可選；nil=不啟用）
	l2    *rollup.Rollup
	l2svc *rollup.Service

	// 跨鏈橋（可選；nil=不啟用）
	bridge           *bridgepkg.Bridge
	bridgeNetworked  bool // 已配置守衛網絡（跨鏈提案經 P2P 傳播聚合多簽）
}

// New 創建節點：打開數據庫、重放動態難度、載入或生成節點密鑰。
func New(cfg *config.Config, nodeID string, baseDifficulty int) (*Node, error) {
	if cfg == nil {
		return nil, errors.New("node: 配置為空")
	}
	dataDir := cfg.DataDir
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("node: 創建數據目錄失敗: %w", err)
	}
	db, err := chaindb.Open(filepath.Join(dataDir, "chain.db"))
	if err != nil {
		return nil, err
	}

	bt := time.Duration(cfg.BlockTime) * time.Second
	if bt <= 0 {
		bt = 3 * time.Second
	}

	n := &Node{
		cfg:       cfg,
		db:        db,
		nodeID:    nodeID,
		blockTime: bt,
		baseDiff:  baseDifficulty,
		adjDiff:   baseDifficulty,
		stopCh:    make(chan struct{}),
		onlineSince: time.Now(),
	}

	// 錢包帳本（隨鏈同步；獨立於鏈庫的事務帳本）。
	ws, err := wallet.Open(dataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	n.walletSvc = wallet.NewService(ws)

	// 交易所撮合引擎（獨立帳本：ex_balances/ex_orders/ex_trades）。
	es, err := exchange.Open(dataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	n.exchangeSvc = exchange.NewService(es)

	// 內建社群（FB 風動態牆/市集/廣告）：community.db。
	cs, err := community.Open(dataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	n.communitySvc = cs

	// DeFi（流動性挖礦＋借貸市場）：defi.db，資產對接 wallet 帳本。
	ds, err := defi.Open(dataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	n.defiSvc = ds
	// 池/借貸市場初始流動性由鏈上基金注資（Reward 鑄造、僅在帳戶為空時執行，冪等）。
	if err := n.seedDefiLiquidity(); err != nil {
		_ = db.Close()
		return nil, err
	}

	// C2C 場外交易：c2c.db。
	cs2, err := c2c.Open(dataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	n.c2cSvc = cs2

	// 會員系統（註冊/登入/登出）：users.db。
	as, err := OpenAuth(dataDir)
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	n.authSvc = as

	// 節點密鑰：持久化於 node_key.json，否則新生成。
	if err := n.loadOrCreateKey(dataDir); err != nil {
		db.Close()
		return nil, err
	}
	// 節點自身註冊為默認礦機（算力 1 vCPU）：保證單節點下 coinbase 剩餘有歸屬。
	// 對齊原本方式：出塊補貼扣除獎勵池份額後，按在線礦工算力瓜分。
	if err := n.walletSvc.Store().RegisterMiner(n.nodeAddress, 1, 0); err != nil {
		db.Close()
		return nil, err
	}

	// BFT 驗證人：以本節點密鑰作為共識密鑰（單機驗證人集）。
	if err := n.initBFT(); err != nil {
		db.Close()
		return nil, err
	}

	// 動態難度：從鏈上歷史時間戳重放當前基礎難度。
	tip := db.GetTipHeight()
	n.adjDiff = difficulty.ReplayDifficulty(n.blockLookup, tip,
		baseDifficulty, bt.Seconds(), difficulty.RetargetInterval)

	// 智能合約管理器：持久化於 tac_evm.db。
	cm, err := vm.NewContractManager(filepath.Join(dataDir, "tac_evm.db"))
	if err != nil {
		db.Close()
		return nil, err
	}
	n.contracts = cm

	// 可選：啟用 Layer2 Optimistic Rollup（含後台批量/提交服務）。
	if cfg.EnableL2 {
		if err := n.AttachL2(); err != nil {
			_ = cm.Close()
			_ = db.Close()
			return nil, err
		}
	}

	// 可選：啟用跨鏈橋。
	if cfg.EnableBridge {
		if err := n.AttachBridge(); err != nil {
			_ = cm.Close()
			_ = db.Close()
			return nil, err
		}
	}

	return n, nil
}

// seedDefiLiquidity 為 DeFi 池與借貸市場注資初始流動性（Reward 鑄造、帳戶為空才執行，冪等）。
func (n *Node) seedDefiLiquidity() error {
	if n.defiSvc == nil || n.walletSvc == nil {
		return nil
	}
	pools, err := n.defiSvc.Pools()
	if err != nil {
		return err
	}
	for _, p := range pools {
		if err := n.seedAccountIfEmpty(defi.PoolAccount(p.ID), p.Token0, p.Reserve0, "defi:pool"+defi.PoolAccount(p.ID)+":seed"); err != nil {
			return err
		}
		if err := n.seedAccountIfEmpty(defi.PoolAccount(p.ID), p.Token1, p.Reserve1, "defi:pool"+defi.PoolAccount(p.ID)+":seed"); err != nil {
			return err
		}
	}
	markets, err := n.defiSvc.LendingMarkets()
	if err != nil {
		return err
	}
	for _, m := range markets {
		if err := n.seedAccountIfEmpty(defi.LendingAccount, m.Asset, m.TotalDeposit, "defi:lending:seed"); err != nil {
			return err
		}
	}
	vaults, err := n.defiSvc.Vaults()
	if err != nil {
		return err
	}
	for _, v := range vaults {
		if err := n.seedAccountIfEmpty(defi.VaultAccount(v.ID), v.UnderlyingAsset, v.TotalAssets, "defi:vault"+defi.VaultAccount(v.ID)+":seed"); err != nil {
			return err
		}
	}
	return nil
}

// seedAccountIfEmpty 帳戶為空時以 Reward 注資指定金額（冪等）。
func (n *Node) seedAccountIfEmpty(account, asset string, amount float64, memo string) error {
	acc, err := n.walletSvc.Balance(account)
	if err != nil {
		return fmt.Errorf("defi: 查詢注資帳戶 %s: %w", account, err)
	}
	amts, err := wallet.NewAmount(amountStr(amount), wallet.Asset(asset))
	if err != nil {
		return fmt.Errorf("defi: 注資金額無效 %s/%s: %w", account, asset, err)
	}
	if accountBalanceIsZero(acc, wallet.Asset(asset)) {
		if err := n.walletSvc.Reward(account, wallet.Asset(asset), amts, memo); err != nil {
			return fmt.Errorf("defi: 注資 %s %s: %w", account, asset, err)
		}
	}
	return nil
}

func amountStr(f float64) string {
	return fmt.Sprintf("%.6f", f)
}

// accountBalanceIsZero 判斷帳戶某資產餘額是否為零（TACm 為 big.Int、穩定幣為 int64）。
func accountBalanceIsZero(a *wallet.Account, asset wallet.Asset) bool {
	switch asset {
	case wallet.AssetTACm:
		return a.TACmBalance == nil || a.TACmBalance.Sign() == 0
	case wallet.AssetTiUSD:
		return a.TiUSDBalance == 0
	default:
		return a.USDTBalance == 0
	}
}

func (n *Node) loadOrCreateKey(dataDir string) error {
	keyPath := filepath.Join(dataDir, "node_key.json")
	if raw, err := os.ReadFile(keyPath); err == nil {
		var kd struct {
			PrivateKey string `json:"private_key"`
			Address    string `json:"address"`
		}
		if err := json.Unmarshal(raw, &kd); err != nil {
			return fmt.Errorf("node: 解析節點密鑰失敗: %w", err)
		}
		pk, err := hex.DecodeString(kd.PrivateKey)
		if err != nil {
			return fmt.Errorf("node: 節點私鑰非法: %w", err)
		}
		kp, err := crypto.KeyPairFromPrivateKey(pk)
		if err != nil {
			return err
		}
		n.keypair = kp
	} else if errors.Is(err, os.ErrNotExist) {
		kp, err := crypto.GenerateKeyPair()
		if err != nil {
			return err
		}
		addr, _ := kp.Address()
		kd := map[string]string{"private_key": hex.EncodeToString(kp.PrivateKey()), "address": addr}
		raw, _ := json.MarshalIndent(kd, "", "  ")
		if err := os.WriteFile(keyPath, raw, 0o600); err != nil {
			return fmt.Errorf("node: 寫入節點密鑰失敗: %w", err)
		}
		n.keypair = kp
	} else {
		return fmt.Errorf("node: 讀取節點密鑰失敗: %w", err)
	}
	addr, err := n.keypair.Address()
	if err != nil {
		return err
	}
	n.nodeAddress = addr
	return nil
}

// blockLookup 適配動態難度包的 BlockLookup 回調。
func (n *Node) blockLookup(height int64) (float64, bool) {
	b, err := n.db.GetBlock(height)
	if err != nil || b == nil {
		return 0, false
	}
	return float64(b.Ts), true
}

// Address 返回節點的鏈上地址。
func (n *Node) Address() string { return n.nodeAddress }

// IdentityPubHex 返回節點壓縮公鑰十六進制（用於創世驗證人規格）。
func (n *Node) IdentityPubHex() string {
	return hex.EncodeToString(n.keypair.PublicKeyCompressed())
}

// DB 暴露底層存儲（供 RPC 層查詢）。
func (n *Node) DB() *chaindb.ChainDB { return n.db }

// Start 啟動出塊循環。
func (n *Node) Start() {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return
	}
	n.running = true
	n.mu.Unlock()
	n.wg.Add(2)
	go n.consensusLoop()
	go n.runBFT()
}

// StartAsFollower 以跟隨/全節點模式啟動：不主動 PoW 出塊，
// 僅參與 BFT 投票，並通過 P2P gossip/同步接收區塊。
func (n *Node) StartAsFollower() {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return
	}
	n.running = true
	n.mu.Unlock()
	n.wg.Add(1)
	go n.runBFT()
}

// StartDistributed 以分佈式 BFT 模式啟動：每個高度僅輪值 proposer
// PoW 出塊並廣播，其餘驗證人接收區塊並投票；最終性由跨節點聚合
// >2/3 precommit 達成；提議超時自動 view change（round+1）。
func (n *Node) StartDistributed() {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return
	}
	n.running = true
	n.distributed = true
	n.consensusWake = make(chan struct{}, 8)
	n.mu.Unlock()
	n.wg.Add(2)
	go n.consensusLoop()
	go n.runBFT()
}

// Close 停止出塊循環並關閉數據庫。
func (n *Node) Close() error {
	n.mu.Lock()
	if !n.running {
		n.mu.Unlock()
		if n.contracts != nil {
			n.contracts.Close()
		}
		if n.l2svc != nil {
			n.l2svc.Close()
		}
		if n.l2 != nil {
			_ = n.l2.Close()
		}
		if n.bridge != nil {
			_ = n.bridge.Close()
		}
		if n.walletSvc != nil {
			_ = n.walletSvc.Store().Close()
		}
		if n.exchangeSvc != nil {
			_ = n.exchangeSvc.Store().Close()
		}
		return n.db.Close()
	}
	n.running = false
	n.mu.Unlock()
	close(n.stopCh)
	n.wg.Wait()
	if n.contracts != nil {
		n.contracts.Close()
	}
	if n.l2svc != nil {
		n.l2svc.Close()
	}
	if n.l2 != nil {
		_ = n.l2.Close()
	}
	if n.bridge != nil {
		_ = n.bridge.Close()
	}
	if n.walletSvc != nil {
		_ = n.walletSvc.Store().Close()
	}
	if n.defiSvc != nil {
		_ = n.defiSvc.Close()
	}
	if n.c2cSvc != nil {
		_ = n.c2cSvc.Close()
	}
	if n.authSvc != nil {
		_ = n.authSvc.Close()
	}
	return n.db.Close()
}

// waitFor 可中斷地等待 d；收到停止信號立即返回 false。
func (n *Node) waitFor(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-n.stopCh:
		return false
	case <-t.C:
		return true
	}
}

// consensusLoop 是出塊主循環：動態間隔 → 取池 → PoW → 驗證 → 寫塊 → 調難度。
func (n *Node) consensusLoop() {
	defer n.wg.Done()
	cs := &consensusState{lastRound: -1}
	for {
		select {
		case <-n.stopCh:
			return
		default:
		}

		// 動態出塊間隔：有交易快速（1s），無交易正常間隔。
		wait := n.blockTime
		if n.db.MempoolSize() > 0 {
			wait = fastBlockTime
		}
		if !n.waitConsensus(wait) {
			return
		}

		// 分佈式 BFT：提議超時 → view change；新輪次到來時立即重試出塊。
		n.advanceConsensusRound(cs)

		if err := n.produceBlock(); err != nil {
			if errors.Is(err, errNotProposer) {
				continue // 非本輪提議者，回到頂部等待網絡塊/view-change
			}
			// 單輪失敗不致命，下一輪重試。
			fmt.Fprintf(os.Stderr, "[consensus] produce error: %v\n", err)
			n.waitFor(time.Second)
		}
	}
}

// consensusState 為 consensusLoop 跨輪保留的輪次計時狀態。
type consensusState struct {
	target     int64
	since      time.Time
	lastRound  int32
}

// waitConsensus 等待下一個出塊時機；分佈式模式下可被 view-change 喚醒。
func (n *Node) waitConsensus(wait time.Duration) bool {
	if !n.isDistributed() {
		return n.waitFor(wait)
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-n.stopCh:
		return false
	case <-n.consensusWake:
		return true
	case <-t.C:
		return true
	}
}

// wakeConsensus 非阻塞喚醒 consensusLoop。
func (n *Node) wakeConsensus() {
	if n.consensusWake == nil {
		return
	}
	select {
	case n.consensusWake <- struct{}{}:
	default:
	}
}

func (n *Node) isDistributed() bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.distributed
}

// consensusTimeout 返回某輪次的提議超時：base×2^round，上限 60s。
func (n *Node) consensusTimeout(round int32) time.Duration {
	base := 3 * n.blockTime
	if base < 2*time.Second {
		base = 2 * time.Second
	}
	if round <= 0 {
		return base
	}
	if round > 30 {
		return 60 * time.Second
	}
	d := base * (1 << round)
	if d > 60*time.Second {
		return 60 * time.Second
	}
	return d
}

// advanceConsensusRound 推進共識輪次：確保 BFT 進入目標高度；若該高度
// 提議超時則 round+1 並廣播 view-change。狀態存於 cs 供下輪使用。
func (n *Node) advanceConsensusRound(cs *consensusState) {
	if !n.isDistributed() {
		return
	}
	tip := n.db.GetTipHeight()
	target := tip + 1
	if target != cs.target {
		// 進入新高度：重置計時。
		cs.target = target
		cs.since = time.Now()
		cs.lastRound = -1
	}
	if target <= 0 {
		return
	}

	// 若 BFT 尚未啟動該高度（提案未到），先 StartHeight 以便輪次推進。
	if h, _, _ := n.bftNode.RoundInfo(); h != target {
		n.bftNode.StartHeight(target)
	}
	_, round, _ := n.bftNode.RoundInfo()
	if round != cs.lastRound {
		cs.since = time.Now()
		cs.lastRound = round
	}

	// 提案已到達（tip ≥ target）：下輪自然進入新高度。
	if tip >= target {
		return
	}

	// 超時：M12 硬化——本節點發 view-change 票（簽名廣播），等待 >2/3 多數認證後切輪。
	// 非分佈式單節點直接切輪（無需網絡認證）。
	if time.Since(cs.since) > n.consensusTimeout(round) {
		if !n.isDistributed() {
			n.bftNode.MoveToNextRound()
			_, newRound, _ := n.bftNode.RoundInfo()
			cs.since = time.Now()
			cs.lastRound = newRound
			return
		}
		n.proposeViewChange(target)
		// 若多數已在（例如其他節點的票已到），recordViewChangeTicket 已切輪；
		// 否則等待網絡聚合。重設計時避免每 tick 重複發票。
		cs.since = time.Now()
	}
}

func (n *Node) produceBlock() error {
	latest, err := n.db.GetLatestBlock()
	if err != nil {
		return err
	}
	var height int64 = 1
	var prevPtr *string
	var prevForHeader any // Canonical 僅接受 nil/string，不接受指針
	if latest != nil {
		height = latest.Height + 1
		h := latest.Hash
		prevPtr = &h
		prevForHeader = h
	}

	// 分佈式 BFT：僅輪值 proposer 出塊，其餘節點等待網絡傳來的提議。
	// 使用當前 BFT round（view change 後由下一個 proposer 接棒）。
	if n.isDistributed() {
		_, round, _ := n.bftNode.RoundInfo()
		proposer := n.vset.Proposer(height, round)
		if proposer == nil || proposer.Address != n.nodeAddress {
			return errNotProposer
		}
	}

	mempool, err := n.db.GetMempool(maxTxPerBlock)
	if err != nil {
		return err
	}

	ts := time.Now().Unix()
	txHashes := make([]string, 0, len(mempool)+1)
	txs := make([]chaindb.Transaction, 0, len(mempool)+1)

	// coinbase 增發交易置於首位（height>0），其哈希進入 Merkle 根。
	// 供應模型：TACM_MAX_SUPPLY 設定後依年衰減發行（上限 52,003,300）；未設定維持減半模式。
	if height > 0 {
		cb := chaindb.BuildCoinbaseTx(height, n.nodeAddress, ts)
		if cfg, err := chaindb.LoadEmission(int64(n.cfg.BlockTime)); err == nil && cfg.Model == "annual_decay" {
			// 上限模式：單塊獎勵由年衰減模型決定（鏈上帳本一致性）。
			cb.Amount = chaindb.FormatFloat(chaindb.EmissionAmount(height, int64(n.cfg.BlockTime)))
			cb.TxHash = chaindb.CoinbaseTxHash(height, n.nodeAddress, chaindb.EmissionAmount(height, int64(n.cfg.BlockTime)), ts)
		}
		txs = append(txs, cb)
		txHashes = append(txHashes, cb.TxHash)
	}

	for _, m := range mempool {
		if h := getString(m, "tx_hash"); h != "" {
			txHashes = append(txHashes, h)
		}
		txs = append(txs, txFromMap(m))
	}
	mroot, err := crypto.MerkleRootStrings(txHashes)
	if err != nil {
		return err
	}
	header := map[string]any{
		"height":      height,
		"prev_hash":   prevForHeader,
		"merkle_root": mroot,
		"proposer":    n.nodeID,
		"ts":          ts,
		"tx_count":    len(txs),
	}

	// PoW：當前有效難度，cap 8；失敗逐級降級重試（最多3次）。
	n.mu.Lock()
	diff := n.adjDiff
	n.mu.Unlock()
	if diff > difficulty.MaxDifficulty {
		diff = difficulty.MaxDifficulty
	}
	maxNonce := 200000
	if diff > 2 {
		maxNonce = 500000
	}

	pow, err := crypto.MineBlock(header, diff, maxNonce)
	for retries := 0; pow == nil && retries < 3 && diff > 1; retries++ {
		diff--
		pow, err = crypto.MineBlock(header, diff, maxNonce)
	}
	if err != nil {
		return fmt.Errorf("挖礦失敗: %w", err)
	}
	if pow == nil {
		return nil // 本輪未找到 nonce，跳過
	}

	header["nonce"] = pow.Nonce
	header["difficulty"] = diff
	ok, err := crypto.VerifyPow(header, diff)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("PoW 驗證失敗")
	}

	block := &chaindb.Block{
		Height:          height,
		Hash:            pow.Hash,
		PrevHash:        prevPtr,
		MerkleRoot:      mroot,
		Proposer:        n.nodeID,
		ProposerAddress: n.nodeAddress,
		Ts:              ts,
		TxCount:         len(txs),
		Difficulty:      diff,
		Nonce:           int64(pow.Nonce),
	}
	if err := n.db.InsertBlock(block, txs); err != nil {
		return err
	}

	// 執行塊內合約交易（部署/調用），持久化 code/storage。
	if err := n.executeBlockContracts(txs); err != nil {
		return err
	}

	// 錢包帳本同步（coinbase 獎勵入帳 + 交易轉帳 + 手續費）。
	if err := n.syncWallet(height, txs); err != nil {
		return fmt.Errorf("錢包同步 h=%d: %w", height, err)
	}
	// 本節點接受新塊，啟動/推進 BFT 最終性投票。
	n.onNewBlock(height, block.Hash)

	// 若已聯網，向 P2P 網絡廣播新塊（含 coinbase 與打包交易）。
	if n.p2pNet != nil {
		n.p2pNet.BroadcastBlock(block, txs)
	}

	// 調整點：按剛完成窗口重算基礎難度。
	if difficulty.ShouldRetarget(height, difficulty.RetargetInterval) {
		win := difficulty.WindowTimestampsForHeight(
			n.blockLookup, height, difficulty.RetargetInterval)
		n.mu.Lock()
		newBase := difficulty.ComputeRetarget(n.adjDiff, win, n.blockTime.Seconds())
		n.adjDiff = newBase
		n.mu.Unlock()
	}
	return nil
}
