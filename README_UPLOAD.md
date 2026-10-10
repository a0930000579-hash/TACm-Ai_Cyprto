# TACm-Go 壓縮檔上傳與 Render 部署說明（M70）

本壓縮檔解壓後會產生一個 **tacm-go 資料夾**（裡面是整套 Go 區塊鏈系統的完整程式碼）。

## 本版重點（M70）——節點獎勵上鏈＋全鏈數據歸零重來

- **出塊獎勵三分發（鏈上 coinbase 恆等式 9%＋16%＋75%＝100%）**：
  - 節點 **9%**：每塊自動歸「出塊節點」地址（memo `coinbase:node`）——節點獎勵歸節點；
  - 池 **16%**：每塊自動挹注獎勵池（memo `coinbase:pool`，交易所資金用途）；
  - 礦工 **75%**：按鏈上在線礦工算力占比瓜分（memo `coinbase:miner`）；無在線礦工時 75% 併入獎勵池（`coinbase:reserve`）。
- **節點獎勵與主網同一規則、全網一致**：任何入口（錨點／節點／瀏覽器）看到同一條鏈、同一個獎勵池、同一個總產出、同一個在線礦工數；總產出＝高度×10，獎勵池＝高度×9.1（無礦工時）或按實際瓜分。
- **審計新增節點份額**：`/api/audit` 新增 `node_pass/node_fail` 與 `on_chain_node_share_tacm`（= 高度×0.9）。
- **全鏈數據歸零重來**：本版起整條鏈重新創世運行。部署後**錨點必須清空舊資料**（見下方部署步驟），Render 重啟即重來。
- **前端中英文同步**：挖礦機制說明改 4 行（節點 9%／池 16%／礦工 75%），/network 審計行、/exchange 文案同步更新；英文為主、中文可切換。

### M70 錨點部署步驟（重要：先停服務→清資料→解壓→build→重啟）

一次一條指令：

```bash
sudo systemctl stop tacnode
rm -rf /var/tac/data
curl -L -o /tmp/tacm-go-m70.zip '<M70 aka 連結>'
cd /root && sudo unzip -o /tmp/tacm-go-m70.zip
cd /root/tacm-go && GOTOOLCHAIN=local go build -o tacweb ./cmd/web
sudo systemctl restart tacnode
curl -s http://2.28.201.174:8080/status
curl -s http://2.28.201.174:8080/api/audit
```

驗收：`/api/audit` 回 `"consistent":true`、`"coinbase_pass":100`、`"node_pass":100`、
`"on_chain_node_share_tacm"`＝高度×0.9、`"on_chain_pool_share_tacm"`＝高度×1.6。

Render 端：重新部署（unzip 後 `go build -o tacweb ./cmd/web`，Start Command 不變），
/network 與 /mining 帶 query 開啟即為新版本。

## 上版重點（M58）

- **白皮書/黃皮書全面「自主 AI 智能鏈」化**：移除全部外部鏈與品牌名稱——
  幣安/Binance、BSC、比特幣/Bitcoin、以太坊/Ethereum、MetaMask、EVM、USDT、Render 等，
  改為自主 TAC 表述（專業級交易所、標準 JSON-RPC 兼容層、TAC ↔ 異構鏈跨鏈橋、TAC VM、
  USD 穩定幣等）。同步更新：whitepaper_zh.html、whitepaper_en.html、yellowpaper_en.html、
  YELLOWPAPER.md、wp_zh.pdf、wp_en.pdf。
- 重建 tacweb 並冒煙驗證：中/英/黃皮書結構完整（十三、開發者生態與跨鏈／Developer Ecosystem／TAC VM），
  外部名稱殘留 **0**，節點正常出塊。
- **完整部署教學已直接提供於對話**（Oracle VPS 錨點→Render follower→防休眠→驗收）；
  zip 內另附 `DEPLOY_STEP_BY_STEP.md` 完整操作單。

## 上版重點（M57）

- **新增 `DEPLOY_STEP_BY_STEP.md`（完整逐步部署操作單）**：從 0 到上線——
  階段 A Oracle Cloud 免費 VPS 申請與實例建立 → 階段 B VPS 一鍵部署錨點（含 Oracle 第二層防火牆）→
  階段 C Render 改跟隨節點（完整 Build/Start Command）→ 階段 D cron 防休眠 → 階段 E 驗收與監控 ＋
  疑難排解表。全程 0 元。

## 上版重點（M56）

- **全系統盤點與清理（無 Python 殘留）**：
  1. 刪除廢棄檔案：`.fix_pools.py`（Python 殘留）、舊二進位 `web`（與 `tacweb` 重複）、全部 `.smoke*` 測試暫存。
  2. `DEPLOY.md`（M26 舊版）移至 `_legacy/DEPLOY_M26.md`（保留不刪）；97 個歷史 zip 移至專案根 `_archive/`（僅保留最新 `tacm-go-m55.zip`）。
  3. `gofmt -w` 統一全碼庫格式（Go 慣用）。
- **整套系統實跑驗證（先測後交）**：
  1. `go test ./...` 全量全綠（含 node 102s 大套件）、`go vet ./...` 通過。
  2. 單節點冒煙：RPC（/health /status /api/chain/stats /api/guest/address）全 200；Web 全部路由（/ /wallet /exchange /dashboard /mining /community /governance /defi /c2c /tokens /dex /auth/login /auth/register）全 200；白皮書中/英 302 正確；`eth_blockNumber` 回正確高度；訪客地址產生正常；出塊持續（h1=29→h2=32）。
  3. **3 節點 P2P 最終確認**：錨點出塊 14，兩個 follower 同步至 13；節點互相發現（B↔C 皆見彼此且 `online:true`）。
- **MAINNET.md 新增 3.1「Render 免費站改為跟隨節點＋公開入口」**：Render 控制台完整 Build/Start Command（`-p2p -p2p-url https://... -p2p-seed http://錨點:8080 -p2p-follower`）＋防休眠 cron 建議＋驗證方式。

## 上版重點（M55）

- **主網上線包（完全自主主網，不依賴 BSC/第三方）**：
  1. **tacweb 新增 P2P CLI**：`-p2p`（啟用聯網）、`-p2p-url`（本節點對外 HTTP 基址＝web 端口）、`-p2p-seed`（引導節點，逗號分隔）、`-p2p-follower`（跟隨節點：不主動 PoW 出塊，經 P2P 同步並參與 BFT）——多節點/主網模式現可經 CLI 直接組網。
  2. **`MAINNET.md`（新，主網上線手冊）**：主網參數、錨點架設（一鍵＋手動）、別人連入三種方式、手機節點（Termux）教學、監控、上線 Checklist、常見問題。
  3. **`scripts/deploy_mainnet.sh`（新，一鍵部署）**：build→systemd 託管→防火牆，支援 `--follower`／`--seed`。
  4. **`scripts/monitor.sh`（新，監控）**：健康/高度/全鏈統計/P2P peers。
  5. **M55 實測證據（本機 3 節點真進程組網）**：錨點 PoW 出塊 A=12，跟隨節點 P2P 同步 B=11，peer `online:true`——主網模式實跑驗證通過。
- 已知坑（P2P seed）：`-p2p-seed`/`-p2p-url` 填 **web 端口 URL**（http://IP:8080，client 自動加 /p2p/），非獨立端口。

## 上版重點（M54）

- **白皮書 4.0（中英同步更新，Web /whitepaper 已上線）**：
  1. 路線圖「已完成階段」擴充至第八階段（P2P/BFT、VM/Token Studio/DEX/流動性挖礦、L2/eth_*/BSC 橋/SDK），未來規劃僅保留 ZK 與主網激勵階段——與實際進度一致。
  2. **新增第十三章「開發者生態與跨鏈」**（中英）：官方 Go/JS SDK 與跨語言簽章互通、eth_\* 兼容層（MetaMask 直連）、BSC 真實跨鏈橋、供應治理（上限 52,003,300＋TiUSD 0.330 最低流通）。
  3. 修正中/英檔尾部殘缺（footer/免責聲明補齊），HTML 結構解析驗證通過。
- **`DEPLOY_OPS.md`（新，部署與運維手冊）**：系統架構、本機啟動＋systemd、Render 部署（含已知坑）、多節點組網（P2P seed）、BSC 橋實裝、備份/恢復、升級流程、監控端點＋常見故障排查表、安全注意。
- **M54 驗證**：白皮書兩檔 HTML 結構 0 錯誤；tacweb 重建；真節點冒煙 `/whitepaper`、`/whitepaper-en` 皆含「開發者生態」章節與 Version 4.0。

## 上版重點（M53）

- **P3 開發者生態（下）：JS SDK `sdk/tacjs`（新）**——瀏覽器/Node 通用官方 SDK：
  1. `generateKey/keyFromPrivateKeyHex`（secp256k1）、`addressFromPubkey`（Hash160＋Base58Check，tx0 前綴，與 Go `PubKeyToAddress` 同構）、`txSighash`（白名單 `from/to/amount/fee/memo/ts/nonce/token`＋Canonical 排序 JSON＋DoubleSHA256）、`signTransaction`（SHA256→RFC6979 ECDSA→**DER hex**，與 Go `SignTransaction` 同構）、`Client{status/account/balance/transaction/submit/transfer}`（fetch 一步轉帳：查 nonce→簽署→提交）。
  2. **跨語言互操作已驗證**：固定私鑰向量 → Go 端 `crypto.VerifyTransactionSignature` 認證通過（`sdk/tacclient/js_interop_test.go`）——**前端 tacjs 簽章，Go 節點可直接驗證入池**。
  3. 依賴 `@noble/secp256k1`＋`@noble/hashes`（純 JS，已打包 node_modules，開箱即用）；`npm test` 5 項單測全綠（含 stub 節點 Client e2e）。
- **M53 測試**：JS 5/5（node --test）＋Go 全量 `go test ./...` 0 失敗（含新 js_interop 測試）＋tacjs 冒煙（DER 簽章 142 hex 正常）。

## 上版重點（M52）

- **P3 開發者生態（上）**：
  1. **官方 Go SDK `sdk/tacclient`**（新）——第三方 dApp 開發者入口：`GenerateKey/KeyFromPrivateKeyHex`（secp256k1＋tx0 地址）、`Client{Status/LatestHeight/Account/Balance/Nonce/Block/Transaction/Mempool}`（節點 RPC 查詢）、`Transfer(key,to,amount,fee,memo)`（查 nonce→ECDSA 簽署→提交，**一步閉環**）、`Submit`（自組交易含合約呼叫）、`example/main.go`（可跑範例，`go run ./sdk/tacclient/example -node <URL>`）。
  2. **區塊瀏覽器交易詳情深化**——`/tx/{hash}` 頁新增：交易序號、**合約交易解碼**（memo `vm:deploy:<gas>:<hex>`／`vm:call:<gas>:<hex>`／舊格式 → 顯示 kind/gas/calldata）、簽名驗證資訊（公鑰＋簽名）；`TxView` 增 `TxIndex/Signature/Pubkey/Contract *ContractView`（`parseContractMemo` 與節點 `splitContractMemo` 同格式，串連一致）。
  3. **`DEVELOPER.md`**（新）——SDK 用法、節點 RPC 端點全表、eth_\* 兼容、BSC 跨鏈、安全注意。
- **M52 測試**：SDK 單測 3 項（金鑰 hex 往返、Client 查詢 stub、Transfer 簽署提交）＋**真節點 e2e `TestTransferEndToEnd`**（內嵌節點＋創世分配→SDK 轉帳 5→出塊後 bob=5／alice=994.9→`GET /tx` 可查）＋ web `TestParseContractMemo` 6 案例；`go test -race ./sdk/...`＋全量 `go test ./...` **全部套件 0 失敗**；tacweb 重建並真節點冒煙（`/status` 出塊正常）。
- **不亂改**：交易欄位白名單（TxSighash）未動，SDK 簽署與節點驗證同格式；既有 tx/block/address 頁面欄位保留，僅新增。

## 上版重點（M51）

- **P2 效能優化：ChainDB 讀快取（熱點讀路徑）**——節點高頻讀（eth_getBalance／eth_getTransactionCount／錢包/瀏覽器輪詢）不再每請求一次 SQLite，改為**記憶體快取＋寫入後整組失效**：
  - `internal/chaindb/cache.go`（新）：`ReadCache`（`sync.RWMutex`＋balance/nonce/tx/account 四張 map）；`getTx/getAccount` 回**副本**防外部修改 aliasing；`clear()` 全清。
  - `read.go`：`GetTransaction/GetAccount/GetBalance/GetNonce` **miss 才查 SQL**，命中直接回快取（"0" 餘額也快取）；`chaindb.go` Open 初始化 cache。
  - `write.go`：`InsertBlock/TruncateFromHeight/RebuildAccounts/SetAccountPubkey` **Commit/Exec 成功後 clear**——任何鏈寫入後快取整組失效，保證與 SQLite 強一致（出塊 1 秒週期內讀 QPS 由 N×SQL 降為 1×SQL＋N×記憶體）。
  - **不亂改**：p2p `flood` 廣播已是逐 peer goroutine 並行（P2-B 已達成無需改）；浮點餘額累加屬既有設計（M49 已確認不動）。
  - **M51 測試**：新增 `cache_test.go` 3 項（`TestReadCacheConsistency`：寫入後失效＋交易回副本防污染；`TestReadCacheInvalidateOnNextBlock`：連續出塊讀新值；`TestReadCacheConcurrent`：8 goroutine×100 讀並發）；`go test -race ./internal/chaindb/` **PASS**；全量 `go test ./...` **22 套件 0 失敗**；本機冒煙：`eth_blockNumber 0x3→0xd` 持續出塊、`eth_getBalance/eth_getTransactionCount` 快取讀值正確 ✓。

## 上版重點（M50）

- **P1：BSC 真實跨鏈橋（TAC ↔ BNB Smart Chain 雙向中繼）**——不再只是橋的「記錄層」，而是**能對 BSC 鏈上真實簽署交易並自動中繼**的完整工程層：
  - `internal/crypto/eth_tx.go`：EIP-155 交易簽署（`SignEthRawTx`：Keccak256 payload → DER 解析 → 0..3 recid 恢復比對回填 V/R/S）、RLP 編碼、**ABI selector／ABI calldata 編解碼**（uint256/address/bytes32/bool/string/bytes，含動態 offset）、`HexEncode/HexDecode`、`EthKey/EthKeyFromHex`、`EthAddressBytes/EthAddressHex`。
  - `internal/bridge/bsc.go`：純 HTTP **BSCClient**（eth_chainId/blockNumber/nonce/gasPrice/estimateGas/sendRawTransaction/receipt/getLogs）、**BridgeContract**（MintBurn 代幣 mint/burn＋LockProxy deposit(address,uint256)/withdraw 的 calldata 組裝、Nonce→GasPrice→EstimateGas→簽署→送鏈全自動）、`ParseDepositLogs`（解析 `Deposit(address,address,address,uint256,uint256)` 事件，含目標地址）、`EthSigner`（hex 私鑰→keccak 0x 地址→EIP-155 簽署）。
  - `internal/bridge/relayer.go`：**雙向自動中繼**（Goroutine＋ticker）——**TAC→BSC**：掃橋上 `locked` 的 tacm→bsc 交易 → 以 Relayer 私鑰簽 `mint(to, amountWei)` 上鏈 → `MintOnTarget` 推進 minted；**BSC→TAC**：掃 LockProxy `Deposit` 事件（進度存橋 store）→ 自動建立 TAC 側解鎖記錄（pending→burning，不重複建單）→ 守衛網絡完成鏈上解鎖。錯誤逐筆記錄、不阻塞其他交易、RPC 不可達優雅降級。
  - 節點接入：`-bridge -bsc-relay -bsc-rpc -bsc-pk -bsc-chain-id -bsc-token -bsc-lock-proxy -bsc-poll-ms` 八個新 flag；`GET /bridge/bsc/status` 查中繼狀態（enabled/rpc/chain_id/token/lock_proxy/signer_addr/last_scanned）。
  - **M50 測試**：crypto 5 項（EIP-155 簽署 round-trip＋恢復比對、ERC20 transfer selector=0xa9059cbb、ABI 編解碼、address 轉換）；bridge 5 項（BSCClient、mint calldata、Deposit 事件解析、EthSigner、**Relayer 雙向 e2e：TAC→BSC 自動 mint 上鏈＋BSC→TAC 自動建單且不重複**）；node e2e 2 項（`TestBSCRelayStatusEndpoint`、`TestBSCRelayMintEndToEnd`：25 TACm 扣 0.1% 費 → mint 24.975e18 wei 精確上鏈）。`go test ./...` **全部套件 0 失敗**；本機冒煙：`-bsc-relay` 啟動 → `/bridge/bsc/status` 回 enabled=true＋signer_addr＋RPC 不可達時優雅降級不出錯 ✓。
  - **BSC 部署教學已寫入 `BSC_DEPLOY.md`**（Solidity 合約摘要＋Remix/Hardhat 部署＋本機/Render 啟動命令＋驗收指令＋安全備註）。

## 上版重點（M49）

- **P1 eth\_\* JSON-RPC 兼容層**——節點新增 `POST /eth` 標準 JSON-RPC 端點，讓 **MetaMask／以太坊生態工具可直連 TAC 自主鏈**（EIP-155 交易、secp256k1 恢復、wei↔TACm 換算）：
  - 已支援方法：`web3_clientVersion`、`net_version`、`eth_chainId`（0x539）、`eth_blockNumber`、`eth_gasPrice`、`eth_getBalance`（wei）、`eth_getTransactionCount`（nonce）、`eth_getCode`、`eth_call`、`eth_getBlockByNumber`、`eth_sendRawTransaction`。
  - `eth_sendRawTransaction`：RLP 解碼類型 0 交易 → EIP-155 簽名恢復（RecoverCompact）→ 映射 TAC 鏈上交易（`from=Hash160(pub)`、memo=`vm:call:<gas>:<data>`、**wei→TACm 十進位精確換算**）→ 入池／出塊／錢包同步完整閉環；壞 nonce／壞簽名／gas 超限／合約建立皆拒絕。
  - 地址互通：`tx0…` ↔ `0x+Hash160`（20 字節）雙向轉換；`eth_getBalance` 以 big.Rat 精確把鏈上 TACm 小數餘額轉 wei。
- **M49 測試**：crypto 單測 3 項（RLP round-trip／EIP-155 恢復／VerifyEthTx）＋ node e2e 3 項（`TestEthRPCBasics`／`TestEthSendRawTransactionAndCall`：5e18 wei 轉帳入塊＋餘額 ≥5e18＋nonce=0x1＋跳號拒絕＋eth_call totalSupply=777000／`TestEthGetBlockByNumber`）。`go test ./...` **20 套件 0 失敗**；本機冒煙（新節點）：`eth_chainId=0x539`、`eth_blockNumber=0x10`、`eth_getBalance=0x0`、`eth_getBlockByNumber` 結構完整 ✓。
- **M49 修復（測試抓到 4 個真 bug）**：①RLP 長 list 的長度頭與內容長度混用（slice 越界 panic）→ `longLen` 回 (頭長, 內容長度)；②eth 金額直接傳 wei 被錢包當 TACm 放大 1e18（餘額不足）→ `weiToTacmStr`（÷1e18、去尾零）；③鏈上餘額「15.0」無法被 big.Int 解析（eth_getBalance 誤回 0）→ big.Rat 精確轉 wei；④測試 helper 對 32-byte return 負補零 panic → 取低 20 字節。

## 上版重點（M48）

- **P0 安全硬化：RPC gas_limit 資源治理**——合約交易（部署/呼叫）支援 **自訂 gas 上限並上鏈**：`POST /contract/deploy` 與 `POST /contract/call` body 新增可選 `gas_limit`（1..10,000,000，預設節點 10M；>10M 拒絕）。gas 段寫入交易 memo（新格式 `vm:deploy:<gas>:<hex>`／`vm:call:<gas>:<hex>`，舊格式向後相容），**出塊執行與入池前模擬一致使用該 gas**——過小 gas 的壞合約在**入池前模擬即被拒絕（400）**，防單筆交易耗盡節點資源。
- **VM gas API 擴充**：`ContractManager` 新增 `ApplyDeployGas/ApplyCallGas/SimulateDeployGas/SimulateCallGas`（帶 gas 執行，均含快照回滾）；原方法委託預設 gas，**既有呼叫全部相容**。
- **M48 測試**：新增 VM 單測 `TestApplyCallGasLimit`（gas=10 過小 → OOG＋回滾；預設 gas 正常成功）＋ node e2e `TestContractStudioGasLimit`（gas_limit=5M 入塊、=100 被拒 400、=99M 被拒 400）；`go test ./...` **全部套件 0 失敗**；本機冒煙（新節點重跑）：gas_limit=5M 發幣出塊（code_size 335）、=100/99M 皆 400 拒絕 ✓。

## 上版重點（M47）

- **P0 安全硬化：EVM 狀態原子性修復**——`ContractManager.ApplyCall/ApplyDeploy` 在執行前對 WorldState 做快照，**OOG（gas 耗盡）/REVERT/執行失敗時一律回滾**，已寫入的合約 storage 不再殘留（對比模擬路徑 SimulateCall/SimulateDeploy 原有快照，Apply 路徑此前缺失）。
- **安全防線驗證（新增 3 個 VM 安全測試）**：
  - `TestInfiniteLoopStoppedByGas`：無限迴圈合約（JUMPDEST→PUSH→JUMP）在 gas 上限內被強制中止，**防合約 DoS 卡死節點**（VM 既有 GasCostTable 計價＋10M 上限＋64MB 記憶體＋code 24576＋呼叫深度 1024 防線全部生效）。
  - `TestApplyCallRollbackOnOOG`：呼叫先 SSTORE 再迴圈 OOG，驗證 **storage 已回滾、槽位乾淨**（修復前殘留 slot0=1）。
  - `TestApplyDeployRollbackOnOOG`：部署構造器先寫 storage 再 OOG，驗證 **合約不註冊＋storage 乾淨**（修復前殘留）。
- **M47 測試**：`internal/vm/manager_test.go` 新增 3 個安全測試；`go test ./...` **22 套件 0 失敗**；本機冒煙閉環（修復後重跑）：發幣 M47Test（supply=1e6）→ 出塊部署（code_size 335）→ transfer 250 入塊 → **alice 餘額精確 = 250（0xfa）**，成功路徑不受快照修復影響。

## 上版重點（M46）

- **DEX 流動性挖礦（Farm）**：`/dex` 頁面新增「流動性挖礦」區塊——**做市者質押 LP 份額賺取獎勵代幣**。鏈上農場（每池一個）：建農場（指定獎勵代幣與每塊獎勵）→ 注資金庫（官方帳戶鏈上轉帳至派生金庫地址）→ 質押/解除質押（已質押份額不可退出流動性）→ 出塊自動累積獎勵（標準 farm 演算法 accPerShare 1e18 定點）→ 領取（金庫鏈上轉帳回帳戶）。全程中英雙語、幣安深色 App 化、8 秒即時輪詢。
- **Farm RPC**：
  - `POST /dex/stake/create {pool,reward_token,reward_per_block}`：建農場（每池一個，重複拒絕；獎勵代幣須已發行）。
  - `POST /dex/stake/fund {pool,amount}`：注資獎勵代幣至金庫（鏈上轉帳）。
  - `POST /dex/stake {pool,shares}`／`POST /dex/stake/unstake {pool,shares}`：質押/解除 LP 份額。
  - `POST /dex/stake/claim {pool}`：領取未領獎勵（金庫→帳戶鏈上結算）。
  - `GET /dex/stake/pools?owner=`：農場列表＋我的份額/待領。
  - `GET /dex/stake/pending?pool=&owner=`：未領獎勵。
- **關鍵修復（M46）**：①**出塊×鏈上等待死鎖**（AddLiquidity 持引擎鎖等入塊 × 出塊 goroutine 等引擎鎖）——農場 tick 改 `StakeTickTry`（TryLock 被持則跳過本塊、下塊再累加）；②**pending 定點尺度**（acc 為 1e18 定點，pending 少除以 1e18 造成天文數字）；③**金庫地址解析**（引擎層 `tac-dex-stake:<pool>` 標記：收款方解析為派生金庫地址、簽名方保留標記由 signerForPool 金庫分支選簽名者）；④**ERC-20 transfer 餘額檢查**（VM d3 位元組碼重寫：`bal<amt → REVERT(0,0)`，修復 from 餘額不足時 uint256 下溢——冒煙曾見 admin 餘額 `2^256−…`，修後精確為 `900005899999`）。
- **M46 測試**：`internal/dex/stake_test.go` 3 個農場單測（建/注資、收益+領取、鎖定+解除）；`internal/node/dex_stake_e2e_test.go` 2 個 RPC e2e（建農場/質押/待領、領取/鎖定）——`go test ./...` **22 套件 0 失敗**；本機冒煙閉環（重跑於餘額檢查修復後）：發幣 TKA/TKB（supply=1e12）→ 建池 → 注入（LP=70710）→ 建農場（rpb=1e6 raw）→ 注資 1e11 → 質押 70710 → 出塊後 pending=5999999 → 領取入帳 5999999 → **admin TKA 餘額=900005899999（=1e12−1e5−1e11+5999999，精確吻合、無下溢）** ✓。

## 上版重點（M45）

- **鏈上 DEX（AMM 恆定乘積去中心化交易所）**：新增 `/dex` 頁面（頂部導覽「DEX」＋「☰ Tools」選單入口）——**發幣→建池→流動性→即時兌換全閉環、免私鑰**。頁面具備：流動性池列表（儲備/LP/即時價格 price0/price1）、建立流動性池（選擇兩個已發行代幣）、注入/退出流動性（LP 份額）、即時兌換（輸入即報價，恆定乘積自動滑價、0.3% 手續費）。全程中英雙語、幣安深色 App 化、8 秒即時輪詢。
- **DEX RPC（節點官方金鑰代簽，鏈上真 ERC-20 transfer 結算）**：
  - `GET /dex/pools`：池列表 `{id,token0,token1,pool_addr,reserve0,reserve1,lp_total,price0,price1}`。
  - `POST /dex/create {token0,token1}`：建池（重複交易對/同幣/不存在代幣拒絕）。
  - `POST /dex/liquidity/add {pool,amount0,amount1}`：注入流動性 → LP 份額（首次 LP=√(a0·a1)）。
  - `POST /dex/liquidity/remove {pool,shares}`：退出流動性（按比例退回兩幣）。
  - `POST /dex/swap {pool,token,amount}`：即時兌換（恆定乘積 x·y=k，手續費 30 bps）。
  - `GET /dex/quote?pool=&token=&amount=`：只讀報價。
  - 池帳戶＝HMAC 派生地址（`DeriveKey("tac-dex-pool-<token0>:<token1>")`），轉帳由節點代簽上鏈、等待入塊後讀餘額結算——**鏈上真實轉帳，非模擬餘額**。
- **關鍵修復（M45）**：①官方代簽連續交易 **pending nonce**（原同 from 連續兩筆 mempool 交易 nonce 相同、第二筆覆蓋第一筆，導致兩次發幣同一合約地址）——node 加 `txNoncePending`，入塊後清空；②**DEX mutex 重入死鎖**（Swap 持鎖時查池列表再取鎖）——改 node 自持 poolID→token 對映無鎖查詢；③nil map panic；④deploy 手寫 nonce 未含 pending。
- **M45 測試**：`internal/dex/dex_test.go` 4 個 AMM 數學測試（建池/恆定乘積/報價/退出/錯誤分支）；`internal/node/dex_e2e_test.go` 3 個 RPC e2e（建池/注入/兌換/退出/拒絕分支）——`go test ./...` **21 套件 0 失敗**；本機冒煙閉環：發幣 TKA/TKB → 建池 p1 → 注入 100000/50000（LP=70710）→ 兌換 1000 TKA→494 TKB（儲備 101000/49506、price0=0.49015842）✓。
- **Web 代幣工作室（Token Studio）**：新增 `/tokens` 頁面（頂部導覽「代幣」＋「☰ Tools」選單入口）——**鏈上標準代幣發行閉環、免私鑰**。頁面具備：發行代幣表單（名稱/代號/總供應量）、已發行代幣列表（supply/name/symbol）、代幣詳情（總供應量/名稱/代號/我的餘額/合約地址）、一鍵代簽轉帳（輸入 tx0 地址＋數量）。全程中英雙語、幣安深色 App 化、8 秒即時輪詢。
- **RPC 發幣/代幣端點（節點官方金鑰代簽，前端無需私鑰）**：
  - `POST /contract/deploy`：發行標準代幣 `{name,symbol,supply}` → 回 `{tx_hash, contract_address, evm_address, creator, supply, name, symbol}`（memo=`vm:deploy:<hex>`，節點金鑰代簽入池）。
  - `GET /contract/call/{address}?from=&calldata=`：只讀模擬呼叫（`vm.SimulateCall`）。
  - `POST /contract/call`：節點代簽鏈上呼叫 `{to,calldata}`。
  - `GET /contract/erc20/{address}?holder=`：**便利查詢**（免組 calldata）→ 回 `{supply,name,symbol,balance}`。
  - `POST /contract/erc20/transfer`：**便利轉帳** `{contract,to,amount}` → 伺服器組 transfer calldata（地址自動轉 0x）＋節點代簽上鏈。
  - `GET /contract/list`：已擴充回傳 `supply/name/symbol`（讀合約 slot0/1/2）。
- **VM 升級（M44）**：`internal/vm/erc20.go` 重寫——`Erc20Runtime()` 加 `name`/`symbol` 分支（runtime 318 bytes）、新增 `Erc20InitWithMeta(totalSupply,name,symbol)`（slot0=supply、slot1=name、slot2=symbol、全部結給部署者）、`Erc20NameCalldata/Erc20SymbolCalldata`；修 1 個 VM bug（init CODECOPY src offset 多算 1 byte）。
- **M44 測試**：`internal/vm/erc20_test.go` 新增 2 個 meta 部署閉環測試（8 測試全過）；`internal/node/contract_studio_e2e_test.go` 新增 3 個 RPC 層 e2e（部署＋查詢、鏈上 transfer、便利查詢/轉帳）——`go test ./...` **20 套件 0 失敗**。
- 承 M43：鏈上標準代幣（ERC-20 風格）發行閉環（`Erc20Runtime/Erc20Init`＋calldata 產生器）、M42 標準相容驗證、黃皮書 `/yellowpaper`、主網/測試網分離、`SECURITY.md`。
- 承 M41：效能基準、鏈上治理（`/governance`）、`tacnet.sh` 一鍵 3 節點分佈式 BFT、壓力測試。
- 承 M38：預設英文、全域中英切換、i18n 網路優先、SW 快取根治語言殘留。

## 一、上傳到 GitHub（手機操作）

1. 下載本壓縮檔後，在手機上解壓 → 得到 `tacm-go/` 資料夾。
2. 打開 GitHub 你的專案倉庫（`TACm-Ai_Cyprto`）。
3. **方式 A（推薦）**：把解壓後的 `tacm-go/` 資料夾**整個上傳／拖入**倉庫根目錄（GitHub 網頁支援拖資料夾上傳，會保留資料夾層級）。上傳後倉庫根目錄應看到：`tacm-go/`、`README_UPLOAD.md`（本檔）。
4. **方式 B**：如果 GitHub 上傳時自動解壓成散檔，請先建一個 `tacm-go` 資料夾，把解壓出的所有檔案（cmd、internal、go.mod、go.sum、DEPLOY.md、Dockerfile 等）**全部移進 `tacm-go/` 資料夾內**，再上傳。

> ⚠️ 上傳完成後，**倉庫根目錄必須有一個 `tacm-go` 資料夾**（裡面有 cmd/、internal/、go.mod）。如果檔案散在倉庫根目錄（沒有 tacm-go 資料夾），請依照上面方式 A/B 調整，否則 Render 建置會失敗。

## 二、Render 部署設定（重要！）

本版結構是「repo 根目錄 = tacm-go 資料夾」，所以 Render 的兩個指令**必須**改成下面這樣（與舊版不同，舊版是直接 ./cmd/web）：

| Render 欄位 | 填入內容 |
|---|---|
| **Build Command** | `cd tacm-go && go build -o tacweb ./cmd/web` |
| **Start Command** | `cd tacm-go && ./tacweb -web-port $PORT -rpc-port $PORT -data-dir ./data -block-time 1 -difficulty 1` |

Render 設定路徑：Render Dashboard → 你的服務 → Settings → Build & Deploy。

## 三、驗證

部署成功後開啟：`https://你的服務名稱.onrender.com`，首頁應顯示 **TAC Autonomous Chain** 儀表板（英文介面、出塊高度持續增加、Total TACm Mined、TiUSD Circulation 900,000 等），右上角可切換「中文」。

- 區塊瀏覽器：`/`
- 錢包：`/wallet`
- 挖礦：`/mining`
- 交易所：`/exchange`
- 社群：`/community`
- DeFi：`/defi`（頂部「☰ Tools」選單內有入口）
- C2C：`/c2c`（頂部「☰ Tools」選單內有入口）
- 代幣：`/tokens`（Token Studio：發行/查詢/代簽轉帳，頂部導覽與「☰ Tools」選單內有入口）
