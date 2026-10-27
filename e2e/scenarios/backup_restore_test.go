//go:build e2e

package scenarios

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

const (
	mihonPrefix  = "E2E Mihon"
	mihonTitle   = "E2E Mihon 01"
	mihonChapter = "/e2e/E2E Mihon/01/ch01"
	mihonReadAt  = int64(1_760_000_000_000)
)

func TestS28_FileRestoreOfMihonBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	resetApp(t, ctx, emuA, srv)

	path := filepath.Join(artifactDir, harness.SanitizeName(t.Name()), "mihon.tachibk")
	if err := harness.WriteFixture(path, mihonBackup()); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		restoreBackupFile(t, ctx, emuA, path)
		awaitLibrary(t, ctx, emuA, fixtureAManga)
		awaitReadCount(t, ctx, emuA, mihonTitle, 2)
		awaitLastReadAt(t, ctx, emuA, mihonTitle, mihonChapter, mihonReadAt)
	}

	titles, err := libraryTitles(t, ctx, emuA)
	if err != nil {
		t.Fatal(err)
	}
	if !sameStringSet(titles, harness.FixtureTitles(mihonPrefix, fixtureAManga)) {
		t.Errorf("library after restoring twice = %v", titles)
	}
}

// mihonBackup carries only fields Mihon writes: no SY metadata, versions or sync timestamps, plus a read
// entry outside the library as Mihon includes with "non-library entries".
func mihonBackup() *pb.Backup {
	b := harness.FixtureBackup(mihonPrefix, fixtureAManga, fixtureAChapters)
	for _, m := range b.BackupManga {
		if m.Title != mihonTitle {
			continue
		}
		m.Chapters[0].Read, m.Chapters[1].Read = true, true
		m.History = []*pb.BackupHistory{{Url: mihonChapter, LastRead: mihonReadAt, ReadDuration: 60_000}}
	}
	outside := harness.FixtureBackup("E2E Outside", 1, 1).BackupManga[0]
	backup.SetFavorite(outside, false)
	outside.Categories = nil
	outside.Chapters[0].Read = true
	b.BackupManga = append(b.BackupManga, outside)
	return b
}

func restoreBackupFile(t *testing.T, ctx context.Context, e *harness.Emulator, localPath string) {
	t.Helper()
	remote, err := e.PushBackup(ctx, localPath)
	if err != nil {
		t.Fatalf("push backup: %v", err)
	}
	if err := e.OpenBackupFile(ctx, remote); err != nil {
		t.Fatalf("open backup: %v", err)
	}
	if err := e.RunFlow(ctx, harness.FlowPath("restore_backup.yaml"), artifactDir, nil); err != nil {
		t.Fatalf("restore_backup flow: %v", err)
	}
}
