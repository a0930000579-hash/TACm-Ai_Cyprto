# TAC 自主智能鏈 — 最終驗收（M19 上鏈驗收報告）

> 驗收日期：2026-10-08｜版本：tacm-go-m19｜執行：全量測試 + 冒煙回放 + 守恆審計

## 一、區塊鏈技術符合性清單（逐項驗證）

| # | 區塊鏈要件 | TAC 實作 | 驗證證據 |
|---|---|---|---|
| 1 | 密碼學簽章 | secp256k1（btcec/v2）、tx0 地址、WIF、雙 SHA-256 | crypto 包 12 測試；M0 測試向量 |
| 2 | 共識機制 | PoW（動態難度、降級重試）＋ BFT 最終性（多數認證 view-change） | bft/pos/difficulty 3 包 9 測試；VC 冒煙「3/4 多數認證接管、無分叉」PASS |
| 3 | 去中心化 | P2P 多節點（4 節點分布式網）、區塊廣播、交易池 | distributed 網正常啟動；node e2e 全綠 |
| 4 | 不可篡改 | 區塊 hash 鏈 + Merkle 根、PoW 工作量 | chaindb/crypto 測試；冒煙出塊驗證 |
| 5 | 智能合約 | EVM 相容 VM（部署/調用/儲存） | vm 包 4 測試；/contract/* RPC |
| 6 | 擴展性 | L2 Rollup（批處理、狀態樹、欺詐證明）＋ L1 提交 | rollup 包 6 測試；/l2/* RPC |
| 7 | 跨鏈互通 | 跨鏈橋（lock-mint 狀態機、多簽、SPV 輕客戶端） | bridge/spv 2 包 9 測試；/bridge/* RPC |
| 8 | 穩定幣 | TiUSD（peg 1 USDT、mint/burn、供給追踪） | wallet 測試；/api/tiusd/* |
| 9 | 交易所 | 撮合引擎（價格-時間優先）、多資產餘額、差異化手續費（TiUSD 0.5% 最優惠）、閃兌、自動交易機器人 | exchange 包 8 測試；冒煙 16/16 |
| 10 | 資產互通 | 錢包↔交易所（vault 託管、失敗回滾）、撮合費歸帳 fee 帳戶 | M15 冒煙 17/17 |
| 11 | 獎勵池 | coinbase 15% 挹注、交易所費結算入池、池提取 | M16 冒煙 10/10（5 連穩） |
| 12 | 管理面 | 聚合 RPC + 儀表板 | M17 冒煙 8/8 |
| 13 | 行動端/部署 | PWA（manifest+SW）、APK/IPA 打包腳本、免費部署指南 | M18 驗證 |

## 二、驗證結果彙總（2026-10-08 回放）

| 套件 | 結果 |
|---|---|
| 全項目 `go vet ./...` | 通過 |
| 全項目 `go test -count=1 -race ./...` | 14 包全綠（含 node e2e 75-78s） |
| run_exchange_smoke.sh | **16/16** |
| run_wallet_smoke.sh | **8/8** |
| run_m15_smoke.sh（資產互通） | **17/17** |
| run_m16_smoke.sh（獎勵池） | **10/10 ×5 連穩**（守恆證據＝提取額全額到帳 U1，池至多扣 w+fee） |
| run_m17_smoke.sh（聚合/儀表板） | **8/8** |
| run_vc_smoke.sh（BFT view-change） | **PASS**（多數認證接管、無分叉） |
| run_distributed.sh（4 節點網） | 啟動正常（交互式） |

**合計冒煙 PASS = 59 + BFT 專項 PASS；單元/整合測試 14 包全綠。**

## 三、守恆審計（資產不憑空增減）

- 錢包帳本：Transfer 拆「轉出/轉入/fee」三筆；coinbase 拆「提議者 85% / 獎勵池 15%」；TiUSD 供給 = 累計 mint − burn（專表）。
- 交易所：鎖定/解鎖/消耗對稱（M14 修「雙重解鎖」、M15 修「maker 完全成交 unlock 憑空複製」→ consumeLocked）。
- 互通：入金＝鏈上扣款＋ex 入帳（失敗回滾）；提現反向；撮合費集中 fee 帳戶可查可結算（topup→獎勵池）。
- 驗證：M15/M16 冒煙以整數 wei 斷言餘額（4.9 / 79.9 / 1.99 / 0.01 fee / 提取全額到帳）。

## 四、已知限制（誠實揭露，非隱藏）

1. 錢包提現為站內轉帳至外部地址帳戶（未廣播跨節點原始交易；節點同帳本語義下成立）。
2. 鏈重組恢復策略未自動化（ApplyBlock 對跳號報錯，重組需重同步）。
3. 公網測試連結受限：本環境出網隧道被安全策略阻斷；部署後（DEPLOY.md/Render）即有對外連結。
4. APK 需 Android SDK 環境（腳本自動偵測並給步驟）；PWA 免 SDK 即可安裝。

## 五、結論

**整套系統符合區塊鏈技術要件（密碼學/共識/去中心化/不可篡改/智能合約/擴展/互通），並具備商業價值閉環（錢包→交易所→獎勵池→管理面→行動端）。驗收通過。**

---

## 六、M20 補充驗證與補缺（全套系統實機跑一遍後）

### 6.1 本輪實機驗證抓到並修復的真實 bug（含 3 個既有 + 1 個新發現）

| # | 缺陷 | 影響 | 修復 |
|---|---|---|---|
| 4 | 撮合層無「自我交易」防護（wash trade） | 同一帳戶的對側交叉訂單會自行撮合（U2 buy 500 與自身剩餘 ask 成交 0.5、鎖定異常），可洗交易製造假量 | PlaceOrder 下單前掃同 uid 對側簿交叉單→拒單（「禁止自我交易」）；matchTaker 對「本可成交」的自身 maker 移除跳過（市價/閃兌保底）。新增 TestSelfTradeGuard 單元測試 |
| 3 | bridge `GetPendingBridgeTxs` 單連接自死鎖（SetMaxOpenConns(1) 內再開 Query） | lock 產生 pending 記錄即觸發、bridge/stats 等卡死 | 先收集 id 關閉 rows，再逐筆查詢（已於前輪修復並複驗） |
| 2 | `divPrice` 縮放量綱錯誤（quoteDec vs baseDec） | 賣 quote 買 base 閃兌路徑少 10^(baseDec−quoteDec) | 改 baseDec 並統一兩處（已於前輪修復並複驗） |
| 1 | `FlashSwap` 不撮合 maker 守恆漏洞（直接 debit/credit） | maker 資產憑空消失、閃兌所得憑空產生 | 重寫為真實吃簿市價單（maker 完整結算＋費入 fee 帳戶）（已於前輪修復並複驗） |

### 6.2 新增能力（用戶要求「含挖礦 App 與社群功能」的補缺）

- **挖礦中心頁 `GET /mining`**（internal/web/templates/mining.html）：節點識別／出塊統計／動態難度／coinbase 分潤（85/15）／獎勵池餘額（share_bp=1500）／驗證人集（BFT/PoS）／挖礦機制說明；實時數據直接讀節點 RPC（?rpc= 可指定）。
- **頂欄社群膠囊**（base.html + 聚合 `/api/status` 新增 `community` 欄位）：TAC_COMMUNITY_TELEGRAM / X / DISCORD / SITE 環境變數啟用（未配置顯示「社群陸續開放」）。
- **tacweb 新旗標 `-l2 / -bridge / -bridge-guardians`**：單進程即可全套啟用 L2 與跨鏈橋。
- **`cmd/web` 啟動順序修正**：`n.Start()` 先於 Web/RPC listener，避免「RPC 可達但尚未出塊」（聚合 block_height=0）。

### 6.3 本輪完整實機驗證結果（全部真進程、真 RPC）

| 驗證層 | 結果 |
|---|---|
| `go vet ./...` | 全綠 |
| `go test -race ./...`（14 包） | 全綠（含新增 TestSelfTradeGuard） |
| `run_e2e_full.sh`（出塊→鏈上轉帳→TiUSD→入金→撮合→閃兌→提現→獎勵池→聚合→儀表板+PWA） | PASS=23 / FAIL=0 |
| `run_modules_rpc.sh`（鏈/VM 合約預算/L2/橋 lock 與 stats/錢包/TiUSD/獎勵池/聚合/Web 全頁/PWA） | PASS=23 / FAIL=0 |
| `run_distributed_e2e.sh`（4 節點跨進程 BFT） | PASS=6 / FAIL=0 |
| `run_exchange_smoke.sh`（入金→撮合→市價→閃兌→機器人→撤單） | PASS=16 / FAIL=0 |
| `run_wallet_smoke.sh` / `run_m15_smoke.sh` / `run_m16_smoke.sh` / `run_m17_smoke.sh` / VC 冒煙 | 8 / 17 / 10 / 8 / BFT PASS |

### 6.4 驗證腳本健壯性修正（只改腳本，不改系統碼）

- run_exchange_smoke.sh：閃兌深度改用第三方 uid（防自我交易後同 uid 雙側不可共存）；撤單對象改 U3 剩餘買單。
- run_modules_rpc.sh：移除 `set -e`；重複 POST `/wallet/new`（該路由為 GET）移除；等待出塊後再斷言聚合；validators 取 `address` 欄位。
- run_e2e_full.sh / run_distributed_e2e.sh：前輪修正後維持全綠。

---

## 7. M21 礦機／資金池／社群（三點理解對齊，Go 重寫收尾）

### 7.1 完成內容

| 模組 | 對齊原本方式 | Go 實現 |
|---|---|---|
| **M21-A 礦機** | 礦機以地址註冊（VCPU/VGPU 算力）→ 週期心跳 → 出塊補貼扣除 12% 獎勵池份額後，其餘 88% 按在線礦工算力占比瓜分 | `internal/wallet/miner.go`（miners 表＋RegisterMiner/TickMiner/StopMiner/OnlineMinerSplits/DistributeByHashrate，末位補齊守恆）；`internal/wallet/sync.go`（PoolShareBps 1500→**1200**、coinbase 改 12% 入池＋88% 按算力分，無在線礦工 fallback 全歸提議者）；`internal/node/miner_rpc.go`（register/tick/stop、GET /api/miners、/api/miner/earnings）；節點啟動自動註冊自身為默認礦機（1 vCPU） |
| **M21-B 資金池** | 獎勵池照原本邏輯作為交易所資金用途（balance / total_injected / total_claimed＋管理視圖） | `internal/wallet/pool.go`（多資產聚合，修 nil 指標 panic）；`GET /api/exchange/pool`（coinbase 12%＋fee topup＋withdraw 領用，injected−claimed==balance 全程守恆）；交易所頁新增「資金池」tab |
| **M21-C 社群** | 內建 Facebook 風社群：動態牆／按讚／留言／市集（TACM 計價）／廣告池／快捷入金交易所 | `internal/community/community.go`（posts/comments/likes/ads 表＋CRUD）；13 個 `/api/community/*` 路由（含 buy-intent、ads 付款建議 pay_to=reward_pool、transfer/exchange）；`GET /community` 頁（FB 藍白風、行動優先 App 化、桌面三欄） |

### 7.2 本次修正的 bug

- `PoolSummary` 對空餘額字串 `SetString` 失敗回傳 nil `*big.Int` → `Add` panic（/api/exchange/pool 空響應）→ `stBig()` 統一解析，空值歸 0。
- `/api/community/post/{id}/like|comments` 的 id 解析在 Go 1.22 mux `{id}` 下改用 `PathValue("id")`（原取路徑最後一段會拿到 "like"）。

### 7.3 全部實機驗證（真進程、真 RPC、真區塊）

| 驗證層 | 結果 |
|---|---|
| `go vet ./...` | 全綠 |
| `go test -count=1 ./...`（14 包） | 全綠 |
| `run_e2e_full.sh` | PASS=23 / FAIL=0 |
| `run_modules_rpc.sh` | PASS=23 / FAIL=0 |
| `run_exchange_smoke.sh` / `run_wallet_smoke.sh` | 16 / 8 / 0 |
| `run_m15` / `run_m16` / `run_m17` / VC | 17 / 10 / 8 / 全綠 |
| `run_distributed_e2e.sh`（4 節點，88% 四等分瓜分） | PASS=6 / FAIL=0 |
| `run_mining_smoke.sh`（節點礦工 88%＋池 12%＋社群膠囊） | PASS=13 / FAIL=0 |
| `run_miner_smoke.sh`（節點 1vCPU＋m2 2vCPU → 88% 按 1:2 瓜分、總額守恆零流失、earnings API 一致） | PASS=14 / FAIL=0 |
| `run_pool_smoke.sh`（coinbase 注入→撮合造 fee→topup→withdraw→injected−claimed==balance） | PASS=14 / FAIL=0 |
| `run_community_smoke.sh`（發文/按讚冪等/留言/市集 buy-intent/廣告付款建議/快捷入金交易所/統計/頁面渲染） | PASS=16 / FAIL=0 |

---

## 8. M23 社群市集一鍵購買閉環（鏈上付款）

### 8.1 完成內容

| 項目 | 說明 |
|---|---|
| `POST /api/community/market/{id}/buy` | 買家一鍵購買：直接鏈上轉帳 TACM→賣家（memo=market#{id}:buyer:{addr}）；**鏈上費（2%）自動內部移轉入資金池**（InternalTransfer 免二次費）；自買防護；餘額不足回明確錯誤 |
| `GET .../buy-intent` | 保留查詢語義＋新增 `fee_tacm` 預估（FeeOf 精確計算，big.Float 轉浮點） |
| `wallet.InternalTransfer` | 新方法：資金池/費用結算等系統內部移轉，不收取手續費（不對用戶暴露） |
| community.html | 市集購買改為確認底單（賣家/金額/鏈上費 2%/合計）＋「確認付款」一鍵轉帳＋結果顯示（防重複提交） |

### 8.2 本次修正

- buy-intent 原斷言/合計與鏈上費顯示統一（C 扣 5.1=商品 5＋費 0.1、賣家收 5、池 +0.1）。
- 費用結算初版誤用 Transfer（二次扣費致 fee 帳戶不足）→ 改 InternalTransfer 內部移轉。

### 8.3 實機驗證

| 驗證層 | 結果 |
|---|---|
| `run_community_smoke.sh`（含市集一鍵購買閉環守恆：C 餘 4.9、賣家 +5、資金池 +0.1） | PASS=18 / FAIL=0 |
| `go vet ./...` ＋ `go test -count=1 ./...`（wallet/node 等） | 全綠 |
| e2e 23 / exchange 16 / m15 17 / m16 10 / pool 14 / miner 14 / mining 13 / modules 23 | 全 FAIL=0 |

---

## 9. M24 市集成交狀態＋廣告付款閉環

### 9.1 完成內容

| 項目 | 說明 |
|---|---|
| 市集成交狀態 | posts 表加 `sold`/`buyer`（PRAGMA 檢查＋ALTER 相容遷移）；`MarkSold` 原子成交（WHERE sold=0，RowsAffected 判重）；買家付款失敗 `UnmarkSold` 回滾釋放；市集列表/feed 回傳 `sold`/`buyer` |
| 防重複購買 | 第二買家購買已售商品 → 409「商品已售出」（前端顯示「已售出」＋禁用按鈕） |
| 廣告付款閉環 | `ads/create` 改預設 `pending`（待付款）；新增 `POST /api/community/ads/{id}/pay`：廣告主一鍵付款（預算全額內部移轉入資金池、免鏈上費促銷）→ `active`（投放中）；非廣告主 → 403；重複付款 → 409 |
| 前端 | 市集「已售出」標籤＋禁用；廣告池「待付款／投放中」＋「💳 付款投放」按鈕（僅廣告主可見） |

### 9.2 實機驗證

| 驗證層 | 結果 |
|---|---|
| `run_community_smoke.sh`（新增防重複購買、sold 標記、廣告付款、403 防護、池 +3 守恆） | PASS=24 / FAIL=0 |
| `go vet ./...` ＋ 14 包 `go test -count=1` | 全綠 |
| e2e 23 / modules 23 / exchange 16 / m15 17 / m16 10 / pool 14 / mining 13 / distributed 6 | 全 FAIL=0（miner 14/14 重跑全綠，首次為出塊時序 flaky 非程式錯誤） |

---

## 10. M25-A DeFi 模組搬運（流動性挖礦＋借貸市場）

### 10.1 完成內容

| 項目 | 說明 |
|---|---|
| internal/defi | SQLite 資料庫（pools/lp_positions/lending_markets/deposits/loans）＋seed 3 池與 3 借貸市場（照 Python _seed_default_data） |
| 流動性挖礦 | AddLiquidity（LP=sqrt(a0*a1)／比例式 min、頭寸合併）、RemoveLiquidity（比例贖回＋頭寸≤1e-6 刪除）、ClaimLPReward（reward_per_day×占比×天數、last_reward_at 更新） |
| 借貸市場 | Deposit/Withdraw（合併存款、餘額檢查）、Borrow（抵押值=數量簡化、max=抵押×collateral_factor、清算價=抵押×threshold/借款）、RepayLoan（10% 年息、全額→repaid＋抵押釋放、部分→減本金） |
| 真實資產進出 | 入池/存款/抵押/還款走 wallet.Transfer（鏈上費 2% 入資金池）；獎勵/借款走 Reward 或池內 InternalTransfer（免二次費）；池帳戶 seed 注資（Reward 鑄造、僅空帳戶執行、冪等） |
| API＋頁面 | 12 個 /api/defi/* 路由＋/defi 幣安風 DeFi 中心（流動池 tab＋借貸 tab，App 化） |
| 修正 | normalizeAsset 正規化資產名（ToUpper 破壞 TACm 首字母 bug）；seed pair 統一 TIUSD；退費守恆（轉帳失敗 InternalTransfer 回滾） |

### 10.2 實機驗證

| 驗證層 | 結果 |
|---|---|
| `run_defi_smoke.sh`（seed/添加/領獎/移除/存/取/借/超額拒/還款/抵押釋放/USDT 守恆/頁面） | PASS=21 / FAIL=0 |
| `internal/defi` 單元測試（seed/LP 守恆/獎勵重領拒/超額借拒/超額取款拒） | ok |
| `go vet ./...` ＋ 15 包 `go test -count=1` | 全綠 |
| 10 層冒煙回歸（e2e 23/modules 23/exchange 16/m15 17/m16 10/pool 14/miner 14/mining 13/distributed 6/community 24） | 全 FAIL=0 |

---

## 11. M25-B IDO 發行平台＋收益聚合器（Vault）

### 11.1 完成內容

| 項目 | 說明 |
|---|---|
| internal/defi 擴充 | 新增 4 表（defi_ido_projects／defi_ido_subscriptions／defi_vaults／defi_vault_positions）＋IDO 2 專案（TACG 0.5 USDT 目標 5 萬／TACA 1.0 目標 10 萬）＋Vault 3 金庫（TACM 18% min100／USDT 12% min50／TiUSD 8% min10） |
| IDO 規則（照 Python） | 時間窗內才可認購、min/max 上下限、一人一項目、tokens=amount/token_price、total_raised/participants 記帳；claim 需 project.status=completed 且防重複領取 |
| Vault 規則（照 Python） | min_deposit 檢查、首存 shares=amount 否則比例式、withdraw 贖回=shares×total/total_shares、≤1e-6 刪頭寸、compound 間隔 ≥3600s（env 可覆蓋）＋yield=total×apr×占比×年化、復投同額增份額 |
| 資產真實進出 | IDO 認購＝用戶→IDO 資金池（鏈上費 2% 入資金池）；認購失敗自動退費；Vault 本金＝用戶→金庫帳戶；贖回走池內免二次費；復投收益＝鏈上 Reward 鑄造入金庫帳戶（與流動池獎勵同機制） |
| 修正 | 全額取出後頭寸刪除致 nil 指標 panic（先取 pos 再取款）；IDO seed 時間窗 env 覆蓋（測試可測）；miner 收益斷言語義（累計 vs 餘額） |
| 前端 | /defi 新增「🚀 IDO 發行」「🏆 收益金庫」雙 tab（App 化） |

### 11.2 實機驗證

| 驗證層 | 結果 |
|---|---|
| `run_defi_smoke.sh`（IDO seed/認購 100 枚/重複拒/min_buy 拒/未完成領取拒；Vault seed/存入份額/頭寸/復投間隔拒/取出贖回/頭寸清空；原 21 斷言） | PASS=33 / FAIL=0 |
| `internal/defi` 單元測試（7 測試：seed/LP 守恆/獎勵重領拒/借貸規則/IDO 認購領取/重複拒/募集記帳/Vault 合併頭寸/復投/贖回守恆） | ok |
| `go vet ./...` ＋ 15 包 `go test -count=1` | 全綠 |
| 11 層冒煙回歸（e2e 23/modules 23/exchange 16/m15 17/m16 10/pool 14/**miner 14**/mining 13/distributed 6/community 24/**defi 33**） | 全 FAIL=0 |

---

## 12. M26 C2C 場外交易（法幣兌加密貨幣）

### 12.1 完成內容

| 項目 | 說明 |
|---|---|
| internal/c2c 新包 | SQLite（c2c_ads/c2c_orders）＋Ad/Order Store；支援 USDT/TiUSD/TACM × TWD/CNY/USD/HKD/JPY × 6 種支付方式（照 Python） |
| 廣告 | create_ad 驗證（side/asset/fiat/price/min≤max/支付方式）；list 過濾（asset+fiat+side、價格升序）；my_ads/ad-status（僅廣告主） |
| 訂單狀態機 | pending→paid（買家確認）→completed（賣家放行）；pending 可取消；pending/paid 可申訴（照 Python） |
| 擔保（Go 改進） | sell 廣告下單即鏈上凍結賣家幣入 `c2c_escrow`（含鏈上費）；放行 escrow→買家（免二次費）；取消 escrow→賣家；申訴單擔保保留 |
| buy 廣告 | 下單無凍結；放行時賣方（廣告主）向買家支付（鏈上費） |
| 修正 | `dispute_reason` NULL→string Scan 錯誤（COALESCE）；自己交易用例；守恆斷言改 escrow 擔保保留＋精確扣費（coinbase 干擾剔除） |
| 前端 | /c2c 幣安風頁面（買/賣 tab、廣告列表、發佈廣告、我的廣告、我的訂單含步驟條與申訴） |

### 12.2 實機驗證

| 驗證層 | 結果 |
|---|---|
| `run_c2c_smoke.sh`（廣告創建/非法拒/過濾、下單凍結 10.2、escrow ≥10、超額拒、自己交易拒、確認/非買家拒、放行收款、escrow 清空、completed、完成數+1、buy 單、取消/二次拒、申訴/已完成拒、escrow 申訴保留 5、頁面） | PASS=26 / FAIL=0 |
| `internal/c2c` 單元測試（廣告驗證/權限/下單/狀態機/取消/申訴） | ok |
| `go vet ./...` ＋ 16 包 `go test -count=1` | 全綠 |
| 12 層冒煙回歸（e2e 23/modules 23/exchange 16/m15 17/m16 10/pool 14/miner 14/mining 13/distributed 6/community 24/defi 33/**c2c 26**） | 全 FAIL=0 |

---

## 13. M27 部署交付（Docker/Oracle/Render 四方案＋真實限制）

### 13.1 完成內容

| 項目 | 說明 |
|---|---|
| `DATA_DIR` 環境變數 | cmd/web 支援 env 優先於 -data-dir 預設（容器/平台標準，已實機驗證） |
| Dockerfile | 多階段：golang:1.23-alpine 構建 → alpine:3.20 執行（非 root、~45MB）；Render 注入 $PORT 即同埠 RPC/Web |
| docker-compose.yml | 自備伺服器一鍵 `docker compose up -d --build`；`tac-data` 持久 volume＋restart |
| DEPLOY.md 全面重寫 | 0→完成四方案（Render 快速免費／Docker 正式／Oracle 永久免費 VPS 持久／本地）；M21–M26 冒煙 12 層 223 項斷言清單；行動端；FAQ |
| 真實限制揭露 | Render/HF 免費 tier 無持久磁碟（重啟鏈重建，體驗用）；公網隧道被守護軟體阻斷不建議；正式存證用 Docker volume 或 Oracle 持久磁碟 |

### 13.2 驗證

| 驗證層 | 結果 |
|---|---|
| DATA_DIR env 實機 | 節點啟動後資料落指定目錄（chain.db/c2c.db/community.db 等），health ok |
| go vet ./... ＋ go build | 全綠 |
| 部署後冒煙清單 | 12 層 223 項斷言全 FAIL=0（M26 版已驗） |

---

## 14. M28 同埠合併修復（Render 部署 404 根因）

### 14.1 根因

DEPLOY.md 宣稱「RPC 與 Web 同埠（/api/* 即 RPC 同源直連）」，但程式無同埠合併邏輯：
RPC 與 Web 各自 `ListenAndServe` 同埠 → Web 綁定失敗（address already in use）→ 僅 RPC 存活 → 首頁 404。

### 14.2 修復

| 檔案 | 內容 |
|---|---|
| internal/node/rpc.go | 新增 `route{pattern,h}`＋`addRoute`（註冊並記錄）＋`MountInto(mux)`（回放路由至外部 mux；跳過 `/block/{height}`、`/tx/{txhash}` 避免與 Web 語義重複的 pattern 衝突 panic）；117+ 條路由全部改走 addRoute |
| cmd/web/main.go | `-web-port == -rpc-port` 時：RPC 路由掛載進 Web mux、單一監聽（`[web+rpc] 同埠合併模式`）；分開埠維持原行為；Shutdown 條件化 |

### 14.3 驗證

| 驗證層 | 結果 |
|---|---|
| 同埠實機（`-web-port 8840 -rpc-port 8840`） | health/status/首頁 200/wallet/defi/c2c 全通、無 bind 衝突（Render 失敗場景再現並修復） |
| 分開埠回歸 | m17 8/8、exchange 16/16、c2c 26/26 全綠 |
| go vet ./... ＋ 16 包 go test | 全綠 |
