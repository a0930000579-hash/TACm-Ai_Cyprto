// Command web 在單進程內啟動 TAC 自主智能鏈節點（RPC + 出塊 + BFT）與 Web 區塊瀏覽。
package main

import (
	"context"
	"os"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tacm/internal/chaindb"
	"tacm/internal/config"
	"tacm/internal/crypto"
	"tacm/internal/node"
	"tacm/internal/web"
)

func main() {
	nodeID := flag.String("node-id", "node1", "節點 ID")
	dataDir := flag.String("data-dir", "", "數據目錄")
	blockTime := flag.Int("block-time", 1, "目標出塊間隔（秒）")
	difficulty := flag.Int("difficulty", 1, "基礎 PoW 難度")
	rpcPort := flag.Int("rpc-port", 8332, "節點 RPC 端口")
	webPort := flag.Int("web-port", 8080, "Web 服務端口")
	enableL2 := flag.Bool("l2", false, "啟用 Layer2 Optimistic Rollup（含後台批量/提交）")
	enableBridge := flag.Bool("bridge", false, "啟用跨鏈橋（Lock&Mint / Burn&Unlock）")
	bridgeGuardians := flag.String("bridge-guardians", "",
		"跨鏈守衛規格：addr:pubkey 逗號分隔（配置後啟用守衛網絡：提案經 P2P 多簽聚合）")
	flag.Parse()

	cfg := config.Default()
	cfg.RPCPort = *rpcPort
	cfg.BlockTime = *blockTime
	cfg.EnableL2 = *enableL2
	cfg.EnableBridge = *enableBridge
	if *dataDir != "" {
		cfg.DataDir = *dataDir
	} else if env := os.Getenv("DATA_DIR"); env != "" {
		// 容器/平台部署標準：DATA_DIR 環境變數優先於預設目錄。
		cfg.DataDir = env
	} else {
		cfg.DataDir = "./tac_data/" + *nodeID
	}

	n, err := node.New(cfg, *nodeID, *difficulty)
	if err != nil {
		log.Fatalf("創建節點失敗: %v", err)
	}

	// 跨鏈橋：單進程亦可啟用守衛網絡（多簽聚合）或本地橋。
	if *enableBridge {
		if *bridgeGuardians != "" {
			guardians, gerr := node.ParseGuardians(*bridgeGuardians)
			if gerr != nil {
				log.Fatalf("解析跨鏈守衛失敗: %v", gerr)
			}
			if aerr := n.AttachBridgeNetwork(guardians); aerr != nil {
				log.Fatalf("接入跨鏈守衛網絡失敗: %v", aerr)
			}
			log.Printf("[bridge] 跨鏈守衛網絡已啟用（守衛 %d 個）", len(guardians))
		} else if err := n.AttachBridge(); err != nil {
			log.Fatalf("啟用跨鏈橋失敗: %v", err)
		}
	}

	// 數據庫為空時生成創世區塊。
	cnt, err := n.DB().GetBlockCount()
	if err != nil {
		log.Fatalf("讀取區塊數失敗: %v", err)
	}
	if cnt == 0 {
		emptyRoot, err := crypto.MerkleRootStrings(nil)
		if err != nil {
			log.Fatalf("計算空 Merkle 根失敗: %v", err)
		}
		genesis := &chaindb.Block{
			Height: 0, Hash: strings.Repeat("0", 64),
			MerkleRoot: emptyRoot, Proposer: "genesis", Ts: time.Now().Unix(),
		}
		if err := n.DB().InsertBlock(genesis, nil); err != nil {
			log.Fatalf("寫入創世塊失敗: %v", err)
		}
		log.Println("[genesis] 創世區塊已生成")
	}

	// 節點 RPC。
	rpcHandler := node.NewRPCServer(n).Handler()
	rpcSrv := &http.Server{Addr: addr(*rpcPort), Handler: rpcHandler}
	go func() {
		log.Printf("[rpc] 節點 RPC: http://0.0.0.0:%d", *rpcPort)
		if err := rpcSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[rpc] 退出: %v", err)
		}
	}()

	// Web 區塊瀏覽。
	wserver, err := web.New(web.NewNodeDataSource(n))
	if err != nil {
		log.Fatalf("創建 Web 服務失敗: %v", err)
	}
	// 先啟動節點共識/出塊，再暴露 Web/RPC（避免「RPC 可達但尚未出塊」）。
	n.Start()
	log.Printf("[node] 節點 %s 已啟動，地址 %s", *nodeID, n.Address())

	webSrv := &http.Server{Addr: addr(*webPort), Handler: wserver.Handler()}
	go func() {
		log.Printf("[web] Web 區塊瀏覽: http://0.0.0.0:%d", *webPort)
		if err := webSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[web] 退出: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("[shutdown] 正在停止...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = rpcSrv.Shutdown(ctx)
	_ = webSrv.Shutdown(ctx)
	if err := n.Close(); err != nil {
		log.Printf("[shutdown] 關閉出錯: %v", err)
	}
}

func addr(port int) string { return ":" + strconv.Itoa(port) }
