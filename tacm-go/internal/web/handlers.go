package web

import (
	"net/http"
	"strconv"

	"github.com/flosch/pongo2/v6"

	"tacm/internal/wallet"
)

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

// handleDefi GET /defi — DeFi 中心（流動性挖礦＋借貸市場，App 化）。
// 實時數據由前端直接讀取節點 RPC（?rpc= 可覆蓋）。
func (s *Server) handleDefi(w http.ResponseWriter, r *http.Request) {
	s.render(w, "defi.html", pongo2.Context{})
}

// handleC2C GET /c2c — C2C 場外交易（法幣兌加密貨幣，App 化）。
func (s *Server) handleC2C(w http.ResponseWriter, r *http.Request) {
	s.render(w, "c2c.html", pongo2.Context{})
}

// handleWallet GET /wallet?address=tx0xxx — 幣安風多資產錢包頁面。
func (s *Server) handleWallet(w http.ResponseWriter, r *http.Request) {
	addr := r.URL.Query().Get("address")
	if addr == "" {
		addr = s.ds.Status().Address
	}
	wv, err := s.ds.Wallet(addr)
	if err != nil {
		http.Error(w, "錢包讀取失敗: "+err.Error(), http.StatusInternalServerError)
		return
	}
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
