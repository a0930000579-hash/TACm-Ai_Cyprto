package node

import (
	"encoding/json"
	"fmt"
	"net/http"

	"tacm/internal/wallet"
)

// walletView 為錢包資訊的 API 視圖（格式化字串 + 最小單位，供前端直用）。
type walletView struct {
	Address     string `json:"address"`
	TACm        string `json:"tacm"`
	TACmRaw     string `json:"tacm_raw"`
	TiUSD       string `json:"tiusd"`
	TiUSDRaw    int64  `json:"tiusd_raw"`
	USDT        string `json:"usdt"`
	USDTRaw     int64  `json:"usdt_raw"`
	SyncedBlock int64  `json:"synced_block"`
	FeeTiUSDBps int    `json:"fee_tiusd_bps"`
	FeeUSDTBps  int    `json:"fee_usdt_bps"`
}

// handleWalletInfo GET /api/wallet/info?address=xxx
func (s *RPCServer) handleWalletInfo(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "wallet_disabled"})
		return
	}
	addr := r.URL.Query().Get("address")
	if addr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "address_required"})
		return
	}
	acc, err := s.node.walletSvc.Balance(addr)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	synced, err := s.node.walletSvc.SyncedHeight()
	if err != nil {
		synced = -1
	}
	writeJSON(w, http.StatusOK, walletView{
		Address:     acc.Address,
		TACm:        wallet.FormatAmountBig(acc.TACmBalance),
		TACmRaw:     acc.TACmBalance.String(),
		TiUSD:       wallet.FormatAmountI64(acc.TiUSDBalance, wallet.AssetTiUSD),
		TiUSDRaw:    acc.TiUSDBalance,
		USDT:        wallet.FormatAmountI64(acc.USDTBalance, wallet.AssetUSDT),
		USDTRaw:     acc.USDTBalance,
		SyncedBlock: synced,
		FeeTiUSDBps: wallet.AssetTiUSD.FeeBps(),
		FeeUSDTBps:  wallet.AssetUSDT.FeeBps(),
	})
}

// handleWalletLedger GET /api/wallet/ledger?limit=50
func (s *RPCServer) handleWalletLedger(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "wallet_disabled"})
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := parseInt(v); err == nil {
			limit = n
		}
	}
	led, err := s.node.walletSvc.Ledger(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "items": led})
}

// walletTxReq 為轉帳/充值/發行請求體。
type walletTxReq struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Memo   string `json:"memo"`
	Note   string `json:"note"`
}

// handleWalletTransfer POST /api/wallet/transfer {from,to,asset,amount,memo}
// 站內多資產轉帳；手續費：TiUSD 0.5%、USDT 1.25%、TACm 2%。
func (s *RPCServer) handleWalletTransfer(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "wallet_disabled"})
		return
	}
	var req walletTxReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	asset := wallet.Asset(req.Asset)
	amount, err := wallet.NewAmount(req.Amount, asset)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	fee, err := s.node.walletSvc.Transfer(req.From, req.To, asset, amount, req.Memo)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "from": req.From, "to": req.To,
		"asset": string(asset), "amount": req.Amount,
		"fee": fee.String(asset), "fee_raw": fee.String(asset),
	})
}

// handleWalletDeposit POST /api/wallet/deposit {to,asset,amount,memo}
// 充值入帳（外部資金來源：鏈上充值/兌入；M13 供運維與測試）。
func (s *RPCServer) handleWalletDeposit(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "wallet_disabled"})
		return
	}
	var req walletTxReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	asset := wallet.Asset(req.Asset)
	amount, err := wallet.NewAmount(req.Amount, asset)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if err := s.node.walletSvc.Deposit(req.To, asset, amount, req.Memo); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "to": req.To, "asset": string(asset), "amount": req.Amount,
	})
}

// handleTiUSDSummary GET /api/tiusd/summary
func (s *RPCServer) handleTiUSDSummary(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "wallet_disabled"})
		return
	}
	sup, err := s.node.walletSvc.TiUSDSummary()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"supply":       wallet.FormatAmountI64(sup.Supply, wallet.AssetTiUSD),
		"supply_raw":   sup.Supply,
		"total_minted": wallet.FormatAmountI64(sup.TotalMinted, wallet.AssetTiUSD),
		"total_burned": wallet.FormatAmountI64(sup.TotalBurned, wallet.AssetTiUSD),
		"peg_price":    "1.0", // TiUSD 錨定 1 USDT
	})
}

// handleTiUSDMint POST /api/tiusd/mint {to,amount,note}
// 發行 TiUSD：供給增加並入帳指定地址。
func (s *RPCServer) handleTiUSDMint(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "wallet_disabled"})
		return
	}
	var req walletTxReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	amount, err := wallet.NewAmount(req.Amount, wallet.AssetTiUSD)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	sup, err := s.node.walletSvc.MintTiUSD(req.To, amount.I64, req.Note)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "to": req.To, "amount": req.Amount,
		"supply": wallet.FormatAmountI64(sup, wallet.AssetTiUSD),
	})
}

// handleTiUSDBurn POST /api/tiusd/burn {amount,note}
func (s *RPCServer) handleTiUSDBurn(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "wallet_disabled"})
		return
	}
	var req walletTxReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "bad_json"})
		return
	}
	amount, err := wallet.NewAmount(req.Amount, wallet.AssetTiUSD)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	sup, err := s.node.walletSvc.BurnTiUSD(amount.I64, req.Note)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "amount": req.Amount,
		"supply": wallet.FormatAmountI64(sup, wallet.AssetTiUSD),
	})
}

// parseInt 解析整數查詢參數。
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscan(s, &n)
	return n, err
}
