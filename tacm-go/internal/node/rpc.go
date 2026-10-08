package node

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"tacm/internal/crypto"
)

// RPCServer 暴露節點的 HTTP RPC 接口。
type RPCServer struct {
	node *Node
	mux  *http.ServeMux
}

// NewRPCServer 建立路由並註冊全部端點。
func NewRPCServer(n *Node) *RPCServer {
	mux := http.NewServeMux()
	s := &RPCServer{node: n, mux: mux}

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /headers", s.handleHeaders)
	mux.HandleFunc("GET /header/{height}", s.handleHeader)
	mux.HandleFunc("GET /proof/{txhash}", s.handleProof)
	mux.HandleFunc("GET /block/{height}", s.handleBlock)
	mux.HandleFunc("GET /blocks", s.handleBlocks)
	mux.HandleFunc("GET /tx/{txhash}", s.handleTx)
	mux.HandleFunc("GET /account/{address}", s.handleAccount)
	mux.HandleFunc("GET /mempool", s.handleMempool)
	mux.HandleFunc("GET /wallet/new", s.handleWalletNew)
	mux.HandleFunc("GET /finality", s.handleFinality)
	mux.HandleFunc("GET /validators", s.handleValidators)
	mux.HandleFunc("GET /finality-proof/{height}", s.handleFinalityProof)
	mux.HandleFunc("POST /api/miner/register", s.handleMinerRegister)
	mux.HandleFunc("POST /api/miner/tick", s.handleMinerTick)
	mux.HandleFunc("POST /api/miner/stop", s.handleMinerStop)
	mux.HandleFunc("GET /api/miners", s.handleMiners)
	mux.HandleFunc("GET /api/miner/earnings", s.handleMinerEarnings)
	mux.HandleFunc("GET /api/exchange/pool", s.handleExchangePool)
	// M21-C 社群（FB 風動態牆/市集/廣告）。
	mux.HandleFunc("GET /api/community/feed", s.handleCommunityFeed)
	mux.HandleFunc("POST /api/community/post", s.handleCommunityPost)
	mux.HandleFunc("POST /api/community/post/{id}/like", s.handleCommunityLike)
	mux.HandleFunc("GET /api/community/post/{id}/comments", s.handleCommunityComments)
	mux.HandleFunc("POST /api/community/post/{id}/comments", s.handleCommunityComment)
	mux.HandleFunc("GET /api/community/market", s.handleCommunityMarket)
	mux.HandleFunc("POST /api/community/market/create", s.handleCommunityMarketCreate)
	mux.HandleFunc("GET /api/community/market/{id}/buy-intent", s.handleCommunityBuyIntent)
	mux.HandleFunc("POST /api/community/market/{id}/buy", s.handleCommunityBuy)
	mux.HandleFunc("GET /api/community/ads", s.handleCommunityAds)
	mux.HandleFunc("POST /api/community/ads/create", s.handleCommunityAdsCreate)
	mux.HandleFunc("POST /api/community/ads/{id}/pay", s.handleCommunityAdPay)
	mux.HandleFunc("POST /api/community/transfer/exchange", s.handleCommunityTransferExchange)
	mux.HandleFunc("GET /api/community/stats", s.handleCommunityStats)
	mux.HandleFunc("GET /api/community/config", s.handleCommunityConfig)

	// M13 錢包/資產/TiUSD 業務端點。
	mux.HandleFunc("GET /api/wallet/info", s.handleWalletInfo)
	mux.HandleFunc("GET /api/wallet/ledger", s.handleWalletLedger)
	mux.HandleFunc("POST /api/wallet/transfer", s.handleWalletTransfer)
	mux.HandleFunc("POST /api/wallet/deposit", s.handleWalletDeposit)
	mux.HandleFunc("GET /api/tiusd/summary", s.handleTiUSDSummary)
	mux.HandleFunc("POST /api/tiusd/mint", s.handleTiUSDMint)
	mux.HandleFunc("POST /api/tiusd/burn", s.handleTiUSDBurn)
	mux.HandleFunc("GET /api/exchange/balances", s.handleExchangeBalances)
	mux.HandleFunc("GET /api/exchange/orders", s.handleExchangeOrders)
	mux.HandleFunc("POST /api/exchange/deposit", s.handleExchangeDeposit)
	mux.HandleFunc("GET /api/exchange/book", s.handleExchangeBook)
	mux.HandleFunc("POST /api/exchange/order", s.handleExchangeOrder)
	mux.HandleFunc("POST /api/exchange/cancel", s.handleExchangeCancel)
	mux.HandleFunc("POST /api/exchange/flash", s.handleExchangeFlash)
	mux.HandleFunc("POST /api/exchange/bot/start", s.handleExchangeBotStart)
	mux.HandleFunc("POST /api/exchange/bot/stop", s.handleExchangeBotStop)
	mux.HandleFunc("GET /api/exchange/bots", s.handleExchangeBots)
	mux.HandleFunc("POST /api/exchange/deposit_from_wallet", s.handleExchangeDepositFromWallet)
	mux.HandleFunc("POST /api/exchange/withdraw_to_wallet", s.handleExchangeWithdrawToWallet)
	mux.HandleFunc("GET /api/exchange/fee", s.handleExchangeFeeAccount)
	mux.HandleFunc("GET /api/defi/pools", s.handleDefiPools)
	mux.HandleFunc("GET /api/defi/my-liquidity", s.handleDefiMyLiquidity)
	mux.HandleFunc("POST /api/defi/add-liquidity", s.handleDefiAddLiquidity)
	mux.HandleFunc("POST /api/defi/remove-liquidity", s.handleDefiRemoveLiquidity)
	mux.HandleFunc("POST /api/defi/claim-reward/{id}", s.handleDefiClaimReward)
	mux.HandleFunc("GET /api/defi/lending-markets", s.handleDefiLendingMarkets)
	mux.HandleFunc("GET /api/defi/my-deposits", s.handleDefiMyDeposits)
	mux.HandleFunc("GET /api/defi/my-loans", s.handleDefiMyLoans)
	mux.HandleFunc("POST /api/defi/deposit", s.handleDefiDeposit)
	mux.HandleFunc("POST /api/defi/withdraw", s.handleDefiWithdraw)
	mux.HandleFunc("POST /api/defi/borrow", s.handleDefiBorrow)
	mux.HandleFunc("POST /api/defi/repay", s.handleDefiRepay)
	mux.HandleFunc("GET /api/defi/ido-projects", s.handleDefiIDOProjects)
	mux.HandleFunc("GET /api/defi/my-ido", s.handleDefiMyIDO)
	mux.HandleFunc("POST /api/defi/ido-subscribe", s.handleDefiIDOSubscribe)
	mux.HandleFunc("POST /api/defi/ido-claim/{id}", s.handleDefiIDOClaim)
	mux.HandleFunc("GET /api/defi/vaults", s.handleDefiVaults)
	mux.HandleFunc("GET /api/defi/my-vaults", s.handleDefiMyVaults)
	mux.HandleFunc("POST /api/defi/vault-deposit", s.handleDefiVaultDeposit)
	mux.HandleFunc("POST /api/defi/vault-withdraw", s.handleDefiVaultWithdraw)
	mux.HandleFunc("POST /api/defi/vault-compound/{id}", s.handleDefiVaultCompound)
	mux.HandleFunc("GET /api/c2c/ads", s.handleC2CAds)
	mux.HandleFunc("POST /api/c2c/ad-create", s.handleC2CAdCreate)
	mux.HandleFunc("GET /api/c2c/my-ads", s.handleC2CMyAds)
	mux.HandleFunc("POST /api/c2c/ad-status", s.handleC2CAdStatus)
	mux.HandleFunc("POST /api/c2c/order-create", s.handleC2COrderCreate)
	mux.HandleFunc("POST /api/c2c/order-confirm", s.handleC2COrderConfirm)
	mux.HandleFunc("POST /api/c2c/order-release", s.handleC2COrderRelease)
	mux.HandleFunc("POST /api/c2c/order-cancel", s.handleC2COrderCancel)
	mux.HandleFunc("POST /api/c2c/order-dispute", s.handleC2COrderDispute)
	mux.HandleFunc("GET /api/c2c/my-orders", s.handleC2CMyOrders)
	mux.HandleFunc("GET /api/rewardpool", s.handleRewardPool)
	mux.HandleFunc("POST /api/rewardpool/topup", s.handleRewardPoolTopup)
	mux.HandleFunc("POST /api/rewardpool/withdraw", s.handleRewardPoolWithdraw)

	mux.HandleFunc("GET /contract/preview", s.handleContractPreview)
	mux.HandleFunc("GET /contract/list", s.handleContractList)
	mux.HandleFunc("GET /contract/get/{address}", s.handleContractGet)
	mux.HandleFunc("GET /contract/storage/{address}/{key}", s.handleContractStorage)

	mux.HandleFunc("GET /api/status", s.handleAggStatus)
	mux.HandleFunc("POST /tx/submit", s.handleTxSubmit)
	mux.HandleFunc("POST /tx/batch", s.handleTxBatch)
	mux.HandleFunc("POST /wallet/import", s.handleWalletImport)

	// Layer2 Optimistic Rollup
	mux.HandleFunc("GET /l2/status", s.handleL2Status)
	mux.HandleFunc("GET /l2/account/{address}", s.handleL2Account)
	mux.HandleFunc("GET /l2/block/{height}", s.handleL2Block)
	mux.HandleFunc("GET /l2/tx/{hash}", s.handleL2Tx)
	mux.HandleFunc("POST /l2/tx", s.handleL2TxSubmit)
	mux.HandleFunc("POST /l2/batch", s.handleL2Batch)
	mux.HandleFunc("POST /l2/submit", s.handleL2Submit)
	mux.HandleFunc("POST /l2/finalize", s.handleL2Finalize)
	mux.HandleFunc("POST /l2/challenge", s.handleL2Challenge)
	mux.HandleFunc("POST /l2/deposit", s.handleL2Deposit)
	mux.HandleFunc("POST /l2/withdraw", s.handleL2Withdraw)

	// 跨鏈橋
	mux.HandleFunc("GET /bridge/chains", s.handleBridgeChains)
	mux.HandleFunc("GET /bridge/stats", s.handleBridgeStats)
	mux.HandleFunc("POST /bridge/lock", s.handleBridgeLock)
	mux.HandleFunc("POST /bridge/burn", s.handleBridgeBurn)
	mux.HandleFunc("POST /bridge/confirm-lock", s.handleBridgeConfirmLock)
	mux.HandleFunc("POST /bridge/confirm-burn", s.handleBridgeConfirmBurn)
	mux.HandleFunc("POST /bridge/mint", s.handleBridgeMint)
	mux.HandleFunc("POST /bridge/unlock", s.handleBridgeUnlock)
	mux.HandleFunc("POST /bridge/status", s.handleBridgeStatus)

	return s
}

// Handler 返回可掛載的 HTTP 處理器（含 CORS）。
func (s *RPCServer) Handler() http.Handler {
	return corsMiddleware(s.mux)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *RPCServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "node_id": s.node.nodeID,
	})
}

func (s *RPCServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.node.GetStatus())
}

func (s *RPCServer) handleHeaders(w http.ResponseWriter, r *http.Request) {
	count := 200
	if v := r.URL.Query().Get("count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			count = n
		}
	}
	res, err := s.node.GetHeadersWindow(count)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *RPCServer) handleHeader(w http.ResponseWriter, r *http.Request) {
	h, err := strconv.ParseInt(r.PathValue("height"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid height")
		return
	}
	b, err := s.node.db.GetBlock(h)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, HeaderOnly(b))
}

func (s *RPCServer) handleProof(w http.ResponseWriter, r *http.Request) {
	p, err := s.node.GetTxProof(r.PathValue("txhash"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if p == nil {
		writeJSON(w, http.StatusOK, map[string]bool{"found": false})
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *RPCServer) handleBlock(w http.ResponseWriter, r *http.Request) {
	h, err := strconv.ParseInt(r.PathValue("height"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid height")
		return
	}
	b, err := s.node.GetBlockDetail(h)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if b == nil {
		writeErr(w, http.StatusNotFound, "block not found")
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *RPCServer) handleBlocks(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	blocks, err := s.node.db.GetBlocks(limit, 0)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

func (s *RPCServer) handleTx(w http.ResponseWriter, r *http.Request) {
	tx, err := s.node.db.GetTransaction(r.PathValue("txhash"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tx == nil {
		writeErr(w, http.StatusNotFound, "tx not found")
		return
	}
	writeJSON(w, http.StatusOK, tx)
}

func (s *RPCServer) handleAccount(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	acc, err := s.node.db.GetAccount(address)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if acc == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"address": address, "balance": "0", "nonce": 0,
		})
		return
	}
	writeJSON(w, http.StatusOK, acc)
}

func (s *RPCServer) handleMempool(w http.ResponseWriter, r *http.Request) {
	txs, err := s.node.db.GetMempool(100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"txs": txs, "count": s.node.db.MempoolSize(),
	})
}

func (s *RPCServer) handleWalletNew(w http.ResponseWriter, r *http.Request) {
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ex, err := kp.Export()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

func (s *RPCServer) handleTxSubmit(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	dec := json.NewDecoder(r.Body)
	dec.UseNumber() // 保留 ts/nonce 原始數字字面量，避免 float64 破壞簽名重算
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	txHash, err := s.node.SubmitTransaction(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tx_hash": txHash, "status": "mempool",
	})
}

func (s *RPCServer) handleTxBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Transactions []map[string]any `json:"transactions"`
		Txs          []map[string]any `json:"txs"`
	}
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	txs := body.Transactions
	if txs == nil {
		txs = body.Txs
	}
	results := make([]map[string]any, 0, len(txs))
	success := 0
	for _, tx := range txs {
		h, err := s.node.SubmitTransaction(tx)
		if err != nil {
			results = append(results, map[string]any{"ok": false, "error": err.Error()})
			continue
		}
		success++
		results = append(results, map[string]any{"ok": true, "tx_hash": h})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "total": len(txs), "success": success,
		"failed": len(txs) - success, "results": results,
	})
}

func (s *RPCServer) handleWalletImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PrivateKey string `json:"private_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	pk, err := hex.DecodeString(body.PrivateKey)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid private key")
		return
	}
	kp, err := crypto.KeyPairFromPrivateKey(pk)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid private key")
		return
	}
	ex, err := kp.Export()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

func (s *RPCServer) handleFinality(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.node.GetFinalityInfo())
}

func (s *RPCServer) handleValidators(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"validators": s.node.ValidatorSet().List(),
		"count":      s.node.ValidatorSet().Len(),
	})
}

func (s *RPCServer) handleFinalityProof(w http.ResponseWriter, r *http.Request) {
	h, err := strconv.ParseInt(r.PathValue("height"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid height")
		return
	}
	writeJSON(w, http.StatusOK, s.node.GetFinalityProof(h))
}
