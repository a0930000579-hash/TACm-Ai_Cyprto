package node

import (
	"fmt"
	"time"

	"tacm/internal/chaindb"
	"tacm/internal/crypto"
)

// Status 為節點對外狀態（M2 單節點；P2P/BFT 字段在後續里程碑補充）。
type Status struct {
	NodeID         string  `json:"node_id"`
	Address        string  `json:"address"`
	Network        string  `json:"network"`
	ChainID        string  `json:"chain_id"`
	BlockHeight    int64   `json:"block_height"`
	FinalHeight    int64   `json:"final_block_height"`
	MempoolSize    int64   `json:"mempool_size"`
	Difficulty     int     `json:"effective_difficulty"`
	UptimeSec      int64   `json:"uptime_sec"`
	Consensus      string  `json:"consensus"`
	EmissionModel  string  `json:"emission_model"`
	MaxSupply      float64 `json:"max_supply"`
	EmissionYears  int     `json:"emission_years"`
	AnnualDecayPct float64 `json:"annual_decay_pct"`
}

// GetStatus 返回節點當前狀態。
func (n *Node) GetStatus() Status {
	n.mu.Lock()
	diff := n.adjDiff
	dist := n.distributed
	n.mu.Unlock()
	consensus := "pow_dynamic"
	if dist {
		consensus = "pow_bft_distributed"
	}
	// 難度是鏈上事實：以鏈頂區塊記錄的難度為準（follower 與錨點顯示同一值），
	// 僅在鏈空（創世後尚未出塊）時退回本地調整值。
	if tip := n.db.GetTipHeight(); tip > 0 {
		if b, err := n.db.GetBlock(tip); err == nil && b != nil && b.Difficulty > 0 {
			diff = b.Difficulty
		}
	}
	st := Status{
		NodeID:        n.nodeID,
		Address:       n.nodeAddress,
		Network:       n.cfg.Network,
		ChainID:       n.cfg.ChainID,
		BlockHeight:   n.db.GetTipHeight(),
		FinalHeight:   n.FinalizedHeight(),
		MempoolSize:   n.db.MempoolSize(),
		Difficulty:    diff,
		UptimeSec:     int64(time.Since(n.onlineSince).Seconds()),
		Consensus:     consensus,
		EmissionModel: "halving",
	}
	if cfg, err := chaindb.LoadEmission(int64(n.cfg.BlockTime)); err == nil && cfg.Model == "annual_decay" {
		st.EmissionModel = cfg.Model
		st.MaxSupply = cfg.MaxSupply
		st.EmissionYears = cfg.EmissionYears
		st.AnnualDecayPct = cfg.AnnualDecayPct
	}
	return st
}

// BlockDetail 為帶交易列表的區塊詳情。
type BlockDetail struct {
	*chaindb.Block
	Transactions []*chaindb.Transaction `json:"transactions"`
}

// GetBlockDetail 返回區塊頭及其交易；不存在返回 (nil, nil)。
func (n *Node) GetBlockDetail(height int64) (*BlockDetail, error) {
	b, err := n.db.GetBlock(height)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, nil
	}
	txs, err := n.db.GetTransactionsByBlock(height)
	if err != nil {
		return nil, err
	}
	return &BlockDetail{Block: b, Transactions: txs}, nil
}

// HeaderOnly 提取輕節點所需的區塊頭字段。
func HeaderOnly(b *chaindb.Block) map[string]any {
	return map[string]any{
		"height": b.Height, "hash": b.Hash, "prev_hash": b.PrevHash,
		"merkle_root": b.MerkleRoot, "proposer": b.Proposer,
		"proposer_address": b.ProposerAddress, "ts": b.Ts,
		"tx_count": b.TxCount, "nonce": b.Nonce, "difficulty": b.Difficulty,
	}
}

// TxProof 為交易的 Merkle 包含證明 + 所在塊頭。
type TxProof struct {
	Found  bool           `json:"found"`
	Height int64          `json:"height"`
	Index  int            `json:"index"`
	Proof  []string       `json:"proof"`
	Header map[string]any `json:"header"`
}

// GetTxProof 構造交易的 Merkle 包含證明；交易不存在返回 (nil, nil)。
func (n *Node) GetTxProof(txHash string) (*TxProof, error) {
	tx, err := n.db.GetTransaction(txHash)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, nil
	}
	height := tx.BlockHeight
	txs, err := n.db.GetTransactionsByBlock(height)
	if err != nil {
		return nil, err
	}
	txHashes := make([]any, len(txs))
	index := -1
	for i, t := range txs {
		txHashes[i] = t.TxHash
		if t.TxHash == txHash {
			index = i
		}
	}
	if index < 0 {
		return nil, fmt.Errorf("交易 %s 不在區塊 %d 的 Merkle 樹中", txHash, height)
	}
	proof, err := crypto.MerkleProof(txHashes, index)
	if err != nil {
		return nil, err
	}
	b, err := n.db.GetBlock(height)
	if err != nil {
		return nil, err
	}
	return &TxProof{
		Found: true, Height: height, Index: index,
		Proof: proof, Header: HeaderOnly(b),
	}, nil
}

// GetHeadersWindow 返回最新 count 個區塊頭與鏈頂高度（輕節點同步用）。
func (n *Node) GetHeadersWindow(count int) (map[string]any, error) {
	tip := n.db.GetTipHeight()
	start := tip - int64(count) + 1
	if start < 0 {
		start = 0
	}
	headers := make([]map[string]any, 0, count)
	for h := start; h <= tip; h++ {
		b, err := n.db.GetBlock(h)
		if err != nil {
			return nil, err
		}
		if b != nil {
			headers = append(headers, HeaderOnly(b))
		}
	}
	return map[string]any{"headers": headers, "tip": tip}, nil
}
