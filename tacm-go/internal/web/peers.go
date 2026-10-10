package web

// SeedInfo 為一個公開種子節點（M63：節點加入指引）。
// 新節點以 -p2p-seed 啟動時連這些節點完成握手並取得全網 peer 列表。
type SeedInfo struct {
	NodeID string `json:"node_id"`
	URL    string `json:"url"`
	Role   string `json:"role"` // anchor 錨點 / follower 跟隨節點
}

// mainnetSeeds 為主網 tacm-mainnet-1 的公開 seed 列表。
// 任何節點與任何入口（瀏覽器/API）看到的 seed 列表必須一致（用戶鎖定口徑：
// 「無論在哪裡看到的人數/節點都必須全網一致」）——此處為唯一權威來源，
// 全部節點共用同一份編譯進去的列表，不會因節點不同而不同。
var mainnetSeeds = []SeedInfo{
	{NodeID: "anchor", URL: "http://2.28.201.174:8080", Role: "anchor"},
	{NodeID: "render1", URL: "https://tacm-ai-cyprto.onrender.com", Role: "follower"},
}
