package sync

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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

func newTestService(t testing.TB) (*service, *database.DB) {
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
	svc.scheduleImport = func(apiKey string) {
		if _, err := svc.ImportPending(context.Background(), apiKey); err != nil {
			t.Errorf("inline import: %v", err)
		}
	}
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

	// A's delta: gets /m with its complete chapter list, c1 now read
	a = sync2(t, svc, "A", a.Cursor, false, &pb.Backup{})
	m := urls(a.Backup)["/m"]
	if !a.Changed || m == nil || len(m.Chapters) != 2 {
		t.Fatalf("A delta = changed=%v %v", a.Changed, a.Backup.BackupManga)
	}
	read := map[string]bool{}
	for _, ch := range m.Chapters {
		read[ch.Url] = ch.Read
	}
	if !read["/c1"] || read["/c2"] {
		t.Errorf("chapter read state = %v", read)
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
	legacyETag, err := repo.SetSyncData(ctx, "key1", raw)
	if err != nil {
		t.Fatal(err)
	}

	// v1 GET serves the legacy blob byte-for-byte under its original etag
	snap, err := svc.GetContent(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snap.Data, raw) {
		t.Fatal("v1 get did not echo the legacy blob verbatim")
	}
	if snap.ETag != *legacyETag {
		t.Errorf("etag after promotion = %q, want the original %q", snap.ETag, *legacyETag)
	}

	// a v2 client starting fresh gets the imported data
	resp := sync2(t, svc, "N", 0, false, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(2, "/new", 1, true)}})
	if urls(resp.Backup)["/legacy"] == nil {
		t.Errorf("imported manga missing from v2 response: %v", resp.Backup.BackupManga)
	}

	// the v2 write made the raw blob stale: v1 GET falls back to a render of everything
	snap, err = svc.GetContent(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(snap.ETag, "seq=") {
		t.Errorf("render fallback etag = %q", snap.ETag)
	}
	got, _ := backup.Decode(snap.Data)
	if len(got.BackupManga) != 2 {
		t.Fatalf("render fallback manga = %v", got.BackupManga)
	}

	// a v1 client uploads its client-merged state: If-Match honoured, then echoed verbatim
	v1blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{
		mangaOf(1, "/legacy", 3, true, chapterOf("/c", 1, true)), mangaOf(2, "/new", 1, true), mangaOf(3, "/v1only", 1, true),
	}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{Name: "Old phone"}, "uuid=stale", v1blob); err != ErrPreconditionFailed {
		t.Errorf("stale If-Match err = %v", err)
	}
	etag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{Name: "Old phone"}, snap.ETag, v1blob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(etag, "uuid=") {
		t.Errorf("v1 put etag = %q", etag)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	if snap.ETag != etag {
		t.Errorf("etag after put %q != get %q", etag, snap.ETag)
	}
	if !bytes.Equal(snap.Data, v1blob) {
		t.Error("v1 get did not echo the uploaded blob verbatim")
	}

	// the upload was also imported for v2 devices
	resp = sync2(t, svc, "N", resp.Cursor, false, &pb.Backup{})
	if urls(resp.Backup)["/v1only"] == nil {
		t.Errorf("v1 upload missing from v2 delta: %v", resp.Backup.BackupManga)
	}

	// garbage is stored and echoed like 1.1.14, never imported
	gtag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", []byte("garbage"))
	if err != nil {
		t.Fatalf("garbage rejected: %v", err)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	if string(snap.Data) != "garbage" || snap.ETag != gtag {
		t.Errorf("garbage echo = %q etag %q want %q", snap.Data, snap.ETag, gtag)
	}
	v2snap, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if v2got, _ := backup.Decode(v2snap.Data); len(v2got.BackupManga) != 3 {
		t.Errorf("garbage leaked into the item store: %v", v2got.BackupManga)
	}

	// history captured the uploads; restoring one serves those bytes verbatim again
	history, _ := svc.ListHistory(ctx, "key1")
	if len(history) < 2 {
		t.Fatalf("history = %v", history)
	}
	var restoreID int
	for _, h := range history {
		if h.ETag == etag {
			restoreID = h.ID
		}
	}
	if restoreID == 0 {
		t.Fatalf("v1 upload not in history: %v", history)
	}
	if _, err := svc.RestoreHistory(ctx, "key1", restoreID); err != nil {
		t.Fatal(err)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	if !bytes.Equal(snap.Data, v1blob) {
		t.Error("restore did not serve the restored payload verbatim")
	}
	// devices with an old cursor get everything again
	resp = sync2(t, svc, "N", resp.Cursor, false, &pb.Backup{})
	if !resp.Changed || len(resp.Backup.BackupManga) == 0 {
		t.Errorf("after restore delta = changed=%v %d manga", resp.Changed, len(resp.Backup.BackupManga))
	}
}

// An undecodable pre-v2 blob must keep being served verbatim forever and never be
// replaced by a render, even as v1 and v2 writes continue on the same key.
func TestV1_UndecodableLegacyPreserved(t *testing.T) {
	svc, db := newTestService(t)
	ctx := context.Background()

	garbage := []byte{0xde, 0xad, 0xbe, 0xef, 0x01}
	repo := database.NewSyncRepo(logger.Mock(), db, 3)
	gtag, err := repo.SetSyncData(ctx, "key1", garbage)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := svc.GetContent(ctx, "key1")
	if err != nil {
		t.Fatalf("v1 get on undecodable legacy blob: %v", err)
	}
	if !bytes.Equal(snap.Data, garbage) || snap.ETag != *gtag {
		t.Fatal("undecodable legacy blob not served verbatim")
	}

	// v2 has nothing: the store is empty, but the blob must survive that
	if _, err := svc.Snapshot(ctx, "key1", 0); err != ErrNoData {
		t.Errorf("v2 snapshot on empty store err = %v, want ErrNoData", err)
	}
	snap, err = svc.GetContent(ctx, "key1")
	if err != nil || !bytes.Equal(snap.Data, garbage) {
		t.Fatalf("legacy blob lost after v2 snapshot: %v", err)
	}

	// a valid v1 upload takes over without having destroyed anything in between
	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	etag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob)
	if err != nil {
		t.Fatal(err)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	if !bytes.Equal(snap.Data, blob) || snap.ETag != etag {
		t.Error("valid upload after undecodable legacy not echoed")
	}
}

// A v1 upload must not disturb what v2 clients see beyond its imported items, and a
// v2 write makes the raw blob stale until the next v1 upload.
func TestV1_V2Interplay(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	sync2(t, svc, "A", 0, true, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/a", 1, true)}})
	before, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}

	// a v1 upload that adds nothing new: the v2 snapshot must be untouched
	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/a", 1, true)}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob); err != nil {
		t.Fatal(err)
	}
	after, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Data, after.Data) || before.ETag != after.ETag {
		t.Error("no-op v1 upload changed the v2 snapshot")
	}

	// the raw blob is current, so v1 gets the echo
	snap, _ := svc.GetContent(ctx, "key1")
	if !bytes.Equal(snap.Data, blob) {
		t.Error("v1 get did not echo while raw is current")
	}

	// a v2 write invalidates it: v1 falls back to the render until the next upload
	sync2(t, svc, "B", 0, true, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(2, "/b", 1, true)}})
	snap, _ = svc.GetContent(ctx, "key1")
	if !strings.HasPrefix(snap.ETag, "seq=") {
		t.Errorf("etag after v2 write = %q, want render fallback", snap.ETag)
	}
	if got, _ := backup.Decode(snap.Data); len(got.BackupManga) != 2 {
		t.Errorf("render fallback = %v", got.BackupManga)
	}
	etag2, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, snap.ETag, blob)
	if err != nil {
		t.Fatal(err)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	if !bytes.Equal(snap.Data, blob) || snap.ETag != etag2 {
		t.Error("echo did not resume after v1 upload")
	}
}

// Restoring an entry that decodes to an empty backup writes no items and cannot bump seq;
// the render must still be rewritten or v2 readers keep the pre-restore content.
func TestRestore_EmptyBackupRefreshesRender(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	empty, _ := backup.Encode(&pb.Backup{})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", empty); err != nil {
		t.Fatal(err)
	}
	full, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", full); err != nil {
		t.Fatal(err)
	}
	if snap, err := svc.Snapshot(ctx, "key1", 0); err != nil {
		t.Fatal(err)
	} else if got, _ := backup.Decode(snap.Data); len(got.BackupManga) != 1 {
		t.Fatalf("pre-restore snapshot = %v", got.BackupManga)
	}

	history, _ := svc.ListHistory(ctx, "key1")
	emptyID := history[len(history)-1].ID
	if _, err := svc.RestoreHistory(ctx, "key1", emptyID); err != nil {
		t.Fatal(err)
	}

	snap, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := backup.Decode(snap.Data); len(got.BackupManga) != 0 {
		t.Errorf("v2 snapshot after empty restore still has %v", got.BackupManga)
	}
	v1snap, _ := svc.GetContent(ctx, "key1")
	if !bytes.Equal(v1snap.Data, empty) {
		t.Error("v1 get did not serve the restored empty payload")
	}
}

// A v1 phone shows up twice — an anonymous upload row and a named event row — unless the
// event tags the named row with the protocol.
func TestV1_EventTagsDeviceProtocol(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob); err != nil {
		t.Fatal(err)
	}
	// the handler records the access (and with it the protocol) after every v1 PUT
	svc.RecordContentAccess(ctx, "key1", domain.DeviceInfo{}, true, ProtocolV1)
	if err := svc.ReportSyncEvent(ctx, "key1", "SYNC_SUCCESS", domain.DeviceInfo{Name: "My Phone"}, ""); err != nil {
		t.Fatal(err)
	}

	devices, err := svc.ListDevices(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]string{}
	for _, d := range devices {
		byID[d.DeviceID] = d.Protocol
	}
	if byID["My Phone"] != "v1" {
		t.Errorf("event device protocol = %q, want v1 (devices: %+v)", byID["My Phone"], devices)
	}

	// a later v2 sync from the same device overrides the tag
	sync2(t, svc, "My Phone", 0, true, &pb.Backup{})
	devices, _ = svc.ListDevices(ctx, "key1")
	for _, d := range devices {
		if d.DeviceID == "My Phone" && d.Protocol != "v2" {
			t.Errorf("protocol after v2 sync = %q", d.Protocol)
		}
	}
}

func TestV1_IfMatch(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})

	// If-Match against an empty key fails like 1.1.14 did
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "uuid=anything", blob); err != ErrPreconditionFailed {
		t.Errorf("If-Match on empty key err = %v", err)
	}
	etag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "uuid=stale", blob); err != ErrPreconditionFailed {
		t.Errorf("stale If-Match err = %v", err)
	}
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, etag, blob); err != nil {
		t.Errorf("matching If-Match err = %v", err)
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

func rawState(t *testing.T, svc *service) (pending bool, current bool, exists bool) {
	t.Helper()
	err := svc.store.Tx(context.Background(), "key1", func(tx domain.SyncStoreTx) error {
		raw, err := tx.RawBlob(context.Background())
		if err != nil {
			return err
		}
		if raw != nil {
			pending, current = raw.Pending, raw.Seq == tx.Seq()
		}
		exists = tx.Exists()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return pending, current, exists
}

func TestV1_PutDefersImport(t *testing.T) {
	svc, _ := newTestService(t)
	svc.scheduleImport = func(string) {}
	ctx := context.Background()

	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true, chapterOf("/c", 1, false))}})
	etag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob)
	if err != nil {
		t.Fatal(err)
	}
	if pending, current, exists := rawState(t, svc); !pending || !current || exists {
		t.Fatalf("after put: pending=%v current=%v exists=%v", pending, current, exists)
	}
	snap, err := svc.GetContent(ctx, "key1")
	if err != nil || !bytes.Equal(snap.Data, blob) || snap.ETag != etag {
		t.Fatalf("echo before import: %v %q", err, snap.ETag)
	}

	if imported, err := svc.ImportPending(ctx, "key1"); err != nil || !imported {
		t.Fatalf("import = %v, %v", imported, err)
	}
	if imported, err := svc.ImportPending(ctx, "key1"); err != nil || imported {
		t.Fatalf("second import = %v, %v", imported, err)
	}
	if pending, current, exists := rawState(t, svc); pending || !current || !exists {
		t.Fatalf("after import: pending=%v current=%v exists=%v", pending, current, exists)
	}
	v2, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := backup.Decode(v2.Data); len(got.BackupManga) != 1 || len(got.BackupManga[0].Chapters) != 1 {
		t.Errorf("snapshot after import = %v", got.BackupManga)
	}
	snap, _ = svc.GetContent(ctx, "key1")
	if !bytes.Equal(snap.Data, blob) || snap.ETag != etag {
		t.Error("echo lost after import")
	}
}

func TestV1_SecondPutSupersedesPending(t *testing.T) {
	svc, _ := newTestService(t)
	svc.scheduleImport = func(string) {}
	ctx := context.Background()

	first, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/a", 1, true)}})
	second, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/b", 1, true)}})
	etag1, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", first)
	if err != nil {
		t.Fatal(err)
	}
	etag2, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, etag1, second)
	if err != nil {
		t.Fatal(err)
	}
	if imported, err := svc.ImportPending(ctx, "key1"); err != nil || !imported {
		t.Fatalf("import = %v, %v", imported, err)
	}
	v2, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := backup.Decode(v2.Data); len(got.BackupManga) != 1 || got.BackupManga[0].Url != "/b" {
		t.Errorf("store after two uploads = %v", got.BackupManga)
	}
	history, _ := svc.ListHistory(ctx, "key1")
	tags := map[string]bool{}
	for _, h := range history {
		tags[h.ETag] = true
	}
	if !tags[etag1] || !tags[etag2] {
		t.Errorf("history missing an upload: %v", history)
	}
}

func TestV1_PendingGarbageClearsFlag(t *testing.T) {
	svc, _ := newTestService(t)
	svc.scheduleImport = func(string) {}
	ctx := context.Background()

	etag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", []byte("garbage"))
	if err != nil {
		t.Fatal(err)
	}
	if imported, err := svc.ImportPending(ctx, "key1"); err != nil || imported {
		t.Fatalf("garbage import = %v, %v", imported, err)
	}
	if pending, current, exists := rawState(t, svc); pending || !current || exists {
		t.Fatalf("after garbage import: pending=%v current=%v exists=%v", pending, current, exists)
	}
	snap, _ := svc.GetContent(ctx, "key1")
	if string(snap.Data) != "garbage" || snap.ETag != etag {
		t.Error("garbage not echoed")
	}
	if _, err := svc.Snapshot(ctx, "key1", 0); err != ErrNoData {
		t.Errorf("snapshot err = %v, want ErrNoData", err)
	}
}

func TestV1_LegacyPromotionIsPending(t *testing.T) {
	svc, db := newTestService(t)
	svc.scheduleImport = func(string) {}
	ctx := context.Background()

	legacy, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/legacy", 1, true)}})
	repo := database.NewSyncRepo(logger.Mock(), db, 3)
	legacyETag, err := repo.SetSyncData(ctx, "key1", legacy)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := svc.GetContent(ctx, "key1")
	if err != nil || !bytes.Equal(snap.Data, legacy) || snap.ETag != *legacyETag {
		t.Fatalf("legacy get: %v", err)
	}
	if pending, current, exists := rawState(t, svc); !pending || !current || exists {
		t.Fatalf("after promotion: pending=%v current=%v exists=%v", pending, current, exists)
	}

	resp := sync2(t, svc, "N", 0, false, &pb.Backup{})
	if !resp.FullRequested || urls(resp.Backup)["/legacy"] == nil {
		t.Errorf("v2 after promotion: full=%v manga=%v", resp.FullRequested, resp.Backup.BackupManga)
	}
	if pending, _, exists := rawState(t, svc); pending || !exists {
		t.Errorf("after v2 merge: pending=%v exists=%v", pending, exists)
	}
}

func TestV2_MergeAfterPendingDoesNotRequestFull(t *testing.T) {
	svc, _ := newTestService(t)
	svc.scheduleImport = func(string) {}
	ctx := context.Background()

	a := sync2(t, svc, "A", 0, true, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/a", 1, true)}})
	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/a", 1, true), mangaOf(1, "/v1", 1, true)}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob); err != nil {
		t.Fatal(err)
	}

	a = sync2(t, svc, "A", a.Cursor, false, &pb.Backup{})
	if a.FullRequested || !a.Changed || urls(a.Backup)["/v1"] == nil {
		t.Errorf("delta after pending upload: full=%v changed=%v manga=%v", a.FullRequested, a.Changed, a.Backup.BackupManga)
	}
	if pending, _, _ := rawState(t, svc); pending {
		t.Error("v2 merge left the upload pending")
	}
}

func TestMerge_DeltaAgainstLargeStoreStaysExact(t *testing.T) {
	svc, _ := newTestService(t)
	lib := &pb.Backup{}
	for i := 0; i < 100; i++ {
		var chapters []*pb.BackupChapter
		for c := 0; c < 60; c++ {
			chapters = append(chapters, chapterOf(fmt.Sprintf("/m%d/c%d", i, c), 1, false))
		}
		lib.BackupManga = append(lib.BackupManga, mangaOf(1, fmt.Sprintf("/m%d", i), 1, true, chapters...))
	}
	a := sync2(t, svc, "A", 0, true, lib)
	b := sync2(t, svc, "B", 0, true, &pb.Backup{})
	if len(b.Backup.BackupManga) != 100 {
		t.Fatalf("B full = %d manga", len(b.Backup.BackupManga))
	}

	changed := &pb.Backup{}
	for i := 0; i < 10; i++ {
		m := proto.Clone(lib.BackupManga[i]).(*pb.BackupManga)
		m.Version = 2
		for _, ch := range m.Chapters {
			ch.Read, ch.Version = true, 2
		}
		changed.BackupManga = append(changed.BackupManga, m)
	}
	sync2(t, svc, "A", a.Cursor, false, changed)
	b = sync2(t, svc, "B", b.Cursor, false, &pb.Backup{})
	if !b.Changed || len(b.Backup.BackupManga) != 10 {
		t.Fatalf("B delta = changed=%v %d manga, want the 10 changed", b.Changed, len(b.Backup.BackupManga))
	}
	for _, m := range b.Backup.BackupManga {
		if m.Version != 2 || len(m.Chapters) != 60 || !m.Chapters[0].Read {
			t.Errorf("delta manga %s = v%d %d chapters read=%v", m.Url, m.Version, len(m.Chapters), m.Chapters[0].Read)
		}
	}
}

func TestV1_BackgroundImport(t *testing.T) {
	svc, _ := newTestService(t)
	svc.scheduleImport = newImporter(5*time.Millisecond, svc.runImport).schedule
	ctx := context.Background()

	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if pending, _, exists := rawState(t, svc); !pending && exists {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background import did not run")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestKeyLocks_RespectContext(t *testing.T) {
	var locks keyLocks
	unlock, err := locks.lock(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := locks.lock(ctx, "k"); err == nil {
		t.Fatal("second lock did not fail on a cancelled context")
	}
	unlock()
	if unlock, err := locks.lock(context.Background(), "k"); err != nil {
		t.Fatal(err)
	} else {
		unlock()
	}
}

func TestV1_GetEchoDoesNotTakeKeyLock(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/m", 1, true)}})
	etag, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob)
	if err != nil {
		t.Fatal(err)
	}

	unlock, err := svc.locks.lock(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	getCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	snap, err := svc.GetContent(getCtx, "key1")
	if err != nil || !bytes.Equal(snap.Data, blob) || snap.ETag != etag {
		t.Fatalf("echo while the key is locked: %v", err)
	}
}

func TestSnapshot_FastPathMatchesLockedPath(t *testing.T) {
	svc, _ := newTestService(t)
	svc.scheduleImport = func(string) {}
	ctx := context.Background()

	sync2(t, svc, "A", 0, true, &pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/a", 1, true)}})
	first, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil || !bytes.Equal(first.Data, second.Data) || first.ETag != second.ETag {
		t.Fatalf("cached snapshot differs: %v", err)
	}

	blob, _ := backup.Encode(&pb.Backup{BackupManga: []*pb.BackupManga{mangaOf(1, "/a", 1, true), mangaOf(1, "/v1", 1, true)}})
	if _, err := svc.PutContent(ctx, "key1", domain.DeviceInfo{}, "", blob); err != nil {
		t.Fatal(err)
	}
	third, err := svc.Snapshot(ctx, "key1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := backup.Decode(third.Data); len(got.BackupManga) != 2 {
		t.Errorf("snapshot after a pending upload = %v", got.BackupManga)
	}
}
