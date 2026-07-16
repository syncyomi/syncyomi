package config

import (
	"reflect"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
)

func TestAppConfig_defaults(t *testing.T) {
	c := &AppConfig{}
	c.defaults()

	if c.Config == nil {
		t.Fatal("defaults() left Config nil")
	}

	// SecureCookie must default to false: the server has no TLS, and a Secure
	// cookie is dropped by browsers on plain HTTP everywhere except localhost.
	if c.Config.SecureCookie {
		t.Error("defaults() SecureCookie = true, want false")
	}
	if c.Config.BaseURL != "/" {
		t.Errorf("defaults() BaseURL = %q, want %q", c.Config.BaseURL, "/")
	}
	if c.Config.Port != 8282 {
		t.Errorf("defaults() Port = %d, want %d", c.Config.Port, 8282)
	}
	if c.Config.Host != "localhost" {
		t.Errorf("defaults() Host = %q, want %q", c.Config.Host, "localhost")
	}
	if !c.Config.CheckForUpdates {
		t.Error("defaults() CheckForUpdates = false, want true")
	}
	if c.Config.DatabaseType != "sqlite" {
		t.Errorf("defaults() DatabaseType = %q, want %q", c.Config.DatabaseType, "sqlite")
	}
}

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
