package backup

import (
	"sort"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/merge"
	"google.golang.org/protobuf/proto"
)

func TestSplitRenderFixtureRoundTrip(t *testing.T) {
	b := loadFixture(t)
	items, err := Split(b)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[merge.Kind]int{}
	for _, it := range items {
		counts[it.Kind]++
	}
	chapters := 0
	for _, m := range b.BackupManga {
		chapters += len(m.Chapters)
	}
	if counts[merge.KindManga] != len(b.BackupManga) || counts[merge.KindChapter] != chapters || counts[merge.KindCategory] != len(b.BackupCategories) {
		t.Errorf("counts = %v", counts)
	}

	out, err := Render(items)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(normalize(b), normalize(out)) {
		t.Error("Split+Render changed the backup")
	}
}

// Render order is deterministic (categories by order, chapters by source order); the
// fixture's order may differ, so compare sorted copies.
func normalize(b *pb.Backup) *pb.Backup {
	c := proto.Clone(b).(*pb.Backup)
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
	return c
}

func TestSplitMangaPayloadHasNoChaptersOrCategories(t *testing.T) {
	b := &pb.Backup{
		BackupCategories: []*pb.BackupCategory{{Name: "A", Order: 0, Uid: 10}, {Name: "B", Order: 1}},
		BackupManga: []*pb.BackupManga{{
			Source: 1, Url: "/m", Categories: []int64{0, 1, 7},
			Chapters: []*pb.BackupChapter{{Url: "/c1"}, {Url: "/c2"}},
		}},
	}
	items, err := Split(b)
	if err != nil {
		t.Fatal(err)
	}
	var manga *merge.Item
	for _, it := range items {
		if it.Kind == merge.KindManga {
			manga = it
		}
	}
	if manga == nil {
		t.Fatal("no manga item")
	}
	m := &pb.BackupManga{}
	if err := proto.Unmarshal(manga.Payload, m); err != nil {
		t.Fatal(err)
	}
	if len(m.Chapters) != 0 || len(m.Categories) != 0 {
		t.Errorf("payload still carries chapters/categories: %v", m)
	}
	if len(manga.Refs) != 2 || manga.Refs[0] != "uid:10" || manga.Refs[1] != "name:B" {
		t.Errorf("refs = %v (unknown order 7 must be dropped)", manga.Refs)
	}
	for _, it := range items {
		if it.Kind == merge.KindChapter && it.ParentKey != "1|/m" {
			t.Errorf("chapter parent = %q", it.ParentKey)
		}
	}
}

func TestRenderSkipsTombstonesAndRemapsOrders(t *testing.T) {
	items := []*merge.Item{
		{Kind: merge.KindCategory, Key: "uid:1", Payload: mustEncode(t, &pb.BackupCategory{Name: "Keep", Order: 5, Uid: 1})},
		{Kind: merge.KindCategory, Key: "uid:2", Deleted: true, Payload: mustEncode(t, &pb.BackupCategory{Name: "Gone", Order: 6, Uid: 2})},
		{Kind: merge.KindManga, Key: "3|/m", Refs: []string{"uid:1", "uid:2", "uid:404"}, Payload: mustEncode(t, &pb.BackupManga{Source: 3, Url: "/m"})},
		{Kind: merge.KindChapter, Key: ChapterKey("3|/m", "/c2"), ParentKey: "3|/m", Payload: mustEncode(t, &pb.BackupChapter{Url: "/c2", SourceOrder: 1})},
		{Kind: merge.KindChapter, Key: ChapterKey("3|/m", "/c1"), ParentKey: "3|/m", Payload: mustEncode(t, &pb.BackupChapter{Url: "/c1", SourceOrder: 0})},
		{Kind: merge.KindChapter, Key: ChapterKey("9|/x", "/c9"), ParentKey: "9|/x", Payload: mustEncode(t, &pb.BackupChapter{Url: "/c9"})},
	}
	b, err := Render(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.BackupCategories) != 1 || b.BackupCategories[0].Name != "Keep" {
		t.Errorf("categories = %v", b.BackupCategories)
	}
	if len(b.BackupManga) != 1 {
		t.Fatalf("manga = %v", b.BackupManga)
	}
	m := b.BackupManga[0]
	if len(m.Categories) != 1 || m.Categories[0] != 5 {
		t.Errorf("manga categories = %v, want [5]", m.Categories)
	}
	if len(m.Chapters) != 2 || m.Chapters[0].Url != "/c1" {
		t.Errorf("chapters = %v", m.Chapters)
	}
	if len(b.BackupSources) != 1 || b.BackupSources[0].SourceId != 3 {
		t.Errorf("missing source auto-added: %v", b.BackupSources)
	}
}

func TestSplitKeepsUnknownTopLevelFields(t *testing.T) {
	b := &pb.Backup{}
	b.ProtoReflect().SetUnknown([]byte{0xc2, 0xb2, 0x04, 0x01, 0x41}) // field 9000 bytes "A"
	items, err := Split(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Kind != merge.KindExtra {
		t.Fatalf("items = %+v", items)
	}
	out, err := Render(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.ProtoReflect().GetUnknown()) == 0 {
		t.Error("extra fields lost on render")
	}
}

func mustEncode(t *testing.T, m proto.Message) []byte {
	t.Helper()
	data, err := encodeMsg(m)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
