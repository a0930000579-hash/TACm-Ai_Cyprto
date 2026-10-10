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
		TxHash:   CoinbaseTxHash(height, proposerAddr, reward, ts),
		FromAddr: "", ToAddr: proposerAddr,
		Amount: FormatFloat(reward), Fee: "0", Nonce: 0, Ts: ts,
		Memo: "coinbase", Status: "confirmed",
	}
}

// RewardPoolAddr 鏈上獎勵池帳戶（與 wallet.RewardPoolAddr 同值，避免包循環引用）。
const RewardPoolAddr = "reward_pool"

// PoolShareBps 每區塊 coinbase 挹注獎勵池的比例（12% = 1200 bp，與 wallet 一致）。
const PoolShareBps = 1200

// CoinbaseSplit 出塊獎勵瓜分份額（Share 為算力整數，參與按算力比例分配）。
type CoinbaseSplit struct {
	Address string
	Share   int64
}

// CoinbaseSplitTxHash 計算瓜分 coinbase 交易的確定性哈希（含序號與收款人，避免多筆衝突）。
func CoinbaseSplitTxHash(height int64, idx int, to string, amount float64, ts int64) string {
	canonical := fmt.Sprintf("tacm:coinsplit:%d:%d:%s:%s:%d",
		height, idx, to, FormatFloat(amount), ts)
	return hex.EncodeToString(crypto.DoubleSHA256([]byte(canonical)))
}

// BuildCoinbaseTxs 構造出塊獎勵的鏈上瓜分交易清單（M58：瓜分明細寫入區塊，全網一致）。
// reward 的 PoolShareBps（12%）進 reward_pool；其餘按 splits 算力比例分給在線礦工；
// 無在線礦工時其餘全數進 reward_pool（維持 M36 語義，避免「未開機卻進帳」）。
// 回傳一或多筆 from 為空的 coinbase 交易；所有節點照單執行 → 收益/獎勵池全網一致。
func BuildCoinbaseTxs(height int64, proposerAddr string, ts int64, splits []CoinbaseSplit, reward float64) []Transaction {
	if reward <= 1e-9 {
		return nil
	}
	share := reward * float64(PoolShareBps) / 10000
	rest := reward - share
	out := []Transaction{}
	add := func(to string, amt float64, tag string) {
		if amt <= 1e-9 {
			return
		}
		out = append(out, Transaction{
			TxHash:   CoinbaseSplitTxHash(height, len(out), to, amt, ts),
			FromAddr: "", ToAddr: to,
			Amount: FormatFloat(amt), Fee: "0", Nonce: 0, Ts: ts,
			Memo: "coinbase:" + tag, Status: "confirmed",
		})
	}
	var totalShare int64
	for _, sp := range splits {
		if sp.Share > 0 {
			totalShare += sp.Share
		}
	}
	if totalShare <= 0 {
		// 無在線礦工：88% 也進獎勵池（M36 語義）。
		add(RewardPoolAddr, rest, "reserve")
		add(RewardPoolAddr, share, "pool")
		return out
	}
	var acc float64
	n := 0
	for _, sp := range splits {
		if sp.Share <= 0 {
			continue
		}
		n++
	}
	idx := 0
	for _, sp := range splits {
		if sp.Share <= 0 {
			continue
		}
		idx++
		var portion float64
		if idx == n {
			portion = rest - acc
		} else {
			portion = rest * float64(sp.Share) / float64(totalShare)
			acc += portion
		}
		add(sp.Address, portion, "miner")
	}
	add(RewardPoolAddr, share, "pool")
	return out
}
