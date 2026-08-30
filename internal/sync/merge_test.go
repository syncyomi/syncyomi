package sync

import (
	"context"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/database"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"google.golang.org/protobuf/proto"
)

type fakeNotifier struct{}

func (fakeNotifier) Find(context.Context, domain.NotificationQueryParams) ([]domain.Notification, int, error) {
	return nil, 0, nil
}
func (fakeNotifier) FindByID(context.Context, int) (*domain.Notification, error) { return nil, nil }
func (fakeNotifier) Store(context.Context, domain.Notification) (*domain.Notification, error) {
	return nil, nil
}
func (fakeNotifier) Update(context.Context, domain.Notification) (*domain.Notification, error) {
	return nil, nil
}
func (fakeNotifier) Delete(context.Context, int) error                         { return nil }
func (fakeNotifier) Send(domain.NotificationEvent, domain.NotificationPayload) {}
func (fakeNotifier) Test(context.Context, domain.Notification) error           { return nil }

func newTestService(t *testing.T) (*service, *database.DB) {
	t.Helper()
	dir := t.TempDir()
	log := logger.Mock()
	db, err := database.NewDB(&domain.Config{DatabaseType: "sqlite", ConfigPath: dir}, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	apiRepo := database.NewAPIRepo(log, db)
	if err := apiRepo.Store(context.Background(), &domain.APIKey{Name: "t", Key: "key1", Scopes: []string{}}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(log, database.NewSyncRepo(log, db, 3), database.NewSyncStore(log, db, 3), fakeNotifier{}, apiRepo).(*service)
	return svc, db
}

func mangaOf(source int64, url string, version int64, favorite bool, chapters ...*pb.BackupChapter) *pb.BackupManga {
	m := &pb.BackupManga{Source: source, Url: url, Title: url, Version: version, Chapters: chapters}
	backup.SetFavorite(m, favorite)
	return m
}

func chapterOf(url string, version int64, read bool) *pb.BackupChapter {
	return &pb.BackupChapter{Url: url, Name: url, Version: version, Read: read}
}

func sync2(t *testing.T, svc *service, device string, cursor int64, full bool, b *pb.Backup, deleted ...int64) *MergeResponse {
	t.Helper()
	resp, err := svc.Merge(context.Background(), MergeRequest{
		APIKey: "key1", Device: domain.DeviceInfo{ID: device, Name: device}, Cursor: cursor, Full: full, Backup: b, DeletedCategories: deleted,
	})
	if err != nil {
		t.Fatalf("%s merge: %v", device, err)
	}
	return resp
}

func urls(b *pb.Backup) map[string]*pb.BackupManga {
	out := map[string]*pb.BackupManga{}
	for _, m := range b.BackupManga {
		out[m.Url] = m
	}
	return out
}

// The bug from jobobby04/TachiyomiSY#1635: B syncs between A adding M and A uploading it.
func TestMerge_TwoDeviceRace(t *testing.T) {
	svc, _ := newTestService(t)

	a := sync2(t, svc, "A", 0, true, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/old", 1, true)}})
	b := sync2(t, svc, "B", 0, true, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/old", 1, true)}})
	if len(b.Backup.BackupManga) != 1 {
		t.Fatalf("B initial = %v", b.Backup.BackupManga)
	}

	// A adds M (locally at T1, uploads after B's sync)
	a = sync2(t, svc, "A", a.Cursor, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	if a.Changed {
		t.Errorf("A upload of its own manga reported changed=%v", a.Changed)
	}

	// B syncs with a delta that does not mention M: M must come back, not be dropped
	b = sync2(t, svc, "B", b.Cursor, false, &pb.Backup{})
	if !b.Changed || urls(b.Backup)["/m"] == nil {
		t.Fatalf("B did not receive M: changed=%v manga=%v", b.Changed, b.Backup.BackupManga)
	}
	// B syncs again: nothing new
	b = sync2(t, svc, "B", b.Cursor, false, &pb.Backup{})
	if b.Changed {
		t.Errorf("B second delta changed=%v", b.Changed)
	}

	// A syncs again with a full snapshot still containing M: no-op, M stays
	a = sync2(t, svc, "A", a.Cursor, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/old", 1, true), mangaOf(1, "/m", 1, true)}})
	if a.Changed {
		t.Errorf("A resync changed=%v", a.Changed)
	}
	snap, err := svc.Snapshot(context.Background(), "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := backup.Decode(snap.Data)
	if len(full.BackupManga) != 2 {
		t.Errorf("store has %d manga, want 2", len(full.BackupManga))
	}
}

func TestMerge_ReadStateAndUnfavoriteFlow(t *testing.T) {
	svc, _ := newTestService(t)

	a := sync2(t, svc, "A", 0, true, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true, chapterOf("/c1", 1, false), chapterOf("/c2", 1, false))}})
	b := sync2(t, svc, "B", 0, true, &pb.Backup{})
	if urls(b.Backup)["/m"] == nil || len(urls(b.Backup)["/m"].Chapters) != 2 {
		t.Fatalf("B full sync = %v", b.Backup.BackupManga)
	}

	// B reads c1 (chapter version bump) and sends only that chapter
	b = sync2(t, svc, "B", b.Cursor, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true, chapterOf("/c1", 2, true))}})
	if b.Changed {
		t.Errorf("B own read reported changed")
	}

	// A's delta: gets /m with only c1
	a = sync2(t, svc, "A", a.Cursor, false, &pb.Backup{})
	m := urls(a.Backup)["/m"]
	if !a.Changed || m == nil || len(m.Chapters) != 1 || m.Chapters[0].Url != "/c1" || !m.Chapters[0].Read {
		t.Fatalf("A delta = changed=%v %v", a.Changed, a.Backup.BackupManga)
	}

	// A unfavorites (manga version bump), B receives favorite=false
	a = sync2(t, svc, "A", a.Cursor, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 2, false)}})
	b = sync2(t, svc, "B", b.Cursor, false, &pb.Backup{})
	m = urls(b.Backup)["/m"]
	if !b.Changed || m == nil || backup.IsFavorite(m) {
		t.Fatalf("B did not receive unfavorite: %v", b.Backup.BackupManga)
	}

	// B's stale copy (version 1, favorite) loses and B gets the server copy back
	b = sync2(t, svc, "B", b.Cursor, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	m = urls(b.Backup)["/m"]
	if !b.Changed || m == nil || m.Version != 2 {
		t.Fatalf("stale resend: changed=%v %v", b.Changed, b.Backup.BackupManga)
	}
}

func TestMerge_CategoryDeleteAndRefs(t *testing.T) {
	svc, _ := newTestService(t)

	cats := []*pb.BackupCategory{{Name: "Reading", Order: 0, Uid: 10, Version: 1}, {Name: "Done", Order: 1, Uid: 20, Version: 1}}
	m := mangaOf(1, "/m", 1, true)
	m.Categories = []int64{1} // Done, by order
	a := sync2(t, svc, "A", 0, true, &pb.Backup{BackupCategories: cats, BackupManga: []*pb.BackupManga{m}})

	// B has the categories in another order: membership must follow the category, not the number
	bcats := []*pb.BackupCategory{{Name: "Done", Order: 0, Uid: 20, Version: 1}, {Name: "Reading", Order: 1, Uid: 10, Version: 1}}
	b := sync2(t, svc, "B", 0, true, &pb.Backup{BackupCategories: bcats})
	got := urls(b.Backup)["/m"]
	if got == nil || len(got.Categories) != 1 {
		t.Fatalf("B manga = %v", b.Backup.BackupManga)
	}
	var doneOrder int64 = -1
	for _, c := range b.Backup.BackupCategories {
		if c.Name == "Done" {
			doneOrder = c.Order
		}
	}
	if got.Categories[0] != doneOrder {
		t.Errorf("manga category order = %d, want Done's order %d", got.Categories[0], doneOrder)
	}

	// B deletes "Done"
	b = sync2(t, svc, "B", b.Cursor, false, &pb.Backup{BackupCategories: []*pb.BackupCategory{bcats[1]}}, 20)
	a = sync2(t, svc, "A", a.Cursor, false, &pb.Backup{BackupCategories: cats})
	if !a.Changed || len(a.Backup.BackupCategories) != 1 || a.Backup.BackupCategories[0].Name != "Reading" {
		t.Fatalf("A after delete: changed=%v cats=%v", a.Changed, a.Backup.BackupCategories)
	}
	if mm := urls(a.Backup)["/m"]; mm != nil && len(mm.Categories) != 0 {
		t.Errorf("manga still references deleted category: %v", mm.Categories)
	}
}

func TestMerge_LegacyImportAndV1(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	// a blob written by a pre-v2 server
	legacy := &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/legacy", 3, true, chapterOf("/c", 1, true))}, BackupCategories: []*pb.BackupCategory{{Name: "Old", Uid: 5}}}
	raw, _ := backup.Encode(legacy)
	repo := database.NewSyncRepo(logger.Mock(), db, 3)
	if _, err := repo.SetSyncData(ctx, "key1", raw); err != nil {
		t.Fatal(err)
	}

	// v1 GET keeps working and serves it
	snap, err := svc.GetContent(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := backup.Decode(snap.Data)
	if len(got.BackupManga) != 1 || got.BackupManga[0].Url != "/legacy" {
		t.Fatalf("v1 get after import = %v", got.BackupManga)
	}

	// a v2 client starting fresh gets the imported data and is told to do a full sync next time
	resp := sync2(t, svc, "N", 0, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(2, "/new", 1, true)}})
	if urls(resp.Backup)["/legacy"] == nil {
		t.Errorf("imported manga missing from v2 response: %v", resp.Backup.BackupManga)
	}

	// a v1 client uploads a client-merged blob: merged, not replaced, and If-Match is honoured
	v1blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(3, "/v1only", 1, true)}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{Name: "Old phone"}, "uuid=stale", v1blob); err != ErrPreconditionFailed {
		t.Errorf("stale If-Match err = %v", err)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	etag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{Name: "Old phone"}, snap.ETag, v1blob)
	if err != nil {
		t.Fatal(err)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	if snap.ETag != etag {
		t.Errorf("etag after put %q != get %q", etag, snap.ETag)
	}
	got, _ = backup.Decode(snap.Data)
	if len(got.BackupManga) != 3 {
		t.Errorf("v1 put replaced instead of merged: %d manga", len(got.BackupManga))
	}
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", []byte("garbage")); err == nil {
		t.Error("garbage accepted")
	}

	// history captured the renders; restoring the first one rebuilds the store
	history, _ := svc.ListHistory(ctx, "key1")
	if len(history) < 2 {
		t.Fatalf("history = %v", history)
	}
	oldest := history[len(history)-1]
	if _, err := svc.RestoreHistory(ctx, "key1", oldest.ID); err != nil {
		t.Fatal(err)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	got, _ = backup.Decode(snap.Data)
	if len(got.BackupManga) >= 3 {
		t.Errorf("restore did not rebuild the store: %d manga", len(got.BackupManga))
	}
	// devices with an old cursor get everything again
	resp = sync2(t, svc, "N", resp.Cursor, false, &pb.Backup{})
	if !resp.Changed || len(resp.Backup.BackupManga) == 0 {
		t.Errorf("after restore delta = changed=%v %d manga", resp.Changed, len(resp.Backup.BackupManga))
	}
}

func TestMerge_CursorAheadAsksForFull(t *testing.T) {
	svc, _ := newTestService(t)
	resp := sync2(t, svc, "A", 999, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	if !resp.FullRequested || resp.Cursor != 1 {
		t.Errorf("resp = %+v", resp)
	}
}

func TestMerge_FixtureRoundTrip(t *testing.T) {
	svc, _ := newTestService(t)
	fixture := loadFixture(t)

	resp := sync2(t, svc, "A", 0, true, fixture)
	if len(resp.Backup.BackupManga) != len(fixture.BackupManga) {
		t.Fatalf("manga = %d, want %d", len(resp.Backup.BackupManga), len(fixture.BackupManga))
	}
	again := sync2(t, svc, "A", resp.Cursor, false, fixture)
	if again.Changed || again.Cursor != resp.Cursor {
		t.Errorf("full resend not idempotent: changed=%v cursor %d -> %d", again.Changed, resp.Cursor, again.Cursor)
	}
	snap, err := svc.Snapshot(context.Background(), "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := backup.Decode(snap.Data)
	if !proto.Equal(sortBackup(fixture), sortBackup(got)) {
		t.Error("snapshot differs from the uploaded fixture")
	}
}
