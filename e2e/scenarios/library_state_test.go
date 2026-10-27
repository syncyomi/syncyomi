//go:build e2e

package scenarios

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"google.golang.org/protobuf/proto"
)

const (
	stateTitle      = "E2E Alpha 01"
	stateChapterURL = "/e2e/E2E Alpha/01/ch01"
)

func TestS24_HistoryWithoutMangaChangePropagates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	readAt := time.Now().Add(-time.Hour).Truncate(time.Second).UnixMilli()
	reread := historyOnly(t, ctx, srv, stateTitle, stateChapterURL, readAt)
	if _, err := harness.NewSyntheticClient(srv, "e2e-history").Merge(ctx, reread, harness.MergeOptions{}); err != nil {
		t.Fatalf("history merge: %v", err)
	}

	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotLastRead(snap, stateTitle, stateChapterURL); got != readAt {
		t.Errorf("server last read of %s = %d, want %d", stateChapterURL, got, readAt)
	}
	for _, e := range []*harness.Emulator{emuA, emuB} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitLastReadAt(t, ctx, e, stateTitle, stateChapterURL, readAt)
	}
}

func TestS25_LibraryRemovalPropagates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	if err := emuA.RunFlow(ctx, harness.FlowPath("remove_from_library.yaml"), artifactDir,
		map[string]string{"TITLE": stateTitle}); err != nil {
		t.Fatalf("remove_from_library flow: %v", err)
	}
	awaitLibrary(t, ctx, emuA, fixtureAManga-1)

	syncViaBroadcast(t, ctx, emuA, srv)
	assertSnapshotRemoved(t, ctx, srv, stateTitle)
	syncViaBroadcast(t, ctx, emuB, srv)
	awaitLibrary(t, ctx, emuB, fixtureAManga-1)

	stale := harness.FixtureBackup("E2E Alpha", fixtureAManga, fixtureAChapters)
	if _, err := harness.NewSyntheticClient(srv, "e2e-stale").Merge(ctx, stale, harness.MergeOptions{Full: true}); err != nil {
		t.Fatalf("stale full upload: %v", err)
	}
	for _, e := range []*harness.Emulator{emuA, emuB, emuA} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitLibrary(t, ctx, e, fixtureAManga-1)
	}
	assertSnapshotRemoved(t, ctx, srv, stateTitle)
}

func TestS26_UnreadPropagates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_read.yaml"), artifactDir,
		map[string]string{"TITLE": stateTitle}); err != nil {
		t.Fatalf("mark_read flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, stateTitle, fixtureAChapters)
	syncViaBroadcast(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuB, srv)
	awaitReadCount(t, ctx, emuB, stateTitle, fixtureAChapters)

	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_unread.yaml"), artifactDir,
		map[string]string{"TITLE": stateTitle}); err != nil {
		t.Fatalf("mark_unread flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, stateTitle, 0)
	syncViaBroadcast(t, ctx, emuA, srv)

	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotReadCount(snap, stateTitle); got != 0 {
		t.Errorf("server has %d read chapters for %q after unread, want 0", got, stateTitle)
	}
	syncViaBroadcast(t, ctx, emuB, srv)
	awaitReadCount(t, ctx, emuB, stateTitle, 0)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitReadCount(t, ctx, emuA, stateTitle, 0)
}

func TestS27_SuwayomiLibraryRemovalReachesAndroid(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")
	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	suwa := startSuwayomi(t, ctx, srv)
	suwaSync(t, ctx, suwa)
	assertSuwaLibrarySize(t, ctx, suwa, fixtureAManga)

	if err := suwa.RemoveFromLibrary(ctx, stateTitle); err != nil {
		t.Fatal(err)
	}
	suwaSync(t, ctx, suwa)
	assertSnapshotRemoved(t, ctx, srv, stateTitle)

	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga-1)
	suwaSync(t, ctx, suwa)
	assertSuwaLibrarySize(t, ctx, suwa, fixtureAManga-1)
}

func historyOnly(t *testing.T, ctx context.Context, srv *harness.SyncServer, title, chapterURL string, readAt int64) *pb.Backup {
	t.Helper()
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(snap.BackupManga, func(m *pb.BackupManga) bool { return m.Title == title })
	if i < 0 {
		t.Fatalf("%q not on the server", title)
	}
	m := proto.Clone(snap.BackupManga[i]).(*pb.BackupManga)
	m.Chapters = nil
	m.History = []*pb.BackupHistory{{Url: chapterURL, LastRead: readAt}}
	return &pb.Backup{BackupManga: []*pb.BackupManga{m}, BackupCategories: snap.BackupCategories}
}

func snapshotLastRead(snap *pb.Backup, title, chapterURL string) int64 {
	for _, m := range snap.BackupManga {
		if m.Title != title {
			continue
		}
		for _, h := range m.History {
			if h.Url == chapterURL {
				return h.LastRead
			}
		}
	}
	return 0
}

func assertSnapshotRemoved(t *testing.T, ctx context.Context, srv *harness.SyncServer, title string) {
	t.Helper()
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range snap.BackupManga {
		if m.Title == title && backup.IsFavorite(m) {
			t.Errorf("server still has %q in the library (version %d)", title, m.Version)
		}
	}
}

func awaitLastReadAt(t *testing.T, ctx context.Context, e *harness.Emulator, title, chapterURL string, want int64) {
	t.Helper()
	pollLiveDB(t, ctx, e, 60*time.Second, "last read of "+chapterURL, func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		got, err := harness.LastReadAt(db, title, chapterURL)
		return err == nil && got == want
	})
}
