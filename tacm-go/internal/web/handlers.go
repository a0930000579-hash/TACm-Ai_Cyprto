package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flosch/pongo2/v6"

	"tacm/internal/wallet"
)

// writeJSON 寫出 JSON 回應（M39 健康/指標 API 共用）。
func writeJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	blocks, err := s.ds.Blocks(10)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "index.html", pongo2.Context{"blocks": blocks, "chain": s.ds.ChainStats()})
}

// handleExplorer GET /explorer — 公開區塊瀏覽器（M62）：全鏈統計＋搜尋＋最新區塊/交易。
// 資料全部來自鏈上（Blocks/RecentTransactions/Status/ChainStats），任何節點同鏈結果一致。
func (s *Server) handleExplorer(w http.ResponseWriter, r *http.Request) {
	blocks, err := s.ds.Blocks(15)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	txs, err := s.ds.RecentTransactions(15)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "explorer.html", pongo2.Context{
		"blocks": blocks, "txs": txs, "chain": s.ds.ChainStats(),
	})
}

// handleSearchAPI GET /api/search?q= — 瀏覽器搜尋（M62）：數字→區塊、tx0 地址→地址頁、
// 其餘先查交易 hash→交易頁；均未命中→type=none。前端依 type 跳轉對應詳情頁。
func (s *Server) handleSearchAPI(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json")
	if q == "" {
		writeJSON(w, map[string]any{"ok": false, "type": "none", "q": q})
		return
	}
	if h, err := strconv.ParseInt(q, 10, 64); err == nil && h >= 0 {
		writeJSON(w, map[string]any{"ok": true, "type": "block", "height": h})
		return
	}
	if strings.HasPrefix(q, "tx0") {
		writeJSON(w, map[string]any{"ok": true, "type": "address", "address": q})
		return
	}
	if t, err := s.ds.Transaction(q); err == nil && t != nil {
		writeJSON(w, map[string]any{"ok": true, "type": "tx", "hash": q, "block_height": t.BlockHeight})
		return
	}
	writeJSON(w, map[string]any{"ok": false, "type": "none", "q": q})
}

// handlePeersAPI GET /api/peers — 公開 seed 列表（M63/M65）：新節點加入網絡的權威入口。
// 列表來自單一權威來源（mainnet 固定編譯、testnet 由 TACM_TESTNET_SEEDS 配置），
// 同一網段內任何入口看到的一致。
func (s *Server) handlePeersAPI(w http.ResponseWriter, r *http.Request) {
	chainID := "tacm-mainnet-1"
	if s.network == "testnet" {
		chainID = "tacm-testnet-1"
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"network":  s.network,
		"chain_id": chainID,
		"seeds":    s.serverSeeds(),
	})
}

// handleJoin GET /join — 節點加入指引頁（M63/M65）：依網段展示 seed 列表、一鍵指令與節點要求。
func (s *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	s.render(w, "join.html", pongo2.Context{
		"seeds":   s.serverSeeds(),
		"repo":    "https://github.com/a0930000579-hash/TACm-Ai_Cyprto",
		"network": s.network,
	})
}

// nodeStatusResp 對應鏈節點 HTTP /status 回應（M64 監控探測用）。
// 注意：節點 /status 的 JSON 欄位名是 block_height/effective_difficulty，
// 與 Web 內部 StatusView（height/difficulty）不同——探測外部節點必須用此協議結構。
type nodeStatusResp struct {
	NodeID             string `json:"node_id"`
	Address            string `json:"address"`
	Network            string `json:"network"`
	ChainID            string `json:"chain_id"`
	BlockHeight        int64  `json:"block_height"`
	FinalBlockHeight   int64  `json:"final_block_height"`
	MempoolSize        int    `json:"mempool_size"`
	EffectiveDifficulty int   `json:"effective_difficulty"`
	UptimeSec          int64  `json:"uptime_sec"`
	Consensus          string `json:"consensus"`
}

// probeSeed 探測單一 seed 節點（M64）：GET {seed}/status、2 秒超時。
// 並行探測由呼叫方以 Goroutine 執行（Go 慣用併發寫法），本函式只做單點探測。
func probeSeed(sd SeedInfo) map[string]any {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(sd.URL + "/status")
	base := map[string]any{"node_id": sd.NodeID, "url": sd.URL, "role": sd.Role, "reachable": false}
	if err != nil {
		base["error"] = "unreachable"
		return base
	}
	defer resp.Body.Close()
	var st nodeStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		base["error"] = "bad_status"
		return base
	}
	base["reachable"] = true
	base["status"] = st
	return base
}

// handleNetworkAPI GET /api/network — 全網狀態統一視界（M64/M65）：
// 並行探測所有公開 seed 節點的 /status，加上本節點狀態，一次回傳全網節點健康度。
// seed 列表來自單一權威來源（依網段），任何節點/入口看到的節點集合一致。
func (s *Server) handleNetworkAPI(w http.ResponseWriter, r *http.Request) {
	seeds := s.seedURLs
	results := make([]map[string]any, len(seeds))
	var wg sync.WaitGroup
	for i, sd := range seeds {
		wg.Add(1)
		go func(i int, sd SeedInfo) {
			defer wg.Done()
			results[i] = probeSeed(sd)
		}(i, sd)
	}
	wg.Wait()
	chainID := "tacm-mainnet-1"
	if s.network == "testnet" {
		chainID = "tacm-testnet-1"
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"network":  s.network,
		"chain_id": chainID,
		"self":     s.ds.Status(),
		"seeds":    results,
	})
}

// handleNetwork GET /network — 節點健康度監控頁（M64）：頁面以 JS 定時拉取 /api/network。
func (s *Server) handleNetwork(w http.ResponseWriter, r *http.Request) {
	s.render(w, "network.html", pongo2.Context{})
}

func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	h, err := strconv.ParseInt(r.PathValue("height"), 10, 64)
	if err != nil {
		http.Error(w, "invalid height", http.StatusBadRequest)
		return
	}
	b, err := s.ds.Block(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if b == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "block.html", pongo2.Context{"b": b})
}

// handleBlockAPI GET /api/block/{height} — 鏈上審計 JSON（M66）：
// 回傳區塊頭與完整交易（含 coinbase 瓜分明細），供 tacctl audit 逐塊驗證
// 「coinbase 總額 == BlockReward」——任何入口看到同一份鏈上數據。
func (s *Server) handleBlockAPI(w http.ResponseWriter, r *http.Request) {
	h, err := strconv.ParseInt(r.PathValue("height"), 10, 64)
	if err != nil {
		http.Error(w, "invalid height", http.StatusBadRequest)
		return
	}
	bd, err := s.ds.BlockDetail(h)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if bd == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, bd)
}

func (s *Server) handleAddress(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("address")
	a, err := s.ds.Address(addr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "address.html", pongo2.Context{"a": a})
}

func (s *Server) handleTx(w http.ResponseWriter, r *http.Request) {
	hash := r.PathValue("hash")
	t, err := s.ds.Transaction(hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		http.NotFound(w, r)
		return
	}
	s.render(w, "tx.html", pongo2.Context{"t": t})
}

// handleDashboard GET /dashboard — 幣安風系統管理儀表板（聚合鏈/錢包/交易所/獎勵池/L2/橋）。
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	s.render(w, "dashboard.html", pongo2.Context{
		"node": s.ds.Status(),
	})
}

// handleExchange GET /exchange — 幣安風交易所頁面（行情/撮合/閃兌/機器人）。
func (s *Server) handleExchange(w http.ResponseWriter, r *http.Request) {
	s.render(w, "exchange.html", pongo2.Context{
		"exchange": map[string]any{"NodeAddress": s.ds.Status().Address},
	})
}

// handleMining GET /mining — 挖礦中心：節點/出塊/PoW/coinbase 分潤/獎勵池/驗證人。
// 實時數據由前端直接讀取節點 RPC（與 exchange 頁一致：?rpc= 可覆蓋）。
func (s *Server) handleMining(w http.ResponseWriter, r *http.Request) {
	s.render(w, "mining.html", pongo2.Context{})
}

// handleCommunity GET /community — 內建社群（FB 風動態牆/市集/廣告，App 化）。
// 實時數據由前端直接讀取節點 RPC（?rpc= 可覆蓋）。
func (s *Server) handleCommunity(w http.ResponseWriter, r *http.Request) {
	s.render(w, "community.html", pongo2.Context{})
}

// handleAI GET /ai — AI 服務：鏈上智能助手（M73-A）。
// 實時回答由前端直接讀取節點 RPC（/api/ai/ask，?rpc= 可覆蓋）。
func (s *Server) handleAI(w http.ResponseWriter, r *http.Request) {
	s.render(w, "ai.html", pongo2.Context{})
}

// handleGovernance GET /governance — 鏈上治理（提案/投票/參數，App 化）。
// 實時數據由前端直接讀取節點 RPC（?rpc= 可覆蓋）。
func (s *Server) handleGovernance(w http.ResponseWriter, r *http.Request) {
	s.render(w, "governance.html", pongo2.Context{})
}

// handleDefi GET /defi — DeFi 中心（流動性挖礦＋借貸市場，App 化）。
// 實時數據由前端直接讀取節點 RPC（?rpc= 可覆蓋）。
func (s *Server) handleDefi(w http.ResponseWriter, r *http.Request) {
	s.render(w, "defi.html", pongo2.Context{})
}

// handleC2C GET /c2c — C2C 場外交易（法幣兌加密貨幣，App 化）。
func (s *Server) handleC2C(w http.ResponseWriter, r *http.Request) {
	s.render(w, "c2c.html", pongo2.Context{})
}

// handleTokens GET /tokens — 代幣工作室（Token Studio）：鏈上標準代幣發行
// （節點官方金鑰代簽）＋列表/詳情/轉帳，App 化。實時數據由前端讀取 RPC。
func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	s.render(w, "tokens.html", pongo2.Context{})
}

// handleNFTs GET /nfts — NFT 工作室（NFT Studio）：鏈上標準 NFT（ERC-721 風格）
// 發行/mint/查詢/轉移（M74-2）。實時數據由前端讀取 RPC。
func (s *Server) handleNFTs(w http.ResponseWriter, r *http.Request) {
	s.render(w, "nfts.html", pongo2.Context{})
}

// handleDex GET /dex — 鏈上 DEX（AMM 恆定乘積）：池列表/建池/流動性/即時兌換，
// 與 Token Studio 打通（發幣→建池→交易閉環）。實時數據由前端讀取 RPC。
func (s *Server) handleDex(w http.ResponseWriter, r *http.Request) {
	s.render(w, "dex.html", pongo2.Context{})
}

// handleWallet GET /wallet?address=tx0xxx — 幣安風多資產錢包頁面。
func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")
	if addr == "" {
		// M37：登入會員優先顯示「會員自己的鏈上錢包」；未登入訪客由前端覆寫訪客獨立地址。
		if u, err := s.ds.CurrentUser(r); err == nil && u != nil && u.WalletAddr != "" {
			addr = u.WalletAddr
		} else {
			addr = s.ds.Status().Address
		}
	}
	wv, err := s.ds.Wallet(addr)
	if err != nil {
		http.Error(w, "錢包讀取失敗: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// M37：「我的地址」渲染當前視圖地址（登入會員＝會員鏈上地址；訪客＝節點地址 fallback，由前端覆寫訪客獨立地址）。
	wv.NodeAddress = addr
	// 鏈上獎勵池餘額（coinbase 15% 挹注 + 交易所手續費結算）。
	pool := "0"
	if poolAcct, err := s.ds.Wallet(wallet.RewardPoolAddr); err == nil {
		pool = poolAcct.TACm
	}
	s.render(w, "wallet.html", pongo2.Context{
		"wallet": wv, "address": addr, "addrParam": addr, "rewardPool": pool,
	})
}

// handleAuthLoginPage 會員登入頁（幣安風；JS 呼叫 /api/auth/login）。
func (s *Server) handleAuthLoginPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "auth.html", pongo2.Context{"activeTab": "auth"})
}

// handleAuthRegisterPage 會員註冊頁（JS 呼叫 /api/auth/register）。
func (s *Server) handleAuthRegisterPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "auth.html", pongo2.Context{"activeTab": "auth"})
}

// handleHealth GET /api/health — 節點存活探針（監控告警/負載均衡用）。
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	st := s.ds.Status()
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"ok":                 true,
		"node_id":            st.NodeID,
		"address":            st.Address,
		"block_height":       st.Height,
		"final_block_height": st.FinalizedHeight,
		"uptime_sec":         st.UptimeSec,
		"consensus":          st.Consensus,
	})
}

// handleMetrics GET /api/metrics — 節點運行指標（區塊高度/難度/礦工/算力/供應）。
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	st := s.ds.Status()
	cs := s.ds.ChainStats()
	w.Header().Set("Content-Type", "application/json")
	writeJSON(w, map[string]any{
		"ok":               true,
		"height":           st.Height,
		"finalized_height": st.FinalizedHeight,
		"difficulty":       st.Difficulty,
		"mempool_size":     st.MempoolSize,
		"uptime_sec":       st.UptimeSec,
		"consensus":        st.Consensus,
		"online_miners":    cs.OnlineMiners,
		"network_hashrate": cs.TotalHashrate,
		"total_mined_tacm": cs.TotalMinedTacm,
		"tiusd_supply":     cs.TiUSDSupply,
		"reward_pool":      cs.RewardPool,
	})
}
