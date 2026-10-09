package node

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tacm/internal/crypto"
	"tacm/internal/wallet"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
)

// 本檔驗證 M49 eth_* JSON-RPC 相容層：
//  1. eth_chainId / eth_blockNumber / web3_clientVersion / net_version
//  2. eth_getBalance / eth_getTransactionCount / eth_getCode
//  3. eth_call（只讀模擬合約查詢）
//  4. eth_sendRawTransaction（EIP-155 簽名→RLP→TAC 交易→入塊）
//  5. 壞簽名交易被拒

func ethRPC(t *testing.T, srv *httptest.Server, method string, params ...any) map[string]any {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL+"/eth", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("eth %s 失敗: %v", method, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析 eth %s 失敗: %v", method, err)
	}
	return out
}

func ethResult(t *testing.T, srv *httptest.Server, method string, params ...any) any {
	t.Helper()
	out := ethRPC(t, srv, method, params...)
	if e, ok := out["error"]; ok {
		t.Fatalf("eth %s error: %v", method, e)
	}
	return out["result"]
}

// testRLP 為測試專用簡易 RLP（僅 eth 交易所需；與 crypto 私有實作一致）。
func testRLP(b ...[]byte) []byte {
	body := []byte{}
	for _, it := range b {
		body = append(body, it...)
	}
	if len(body) < 56 {
		return append([]byte{0xC0 + byte(len(body))}, body...)
	}
	lenB := big.NewInt(int64(len(body))).Bytes()
	return append(append([]byte{0xF7 + byte(len(lenB))}, lenB...), body...)
}

func testRLPInt(v *big.Int) []byte {
	if v.Sign() == 0 {
		return []byte{0x80}
	}
	b := v.Bytes()
	if len(b) == 1 && b[0] < 0x80 {
		return b
	}
	return append([]byte{0x80 + byte(len(b))}, b...)
}

func testRLPBytes(b []byte) []byte {
	if len(b) == 1 && b[0] < 0x80 {
		return b
	}
	if len(b) < 56 {
		return append([]byte{0x80 + byte(len(b))}, b...)
	}
	lenB := big.NewInt(int64(len(b))).Bytes()
	return append(append([]byte{0xB7 + byte(len(lenB))}, lenB...), b...)
}

// ethSignRawTx 用測試私鑰簽署 EIP-155 交易並回 RLP hex。
func ethSignRawTx(t *testing.T, key *btcec.PrivateKey, nonce, gasPrice, gas, value int64,
	to []byte, data []byte, chainID int64) string {
	t.Helper()
	payload := testRLP(
		testRLPInt(big.NewInt(nonce)),
		testRLPInt(big.NewInt(gasPrice)),
		testRLPInt(big.NewInt(gas)),
		testRLPBytes(to),
		testRLPInt(big.NewInt(value)),
		testRLPBytes(data),
	)
	digest := crypto.Keccak256(payload)
	sig := ecdsa.Sign(key, digest)
	r, s := derToRSLocal(sig.Serialize())
	expected := key.PubKey()
	var recid byte
	for rc := byte(0); rc < 4; rc++ {
		compact := make([]byte, 65)
		compact[0] = 27 + rc
		r.FillBytes(compact[1:33])
		s.FillBytes(compact[33:65])
		pub, _, err := ecdsa.RecoverCompact(compact, digest)
		if err == nil && pub.IsEqual(expected) {
			recid = rc
			break
		}
	}
	var v int64
	if chainID > 0 {
		v = 35 + 2*chainID + int64(recid)
	} else {
		v = 27 + int64(recid)
	}
	raw := testRLP(
		testRLPInt(big.NewInt(nonce)),
		testRLPInt(big.NewInt(gasPrice)),
		testRLPInt(big.NewInt(gas)),
		testRLPBytes(to),
		testRLPInt(big.NewInt(value)),
		testRLPBytes(data),
		testRLPInt(big.NewInt(v)),
		testRLPInt(r),
		testRLPInt(s),
	)
	return hex.EncodeToString(raw)
}

// derToRSLocal 解析 DER 簽名 R/S（測試輔助）。
func derToRSLocal(der []byte) (*big.Int, *big.Int) {
	i := 2
	readInt := func() *big.Int {
		if i+2 > len(der) || der[i] != 0x02 {
			panic("bad der")
		}
		l := int(der[i+1])
		v := new(big.Int).SetBytes(der[i+2 : i+2+l])
		i += 2 + l
		return v
	}
	return readInt(), readInt()
}

func TestEthRPCBasics(t *testing.T) {
	_, srv, admin := contractStudioSetup(t)

	if got := ethResult(t, srv, "eth_chainId"); got != "0x539" {
		t.Fatalf("eth_chainId=%v", got)
	}
	if got := ethResult(t, srv, "net_version"); got != "0x539" {
		t.Fatalf("net_version=%v", got)
	}
	if got := ethResult(t, srv, "web3_clientVersion"); got == nil {
		t.Fatal("web3_clientVersion 為空")
	}
	bn, ok := ethResult(t, srv, "eth_blockNumber").(string)
	if !ok || !strings.HasPrefix(bn, "0x") {
		t.Fatalf("eth_blockNumber=%v", bn)
	}
	// 節點金鑰地址餘額（0x 格式）。
	admin0x, err := tx0To0x(admin)
	if err != nil {
		t.Fatal(err)
	}
	bal, ok := ethResult(t, srv, "eth_getBalance", admin0x).(string)
	if !ok {
		t.Fatalf("eth_getBalance=%v", bal)
	}
	// 未分配地址餘額為 0x0。
	bal0, _ := ethResult(t, srv, "eth_getBalance", "0x"+strings.Repeat("aa", 20)).(string)
	if bal0 != "0x0" && bal0 != "" {
		t.Fatalf("空地址餘額=%v", bal0)
	}
	_ = ethResult(t, srv, "eth_gasPrice")
}

func TestEthSendRawTransactionAndCall(t *testing.T) {
	n, srv, admin := contractStudioSetup(t)

	key, err := btcec.NewPrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	fromHash := crypto.Hash160(key.PubKey().SerializeCompressed())
	from := crypto.Hash160ToAddress(fromHash)

	// 給 eth 簽名者創世分配＋錢包帳戶（鏈上餘額與錢包帳戶需一致）。
	insertGenesisWithAlloc(t, n, from)
	if err := n.Wallet().Deposit(from, wallet.AssetTACm,
		wallet.Amount{Big: new(big.Int).Mul(big.NewInt(10), new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))},
		"eth-test-fund"); err != nil {
		t.Fatalf("Deposit 失敗: %v", err)
	}

	admin0x, err := tx0To0x(admin)
	if err != nil {
		t.Fatal(err)
	}

	// 1. 轉帳交易（value=5, gas=100, gasPrice=1, fee=100）入塊。
	raw := ethSignRawTx(t, key, 0, 1, 100, 5_000_000_000_000_000_000, mustBytes20(t, admin0x), nil, crypto.EthChainID)
	out := ethRPC(t, srv, "eth_sendRawTransaction", "0x"+raw)
	if e, ok := out["error"]; ok {
		t.Fatalf("eth_sendRawTransaction error: %v", e)
	}
	txHash, _ := out["result"].(string)
	if txHash == "" {
		t.Fatal("無 tx hash")
	}
	waitFor(t, func() bool {
		tx, err := n.db.GetTransaction(txHash)
		if tx != nil && tx.Status != "confirmed" {
			t.Logf("tx status=%s height=%v", tx.Status, tx.BlockHeight)
		}
		return err == nil && tx != nil && tx.Status == "confirmed"
	}, 8*time.Second, "eth 交易未入塊")
	t.Logf("鏈高=%v adminBal(chain)=%v", n.db.GetTipHeight(), n.db.GetBalance(admin))

	// 2. eth_getBalance：admin 收到 5e18 wei（餘額含挖礦獎勵，檢查 ≥5e18）。
	adminBal, ok := ethResult(t, srv, "eth_getBalance", admin0x).(string)
	if !ok {
		t.Fatal("eth_getBalance admin 失敗")
	}
	adminBalInt, _ := new(big.Int).SetString(strings.TrimPrefix(adminBal, "0x"), 16)
	if adminBalInt.Cmp(new(big.Int).Mul(big.NewInt(5), big.NewInt(1_000_000_000_000_000_000))) < 0 {
		t.Fatalf("admin 餘額應 ≥5e18，got %v", adminBal)
	}

	// 3. eth_getTransactionCount：發送者 nonce=1。
	from0x, _ := tx0To0x(from)
	nonce, _ := ethResult(t, srv, "eth_getTransactionCount", from0x).(string)
	if nonce != "0x1" {
		t.Fatalf("nonce=%v 期望 0x1", nonce)
	}

	// 4. 壞簽名交易被拒。
	// 壞交易：nonce 跳號（2）→ 鏈上 nonce 校驗拒絕。
	bad := ethSignRawTx(t, key, 2, 1, 100, 5_000_000_000_000_000_000, mustBytes20(t, admin0x), nil, crypto.EthChainID)
	out2 := ethRPC(t, srv, "eth_sendRawTransaction", "0x"+bad)
	if e, ok := out2["error"]; !ok {
		t.Fatalf("跳號交易應被拒，結果=%v", out2["result"])
	} else {
		t.Logf("跳號交易被拒（符合預期）: %v", e)
	}

	// 5. eth_call：部署代幣後查詢 totalSupply。
	dep := studioPostJSON(t, srv, "/contract/deploy", map[string]any{
		"name": "EthToken", "symbol": "ETK", "supply": "777000",
	})
	if dep["ok"] != true {
		t.Fatalf("部署失敗: %v", dep)
	}
	addr := dep["contract_address"].(string)
	waitFor(t, func() bool {
		info := n.contracts.Get(must0x(t, addr))
		return info != nil && info.CodeSize > 0
	}, 8*time.Second, "合約未入塊")

	// 6. eth_getCode：0x 開頭且非空。
	code, _ := ethResult(t, srv, "eth_getCode", must0x(t, addr)).(string)
	if !strings.HasPrefix(code, "0x") || len(code) <= 2 {
		t.Fatalf("eth_getCode=%v", code)
	}

	// 7. eth_call：totalSupply() = 0x18160ddd → 777000。
	callOut := ethRPC(t, srv, "eth_call", map[string]any{
		"to":   must0x(t, addr),
		"data": "0x18160ddd",
	})
	if e, ok := callOut["error"]; ok {
		t.Fatalf("eth_call error: %v", e)
	}
	ret, _ := callOut["result"].(string)
	got := new(big.Int).SetBytes(mustBytes20(t, strings.TrimPrefix(ret, "0x")))
	if got.Cmp(big.NewInt(777000)) != 0 {
		t.Fatalf("totalSupply=%v 期望 777000", got)
	}
}

func TestEthGetBlockByNumber(t *testing.T) {
	_, srv, _ := contractStudioSetup(t)
	blk, ok := ethResult(t, srv, "eth_getBlockByNumber", "latest", false).(map[string]any)
	if !ok {
		t.Fatal("eth_getBlockByNumber 失敗")
	}
	if blk["number"] == nil {
		t.Fatal("block 無 number")
	}
}

// mustBytes20 把 0x hex（40 字元）轉 20 bytes。
func mustBytes20(t *testing.T, s string) []byte {
	t.Helper()
	s = strings.TrimPrefix(s, "0x")
	// 可能大於 40（如 32-byte return）：取低 20 字節；不足則左補零。
	if len(s) > 40 {
		s = s[len(s)-40:]
	} else if len(s) < 40 {
		s = strings.Repeat("0", 40-len(s)) + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
