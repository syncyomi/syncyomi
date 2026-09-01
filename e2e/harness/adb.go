package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	AppPackage  = "eu.kanade.tachiyomi.sy.debug"
	prefsFile   = AppPackage + "_preferences.xml"
	mainAct     = AppPackage + "/eu.kanade.tachiyomi.ui.main.MainActivity"
	deviceDBDir = "databases"
)

// SyncPrefs is everything the app needs to sync without touching the UI.
type SyncPrefs struct {
	Host   string
	APIKey string
}

func prefsXML(p SyncPrefs) string {
	return fmt.Sprintf(`<?xml version='1.0' encoding='utf-8' standalone='yes' ?>
<map>
    <string name="sync_client_host">%s</string>
    <string name="sync_client_api_key">%s</string>
    <int name="sync_service" value="1" />
    <boolean name="__APP_STATE_onboarding_complete" value="true" />
    <boolean name="eh_debug_toggle_enable_debug_overlay" value="false" />
    <boolean name="appSettings" value="false" />
    <boolean name="extensionRepoSettings" value="false" />
    <boolean name="sourceSettings" value="false" />
    <boolean name="privateSettings" value="false" />
</map>
`, p.Host, p.APIKey)
}

// InstallApp installs (replacing) the APK on the emulator.
func (e *Emulator) InstallApp(ctx context.Context, apkPath string) error {
	_, err := e.Adb(ctx, "install", "-r", "-g", apkPath)
	return err
}

// ResetApp wipes app data and seeds sync preferences; the app is left stopped.
func (e *Emulator) ResetApp(ctx context.Context, p SyncPrefs) error {
	if out, err := e.Adb(ctx, "shell", "pm", "clear", AppPackage); err != nil || !strings.Contains(out, "Success") {
		return fmt.Errorf("pm clear: %v %s", err, out)
	}
	if err := e.WriteSyncPrefs(ctx, p); err != nil {
		return err
	}
	// Notifications permission so no runtime prompt blocks UI flows on API 33+.
	_, _ = e.Adb(ctx, "shell", "pm", "grant", AppPackage, "android.permission.POST_NOTIFICATIONS")
	// Storage access the app would normally get during onboarding; needed to read
	// pushed backup files from /sdcard.
	_, _ = e.Adb(ctx, "shell", "appops", "set", AppPackage, "MANAGE_EXTERNAL_STORAGE", "allow")
	return nil
}

// WriteSyncPrefs force-stops the app and replaces its preferences, keeping the
// library DB — used to re-point a device at a different server mid-scenario.
// Replacing the whole file also drops v2 cursor/probe app-state, so the next
// sync against the new host starts from scratch, as a fresh pairing would.
func (e *Emulator) WriteSyncPrefs(ctx context.Context, p SyncPrefs) error {
	if err := e.ForceStopApp(ctx); err != nil {
		return err
	}
	script := fmt.Sprintf(`run-as %s sh -c 'mkdir -p shared_prefs && cat > shared_prefs/%s'`, AppPackage, prefsFile)
	if out, err := e.AdbShellStdin(ctx, prefsXML(p), script); err != nil {
		return fmt.Errorf("write prefs: %w: %s", err, out)
	}
	return nil
}

// AdbShellStdin runs a shell command with the given stdin content.
func (e *Emulator) AdbShellStdin(ctx context.Context, stdin, command string) (string, error) {
	cmd := adbCommand(ctx, e.Serial, "shell", command)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return string(out), nil
}

// LaunchApp starts the main activity and waits briefly for it to settle.
func (e *Emulator) LaunchApp(ctx context.Context) error {
	if _, err := e.Adb(ctx, "shell", "am", "start", "-n", mainAct); err != nil {
		return err
	}
	time.Sleep(3 * time.Second)
	return nil
}

func (e *Emulator) ForceStopApp(ctx context.Context) error {
	_, err := e.Adb(ctx, "shell", "am", "force-stop", AppPackage)
	return err
}

// TriggerSyncBroadcast fires the debug-only sync receiver (Phase 2 app hook).
func (e *Emulator) TriggerSyncBroadcast(ctx context.Context) error {
	out, err := e.Adb(ctx, "shell", "am", "broadcast",
		"-a", AppPackage+".TRIGGER_SYNC", "-p", AppPackage)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "Broadcast completed") {
		return fmt.Errorf("broadcast not delivered: %s", out)
	}
	return nil
}

// PushBackup copies a .tachibk fixture to the device and returns its device path.
func (e *Emulator) PushBackup(ctx context.Context, localPath string) (string, error) {
	remote := "/sdcard/Download/" + filepath.Base(localPath)
	if _, err := e.Adb(ctx, "push", localPath, remote); err != nil {
		return "", err
	}
	return remote, nil
}

// OpenBackupFile fires a VIEW intent so the app opens its restore screen.
func (e *Emulator) OpenBackupFile(ctx context.Context, devicePath string) error {
	out, err := e.Adb(ctx, "shell", "am", "start",
		"-a", "android.intent.action.VIEW",
		"-d", "file://"+devicePath,
		"-t", "application/octet-stream",
		"-n", AppPackage+"/eu.kanade.tachiyomi.ui.main.MainActivity")
	if err != nil {
		return err
	}
	if strings.Contains(out, "Error") {
		return fmt.Errorf("view intent: %s", out)
	}
	return nil
}

// PullAppDB force-stops the app and pulls tachiyomi.db (+wal/shm) into destDir,
// returning the local db path.
func (e *Emulator) PullAppDB(ctx context.Context, destDir string) (string, error) {
	if err := e.ForceStopApp(ctx); err != nil {
		return "", err
	}
	return e.pullAppDBFiles(ctx, destDir)
}

// PullAppDBLive pulls the db files without stopping the app. The copy can be
// mid-write and unreadable — callers must treat failures as "not yet" and retry.
func (e *Emulator) PullAppDBLive(ctx context.Context, destDir string) (string, error) {
	return e.pullAppDBFiles(ctx, destDir)
}

func (e *Emulator) pullAppDBFiles(ctx context.Context, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	var dbPath string
	for _, name := range []string{"tachiyomi.db", "tachiyomi.db-wal", "tachiyomi.db-shm"} {
		local := filepath.Join(destDir, name)
		cmd := adbCommand(ctx, e.Serial, "exec-out", "run-as", AppPackage, "cat", deviceDBDir+"/"+name)
		data, err := cmd.Output()
		if err != nil {
			if name == "tachiyomi.db" {
				return "", fmt.Errorf("pull %s: %w", name, err)
			}
			continue // wal/shm may not exist after clean close
		}
		if err := os.WriteFile(local, data, 0o644); err != nil {
			return "", err
		}
		if name == "tachiyomi.db" {
			dbPath = local
		}
	}
	return dbPath, nil
}

// ReadSyncPrefs returns the raw preferences XML currently on the device.
func (e *Emulator) ReadSyncPrefs(ctx context.Context) (string, error) {
	return e.Adb(ctx, "shell", "run-as", AppPackage, "cat", "shared_prefs/"+prefsFile)
}

// LastSyncTimestamp parses __APP_STATE_last_sync_timestamp from the prefs XML (0 if absent).
func (e *Emulator) LastSyncTimestamp(ctx context.Context) (int64, error) {
	xml, err := e.ReadSyncPrefs(ctx)
	if err != nil {
		return 0, err
	}
	return parseLongPref(xml, "__APP_STATE_last_sync_timestamp"), nil
}

// WaitForClientSync polls until the app's last-sync pref advances past prev
// (its value before the trigger). The app writes this pref only once it has
// fully applied the server response, so this — not the server-side device
// record — is the safe point to stop or inspect the app. Compared against the
// previous pref value rather than host wall time to sidestep clock skew.
func (e *Emulator) WaitForClientSync(ctx context.Context, prev int64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if ts, err := e.LastSyncTimestamp(ctx); err == nil && ts > prev {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("%s: client never recorded a sync within %s", e.AVD, timeout)
}

func parseLongPref(xml, name string) int64 {
	marker := fmt.Sprintf(`name="%s" value="`, name)
	i := strings.Index(xml, marker)
	if i < 0 {
		return 0
	}
	rest := xml[i+len(marker):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return 0
	}
	var v int64
	fmt.Sscanf(rest[:j], "%d", &v)
	return v
}
