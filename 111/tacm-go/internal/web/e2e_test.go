package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tacm/internal/chaindb"
	"tacm/internal/config"
	"tacm/internal/node"
)

// TestExplorerRealNode 以運行中的真實節點端到端驗證區塊瀏覽（非靜態、非替身）。
func TestExplorerRealNode(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.BlockTime = 1

	n, err := node.New(cfg, "webe2e", 1)
	if err != nil {
		t.Fatalf("New node: %v", err)
	}
	defer n.Close()

	now := time.Now().Unix()
	genesis := &chaindb.Block{
		Height: 0, Hash: strings.Repeat("0", 64),
		Proposer: "genesis", Ts: now,
	}
	if err := n.DB().InsertBlock(genesis, nil); err != nil {
		t.Fatalf("創世: %v", err)
	}
	n.Start()

	// 等待至少出 2 個塊。
	deadline := time.Now().Add(15 * time.Second)
	for n.DB().GetTipHeight() < 2 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if n.DB().GetTipHeight() < 2 {
		t.Fatalf("未如期出塊，height=%d", n.DB().GetTipHeight())
	}

	srv, err := New(NewNodeDataSource(n))
	if err != nil {
		t.Fatalf("New web: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 首頁顯示真實鏈頂高度。
	resp, _ := http.Get(ts.URL + "/")
	body := readAllString(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("首頁 狀態=%d", resp.StatusCode)
	}
	if !strings.Contains(body, "最新區塊") {
		t.Errorf("首頁缺少區塊列表區塊")
	}
	resp.Body.Close()

	// 區塊詳情頁。
	resp, _ = http.Get(ts.URL + "/block/1")
	body = readAllString(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("區塊頁 狀態=%d", resp.StatusCode)
	}
	if !strings.Contains(body, "Merkle 根") {
		t.Errorf("區塊頁缺少 Merkle 字段")
	}
	resp.Body.Close()

	// 節點地址頁顯示 coinbase 累計餘額。
	resp, _ = http.Get(ts.URL + "/address/" + n.Address())
	body = readAllString(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("地址頁 狀態=%d", resp.StatusCode)
	}
	if !strings.Contains(body, "可用餘額") {
		t.Errorf("地址頁缺少餘額")
	}
	resp.Body.Close()

	// 真實 coinbase 交易詳情頁。
	detail, err := n.GetBlockDetail(1)
	if err != nil || len(detail.Transactions) == 0 {
		t.Fatalf("區塊1 無交易: err=%v", err)
	}
	txHash := detail.Transactions[0].TxHash
	resp, _ = http.Get(ts.URL + "/tx/" + txHash)
	body = readAllString(resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("交易頁 狀態=%d", resp.StatusCode)
	}
	if !strings.Contains(body, txHash[:12]) {
		t.Errorf("交易頁未顯示真實交易哈希")
	}
	resp.Body.Close()
}
