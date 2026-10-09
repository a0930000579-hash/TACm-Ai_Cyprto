# TACm-Go 壓縮檔上傳與 Render 部署說明（M38.4）

本壓縮檔解壓後會產生一個 **tacm-go 資料夾**（裡面是整套 Go 區塊鏈系統的完整程式碼）。

## 本版重點（M38.4）

- **預設語言改為英文**：全站預設英文介面，右上角「中文 / EN」按鈕可即時切換中英雙語（中文為第二語言）。
- 全域英文化：首頁儀表板、錢包、挖礦、交易所、社群、DeFi、C2C、白皮書入口、區塊瀏覽器、儀表板全部英文化；切換中文時可完整切回中文。
- 所有頁面 title（瀏覽器標籤）英文化：TAC Autonomous Chain · Network / Wallet / Mining / Exchange / Community / DeFi / C2C。
- 動態內容自動翻譯：挖礦排行、出塊速率、訂單狀態等 JS 即時渲染的內容，會隨語言切換自動翻譯（不需重新整理）。

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
