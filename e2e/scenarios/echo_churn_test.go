//go:build e2e

package scenarios

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
)

func serverSeq(t *testing.T, ctx context.Context, srv *harness.SyncServer) int64 {
	t.Helper()
	seq, err := srv.Seq(ctx)
	if err != nil {
		t.Fatalf("read server seq: %v", err)
	}
	return seq
}

func deviceWatermark(t *testing.T, ctx context.Context, e *harness.Emulator) int64 {
	t.Helper()
	dbPath, err := e.PullAppDB(ctx, filepath.Join(artifactDir, harness.SanitizeName(t.Name()), "db-watermark-"+e.AVD))
	if err != nil {
		t.Fatal(err)
	}
	db, err := harness.OpenAppDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	v, err := harness.MaxLastModifiedAt(db)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// TestS17_RestoreEchoChurn: applying server data must not look like a local change, or the
// next delta uploads it straight back.
func TestS17_RestoreEchoChurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")
	resetApp(t, ctx, emuA, srv)
	before := time.Now().Add(-time.Minute).Unix()
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	// the seed carries no timestamps, so a recent watermark was stamped by the restore itself
	if wm := deviceWatermark(t, ctx, emuA); wm >= before {
		t.Errorf("restore stamped rows as locally modified: watermark %d (sync began at %d)", wm, before)
	}

	s0 := serverSeq(t, ctx, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	s1 := serverSeq(t, ctx, srv)
	if s1 != s0 {
		t.Errorf("a sync with no changes rewrote the server store (restore echo): seq %d -> %d", s0, s1)
	}
	syncViaBroadcast(t, ctx, emuA, srv)
	s2 := serverSeq(t, ctx, srv)
	if s2 != s1 {
		t.Errorf("churn does not converge: seq %d -> %d on another no-change sync", s1, s2)
	}
}

// TestS18_OneChangeOneWrite: one user change on A costs one server write; B applying it and
// A syncing again write nothing and run no restore pass.
func TestS18_OneChangeOneWrite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)
	// an idle sync each so a first-sync echo, if any, is not charged to the budget
	syncViaBroadcast(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuB, srv)
	s0 := serverSeq(t, ctx, srv)

	const title = "E2E Alpha 01"
	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_read.yaml"), artifactDir,
		map[string]string{"TITLE": title}); err != nil {
		t.Fatalf("mark_read flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, title, fixtureAChapters)
	syncViaBroadcast(t, ctx, emuA, srv)
	s1 := serverSeq(t, ctx, srv)
	if s1 != s0+1 {
		t.Errorf("A's read sync moved seq %d -> %d, want exactly one write", s0, s1)
	}

	syncViaBroadcast(t, ctx, emuB, srv)
	awaitReadCount(t, ctx, emuB, title, fixtureAChapters)
	s2 := serverSeq(t, ctx, srv)
	if s2 != s1 {
		t.Errorf("B's sync rewrote the server while applying A's change: seq %d -> %d", s1, s2)
	}
	syncViaBroadcast(t, ctx, emuB, srv)
	s3 := serverSeq(t, ctx, srv)
	if s3 != s2 {
		t.Errorf("B's no-change sync rewrote the server store (restore echo): seq %d -> %d", s2, s3)
	}
	if writes := s3 - s0; writes != 1 {
		t.Errorf("1 user change caused %d server writes (seq %d -> %d), want 1", writes, s0, s3)
	}

	prevNotify, err := emuA.LastRestoreCompleteNotification(ctx)
	if err != nil {
		t.Fatalf("read notifications on %s: %v", emuA.AVD, err)
	}
	prevWatermark := deviceWatermark(t, ctx, emuA)
	syncViaBroadcast(t, ctx, emuA, srv)
	if s4 := serverSeq(t, ctx, srv); s4 != s3 {
		t.Errorf("A's no-change sync rewrote the server store: seq %d -> %d", s3, s4)
	}
	if got, err := emuA.LastRestoreCompleteNotification(ctx); err != nil {
		t.Fatalf("read notifications on %s: %v", emuA.AVD, err)
	} else if got > prevNotify {
		t.Errorf("A ran a restore pass for content it already had (restore notification %d -> %d)", prevNotify, got)
	}
	if wm := deviceWatermark(t, ctx, emuA); wm != prevWatermark {
		t.Errorf("A's no-change sync stamped rows as locally modified: watermark %d -> %d", prevWatermark, wm)
	}
}

// TestS19_SuwayomiRestoreEcho: the same contract between Suwayomi and Android, both ways.
func TestS19_SuwayomiRestoreEcho(t *testing.T) {
	suwayomiJar(t)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
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
	// Suwayomi's first sync is a one-time converging sync that bumps every version, so the
	// following sync writes the library once; that is not the echo measured here
	suwaSync(t, ctx, suwa)

	s0 := serverSeq(t, ctx, srv)
	suwaSync(t, ctx, suwa)
	suwaSync(t, ctx, suwa)
	if s1 := serverSeq(t, ctx, srv); s1 != s0 {
		t.Errorf("Suwayomi's no-change syncs rewrote the server store: seq %d -> %d", s0, s1)
	}

	// Android -> Suwayomi
	const fromAndroid = "E2E Alpha 01"
	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_read.yaml"), artifactDir,
		map[string]string{"TITLE": fromAndroid}); err != nil {
		t.Fatalf("mark_read flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, fromAndroid, fixtureAChapters)
	syncViaBroadcast(t, ctx, emuA, srv)
	s2 := serverSeq(t, ctx, srv)
	suwaSync(t, ctx, suwa)
	if got := suwaReadCount(t, ctx, suwa, fromAndroid); got != fixtureAChapters {
		t.Errorf("suwayomi read count for %q = %d, want %d", fromAndroid, got, fixtureAChapters)
	}
	suwaSync(t, ctx, suwa)
	if s3 := serverSeq(t, ctx, srv); s3 != s2 {
		t.Errorf("Suwayomi wrote Android's change back (restore echo): seq %d -> %d", s2, s3)
	}

	// Suwayomi -> Android
	const fromSuwayomi = "E2E Alpha 05"
	if err := suwa.MarkChaptersRead(ctx, fromSuwayomi); err != nil {
		t.Fatal(err)
	}
	suwaSync(t, ctx, suwa)
	s4 := serverSeq(t, ctx, srv)
	if s4 != s2+1 {
		t.Errorf("Suwayomi's read sync moved seq %d -> %d, want exactly one write", s2, s4)
	}
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitReadCount(t, ctx, emuA, fromSuwayomi, fixtureAChapters)
	syncViaBroadcast(t, ctx, emuA, srv)
	if s5 := serverSeq(t, ctx, srv); s5 != s4 {
		t.Errorf("Android wrote Suwayomi's change back (restore echo): seq %d -> %d", s4, s5)
	}
}

func suwaReadCount(t *testing.T, ctx context.Context, suwa *harness.Suwayomi, title string) int {
	t.Helper()
	library, err := suwa.Library(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range library {
		if m.Title == title {
			return m.ReadCount
		}
	}
	t.Fatalf("suwayomi has no manga %q", title)
	return 0
}
