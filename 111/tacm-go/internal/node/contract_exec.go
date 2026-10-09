package node

import (
	"fmt"
	"strings"

	"tacm/internal/chaindb"
)

// syncChainAccount 把某地址鏈上餘額/nonce 同步進合約世界（執行前）。
func (n *Node) syncChainAccount(tx0addr string) error {
	if tx0addr == "" {
		return nil
	}
	evm, err := tx0To0x(tx0addr)
	if err != nil {
		return err
	}
	bal := parseAmount(n.db.GetBalance(tx0addr))
	nonce := n.db.GetNonce(tx0addr)
	n.contracts.SyncAccount(evm, bal, uint64(nonce))
	return nil
}

// executeBlockContracts 在區塊插入後執行其中的合約交易（部署/調用）。
func (n *Node) executeBlockContracts(txs []chaindb.Transaction) error {
	for i := range txs {
		if err := n.applyContractTx(&txs[i]); err != nil {
			return err
		}
	}
	return nil
}

func (n *Node) applyContractTx(t *chaindb.Transaction) error {
	if !isContractMemo(t.Memo) {
		return nil
	}
	to0x, err := tx0To0x(t.ToAddr)
	if err != nil {
		return fmt.Errorf("node: 合約目標地址錯誤: %w", err)
	}
	from0x, err := tx0To0x(t.FromAddr)
	if err != nil {
		return err
	}
	value := parseAmount(t.Amount)

	if err := n.syncChainAccount(t.FromAddr); err != nil {
		return err
	}
	if err := n.syncChainAccount(t.ToAddr); err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(t.Memo, vmDeployPrefix):
		initCode, err := hexInput(t.Memo[len(vmDeployPrefix):])
		if err != nil {
			return fmt.Errorf("node: init 字節碼錯誤: %w", err)
		}
		dr, err := n.contracts.ApplyDeploy(to0x, from0x, initCode, value)
		if err != nil {
			return err
		}
		if !dr.OK {
			return fmt.Errorf("node: 合約部署失敗(tx %s): %s", t.TxHash, dr.Error)
		}
	case strings.HasPrefix(t.Memo, vmCallPrefix):
		calldata, err := hexInput(t.Memo[len(vmCallPrefix):])
		if err != nil {
			return fmt.Errorf("node: calldata 錯誤: %w", err)
		}
		cr, err := n.contracts.ApplyCall(to0x, from0x, calldata, value)
		if err != nil {
			return err
		}
		if !cr.OK {
			return fmt.Errorf("node: 合約調用失敗(tx %s): %s",
				t.TxHash, cr.RevertReason)
		}
	}
	return nil
}

// simulateContractTx 入池前只讀模擬；失敗返回錯誤以拒絕交易入池。
func (n *Node) simulateContractTx(t *chaindb.Transaction) error {
	if !isContractMemo(t.Memo) {
		return nil
	}
	to0x, err := tx0To0x(t.ToAddr)
	if err != nil {
		return err
	}
	from0x, err := tx0To0x(t.FromAddr)
	if err != nil {
		return err
	}
	value := parseAmount(t.Amount)
	if err := n.syncChainAccount(t.FromAddr); err != nil {
		return err
	}
	if err := n.syncChainAccount(t.ToAddr); err != nil {
		return err
	}

	switch {
	case strings.HasPrefix(t.Memo, vmDeployPrefix):
		initCode, err := hexInput(t.Memo[len(vmDeployPrefix):])
		if err != nil {
			return err
		}
		dr, err := n.contracts.SimulateDeploy(to0x, from0x, initCode, value)
		if err != nil {
			return err
		}
		if !dr.OK {
			return fmt.Errorf("合約構造失敗: %s", dr.Error)
		}
	case strings.HasPrefix(t.Memo, vmCallPrefix):
		calldata, err := hexInput(t.Memo[len(vmCallPrefix):])
		if err != nil {
			return err
		}
		cr, err := n.contracts.SimulateCall(to0x, from0x, calldata, value)
		if err != nil {
			return err
		}
		if !cr.OK {
			return fmt.Errorf("合約調用將失敗: %s", cr.RevertReason)
		}
	}
	return nil
}

// RebuildContracts 清空合約庫並按歷史順序重放全部合約交易，重建 code/storage。
// 在合約庫缺失或落後時調用；code/storage 為重建核心，歷史餘額以當前鏈狀態近似。
func (n *Node) RebuildContracts() error {
	if err := n.contracts.Reset(); err != nil {
		return err
	}
	txs, err := n.db.GetContractTransactions(vmDeployPrefix, vmCallPrefix)
	if err != nil {
		return err
	}
	for i := range txs {
		if err := n.syncChainAccount(txs[i].FromAddr); err != nil {
			return err
		}
		if err := n.syncChainAccount(txs[i].ToAddr); err != nil {
			return err
		}
		if err := n.applyContractTx(&txs[i]); err != nil {
			return err
		}
	}
	return nil
}
