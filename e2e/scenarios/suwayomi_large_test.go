//go:build e2e

package scenarios

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

const (
	suwayomiLargeManga      = 3600
	suwayomiLargeChapters   = 60
	suwayomiLargeNameLength = 200
	suwayomiPushHeapMB      = 320
	suwayomiRestoreHeapMB   = 256
	suwayomiImportHeapMB    = 1024
)

func suwayomiLargeFixture(prefix string) *pb.Backup {
	fixture := harness.FixtureBackup(prefix, suwayomiLargeManga, suwayomiLargeChapters)
	harness.PadFixtureChapterNames(fixture, suwayomiLargeNameLength)
	return fixture
}

func startSuwayomiWithHeap(t *testing.T, ctx context.Context, srv *harness.SyncServer, heapMB int) *harness.Suwayomi {
	t.Helper()
	dir := filepath.Join(artifactDir, harness.SanitizeName(t.Name()))
	suwa, err := harness.StartSuwayomi(ctx, suwayomiJar(t), srv, dir, fmt.Sprintf("-Xmx%dm", heapMB))
	if err != nil {
		t.Fatalf("start suwayomi: %v", err)
	}
	return suwa
}

func TestS22_SuwayomiLargeLibraryFirstPush(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv)
	gz, err := backup.EncodeGzip(suwayomiLargeFixture("S22"))
	if err != nil {
		t.Fatal(err)
	}

	importer := startSuwayomiWithHeap(t, ctx, srv, suwayomiImportHeapMB)
	if err := importer.ImportBackup(ctx, gz); err != nil {
		importer.Stop()
		t.Fatalf("import backup: %v", err)
	}
	assertSuwaLargeLibrary(t, ctx, importer)
	importer.Stop()

	suwa := startSuwayomiWithHeap(t, ctx, srv, suwayomiPushHeapMB)
	t.Cleanup(suwa.Stop)
	assertSuwaLargeLibrary(t, ctx, suwa)

	suwaSyncWithin(t, ctx, suwa, 5*time.Minute)
	if n := suwa.OOMCount(); n != 0 {
		t.Fatalf("suwayomi logged OutOfMemoryError %d times (log: %s)", n, suwa.LogPath)
	}
	assertSnapshotSize(t, ctx, srv, suwayomiLargeManga, suwayomiLargeManga*suwayomiLargeChapters)
}

func TestS23_SuwayomiLargeLibraryRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv)
	seed := harness.NewSyntheticClient(srv, "e2e-s23-seed")
	if _, err := seed.Merge(ctx, suwayomiLargeFixture("S23"), harness.MergeOptions{Full: true, Streamed: true, Gzip: true}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	suwa := startSuwayomiWithHeap(t, ctx, srv, suwayomiRestoreHeapMB)
	t.Cleanup(suwa.Stop)
	suwaSyncWithin(t, ctx, suwa, 5*time.Minute)
	assertSuwaLargeLibrary(t, ctx, suwa)
}

func assertSuwaLargeLibrary(t *testing.T, ctx context.Context, suwa *harness.Suwayomi) {
	t.Helper()
	if n := suwa.OOMCount(); n != 0 {
		t.Fatalf("suwayomi logged OutOfMemoryError %d times (log: %s)", n, suwa.LogPath)
	}
	count, err := suwa.LibraryMangaCount(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != suwayomiLargeManga {
		t.Fatalf("suwayomi has %d manga, want %d", count, suwayomiLargeManga)
	}
	ids, err := suwa.SampleMangaIDs(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		chapters, err := suwa.ChapterCount(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if chapters != suwayomiLargeChapters {
			t.Fatalf("manga %d has %d chapters, want %d", id, chapters, suwayomiLargeChapters)
		}
	}
}
