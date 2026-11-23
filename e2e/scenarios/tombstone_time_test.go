//go:build e2e

package scenarios

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

// TestS30_TombstoneActionTime pins the server's reading of a deletion's action time (`at`, Unix
// seconds): a deletion applies to a row it is not older than, so a delayed deletion can never undo a
// read made after it, while an equal second still applies because the comparison is whole seconds.
func TestS30_TombstoneActionTime(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv)

	const prefix = "E2E Tomb Time"
	seeder := harness.NewSyntheticClient(srv, "e2e-at-seed")
	if _, err := seeder.Merge(ctx, harness.FixtureBackup(prefix, 2, 3), harness.MergeOptions{Full: true}); err != nil {
		t.Fatalf("seed merge: %v", err)
	}

	title := prefix + " 01"
	chapterURL := "/e2e/" + prefix + "/01/ch01"
	histKey := backup.HistoryKey(backup.MangaKey(harness.FixtureSource, "/e2e/"+prefix+"/01"), chapterURL)

	// Staged on a whole second in the past so the deletion boundaries below are exact: readAt-600 is
	// unambiguously older than the row, readAt is its own second.
	readAt := time.Now().Add(-time.Hour).Unix()
	staged := historyOnly(t, ctx, srv, title, chapterURL, readAt*1000)
	if _, err := seeder.Merge(ctx, staged, harness.MergeOptions{}); err != nil {
		t.Fatalf("history merge: %v", err)
	}
	// The row is on the server carrying the millis the client sent; the second the store compares
	// deletions against is that value truncated, which S32 covers from the client's side.
	if got := snapshotLastRead(snapshotNow(t, ctx, srv), title, chapterURL); got != readAt*1000 {
		t.Fatalf("server last read of %s = %d, want the %d the client sent", chapterURL, got, readAt*1000)
	}

	// The peer is created before the removal, so its delta reports the tombstones: a delta only carries
	// what changed after the client's cursor.
	peer := harness.NewSyntheticClient(srv, "e2e-at-peer")
	before := peerDelta(t, ctx, peer)
	if got := len(before.Deletions); got != 0 {
		t.Errorf("the delta before any deletion carries %d deletions, want none: %v", got, before.Deletions)
	}
	if urls := historyURLs(requireManga(t, before, title)); !slices.Contains(urls, chapterURL) {
		t.Errorf("the delta before any deletion omits the live read %s: %v", chapterURL, urls)
	}

	// A deletion older than the read must not win, however late it arrives.
	requireDeclinedDeletion(t, ctx, srv, seeder, "a deletion older than the read",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "history", Key: histKey, At: readAt - 600}}})
	if got := snapshotLastRead(snapshotNow(t, ctx, srv), title, chapterURL); got != readAt*1000 {
		t.Errorf("a declined deletion dropped the read: server last read = %d, want the unchanged %d", got, readAt*1000)
	}
	requireNoDeletion(t, peerDelta(t, ctx, peer), "history", histKey)
	// The peer's cursor has passed the read, so its delta cannot show the item again: the live read is
	// read back through a full render, which lists what is live and no tombstones.
	afterDecline := serverRender(t, ctx, srv, "e2e-at-watch-decline")
	if urls := historyURLs(requireManga(t, afterDecline, title)); !slices.Contains(urls, chapterURL) {
		t.Errorf("a declined deletion left the server without the live read %s: %v", chapterURL, urls)
	}

	// The boundary: a deletion at the read's own second applies.
	requireAppliedDeletion(t, ctx, srv, seeder, "a deletion at the read's own second",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "history", Key: histKey, At: readAt}}})
	if got := snapshotLastRead(snapshotNow(t, ctx, srv), title, chapterURL); got != 0 {
		t.Errorf("the read survived a deletion at its own second: last read = %d, want it gone", got)
	}
	delta := peerDelta(t, ctx, peer)
	if got := len(deletionsOfKind(delta, "history")); got != 1 {
		t.Fatalf("the equal-second deletion put %d history tombstones in the peer delta, want exactly 1: %v",
			got, delta.Deletions)
	}
	if got := deltaDeletion(t, delta, "history", histKey).At; got != readAt {
		t.Errorf("the tombstone carries at = %d, want the deletion's action time %d: field 4 is the action time, not the receipt time",
			got, readAt)
	}

	// Deleting the same key again changes nothing, but is still acknowledged so a client may clear its
	// queue instead of retrying the tombstone forever.
	requireDeclinedDeletion(t, ctx, srv, seeder, "the same key deleted again",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "history", Key: histKey, At: readAt}}})
	requireNoDeletion(t, peerDelta(t, ctx, peer), "history", histKey)

	// A legacy-shaped deletion carries no action time, so the server stamps it with its own receipt
	// time: the delta reports that time, never zero.
	legacyURL := "/e2e/" + prefix + "/02/ch01"
	legacyKey := backup.ChapterKey(backup.MangaKey(harness.FixtureSource, "/e2e/"+prefix+"/02"), legacyURL)
	receiptBefore := time.Now().Unix()
	requireAppliedDeletion(t, ctx, srv, seeder, "a legacy chapter deletion without an action time",
		harness.DeletionsOptions{DeletedChapters: []string{legacyKey}})
	receiptAfter := time.Now().Unix()
	legacyAt := deltaDeletion(t, peerDelta(t, ctx, peer), "chapter", legacyKey).At
	if legacyAt < receiptBefore || legacyAt > receiptAfter {
		t.Errorf("the legacy tombstone carries at = %d, want the receipt time in [%d, %d]",
			legacyAt, receiptBefore, receiptAfter)
	}
}

// requireDeclinedDeletion asserts the server acknowledged a tombstone without applying it: a deletion
// the server declines is still a 200 with the item acknowledged, so a client may clear its queue.
func requireDeclinedDeletion(t *testing.T, ctx context.Context, srv *harness.SyncServer, c *harness.SyntheticClient, what string, opts harness.DeletionsOptions) {
	t.Helper()
	before := seqNow(t, ctx, srv)
	res, err := c.Deletions(ctx, opts)
	if err != nil {
		t.Fatalf("%s deletions: %v", what, err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("%s deletions: status = %d, want 200 (body %s)", what, res.StatusCode, res.Body)
	}
	if res.Acknowledged != 1 || res.Applied != 0 {
		t.Errorf("%s deletions = ack %d applied %d, want ack 1 applied 0: a declined deletion is acknowledged but not applied",
			what, res.Acknowledged, res.Applied)
	}
	if got := seqNow(t, ctx, srv); got != before {
		t.Errorf("%s advanced seq %d -> %d, want it unchanged: nothing was deleted", what, before, got)
	}
}

// requireNoDeletion is the negative of requireDeletion: the delta must carry no tombstone for the key.
func requireNoDeletion(t *testing.T, b *pb.Backup, kind, key string) {
	t.Helper()
	for _, d := range b.Deletions {
		if d.Kind == kind && d.Key == key {
			t.Fatalf("the delta carries a %s tombstone for %s (at %d), want none: deletions = %v",
				kind, key, d.At, b.Deletions)
		}
	}
}

// deltaDeletion returns the tombstone a delta carries for the key.
func deltaDeletion(t *testing.T, b *pb.Backup, kind, key string) *pb.BackupDeletedItem {
	t.Helper()
	for _, d := range b.Deletions {
		if d.Kind == kind && d.Key == key {
			return d
		}
	}
	t.Fatalf("the delta omits the %s tombstone %s: deletions = %v", kind, key, b.Deletions)
	return nil
}

func deletionsOfKind(b *pb.Backup, kind string) []*pb.BackupDeletedItem {
	var out []*pb.BackupDeletedItem
	for _, d := range b.Deletions {
		if d.Kind == kind {
			out = append(out, d)
		}
	}
	return out
}

// serverRender reads the whole server state through a client that has never synced: a cursor of 0 makes
// the server render everything instead of a delta, so this is what still shows an item written before
// any other client's cursor. That render never carries tombstones, so it proves nothing about
// deletions — an incremental delta from a client primed at the right moment does.
func serverRender(t *testing.T, ctx context.Context, srv *harness.SyncServer, deviceID string) *pb.Backup {
	t.Helper()
	c := harness.NewSyntheticClient(srv, deviceID)
	b, err := c.Merge(ctx, nil, harness.MergeOptions{})
	if err != nil {
		t.Fatalf("full render from an unsynced cursor: %v", err)
	}
	return b
}
