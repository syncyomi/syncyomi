//go:build e2e

package scenarios

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
)

// TestS7_AndroidSuwayomiBothDirections: Android app syncs its library up, the
// Suwayomi server pulls it in, and Android picks up a Suwayomi-side sync back.
func TestS7_AndroidSuwayomiBothDirections(t *testing.T) {
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

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	srv := startServer(t, mainPort)
	harness.CollectOnFailure(t, artifactDir, srv, emuA)
	seedServer(t, ctx, srv, "E2E Alpha")

	resetApp(t, ctx, emuA, srv)
	syncViaBroadcast(t, ctx, emuA, srv)
	awaitLibrary(t, ctx, emuA, fixtureAManga)

	suwa, err := harness.StartSuwayomi(ctx, jar, srv, filepath.Join(artifactDir, harness.SanitizeName(t.Name())))
	if err != nil {
		t.Fatalf("start suwayomi: %v", err)
	}
	t.Cleanup(suwa.Stop)

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

	// Reverse direction: another Suwayomi sync then an Android sync should both
	// succeed and leave every side converged on the same library.
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
