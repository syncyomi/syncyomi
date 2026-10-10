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

	if c.Config.SecureCookie {
		t.Error("defaults() SecureCookie = true, want false: browsers drop a Secure cookie on plain HTTP everywhere except localhost")
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
	settled := []string{"# Check for updates", "#", "checkForUpdates = true", "# Log level", "#", "# Default: \"DEBUG\"", "#", "# Options: \"ERROR\", \"DEBUG\", \"INFO\", \"WARN\", \"TRACE\"", "#", `logLevel = "TRACE"`, "# Log Path", "#", "# Optional", "#", "#logPath = \"\""}
	withSessionSecret := func(line string) []string {
		return append(append([]string{}, settled...), line)
	}

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
		{
			name:   "appends sessionSecret when set and missing",
			config: &domain.Config{CheckForUpdates: true, LogLevel: "TRACE", SessionSecret: "abc123"},
			lines:  append([]string{}, settled...),
			want:   append(withSessionSecret("# Session secret"), "#", `sessionSecret = "abc123"`),
		},
		{
			name:   "rewrites the placeholder sessionSecret",
			config: &domain.Config{CheckForUpdates: true, LogLevel: "TRACE", SessionSecret: "abc123"},
			lines:  withSessionSecret(`sessionSecret = "secret-session-key"`),
			want:   withSessionSecret(`sessionSecret = "abc123"`),
		},
		{
			name:   "rewrites sessionSecret written without spaces",
			config: &domain.Config{CheckForUpdates: true, LogLevel: "TRACE", SessionSecret: "abc123"},
			lines:  withSessionSecret(`sessionSecret="x"`),
			want:   withSessionSecret(`sessionSecret = "abc123"`),
		},
		{
			name:   "activates a commented-out sessionSecret",
			config: &domain.Config{CheckForUpdates: true, LogLevel: "TRACE", SessionSecret: "abc123"},
			lines:  withSessionSecret(`#sessionSecret = "x"`),
			want:   withSessionSecret(`sessionSecret = "abc123"`),
		},
		{
			name:   "leaves sessionSecret lines alone when the in-memory secret is empty",
			config: &domain.Config{CheckForUpdates: true, LogLevel: "TRACE"},
			lines:  withSessionSecret(`sessionSecret = "keep"`),
			want:   withSessionSecret(`sessionSecret = "keep"`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AppConfig{Config: tt.config}

			got := c.processLines(tt.lines)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("processLines() = %v, want %v", got, tt.want)
			}
		})
	}
}
