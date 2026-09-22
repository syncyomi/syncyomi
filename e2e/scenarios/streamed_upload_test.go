//go:build e2e

package scenarios

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

const (
	streamedManga    = 3600
	streamedChapters = 60
)

func TestS20_StreamedLargeLibrary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv)
	fixture := harness.FixtureBackup("S20", streamedManga, streamedChapters)

	c := harness.NewSyntheticClient(srv, "e2e-streamed")
	if _, err := c.Merge(ctx, fixture, harness.MergeOptions{Full: true, Streamed: true, Gzip: true}); err != nil {
		t.Fatalf("streamed full merge: %v", err)
	}
	if c.Cursor == 0 {
		t.Fatal("cursor did not advance after the streamed full merge")
	}
	assertSnapshotSize(t, ctx, srv, streamedManga, streamedManga*streamedChapters)

	delta := harness.FixtureBackup("S20", 2, streamedChapters)
	harness.MarkChaptersRead(delta, "S20 01", 3)
	cursor := c.Cursor
	if _, err := c.Merge(ctx, delta, harness.MergeOptions{Streamed: true, Gzip: true}); err != nil {
		t.Fatalf("streamed delta merge: %v", err)
	}
	if c.Cursor <= cursor {
		t.Fatalf("cursor %d did not advance past %d after the streamed delta", c.Cursor, cursor)
	}
	snap := assertSnapshotSize(t, ctx, srv, streamedManga, streamedManga*streamedChapters)
	if got := snapshotReadCount(snap, "S20 01"); got != 3 {
		t.Errorf("S20 01 has %d read chapters after the streamed delta, want 3", got)
	}

	v1srv := startServer(t, stagePortA)
	v1 := harness.NewSyntheticClient(v1srv, "")
	whole, err := backup.Encode(fixture)
	if err != nil {
		t.Fatal(err)
	}
	etag, status, err := v1.PutV1Streamed(ctx, fixture, "", true)
	if err != nil || status != http.StatusOK {
		t.Fatalf("streamed v1 put = %d, %v", status, err)
	}
	data, tag, status, err := v1.GetV1(ctx, "")
	if err != nil || status != http.StatusOK {
		t.Fatalf("v1 get = %d, %v", status, err)
	}
	if tag != etag || !bytes.Equal(data, whole) {
		t.Fatal("streamed v1 upload was not echoed as the whole encoding")
	}
}

func TestS21_DeviceLargeLibraryFirstPush(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	fixture, err := harness.ReadFixture(filepath.Join(harness.RepoRoot(), "internal", "backup", "testdata", "backup_scrubbed.tachibk"))
	if err != nil {
		t.Fatalf("load scrubbed fixture: %v", err)
	}
	titles := harness.SnapshotTitles(fixture)

	stage := startServer(t, stagePortA)
	main := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, main, emuA)

	seed := harness.NewSyntheticClient(stage, "e2e-s21-seed")
	if _, err := seed.Merge(ctx, fixture, harness.MergeOptions{Full: true}); err != nil {
		t.Fatalf("seed staging: %v", err)
	}

	resetApp(t, ctx, emuA, stage)
	syncViaBroadcast(t, ctx, emuA, stage)
	awaitLibraryFor(t, ctx, emuA, len(titles), 5*time.Minute)

	repoint(t, ctx, emuA, main)
	syncViaBroadcast(t, ctx, emuA, main)

	snap, err := main.Snapshot(ctx)
	if err != nil {
		t.Fatalf("server snapshot: %v", err)
	}
	got := harness.SnapshotTitles(snap)
	if !sameStringSet(got, titles) {
		t.Errorf("server has %d titles after the device's first push, want the device's %d", len(got), len(titles))
	}
}

func assertSnapshotSize(t *testing.T, ctx context.Context, srv *harness.SyncServer, mangas, chapters int) *pb.Backup {
	t.Helper()
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	total := 0
	for _, m := range snap.BackupManga {
		total += len(m.Chapters)
	}
	if len(snap.BackupManga) != mangas || total != chapters {
		t.Fatalf("snapshot has %d manga and %d chapters, want %d and %d", len(snap.BackupManga), total, mangas, chapters)
	}
	return snap
}
