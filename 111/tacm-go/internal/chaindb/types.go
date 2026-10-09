// Package chaindb 實現 TAC 自主智能鏈的節點本地存儲：區塊、交易、賬戶狀態、
// 內存池與對等節點表。使用純 Go 的 SQLite 驅動，每個節點持有獨立數據庫。
package chaindb

// Block 為區塊頭記錄（blocks 表）。ProposerAddress 不寫入 blocks 表，
// 僅在插入時用於 coinbase 增發與手續費結算。
type Block struct {
	Height          int64   `json:"height"`
	Hash            string  `json:"hash"`
	PrevHash        *string `json:"prev_hash"`
	MerkleRoot      string  `json:"merkle_root"`
	Proposer        string  `json:"proposer"`
	ProposerAddress string  `json:"proposer_address,omitempty"`
	Ts              int64   `json:"ts"`
	TxCount         int     `json:"tx_count"`
	Difficulty      int     `json:"difficulty"`
	Nonce           int64   `json:"nonce"`
	Size            int     `json:"size"`
}

// Transaction 為已打包交易記錄（transactions 表）。
type Transaction struct {
	TxHash      string `json:"tx_hash"`
	BlockHeight int64  `json:"block_height"`
	BlockHash   string `json:"block_hash"`
	TxIndex     int    `json:"tx_index"`
	FromAddr    string `json:"from"`
	ToAddr      string `json:"to"`
	Amount      string `json:"amount"`
	Fee         string `json:"fee"`
	Nonce       int64  `json:"nonce"`
	Ts          int64  `json:"ts"`
	Signature   string `json:"signature"`
	Pubkey      string `json:"pubkey"`
	Memo        string `json:"memo"`
	Status      string `json:"status"`
}

// Account 為地址賬戶狀態（accounts 表）。
type Account struct {
	Address    string `json:"address"`
	Balance    string `json:"balance"`
	Nonce      int64  `json:"nonce"`
	Pubkey     string `json:"pubkey"`
	FirstSeen  int64  `json:"first_seen"`
	LastActive int64  `json:"last_active"`
}

// Peer 為已知對等節點（peers 表）。
type Peer struct {
	NodeID   string `json:"node_id"`
	RPCURL   string `json:"rpc_url"`
	Owner    string `json:"owner"`
	LastSeen int64  `json:"last_seen"`
	Online   int    `json:"online"`
	Height   int64  `json:"height"`
}
