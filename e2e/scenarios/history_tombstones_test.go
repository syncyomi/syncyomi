//go:build e2e

package scenarios

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

// TestS31_DeviceHistoryTombstoneReachesPeer proves that clearing history in the TachiyomiSY UI reaches
// the server as a `history` tombstone carrying the removal's action time, that the peer loses the entry,
// that neither a stale re-upload nor a resync brings it back, and that the client drops the queued key
// once the server acknowledged it.
func TestS31_DeviceHistoryTombstoneReachesPeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	readAt := time.Now().Add(-time.Hour).Truncate(time.Second).UnixMilli()
	staged := historyOnly(t, ctx, srv, stateTitle, stateChapterURL, readAt)
	if _, err := harness.NewSyntheticClient(srv, "e2e-history-seed").Merge(ctx, staged, harness.MergeOptions{}); err != nil {
		t.Fatalf("history merge: %v", err)
	}
	for _, e := range []*harness.Emulator{emuA, emuB} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitLastReadAt(t, ctx, e, stateTitle, stateChapterURL, readAt)
	}

	// Created before the removal, so its delta reports the tombstone A queues.
	peer := harness.NewSyntheticClient(srv, "e2e-history-peer")
	peerDelta(t, ctx, peer)
	histKey := backup.HistoryKey(backup.MangaKey(harness.FixtureSource, "/e2e/E2E Alpha/01"), stateChapterURL)

	before := deviceStateNow(t, ctx, emuA, stateTitle)

	if err := emuA.RunFlow(ctx, harness.FlowPath("clear_history.yaml"), artifactDir, nil); err != nil {
		t.Fatalf("clear_history flow: %v", err)
	}
	awaitQueuedHistory(t, ctx, emuA, stateChapterURL)

	syncViaBroadcast(t, ctx, emuA, srv)

	requireNoHistoryURL(t, snapshotNow(t, ctx, srv), stateTitle, stateChapterURL)
	delta := peerDelta(t, ctx, peer)
	queued := deletionsOfKind(delta, "history")
	if len(queued) != 1 {
		t.Fatalf("the peer delta carries %d history tombstones, want exactly the one A queued: %v",
			len(queued), delta.Deletions)
	}
	if got := queued[0].Key; got != histKey {
		t.Errorf("the peer delta's history tombstone keys %q, want the chapter key %q", got, histKey)
	}
	if now := time.Now().Unix(); queued[0].At <= 0 || queued[0].At > now {
		t.Errorf("the peer delta's history tombstone carries at = %d, want the removal's action time in (0, %d]",
			queued[0].At, now)
	}
	awaitQueueCleared(t, ctx, emuA, stateChapterURL)
	after := deviceStateNow(t, ctx, emuA, stateTitle)
	if len(after.titles) != fixtureAManga {
		t.Errorf("clearing history changed A's library: %d entries, want %d: %v",
			len(after.titles), fixtureAManga, after.titles)
	}
	if after.readCount != before.readCount {
		t.Errorf("clearing history changed A's read chapters of %q: %d -> %d, want unchanged",
			stateTitle, before.readCount, after.readCount)
	}

	syncViaBroadcast(t, ctx, emuB, srv)
	awaitHistoryGone(t, ctx, emuB, stateTitle, stateChapterURL)

	// A stale re-upload of the read must not resurrect it anywhere: its time is older than the tombstone.
	stale := historyOnly(t, ctx, srv, stateTitle, stateChapterURL, readAt)
	if _, err := harness.NewSyntheticClient(srv, "e2e-history-stale").Merge(ctx, stale, harness.MergeOptions{}); err != nil {
		t.Fatalf("stale history merge: %v", err)
	}
	requireNoHistoryURL(t, snapshotNow(t, ctx, srv), stateTitle, stateChapterURL)
	for _, e := range []*harness.Emulator{emuA, emuB, emuA, emuB} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitHistoryGone(t, ctx, e, stateTitle, stateChapterURL)
	}
	requireNoHistoryURL(t, snapshotNow(t, ctx, srv), stateTitle, stateChapterURL)

	// Second phase: both devices queue the same history tombstone before either syncs, so A's deletion
	// arrives at a row B already deleted. It is declined, and still acknowledged: A's queue must clear
	// and the entry must stay gone everywhere.
	const (
		secondTitle = "E2E Alpha 02"
		secondURL   = "/e2e/E2E Alpha/02/ch01"
	)
	secondReadAt := time.Now().Add(-30 * time.Minute).Truncate(time.Second).UnixMilli()
	second := historyOnly(t, ctx, srv, secondTitle, secondURL, secondReadAt)
	if _, err := harness.NewSyntheticClient(srv, "e2e-history-second").Merge(ctx, second, harness.MergeOptions{}); err != nil {
		t.Fatalf("second history merge: %v", err)
	}
	for _, e := range []*harness.Emulator{emuA, emuB} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitLastReadAt(t, ctx, e, secondTitle, secondURL, secondReadAt)
	}
	for _, e := range []*harness.Emulator{emuA, emuB} {
		if err := e.RunFlow(ctx, harness.FlowPath("clear_history.yaml"), artifactDir, nil); err != nil {
			t.Fatalf("clear_history flow on %s: %v", e.AVD, err)
		}
		awaitQueuedHistory(t, ctx, e, secondURL)
	}

	syncViaBroadcast(t, ctx, emuB, srv)
	requireNoHistoryURL(t, snapshotNow(t, ctx, srv), secondTitle, secondURL)
	syncViaBroadcast(t, ctx, emuA, srv)
	requireNoHistoryURL(t, snapshotNow(t, ctx, srv), secondTitle, secondURL)
	awaitQueueCleared(t, ctx, emuA, secondURL)
	for _, e := range []*harness.Emulator{emuA, emuB} {
		awaitHistoryGone(t, ctx, e, secondTitle, secondURL)
	}
	// A third sync must not re-send what was already acknowledged.
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitQueueCleared(t, ctx, emuA, secondURL)
}

// TestS32_TombstoneSecondGranularity proves the client applies the server's rule on whole seconds: a
// read whose millis fall inside the tombstone's second is deleted, and a tombstone older than the read
// it names leaves the read alone on both devices.
func TestS32_TombstoneSecondGranularity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	t0 := time.Now().Add(-time.Hour).Unix()
	// The read sits 900 ms inside t0's second: the client compares read_at/1000 against `at`, so it
	// deletes the row; a comparison on raw milliseconds would keep it and this test would fail.
	readAt := t0*1000 + 900
	seeder := harness.NewSyntheticClient(srv, "e2e-granularity")
	staged := historyOnly(t, ctx, srv, stateTitle, stateChapterURL, readAt)
	if _, err := seeder.Merge(ctx, staged, harness.MergeOptions{}); err != nil {
		t.Fatalf("history merge: %v", err)
	}
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLastReadAt(t, ctx, emuA, stateTitle, stateChapterURL, readAt)

	histKey := backup.HistoryKey(backup.MangaKey(harness.FixtureSource, "/e2e/E2E Alpha/01"), stateChapterURL)
	requireAppliedDeletion(t, ctx, srv, seeder, "a deletion at the second the read falls inside",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "history", Key: histKey, At: t0}}})

	syncViaBroadcast(t, ctx, emuA, srv)
	awaitHistoryGone(t, ctx, emuA, stateTitle, stateChapterURL)

	// The survival half: a deletion older than the read it names never wins, on the server or on either
	// device, and its tombstone never reaches a delta.
	const (
		newerTitle = "E2E Alpha 02"
		newerURL   = "/e2e/E2E Alpha/02/ch01"
	)
	newerAt := (t0 + 120) * 1000
	newerKey := backup.HistoryKey(backup.MangaKey(harness.FixtureSource, "/e2e/E2E Alpha/02"), newerURL)
	newer := historyOnly(t, ctx, srv, newerTitle, newerURL, newerAt)
	if _, err := seeder.Merge(ctx, newer, harness.MergeOptions{}); err != nil {
		t.Fatalf("newer history merge: %v", err)
	}
	for _, e := range []*harness.Emulator{emuA, emuB} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitLastReadAt(t, ctx, e, newerTitle, newerURL, newerAt)
	}

	requireDeclinedDeletion(t, ctx, srv, seeder, "a deletion older than the newer read",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "history", Key: newerKey, At: t0}}})
	if got := snapshotLastRead(snapshotNow(t, ctx, srv), newerTitle, newerURL); got != newerAt {
		t.Errorf("the declined deletion dropped the newer read: server last read = %d, want %d", got, newerAt)
	}
	afterDecline := deltaFromStart(t, ctx, srv, "e2e-granularity-watch")
	requireNoDeletion(t, afterDecline, "history", newerKey)
	if urls := historyURLs(requireManga(t, afterDecline, newerTitle)); !slices.Contains(urls, newerURL) {
		t.Errorf("the declined deletion left the server without the live read %s: %v", newerURL, urls)
	}
	for _, e := range []*harness.Emulator{emuA, emuB} {
		syncViaBroadcast(t, ctx, e, srv)
		awaitLastReadAt(t, ctx, e, newerTitle, newerURL, newerAt)
	}
}

type deviceState struct {
	titles    []string
	readCount int
}

// deviceStateNow reads the device's library size and the read chapters of one title in a single pull.
func deviceStateNow(t *testing.T, ctx context.Context, e *harness.Emulator, title string) deviceState {
	t.Helper()
	dbPath, err := e.PullAppDB(ctx, filepath.Join(artifactDir, harness.SanitizeName(t.Name()), "db-"+e.AVD))
	if err != nil {
		t.Fatalf("pull %s database: %v", e.AVD, err)
	}
	db, err := harness.OpenAppDB(dbPath)
	if err != nil {
		t.Fatalf("open %s database: %v", e.AVD, err)
	}
	defer db.Close()
	titles, err := harness.LibraryTitles(db)
	if err != nil {
		t.Fatalf("read %s library: %v", e.AVD, err)
	}
	read, err := harness.ReadChapterCount(db, title)
	if err != nil {
		t.Fatalf("read %s read count of %q: %v", e.AVD, title, err)
	}
	return deviceState{titles: titles, readCount: read}
}

// requireNoHistoryURL asserts the payload still carries the manga but no longer the history entry: a
// history tombstone removes the read, not the library entry it belongs to.
func requireNoHistoryURL(t *testing.T, b *pb.Backup, title, chapterURL string) {
	t.Helper()
	if urls := historyURLs(requireManga(t, b, title)); slices.Contains(urls, chapterURL) {
		t.Errorf("%q still carries the history entry %s: %v", title, chapterURL, urls)
	}
}

func awaitHistoryGone(t *testing.T, ctx context.Context, e *harness.Emulator, title, chapterURL string) {
	t.Helper()
	pollLiveDB(t, ctx, e, 60*time.Second, "history entry "+chapterURL, func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		got, err := harness.LastReadAt(db, title, chapterURL)
		return err == nil && got == 0
	})
}

// awaitQueuedHistory polls the client's queued-tombstone pref. The client queues the key after the local
// rows are gone, on a background coroutine, so polling the database would fire the sync before the
// tombstone exists and the server would never hear about the removal.
func awaitQueuedHistory(t *testing.T, ctx context.Context, e *harness.Emulator, chapterURL string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if xml, err := e.ReadSyncPrefs(ctx); err == nil && strings.Contains(xml, chapterURL) {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%s never queued a history tombstone for %s", e.AVD, chapterURL)
}

// awaitQueueCleared polls until the queued key is gone: the client clears it on any 2xx, but the
// preference is written on a background coroutine, so a single read right after the sync can still
// show it.
func awaitQueueCleared(t *testing.T, ctx context.Context, e *harness.Emulator, chapterURL string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		xml, err := e.ReadSyncPrefs(ctx)
		if err != nil {
			t.Fatalf("read prefs on %s: %v", e.AVD, err)
		}
		if !strings.Contains(xml, chapterURL) {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("%s still queues the history tombstone for %s after the server answered 2xx: a 2xx must clear the queue",
		e.AVD, chapterURL)
}
