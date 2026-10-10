package chaindb

import (
	"fmt"
	"math"
	"testing"
)

// TestBuildCoinbaseTxs：鏈上瓜分明細——12% 進獎勵池 + 88% 按算力瓜分（末位補齊）；
// 無在線礦工時全數入池；多筆哈希唯一。
func TestBuildCoinbaseTxs(t *testing.T) {
	// 有礦工：a1 算力 1、a2 算力 3 → 88% 份額 a1=2.2、a2=6.6；池 1.2。
	txs := BuildCoinbaseTxs(5, "proposer", 12345,
		[]CoinbaseSplit{{Address: "a1", Share: 1}, {Address: "a2", Share: 3}}, 10)
	if len(txs) != 3 {
		t.Fatalf("交易數=%d 應為 3 (a1+a2+pool)", len(txs))
	}
	got := map[string]float64{}
	for _, tx := range txs {
		if tx.FromAddr != "" {
			t.Fatalf("coinbase from 應為空: %s", tx.FromAddr)
		}
		var v float64
		if _, err := fmt.Sscanf(tx.Amount, "%g", &v); err != nil {
			t.Fatal(err)
		}
		got[tx.ToAddr] = v
	}
	if math.Abs(got["a1"]-2.2) > 1e-9 || math.Abs(got["a2"]-6.6) > 1e-9 || math.Abs(got[RewardPoolAddr]-1.2) > 1e-9 {
		t.Fatalf("瓜分錯誤: %+v", got)
	}

	// 無礦工：88%+12% 全數入池（M36 語義，可拆多筆但收款人必須全為 reward_pool）。
	txs2 := BuildCoinbaseTxs(6, "proposer", 12346, nil, 10)
	total := 0.0
	for _, tx := range txs2 {
		if tx.ToAddr != RewardPoolAddr {
			t.Fatalf("無礦工收款人應全為獎勵池: %+v", tx)
		}
		var v float64
		if _, err := fmt.Sscanf(tx.Amount, "%g", &v); err != nil {
			t.Fatal(err)
		}
		total += v
	}
	if math.Abs(total-10) > 1e-9 {
		t.Fatalf("無礦工入池總額=%g want 10", total)
	}

	// 多筆哈希唯一（避免 merkle 衝突）。
	seen := map[string]bool{}
	for _, tx := range txs {
		if seen[tx.TxHash] {
			t.Fatalf("coinbase 哈希重複: %s", tx.TxHash)
		}
		seen[tx.TxHash] = true
	}
}
