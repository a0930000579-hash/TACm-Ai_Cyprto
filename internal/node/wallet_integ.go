package node

import (
	"tacm/internal/chaindb"
	"tacm/internal/wallet"
)

// Wallet 返回節點錢包服務（RPC/Web 層使用）。
func (n *Node) Wallet() *wallet.Service { return n.walletSvc }

// syncWallet 把已落庫區塊的交易同步進錢包帳本。
// 冪等：wallet.ApplyBlock 僅接受連續高度；重組場景由節點層處理（M13 標註限制）。
func (n *Node) syncWallet(height int64, txs []chaindb.Transaction) error {
	if n.walletSvc == nil {
		return nil
	}
	return n.walletSvc.ApplyBlock(height, txs)
}
