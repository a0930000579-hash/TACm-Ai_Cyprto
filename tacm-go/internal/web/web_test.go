package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeDS struct {
	st           StatusView
	blockList    []BlockView
	blockResult  *BlockView
	txResult     *TxView
	addrResult   *AddressView
	walletResult *WalletView
}

func (f *fakeDS) Status() StatusView                   { return f.st }
func (f *fakeDS) Blocks(int) ([]BlockView, error)      { return f.blockList, nil }
func (f *fakeDS) Block(int64) (*BlockView, error)      { return f.blockResult, nil }
func (f *fakeDS) Transaction(string) (*TxView, error)  { return f.txResult, nil }
func (f *fakeDS) Address(string) (*AddressView, error) { return f.addrResult, nil }
func (f *fakeDS) Wallet(string) (*WalletView, error)   { return f.walletResult, nil }

func newFake() *fakeDS {
	now := time.Now().Unix()
	return &fakeDS{
		st:        StatusView{Height: 10, FinalizedHeight: 10, Difficulty: 3, MempoolSize: 1},
		blockList: []BlockView{{Height: 10, Hash: "0123456789abcdef0123456789abcdef", ProposerAddress: "tx0proposeraddr", TxCount: 1, Ts: now}},
		blockResult: &BlockView{Height: 5, Hash: "blockhash5", ProposerAddress: "tx0proposeraddr",
			Transactions: []TxView{{Hash: "txhash1", To: "tx0recipient", Amount: "10"}}},
		txResult:   &TxView{Hash: "txhash1", BlockHeight: 5, To: "tx0recipient", Amount: "10", Status: "confirmed"},
		addrResult: &AddressView{Address: "tx0proposeraddr", Balance: "100", Transactions: []TxView{{Hash: "txhash1"}}},
		walletResult: &WalletView{
			Address: "tx0proposeraddr", TACm: "1250000000000000000", TiUSD: "5000000",
			USDT: "3000000", SyncedBlock: 8, FeeTiUSDBps: 50, FeeUSDTBps: 125, FeeTACmBps: 200,
			TiUSDSupply: 5000000, TiUSDMinted: 5000000,
			Ledger: []WalletLedgerView{{Ts: 1700000000, Kind: "reward", Asset: "TACm", Account: "tx0proposeraddr", Delta: "1000000000000000000", Memo: "block:1:0"}},
		},
	}
}

func TestServerPages(t *testing.T) {
	srv, err := New(newFake())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	cases := []struct {
		path       string
		wantStatus int
		contains   string
	}{
		{"/", http.StatusOK, "最新區塊"},
		{"/block/5", http.StatusOK, "blockhash5"},
		{"/address/tx0proposeraddr", http.StatusOK, "100"},
		{"/tx/txhash1", http.StatusOK, "txhash1"},
		{"/wallet", http.StatusOK, "資產總覽"},
		{"/wallet?address=tx0proposeraddr", http.StatusOK, "1250000000000000000"},
		{"/static/css/style.css", http.StatusOK, "var(--bg)"},
	}
	for _, c := range cases {
		resp, err := http.Get(ts.URL + c.path)
		if err != nil {
			t.Fatalf("GET %s: %v", c.path, err)
		}
		body := readAllString(resp)
		if resp.StatusCode != c.wantStatus {
			t.Errorf("GET %s 狀態=%d 想要 %d", c.path, resp.StatusCode, c.wantStatus)
		}
		if c.contains != "" && !strings.Contains(body, c.contains) {
			t.Errorf("GET %s 未包含 %q", c.path, c.contains)
		}
		resp.Body.Close()
	}
}

func TestServerErrors(t *testing.T) {
	f := newFake()
	f.blockResult = nil // 缺失區塊
	f.txResult = nil    // 缺失交易
	srv, _ := New(f)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// 缺失區塊 → 404
	resp, _ := http.Get(ts.URL + "/block/999")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("缺失區塊 狀態=%d 想要 404", resp.StatusCode)
	}
	resp.Body.Close()

	// 非法高度 → 400
	resp, _ = http.Get(ts.URL + "/block/abc")
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("非法高度 狀態=%d 想要 400", resp.StatusCode)
	}
	resp.Body.Close()

	// 缺失交易 → 404
	resp, _ = http.Get(ts.URL + "/tx/missing")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("缺失交易 狀態=%d 想要 404", resp.StatusCode)
	}
	resp.Body.Close()

	// 未知路徑 → 404
	resp, _ = http.Get(ts.URL + "/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("未知路徑 狀態=%d 想要 404", resp.StatusCode)
	}
	resp.Body.Close()
}

func readAllString(resp *http.Response) string {
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}
