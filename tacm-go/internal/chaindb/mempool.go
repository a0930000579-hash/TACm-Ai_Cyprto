package chaindb

import (
	"encoding/json"
	"fmt"
)

// AddMempoolTx 將交易加入內存池：先淘汰過期交易，再寫入；
// 容量超限時淘汰手續費最低的交易。
func (c *ChainDB) AddMempoolTx(txHash string, payload map[string]any, signature, fee string) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	cutoff := nowUnix() - MempoolExpireSec
	if _, err := tx.Exec("DELETE FROM mempool WHERE received_at < ?", cutoff); err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("chaindb: 序列化內存池交易失敗: %w", err)
	}
	if _, err := tx.Exec(`
		INSERT OR REPLACE INTO mempool (tx_hash, payload, signature, received_at, fee)
		VALUES (?, ?, ?, ?, ?)`, txHash, string(raw), signature, nowUnix(), fee); err != nil {
		return err
	}

	var count int64
	if err := tx.QueryRow("SELECT COUNT(*) FROM mempool").Scan(&count); err != nil {
		return err
	}
	if count > MaxMempoolSize {
		evict := count - MaxMempoolSize
		if _, err := tx.Exec(`
			DELETE FROM mempool WHERE tx_hash IN (
			    SELECT tx_hash FROM mempool ORDER BY CAST(fee AS REAL) ASC LIMIT ?)`,
			evict); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMempool 返回按手續費降序的內存池交易，每筆補上 tx_hash 與 signature。
func (c *ChainDB) GetMempool(limit int) ([]map[string]any, error) {
	rows, err := c.db.Query(
		"SELECT tx_hash, payload, signature FROM mempool ORDER BY CAST(fee AS REAL) DESC LIMIT ?",
		limit)
	if err != nil {
		return nil, fmt.Errorf("chaindb: 讀內存池失敗: %w", err)
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		var txHash, payload, signature string
		if err := rows.Scan(&txHash, &payload, &signature); err != nil {
			return nil, err
		}
		var tx map[string]any
		if err := json.Unmarshal([]byte(payload), &tx); err != nil {
			continue // 損壞記錄跳過
		}
		tx["tx_hash"] = txHash
		tx["signature"] = signature
		out = append(out, tx)
	}
	return out, rows.Err()
}

// RemoveMempoolTx 按哈希移除內存池交易。
func (c *ChainDB) RemoveMempoolTx(txHash string) error {
	_, err := c.db.Exec("DELETE FROM mempool WHERE tx_hash = ?", txHash)
	return err
}

// MempoolSize 返回內存池交易數。
func (c *ChainDB) MempoolSize() int64 {
	var n int64
	if err := c.db.QueryRow("SELECT COUNT(*) FROM mempool").Scan(&n); err != nil {
		return 0
	}
	return n
}

// ---- Peer ----

// UpsertPeer 登記或更新對等節點信息（標記為在線）。
func (c *ChainDB) UpsertPeer(nodeID, rpcURL, owner string, height int64) error {
	_, err := c.db.Exec(`
		INSERT OR REPLACE INTO peers (node_id, rpc_url, owner, last_seen, online, height)
		VALUES (?, ?, ?, ?, 1, ?)`, nodeID, rpcURL, owner, nowUnix(), height)
	if err != nil {
		return fmt.Errorf("chaindb: 更新 peer 失敗: %w", err)
	}
	return nil
}

// GetPeers 返回已知節點；onlineOnly 時僅返回在線節點。
func (c *ChainDB) GetPeers(onlineOnly bool) ([]*Peer, error) {
	q := "SELECT node_id, rpc_url, owner, last_seen, online, height FROM peers"
	if onlineOnly {
		q += " WHERE online = 1"
	}
	q += " ORDER BY last_seen DESC"
	rows, err := c.db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("chaindb: 查 peers 失敗: %w", err)
	}
	defer rows.Close()

	var out []*Peer
	for rows.Next() {
		var p Peer
		if err := rows.Scan(&p.NodeID, &p.RPCURL, &p.Owner, &p.LastSeen,
			&p.Online, &p.Height); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}
