//go:build e2e

package scenarios

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

func TestS29_UnifiedTombstones(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv)

	const prefix = "E2E Tomb"

	seed := harness.FixtureBackup(prefix, 3, 3)
	for _, m := range seed.BackupManga {
		m.Version = 3
		for _, ch := range m.Chapters {
			ch.Version = 3
		}
	}
	seed.BackupSavedSearches = []*pb.BackupSavedSearch{
		{Name: "E2E Search", Query: "q", Source: harness.FixtureSource},
	}
	seeder := harness.NewSyntheticClient(srv, "e2e-tombstone-seed")
	if _, err := seeder.Merge(ctx, seed, harness.MergeOptions{Full: true}); err != nil {
		t.Fatalf("seed merge: %v", err)
	}

	snap := snapshotNow(t, ctx, srv)
	if got, want := harness.SnapshotTitles(snap), harness.FixtureTitles(prefix, 3); !slices.Equal(got, want) {
		t.Fatalf("baseline titles = %v, want %v", got, want)
	}
	if got := requireManga(t, snap, "E2E Tomb 01"); len(got.Chapters) != 3 {
		t.Errorf("baseline E2E Tomb 01 has %d chapters, want 3", len(got.Chapters))
	}
	if got, want := savedSearchNames(snap), []string{"E2E Search"}; !slices.Equal(got, want) {
		t.Errorf("baseline saved searches = %v, want %v", got, want)
	}

	peer := harness.NewSyntheticClient(srv, "e2e-tombstone-peer")
	if _, err := peer.Merge(ctx, nil, harness.MergeOptions{}); err != nil {
		t.Fatalf("peer initial merge: %v", err)
	}

	noop := peerDelta(t, ctx, peer)
	if len(noop.Deletions) != 0 {
		t.Errorf("no-op delta carries %d deletions, want 0: %v", len(noop.Deletions), noop.Deletions)
	}
	if got := harness.SnapshotTitles(noop); len(got) != 0 {
		t.Errorf("no-op delta carries manga %v, want none", got)
	}

	seq0 := seqNow(t, ctx, srv)
	validation := []struct {
		name         string
		opts         harness.DeletionsOptions
		status       int
		acknowledged int
		applied      int
	}{
		{"unknown kind", harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "bogus", Key: "x"}}}, 400, 0, 0},
		{"empty key", harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "manga", Key: ""}}}, 400, 0, 0},
		{"oversized key", harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "manga", Key: strings.Repeat("k", 4097)}}}, 400, 0, 0},
		{"malformed body", harness.DeletionsOptions{RawBody: []byte("{")}, 400, 0, 0},
		{"unknown key", harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "manga", Key: "9999999999|/missing"}}}, 200, 1, 0},
		{"empty body", harness.DeletionsOptions{RawBody: []byte{}}, 200, 0, 0},
	}
	for _, tc := range validation {
		res, err := seeder.Deletions(ctx, tc.opts)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if res.StatusCode != tc.status {
			t.Errorf("%s: status = %d, want %d (body %s)", tc.name, res.StatusCode, tc.status, res.Body)
			continue
		}
		if res.Acknowledged != tc.acknowledged || res.Applied != tc.applied {
			t.Errorf("%s: ack %d applied %d, want ack %d applied %d",
				tc.name, res.Acknowledged, res.Applied, tc.acknowledged, tc.applied)
		}
		if got := seqNow(t, ctx, srv); got != seq0 {
			t.Errorf("%s advanced seq: %d -> %d", tc.name, seq0, got)
		}
	}

	manga01Key := backup.MangaKey(harness.FixtureSource, "/e2e/E2E Tomb/01")
	ch01URL := "/e2e/E2E Tomb/01/ch01"
	ch01Key := backup.ChapterKey(manga01Key, ch01URL)
	requireAppliedDeletion(t, ctx, srv, seeder, "chapter ch01",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "chapter", Key: ch01Key}}})

	snap = snapshotNow(t, ctx, srv)
	m01 := requireManga(t, snap, "E2E Tomb 01")
	if got, want := len(m01.Chapters), 2; got != want {
		t.Errorf("E2E Tomb 01 has %d chapters after tombstone, want %d", got, want)
	}
	if urls := chapterURLs(m01); slices.Contains(urls, ch01URL) {
		t.Errorf("E2E Tomb 01 still lists the tombstoned chapter %s: %v", ch01URL, urls)
	}

	delta := peerDelta(t, ctx, peer)
	d01 := requireManga(t, delta, "E2E Tomb 01")
	if urls := chapterURLs(d01); slices.Contains(urls, ch01URL) {
		t.Errorf("peer delta still carries the tombstoned chapter %s: %v", ch01URL, urls)
	}
	if got, want := len(d01.Chapters), 2; got != want {
		t.Errorf("peer delta E2E Tomb 01 has %d chapters, want %d", got, want)
	}

	ch02URL := "/e2e/E2E Tomb/01/ch02"
	ch02Key := backup.ChapterKey(manga01Key, ch02URL)
	requireAppliedDeletion(t, ctx, srv, seeder, "legacy chapter ch02",
		harness.DeletionsOptions{DeletedChapters: []string{ch02Key}})

	snap = snapshotNow(t, ctx, srv)
	m01 = requireManga(t, snap, "E2E Tomb 01")
	if got, want := len(m01.Chapters), 1; got != want {
		t.Errorf("E2E Tomb 01 has %d chapters after legacy tombstone, want %d", got, want)
	}
	if urls := chapterURLs(m01); slices.Contains(urls, ch02URL) {
		t.Errorf("E2E Tomb 01 still lists the legacy-tombstoned chapter %s: %v", ch02URL, urls)
	}

	delta = peerDelta(t, ctx, peer)
	if urls := chapterURLs(requireManga(t, delta, "E2E Tomb 01")); slices.Contains(urls, ch02URL) {
		t.Errorf("peer delta still carries the legacy-tombstoned chapter %s: %v", ch02URL, urls)
	}

	manga02Key := backup.MangaKey(harness.FixtureSource, "/e2e/E2E Tomb/02")
	requireAppliedDeletion(t, ctx, srv, seeder, "manga 02",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "manga", Key: manga02Key}}})

	snap = snapshotNow(t, ctx, srv)
	if m := mangaByTitle(snap, "E2E Tomb 02"); m != nil {
		t.Errorf("snapshot still has E2E Tomb 02 with %d chapters", len(m.Chapters))
	}
	assertNoChapterURL(t, snap, "/e2e/E2E Tomb/02/ch01")
	if got, want := harness.SnapshotTitles(snap), []string{"E2E Tomb 01", "E2E Tomb 03"}; !slices.Equal(got, want) {
		t.Errorf("titles after manga tombstone = %v, want %v", got, want)
	}

	delta, header := peerDeltaHeaders(t, ctx, peer)
	requireDeletion(t, delta, "manga", manga02Key)
	if got := header.Get("X-Sync-Full-Requested"); got != "" {
		t.Errorf("peer delta after a manga tombstone: X-Sync-Full-Requested = %q, want none: the tombstone is in deletions", got)
	}
	if m := mangaByTitle(delta, "E2E Tomb 02"); m != nil {
		t.Errorf("peer delta still has E2E Tomb 02")
	}
	assertNoChapterURL(t, delta, "/e2e/E2E Tomb/02/ch01")

	full, fullHeader, err := peer.MergeHeaders(ctx, nil, harness.MergeOptions{Full: true})
	if err != nil {
		t.Fatalf("peer full sync: %v", err)
	}
	if got := fullHeader.Get("X-Sync-Full-Requested"); got != "" {
		t.Errorf("peer full sync: X-Sync-Full-Requested = %q, want none", got)
	}
	if m := mangaByTitle(full, "E2E Tomb 02"); m != nil {
		t.Errorf("full sync after the manga tombstone still has E2E Tomb 02 with %d chapters", len(m.Chapters))
	}
	if got, want := harness.SnapshotTitles(full), []string{"E2E Tomb 01", "E2E Tomb 03"}; !slices.Equal(got, want) {
		t.Errorf("full sync titles = %v, want %v", got, want)
	}

	searchName := "E2E Search"
	searchKey := searchName + "|" + strconv.FormatInt(harness.FixtureSource, 10)
	requireAppliedDeletion(t, ctx, srv, seeder, "saved search",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "saved_search", Key: searchKey}}})

	snap = snapshotNow(t, ctx, srv)
	if names := savedSearchNames(snap); slices.Contains(names, searchName) {
		t.Errorf("snapshot still has saved search %q: %v", searchName, names)
	}

	delta, header = peerDeltaHeaders(t, ctx, peer)
	requireDeletion(t, delta, "saved_search", searchKey)
	if got := header.Get("X-Sync-Full-Requested"); got != "" {
		t.Errorf("peer delta after a section tombstone: X-Sync-Full-Requested = %q, want none: the tombstone is in deletions", got)
	}
	if names := savedSearchNames(delta); slices.Contains(names, searchName) {
		t.Errorf("peer delta still has saved search %q: %v", searchName, names)
	}

	catUID := harness.FixtureCategoryUID(prefix)
	catKey := "uid:" + strconv.FormatInt(catUID, 10)
	requireAppliedDeletion(t, ctx, srv, seeder, "category",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "category", Key: catKey}}})

	snap = snapshotNow(t, ctx, srv)
	if names := categoryNames(snap.BackupCategories); slices.Contains(names, prefix) {
		t.Errorf("snapshot still has category %q: %v", prefix, names)
	}

	sameVersion := &pb.Backup{BackupCategories: []*pb.BackupCategory{{
		Name: prefix, Order: 0, Id: 1, Uid: catUID, Version: 0,
		LastModifiedAt: time.Now().UnixMilli(),
	}}}
	if _, err := seeder.Merge(ctx, sameVersion, harness.MergeOptions{}); err != nil {
		t.Fatalf("merge same-version category: %v", err)
	}
	snap = snapshotNow(t, ctx, srv)
	if names := categoryNames(snap.BackupCategories); slices.Contains(names, prefix) {
		t.Errorf("equal version + newer timestamp resurrected category %q: %v", prefix, names)
	}

	higherVersion := &pb.Backup{BackupCategories: []*pb.BackupCategory{{
		Name: prefix, Order: 0, Id: 1, Uid: catUID, Version: 1,
	}}}
	if _, err := seeder.Merge(ctx, higherVersion, harness.MergeOptions{}); err != nil {
		t.Fatalf("merge higher-version category: %v", err)
	}
	snap = snapshotNow(t, ctx, srv)
	if names := categoryNames(snap.BackupCategories); !slices.Contains(names, prefix) {
		t.Errorf("higher version did not resurrect category %q: %v", prefix, names)
	}

	manga03Key := backup.MangaKey(harness.FixtureSource, "/e2e/E2E Tomb/03")
	ch03URL01 := "/e2e/E2E Tomb/03/ch01"
	manga03ChapterKey := backup.ChapterKey(manga03Key, ch03URL01)
	requireAppliedDeletion(t, ctx, srv, seeder, "chapter ch01 of manga 03",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "chapter", Key: manga03ChapterKey}}})

	snap = snapshotNow(t, ctx, srv)
	if got := requireManga(t, snap, "E2E Tomb 03"); len(got.Chapters) != 2 {
		t.Errorf("E2E Tomb 03 has %d chapters after tombstone, want 2", len(got.Chapters))
	}

	reAdd := func(version int64, modifiedAt int64) *pb.Backup {
		return &pb.Backup{BackupManga: []*pb.BackupManga{{
			Source: harness.FixtureSource, Url: "/e2e/E2E Tomb/03", Title: "E2E Tomb 03",
			Status: 1, Initialized: true, Version: 3,
			Chapters: []*pb.BackupChapter{{
				Url: ch03URL01, Name: "Chapter 1", ChapterNumber: 1,
				Version: version, LastModifiedAt: modifiedAt, Read: true,
			}},
		}}}
	}

	if _, err := seeder.Merge(ctx, reAdd(1, time.Now().Add(time.Hour).Unix()), harness.MergeOptions{}); err != nil {
		t.Fatalf("merge newer re-add: %v", err)
	}
	snap = snapshotNow(t, ctx, srv)
	m03 := requireManga(t, snap, "E2E Tomb 03")
	if got, want := len(m03.Chapters), 3; got != want {
		t.Errorf("E2E Tomb 03 has %d chapters after newer re-add, want %d", got, want)
	}
	if urls := chapterURLs(m03); !slices.Contains(urls, ch03URL01) {
		t.Errorf("newer re-add did not resurrect %s: %v", ch03URL01, urls)
	}

	requireAppliedDeletion(t, ctx, srv, seeder, "chapter ch01 of manga 03 again",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "chapter", Key: manga03ChapterKey}}})

	if _, err := seeder.Merge(ctx, reAdd(1, time.Now().Add(-time.Hour).Unix()), harness.MergeOptions{}); err != nil {
		t.Fatalf("merge stale re-add: %v", err)
	}
	snap = snapshotNow(t, ctx, srv)
	m03 = requireManga(t, snap, "E2E Tomb 03")
	if got, want := len(m03.Chapters), 2; got != want {
		t.Errorf("E2E Tomb 03 has %d chapters after stale re-add, want %d", got, want)
	}
	if urls := chapterURLs(m03); slices.Contains(urls, ch03URL01) {
		t.Errorf("stale re-add resurrected %s: %v", ch03URL01, urls)
	}

	histMangaKey := backup.MangaKey(harness.FixtureSource, "/e2e/E2E Tomb/04")
	histURL := "/e2e/E2E Tomb/04/ch01"
	histOtherURL := "/e2e/E2E Tomb/04/ch02"
	histKey := backup.HistoryKey(histMangaKey, histURL)
	histBackup := func(lastRead int64) *pb.Backup {
		return &pb.Backup{BackupManga: []*pb.BackupManga{{
			Source: harness.FixtureSource, Url: "/e2e/E2E Tomb/04", Title: "E2E Tomb 04",
			Status: 1, Initialized: true, Version: 1,
			History: []*pb.BackupHistory{{Url: histURL, LastRead: lastRead}},
		}}}
	}
	histSeed := histBackup(time.Now().Unix())
	histSeed.BackupManga[0].History = append(histSeed.BackupManga[0].History,
		&pb.BackupHistory{Url: histOtherURL, LastRead: time.Now().Add(-time.Hour).Unix()})
	if _, err := seeder.Merge(ctx, histSeed, harness.MergeOptions{}); err != nil {
		t.Fatalf("merge history: %v", err)
	}
	peerDelta(t, ctx, peer)
	requireAppliedDeletion(t, ctx, srv, seeder, "history entry",
		harness.DeletionsOptions{Items: []harness.DeletionItem{{Kind: "history", Key: histKey}}})

	delta, header = peerDeltaHeaders(t, ctx, peer)
	if got := header.Get("X-Sync-Full-Requested"); got != "" {
		t.Errorf("peer delta after a history tombstone: X-Sync-Full-Requested = %q, want none", got)
	}
	if urls := historyURLs(requireManga(t, delta, "E2E Tomb 04")); !slices.Equal(urls, []string{histOtherURL}) {
		t.Errorf("peer delta history = %v, want only the surviving %s", urls, histOtherURL)
	}

	if _, err := seeder.Merge(ctx, histBackup(time.Now().Add(-time.Hour).Unix()), harness.MergeOptions{}); err != nil {
		t.Fatalf("merge stale history: %v", err)
	}
	snap = snapshotNow(t, ctx, srv)
	if urls := historyURLs(requireManga(t, snap, "E2E Tomb 04")); slices.Contains(urls, histURL) {
		t.Errorf("stale history re-upload resurrected %s: %v", histURL, urls)
	}

	if _, err := seeder.Merge(ctx, histBackup(time.Now().Add(time.Hour).Unix()), harness.MergeOptions{}); err != nil {
		t.Fatalf("merge newer history: %v", err)
	}
	snap = snapshotNow(t, ctx, srv)
	if urls := historyURLs(requireManga(t, snap, "E2E Tomb 04")); !slices.Contains(urls, histURL) {
		t.Errorf("newer history did not resurrect %s: %v", histURL, urls)
	}
}

func snapshotNow(t *testing.T, ctx context.Context, srv *harness.SyncServer) *pb.Backup {
	t.Helper()
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return snap
}

func seqNow(t *testing.T, ctx context.Context, srv *harness.SyncServer) int64 {
	t.Helper()
	seq, err := srv.Seq(ctx)
	if err != nil {
		t.Fatalf("seq: %v", err)
	}
	return seq
}

func requireAppliedDeletion(t *testing.T, ctx context.Context, srv *harness.SyncServer, c *harness.SyntheticClient, what string, opts harness.DeletionsOptions) {
	t.Helper()
	before := seqNow(t, ctx, srv)
	res, err := c.Deletions(ctx, opts)
	if err != nil {
		t.Fatalf("%s deletions: %v", what, err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("%s deletions: status = %d, want 200 (body %s)", what, res.StatusCode, res.Body)
	}
	if res.Acknowledged != 1 || res.Applied != 1 {
		t.Errorf("%s deletions = ack %d applied %d, want ack 1 applied 1", what, res.Acknowledged, res.Applied)
	}
	if got := seqNow(t, ctx, srv); got <= before {
		t.Errorf("%s tombstone did not advance seq: %d -> %d", what, before, got)
	}
}

func peerDelta(t *testing.T, ctx context.Context, c *harness.SyntheticClient) *pb.Backup {
	t.Helper()
	b, err := c.Merge(ctx, nil, harness.MergeOptions{})
	if err != nil {
		t.Fatalf("peer delta: %v", err)
	}
	return b
}

func requireDeletion(t *testing.T, b *pb.Backup, kind, key string) {
	t.Helper()
	for _, d := range b.Deletions {
		if d.Kind == kind && d.Key == key {
			if d.Seq <= 0 {
				t.Errorf("deletion %s %s carries seq %d, want the seq of the deletion", kind, key, d.Seq)
			}
			return
		}
	}
	t.Fatalf("delta omits the %s tombstone %s: deletions = %v", kind, key, b.Deletions)
}

func peerDeltaHeaders(t *testing.T, ctx context.Context, c *harness.SyntheticClient) (*pb.Backup, http.Header) {
	t.Helper()
	b, header, err := c.MergeHeaders(ctx, nil, harness.MergeOptions{})
	if err != nil {
		t.Fatalf("peer delta: %v", err)
	}
	return b, header
}

func mangaByTitle(b *pb.Backup, title string) *pb.BackupManga {
	for _, m := range b.BackupManga {
		if m.Title == title {
			return m
		}
	}
	return nil
}

func requireManga(t *testing.T, b *pb.Backup, title string) *pb.BackupManga {
	t.Helper()
	m := mangaByTitle(b, title)
	if m == nil {
		t.Fatalf("%q missing; present titles = %v", title, harness.SnapshotTitles(b))
	}
	return m
}

func chapterURLs(m *pb.BackupManga) []string {
	urls := make([]string, 0, len(m.Chapters))
	for _, ch := range m.Chapters {
		urls = append(urls, ch.Url)
	}
	return urls
}

func historyURLs(m *pb.BackupManga) []string {
	urls := make([]string, 0, len(m.History))
	for _, h := range m.History {
		urls = append(urls, h.Url)
	}
	return urls
}

func savedSearchNames(b *pb.Backup) []string {
	names := make([]string, 0, len(b.BackupSavedSearches))
	for _, s := range b.BackupSavedSearches {
		names = append(names, s.Name)
	}
	return names
}

func assertNoChapterURL(t *testing.T, b *pb.Backup, url string) {
	t.Helper()
	for _, m := range b.BackupManga {
		if urls := chapterURLs(m); slices.Contains(urls, url) {
			t.Errorf("manga %q still has chapter %s: %v", m.Title, url, urls)
		}
	}
}
