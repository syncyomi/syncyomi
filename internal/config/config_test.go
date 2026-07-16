package config

import (
	"reflect"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
)

func TestAppConfig_processLines(t *testing.T) {
	tests := []struct {
		name   string
		config *domain.Config
		lines  []string
		want   []string
	}{
		{
			name:   "append missing",
			config: &domain.Config{CheckForUpdates: true, LogLevel: "TRACE"},
			lines:  []string{},
			want:   []string{"# Check for updates", "#", "checkForUpdates = true", "# Log level", "#", "# Default: \"DEBUG\"", "#", "# Options: \"ERROR\", \"DEBUG\", \"INFO\", \"WARN\", \"TRACE\"", "#", `logLevel = "TRACE"`, "# Log Path", "#", "# Optional", "#", "#logPath = \"\""},
		},
		{
			name:   "update existing",
			config: &domain.Config{CheckForUpdates: true, LogLevel: "TRACE"},
			lines:  []string{"# Check for updates", "#", "checkForUpdates = false", "# Log level", "#", "# Default: \"DEBUG\"", "#", "# Options: \"ERROR\", \"DEBUG\", \"INFO\", \"WARN\", \"TRACE\"", "#", `logLevel = "TRACE"`, "# Log Path", "#", "# Optional", "#", "#logPath = \"\""},
			want:   []string{"# Check for updates", "#", "checkForUpdates = true", "# Log level", "#", "# Default: \"DEBUG\"", "#", "# Options: \"ERROR\", \"DEBUG\", \"INFO\", \"WARN\", \"TRACE\"", "#", `logLevel = "TRACE"`, "# Log Path", "#", "# Optional", "#", "#logPath = \"\""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// processLines never touches c.m, so the zero-value mutex is fine here.
			c := &AppConfig{Config: tt.config}

			got := c.processLines(tt.lines)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processLines() = %v, want %v", got, tt.want)
			}
		})
	}
}
