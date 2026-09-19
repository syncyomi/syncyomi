//go:build e2e

package scenarios

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

func suwayomiJar(t *testing.T) string {
	t.Helper()
	jar := os.Getenv("E2E_SUWAYOMI_JAR")
	if jar == "" {
		home, _ := os.UserHomeDir()
		matches, _ := filepath.Glob(filepath.Join(home, "projects", "Suwayomi-Server", "server", "build", "Suwayomi-Server-*.jar"))
		if len(matches) > 0 {
			jar = matches[len(matches)-1]
		}
	}
	if jar == "" {
		t.Skip("E2E_SUWAYOMI_JAR not set and no jar found; skipping")
	}
	return jar
}

func startSuwayomi(t *testing.T, ctx context.Context, srv *harness.SyncServer) *harness.Suwayomi {
	t.Helper()
	suwa, err := harness.StartSuwayomi(ctx, suwayomiJar(t), srv, filepath.Join(artifactDir, harness.SanitizeName(t.Name())))
	if err != nil {
		t.Fatalf("start suwayomi: %v", err)
	}
	t.Cleanup(suwa.Stop)
	return suwa
}

func suwaSync(t *testing.T, ctx context.Context, suwa *harness.Suwayomi) {
	t.Helper()
	start := time.Now()
	if result, err := suwa.StartSync(ctx); err != nil {
		t.Fatalf("startSync: %v", err)
	} else if result != "SUCCESS" {
		t.Fatalf("startSync result = %s", result)
	}
	if err := suwa.WaitForSyncSuccess(ctx, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	t.Logf("suwayomi sync took %s", time.Since(start).Round(time.Millisecond))
}

func TestS7_AndroidSuwayomiBothDirections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")

	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	suwa := startSuwayomi(t, ctx, srv)

	if result, err := suwa.StartSync(ctx); err != nil {
		t.Fatalf("startSync: %v", err)
	} else if result != "SUCCESS" {
		t.Fatalf("startSync result = %s", result)
	}
	if err := suwa.WaitForSyncSuccess(ctx, 2*time.Minute); err != nil {
		t.Fatal(err)
	}

	want := harness.FixtureTitles("E2E Alpha", fixtureAManga)
	got, err := suwa.LibraryTitles(ctx)
	if err != nil {
		t.Fatalf("suwayomi library: %v", err)
	}
	if !sameStringSet(got, want) {
		t.Errorf("suwayomi library = %v, want %v", got, want)
	}

	if _, err := suwa.StartSync(ctx); err != nil {
		t.Fatal(err)
	}
	if err := suwa.WaitForSyncSuccess(ctx, 2*time.Minute); err != nil {
		t.Fatal(err)
	}
	syncViaBroadcast(t, ctx, emuA, srv)

	gotA, err := libraryTitles(t, ctx, emuA)
	if err != nil {
		t.Fatalf("read A library: %v", err)
	}
	if !sameStringSet(gotA, want) {
		t.Errorf("android library after round-trip = %v, want %v", gotA, want)
	}
}

func TestS14_SuwayomiCoreConvergence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv)
	seedServer(t, ctx, srv, "E2E Alpha")

	suwa := startSuwayomi(t, ctx, srv)
	suwaSync(t, ctx, suwa)
	assertSuwaLibrarySize(t, ctx, suwa, fixtureAManga)

	edits := harness.FixtureBackup("E2E Alpha", fixtureAManga, fixtureAChapters)
	harness.MarkChaptersRead(edits, "E2E Alpha 01", fixtureAChapters)
	edits.BackupCategories[0].Name = "E2E Renamed"
	edits.BackupCategories[0].Version = 1
	zeta := &pb.BackupCategory{Name: "E2E Zeta", Order: 1, Id: 2, Uid: harness.FixtureCategoryUID("E2E Zeta"), Version: 1}
	edits.BackupCategories = append(edits.BackupCategories, zeta)
	for _, m := range edits.BackupManga {
		if m.Title == "E2E Alpha 02" {
			m.Categories = []int64{zeta.Order}
			m.Version = 2
		}
	}
	c := harness.NewSyntheticClient(srv, "e2e-suwa-edits")
	if _, err := c.Merge(ctx, edits, harness.MergeOptions{}); err != nil {
		t.Fatalf("edits merge: %v", err)
	}

	suwaSync(t, ctx, suwa)
	library, err := suwa.Library(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]harness.SuwayomiManga{}
	for _, m := range library {
		byTitle[m.Title] = m
	}
	if got := byTitle["E2E Alpha 01"].ReadCount; got != fixtureAChapters {
		t.Errorf("suwayomi read count for Alpha 01 = %d, want %d", got, fixtureAChapters)
	}
	if got := byTitle["E2E Alpha 02"].Categories; !slices.Contains(got, "E2E Zeta") || slices.Contains(got, "E2E Renamed") {
		t.Errorf("suwayomi Alpha 02 categories = %v, want a MOVE to E2E Zeta (old membership removed)", got)
	}
	cats, err := suwa.CategoryNames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cats, "E2E Renamed") || slices.Contains(cats, "E2E Alpha") {
		t.Errorf("suwayomi categories = %v, want rename E2E Alpha -> E2E Renamed applied", cats)
	}
	if ri, zi := slices.Index(cats, "E2E Renamed"), slices.Index(cats, "E2E Zeta"); ri > zi {
		t.Errorf("suwayomi category order = %v, want E2E Renamed before E2E Zeta", cats)
	}

	if err := suwa.MarkChaptersRead(ctx, "E2E Alpha 03"); err != nil {
		t.Fatal(err)
	}
	suwaSync(t, ctx, suwa)
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotReadCount(snap, "E2E Alpha 03"); got != fixtureAChapters {
		t.Errorf("server read count for Alpha 03 after suwayomi push = %d, want %d", got, fixtureAChapters)
	}

	if _, err := c.Merge(ctx, nil, harness.MergeOptions{DeletedCategories: []int64{zeta.Uid}}); err != nil {
		t.Fatalf("tombstone merge: %v", err)
	}
	suwaSync(t, ctx, suwa)
	cats, err = suwa.CategoryNames(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(cats, "E2E Zeta") {
		t.Errorf("suwayomi still has E2E Zeta after tombstone: %v", cats)
	}
	assertSuwaLibrarySize(t, ctx, suwa, fixtureAManga)

	if _, err := suwa.CreateCategoryAt(ctx, "E2E Suwa", 1); err != nil {
		t.Fatal(err)
	}
	if err := suwa.AddMangaToCategory(ctx, "E2E Alpha 02", "E2E Suwa"); err != nil {
		t.Fatal(err)
	}
	suwaSync(t, ctx, suwa)
	snap, err = srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotCategoryOrder(snap); !slices.Equal(got, []string{"E2E Suwa", "E2E Renamed"}) {
		t.Errorf("server category order after suwayomi push = %v, want [E2E Suwa, E2E Renamed]", got)
	}
	orders := make([]int64, 0, len(snap.BackupCategories))
	for _, c := range snap.BackupCategories {
		orders = append(orders, c.Order)
	}
	slices.Sort(orders)
	for i, o := range orders {
		if o != int64(i) {
			t.Errorf("server category orders = %v, want 0-based contiguous", orders)
			break
		}
	}
	if !snapshotMangaInCategory(snap, "E2E Alpha 02", "E2E Suwa") {
		t.Errorf("server missing E2E Alpha 02 in E2E Suwa after suwayomi push")
	}
}

func TestS15_CrossPlatformDeepSync(t *testing.T) {
	suwayomiJar(t)

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")
	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	if err := emuA.RunFlow(ctx, harness.FlowPath("mark_read.yaml"), artifactDir,
		map[string]string{"TITLE": "E2E Alpha 01"}); err != nil {
		t.Fatalf("mark_read flow: %v", err)
	}
	awaitReadCount(t, ctx, emuA, "E2E Alpha 01", fixtureAChapters)

	if err := emuA.RunFlow(ctx, harness.FlowPath("create_category.yaml"), artifactDir,
		map[string]string{"NAME": "E2E Zeta"}); err != nil {
		t.Fatalf("create category flow: %v", err)
	}
	if err := emuA.RunFlow(ctx, harness.FlowPath("move_category.yaml"), artifactDir,
		map[string]string{"TITLE": "E2E Alpha 03", "NEW": "E2E Zeta", "OLD": "E2E Alpha"}); err != nil {
		t.Fatalf("move category flow: %v", err)
	}
	awaitMangaInCategory(t, ctx, emuA, "E2E Alpha 03", "E2E Zeta")
	awaitMangaNotInCategory(t, ctx, emuA, "E2E Alpha 03", "E2E Alpha")
	syncViaBroadcast(t, ctx, emuA, srv)

	suwa := startSuwayomi(t, ctx, srv)
	suwaSync(t, ctx, suwa)
	library, err := suwa.Library(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byTitle := map[string]harness.SuwayomiManga{}
	for _, m := range library {
		byTitle[m.Title] = m
	}
	if got := byTitle["E2E Alpha 01"].ReadCount; got != fixtureAChapters {
		t.Errorf("suwayomi read count for Alpha 01 = %d, want %d", got, fixtureAChapters)
	}
	if got := byTitle["E2E Alpha 03"].Categories; !slices.Contains(got, "E2E Zeta") || slices.Contains(got, "E2E Alpha") {
		t.Errorf("suwayomi Alpha 03 categories = %v, want a MOVE to E2E Zeta (E2E Alpha removed)", got)
	}

	if err := suwa.MarkChaptersRead(ctx, "E2E Alpha 05"); err != nil {
		t.Fatal(err)
	}
	suwaSync(t, ctx, suwa)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitReadCount(t, ctx, emuA, "E2E Alpha 05", fixtureAChapters)
}

func assertSuwaLibrarySize(t *testing.T, ctx context.Context, suwa *harness.Suwayomi, want int) {
	t.Helper()
	titles, err := suwa.LibraryTitles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != want {
		t.Fatalf("suwayomi library has %d entries, want %d: %v", len(titles), want, titles)
	}
}
