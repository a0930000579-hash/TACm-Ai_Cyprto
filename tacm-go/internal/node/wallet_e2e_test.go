package node

import (
	"tacm/internal/wallet"
	"bytes"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"tacm/internal/crypto"
	"time"
)

// 啟動單節點出塊，等待錢包同步到指定高度。
func startMiningNode(t *testing.T) (*Node, int) {
	t.Helper()
	n, err := New(testCfg(t), "w0", 1)
	if err != nil {
		t.Fatal(err)
	}
	insertGenesis(t, n)
	n.Start()
	t.Cleanup(func() { _ = n.Close() })
	waitFor(t, func() bool { return n.DB().GetTipHeight() >= 2 }, 20*time.Second, "等待出塊到 h2")
	h, err := n.Wallet().SyncedHeight()
	if err != nil {
		t.Fatalf("同步高度讀取失敗: %v", err)
	}
	return n, int(h)
}

// coinbase 獎勵入帳：挖礦後 proposer 錢包餘額 > 0。
func TestWalletSyncCoinbase(t *testing.T) {
	n, synced := startMiningNode(t)
	if synced < 2 {
		t.Fatalf("錢包同步高度=%d 應 ≥2", synced)
	}
	acc, err := n.Wallet().Balance(n.nodeAddress)
	if err != nil {
		t.Fatal(err)
	}
	if acc.TACmBalance.Sign() <= 0 {
		t.Fatalf("proposer 應收到 coinbase 獎勵, 餘額=%s", acc.TACmBalance)
	}
	// 分潤機制（原本方式）：每塊 10 TACM，12% 挹注獎勵池、其餘 88% 按在線礦工算力瓜分；
	// 單節點下節點自身為唯一在線礦工（算力 1vCPU）→ 88% 歸節點。
	want, _ := new(big.Int).SetString("17600000000000000000", 10) // 88%×10×2
	if acc.TACmBalance.Cmp(want) < 0 {
		t.Fatalf("節點(礦工)分潤=%s 應 ≥1.76e19 (88pct coinbase×2塊)", acc.TACmBalance)
	}
	pool, err := n.Wallet().Balance(wallet.RewardPoolAddr)
	if err != nil {
		t.Fatal(err)
	}
	wantPool, _ := new(big.Int).SetString("2400000000000000000", 10) // 12%×10×2
	if pool.TACmBalance.Cmp(wantPool) != 0 {
		t.Fatalf("獎勵池挹注=%s want 2.4e18 (12pct coinbase×2塊)", pool.TACmBalance)
	}
}

// 站內 TiUSD 轉帳（0.5% 手續費）+ RPC 端點。
func TestWalletTransferRPC(t *testing.T) {
	n, _ := startMiningNode(t)
	rpc := NewRPCServer(n)
	ts := httptest.NewServer(rpc.Handler())
	defer ts.Close()

	// 充值 100 TiUSD 給 A。
	dep := postJSON(t, ts.URL+"/api/wallet/deposit", walletTxReq{To: "addrA", Asset: "TIUSD", Amount: "100"})
	if !dep["ok"].(bool) {
		t.Fatalf("充值失敗: %v", dep["error"])
	}
	// A → B 轉 10 TiUSD。
	tr := postJSON(t, ts.URL+"/api/wallet/transfer", walletTxReq{From: "addrA", To: "addrB", Asset: "TIUSD", Amount: "10", Memo: "pay"})
	if !tr["ok"].(bool) {
		t.Fatalf("轉帳失敗: %v", tr["error"])
	}
	// 手續費 10×0.5% = 0.05（fee_raw 為字串）。
	if fee := tr["fee_raw"].(string); fee != "0.05" {
		t.Fatalf("fee=%q want 0.05", fee)
	}
	a := getInfo(t, ts.URL, "addrA")
	if a.TiUSDRaw != 100_000_000-10_000_000-50_000 {
		t.Fatalf("A 餘額=%d", a.TiUSDRaw)
	}
	b := getInfo(t, ts.URL, "addrB")
	if b.TiUSDRaw != 10_000_000 {
		t.Fatalf("B 餘額=%d", b.TiUSDRaw)
	}
	// USDT 轉帳 1.25%：充值 100 → 轉 10 → fee 0.125。
	if err := postOK(t, ts.URL+"/api/wallet/deposit", walletTxReq{To: "addrC", Asset: "USDT", Amount: "100"}); err != nil {
		t.Fatal(err)
	}
	tr2 := postJSON(t, ts.URL+"/api/wallet/transfer", walletTxReq{From: "addrC", To: "addrD", Asset: "USDT", Amount: "10", Memo: "p"})
	if !tr2["ok"].(bool) {
		t.Fatalf("USDT 轉帳失敗: %v", tr2["error"])
	}
	if fee := tr2["fee_raw"].(string); fee != "0.125" {
		t.Fatalf("USDT fee=%q want 0.125", fee)
	}
	// 餘額不足拒絕。
	if err := postOK(t, ts.URL+"/api/wallet/transfer", walletTxReq{From: "addrB", To: "addrA", Asset: "TIUSD", Amount: "999"}); err == nil {
		t.Fatal("餘額不足應報錯")
	}
	// 帳本有審計分錄。
	res := getJSON(t, ts.URL+"/api/wallet/ledger?limit=50")
	items := res["items"].([]any)
	if len(items) < 6 {
		t.Fatalf("審計分錄太少: %d", len(items))
	}
}

// TiUSD 發行/銷毀 RPC + 供給摘要。
func TestTiUSDRPC(t *testing.T) {
	n, _ := startMiningNode(t)
	rpc := NewRPCServer(n)
	ts := httptest.NewServer(rpc.Handler())
	defer ts.Close()

	if err := postOK(t, ts.URL+"/api/tiusd/mint", walletTxReq{To: "Vault", Amount: "50", Note: "mint"}); err != nil {
		t.Fatal(err)
	}
	sum := getJSON(t, ts.URL+"/api/tiusd/summary")
	if sum["supply_raw"].(float64) != 50_000_000 {
		t.Fatalf("供給=%v want 50 TiUSD", sum["supply_raw"])
	}
	if err := postOK(t, ts.URL+"/api/tiusd/burn", walletTxReq{Amount: "20", Note: "burn"}); err != nil {
		t.Fatal(err)
	}
	sum = getJSON(t, ts.URL+"/api/tiusd/summary")
	if sum["supply_raw"].(float64) != 30_000_000 {
		t.Fatalf("銷毀後供給=%v want 30", sum["supply_raw"])
	}
	v := getInfo(t, ts.URL, "Vault")
	if v.TiUSDRaw != 50_000_000 {
		t.Fatalf("Vault 入帳=%d want 50 TiUSD", v.TiUSDRaw)
	}
}

// 鏈上交易（mempool 簽名轉帳 → 打包）隨區塊同步進錢包。
func TestWalletSyncChainTx(t *testing.T) {
	n, _ := startMiningNode(t)
	bobKP, err := crypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	bobAddr := mustAddr(t, bobKP)
	tx := signedTx(t, n.keypair, bobAddr, "1.5", "0.03", n.DB().GetNonce(n.nodeAddress))
	if _, err := n.SubmitTransaction(tx); err != nil {
		t.Fatal(err)
	}
	// 等待包含該交易的區塊入錢包。
	waitFor(t, func() bool {
		acc, err := n.Wallet().Balance(bobAddr)
		return err == nil && acc.TACmBalance.Sign() > 0
	}, 15*time.Second, "等待鏈上轉帳同步到錢包")
	acc, _ := n.Wallet().Balance(bobAddr)
	want, _ := new(big.Int).SetString("1500000000000000000", 10)
	if acc.TACmBalance.Cmp(want) != 0 {
		t.Fatalf("bob 餘額=%s want 1.5 TACM", acc.TACmBalance)
	}
	// 付款人（proposer）被扣款（含手續費）。
	proposer, _ := n.Wallet().Balance(n.nodeAddress)
	if proposer.TACmBalance.Sign() <= 0 {
		t.Fatal("proposer 餘額異常")
	}
}

// 測試輔助。

func postJSON(t *testing.T, url string, body any) map[string]any {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析響應失敗: %v", err)
	}
	return out
}

func postOK(t *testing.T, url string, body any) error {
	t.Helper()
	out := postJSON(t, url, body)
	if ok, _ := out["ok"].(bool); !ok {
		if e, _ := out["error"].(string); e != "" {
			return &rpcErr{e}
		}
		return &rpcErr{"unknown"}
	}
	return nil
}

type rpcErr struct{ s string }

func (e *rpcErr) Error() string { return e.s }

func getInfo(t *testing.T, base, addr string) walletView {
	t.Helper()
	res := getJSON(t, base+"/api/wallet/info?address="+addr)
	b, _ := json.Marshal(res)
	var v walletView
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("解析 walletView 失敗: %v", err)
	}
	return v
}
