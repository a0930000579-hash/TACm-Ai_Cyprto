package community

// community.go — 內建 Facebook 風社群（搬運自原本 community_app/community_db 並優化）：
//   動態牆（post）、按讚、留言、市集（kind=market、TACM 計價）、廣告池。
//   身份＝錢包地址（Go 版無帳密系統，以 tx0 地址代表成員）。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store 社群資料庫。
type Store struct {
	db *sql.DB
}

const communitySchema = `
CREATE TABLE IF NOT EXISTS posts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  address TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'post',
  content TEXT,
  image_url TEXT,
  created_at INTEGER NOT NULL,
  mine_id TEXT DEFAULT NULL,
  price_tacm REAL DEFAULT NULL,
  sold INTEGER NOT NULL DEFAULT 0,
  buyer TEXT DEFAULT NULL
);
CREATE INDEX IF NOT EXISTS idx_posts_created ON posts(created_at DESC);
CREATE TABLE IF NOT EXISTS comments (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  post_id INTEGER NOT NULL,
  address TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS likes (
  post_id INTEGER NOT NULL,
  address TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (post_id, address)
);
CREATE TABLE IF NOT EXISTS ads (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  address TEXT NOT NULL,
  title TEXT NOT NULL,
  body TEXT,
  budget_tacm REAL NOT NULL DEFAULT 0,
  target TEXT,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS follows (
  follower TEXT NOT NULL,
  followee TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY (follower, followee)
);
CREATE INDEX IF NOT EXISTS idx_follows_followee ON follows(followee);
CREATE TABLE IF NOT EXISTS rewards (
  address TEXT PRIMARY KEY,
  points INTEGER NOT NULL DEFAULT 0,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS reward_meta (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  period INTEGER NOT NULL DEFAULT 0,
  last_tip INTEGER NOT NULL DEFAULT 0
);
`

// Open 打開（或創建）社群資料庫。
func Open(dataDir string) (*Store, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("community: 建立數據目錄: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)",
		filepath.Join(dataDir, "community.db"))
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("community: 開啟數據庫: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(communitySchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("community: 初始化 schema: %w", err)
	}
	// 相容遷移：舊 posts 表無 sold/buyer 欄位時補上（SQLite 無 ADD COLUMN IF NOT EXISTS）。
	if err := migrateColumns(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// migrateColumns 檢查並補齊 posts.sold / posts.buyer 欄位（舊庫相容）。
func migrateColumns(db *sql.DB) error {
	cols := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(posts)`)
	if err != nil {
		return fmt.Errorf("community: 檢查欄位: %w", err)
	}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			_ = rows.Close()
			return fmt.Errorf("community: 讀取欄位: %w", err)
		}
		cols[name] = true
	}
	_ = rows.Close()
	if !cols["sold"] {
		if _, err := db.Exec(`ALTER TABLE posts ADD COLUMN sold INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("community: 遷移 sold 欄位: %w", err)
		}
	}
	if !cols["buyer"] {
		if _, err := db.Exec(`ALTER TABLE posts ADD COLUMN buyer TEXT DEFAULT NULL`); err != nil {
			return fmt.Errorf("community: 遷移 buyer 欄位: %w", err)
		}
	}
	return nil
}

// Close 關閉社群資料庫。
func (s *Store) Close() error { return s.db.Close() }

// Post 貼文視圖（含讚/留言數與市集成交狀態）。
type Post struct {
	ID        int64    `json:"id"`
	Address   string   `json:"address"`
	Kind      string   `json:"kind"`
	Content   string   `json:"content"`
	ImageURL  *string  `json:"image_url,omitempty"`
	CreatedAt int64    `json:"created_at"`
	MineID    *string  `json:"mine_id,omitempty"`
	PriceTACM *float64 `json:"price_tacm,omitempty"`
	Sold      bool     `json:"sold"`
	Buyer     string   `json:"buyer,omitempty"`
	Likes     int64    `json:"likes"`
	Comments  int64    `json:"comments"`
	Author    string   `json:"author"` // 地址（前端虛化顯示）
}

// Comment 留言視圖。
type Comment struct {
	ID        int64  `json:"id"`
	PostID    int64  `json:"post_id"`
	Address   string `json:"address"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

// Ad 廣告視圖。
type Ad struct {
	ID         int64   `json:"id"`
	Address    string  `json:"address"`
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	BudgetTACM float64 `json:"budget_tacm"`
	Target     string  `json:"target"`
	Status     string  `json:"status"`
	CreatedAt  int64   `json:"created_at"`
}

// Stats 社群統計。
type Stats struct {
	Posts    int64 `json:"posts"`
	Comments int64 `json:"comments"`
	Likes    int64 `json:"likes"`
	Ads      int64 `json:"ads"`
	Members  int64 `json:"members"` // 發文/留言/按讚的去重地址數
}

const postCols = `p.id, p.address, p.kind, COALESCE(p.content,''), p.image_url, p.created_at, p.mine_id, p.price_tacm, p.sold, COALESCE(p.buyer,''),
 (SELECT COUNT(1) FROM likes l WHERE l.post_id=p.id) AS likes,
 (SELECT COUNT(1) FROM comments c WHERE c.post_id=p.id) AS comments`

func scanPost(sc interface{ Scan(...any) error }) (*Post, error) {
	var p Post
	var image, mine, buyer sql.NullString
	var price sql.NullFloat64
	if err := sc.Scan(&p.ID, &p.Address, &p.Kind, &p.Content, &image, &p.CreatedAt, &mine, &price,
		&p.Sold, &buyer, &p.Likes, &p.Comments); err != nil {
		return nil, err
	}
	if image.Valid {
		p.ImageURL = &image.String
	}
	if mine.Valid {
		p.MineID = &mine.String
	}
	if price.Valid {
		p.PriceTACM = &price.Float64
	}
	if buyer.Valid && buyer.String != "" {
		p.Buyer = buyer.String
	}
	p.Author = p.Address
	return &p, nil
}

// Feed 動態牆（全部貼文，最新在前）。
func (s *Store) Feed(limit, offset int) ([]Post, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT `+postCols+` FROM posts p ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("community: 讀取動態: %w", err)
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, fmt.Errorf("community: 掃描貼文: %w", err)
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// CreatePost 發文（kind=post|market）。
func (s *Store) CreatePost(address, kind, content string, imageURL string, priceTACM *float64) (int64, error) {
	if address == "" {
		return 0, fmt.Errorf("community: 缺少成員地址")
	}
	if content == "" && imageURL == "" {
		return 0, fmt.Errorf("community: 內容不可為空")
	}
	var img any
	if imageURL != "" {
		img = imageURL
	}
	res, err := s.db.Exec(`INSERT INTO posts(address, kind, content, image_url, created_at, price_tacm) VALUES(?,?,?,?,?,?)`,
		address, kind, content, img, time.Now().Unix(), priceTACM)
	if err != nil {
		return 0, fmt.Errorf("community: 發文: %w", err)
	}
	return res.LastInsertId()
}

// Like 按讚（INSERT OR IGNORE 冪等）。
func (s *Store) Like(postID int64, address string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO likes(post_id, address, created_at) VALUES(?,?,?)`,
		postID, address, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("community: 按讚: %w", err)
	}
	return nil
}

// Comments 留言列表（舊→新）。
func (s *Store) Comments(postID int64, limit int) ([]Comment, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, post_id, address, body, created_at FROM comments WHERE post_id=? ORDER BY created_at ASC LIMIT ?`, postID, limit)
	if err != nil {
		return nil, fmt.Errorf("community: 讀取留言: %w", err)
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.PostID, &c.Address, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AddComment 留言。
func (s *Store) AddComment(postID int64, address, body string) error {
	if body == "" {
		return fmt.Errorf("community: 留言不可為空")
	}
	_, err := s.db.Exec(`INSERT INTO comments(post_id, address, body, created_at) VALUES(?,?,?,?)`,
		postID, address, body, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("community: 留言: %w", err)
	}
	return nil
}

// Market 市集列表（kind=market）。
func (s *Store) Market(limit int) ([]Post, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT `+postCols+` FROM posts p WHERE p.kind='market' ORDER BY p.created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("community: 讀取市集: %w", err)
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPost 單篇貼文（buy-intent 用）。
func (s *Store) GetPost(id int64) (*Post, error) {
	row := s.db.QueryRow(`SELECT `+postCols+` FROM posts p WHERE p.id=?`, id)
	p, err := scanPost(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("community: 查詢貼文: %w", err)
	}
	return p, nil
}

// MarkSold 原子成交標記：僅在商品尚未售出時成功（防重複購買）；回傳是否成功。
func (s *Store) MarkSold(id int64, buyer string) (bool, error) {
	res, err := s.db.Exec(`UPDATE posts SET sold=1, buyer=? WHERE id=? AND sold=0`, buyer, id)
	if err != nil {
		return false, fmt.Errorf("community: 成交標記: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("community: 成交標記影響行數: %w", err)
	}
	return n > 0, nil
}

// UnmarkSold 成交回滾（付款失敗時釋放商品）。
func (s *Store) UnmarkSold(id int64, buyer string) error {
	if _, err := s.db.Exec(`UPDATE posts SET sold=0, buyer=NULL WHERE id=? AND buyer=?`, id, buyer); err != nil {
		return fmt.Errorf("community: 成交回滾: %w", err)
	}
	return nil
}

// Ads 廣告池（pending=待付款、active=投放中；最新在前）。
func (s *Store) Ads(limit int) ([]Ad, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`SELECT id, address, title, body, budget_tacm, target, status, created_at FROM ads ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("community: 讀取廣告: %w", err)
	}
	defer rows.Close()
	out := []Ad{}
	for rows.Next() {
		var a Ad
		if err := rows.Scan(&a.ID, &a.Address, &a.Title, &a.Body, &a.BudgetTACM, &a.Target, &a.Status, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CreateAd 發佈廣告（預設 pending：付款後才 active）。
func (s *Store) CreateAd(address, title, body, target string, budget float64) (int64, error) {
	if title == "" || budget <= 0 {
		return 0, fmt.Errorf("community: 請填寫標題與預算")
	}
	res, err := s.db.Exec(`INSERT INTO ads(address, title, body, budget_tacm, target, status, created_at) VALUES(?,?,?,?,?,'pending',?)`,
		address, title, body, budget, target, time.Now().Unix())
	if err != nil {
		return 0, fmt.Errorf("community: 發佈廣告: %w", err)
	}
	return res.LastInsertId()
}

// GetAd 單筆廣告（付款驗證用）。
func (s *Store) GetAd(id int64) (*Ad, error) {
	row := s.db.QueryRow(`SELECT id, address, title, body, budget_tacm, target, status, created_at FROM ads WHERE id=?`, id)
	var a Ad
	if err := row.Scan(&a.ID, &a.Address, &a.Title, &a.Body, &a.BudgetTACM, &a.Target, &a.Status, &a.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("community: 查詢廣告: %w", err)
	}
	return &a, nil
}

// MarkAdPaid 廣告付款完成 → active。
func (s *Store) MarkAdPaid(id int64) error {
	if _, err := s.db.Exec(`UPDATE ads SET status='active' WHERE id=?`, id); err != nil {
		return fmt.Errorf("community: 廣告付款標記: %w", err)
	}
	return nil
}

// Stats 社群統計（去重成員）。
func (s *Store) Stats() (*Stats, error) {
	var st Stats
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM posts`).Scan(&st.Posts); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM comments`).Scan(&st.Comments); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM likes`).Scan(&st.Likes); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM ads`).Scan(&st.Ads); err != nil {
		return nil, err
	}
	if err := s.db.QueryRow(`SELECT COUNT(DISTINCT address) FROM (SELECT address FROM posts UNION SELECT address FROM comments UNION SELECT address FROM likes)`).Scan(&st.Members); err != nil {
		return nil, err
	}
	return &st, nil
}

// ---------- M73-B：關注（動態牆） ----------

// Follow 追蹤一位成員（冪等）。
func (s *Store) Follow(follower, followee string) error {
	if follower == "" || followee == "" {
		return fmt.Errorf("community: 追蹤參數缺失")
	}
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO follows(follower,followee,created_at) VALUES(?,?,?)`,
		follower, followee, time.Now().Unix()); err != nil {
		return fmt.Errorf("community: 追蹤失敗: %w", err)
	}
	return nil
}

// Unfollow 取消追蹤。
func (s *Store) Unfollow(follower, followee string) error {
	if _, err := s.db.Exec(`DELETE FROM follows WHERE follower=? AND followee=?`, follower, followee); err != nil {
		return fmt.Errorf("community: 取消追蹤失敗: %w", err)
	}
	return nil
}

// Following 列出已追蹤的成員（依追蹤時間新→舊）。
func (s *Store) Following(follower string) ([]string, error) {
	rows, err := s.db.Query(`SELECT followee FROM follows WHERE follower=? ORDER BY created_at DESC`, follower)
	if err != nil {
		return nil, fmt.Errorf("community: 查詢追蹤列表: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// IsFollowing 是否已追蹤。
func (s *Store) IsFollowing(follower, followee string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM follows WHERE follower=? AND followee=?`, follower, followee).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// FollowingFeed 動態牆：只顯示已追蹤成員的貼文（含自己的）。
func (s *Store) FollowingFeed(follower string, limit, offset int) ([]Post, error) {
	if limit <= 0 {
		limit = 30
	}
	rows, err := s.db.Query(`SELECT `+postCols+` FROM posts p
		WHERE p.address=? OR p.address IN (SELECT followee FROM follows WHERE follower=?)
		ORDER BY p.created_at DESC LIMIT ? OFFSET ?`, follower, follower, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("community: 追蹤動態牆: %w", err)
	}
	defer rows.Close()
	out := []Post{}
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.Address, &p.Kind, &p.Content, &p.ImageURL, &p.CreatedAt, &p.MineID, &p.PriceTACM, &p.Sold, &p.Buyer, &p.Likes, &p.Comments); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---------- M73-B：互動積分與鏈上獎勵結算 ----------

// AddPoints 累積互動積分（發文/按讚/留言/成交等）。
func (s *Store) AddPoints(address string, pts int64) error {
	if address == "" || pts <= 0 {
		return nil
	}
	if _, err := s.db.Exec(`INSERT INTO rewards(address,points,updated_at) VALUES(?,?,?)
		ON CONFLICT(address) DO UPDATE SET points=points+?, updated_at=?`,
		address, pts, time.Now().Unix(), pts, time.Now().Unix()); err != nil {
		return fmt.Errorf("community: 累積積分: %w", err)
	}
	return nil
}

// Points 查詢成員積分。
func (s *Store) Points(address string) (int64, error) {
	var p int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(points),0) FROM rewards WHERE address=?`, address).Scan(&p); err != nil {
		return 0, fmt.Errorf("community: 查詢積分: %w", err)
	}
	return p, nil
}

// RewardRow 積分明細（結算與排行共用）。
type RewardRow struct {
	Address string `json:"address"`
	Points  int64  `json:"points"`
}

// TopRewards 積分排行（前 N）。
func (s *Store) TopRewards(limit int) ([]RewardRow, error) {
	rows, err := s.db.Query(`SELECT address, points FROM rewards WHERE points>0 ORDER BY points DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("community: 積分排行: %w", err)
	}
	defer rows.Close()
	out := []RewardRow{}
	for rows.Next() {
		var r RewardRow
		if err := rows.Scan(&r.Address, &r.Points); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SnapshotAndClearRewards 結算快照：讀取全部積分並清零（返回有積分者）。
// 快照與清零在同一事務，保證「每期只結算一次」。
func (s *Store) SnapshotAndClearRewards() ([]RewardRow, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT address, points FROM rewards WHERE points>0`)
	if err != nil {
		return nil, err
	}
	out := []RewardRow{}
	for rows.Next() {
		var r RewardRow
		if err := rows.Scan(&r.Address, &r.Points); err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, r)
	}
	_ = rows.Close()
	if len(out) > 0 {
		if _, err := tx.Exec(`UPDATE rewards SET points=0, updated_at=?`, time.Now().Unix()); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// RewardMeta 上一期結算資訊。
type RewardMeta struct {
	Period  int64 `json:"period"`
	LastTip int64 `json:"last_tip"`
}

// GetRewardMeta 讀取結算期號與上次結算鏈高。
func (s *Store) GetRewardMeta() (RewardMeta, error) {
	var m RewardMeta
	if err := s.db.QueryRow(`SELECT period, last_tip FROM reward_meta WHERE id=1`).Scan(&m.Period, &m.LastTip); err != nil {
		if err == sql.ErrNoRows {
			return RewardMeta{}, nil
		}
		return m, err
	}
	return m, nil
}

// SetRewardMeta 更新結算期號與鏈高。
func (s *Store) SetRewardMeta(period, lastTip int64) error {
	if _, err := s.db.Exec(`INSERT INTO reward_meta(id,period,last_tip) VALUES(1,?,?)
		ON CONFLICT(id) DO UPDATE SET period=?, last_tip=?`, period, lastTip, period, lastTip); err != nil {
		return fmt.Errorf("community: 更新結算期號: %w", err)
	}
	return nil
}
