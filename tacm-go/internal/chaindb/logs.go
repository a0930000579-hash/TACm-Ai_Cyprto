package chaindb

import (
	"database/sql"
	"fmt"
	"strings"
)

// LogRow 為鏈上事件日誌記錄（logs 表），以以太 eth_getLogs 欄位語義對外輸出。
type LogRow struct {
	ID          int64    `json:"-"`
	TxHash      string   `json:"transaction_hash"`
	BlockHeight int64    `json:"block_number"`
	TxIndex     int      `json:"transaction_index"`
	LogIndex    int      `json:"log_index"`
	Address     string   `json:"address"`
	Topics      []string `json:"topics"`
	Data        string   `json:"data"`
}

// LogFilter 為事件日誌查詢條件；空值欄位不參與過濾。
type LogFilter struct {
	FromBlock int64
	ToBlock   int64
	TxHash    string
	Address   string // 0x 前綴 EVM 地址（logs 表存 0x 前綴）
	Topic0    string // 0x 前綴事件簽名
	Limit     int
}

// InsertLogs 寫入一筆交易的完整事件日誌（合約執行後呼叫）。
func (c *ChainDB) InsertLogs(txHash string, blockHeight int64, txIndex int, rows []LogRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := c.db.Begin()
	if err != nil {
		return fmt.Errorf("chaindb: 開啟日誌事務失敗: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for i := range rows {
		l := &rows[i]
		t0, t1, t2, t3 := "", "", "", ""
		if len(l.Topics) > 0 {
			t0 = l.Topics[0]
		}
		if len(l.Topics) > 1 {
			t1 = l.Topics[1]
		}
		if len(l.Topics) > 2 {
			t2 = l.Topics[2]
		}
		if len(l.Topics) > 3 {
			t3 = l.Topics[3]
		}
		if _, err := tx.Exec(`
			INSERT INTO logs
			(tx_hash, block_height, tx_index, log_index, address, topic0, topic1, topic2, topic3, data)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			txHash, blockHeight, txIndex, i, l.Address, t0, t1, t2, t3, l.Data); err != nil {
			return fmt.Errorf("chaindb: 寫入日誌失敗: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("chaindb: 提交日誌事務失敗: %w", err)
	}
	return nil
}

// GetLogs 依條件查詢事件日誌（按區塊/索引升序）。
func (c *ChainDB) GetLogs(f LogFilter) ([]LogRow, error) {
	where := []string{"1=1"}
	args := []any{}
	if f.FromBlock > 0 {
		where = append(where, "block_height >= ?")
		args = append(args, f.FromBlock)
	}
	if f.ToBlock > 0 {
		where = append(where, "block_height <= ?")
		args = append(args, f.ToBlock)
	}
	if f.TxHash != "" {
		where = append(where, "tx_hash = ?")
		args = append(args, f.TxHash)
	}
	if f.Address != "" {
		where = append(where, "address = ?")
		args = append(args, f.Address)
	}
	if f.Topic0 != "" {
		where = append(where, "topic0 = ?")
		args = append(args, f.Topic0)
	}
	limit := f.Limit
	if limit <= 0 || limit > 10000 {
		limit = 10000
	}
	args = append(args, limit)
	rows, err := c.db.Query(`
		SELECT id, tx_hash, block_height, tx_index, log_index, address,
		       topic0, topic1, topic2, topic3, data
		FROM logs WHERE `+strings.Join(where, " AND ")+`
		ORDER BY block_height ASC, tx_index ASC, log_index ASC LIMIT ?`, args...)
	if err != nil {
		return nil, fmt.Errorf("chaindb: 查日誌失敗: %w", err)
	}
	defer rows.Close()
	var out []LogRow
	for rows.Next() {
		var (
			l                   LogRow
			addr                string
			t0, t1, t2, t3, dt  sql.NullString
		)
		if err := rows.Scan(&l.ID, &l.TxHash, &l.BlockHeight, &l.TxIndex, &l.LogIndex,
			&addr, &t0, &t1, &t2, &t3, &dt); err != nil {
			return nil, fmt.Errorf("chaindb: 掃描日誌失敗: %w", err)
		}
		l.Address = addr
		l.Topics = []string{}
		if t0.Valid {
			l.Topics = append(l.Topics, t0.String)
		}
		if t1.Valid {
			l.Topics = append(l.Topics, t1.String)
		}
		if t2.Valid {
			l.Topics = append(l.Topics, t2.String)
		}
		if t3.Valid {
			l.Topics = append(l.Topics, t3.String)
		}
		l.Data = dt.String
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpdateTxGasUsed 回寫合約交易實際消耗的 gas（收據用）。
func (c *ChainDB) UpdateTxGasUsed(txHash string, gasUsed int64) error {
	if _, err := c.db.Exec("UPDATE transactions SET gas_used = ? WHERE tx_hash = ?", gasUsed, txHash); err != nil {
		return fmt.Errorf("chaindb: 更新交易 gas_used 失敗: %w", err)
	}
	if c.cache != nil {
		c.cache.invalidateTx(txHash)
	}
	return nil
}
