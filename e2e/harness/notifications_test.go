package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func loadDump(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseSyncError(t *testing.T) {
	tests := []struct {
		name string
		dump string
		want SyncError
	}{
		{
			name: "reads the failed sync's time and message",
			dump: loadDump(t, "dumpsys_notification_sync_failed.txt"),
			want: SyncError{When: 1790473678527, Text: "Failed to connect to /10.0.2.2:8799"},
		},
		{
			name: "ignores the in-progress sync notification",
			dump: loadDump(t, "dumpsys_notification_sync_running.txt"),
			want: SyncError{},
		},
		{
			name: "returns nothing when no notification is posted",
			dump: "",
			want: SyncError{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSyncError(tc.dump)

			if got != tc.want {
				t.Errorf("parseSyncError() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestSyncErrorTransient(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"a failed connect never reached the server", "Failed to connect to /10.0.2.2:8790", true},
		{"a connect timeout never reached the server", "failed to connect to /10.0.2.2 (port 8790) from /:: (port 0) after 30000ms", true},
		{"a read timeout may have reached the server", "timeout", false},
		{"a server error reached the server", "Failed to sync: 500 internal error", false},
		{"an empty message is not a connect failure", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SyncError{Text: tc.text}.Transient()

			if got != tc.want {
				t.Errorf("SyncError{Text: %q}.Transient() = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
