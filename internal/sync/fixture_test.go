package sync

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"google.golang.org/protobuf/proto"
)

func loadFixture(t *testing.T) *pb.Backup {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "backup", "testdata", "backup_scrubbed.tachibk"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := backup.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sortBackup(b *pb.Backup) *pb.Backup {
	c := proto.Clone(b).(*pb.Backup)
	sort.SliceStable(c.BackupManga, func(i, j int) bool {
		return backup.MangaKey(c.BackupManga[i].Source, c.BackupManga[i].Url) < backup.MangaKey(c.BackupManga[j].Source, c.BackupManga[j].Url)
	})
	sort.SliceStable(c.BackupCategories, func(i, j int) bool { return c.BackupCategories[i].Order < c.BackupCategories[j].Order })
	for _, m := range c.BackupManga {
		sort.SliceStable(m.Chapters, func(i, j int) bool {
			if m.Chapters[i].SourceOrder != m.Chapters[j].SourceOrder {
				return m.Chapters[i].SourceOrder < m.Chapters[j].SourceOrder
			}
			return m.Chapters[i].Url < m.Chapters[j].Url
		})
		sort.Slice(m.Categories, func(i, j int) bool { return m.Categories[i] < m.Categories[j] })
	}
	sort.SliceStable(c.BackupSources, func(i, j int) bool { return c.BackupSources[i].SourceId < c.BackupSources[j].SourceId })
	sort.SliceStable(c.BackupPreferences, func(i, j int) bool { return c.BackupPreferences[i].Key < c.BackupPreferences[j].Key })
	sort.SliceStable(c.BackupSourcePreferences, func(i, j int) bool {
		return c.BackupSourcePreferences[i].SourceKey < c.BackupSourcePreferences[j].SourceKey
	})
	for _, sp := range c.BackupSourcePreferences {
		sort.SliceStable(sp.Prefs, func(i, j int) bool { return sp.Prefs[i].Key < sp.Prefs[j].Key })
	}
	sort.SliceStable(c.BackupExtensionStores, func(i, j int) bool { return c.BackupExtensionStores[i].IndexUrl < c.BackupExtensionStores[j].IndexUrl })
	sort.SliceStable(c.BackupSavedSearches, func(i, j int) bool {
		return backup.SavedSearchKey(c.BackupSavedSearches[i]) < backup.SavedSearchKey(c.BackupSavedSearches[j])
	})
	return c
}
