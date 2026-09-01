//go:build e2e

package scenarios

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

// TestS10_CategoryRename: A renames the shared category through the real UI;
// the server keeps the same uid under the new name (no duplicate), B converges,
// and re-syncing A does not resurrect the old name.
func TestS10_CategoryRename(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	renameCategory(t, ctx, emuA, "E2E Alpha", "E2E Renamed")
	syncViaBroadcast(t, ctx, emuA, srv)

	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	userCats := snap.BackupCategories
	if len(userCats) != 1 {
		t.Fatalf("server has %d categories, want 1 (rename duplicated?): %v", len(userCats), categoryNames(userCats))
	}
	if got := userCats[0].Name; got != "E2E Renamed" {
		t.Errorf("server category name = %q, want E2E Renamed", got)
	}
	if got, want := userCats[0].Uid, harness.FixtureCategoryUID("E2E Alpha"); got != want {
		t.Errorf("rename changed the category uid: %d, want %d", got, want)
	}

	syncViaBroadcast(t, ctx, emuB, srv)
	awaitCategoryPresent(t, ctx, emuB, "E2E Renamed")
	awaitCategoryGone(t, ctx, emuB, "E2E Alpha")
	awaitMangaInCategory(t, ctx, emuB, "E2E Alpha 01", "E2E Renamed")

	syncViaBroadcast(t, ctx, emuA, srv)
	awaitCategoryGone(t, ctx, emuA, "E2E Alpha")
	awaitCategoryPresent(t, ctx, emuA, "E2E Renamed")
}

// TestS11_CategoryReorder: a remote reorder (swapped positions, bumped versions)
// lands on the device and survives the device's next push unchanged.
func TestS11_CategoryReorder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")
	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	// Remote adds a second category, then swaps the two positions.
	reorder := &pb.Backup{
		BackupCategories: []*pb.BackupCategory{
			{Name: "E2E Alpha", Order: 1, Id: 1, Uid: harness.FixtureCategoryUID("E2E Alpha"), Version: 1},
			{Name: "E2E Zeta", Order: 0, Id: 2, Uid: harness.FixtureCategoryUID("E2E Zeta"), Version: 1},
		},
	}
	c := harness.NewSyntheticClient(srv, "e2e-reorder")
	if _, err := c.Merge(ctx, reorder, harness.MergeOptions{}); err != nil {
		t.Fatalf("reorder merge: %v", err)
	}

	syncViaBroadcast(t, ctx, emuA, srv)
	awaitCategoryPresent(t, ctx, emuA, "E2E Zeta")
	wantOrder := []string{"E2E Zeta", "E2E Alpha"}
	awaitCategoryOrder(t, ctx, emuA, wantOrder)

	// The device's own push must not churn the order back.
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitCategoryOrder(t, ctx, emuA, wantOrder)
	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshotCategoryOrder(snap); !slices.Equal(got, wantOrder) {
		t.Errorf("server category order = %v, want %v", got, wantOrder)
	}
}

// TestS12_CreateAndAssignCategory: A creates a category and assigns a manga to
// it through the real UI; server and B converge.
func TestS12_CreateAndAssignCategory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	createAndAssign(t, ctx, srv, "E2E Zeta", "E2E Alpha 03")

	syncViaBroadcast(t, ctx, emuB, srv)
	awaitCategoryPresent(t, ctx, emuB, "E2E Zeta")
	awaitMangaInCategory(t, ctx, emuB, "E2E Alpha 03", "E2E Zeta")
}

// TestS13_DeviceOriginatedCategoryTombstone: A deletes its category through the
// real UI, which must send the deleted-uid tombstone; the category disappears
// everywhere and never resurrects, while the manga survives.
func TestS13_DeviceOriginatedCategoryTombstone(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA, emuB)
	pairBoth(t, ctx, srv)

	createAndAssign(t, ctx, srv, "E2E Zeta", "E2E Alpha 03")
	syncViaBroadcast(t, ctx, emuB, srv)
	awaitCategoryPresent(t, ctx, emuB, "E2E Zeta")

	// Rows sort by position: "E2E Alpha" at 0, "E2E Zeta" at 1.
	if err := emuA.RunFlow(ctx, harness.FlowPath("delete_category.yaml"), artifactDir,
		map[string]string{"INDEX": "1", "NAME": "E2E Zeta"}); err != nil {
		t.Fatalf("delete flow: %v", err)
	}
	syncViaBroadcast(t, ctx, emuA, srv)

	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if names := categoryNames(snap.BackupCategories); slices.Contains(names, "E2E Zeta") {
		t.Fatalf("server still has E2E Zeta after device tombstone: %v", names)
	}

	syncViaBroadcast(t, ctx, emuB, srv)
	awaitCategoryGone(t, ctx, emuB, "E2E Zeta")

	// No resurrection on further syncs, and the manga survives everywhere.
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitCategoryGone(t, ctx, emuA, "E2E Zeta")
	syncViaBroadcast(t, ctx, emuB, srv)
	awaitCategoryGone(t, ctx, emuB, "E2E Zeta")
	for _, e := range []*harness.Emulator{emuA, emuB} {
		titles, err := libraryTitles(t, ctx, e)
		if err != nil {
			t.Fatal(err)
		}
		if len(titles) != fixtureAManga {
			t.Errorf("%s has %d manga after category delete, want %d", e.AVD, len(titles), fixtureAManga)
		}
	}
}

// renameCategory drives the rename dialog. The pre-filled field opens with the
// cursor at position 0, so the flow is split around an adb KEYCODE_MOVE_END to
// get the cursor behind the old name before erasing it.
func renameCategory(t *testing.T, ctx context.Context, e *harness.Emulator, oldName, newName string) {
	t.Helper()
	if err := e.RunFlow(ctx, harness.FlowPath("rename_category_open.yaml"), artifactDir,
		map[string]string{"OLD": oldName}); err != nil {
		t.Fatalf("rename open flow: %v", err)
	}
	if _, err := e.Adb(ctx, "shell", "input", "keyevent", "KEYCODE_MOVE_END"); err != nil {
		t.Fatalf("move cursor: %v", err)
	}
	if err := e.RunFlow(ctx, harness.FlowPath("rename_category_finish.yaml"), artifactDir,
		map[string]string{"NEW": newName}); err != nil {
		t.Fatalf("rename finish flow: %v", err)
	}
}

// createAndAssign drives the create-category and set-categories UI on A and
// syncs the result up, asserting the server holds both.
func createAndAssign(t *testing.T, ctx context.Context, srv *harness.SyncServer, category, title string) {
	t.Helper()
	if err := emuA.RunFlow(ctx, harness.FlowPath("create_category.yaml"), artifactDir,
		map[string]string{"NAME": category}); err != nil {
		t.Fatalf("create category flow: %v", err)
	}
	if err := emuA.RunFlow(ctx, harness.FlowPath("set_categories.yaml"), artifactDir,
		map[string]string{"TITLE": title, "CATEGORY": category}); err != nil {
		t.Fatalf("set categories flow: %v", err)
	}
	awaitMangaInCategory(t, ctx, emuA, title, category)
	syncViaBroadcast(t, ctx, emuA, srv)

	snap, err := srv.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if names := categoryNames(snap.BackupCategories); !slices.Contains(names, category) {
		t.Fatalf("server categories %v missing %q after push", names, category)
	}
	if !snapshotMangaInCategory(snap, title, category) {
		t.Fatalf("server does not have %q in category %q", title, category)
	}
}

func awaitCategoryPresent(t *testing.T, ctx context.Context, e *harness.Emulator, name string) {
	t.Helper()
	pollLiveDB(t, ctx, e, 60*time.Second, "category "+name+" present", func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		names, err := harness.CategoryNames(db)
		return err == nil && slices.Contains(names, name)
	})
}

// awaitCategoryOrder waits until the user categories appear exactly in the
// given position order.
func awaitCategoryOrder(t *testing.T, ctx context.Context, e *harness.Emulator, want []string) {
	t.Helper()
	pollLiveDB(t, ctx, e, 60*time.Second, "category order", func(dbPath string) bool {
		db, err := harness.OpenAppDB(dbPath)
		if err != nil {
			return false
		}
		defer db.Close()
		sorts, err := harness.CategorySorts(db)
		if err != nil || len(sorts) != len(want) {
			return false
		}
		for i, c := range sorts {
			if c.Name != want[i] {
				return false
			}
		}
		return true
	})
}

func categoryNames(cats []*pb.BackupCategory) []string {
	names := make([]string, 0, len(cats))
	for _, c := range cats {
		names = append(names, c.Name)
	}
	return names
}

// snapshotCategoryOrder returns the server's category names by ascending order value.
func snapshotCategoryOrder(snap *pb.Backup) []string {
	cats := slices.Clone(snap.BackupCategories)
	slices.SortFunc(cats, func(a, b *pb.BackupCategory) int {
		return int(a.Order - b.Order)
	})
	return categoryNames(cats)
}
