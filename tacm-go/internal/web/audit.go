package web

// M67 鏈上審計（公開端點 /api/audit）：任何節點/入口只要同步同一條鏈，
// 逐塊驗證 coinbase 不變式得到的結果就完全一致——直接回答「收益與獎勵池
// 數字必須代表鏈上真實、A+B 不得大於實際總產量」。
//
// 驗證規則與 cmd/tacctl audit 完全一致：
//  1. 每塊 coinbase 交易總額 == chaindb.BlockReward(height)（coinbase 是唯一增發源）
//  2. coinbase:pool 份額 == 12%（PoolShareBps=1200）
//  3. coinbase:miner / coinbase:reserve 總額 == 88%
//  4. 鏈結構：高度連續、時間戳不倒退、難度合法（創世塊豁免難度）
//
// 與 tacctl 的差異僅在資料來源：tacctl 走 HTTP 拉 /api/block/{h}，
// 本端點直接讀 DataSource（同節點鏈資料），避免自拉自身的網路往返。

import (
	"math"
	"net/http"
	"strconv"
	"strings"

	"tacm/internal/chaindb"
)

// auditEps 浮點容差（與 cmd/tacctl 一致）。
const auditEps = 1e-6

// isCoinbaseMemo 判定交易是否為 coinbase（memo 恰為 "coinbase" 或以 "coinbase:" 開頭）。
func isCoinbaseMemo(memo string) bool {
	return memo == "coinbase" || strings.HasPrefix(memo, "coinbase:")
}

// AuditView 公開審計結果（記憶存檔點：M67 核心結構）。
type AuditView struct {
	OK            bool     `json:"ok"`
	Network       string   `json:"network"`
	ChainID       string   `json:"chain_id"`
	TipHeight     int64    `json:"tip_height"`
	CheckedBlocks int      `json:"checked_blocks"`
	CoinbasePass  int      `json:"coinbase_pass"` // coinbase 總額 == BlockReward 的塊數
	CoinbaseFail  int      `json:"coinbase_fail"`
	PoolPass      int      `json:"pool_pass"` // pool 份額 == 12% 的塊數
	PoolFail      int      `json:"pool_fail"`
	SplitPass     int      `json:"split_pass"` // miner/reserve 總額 == 88% 的塊數
	SplitFail     int      `json:"split_fail"`
	StructureOK   bool     `json:"structure_ok"` // 高度連續/時間戳遞增/難度合法
	Consistent    bool     `json:"consistent"`   // 全部檢查通過（鏈上供應與獎勵一致）
	AuditedMint  string `json:"audited_mint_tacm"`  // 審計窗口內 coinbase 增發（僅窗口，勿與全鏈混淆）
	TotalMined   string `json:"total_mined_tacm"`   // 全鏈真實總產出（掃鏈 coinbase 總額，M68 起為鏈上事實）
	PoolShare    string `json:"on_chain_pool_share_tacm"` // coinbase:pool 累計挹注（獎勵池來源，鏈上事實）
	RewardPool   string `json:"reward_pool_tacm"`   // 獎勵池帳戶鏈上餘額（與 /explorer 同源）
	OnlineMiners int    `json:"online_miners"`      // 鏈上在線礦工（最近 60 塊瓜分視角）
	Issues       []string `json:"issues"`
}

// auditRecentBlocks 自鏈頂往下驗證最近 n 塊（含創世則跳過 coinbase 驗證）。
func auditRecentBlocks(ds DataSource, n int) AuditView {
	st := ds.Status()
	out := AuditView{
		Network: "mainnet",
		ChainID: "tacm-mainnet-1",
		Issues:  []string{},
	}
	if ds == nil {
		out.Issues = append(out.Issues, "資料源不可用")
		return out
	}
	tip := st.Height
	if tip < 0 {
		out.Issues = append(out.Issues, "鏈頂不可用")
		return out
	}
	out.TipHeight = tip
	out.StructureOK = true // 初始為真，任一步驟異常才置 false
	if n < 1 {
		n = 1
	}
	if n > 500 {
		n = 500
	}

	start := tip - int64(n) + 1
	if start < 0 {
		start = 0
	}
	out.CheckedBlocks = 0
	prevTs := int64(0)
	first := true
	var minted float64
	for h := start; h <= tip; h++ {
		bd, err := ds.BlockDetail(h)
		if err != nil {
			out.StructureOK = false
			out.Issues = append(out.Issues, "h="+strconv.FormatInt(h, 10)+" 讀取失敗: "+err.Error())
			continue
		}
		if bd == nil || bd.Block == nil {
			out.StructureOK = false
			out.Issues = append(out.Issues, "h="+strconv.FormatInt(h, 10)+" 區塊缺失（鏈不連續）")
			continue
		}
		out.CheckedBlocks++
		// 結構：高度連續由迴圈保證；時間戳不倒退；難度合法（創世豁免）。
		if first {
			prevTs = bd.Block.Ts
			first = false
		} else if bd.Block.Ts < prevTs {
			out.StructureOK = false
			out.Issues = append(out.Issues, "h="+strconv.FormatInt(h, 10)+" 時間戳倒退")
		} else {
			prevTs = bd.Block.Ts
		}
		if bd.Block.Height > 0 && bd.Block.Difficulty < 1 {
			out.StructureOK = false
			out.Issues = append(out.Issues, "h="+strconv.FormatInt(h, 10)+" 難度非法")
		}
		// coinbase 不變式（創世塊 h<=0 無增發，跳過）。
		if bd.Block.Height <= 0 {
			continue
		}
		reward := chaindb.BlockReward(bd.Block.Height)
		var coinbaseSum, poolAmt, minerSum float64
		for _, tx := range bd.Transactions {
			if tx == nil || !isCoinbaseMemo(tx.Memo) {
				continue
			}
			amt, err := strconv.ParseFloat(tx.Amount, 64)
			if err != nil {
				continue
			}
			coinbaseSum += amt
			if strings.HasPrefix(tx.Memo, "coinbase:pool") {
				poolAmt += amt
			} else {
				minerSum += amt
			}
		}
		minted += coinbaseSum
		expPool := reward * float64(chaindb.PoolShareBps) / 10000
		expRest := reward - expPool
		if math.Abs(coinbaseSum-reward) <= auditEps {
			out.CoinbasePass++
		} else {
			out.CoinbaseFail++
			out.Issues = append(out.Issues, "h="+strconv.FormatInt(h, 10)+" coinbase 總額 "+strconv.FormatFloat(coinbaseSum, 'f', 8, 64)+" != 出塊獎勵 "+strconv.FormatFloat(reward, 'f', 8, 64))
		}
		if math.Abs(poolAmt-expPool) <= auditEps {
			out.PoolPass++
		} else {
			out.PoolFail++
			out.Issues = append(out.Issues, "h="+strconv.FormatInt(h, 10)+" pool 份額異常")
		}
		if math.Abs(minerSum-expRest) <= auditEps {
			out.SplitPass++
		} else {
			out.SplitFail++
			out.Issues = append(out.Issues, "h="+strconv.FormatInt(h, 10)+" miner/reserve 份額異常")
		}
	}
	out.AuditedMint = chaindb.FormatFloat(minted)
	out.StructureOK = out.StructureOK && out.CheckedBlocks == int(tip-start+1)
	// 全鏈真實數據（M68 起改走鏈上掃描聚合，取代本地 chain_stats 快照）：
	// 總產出＝掃鏈 coinbase 總額；pool 挹注＝coinbase:pool 累計；
	// 獎勵池帳戶餘額＝與 /explorer 同源；在線礦工＝鏈上 hb 視角。任何節點同步同鏈結果一致。
	if agg, err := ds.OnChainCoinbase(); err == nil {
		out.TotalMined = chaindb.FormatFloat(agg.Total)
		out.PoolShare = chaindb.FormatFloat(agg.Pool)
	}
	cs := ds.ChainStats()
	out.RewardPool = cs.RewardPool
	out.OnlineMiners = cs.OnlineMiners
	out.Consistent = out.CoinbaseFail == 0 && out.PoolFail == 0 && out.SplitFail == 0 && out.StructureOK
	out.OK = out.Consistent
	return out
}

// handleAuditAPI GET /api/audit?blocks=N — 公開鏈上審計（M67）：
// 驗證最近 N 塊（預設 100，上限 500）coinbase 不變式＋回傳全鏈真實供應/獎勵池。
func (s *Server) handleAuditAPI(w http.ResponseWriter, r *http.Request) {
	n := 100
	if v := r.URL.Query().Get("blocks"); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 {
			n = p
		}
	}
	av := auditRecentBlocks(s.ds, n)
	av.Network = s.network
	if s.network == "testnet" {
		av.ChainID = "tacm-testnet-1"
	}
	writeJSON(w, av)
}
