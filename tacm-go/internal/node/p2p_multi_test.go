package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

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
