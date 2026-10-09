# TACm-Go 壓縮檔上傳與 Render 部署說明（M46）

本壓縮檔解壓後會產生一個 **tacm-go 資料夾**（裡面是整套 Go 區塊鏈系統的完整程式碼）。

## 本版重點（M46）

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
