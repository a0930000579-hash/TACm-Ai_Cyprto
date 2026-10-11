package web

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"tacm/internal/chaindb"
	"tacm/internal/config"
	"tacm/internal/node"
)

// TestWSHandshakeAccept RFC6455 §1.3 官方範例向量。
func TestWSHandshakeAccept(t *testing.T) {
	got := wsAccept("dGhlIHNhbXBsZSBub25jZQ==")
	want := "s3pPLMBiTxaQ9kYGzzhZRbK+xOo="
	if got != want {
		t.Fatalf("accept = %q, want %q", got, want)
	}
}

// TestWSFrameRoundTrip 編碼（unmasked）＋解碼（masked）往返。
func TestWSFrameRoundTrip(t *testing.T) {
	// server→client unmasked
	var buf bytes.Buffer
	if err := writeFrame(&buf, opText, []byte(`{"ok":true}`), false); err != nil {
		t.Fatal(err)
	}
	op, payload, err := readFrame(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if op != opText || string(payload) != `{"ok":true}` {
		t.Fatalf("roundtrip: op=%d payload=%s", op, payload)
	}

	// client→server masked（RFC 6455 masked bit + mask key）
	var buf2 bytes.Buffer
	raw := []byte(`{"method":"eth_subscribe"}`)
	// 手工構建 masked frame：b0=0x81, b1=0x80|len, mask=4 bytes
	buf2.Write([]byte{0x81, 0x80 | byte(len(raw)), 0x11, 0x22, 0x33, 0x44})
	for i, c := range raw {
		buf2.Write([]byte{c ^ []byte{0x11, 0x22, 0x33, 0x44}[i%4]})
	}
	op2, payload2, err := readFrame(&buf2)
	if err != nil {
		t.Fatal(err)
	}
	if op2 != opText || string(payload2) != string(raw) {
		t.Fatalf("masked: op=%d payload=%s", op2, payload2)
	}
}

// wsTestClient 最小 WebSocket 客戶端（用本包 frame 函數）。
type wsTestClient struct {
	conn net.Conn
	r    *bufio.Reader
}

func dialWS(t *testing.T, url string) *wsTestClient {
	t.Helper()
	conn, err := net.Dial("tcp", strings.TrimPrefix(url, "http://"))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	key := base64.StdEncoding.EncodeToString([]byte("test-key-1234567890"))
	req := "GET /ws HTTP/1.1\r\nHost: x\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(conn)
	line, err := r.ReadString('\n')
	if err != nil || !strings.Contains(line, "101") {
		t.Fatalf("handshake: %q err=%v", line, err)
	}
	// 讀完剩餘 header
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(l) == "" {
			break
		}
	}
	return &wsTestClient{conn: conn, r: r}
}

func (c *wsTestClient) send(t *testing.T, payload string) {
	t.Helper()
	raw := []byte(payload)
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	hdr := []byte{0x81, 0x80 | byte(len(raw))}
	hdr = append(hdr, mask...)
	if _, err := c.conn.Write(hdr); err != nil {
		t.Fatal(err)
	}
	out := make([]byte, len(raw))
	for i := 0; i < len(raw); i++ {
		out[i] = raw[i] ^ mask[i%4]
	}
	if _, err := c.conn.Write(out); err != nil {
		t.Fatal(err)
	}
}

func (c *wsTestClient) recv(t *testing.T, timeout time.Duration) (byte, []byte) {
	t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(timeout))
	op, payload, err := readFrame(c.r)
	if err != nil {
		t.Fatalf("recv: %v", err)
	}
	return op, payload
}

// TestWSEthSubscribeE2E 真實節點：訂閱 newHeads → 節點出塊 → 收到通知。
func TestWSEthSubscribeE2E(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.BlockTime = 1

	n, err := node.New(cfg, "wse2e", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	genesis := &chaindb.Block{
		Height: 0, Hash: strings.Repeat("0", 64),
		Proposer: "genesis", Ts: time.Now().Unix(),
	}
	if err := n.DB().InsertBlock(genesis, nil); err != nil {
		t.Fatal(err)
	}
	n.Start()

	srv, err := New(NewNodeDataSource(n))
	if err != nil {
		t.Fatal(err)
	}
	srv.StartWS()
	defer srv.CloseWS()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c := dialWS(t, ts.URL)
	defer c.conn.Close()

	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"eth_subscribe","params":["newHeads"]}`)
	_, sub := c.recv(t, 5*time.Second)
	var sr struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  string `json:"result"`
	}
	if err := json.Unmarshal(sub, &sr); err != nil {
		t.Fatalf("subscribe resp: %v (%s)", err, sub)
	}
	if sr.Result == "" || sr.ID != 1 {
		t.Fatalf("subscribe resp: %s", sub)
	}

	// 節點持續出塊 → 應在數秒內收到 newHeads 通知。
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		_, msg := c.recv(t, 4*time.Second)
		var nt struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  struct {
				Result struct {
					Type string         `json:"type"`
					Data map[string]any `json:"data"`
				} `json:"result"`
			} `json:"params"`
		}
		if err := json.Unmarshal(msg, &nt); err != nil {
			t.Fatalf("notify: %v (%s)", err, msg)
		}
		if nt.Method == "eth_subscription" && nt.Params.Result.Type == "newHeads" {
			if nt.Params.Result.Data["number"] == "" {
				t.Fatalf("newHeads missing number: %s", msg)
			}
			return // 收到新頭部通知，測試通過
		}
	}
	t.Fatal("未在超時內收到 newHeads 通知")
}

// TestWSEthUnsubscribe 訂閱→取消→回應 true。
func TestWSEthUnsubscribe(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.BlockTime = 60 // 不出塊，專測訂閱管理

	n, err := node.New(cfg, "wsu2", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	genesis := &chaindb.Block{
		Height: 0, Hash: strings.Repeat("0", 64),
		Proposer: "genesis", Ts: time.Now().Unix(),
	}
	if err := n.DB().InsertBlock(genesis, nil); err != nil {
		t.Fatal(err)
	}
	n.Start()

	srv, err := New(NewNodeDataSource(n))
	if err != nil {
		t.Fatal(err)
	}
	srv.StartWS()
	defer srv.CloseWS()
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	c := dialWS(t, ts.URL)
	defer c.conn.Close()
	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"eth_subscribe","params":["newHeads"]}`)
	_, sub := c.recv(t, 5*time.Second)
	var sr struct {
		Result string `json:"result"`
	}
	_ = json.Unmarshal(sub, &sr)

	c.send(t, `{"jsonrpc":"2.0","id":2,"method":"eth_unsubscribe","params":["`+sr.Result+`"]}`)
	_, un := c.recv(t, 5*time.Second)
	var ur struct {
		ID     int  `json:"id"`
		Result bool `json:"result"`
	}
	if err := json.Unmarshal(un, &ur); err != nil {
		t.Fatalf("unsubscribe: %v (%s)", err, un)
	}
	if ur.ID != 2 || !ur.Result {
		t.Fatalf("unsubscribe resp: %s", un)
	}
}

var _ = http.MethodGet
