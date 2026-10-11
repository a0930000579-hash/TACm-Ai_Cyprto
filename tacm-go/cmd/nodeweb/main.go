// nodeweb — TAC Ai Smart Chain 節點獨立品牌入口（M86）。
//
// 與主網 tacweb（landing/錢包/交易所/挖礦等用戶功能）完全分離：
// 本可執行檔只啟動「節點 + 節點品牌儀表板」，供節點運維者查看
// 鏈狀態 / 同步進度 / P2P peers / 驗證人集 / 出塊記錄 / 節點金鑰與獎勵。
//
// 用法範例（主網跟隨節點，獨立 UI 埠 8081）：
//
//	sudo ./nodeweb -web-port 8081 -p2p-url http://<本機IP>:8081 \
//	  -p2p-seed http://2.28.201.174:8080 -p2p-follower
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"tacm/internal/chaindb"
	"tacm/internal/config"
	"tacm/internal/crypto"
	"tacm/internal/node"
	"tacm/internal/nodeui"
	"tacm/internal/p2p"
)

func main() {
	nodeID := flag.String("node-id", "tac-node", "節點 ID")
	dataDir := flag.String("data-dir", "", "數據目錄（預設 ./tac_data/<node-id>）")
	blockTime := flag.Int("block-time", 1, "目標出塊間隔（秒）")
	difficulty := flag.Int("difficulty", 1, "基礎 PoW 難度")
	webPort := flag.Int("web-port", 8081, "節點 UI 服務端口")
	networkFlag := flag.String("network", "mainnet", "網路段：mainnet | testnet")
	p2pEnabled := flag.Bool("p2p", true, "啟用 P2P 聯網（主網節點必開）")
	p2pURL := flag.String("p2p-url", "", "本節點對外可達 HTTP 基址（如 http://1.2.3.4:8081）")
	p2pSeed := flag.String("p2p-seed", "http://2.28.201.174:8080", "初始引導節點 HTTP 基址（逗號分隔）")
	p2pFollower := flag.Bool("p2p-follower", true, "以跟隨節點模式啟動（不主動 PoW 出塊，經 P2P 同步區塊並參與 BFT 投票）")
	mainnetURL := flag.String("mainnet-url", "http://2.28.201.174:8080", "開放挖礦/主網入口 URL（節點 UI 的「開啟主網」按鈕）")
	flag.Parse()

	cfg := config.Default()
	cfg.RPCPort = *webPort
	cfg.BlockTime = *blockTime
	cfg.Network = strings.ToLower(*networkFlag)
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	} else if env := os.Getenv("DATA_DIR"); env != "" {
		cfg.DataDir = env
	} else if cfg.Network == "testnet" {
		cfg.DataDir = "./tac_node_data_testnet/" + *nodeID
	} else {
		cfg.DataDir = "./tac_node_data/" + *nodeID
	}
	if cfg.Network == "testnet" && strings.HasPrefix(cfg.ChainID, "tacm-mainnet") {
		cfg.ChainID = "tacm-testnet-1"
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校驗失敗: %v", err)
	}

	n, err := node.New(cfg, *nodeID, *difficulty)
	if err != nil {
		log.Fatalf("創建節點失敗: %v", err)
	}

	// 創世區塊（與主網 cmd/web 一致）：僅在鏈庫為空時生成；
	// 含 M73-B 社群基金 100,000 TACm 創世分配（節點派生金鑰）。
	cnt, cerr := n.DB().GetBlockCount()
	if cerr != nil {
		log.Fatalf("讀取鏈庫失敗: %v", cerr)
	}
	if cnt == 0 {
		emptyRoot, merr := crypto.MerkleRootStrings(nil)
		if merr != nil {
			log.Fatalf("計算空 Merkle 根失敗: %v", merr)
		}
		poolAddr, perr := n.CommunityPoolAddress()
		if perr != nil {
			log.Fatalf("計算社群基金地址失敗: %v", perr)
		}
		genesis := &chaindb.Block{
			Height: 0, Hash: strings.Repeat("0", 64),
			MerkleRoot: emptyRoot, Proposer: "genesis", Ts: time.Now().Unix(),
		}
		genesisTxs := []chaindb.Transaction{{
			TxHash: "genesis-community-pool", ToAddr: poolAddr,
			Amount: "100000", Fee: "0", Ts: time.Now().Unix(),
			Memo: "genesis:community_pool", Status: "confirmed",
		}}
		if err := n.DB().InsertBlock(genesis, genesisTxs); err != nil {
			log.Fatalf("寫入創世塊失敗: %v", err)
		}
		log.Printf("[genesis] 創世區塊已生成（社群基金 %s = 100000 TACm）", poolAddr)
	}

	// P2P 聯網：錨點節點提供 seed，其餘節點以 -p2p-seed 連入。
	var p2pNet *p2p.Network
	if *p2pEnabled {
		if *p2pURL == "" {
			log.Fatalf("啟用 -p2p 需提供 -p2p-url（本節點 P2P 對外基址，如 http://1.2.3.4:8081）")
		}
		var seeds []string
		for _, s0 := range strings.Split(*p2pSeed, ",") {
			s0 = strings.TrimSpace(s0)
			if s0 != "" {
				seeds = append(seeds, s0)
			}
		}
		netw, err := n.AttachP2P(*p2pURL, seeds)
		if err != nil {
			log.Fatalf("P2P 初始化失敗: %v", err)
		}
		p2pNet = netw
	}

	if *p2pFollower {
		n.StartAsFollower()
	} else {
		n.StartDistributed()
	}
	if p2pNet != nil {
		p2pNet.Start(context.Background())
	}

	// 節點品牌 UI（獨立資料夾 internal/nodeui、獨立 binary nodeweb）。
	ui := nodeui.New(n, *mainnetURL)
	addr := fmt.Sprintf("0.0.0.0:%d", *webPort)
	srv := &http.Server{Addr: addr, Handler: ui.Handler()}
	go func() {
		log.Printf("[nodeweb] 節點品牌 UI 已上線: http://0.0.0.0:%d", *webPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("節點 UI 服務失敗: %v", err)
		}
	}()

	// 優雅關閉。
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	if p2pNet != nil {
		p2pNet.Close()
	}
	_ = n.Close()
	log.Println("[nodeweb] 已關閉")
}
