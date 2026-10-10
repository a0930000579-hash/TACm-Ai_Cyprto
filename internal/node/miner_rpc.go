package node

// miner_rpc.go — 礦機 RPC（對齊原本方式：註冊/心跳/在線礦工/收益）。

import (
	"encoding/json"
	"math/big"
	"net/http"
	"strconv"

	"tacm/internal/wallet"
)

// MinerRegisterReq 礦機註冊請求（VCPU/VGPU 算力）。
type MinerRegisterReq struct {
	Address string `json:"address"`
	VCPU    int    `json:"vcpu"`
	VGPU    int    `json:"vgpu"`
}

// MinerTickReq 礦機心跳請求。
type MinerTickReq struct {
	Address string `json:"address"`
}

// handleMinerRegister POST /api/miner/register — 註冊/更新一台礦機。
func (s *RPCServer) handleMinerRegister(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "錢包未初始化")
		return
	}
	var req MinerRegisterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "參數解析失敗: "+err.Error())
		return
	}
	if req.VCPU == 0 && req.VGPU == 0 {
		req.VCPU = 1 // 預設 1 vCPU
	}
	if err := s.node.walletSvc.Store().RegisterMiner(req.Address, req.VCPU, req.VGPU); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "address": req.Address, "vcpu": req.VCPU, "vgpu": req.VGPU})
}

// handleMinerStart POST /api/miner/start — 開機挖礦（M33：開啟伺服器端排程＋立即心跳）。
// 開啟後由節點 minerHeartbeatLoop 每 10s 持續心跳——離開頁面仍繼續挖礦。
func (s *RPCServer) handleMinerStart(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "錢包未初始化")
		return
	}
	var req MinerTickReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "參數解析失敗: "+err.Error())
		return
	}
	// 開機＝註冊（固定算力）＋啟用排程＋立即心跳（訊號即時轉綠）。
	if err := s.node.walletSvc.Store().RegisterMiner(req.Address, 0, 0); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.node.walletSvc.Store().SetActiveMiner(req.Address, true); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "address": req.Address, "active": true})
}

// handleMinerTick POST /api/miner/tick — 礦機心跳。
func (s *RPCServer) handleMinerTick(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "錢包未初始化")
		return
	}
	var req MinerTickReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "參數解析失敗: "+err.Error())
		return
	}
	if err := s.node.walletSvc.Store().TickMiner(req.Address); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "address": req.Address})
}

// handleMinerStop POST /api/miner/stop — 礦機離線。
func (s *RPCServer) handleMinerStop(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "錢包未初始化")
		return
	}
	var req MinerTickReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "參數解析失敗: "+err.Error())
		return
	}
	if err := s.node.walletSvc.Store().StopMiner(req.Address); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "address": req.Address, "status": "offline"})
}

// handleMiners GET /api/miners — 全部礦機＋在線統計。
func (s *RPCServer) handleMiners(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "錢包未初始化")
		return
	}
	ms, err := s.node.walletSvc.Store().Miners()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// M58：splits 改用「鏈上瓜分視角」——任何節點只要同步了同一條鏈，看到的在線礦工與份額
	// 就完全一致；share 為瓜分額換算的等效算力（佔比＝瓜分額佔比＝算力佔比）。
	// 不再用各節點本地 SQL（避免「錨點 3 台、follower 1 台」的入口不一致）。
	splits := s.onChainMinerSplits(60)
	// M32：附加每台礦機的推薦註冊人數（算力加成來源）。
	out := make([]map[string]any, 0, len(ms))
	for _, m := range ms {
		refs, _ := s.node.walletSvc.Store().ReferralCount(m.Address)
		out = append(out, map[string]any{
			"address": m.Address, "vcpu": m.VCPU, "vgpu": m.VGPU,
			"hashrate": m.Hashrate, "status": m.Status,
			"last_heartbeat_ts": m.LastHeartbeatTs, "registered_ts": m.RegisteredTs,
			"online": m.Online, "refs": refs, "active": m.Active,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "miners": out, "online_count": len(splits), "splits": splits,
	})
}

// onChainMinerSplits 從鏈上反推「全網在線礦工瓜分視角」（最近 window 塊的 coinbase 瓜分）：
// 收款人（非 reward_pool、非空）即鏈上實際瓜分到獎勵的礦工；share 為瓜分額換算的等效算力
// （每台標稱 5M＝1vCPU×1M + 2vGPU×2M），佔比即算力佔比。任何節點同步同一條鏈結果一致。
func (s *RPCServer) onChainMinerSplits(window int64) []map[string]any {
	tip := s.node.DB().GetTipHeight()
	start := tip - window + 1
	if start < 1 {
		start = 1
	}
	shares := map[string]float64{}
	var total float64
	for h := start; h <= tip; h++ {
		txs, err := s.node.DB().GetTransactionsByBlock(h)
		if err != nil {
			continue
		}
		for _, tx := range txs {
			if tx.FromAddr == "" && tx.ToAddr != "" && tx.ToAddr != wallet.RewardPoolAddr {
				if v, err := strconv.ParseFloat(tx.Amount, 64); err == nil {
					shares[tx.ToAddr] += v
					total += v
				}
			}
		}
	}
	n := len(shares)
	out := make([]map[string]any, 0, n)
	if n > 0 && total > 0 {
		nominal := float64(n) * 5e6 // 每台標稱 5M（1vCPU×1M + 2vGPU×2M）
		for a, v := range shares {
			out = append(out, map[string]any{"address": a, "share": v / total * nominal})
		}
	}
	return out
}

// handleMinerEarnings GET /api/miner/earnings?address= — 礦工累計收益（KindReward 分錄）。
func (s *RPCServer) handleMinerEarnings(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "錢包未初始化")
		return
	}
	addr := r.URL.Query().Get("address")
	if addr == "" {
		writeErr(w, http.StatusBadRequest, "缺少 address")
		return
	}
	led, err := s.node.walletSvc.Ledger(5000)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	earned := big.NewInt(0)
	count := 0
	for _, e := range led {
		if e.Account == addr && e.Kind == wallet.KindReward && e.Asset == wallet.AssetTACm {
			count++
			// e.Delta 為十進制 wei 字串（正數）。
			if v, ok := new(big.Int).SetString(e.Delta, 10); ok && v.Sign() > 0 {
				earned.Add(earned, v)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "address": addr, "earned_raw": earned.String(), "reward_count": count})
}

// handleChainStats GET /api/chain/stats — 全鏈統計（顯示於礦機頁）：
//
//	總產出 TACm（累計出塊獎勵已分配）、TiUSD 總流通、獎勵池、總供應上限。
func (s *RPCServer) handleChainStats(w http.ResponseWriter, r *http.Request) {
	if s.node.walletSvc == nil {
		writeErr(w, http.StatusServiceUnavailable, "錢包未初始化")
		return
	}
	total, err := s.node.walletSvc.Store().ChainMinedTacm()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "讀取全鏈產出失敗")
		return
	}
	sup, _ := s.node.walletSvc.TiUSDSummary()
	pool := big.NewInt(0)
	if pa, err := s.node.walletSvc.Balance(wallet.RewardPoolAddr); err == nil {
		pool = pa.TACmBalance
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"total_mined_tacm": wallet.FormatAmountBig(total),
		"tiusd_supply":     sup.Supply,
		"reward_pool":      wallet.FormatAmountBig(pool),
	})
}
