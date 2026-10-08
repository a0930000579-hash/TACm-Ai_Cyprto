package web

import (
	"tacm/internal/chaindb"
	"tacm/internal/node"
	"tacm/internal/wallet"
)

// DataSource 抽象 Web 層所需的鏈數據，便於以節點實現或測試替身注入。
type DataSource interface {
	Status() StatusView
	Blocks(limit int) ([]BlockView, error)
	Block(height int64) (*BlockView, error)
	Transaction(hash string) (*TxView, error)
	Address(addr string) (*AddressView, error)
	Wallet(addr string) (*WalletView, error)
}

type nodeDS struct{ n *node.Node }

// NewNodeDataSource 把運行中的節點適配為 Web 數據源。
func NewNodeDataSource(n *node.Node) DataSource { return &nodeDS{n: n} }

// walletView 把錢包服務快照轉為 Web 視圖。
func (d *nodeDS) Wallet(addr string) (*WalletView, error) {
	if d.n.Wallet() == nil {
		return nil, nil // 錢包未啟用：頁面顯示提示
	}
	acc, err := d.n.Wallet().Balance(addr)
	if err != nil {
		return nil, err
	}
	led, err := d.n.Wallet().Ledger(30)
	if err != nil {
		return nil, err
	}
	sup, _ := d.n.Wallet().TiUSDSummary()
	synced, _ := d.n.Wallet().SyncedHeight()
	w := &WalletView{
		Address:     addr,
		// M32：錢包只顯示個人資產（十進制縮放）——raw 不再外洩。
		TACm:        wallet.FormatAmountBig(acc.TACmBalance),
		TiUSD:       wallet.FormatAmountI64(acc.TiUSDBalance, wallet.AssetTiUSD),
		USDT:        wallet.FormatAmountI64(acc.USDTBalance, wallet.AssetUSDT),
		SyncedBlock: synced,
		FeeTiUSDBps: 50, FeeUSDTBps: 125, FeeTACmBps: 200,
		NodeAddress: d.n.Address(),
	}
	// TiUSD 供給（最小單位 → 十進制）。
	if sup != nil {
		w.TiUSDSupply = sup.Supply
		w.TiUSDMinted = sup.TotalMinted
		w.TiUSDBurned = sup.TotalBurned
	}
	for _, e := range led {
		w.Ledger = append(w.Ledger, WalletLedgerView{
			Ts: e.Ts, Kind: string(e.Kind), Asset: string(e.Asset),
			Account: e.Account, Delta: e.Delta, Memo: e.Memo,
		})
	}
	return w, nil
}

func txView(t *chaindb.Transaction) TxView {
	return TxView{
		Hash: t.TxHash, BlockHeight: t.BlockHeight, From: t.FromAddr, To: t.ToAddr,
		Amount: t.Amount, Fee: t.Fee, Nonce: t.Nonce, Ts: t.Ts,
		Memo: t.Memo, Status: t.Status,
	}
}

func blockView(b *chaindb.Block) BlockView {
	prev := ""
	if b.PrevHash != nil {
		prev = *b.PrevHash
	}
	bv := BlockView{
		Height: b.Height, Hash: b.Hash, PrevHash: prev, MerkleRoot: b.MerkleRoot,
		Proposer: b.Proposer, ProposerAddress: b.ProposerAddress, Ts: b.Ts,
		TxCount: b.TxCount, Difficulty: b.Difficulty, Nonce: b.Nonce, Size: b.Size,
	}
	if b.Height > 0 {
		bv.PrevHeight = b.Height - 1
	}
	return bv
}

func (d *nodeDS) Status() StatusView {
	s := d.n.GetStatus()
	return StatusView{
		NodeID: s.NodeID, Address: s.Address, Height: s.BlockHeight,
		FinalizedHeight: d.n.FinalizedHeight(), Difficulty: s.Difficulty,
		MempoolSize: int(s.MempoolSize), UptimeSec: s.UptimeSec,
		Consensus: s.Consensus,
	}
}

func (d *nodeDS) Blocks(limit int) ([]BlockView, error) {
	blocks, err := d.n.DB().GetBlocks(limit, 0)
	if err != nil {
		return nil, err
	}
	out := make([]BlockView, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, blockView(b))
	}
	return out, nil
}

func (d *nodeDS) Block(height int64) (*BlockView, error) {
	detail, err := d.n.GetBlockDetail(height)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, nil
	}
	bv := blockView(detail.Block)
	bv.Transactions = make([]TxView, 0, len(detail.Transactions))
	for _, t := range detail.Transactions {
		bv.Transactions = append(bv.Transactions, txView(t))
	}
	return &bv, nil
}

func (d *nodeDS) Transaction(hash string) (*TxView, error) {
	t, err := d.n.DB().GetTransaction(hash)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, nil
	}
	tv := txView(t)
	return &tv, nil
}

func (d *nodeDS) Address(addr string) (*AddressView, error) {
	av := &AddressView{Address: addr, Balance: "0"}
	acc, err := d.n.DB().GetAccount(addr)
	if err != nil {
		return nil, err
	}
	if acc != nil {
		av.Balance = acc.Balance
		av.Nonce = acc.Nonce
	}
	txs, err := d.n.DB().GetTransactionsByAddress(addr, 50)
	if err != nil {
		return nil, err
	}
	av.Transactions = make([]TxView, 0, len(txs))
	for _, t := range txs {
		av.Transactions = append(av.Transactions, txView(t))
	}
	return av, nil
}
