//go:build e2e_v1

// Package v1 exercises the deprecated v1 sync protocol over real HTTP against a freshly
// built server. Unlike the main e2e suite it needs no emulators, APKs or Suwayomi, so it
// runs in minutes and lives behind its own build tag and CI workflow.
package v1

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/e2e/harness"
)

var artifactDir string

func TestMain(m *testing.M) {
	artifactDir = filepath.Join(harness.E2ERoot(), "artifacts", time.Now().Format("20060102-150405")+"-v1")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("artifacts:", artifactDir)
	os.Exit(m.Run())
}

// startServer boots a fresh SyncYomi server for one test and registers cleanup.
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
