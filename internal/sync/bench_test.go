package sync

import (
	"context"
	"fmt"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"google.golang.org/protobuf/proto"
)

const (
	benchManga    = 3600
	benchChapters = 60
)

func benchBackup(mangaCount, chapterCount int) *pb.Backup {
	b := &pb.Backup{
		BackupCategories: []*pb.BackupCategory{{Name: "Reading", Order: 0, Id: 1, Uid: 1, Version: 1}},
		BackupSources:    []*pb.BackupSource{{Name: "Bench Source", SourceId: 42}},
	}
	description := fmt.Sprintf("%0300d", 0)
	for i := 0; i < mangaCount; i++ {
		m := &pb.BackupManga{
			Source:         42,
			Url:            fmt.Sprintf("/manga/%05d-some-fairly-long-slug-title", i),
			Title:          fmt.Sprintf("Bench Manga %05d: A Title Of Ordinary Length", i),
			Artist:         proto.String("Artist Name"),
			Author:         proto.String("Author Name"),
			Description:    proto.String(description),
			Genre:          []string{"Action", "Adventure", "Comedy", "Drama", "Fantasy"},
			Status:         1,
			ThumbnailUrl:   proto.String(fmt.Sprintf("https://cdn.example.org/covers/%05d/cover-large.jpg", i)),
			DateAdded:      1_700_000_000_000 + int64(i),
			Categories:     []int64{0},
			LastModifiedAt: 1_700_000_000_000 + int64(i),
			Version:        1,
			Initialized:    true,
		}
		backup.SetFavorite(m, true)
		for c := 0; c < chapterCount; c++ {
			m.Chapters = append(m.Chapters, &pb.BackupChapter{
				Url:            fmt.Sprintf("/manga/%05d/chapter/%04d-release", i, c),
				Name:           fmt.Sprintf("Chapter %d: Something Happens", c),
				Scanlator:      proto.String("Scan Group"),
				Read:           c%3 == 0,
				DateFetch:      1_700_000_000_000 + int64(c),
				DateUpload:     1_690_000_000_000 + int64(c),
				ChapterNumber:  float32(c),
				SourceOrder:    int64(chapterCount - c),
				LastModifiedAt: 1_700_000_000_000 + int64(c),
				Version:        1,
			})
		}
		b.BackupManga = append(b.BackupManga, m)
	}
	return b
}

func seedV1(b *testing.B, svc *service, raw []byte) string {
	b.Helper()
	etag, err := svc.PutContent(context.Background(), "key1", domain.DeviceInfo{}, "", raw)
	if err != nil {
		b.Fatal(err)
	}
	return etag
}

func BenchmarkV1PutContent(b *testing.B) {
	raw, err := backup.Encode(benchBackup(benchManga, benchChapters))
	if err != nil {
		b.Fatal(err)
	}
	b.Logf("payload %d bytes", len(raw))

	b.Run("first", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			svc, _ := newTestService(b)
			b.StartTimer()
			seedV1(b, svc, raw)
		}
	})
	b.Run("repeat", func(b *testing.B) {
		svc, _ := newTestService(b)
		etag := seedV1(b, svc, raw)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var err error
			if etag, err = svc.PutContent(context.Background(), "key1", domain.DeviceInfo{}, etag, raw); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkV2MergeFull(b *testing.B) {
	lib := benchBackup(benchManga, benchChapters)
	svc, _ := newTestService(b)
	resp, err := svc.Merge(context.Background(), MergeRequest{APIKey: "key1", Device: domain.DeviceInfo{ID: "A"}, Full: true, Backup: lib})
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Merge(context.Background(), MergeRequest{APIKey: "key1", Device: domain.DeviceInfo{ID: "A"}, Cursor: resp.Cursor, Full: true, Backup: lib}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkV2MergeDelta(b *testing.B) {
	lib := benchBackup(benchManga, benchChapters)
	svc, _ := newTestService(b)
	resp, err := svc.Merge(context.Background(), MergeRequest{APIKey: "key1", Device: domain.DeviceInfo{ID: "A"}, Full: true, Backup: lib})
	if err != nil {
		b.Fatal(err)
	}
	cursor := resp.Cursor
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m := proto.Clone(lib.BackupManga[i%len(lib.BackupManga)]).(*pb.BackupManga)
		m.Chapters = m.Chapters[:20]
		for _, ch := range m.Chapters {
			ch.Read = true
			ch.Version += int64(i + 1)
		}
		m.Version += int64(i + 1)
		resp, err := svc.Merge(context.Background(), MergeRequest{APIKey: "key1", Device: domain.DeviceInfo{ID: "A"}, Cursor: cursor, Backup: &pb.Backup{BackupManga: []*pb.BackupManga{m}}})
		if err != nil {
			b.Fatal(err)
		}
		cursor = resp.Cursor
	}
}
