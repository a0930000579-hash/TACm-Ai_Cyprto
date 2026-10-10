package node

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"tacm/internal/crypto"
	"tacm/internal/vm"
)

// 本檔提供「鏈上 NFT（ERC-721 風格）」RPC 端點（M74-2）：
//   - POST /contract/nft/deploy   —— 節點官方金鑰代簽部署標準 NFT 集合
//   - GET  /contract/nft/{address}—— 便利查詢 name/symbol/totalSupply＋holder balance＋ownerOf
//   - POST /contract/nft/mint     —— 節點代簽 mint(to, tokenId)
//   - POST /contract/nft/transfer —— 節點代簽 transferFrom(from, to, tokenId)
//
// 與既有 ERC-20 端點（/contract/deploy、/contract/erc20/...）並存不衝突；
// 一般用戶若持有自己的私鑰，仍可直接 POST /tx/submit 走真實 ECDSA 管道。

// handleContractNFTDeploy POST /contract/nft/deploy
// body: {name, symbol}（name/symbol ≤32 bytes 可空）
func (s *RPCServer) handleContractNFTDeploy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name     string `json:"name"`
		Symbol   string `json:"symbol"`
		GasLimit uint64 `json:"gas_limit,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad JSON body")
		return
	}
	if len(req.Name) > 32 || len(req.Symbol) > 32 {
		writeErr(w, http.StatusBadRequest, "name/symbol must be ≤32 bytes")
		return
	}
	if req.GasLimit > vm.NewContext().Gas {
		writeErr(w, http.StatusBadRequest, "gas_limit exceeds node maximum (10000000)")
		return
	}
	kp := s.node.keypair
	if kp == nil {
		writeErr(w, http.StatusServiceUnavailable, "node key unavailable")
		return
	}
	from, err := kp.Address()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	nonce := s.node.db.GetNonce(from) + s.node.pendingTxCount(from)
	evmFrom, err := tx0To0x(from)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	addr0x := vm.CreateAddress(evmFrom, uint64(nonce))
	to, err := evmToTx0(addr0x)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	init := vm.Erc721InitWithMeta([]byte(req.Name), []byte(req.Symbol))
	memo := vmDeployPrefix + hex.EncodeToString(init)
	if req.GasLimit > 0 {
		memo = fmt.Sprintf("%s%d:%s", vmDeployPrefix, req.GasLimit, hex.EncodeToString(init))
	}
	tx := map[string]any{
		"from": from, "to": to,
		"amount": "0", "fee": "0",
		"nonce": nonce, "ts": time.Now().Unix(),
		"pubkey": hex.EncodeToString(kp.PublicKeyCompressed()),
		"memo":   memo,
	}
	sig, err := crypto.SignTransaction(tx, kp.PrivateKey())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	tx["signature"] = sig
	txHash, err := s.node.SubmitTransaction(tx)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tx_hash": txHash, "status": "mempool",
		"contract_address": to, "evm_address": addr0x,
		"creator": from, "name": req.Name, "symbol": req.Symbol,
	})
}

// nftSlotBalance 計算 ERC-721 balanceOf(owner) 的 storage slot（keccak(owner ‖ 4)）。
func nftSlotBalance(owner0x string) (*big.Int, error) {
	k, err := holderKey(owner0x)
	if err != nil {
		return nil, err
	}
	return vm.Keccak256Hash(vm.IntToBytes(k, 32), vm.IntToBytes(big.NewInt(4), 32)), nil
}

// nftSlotOwner 計算 ERC-721 ownerOf(tokenId) 的 storage slot（keccak(tokenId ‖ 3)）。
func nftSlotOwner(tokenID *big.Int) *big.Int {
	return vm.Keccak256Hash(vm.IntToBytes(tokenID, 32), vm.IntToBytes(big.NewInt(3), 32))
}

// nftSlotURI 計算 ERC-721 tokenURI(tokenId) 的 storage slot（keccak(tokenId ‖ 5)）。
func nftSlotURI(tokenID *big.Int) *big.Int {
	return vm.Keccak256Hash(vm.IntToBytes(tokenID, 32), vm.IntToBytes(big.NewInt(5), 32))
}

// handleContractNFTInfo GET /contract/nft/{address}?holder=tx0...&token_id=1
// 便利查詢：name/symbol/totalSupply＋（可選）holder 的 NFT 持有數與 tokenId 的 owner。
func (s *RPCServer) handleContractNFTInfo(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("address")
	evm, err := tx0To0x(addr)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.node.contracts.Get(evm) == nil {
		writeErr(w, http.StatusNotFound, "contract not found")
		return
	}
	out := map[string]any{
		"ok": true, "address": addr, "evm_address": evm,
		"name":    metaString(s.node.contracts.StorageAt(evm, big.NewInt(1))),
		"symbol":  metaString(s.node.contracts.StorageAt(evm, big.NewInt(2))),
		"supply":  s.node.contracts.StorageAt(evm, big.NewInt(0)).String(),
		"balance": "0", "owner": "",
	}
	if holder := r.URL.Query().Get("holder"); holder != "" {
		bs, err := nftSlotBalance(holder)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid holder address")
			return
		}
		out["balance"] = s.node.contracts.StorageAt(evm, bs).String()
	}
	if tid := r.URL.Query().Get("token_id"); tid != "" {
		tidInt, ok := new(big.Int).SetString(tid, 10)
		if !ok || tidInt.Sign() < 0 {
			writeErr(w, http.StatusBadRequest, "invalid token_id")
			return
		}
		owner := s.node.contracts.StorageAt(evm, nftSlotOwner(tidInt))
		out["owner"] = owner.String()
		out["token_uri"] = s.node.contracts.StorageAt(evm, nftSlotURI(tidInt)).String()
	}
	writeJSON(w, http.StatusOK, out)
}

// handleContractNFTMint POST /contract/nft/mint
// body: {contract, to, token_id}——伺服器組 mint calldata 並以節點金鑰代簽上鏈。
func (s *RPCServer) handleContractNFTMint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Contract string `json:"contract"`
		To       string `json:"to"`
		TokenID  string `json:"token_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad JSON body")
		return
	}
	if !crypto.IsValidAddress(req.Contract) || !crypto.IsValidAddress(req.To) {
		writeErr(w, http.StatusBadRequest, "invalid contract/to address")
		return
	}
	tid, ok := new(big.Int).SetString(req.TokenID, 10)
	if !ok || tid.Sign() < 0 {
		writeErr(w, http.StatusBadRequest, "invalid token_id")
		return
	}
	to0x, err := tx0To0x(req.To)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	calldata := vm.Erc721MintCalldata(to0x, tid)
	txHash, err := s.node.SubmitSignedContractCall(req.Contract, hex.EncodeToString(calldata))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	from, _ := s.node.keypair.Address()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tx_hash": txHash, "status": "mempool",
		"from": from, "to": req.Contract,
		"recipient": req.To, "token_id": tid.String(),
	})
}

// handleContractNFTTransfer POST /contract/nft/transfer
// body: {contract, from, to, token_id}——伺服器組 transferFrom calldata 並代簽上鏈。
func (s *RPCServer) handleContractNFTTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Contract string `json:"contract"`
		From     string `json:"from"`
		To       string `json:"to"`
		TokenID  string `json:"token_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad JSON body")
		return
	}
	if !crypto.IsValidAddress(req.Contract) || !crypto.IsValidAddress(req.From) || !crypto.IsValidAddress(req.To) {
		writeErr(w, http.StatusBadRequest, "invalid contract/from/to address")
		return
	}
	tid, ok := new(big.Int).SetString(req.TokenID, 10)
	if !ok || tid.Sign() < 0 {
		writeErr(w, http.StatusBadRequest, "invalid token_id")
		return
	}
	from0x, err := tx0To0x(req.From)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	to0x, err := tx0To0x(req.To)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	calldata := vm.Erc721TransferFromCalldata(from0x, to0x, tid)
	txHash, err := s.node.SubmitSignedContractCall(req.Contract, hex.EncodeToString(calldata))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	from, _ := s.node.keypair.Address()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "tx_hash": txHash, "status": "mempool",
		"from": from, "to": req.Contract,
		"token_from": req.From, "recipient": req.To, "token_id": tid.String(),
	})
}
