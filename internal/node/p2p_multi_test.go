package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"tacm/internal/chaindb"
	"tacm/internal/config"
	"tacm/internal/crypto"
	"tacm/internal/p2p"
)

type fullNode struct {
	n   *Node
	net *p2p.Network
	url string
	srv *http.Server
}

func listenPort(t *testing.T) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return l, l.Addr().(*net.TCPAddr).Port
}

func startFullNode(t *testing.T, id string, bootstrap []string, follower bool, alice string) *fullNode {
	l, port := listenPort(t)
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.BlockTime = 1

	n, err := New(cfg, id, 1)
	if err != nil {
		t.Fatal(err)
	}
	if alice != "" {
		insertGenesisWithAlloc(t, n, alice)
	} else {
		insertGenesis(t, n)
	}

	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	netw, err := n.AttachP2P(url, bootstrap)
	if err != nil {
		t.Fatal(err)
	}

	root := http.NewServeMux()
	root.Handle("/p2p/", netw.Handler())
	root.Handle("/", NewRPCServer(n).Handler())
	srv := &http.Server{Handler: root}
	go func() { _ = srv.Serve(l) }()

	netw.Start(context.Background())
	if follower {
		n.StartAsFollower()
	} else {
		n.Start()
	}

	t.Cleanup(func() {
		netw.Close()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = n.Close()
	})
	return &fullNode{n: n, net: netw, url: url, srv: srv}
}

func waitHeight(t *testing.T, n *Node, h int64, to time.Duration) {
	t.Helper()
	deadline := time.Now().Add(to)
	for time.Now().Before(deadline) {
		if n.DB().GetTipHeight() >= h {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
	t.Fatalf("節點未達高度 %d，實際 %d", h, n.DB().GetTipHeight())
}

func waitBalance(t *testing.T, n *Node, addr string, want float64, to time.Duration) {
	t.Helper()
	deadline := time.Now().Add(to)
	for time.Now().Before(deadline) {
		v, _ := strconv.ParseFloat(n.DB().GetBalance(addr), 64)
		if abs(v-want) < 1e-9 {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
	t.Fatalf("地址 %s 餘額未達 %g，實際 %s", addr, want, n.DB().GetBalance(addr))
}

// 區塊 gossip：出塊節點 A 的塊即時傳到跟隨節點 B/C。
func TestP2PBlockGossip(t *testing.T) {
	A := startFullNode(t, "nodeA", nil, false, "")
	B := startFullNode(t, "nodeB", []string{A.url}, true, "")
	C := startFullNode(t, "nodeC", []string{A.url}, true, "")

	waitHeight(t, A.n, 4, 12*time.Second)
	waitHeight(t, B.n, 3, 12*time.Second)
	waitHeight(t, C.n, 3, 12*time.Second)
}

// 交易 gossip：提交到跟隨節點 B 的交易，轉發到出塊節點 A 打包，全網賬本一致。
func TestP2PTransactionGossip(t *testing.T) {
	aliceKP, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	alice := mustAddr(t, aliceKP)

	A := startFullNode(t, "nodeA", nil, false, alice)
	B := startFullNode(t, "nodeB", []string{A.url}, true, alice)
	C := startFullNode(t, "nodeC", []string{A.url}, true, alice)

	waitHeight(t, A.n, 3, 12*time.Second)
	waitHeight(t, B.n, 2, 12*time.Second)
	waitHeight(t, C.n, 2, 12*time.Second)

	bobKP, _ := crypto.GenerateKeyPair()
	bob := mustAddr(t, bobKP)
	tx := signedTx(t, aliceKP, bob, "50", "1", 0)

	body, _ := json.Marshal(tx)
	resp, err := http.Post(B.url+"/tx/submit", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		r, _ := io.ReadAll(resp.Body)
		t.Fatalf("提交交易失敗 %d: %s", resp.StatusCode, r)
	}
	resp.Body.Close()

	waitBalance(t, A.n, bob, 50, 15*time.Second)
	waitBalance(t, B.n, bob, 50, 15*time.Second)
	waitBalance(t, C.n, bob, 50, 15*time.Second)
}

// 晚啟動節點通過主動同步追平網絡。
func TestP2PLateSync(t *testing.T) {
	A := startFullNode(t, "nodeA", nil, false, "")
	waitHeight(t, A.n, 6, 12*time.Second)

	D := startFullNode(t, "nodeD", []string{A.url}, true, "")
	waitHeight(t, D.n, 6, 25*time.Second)

	// 同步後，同一高度（6）的區塊哈希必須一致（A 仍可能繼續出塊，
	// 故固定比較高度 6，而非各自當前鏈頂）。
	ab, _ := A.n.DB().GetBlock(6)
	db2, _ := D.n.DB().GetBlock(6)
	if ab == nil || db2 == nil {
		t.Fatal("缺少高度 6 區塊")
	}
	if ab.Hash != db2.Hash {
		t.Errorf("同步後高度 6 哈希不一致: %s vs %s", ab.Hash, db2.Hash)
	}
}

// TestP2PLedgerConsistency：錨點出塊（coinbase 鏈上瓜分給開機礦工）+ follower 晚同步後，
// 兩節點的鏈上帳本（區塊、礦工餘額、獎勵池、節點地址、錢包視角、全鏈總產出）必須完全一致。
// 對應「全網數據必須由鏈上同一份數據決定、任何入口看到都相同」的一致性要求。
func TestP2PLedgerConsistency(t *testing.T) {
	anchor := startFullNode(t, "anchor", nil, false, "")
	st := anchor.n.Wallet().Store()
	kp1, _ := crypto.GenerateKeyPair()
	a1, _ := kp1.Address()
	kp2, _ := crypto.GenerateKeyPair()
	a2, _ := kp2.Address()
	for _, a := range []string{a1, a2} {
		if err := st.RegisterMiner(a, 1, 1); err != nil {
			t.Fatal(err)
		}
		if err := st.SetActiveMiner(a, true); err != nil {
			t.Fatal(err)
		}
	}
	waitHeight(t, anchor.n, 6, 15*time.Second)

	f := startFullNode(t, "fol", []string{anchor.url}, true, "")
	// 等 follower 至少追到 6（與錨點同一條鏈的核心區塊）。
	waitHeight(t, f.n, 6, 25*time.Second)

	// 1) 已同步區塊哈希全一致（Merkle 綁定 coinbase 明細 → 交易集完全一致）。
	fh := f.n.DB().GetTipHeight()
	for h := int64(1); h <= fh; h++ {
		ab, _ := anchor.n.DB().GetBlock(h)
		fb, _ := f.n.DB().GetBlock(h)
		if ab == nil || fb == nil || ab.Hash != fb.Hash {
			t.Fatalf("h=%d 兩節點區塊哈希不一致: %s vs %s", h, ab.Hash, fb.Hash)
		}
	}

	// 2) 帳本＝鏈上照單累計：掃已同步區塊的 coinbase 交易累計。
	//    「共同高度段」（follower 已同步段）兩節點累計必須一致；
	//    且各節點「餘額＝各自已同步段的鏈上累計」——證明「照單入帳、不本地另算」。
	sumCoinbase := func(n *Node, limit int64) map[string]float64 {
		got := map[string]float64{}
		tip := n.DB().GetTipHeight()
		if limit > 0 && limit < tip {
			tip = limit
		}
		for h := int64(1); h <= tip; h++ {
			txs, err := n.DB().GetTransactionsByBlock(h)
			if err != nil {
				t.Fatal(err)
			}
			for _, tx := range txs {
				if tx.FromAddr == "" && tx.ToAddr != "" {
					if v, err := strconv.ParseFloat(tx.Amount, 64); err == nil {
						got[tx.ToAddr] += v
					}
				}
			}
		}
		return got
	}
	fh = f.n.DB().GetTipHeight()
	ag := sumCoinbase(anchor.n, fh) // anchor 前 fh 塊＝與 follower 相同的已同步段
	fg := sumCoinbase(f.n, 0)       // follower 全部（即 fh 塊）
	for _, addr := range []string{a1, a2, chaindb.RewardPoolAddr} {
		if math.Abs(ag[addr]-fg[addr]) > 1e-6 {
			t.Fatalf("共同高度段瓜分明細不一致 %s: anchor=%g follower=%g", addr, ag[addr], fg[addr])
		}
		bal, _ := parseBal(f.n.DB().GetBalance(addr))
		if math.Abs(fg[addr]-bal) > 1e-6 {
			t.Fatalf("follower 餘額≠鏈上累計 %s: 鏈=%g 餘額=%g", addr, fg[addr], bal)
		}
	}
	// anchor 全部段：餘額＝鏈上累計（含它多出的塊，同規則）。
	agAll := sumCoinbase(anchor.n, 0)
	for _, addr := range []string{a1, a2, chaindb.RewardPoolAddr} {
		balA, _ := parseBal(anchor.n.DB().GetBalance(addr))
		if math.Abs(agAll[addr]-balA) > 1e-6 {
			t.Fatalf("anchor 餘額≠鏈上累計 %s: 鏈=%g 餘額=%g", addr, agAll[addr], balA)
		}
	}

	// 3) 全鏈總產出＝高度×10（每塊獎勵總額 10，兩節點同規則、無超發）。
	ah := anchor.n.DB().GetTipHeight()
	am, _ := anchor.n.Wallet().Store().ChainMinedTacm()
	fm, _ := f.n.Wallet().Store().ChainMinedTacm()
	perBlock := new(big.Int).Mul(big.NewInt(10), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	if want := new(big.Int).Mul(perBlock, big.NewInt(ah)); am.Cmp(want) != 0 {
		t.Fatalf("anchor 總產出≠高度×10: %s vs %d×10", am, ah)
	}
	if want := new(big.Int).Mul(perBlock, big.NewInt(fh)); fm.Cmp(want) != 0 {
		t.Fatalf("follower 總產出≠高度×10: %s vs %d×10", fm, fh)
	}
}
