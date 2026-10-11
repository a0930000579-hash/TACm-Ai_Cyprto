package web

// M85-B：零依賴 WebSocket（RFC6455）＋以太坊式 eth_subscribe 訂閱。
// 支援：newHeads（鏈頂變化推送）、newPendingTransactions（交易池新增推送）、
// eth_unsubscribe。JSON-RPC 2.0 回應/通知格式，與 MetaMask/主流開發者工具相容。

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// ---- frame opcodes ----
const (
	opText  = 1
	opClose = 8
	opPing  = 9
	opPong  = 10
)

// wsConn 代表一條 WebSocket 連接（單寫鎖保護並發寫入）。
type wsConn struct {
	conn io.ReadWriteCloser
	mu   sync.Mutex
	subs map[string]string // 訂閱ID → 訂閱類型
}

func (c *wsConn) writeText(payload []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return writeFrame(c.conn, opText, payload, false)
}

// wsHub 管理所有活動連接與訂閱。
type wsHub struct {
	mu    sync.Mutex
	conns map[*wsConn]struct{}
}

func newWSHub() *wsHub { return &wsHub{conns: make(map[*wsConn]struct{})} }

func (h *wsHub) add(c *wsConn)    { h.mu.Lock(); h.conns[c] = struct{}{}; h.mu.Unlock() }
func (h *wsHub) remove(c *wsConn) { h.mu.Lock(); delete(h.conns, c); h.mu.Unlock() }

// broadcast 推送 eth_subscription 通知給訂閱了 subType 的連接。
func (h *wsHub) broadcast(subType string, params any) {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "method": "eth_subscription", "params": params,
	})
	if err != nil {
		return
	}
	h.mu.Lock()
	conns := make([]*wsConn, 0, len(h.conns))
	for c := range h.conns {
		conns = append(conns, c)
	}
	h.mu.Unlock()
	for _, c := range conns {
		c.mu.Lock()
		want := false
		for _, typ := range c.subs {
			if typ == subType {
				want = true
				break
			}
		}
		c.mu.Unlock()
		if want {
			_ = c.writeText(payload)
		}
	}
}

// handleWS 是 WebSocket 升級與訂閱處理入口（GET /ws）。
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if !isWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusBadRequest)
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return
	}
	accept := wsAccept(key)
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := conn.Write([]byte(resp)); err != nil {
		return
	}
	// 移除 bufio 殘留後改用原生 conn 讀寫（保持 RFC6455 幀位元組流）。
	c := &wsConn{conn: &brwConn{conn: conn, brw: brw}, subs: make(map[string]string)}
	s.hub.add(c)
	defer s.hub.remove(c)

	// 讀迴圈：解析 JSON-RPC 方法。
	for {
		op, payload, err := readFrame(c.conn)
		if err != nil {
			return
		}
		switch op {
		case opClose:
			_ = c.writeText(nil) // 不發內容，直接關閉
			return
		case opPing:
			_ = writeFrame(c.conn, opPong, payload, false)
			continue
		case opText:
			if len(payload) == 0 {
				continue
			}
			s.handleWSMessage(c, payload)
		}
	}
}

// brwConn 包裝 Hijack 後的 bufio.ReadWriter，使讀寫都落在同一緩衝流。
type brwConn struct {
	conn io.ReadWriteCloser
	brw  *bufio.ReadWriter
}

func (b *brwConn) Read(p []byte) (int, error) { return b.brw.Read(p) }
func (b *brwConn) Write(p []byte) (int, error) {
	n, err := b.brw.Write(p)
	if err == nil {
		err = b.brw.Flush() // bufio.Writer 緩衝，必須立即送出
	}
	return n, err
}
func (b *brwConn) Close() error { return b.conn.Close() }

// handleWSMessage 處理單條 JSON-RPC 請求。
func (s *Server) handleWSMessage(c *wsConn, payload []byte) {
	var req struct {
		JSONRPC string `json:"jsonrpc"`
		ID      any    `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		s.wsReplyError(c, nil, -32700, "parse error")
		return
	}
	switch req.Method {
	case "eth_subscribe":
		if len(req.Params) < 1 {
			s.wsReplyError(c, req.ID, -32602, "missing subscription type")
			return
		}
		typ, _ := req.Params[0].(string)
		switch typ {
		case "newHeads", "newPendingTransactions":
			subID := fmt.Sprintf("0x%x", time.Now().UnixNano())
			c.mu.Lock()
			c.subs[subID] = typ
			c.mu.Unlock()
			s.wsReplyResult(c, req.ID, subID)
		default:
			s.wsReplyError(c, req.ID, -32602, "unsupported subscription: "+typ)
		}
	case "eth_unsubscribe":
		if len(req.Params) < 1 {
			s.wsReplyError(c, req.ID, -32602, "missing subscription id")
			return
		}
		id, _ := req.Params[0].(string)
		c.mu.Lock()
		_, ok := c.subs[id]
		delete(c.subs, id)
		c.mu.Unlock()
		s.wsReplyResult(c, req.ID, ok)
	default:
		s.wsReplyError(c, req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) wsReplyResult(c *wsConn, id any, result any) {
	payload, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	_ = c.writeText(payload)
}

func (s *Server) wsReplyError(c *wsConn, id any, code int, msg string) {
	payload, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id,
		"error": map[string]any{"code": code, "message": msg},
	})
	_ = c.writeText(payload)
}

// wsPump 輪詢推送循環：newHeads（鏈頂變化）＋newPendingTransactions（交易池新增）。
func (s *Server) wsPump(stop <-chan struct{}) {
	lastHeight := int64(-1)
	lastPool := -1
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			st := s.ds.Status()
			h := st.Height
			if h != lastHeight {
				lastHeight = h
				if h >= 0 {
					head := map[string]any{
						"number":     "0x" + strconv.FormatInt(h, 16),
						"hash":       "",
						"proposer":   "",
						"difficulty": st.Difficulty,
						"ts":         0,
					}
					if bv, berr := s.ds.Block(h); berr == nil && bv != nil {
						head["hash"] = bv.Hash
						head["proposer"] = bv.Proposer
						head["ts"] = bv.Ts
					}
					s.hub.broadcast("newHeads", map[string]any{
						"subscription": "", "result": map[string]any{
							"type": "newHeads", "data": head,
						},
					})
				}
			}
			ps := st.MempoolSize
			if ps != lastPool {
				lastPool = ps
				hashes, err := s.ds.PendingTxHashes(50)
				if err == nil && len(hashes) > 0 {
					s.hub.broadcast("newPendingTransactions", map[string]any{
						"subscription": "", "result": map[string]any{
							"type": "newPendingTransactions", "data": hashes,
						},
					})
				}
			}
		}
	}
}

// ---- RFC6455 底層 ----

// wsAccept 計算 Sec-WebSocket-Accept。
func wsAccept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// writeFrame 寫一幀（server→client 不 mask）。
func writeFrame(w io.Writer, opcode byte, payload []byte, masked bool) error {
	b0 := 0x80 | (opcode & 0x0f)
	header := []byte{byte(b0)}
	l := len(payload)
	maskBit := byte(0)
	if masked {
		maskBit = 0x80
	}
	switch {
	case l < 126:
		header = append(header, maskBit|byte(l))
	case l < 65536:
		header = append(header, maskBit|126, byte(l>>8), byte(l))
	default:
		header = append(header, maskBit|127)
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(l))
		header = append(header, b[:]...)
	}
	if _, err := w.Write(header); err != nil {
		return err
	}
	if !masked {
		_, err := w.Write(payload)
		return err
	}
	mask := []byte{0x11, 0x22, 0x33, 0x44}
	if _, err := w.Write(mask); err != nil {
		return err
	}
	out := make([]byte, l)
	for i := 0; i < l; i++ {
		out[i] = payload[i] ^ mask[i%4]
	}
	_, err := w.Write(out)
	return err
}

// readFrame 讀一幀（client→server 為 masked）。
func readFrame(r io.Reader) (byte, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return 0, nil, err
	}
	fin := hdr[0]&0x80 != 0
	op := hdr[0] & 0x0f
	if !fin {
		return 0, nil, errors.New("ws: fragmented frames unsupported")
	}
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7f)
	switch length {
	case 126:
		var b [2]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		length = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, nil, err
		}
		length = binary.BigEndian.Uint64(b[:])
	}
	if length > 1<<20 {
		return 0, nil, errors.New("ws: frame too large")
	}
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(r, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}
	return op, payload, nil
}
