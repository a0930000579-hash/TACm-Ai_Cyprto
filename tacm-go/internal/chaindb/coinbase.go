package chaindb

import (
	"encoding/hex"
	"fmt"

	"tacm/internal/crypto"
)

// CoinbaseTxHash 計算 coinbase 交易的確定性哈希（coinbase 無簽名，
// 由高度、提議者、獎勵與時間唯一確定）。
func CoinbaseTxHash(height int64, proposer string, reward float64, ts int64) string {
	canonical := fmt.Sprintf("tacm:coinbase:%d:%s:%s:%d",
		height, proposer, FormatFloat(reward), ts)
	return hex.EncodeToString(crypto.DoubleSHA256([]byte(canonical)))
}

// BuildCoinbaseTx 構造區塊的 coinbase 增發交易（from 為空，僅入賬不扣款）。
// height=0（創世）或無提議者時返回獎勵為 0 的空交易。
func BuildCoinbaseTx(height int64, proposerAddr string, ts int64) Transaction {
	reward := BlockReward(height)
	return Transaction{
		TxHash: CoinbaseTxHash(height, proposerAddr, reward, ts),
		FromAddr: "", ToAddr: proposerAddr,
		Amount: FormatFloat(reward), Fee: "0", Nonce: 0, Ts: ts,
		Memo: "coinbase", Status: "confirmed",
	}
}
