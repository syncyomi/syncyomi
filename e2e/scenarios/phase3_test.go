//go:build e2e

package scenarios

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

// pairBoth gives A and B the seeded Alpha library on a fresh server.
func pairBoth(t *testing.T, ctx context.Context, srv *harness.SyncServer) {
	t.Helper()
	seedServer(t, ctx, srv, "E2E Alpha")
	resetApp(t, ctx, emuA, srv)
	resetApp(t, ctx, emuB, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)
	syncViaBroadcast(t, ctx, emuB, srv)
	awaitLibrary(t, ctx, emuB, fixtureAManga)
}

// TestS3_ReadProgressPropagation: A marks a manga read through the real UI, the
// state reaches the server and B, and re-syncing A does not regress it.
func TestS3_ReadProgressPropagation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	const title = "E2E Alpha 01"
	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_read.yaml"), artifactDir,
		map[string]string{"TITLE": title}); err != nil {
		t.Fatalf("mark_read flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, title, fixtureAChapters)

	syncViaBroadcast(t, ctx, emuA, srv)
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotReadCount(snap, title); got != fixtureAChapters {
		t.Errorf("server has %d read chapters for %q, want %d", got, title, fixtureAChapters)
	}

	syncViaBroadcast(t, ctx, emuB, srv)
	awaitReadCount(t, ctx, emuB, title, fixtureAChapters)

	syncViaBroadcast(t, ctx, emuA, srv)
	awaitReadCount(t, ctx, emuA, title, fixtureAChapters)
}

// TestS4_CategoryDeletionTombstone: a deleted-category tombstone removes the
// category on every device and it does not resurrect on later syncs.
func TestS4_CategoryDeletionTombstone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	c := harness.NewSyntheticClient(srv, "e2e-tombstone")
	uid := harness.FixtureCategoryUID("E2E Alpha")
	if _, err := c.Merge(ctx, nil, harness.MergeOptions{DeletedCategories: []int64{uid}}); err != nil {
		t.Fatalf("tombstone merge: %v", err)
	}

	for _, e := range []*harness.Emulator{emuA, emuB} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitCategoryGone(t, ctx, e, "E2E Alpha")
	}
	// The category lived only on the devices' own backups now; a re-sync must
	// not bring it back.
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitCategoryGone(t, ctx, emuA, "E2E Alpha")
	for _, e := range []*harness.Emulator{emuA, emuB} {
		titles, err := libraryTitles(t, ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		if len(titles) != fixtureAManga {
			t.Errorf("%s lost manga with the category: %d left, want %d", e.AVD, len(titles), fixtureAManga)
		}
	}
}

// TestS5_ConflictBothEditsSurvive: A marks chapters read while another device
// recategorizes the same manga with a higher version; after syncing, A holds
// both edits and so does the server.
func TestS5_ConflictBothEditsSurvive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")
	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	const title = "E2E Alpha 02"
	// Local edit on A, not yet synced.
	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_read.yaml"), artifactDir,
		map[string]string{"TITLE": title}); err != nil {
		t.Fatalf("mark_read flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, title, fixtureAChapters)

	// Concurrent remote edit: same manga moved to a new category, higher version.
	remote := harness.FixtureBackup("E2E Alpha", fixtureAManga, fixtureAChapters)
	conflictCat := &pb.BackupCategory{Name: "E2E Conflict", Order: 1, Id: 2, Uid: harness.FixtureCategoryUID("E2E Conflict")}
	remote.BackupCategories = append(remote.BackupCategories, conflictCat)
	var moved *pb.BackupManga
	for _, m := range remote.BackupManga {
		if m.Title == title {
			moved = m
		}
	}
	moved.Categories = []int64{conflictCat.Order}
	moved.Version = 2
	remote.BackupManga = []*pb.BackupManga{moved} // delta: just the contested manga
	c := harness.NewSyntheticClient(srv, "e2e-conflict")
	if _, err := c.Merge(ctx, remote, harness.MergeOptions{}); err != nil {
		t.Fatalf("conflict merge: %v", err)
	}

	syncViaBroadcast(t, ctx, emuA, srv)
	awaitMangaInCategory(t, ctx, emuA, title, "E2E Conflict")
	awaitReadCount(t, ctx, emuA, title, fixtureAChapters)

	// One more sync so A pushes its merged state; server must hold both edits.
	syncViaBroadcast(t, ctx, emuA, srv)
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotReadCount(snap, title); got != fixtureAChapters {
		t.Errorf("server read count for %q = %d, want %d (read state regressed)", title, got, fixtureAChapters)
	}
	if !snapshotMangaInCategory(snap, title, "E2E Conflict") {
		t.Errorf("server lost the category move for %q", title)
	}
}

// TestS6_StaleCursorConvergence: the server advances several generations while
// A is away; A's next delta sync converges without duplicates.
func TestS6_StaleCursorConvergence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")
	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	for _, prefix := range []string{"E2E Gamma", "E2E Delta"} {
		seedServer(t, ctx, srv, prefix)
	}

	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, 3*fixtureAManga)
	titles, err := libraryTitles(t, ctx, emuA)
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 3*fixtureAManga {
		t.Errorf("library has %d entries, want %d", len(titles), 3*fixtureAManga)
	}
	seen := map[string]bool{}
	for _, title := range titles {
		if seen[title] {
			t.Errorf("duplicate library entry %q", title)
		}
		seen[title] = true
	}
}

// TestS8_DedupeIdempotencySoak: a realistic scrubbed library syncs down, then
// repeated syncs leave every row count and the server snapshot stable.
func TestS8_DedupeIdempotencySoak(t *testing.T) {
	if testing.Short() {
		t.Skip("soak test skipped with -short")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	fixture, err := harness.ReadFixture(filepath.Join(harness.RepoRoot(), "internal", "backup", "testdata", "backup_scrubbed.tachibk"))
	if err != nil {
		t.Fatalf("load scrubbed fixture: %v", err)
	}
	favorites := len(harness.SnapshotTitles(fixture))

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	c := harness.NewSyntheticClient(srv, "e2e-soak-seed")
	if _, err := c.Merge(ctx, fixture, harness.MergeOptions{Full: true}); err != nil {
		t.Fatalf("seed scrubbed fixture: %v", err)
	}

	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibraryFor(t, ctx, emuA, favorites, 5*time.Minute)

	var prevMangas, prevChapters, prevCategories int
	var prevSnapshot []string
	for i := 0; i < 3; i++ {
		syncViaBroadcast(t, ctx, emuA, srv)
		awaitLibraryFor(t, ctx, emuA, favorites, 2*time.Minute)

		dbPath, err := emuA.PullAppDB(ctx, filepath.Join(artifactDir, harness.SanitizeName(t.Name()), "db-soak"))
		if err != nil {
			t.Fatal(err)
		}
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		mangas, chapters, categories, err := harness.TableCounts(db)
		db.Close()
		if err != nil {
			t.Fatal(err)
		}
		snap, err := srv.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		snapTitles := harness.SnapshotTitles(snap)

		if i > 0 {
			if mangas != prevMangas || chapters != prevChapters || categories != prevCategories {
				t.Errorf("pass %d: row counts drifted: mangas %d→%d chapters %d→%d categories %d→%d",
					i, prevMangas, mangas, prevChapters, chapters, prevCategories, categories)
			}
			if !slices.Equal(snapTitles, prevSnapshot) {
				t.Errorf("pass %d: server snapshot titles drifted (%d → %d entries)", i, len(prevSnapshot), len(snapTitles))
			}
		}
		prevMangas, prevChapters, prevCategories, prevSnapshot = mangas, chapters, categories, snapTitles
	}
}

// --- polling helpers on the live app DB ---

func awaitReadCount(t *testing.T, ctx context.Context, e *harness.Emulator, title string, want int) {
	t.Helper()
	pollLiveDB(t, ctx, e, 60*time.Second, "read count", func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		n, err := harness.ReadChapterCount(db, title)
		return err == nil && n == want
	})
}

func awaitCategoryGone(t *testing.T, ctx context.Context, e *harness.Emulator, name string) {
	t.Helper()
	pollLiveDB(t, ctx, e, 60*time.Second, "category removal", func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		names, err := harness.CategoryNames(db)
		return err == nil && !slices.Contains(names, name)
	})
}

func awaitMangaInCategory(t *testing.T, ctx context.Context, e *harness.Emulator, title, category string) {
	t.Helper()
	pollLiveDB(t, ctx, e, 60*time.Second, "category move", func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		names, err := harness.MangaCategoryNames(db, title)
		return err == nil && slices.Contains(names, category)
	})
}

func awaitLibraryFor(t *testing.T, ctx context.Context, e *harness.Emulator, want int, timeout time.Duration) {
	t.Helper()
	pollLiveDB(t, ctx, e, timeout, "library size", func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		titles, err := harness.LibraryTitles(db)
		return err == nil && len(titles) == want
	})
}

func pollLiveDB(t *testing.T, ctx context.Context, e *harness.Emulator, timeout time.Duration, what string, ok func(dbPath string) bool) {
	t.Helper()
	dir := filepath.Join(artifactDir, harness.SanitizeName(t.Name()), "db-live-"+e.AVD)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if dbPath, err := e.PullAppDBLive(ctx, dir); err == nil && ok(dbPath) {
			return
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("%s: %s never reached expected state within %s", e.AVD, what, timeout)
}

// --- server snapshot helpers ---

func snapshotReadCount(snap *pb.Backup, title string) int {
	for _, m := range snap.BackupManga {
		if m.Title != title || !backup.IsFavorite(m) {
			continue
		}
		n := 0
		for _, ch := range m.Chapters {
			if ch.Read {
				n++
			}
		}
		return n
	}
	return -1
}

func snapshotMangaInCategory(snap *pb.Backup, title, category string) bool {
	orderByName := map[string]int64{}
	for _, c := range snap.BackupCategories {
		orderByName[c.Name] = c.Order
	}
	want, found := orderByName[category]
	if !found {
		return false
	}
	for _, m := range snap.BackupManga {
		if m.Title == title && backup.IsFavorite(m) {
			return slices.Contains(m.Categories, want)
		}
	}
	return false
}
