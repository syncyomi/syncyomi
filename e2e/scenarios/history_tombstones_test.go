//go:build e2e

package scenarios

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
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

	// Chapters are marked read so the comparison after the clear means "clearing history does not
	// unmark chapters": with nothing read, both counts would be 0 and the check could not fail.
	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_read.yaml"), artifactDir,
		map[string]string{"TITLE": stateTitle}); err != nil {
		t.Fatalf("mark_read flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, stateTitle, fixtureAChapters)
	before := deviceStateNow(t, ctx, emuA, stateTitle)
	if before.readCount != fixtureAChapters {
		t.Fatalf("A has %d read chapters of %q before the clear, want the %d mark_read.yaml marks",
			before.readCount, stateTitle, fixtureAChapters)
	}

	if err := emuA.RunFlow(ctx, harness.FlowPath("clear_history.yaml"), artifactDir, nil); err != nil {
		t.Fatalf("clear_history flow: %v", err)
	}
	queuedAt := awaitQueuedHistory(t, ctx, emuA, stateChapterURL)

	// The clear ran before the sync started, so the client's own action time is at or before this
	// instant while a server receipt stamp can only be later.
	syncStarted := time.Now().Unix()
	if queuedAt > syncStarted {
		t.Errorf("the client queued the removal at %d, after the sync started at %d: that is not the removal's own time",
			queuedAt, syncStarted)
	}
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
	if serverAt := queued[0].At; queuedAt > time.Now().Unix() {
		// The device clock is ahead of this host, so the server clamped the time to its own clock.
		if serverAt <= 0 || serverAt > queuedAt {
			t.Errorf("the peer delta's history tombstone carries at = %d, want the action time %d the client queued, clamped to the server's clock",
				serverAt, queuedAt)
		}
	} else if serverAt != queuedAt {
		t.Errorf("the peer delta's history tombstone carries at = %d, want the %d the client queued with the removal: a receipt stamp would not match",
			serverAt, queuedAt)
	}
	awaitQueueCleared(t, ctx, emuA, stateChapterURL)
	after := deviceStateNow(t, ctx, emuA, stateTitle)
	if len(after.titles) != fixtureAManga {
		t.Errorf("clearing history changed A's library: %d entries, want %d: %v",
			len(after.titles), fixtureAManga, after.titles)
	}
	if after.readCount != before.readCount {
		t.Errorf("clearing history unmarked chapters of %q: %d read before, %d after, want unchanged",
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
	secondKey := backup.HistoryKey(backup.MangaKey(harness.FixtureSource, "/e2e/E2E Alpha/02"), secondURL)
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

	// Primed before B's sync, so its delta is where the tombstone first appears, with the seq and time
	// the store gave it.
	watchBeforeB := harness.NewSyntheticClient(srv, "e2e-history-second-before-b")
	peerDelta(t, ctx, watchBeforeB)
	syncViaBroadcast(t, ctx, emuB, srv)
	requireNoHistoryURL(t, snapshotNow(t, ctx, srv), secondTitle, secondURL)
	applied := deltaDeletion(t, peerDelta(t, ctx, watchBeforeB), "history", secondKey)
	if applied.Seq <= 0 || applied.At <= 0 {
		t.Fatalf("the tombstone B applied carries seq %d at %d, want both recorded: the decline check needs them",
			applied.Seq, applied.At)
	}

	// Primed after B's sync, so only a write A's sync makes can ever appear in it.
	watchBeforeA := harness.NewSyntheticClient(srv, "e2e-history-second-before-a")
	peerDelta(t, ctx, watchBeforeA)

	syncViaBroadcast(t, ctx, emuA, srv)
	requireNoHistoryURL(t, snapshotNow(t, ctx, srv), secondTitle, secondURL)
	// A's deletion names a row B already tombstoned, so the merge drops it before the store: the row
	// keeps the seq and time B's tombstone gave it (the values captured above), and neither watcher is
	// told again. A re-apply would bump the seq and restamp modified_at, which both of them would report.
	requireNoDeletion(t, peerDelta(t, ctx, watchBeforeA), "history", secondKey)
	requireNoDeletion(t, peerDelta(t, ctx, watchBeforeB), "history", secondKey)
	awaitQueueCleared(t, ctx, emuA, secondURL)
	for _, e := range []*harness.Emulator{emuA, emuB} {
		awaitHistoryGone(t, ctx, e, secondTitle, secondURL)
	}
	// Another sync must not put the key back in the queue: this proves it stays absent, not that the
	// sync sent nothing.
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitQueueCleared(t, ctx, emuA, secondURL)
}

// TestS32_TombstoneSecondGranularity proves the client applies the server's rule on whole seconds: a
// read whose millis fall inside the tombstone's second is deleted, while a deletion older than the read
// it names is declined by the server, so no device is ever told and the read survives.
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

	// The survival half: the server declines a deletion older than the read it names, so no device is
	// ever told about it and the read stays where it is. The device checks below fail for a client that
	// dropped the row without being told; they do not exercise its own keep-a-newer-read branch.
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

	// Primed before the deletion: its next delta covers exactly the window the deletion would write in,
	// so a tombstone the server wrongly emitted would show up there.
	watcher := harness.NewSyntheticClient(srv, "e2e-granularity-watch")
	peerDelta(t, ctx, watcher)
	requireDeclinedDeletion(t, ctx, srv, seeder, "a deletion older than the newer read",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "history", Key: newerKey, At: t0}}})
	if got := snapshotLastRead(snapshotNow(t, ctx, srv), newerTitle, newerURL); got != newerAt {
		t.Errorf("the declined deletion dropped the newer read: server last read = %d, want %d", got, newerAt)
	}
	requireNoDeletion(t, peerDelta(t, ctx, watcher), "history", newerKey)
	live := serverRender(t, ctx, srv, "e2e-granularity-render")
	if urls := historyURLs(requireManga(t, live, newerTitle)); !slices.Contains(urls, newerURL) {
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

// awaitHistoryGone waits for the history row to be gone; LastReadAt reports 0 for a missing row.
func awaitHistoryGone(t *testing.T, ctx context.Context, e *harness.Emulator, title, chapterURL string) {
	t.Helper()
	awaitLastReadAt(t, ctx, e, title, chapterURL, 0)
}

// awaitQueuedHistory polls the client's queued-tombstone pref and returns the action time the client
// stamped into it. The client queues the key after the local rows are gone, on a background coroutine,
// so polling the database would fire the sync before the tombstone exists and the server would never
// hear about the removal.
func awaitQueuedHistory(t *testing.T, ctx context.Context, e *harness.Emulator, chapterURL string) int64 {
	t.Helper()
	var (
		queuedAt int64
		sawKey   bool
		lastErr  error
	)
	err := harness.WaitFor(ctx, 60*time.Second, func() bool {
		xml, err := e.ReadSyncPrefs(ctx)
		if err != nil {
			lastErr = err
			return false
		}
		sawKey = strings.Contains(xml, chapterURL)
		at, ok := queuedHistoryActionTime(xml, chapterURL)
		queuedAt = at
		return ok
	})
	if err == nil {
		return queuedAt
	}
	if sawKey {
		t.Fatalf("%s queued a history tombstone for %s without an action time: the client must append the removal's own second to the queued key",
			e.AVD, chapterURL)
	}
	t.Fatalf("%s never queued a history tombstone for %s: %v (last prefs read: %v)", e.AVD, chapterURL, err, lastErr)
	return 0
}

// queuedHistoryActionTime reads the action time the client appends to a queued history tombstone, whose
// entry is `<sourceId>|<mangaUrl>\x1f<chapterUrl>\x1f<seconds>`. The XML writer leaves the separator
// raw, but an escaped form is accepted too so the reader does not depend on the writer's escaping.
func queuedHistoryActionTime(xml, chapterURL string) (int64, bool) {
	i := strings.Index(xml, chapterURL)
	if i < 0 {
		return 0, false
	}
	rest := xml[i+len(chapterURL):]
	for _, sep := range []string{"\x1f", "&#31;", "&#x1f;"} {
		rest = strings.TrimPrefix(rest, sep)
	}
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	at, err := strconv.ParseInt(rest[:end], 10, 64)
	if err != nil || at <= 0 {
		return 0, false
	}
	return at, true
}

// awaitQueueCleared polls until the queued key is gone: the client clears it on any 2xx, but the
// preference is written on a background coroutine, so a single read right after the sync can still
// show it.
func awaitQueueCleared(t *testing.T, ctx context.Context, e *harness.Emulator, chapterURL string) {
	t.Helper()
	var lastErr error
	err := harness.WaitFor(ctx, 30*time.Second, func() bool {
		xml, err := e.ReadSyncPrefs(ctx)
		if err != nil {
			lastErr = err
			return false
		}
		return !strings.Contains(xml, chapterURL)
	})
	if err != nil {
		t.Fatalf("%s still queues the history tombstone for %s after the server answered 2xx: %v (last prefs read: %v)",
			e.AVD, chapterURL, err, lastErr)
	}
}
