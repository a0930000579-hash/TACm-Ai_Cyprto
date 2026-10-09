package node

import (
	"errors"
	"fmt"
	"strconv"

	"tacm/internal/chaindb"
	"tacm/internal/consensus/bft"
	"tacm/internal/crypto"
	"tacm/internal/p2p"
)

type p2pHost struct{ n *Node }

// P2PHost 返回適配 P2P 網絡層的節點宿主。
func (n *Node) P2PHost() p2p.Host { return &p2pHost{n: n} }

func (h *p2pHost) Height() int64          { return h.n.db.GetTipHeight() }
func (h *p2pHost) FinalizedHeight() int64 { return h.n.FinalizedHeight() }

func (h *p2pHost) BlockDetail(height int64) (*chaindb.Block, []chaindb.Transaction, error) {
	d, err := h.n.GetBlockDetail(height)
	if err != nil {
		return nil, nil, err
	}
	if d == nil {
		return nil, nil, nil
	}
	txs := make([]chaindb.Transaction, 0, len(d.Transactions))
	for _, t := range d.Transactions {
		txs = append(txs, *t)
	}
	return d.Block, txs, nil
}

// OnIncomingBlock 驗證並接入網絡傳來的下一個區塊；已有/缺口/分叉分別處理。
func (h *p2pHost) OnIncomingBlock(b *chaindb.Block, txs []chaindb.Transaction) error {
	n := h.n
	tip := n.db.GetTipHeight()
	if b.Height <= tip {
		return nil // 已存在
	}
	if b.Height != tip+1 {
		return fmt.Errorf("區塊缺口: tip=%d 收到 %d", tip, b.Height)
	}

	// prev_hash 鏈接校驗。
	tipBlock, err := n.db.GetBlock(tip)
	if err != nil {
		return err
	}
	wantPrev := ""
	if tipBlock != nil {
		wantPrev = tipBlock.Hash
	}
	prev := ""
	if b.PrevHash != nil {
		prev = *b.PrevHash
	}
	if prev != wantPrev {
		return errors.New("prev_hash 不匹配（可能分叉，交由重組）")
	}

	if err := verifyIncomingTxs(n, b, txs); err != nil {
		return err
	}

	// Merkle root 重算核對。
	txHashes := make([]string, 0, len(txs))
	for i := range txs {
		txHashes = append(txHashes, txs[i].TxHash)
	}
	mroot, err := crypto.MerkleRootStrings(txHashes)
	if err != nil {
		return err
	}
	if mroot != b.MerkleRoot {
		return errors.New("Merkle 根不匹配")
	}

	// PoW 核對。
	header := map[string]any{
		"height": b.Height, "prev_hash": prev, "merkle_root": b.MerkleRoot,
		"proposer": b.Proposer, "ts": b.Ts, "tx_count": len(txs),
		"nonce": b.Nonce, "difficulty": b.Difficulty,
	}
	ok, err := crypto.VerifyPow(header, b.Difficulty)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("PoW 驗證失敗")
	}

	b.TxCount = len(txs)
	if err := n.db.InsertBlock(b, txs); err != nil {
		return err
	}
	// 在本地執行外來塊的合約交易，部署/更新 code/storage。
	if err := n.executeBlockContracts(txs); err != nil {
		return err
	}
	// 錢包帳本同步（與本地出塊一致）。
	if err := n.syncWallet(b.Height, txs); err != nil {
		return fmt.Errorf("錢包同步 h=%d: %w", b.Height, err)
	}
	n.onNewBlock(b.Height, b.Hash)
	return nil
}

// OnIncomingTx 驗證網絡傳來的交易並加入內存池。
func (h *p2pHost) OnIncomingTx(tx map[string]any) error {
	if _, ok := tx["tx_hash"].(string); !ok {
		return errors.New("交易缺少 tx_hash")
	}
	// 已在內存池/賬本則忽略。
	if existing, _ := h.n.db.GetTransaction(getString(tx, "tx_hash")); existing != nil {
		return nil
	}
	_, err := h.n.SubmitTransaction(tx)
	return err
}

// OnIncomingVote 驗證並聚合網絡傳來的共識投票。
func (h *p2pHost) OnIncomingVote(v *bft.Vote) error {
	return h.n.handleIncomingVote(v)
}

// OnIncomingViewChange 接收網絡 view-change 票：驗證簽名 → 多數認證 → 切換輪次。
func (h *p2pHost) OnIncomingViewChange(height int64, round int32, validator, signature string) error {
	return h.n.handleIncomingViewChange(height, round, validator, signature)
}

// OnIncomingBridge 接收跨鏈網絡消息（提案/守衛簽名/執行結果）。
func (h *p2pHost) OnIncomingBridge(bg *p2p.BridgeGossip) error {
	return h.n.handleIncomingBridge(bg)
}

// acctState 為接入區塊時逐筆模擬的賬戶臨時狀態。
type acctState struct {
	bal   float64
	nonce int64
}

// verifyIncomingTxs 完整驗證區塊交易：coinbase 首位、普通交易簽名、
// nonce 順序與餘額（含同塊內連鎖轉賬）、手續費歸提議者。
func verifyIncomingTxs(n *Node, b *chaindb.Block, txs []chaindb.Transaction) error {
	if len(txs) == 0 {
		return errors.New("區塊缺少 coinbase 交易")
	}
	cb := txs[0]
	if cb.FromAddr != "" {
		return errors.New("首筆交易必須為 coinbase（from 為空）")
	}
	if cb.ToAddr != b.ProposerAddress {
		return errors.New("coinbase 接收方必須為提議者")
	}
	wantReward := chaindb.BlockReward(b.Height)
	gotReward, err := strconv.ParseFloat(cb.Amount, 64)
	if err != nil {
		return errors.New("coinbase 金額非法")
	}
	if abs(gotReward-wantReward) > 1e-9 {
		return fmt.Errorf("coinbase 金額=%g 應為 %g", gotReward, wantReward)
	}

	states := make(map[string]*acctState)
	get := func(addr string) *acctState {
		s, ok := states[addr]
		if !ok {
			bal, _ := strconv.ParseFloat(n.db.GetBalance(addr), 64)
			s = &acctState{bal: bal, nonce: n.db.GetNonce(addr)}
			states[addr] = s
		}
		return s
	}

	for i := 1; i < len(txs); i++ {
		t := txs[i]
		m := map[string]any{
			"from": t.FromAddr, "to": t.ToAddr, "amount": t.Amount, "fee": t.Fee,
			"memo": t.Memo, "ts": t.Ts, "nonce": t.Nonce, "pubkey": t.Pubkey,
		}
		if !verifyTxSignature(m, t.Signature, t.FromAddr) {
			return fmt.Errorf("交易 %d（%s）簽名無效", i, t.TxHash)
		}
		s := get(t.FromAddr)
		if t.Nonce != s.nonce {
			return fmt.Errorf("交易 %d nonce 錯亂: 應為 %d 得到 %d", i, s.nonce, t.Nonce)
		}
		amt, _ := strconv.ParseFloat(t.Amount, 64)
		fee, _ := strconv.ParseFloat(t.Fee, 64)
		if s.bal < amt+fee {
			return fmt.Errorf("交易 %d 付款方餘額不足", i)
		}
		s.bal -= amt + fee
		s.nonce++
		get(t.ToAddr).bal += amt
		if fee > 0 && b.ProposerAddress != t.FromAddr {
			get(b.ProposerAddress).bal += fee
		}
	}
	return nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
