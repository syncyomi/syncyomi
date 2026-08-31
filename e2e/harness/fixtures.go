package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

// FixtureSource is a fake source id; restores work without it being installed.
const FixtureSource int64 = 9999999999

const fixtureEpochMillis int64 = 1756600000000 // fixed so generated fixtures are byte-stable

// FixtureBackup builds a backup with fully known content: manga titled
// "<prefix> 01".."<prefix> NN" with chapterCount chapters each, in one category
// named after the prefix.
func FixtureBackup(prefix string, mangaCount, chapterCount int) *pb.Backup {
	category := &pb.BackupCategory{
		Name:  prefix,
		Order: 0,
		Id:    1,
		Uid:   hashUID(prefix),
	}
	b := &pb.Backup{
		BackupCategories: []*pb.BackupCategory{category},
		BackupSources: []*pb.BackupSource{
			{Name: "E2E Fixture Source", SourceId: FixtureSource},
		},
	}
	for i := 1; i <= mangaCount; i++ {
		title := fmt.Sprintf("%s %02d", prefix, i)
		m := &pb.BackupManga{
			Source:      FixtureSource,
			Url:         fmt.Sprintf("/e2e/%s/%02d", prefix, i),
			Title:       title,
			Status:      1,
			DateAdded:   fixtureEpochMillis,
			Categories:  []int64{category.Order},
			Initialized: true,
		}
		backup.SetFavorite(m, true)
		for c := 1; c <= chapterCount; c++ {
			m.Chapters = append(m.Chapters, &pb.BackupChapter{
				Url:           fmt.Sprintf("/e2e/%s/%02d/ch%02d", prefix, i, c),
				Name:          fmt.Sprintf("Chapter %d", c),
				ChapterNumber: float32(c),
				SourceOrder:   int64(chapterCount - c),
				DateFetch:     fixtureEpochMillis,
			})
		}
		b.BackupManga = append(b.BackupManga, m)
	}
	return b
}

// FixtureCategoryUID returns the uid FixtureBackup assigns to its category.
func FixtureCategoryUID(prefix string) int64 {
	return hashUID(prefix)
}

func hashUID(s string) int64 {
	var h int64 = 1125899906842597
	for _, c := range s {
		h = 31*h + int64(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}

// FixtureTitles returns the titles FixtureBackup generates, for assertions.
func FixtureTitles(prefix string, mangaCount int) []string {
	titles := make([]string, 0, mangaCount)
	for i := 1; i <= mangaCount; i++ {
		titles = append(titles, fmt.Sprintf("%s %02d", prefix, i))
	}
	return titles
}

func WriteFixture(path string, b *pb.Backup) error {
	data, err := backup.EncodeGzip(b)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func ReadFixture(path string) (*pb.Backup, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return backup.Decode(data)
}

// MarkChaptersRead flips the first n chapters of the given manga title to read,
// bumping versions the way a client edit would.
func MarkChaptersRead(b *pb.Backup, title string, n int) {
	for _, m := range b.BackupManga {
		if m.Title != title {
			continue
		}
		for i, ch := range m.Chapters {
			if i >= n {
				break
			}
			ch.Read = true
			ch.LastPageRead = 0
			ch.Version++
		}
		m.Version++
	}
}
