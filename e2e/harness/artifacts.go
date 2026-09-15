package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// CollectOnFailure registers a cleanup that, if the test failed, dumps logcat
// from every emulator, the client prefs, the decoded server snapshot and the
// host adb server's state into artifactDir/<testName>/.
func CollectOnFailure(t *testing.T, artifactDir string, server *SyncServer, emulators ...*Emulator) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		dir := filepath.Join(artifactDir, SanitizeName(t.Name()))
		_ = os.MkdirAll(dir, 0o755)

		for _, e := range emulators {
			if out, err := e.Adb(ctx, "logcat", "-d"); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "logcat-"+e.AVD+".txt"), []byte(out), 0o644)
			}
			if xml, err := e.ReadSyncPrefs(ctx); err == nil {
				_ = os.WriteFile(filepath.Join(dir, "prefs-"+e.AVD+".xml"), []byte(xml), 0o644)
			}
		}
		// The adb server log is the only record of why a transport to an
		// emulator was dropped; adb writes it to $TMPDIR/adb.<uid>.log.
		if data, err := os.ReadFile(filepath.Join(os.TempDir(), fmt.Sprintf("adb.%d.log", os.Getuid()))); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "adb-server.log"), data, 0o644)
		}
		if out, err := exec.CommandContext(ctx, "adb", "devices", "-l").CombinedOutput(); err == nil {
			_ = os.WriteFile(filepath.Join(dir, "adb-devices.txt"), out, 0o644)
		}
		if server != nil {
			if snap, err := server.Snapshot(ctx); err == nil {
				if data, err := json.MarshalIndent(snap, "", "  "); err == nil {
					_ = os.WriteFile(filepath.Join(dir, "server-snapshot.json"), data, 0o644)
				}
			}
			if devices, err := server.Devices(ctx); err == nil {
				if data, err := json.MarshalIndent(devices, "", "  "); err == nil {
					_ = os.WriteFile(filepath.Join(dir, "server-devices.json"), data, 0o644)
				}
			}
		}
		t.Logf("failure artifacts collected in %s", dir)
	})
}

func SanitizeName(name string) string {
	out := []rune(name)
	for i, r := range out {
		if r == '/' || r == ' ' {
			out[i] = '_'
		}
	}
	return string(out)
}
