package node

import (
	"net/http"
	"strconv"

	"tacm/internal/crypto"
	"tacm/internal/vm"
)

func errBody(err error) map[string]string { return map[string]string{"error": err.Error()} }

// handleContractPreview 預算部署地址：GET /contract/preview?creator=&nonce=
// nonce 缺省用創建者當前鏈上 nonce。
func (s *RPCServer) handleContractPreview(w http.ResponseWriter, r *http.Request) {
	creator := r.URL.Query().Get("creator")
	if !crypto.IsValidAddress(creator) {
		writeJSON(w, http.StatusBadRequest,
			map[string]string{"error": "invalid creator address"})
		return
	}
	evm, err := tx0To0x(creator)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	nonce := s.node.db.GetNonce(creator)
	if ns := r.URL.Query().Get("nonce"); ns != "" {
		v, err := strconv.ParseInt(ns, 10, 64)
		if err != nil || v < 0 {
			writeJSON(w, http.StatusBadRequest,
				map[string]string{"error": "invalid nonce"})
			return
		}
		nonce = v
	}
	addr0x := vm.CreateAddress(evm, uint64(nonce))
	tx0, err := evmToTx0(addr0x)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errBody(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"contract_address": tx0,
		"evm_address":      addr0x,
		"nonce":            nonce,
	})
}

// handleContractGet 查詢合約：GET /contract/get/{address}
func (s *RPCServer) handleContractGet(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("address")
	evm, err := tx0To0x(addr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	info := s.node.contracts.Get(evm)
	if info == nil {
		writeJSON(w, http.StatusNotFound,
			map[string]string{"error": "contract not found"})
		return
	}
	tx0, _ := evmToTx0(info.Address)
	writeJSON(w, http.StatusOK, map[string]any{
		"address":       tx0,
		"evm_address":   info.Address,
		"code_size":     info.CodeSize,
		"code_hash":     info.CodeHash,
		"storage_count": info.StorageCount,
		"balance":       info.Balance,
	})
}

// handleContractStorage 查槽值：GET /contract/storage/{address}/{key}
func (s *RPCServer) handleContractStorage(w http.ResponseWriter, r *http.Request) {
	addr := r.PathValue("address")
	keyStr := r.PathValue("key")
	evm, err := tx0To0x(addr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errBody(err))
		return
	}
	key := parseAmount(keyStr)
	val := s.node.contracts.StorageAt(evm, key)
	writeJSON(w, http.StatusOK, map[string]any{
		"address": addr, "key": keyStr, "value": val.String(),
	})
}

// handleContractList 列出全部合約：GET /contract/list
func (s *RPCServer) handleContractList(w http.ResponseWriter, r *http.Request) {
	infos := s.node.contracts.List()
	out := make([]map[string]any, 0, len(infos))
	for _, info := range infos {
		tx0, _ := evmToTx0(info.Address)
		out = append(out, map[string]any{
			"address":       tx0,
			"evm_address":   info.Address,
			"code_size":     info.CodeSize,
			"code_hash":     info.CodeHash,
			"storage_count": info.StorageCount,
			"balance":       info.Balance,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"contracts": out, "count": len(out),
	})
}
