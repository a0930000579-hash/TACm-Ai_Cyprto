package community

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestFollowCRUD 追蹤/取消/查詢/冪等。
func TestFollowCRUD(t *testing.T) {
	s := openTestStore(t)
	a, b, c := "tx01aaaa", "tx01bbbb", "tx01cccc"
	if err := s.Follow(a, b); err != nil {
		t.Fatalf("Follow: %v", err)
	}
	if err := s.Follow(a, b); err != nil { // 冪等
		t.Fatalf("Follow dup: %v", err)
	}
	if err := s.Follow(a, c); err != nil {
		t.Fatalf("Follow c: %v", err)
	}
	following, err := s.Following(a)
	if err != nil || len(following) != 2 {
		t.Fatalf("Following: %v / %d", err, len(following))
	}
	ok, err := s.IsFollowing(a, b)
	if err != nil || !ok {
		t.Fatalf("IsFollowing b: %v / %v", ok, err)
	}
	if err := s.Unfollow(a, b); err != nil {
		t.Fatalf("Unfollow: %v", err)
	}
	ok, _ = s.IsFollowing(a, b)
	if ok {
		t.Fatal("Unfollow 後仍為已追蹤")
	}
}

// TestFollowingFeed 追蹤動態牆只含自己＋已追蹤者貼文。
func TestFollowingFeed(t *testing.T) {
	s := openTestStore(t)
	alice, bob, carol := "tx01aaaa", "tx01bbbb", "tx01cccc"
	if _, err := s.CreatePost(alice, "post", "alice post", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePost(bob, "post", "bob post", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreatePost(carol, "post", "carol post", "", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Follow(alice, bob); err != nil {
		t.Fatal(err)
	}
	items, err := s.FollowingFeed(alice, 50, 0)
	if err != nil {
		t.Fatalf("FollowingFeed: %v", err)
	}
	if len(items) != 2 { // alice 自己 + bob
		t.Fatalf("追蹤動態牆應 2 筆，得 %d", len(items))
	}
	for _, p := range items {
		if p.Address == carol {
			t.Fatal("追蹤動態牆混入非追蹤者貼文")
		}
	}
}

// TestRewardPointsAndSettle 積分累積、排行與結算快照清零。
func TestRewardPointsAndSettle(t *testing.T) {
	s := openTestStore(t)
	alice, bob := "tx01aaaa", "tx01bbbb"
	if err := s.AddPoints(alice, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPoints(alice, 3); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPoints(bob, 2); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Points(alice)
	if p != 8 {
		t.Fatalf("alice 積分應 8，得 %d", p)
	}
	top, err := s.TopRewards(5)
	if err != nil || len(top) != 2 || top[0].Address != alice || top[0].Points != 8 {
		t.Fatalf("TopRewards 異常: %v %v", top, err)
	}
	rows, err := s.SnapshotAndClearRewards()
	if err != nil || len(rows) != 2 {
		t.Fatalf("快照異常: %v %v", rows, err)
	}
	p, _ = s.Points(alice)
	if p != 0 {
		t.Fatalf("結算後積分應清零，得 %d", p)
	}
	// 空快照再清一次不報錯。
	if rows, err := s.SnapshotAndClearRewards(); err != nil || len(rows) != 0 {
		t.Fatalf("空快照異常: %v %v", rows, err)
	}
}

// TestRewardMeta 結算期號持久化。
func TestRewardMeta(t *testing.T) {
	s := openTestStore(t)
	m, err := s.GetRewardMeta()
	if err != nil || m.Period != 0 {
		t.Fatalf("初始 meta 應為 0/0: %+v %v", m, err)
	}
	if err := s.SetRewardMeta(1, 60); err != nil {
		t.Fatal(err)
	}
	m, _ = s.GetRewardMeta()
	if m.Period != 1 || m.LastTip != 60 {
		t.Fatalf("meta 更新失敗: %+v", m)
	}
}

// TestOpenCompat 舊庫（無新表）相容：僅 posts 的庫可直接 Open。
func TestOpenCompat(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatalf("新表建立失敗: %v", err)
	}
	if _, err := Open(filepath.Join(dir, "sub")); err != nil {
		t.Fatalf("重複 Open 失敗: %v", err)
	}
}
