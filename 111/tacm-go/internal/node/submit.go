package node

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"tacm/internal/chaindb"
	"tacm/internal/crypto"
)

// SubmitTransaction 驗證並提交交易到內存池。檢查必要字段、地址、真實 ECDSA
// 簽名、nonce 順序與餘額；成功返回交易哈希。
func (n *Node) SubmitTransaction(tx map[string]any) (string, error) {
	required := []string{"from", "to", "amount", "fee", "nonce", "ts", "signature", "pubkey"}
	for _, f := range required {
		v, ok := tx[f]
		if !ok || v == nil {
			return "", fmt.Errorf("missing field: %s", f)
		}
	}

	from := getString(tx, "from")
	to := getString(tx, "to")
	signature := getString(tx, "signature")

	if !crypto.IsValidAddress(from) {
		return "", errors.New("invalid from address")
	}
	if !crypto.IsValidAddress(to) {
		return "", errors.New("invalid to address")
	}
	if !crypto.VerifyTransactionSignature(tx, signature, from) {
		return "", errors.New("invalid signature")
	}

	// 交易哈希：DoubleSHA256(Canonical({p:核心字段, sig:簽名}))。
	p := map[string]any{
		"from": from, "to": to,
		"amount": getString(tx, "amount"), "fee": getString(tx, "fee"),
		"nonce": getInt64(tx, "nonce"), "ts": getInt64(tx, "ts"),
	}
	wrap := map[string]any{"p": p, "sig": signature}
	raw, err := crypto.Canonical(wrap)
	if err != nil {
		return "", fmt.Errorf("計算交易哈希失敗: %w", err)
	}
	txHash := hex.EncodeToString(crypto.DoubleSHA256(raw))
	tx["tx_hash"] = txHash

	// nonce 順序校驗（防重放/亂序）。
	expectedNonce := n.db.GetNonce(from)
	if getInt64(tx, "nonce") != expectedNonce {
		return "", fmt.Errorf("bad nonce: expected %d, got %d",
			expectedNonce, getInt64(tx, "nonce"))
	}

	// 餘額校驗。
	amount, err := strconv.ParseFloat(getString(tx, "amount"), 64)
	if err != nil {
		return "", errors.New("invalid amount")
	}
	fee, err := strconv.ParseFloat(getString(tx, "fee"), 64)
	if err != nil {
		return "", errors.New("invalid fee")
	}
	balance, _ := strconv.ParseFloat(n.db.GetBalance(from), 64)
	if balance < amount+fee {
		return "", fmt.Errorf("insufficient balance: have %g, need %g",
			balance, amount+fee)
	}

	// 合約交易：入池前只讀模擬構造/調用，失敗則拒絕（避免壞合約入塊）。
	memo := getString(tx, "memo")
	if isContractMemo(memo) {
		simTx := &chaindb.Transaction{
			FromAddr: from, ToAddr: to,
			Amount: getString(tx, "amount"), Fee: getString(tx, "fee"),
			Memo: memo,
		}
		if err := n.simulateContractTx(simTx); err != nil {
			return "", err
		}
	}

	if err := n.db.AddMempoolTx(txHash, tx, signature, getString(tx, "fee")); err != nil {
		return "", err
	}

	if n.p2pNet != nil {
		n.p2pNet.BroadcastTx(tx)
	}
	return txHash, nil
}
