package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"tacm/internal/chaindb"
)

func (n *Network) syncLoop(ctx context.Context) {
	defer n.wg.Done()
	ticker := time.NewTicker(n.cfg.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.syncOnce()
		}
	}
}

// bestPeer 返回在線且鏈頂最高的節點（值快照）。
func (n *Network) bestPeer() (Peer, bool) {
	var best Peer
	found := false
	for _, p := range n.onlinePeers() {
		if !found || p.Height > best.Height {
			best, found = p, true
		}
	}
	return best, found
}

func (n *Network) syncOnce() {
	best, ok := n.bestPeer()
	if !ok {
		return
	}
	self := n.host.Height()
	if best.Height <= self {
		return
	}

	// 比較共同高度 self 的區塊哈希，判斷是線性落後還是分叉。
	if self >= 0 {
		pb, _, perr := n.fetchPeerBlock(best, self)
		lb, _, lerr := n.host.BlockDetail(self)
		if perr != nil || pb == nil || lerr != nil || lb == nil {
			return
		}
		if pb.Hash != lb.Hash {
			n.doReorg(best, self)
			return
		}
	}

	// M86：線性追塊——批次拉取（一次 HTTP 拉 up to 500 塊），
	// 批次內某塊接入失敗（含臨時缺口）時降級為單塊重試，不再整輪中止。
	for h := self + 1; h <= best.Height; {
		batch, err := n.fetchPeerBlocks(best, h, best.Height)
		if err != nil {
			// 批次失敗：降級單塊（對缺口/暫態錯誤有更細的重試粒度）。
			b, txs, ferr := n.fetchPeerBlock(best, h)
			if ferr != nil || b == nil {
				return
			}
			if err := n.host.OnIncomingBlock(b, txs); err != nil {
				return
			}
			h++
			continue
		}
		if len(batch) == 0 {
			return
		}
		progress := false
		for _, bd := range batch {
			if err := n.host.OnIncomingBlock(bd.Block, bd.Txs); err != nil {
				// 缺口/驗證失敗：單塊再試一次；仍失敗則中止本輪（等下一 tick）。
				sb, stxs, serr := n.fetchPeerBlock(best, bd.Block.Height)
				if serr != nil || sb == nil {
					return
				}
				if err := n.host.OnIncomingBlock(sb, stxs); err != nil {
					return
				}
			}
			h = bd.Block.Height + 1
			progress = true
		}
		if !progress {
			return
		}
	}
}

// fetchPeerBlocks M86：向 best 節點批量拉取 [from, best.Height] 區塊（上限 500/批）。
func (n *Network) fetchPeerBlocks(p Peer, from, to int64) ([]blockData, error) {
	if n.ctx == nil {
		return nil, fmt.Errorf("p2p: not started")
	}
	url := p.BaseURL + "/p2p/blocks?from=" + strconv.FormatInt(from, 10) +
		"&to=" + strconv.FormatInt(to, 10)
	req, err := http.NewRequestWithContext(n.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch blocks from %d status %d", from, resp.StatusCode)
	}
	var d struct {
		Blocks []blockData `json:"blocks"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, err
	}
	return d.Blocks, nil
}

// doReorg 回退找到共同分叉點，拉取對方分叉後全部區塊並交節點重組。
func (n *Network) doReorg(best Peer, from int64) {
	fork := int64(-1)
	for h := from; h >= 0; h-- {
		pb, _, perr := n.fetchPeerBlock(best, h)
		lb, _, lerr := n.host.BlockDetail(h)
		if perr != nil || pb == nil || lerr != nil || lb == nil {
			continue
		}
		if pb.Hash == lb.Hash {
			fork = h
			break
		}
	}
	if fork < 0 {
		return // 創世都不一致，拒絕重組
	}

	blocks := make([]chaindb.Block, 0, best.Height-fork)
	allTxs := make([][]chaindb.Transaction, 0, best.Height-fork)
	for h := fork + 1; h <= best.Height; h++ {
		b, txs, err := n.fetchPeerBlock(best, h)
		if err != nil || b == nil {
			return
		}
		blocks = append(blocks, *b)
		allTxs = append(allTxs, txs)
	}
	_ = n.host.ReorgChain(fork, blocks, allTxs)
}

func (n *Network) fetchPeerBlock(p Peer, h int64) (*chaindb.Block, []chaindb.Transaction, error) {
	if n.ctx == nil {
		return nil, nil, fmt.Errorf("p2p: not started")
	}
	url := p.BaseURL + "/p2p/block/" + strconv.FormatInt(h, 10)
	req, err := http.NewRequestWithContext(n.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetch block %d status %d", h, resp.StatusCode)
	}
	var d blockData
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, nil, err
	}
	return d.Block, d.Txs, nil
}
