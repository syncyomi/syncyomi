//go:build e2e

package scenarios

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
)

var (
	emuA, emuB  *harness.Emulator
	apkPath     string
	artifactDir string
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	apkPath = os.Getenv("E2E_TACHIYOMI_APK")
	if apkPath == "" {
		home, _ := os.UserHomeDir()
		apkPath = filepath.Join(home, "projects", "TachiyomiSY",
			"app", "build", "outputs", "apk", "debug", "app-x86_64-debug.apk")
	}
	if _, err := os.Stat(apkPath); err != nil {
		fmt.Fprintf(os.Stderr, "TachiyomiSY APK not found at %s (set E2E_TACHIYOMI_APK)\n", apkPath)
		return 1
	}

	artifactDir = filepath.Join(harness.E2ERoot(), "artifacts", time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Println("artifacts:", artifactDir)

	ctx := context.Background()
	fmt.Println("booting emulators (first boot can take a few minutes)...")
	type bootResult struct {
		emu *harness.Emulator
		err error
	}
	chA := make(chan bootResult, 1)
	chB := make(chan bootResult, 1)
	go func() {
		e, err := harness.StartEmulator(ctx, "syncE2E-a", 5554, artifactDir, false)
		chA <- bootResult{e, err}
	}()
	go func() {
		e, err := harness.StartEmulator(ctx, "syncE2E-b", 5556, artifactDir, false)
		chB <- bootResult{e, err}
	}()
	ra, rb := <-chA, <-chB
	emuA, emuB = ra.emu, rb.emu
	keep := os.Getenv("E2E_KEEP") == "1"
	defer func() {
		if keep {
			fmt.Println("E2E_KEEP=1: leaving emulators running")
			return
		}
		if emuA != nil {
			emuA.Stop()
		}
		if emuB != nil {
			emuB.Stop()
		}
	}()
	if ra.err != nil || rb.err != nil {
		fmt.Fprintf(os.Stderr, "emulator boot failed: a=%v b=%v\n", ra.err, rb.err)
		return 1
	}

	for _, e := range []*harness.Emulator{emuA, emuB} {
		if err := e.InstallApp(ctx, apkPath); err != nil {
			fmt.Fprintf(os.Stderr, "install on %s: %v\n", e.AVD, err)
			return 1
		}
	}
	return m.Run()
}

// startServer boots a fresh SyncYomi server on the given port for one test and
// registers cleanup. Multiple servers per test get distinct ports and data dirs.
func startServer(t *testing.T, port int) *harness.SyncServer {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dir := filepath.Join(artifactDir, harness.SanitizeName(t.Name()), fmt.Sprintf("server-%d", port))
	srv, err := harness.StartServer(ctx, harness.RepoRoot(), dir, port)
	if err != nil {
		t.Fatalf("start server on %d: %v", port, err)
	}
	t.Cleanup(srv.Stop)
	return srv
}

// seedServer pushes a backup into the server through the v2 protocol, acting as
// a synthetic device. This is the library-seeding path: the app pulls it down on
// its first sync. (In-app backup restore is unusable until jobobby04/TachiyomiSY#1634
// is fixed — restores die with SQLITE_BUSY on current builds.)
func seedServer(t *testing.T, ctx context.Context, srv *harness.SyncServer, prefix string) {
	t.Helper()
	c := harness.NewSyntheticClient(srv, "e2e-seed-"+prefix)
	if _, err := c.Merge(ctx, harness.FixtureBackup(prefix, fixtureAManga, fixtureAChapters), harness.MergeOptions{Full: true}); err != nil {
		t.Fatalf("seed %s: %v", prefix, err)
	}
}

// repoint aims a device at a different server without wiping its library, so
// locally divergent state can be built up before devices meet on one server.
func repoint(t *testing.T, ctx context.Context, e *harness.Emulator, srv *harness.SyncServer) {
	t.Helper()
	err := e.WriteSyncPrefs(ctx, harness.SyncPrefs{Host: srv.HostURLForEmulator(), APIKey: srv.APIKey})
	if err != nil {
		t.Fatalf("repoint %s: %v", e.AVD, err)
	}
}

// resetApp wipes app state on the emulator and seeds sync prefs for the server.
func resetApp(t *testing.T, ctx context.Context, e *harness.Emulator, srv *harness.SyncServer) {
	t.Helper()
	err := e.ResetApp(ctx, harness.SyncPrefs{Host: srv.HostURLForEmulator(), APIKey: srv.APIKey})
	if err != nil {
		t.Fatalf("reset app on %s: %v", e.AVD, err)
	}
}

func libraryTitles(t *testing.T, ctx context.Context, e *harness.Emulator) ([]string, error) {
	t.Helper()
	dbPath, err := e.PullAppDB(ctx, filepath.Join(artifactDir, harness.SanitizeName(t.Name()), "db-"+e.AVD))
	if err != nil {
		return nil, err
	}
	db, err := harness.OpenAppDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return harness.LibraryTitles(db)
}

// awaitLibrary polls the app DB (without stopping the app) until the library
// holds want favorites. The sync's apply step runs as a separate WorkManager
// job after SyncDataJob reports success, so server records and the last-sync
// pref both fire before data lands — the DB itself is the only honest signal.
func awaitLibrary(t *testing.T, ctx context.Context, e *harness.Emulator, want int) {
	t.Helper()
	awaitLibraryFor(t, ctx, e, want, 60*time.Second)
}
