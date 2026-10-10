package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/spf13/viper"
)

var hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

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
	if c.Config.SessionSecret != "" {
		t.Errorf("defaults() SessionSecret = %q, want empty so load() generates one", c.Config.SessionSecret)
	}
}

func TestAppConfig_ensureSessionSecret(t *testing.T) {
	tests := []struct {
		name        string
		secret      string
		wantChanged bool
	}{
		{name: "generates a 64-hex-char secret when empty", secret: "", wantChanged: true},
		{name: "replaces the legacy placeholder", secret: legacySessionSecret, wantChanged: true},
		{name: "keeps an operator-set secret", secret: "operator-secret", wantChanged: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &AppConfig{Config: &domain.Config{SessionSecret: tt.secret}}

			changed := c.ensureSessionSecret()

			if changed != tt.wantChanged {
				t.Errorf("ensureSessionSecret() = %t, want %t", changed, tt.wantChanged)
			}
			if !tt.wantChanged && c.Config.SessionSecret != tt.secret {
				t.Errorf("SessionSecret = %q, want %q", c.Config.SessionSecret, tt.secret)
			}
			if tt.wantChanged && !hex64.MatchString(c.Config.SessionSecret) {
				t.Errorf("SessionSecret = %q, want 64 hex chars", c.Config.SessionSecret)
			}
		})
	}
}

func writeConfigFile(t *testing.T, dir string, content string) string {
	t.Helper()
	file := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return file
}

func readConfigFile(t *testing.T, file string) string {
	t.Helper()
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func loadConfig(t *testing.T, dir string) (*AppConfig, string) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	c := New(dir, "test")

	return c, logs.String()
}

func assertSecretNotLogged(t *testing.T, logs string, secret string) {
	t.Helper()
	if secret != "" && strings.Contains(logs, secret) {
		t.Errorf("log output contains the session secret: %q", logs)
	}
}

func TestNew_sessionSecret(t *testing.T) {
	const hostOnly = "host = \"127.0.0.1\"\nport = 8282\n"

	t.Run("generates and persists a secret when the key is missing", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFile(t, dir, hostOnly)

		c, logs := loadConfig(t, dir)

		secret := c.Config.SessionSecret
		if !hex64.MatchString(secret) {
			t.Fatalf("SessionSecret = %q, want 64 hex chars", secret)
		}
		if got := readConfigFile(t, file); !strings.Contains(got, `sessionSecret = "`+secret+`"`) {
			t.Errorf("config.toml = %q, want it to contain the generated sessionSecret line", got)
		}
		if !strings.Contains(logs, "generated a new one and saved it") {
			t.Errorf("log = %q, want the generated-and-saved notice", logs)
		}
		assertSecretNotLogged(t, logs, secret)

		again, logs := loadConfig(t, dir)

		if again.Config.SessionSecret != secret {
			t.Errorf("second load SessionSecret = %q, want %q", again.Config.SessionSecret, secret)
		}
		if strings.Contains(logs, "sessionSecret") {
			t.Errorf("second load log = %q, want no sessionSecret notice", logs)
		}
	})

	t.Run("creates config.toml with a 64-hex-char secret in an empty dir", func(t *testing.T) {
		dir := t.TempDir()

		c, logs := loadConfig(t, dir)

		secret := c.Config.SessionSecret
		if !hex64.MatchString(secret) {
			t.Errorf("SessionSecret = %q, want 64 hex chars", secret)
		}
		if got := readConfigFile(t, filepath.Join(dir, "config.toml")); !strings.Contains(got, `sessionSecret = "`+secret+`"`) {
			t.Errorf("config.toml = %q, want it to contain the generated sessionSecret line", got)
		}
		if strings.Contains(logs, "sessionSecret") {
			t.Errorf("log = %q, want no sessionSecret notice on a fresh config", logs)
		}
	})

	t.Run("replaces the placeholder in the file", func(t *testing.T) {
		dir := t.TempDir()
		file := writeConfigFile(t, dir, hostOnly+`sessionSecret = "`+legacySessionSecret+"\"\n")

		c, logs := loadConfig(t, dir)

		if !hex64.MatchString(c.Config.SessionSecret) {
			t.Errorf("SessionSecret = %q, want 64 hex chars", c.Config.SessionSecret)
		}
		if got := readConfigFile(t, file); strings.Contains(got, legacySessionSecret) {
			t.Errorf("config.toml = %q, want the placeholder gone", got)
		}
		assertSecretNotLogged(t, logs, c.Config.SessionSecret)
	})

	t.Run("keeps an operator-set secret and leaves the file untouched", func(t *testing.T) {
		dir := t.TempDir()
		content := hostOnly + "sessionSecret = \"operator-secret\"\n"
		file := writeConfigFile(t, dir, content)

		c, logs := loadConfig(t, dir)

		if c.Config.SessionSecret != "operator-secret" {
			t.Errorf("SessionSecret = %q, want %q", c.Config.SessionSecret, "operator-secret")
		}
		if got := readConfigFile(t, file); got != content {
			t.Errorf("config.toml = %q, want unchanged %q", got, content)
		}
		if strings.Contains(logs, "sessionSecret") {
			t.Errorf("log = %q, want no sessionSecret notice", logs)
		}
	})

	t.Run("uses a one-off secret and warns when the file is read-only", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores file permissions")
		}
		dir := t.TempDir()
		file := writeConfigFile(t, dir, hostOnly)
		if err := os.Chmod(file, 0o444); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(file, 0o644) })

		c, logs := loadConfig(t, dir)

		secret := c.Config.SessionSecret
		if secret == "" || secret == legacySessionSecret {
			t.Errorf("SessionSecret = %q, want a generated one-off secret", secret)
		}
		if got := readConfigFile(t, file); got != hostOnly {
			t.Errorf("config.toml = %q, want unchanged %q", got, hostOnly)
		}
		if !strings.Contains(logs, "could not be saved") {
			t.Errorf("log = %q, want the could-not-be-saved warning", logs)
		}
		assertSecretNotLogged(t, logs, secret)
	})
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
