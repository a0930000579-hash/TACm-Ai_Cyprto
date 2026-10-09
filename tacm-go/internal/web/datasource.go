package web

import (
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"

	"tacm/internal/chaindb"
	"tacm/internal/node"
	"tacm/internal/wallet"
)

// DataSource 抽象 Web 層所需的鏈數據，便於以節點實現或測試替身注入。
type DataSource interface {
	Status() StatusView
	Blocks(limit int) ([]BlockView, error)
	Block(height int64) (*BlockView, error)
	Transaction(hash string) (*TxView, error)
	Address(addr string) (*AddressView, error)
	Wallet(addr string) (*WalletView, error)
	ChainStats() ChainStatsView
	// M37：讀取登入會員（session），供錢包/首頁等頁面依會員綁定地址渲染。
	CurrentUser(r *http.Request) (*node.AuthUser, error)
}

type nodeDS struct{ n *node.Node }

// CurrentUser 讀取登入會員（session cookie）。
func (d *nodeDS) CurrentUser(r *http.Request) (*node.AuthUser, error) {
	return d.n.Auth().CurrentUser(r)
}

// NewNodeDataSource 把運行中的節點適配為 Web 數據源。
func NewNodeDataSource(n *node.Node) DataSource { return &nodeDS{n: n} }

// walletView 把錢包服務快照轉為 Web 視圖。
func (d *nodeDS) Wallet(addr string) (*WalletView, error) {
	if d.n.Wallet() == nil {
		return nil, nil // 錢包未啟用：頁面顯示提示
	}
	acc, err := d.n.Wallet().Balance(addr)
	if err != nil {
		return nil, err
	}
	led, err := d.n.Wallet().Ledger(30)
	if err != nil {
		return nil, err
	}
	sup, _ := d.n.Wallet().TiUSDSummary()
	synced, _ := d.n.Wallet().SyncedHeight()
	w := &WalletView{
		Address: addr,
		// M32：錢包只顯示個人資產（十進制縮放）——raw 不再外洩。
		TACm:        wallet.FormatAmountBig(acc.TACmBalance),
		TiUSD:       wallet.FormatAmountI64(acc.TiUSDBalance, wallet.AssetTiUSD),
		USDT:        wallet.FormatAmountI64(acc.USDTBalance, wallet.AssetUSDT),
		SyncedBlock: synced,
		FeeTiUSDBps: 50, FeeUSDTBps: 125, FeeTACmBps: 200,
		NodeAddress: d.n.Address(),
	}
	// TiUSD 供給（最小單位 → 十進制）。
	if sup != nil {
		w.TiUSDSupply = sup.Supply
		w.TiUSDMinted = sup.TotalMinted
		w.TiUSDBurned = sup.TotalBurned
	}
	for _, e := range led {
		// M33：帳本變動以十進制顯示（raw 不再外洩）；保留 +/− 符號。
		w.Ledger = append(w.Ledger, WalletLedgerView{
			Ts: e.Ts, Kind: string(e.Kind), Asset: string(e.Asset),
			Account: e.Account, Delta: formatDelta(e.Delta, string(e.Asset)), Memo: e.Memo,
		})
	}
	return w, nil
}

// formatDelta 帳本變動縮放（依資產精度），保留符號。
func formatDelta(delta, asset string) string {
	v, ok := new(big.Int).SetString(delta, 10)
	if !ok {
		return delta
	}
	neg := v.Sign() < 0
	if neg {
		v = new(big.Int).Neg(v)
	}
	var out string
	switch wallet.Asset(asset) {
	case wallet.AssetTiUSD:
		out = wallet.FormatAmountBig(v)
	default: // TACm/USDT 18 位
		out = wallet.FormatAmountBig(v)
	}
	if neg {
		out = "-" + out
	}
	return out
}

func txView(t *chaindb.Transaction) TxView {
	return TxView{
		Hash: t.TxHash, BlockHeight: t.BlockHeight, TxIndex: t.TxIndex,
		From: t.FromAddr, To: t.ToAddr,
		Amount: t.Amount, Fee: t.Fee, Nonce: t.Nonce, Ts: t.Ts,
		Memo: t.Memo, Status: t.Status,
		Signature: t.Signature, Pubkey: t.Pubkey,
		Contract: parseContractMemo(t.Memo),
	}
}

// parseContractMemo 解析合約交易 memo（與節點 splitContractMemo 同格式）：
// vm:deploy:<gas>:<hex> / vm:call:<gas>:<hex> 新格式與 vm:deploy:<hex> /
// vm:call:<hex> 舊格式；非合約 memo 返回 nil。
func parseContractMemo(memo string) *ContractView {
	var kind, rest string
	switch {
	case strings.HasPrefix(memo, "vm:deploy:"):
		kind, rest = "deploy", memo[len("vm:deploy:"):]
	case strings.HasPrefix(memo, "vm:call:"):
		kind, rest = "call", memo[len("vm:call:"):]
	default:
		return nil
	}
	cv := &ContractView{Kind: kind}
	if i := strings.IndexByte(rest, ':'); i >= 0 {
		if g, err := strconv.ParseUint(rest[:i], 10, 64); err == nil {
			cv.Gas = g
		}
		cv.Data = rest[i+1:]
	} else {
		cv.Data = rest
	}
	return cv
}

func blockView(b *chaindb.Block) BlockView {
	prev := ""
	if b.PrevHash != nil {
		prev = *b.PrevHash
	}
	bv := BlockView{
		Height: b.Height, Hash: b.Hash, PrevHash: prev, MerkleRoot: b.MerkleRoot,
		Proposer: b.Proposer, ProposerAddress: b.ProposerAddress, Ts: b.Ts,
		TxCount: b.TxCount, Difficulty: b.Difficulty, Nonce: b.Nonce, Size: b.Size,
	}
	if b.Height > 0 {
		bv.PrevHeight = b.Height - 1
	}
	return bv
}

// ChainStatsView 首頁全鏈儀表（與 /api/chain/stats 同口徑：總產出僅計 coinbase 出塊分配）。
type ChainStatsView struct {
	TotalMinedTacm string `json:"total_mined_tacm"`
	TiUSDSupply    string `json:"tiusd_supply"`
	RewardPool     string `json:"reward_pool"`
	OnlineMiners   int    `json:"online_miners"`
	TotalHashrate  string `json:"total_hashrate"`
}

// ChainStats 全鏈統計（M34）：總產出＝coinbase :miner/:proposer/:pool 累計；
// 獎勵池＝reward_pool 帳戶餘額；TiUSD＝供給；全網算力＝窗口內在線礦工 hashrate 和。
func (d *nodeDS) ChainStats() ChainStatsView {
	out := ChainStatsView{}
	if d.n.Wallet() == nil {
		return out
	}
	total, err := d.n.Wallet().Store().ChainMinedTacm()
	if err != nil {
		total = big.NewInt(0)
	}
	out.TotalMinedTacm = wallet.FormatAmountBig(total)
	if acc, err := d.n.Wallet().Balance(wallet.RewardPoolAddr); err == nil {
		out.RewardPool = wallet.FormatAmountBig(acc.TACmBalance)
	}
	if sup, err := d.n.Wallet().TiUSDSummary(); err == nil && sup != nil {
		// M38.1：TiUSD 最小單位為 1e6（micro），須以 TiUSD 精度縮放，不能用 TACm 的 1e18（FormatAmountBig）。
		out.TiUSDSupply = wallet.FormatAmountI64(sup.Supply, wallet.AssetTiUSD)
	}
	if ms, err := d.n.Wallet().Store().Miners(); err == nil {
		var totalHr float64
		on := 0
		for _, m := range ms {
			if m.Online {
				on++
				if v, ok := new(big.Int).SetString(m.Hashrate, 10); ok {
					f, _ := new(big.Float).SetInt(v).Float64()
					totalHr += f
				}
			}
		}
		out.OnlineMiners = on
		out.TotalHashrate = fmt.Sprintf("%.2fM", totalHr/1e6)
	}
	return out
}

func (d *nodeDS) Status() StatusView {
	s := d.n.GetStatus()
	return StatusView{
		NodeID: s.NodeID, Address: s.Address, Height: s.BlockHeight,
		FinalizedHeight: d.n.FinalizedHeight(), Difficulty: s.Difficulty,
		MempoolSize: int(s.MempoolSize), UptimeSec: s.UptimeSec,
		Consensus: s.Consensus,
	}
}

func (d *nodeDS) Blocks(limit int) ([]BlockView, error) {
	blocks, err := d.n.DB().GetBlocks(limit, 0)
	if err != nil {
		return nil, err
	}
	out := make([]BlockView, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, blockView(b))
	}
	return out, nil
}

func (d *nodeDS) Block(height int64) (*BlockView, error) {
	detail, err := d.n.GetBlockDetail(height)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, nil
	}
	bv := blockView(detail.Block)
	bv.Transactions = make([]TxView, 0, len(detail.Transactions))
	for _, t := range detail.Transactions {
		bv.Transactions = append(bv.Transactions, txView(t))
	}
	return &bv, nil
}

func (d *nodeDS) Transaction(hash string) (*TxView, error) {
	t, err := d.n.DB().GetTransaction(hash)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	tv := txView(t)
	return &tv, nil
}

func (d *nodeDS) Address(addr string) (*AddressView, error) {
	av := &AddressView{Address: addr, Balance: "0"}
	acc, err := d.n.DB().GetAccount(addr)
	if err != nil {
		return nil, err
	}
	if acc != nil {
		av.Balance = acc.Balance
		av.Nonce = acc.Nonce
	}
	txs, err := d.n.DB().GetTransactionsByAddress(addr, 50)
	if err != nil {
		return nil, err
	}
	av.Transactions = make([]TxView, 0, len(txs))
	for _, t := range txs {
		av.Transactions = append(av.Transactions, txView(t))
	}
	return av, nil
}
