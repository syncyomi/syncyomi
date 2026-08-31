package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	ServerPort  = 8790
	adminUser   = "e2e"
	adminPass   = "e2e-password"
	apiKeyName  = "e2e"
	startupWait = 30 * time.Second
)

// SyncServer is a SyncYomi server booted from the repo source with a fresh data dir.
type SyncServer struct {
	BaseURL string
	APIKey  string
	DataDir string
	LogPath string
	Port    int

	cmd     *exec.Cmd
	logFile *os.File
	admin   *http.Client
}

func configTOML(port int) string {
	return fmt.Sprintf(`host = "0.0.0.0"
port = %d
logLevel = "DEBUG"
sessionSecret = "e2e-session-secret"
checkForUpdates = false
databaseType = "sqlite"
`, port)
}

// StartServer builds and boots the server on the given port, onboards the
// admin user and provisions an API key.
func StartServer(ctx context.Context, repoRoot, artifactDir string, port int) (*SyncServer, error) {
	dataDir := filepath.Join(artifactDir, "server-data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dataDir, "config.toml"), []byte(configTOML(port)), 0o644); err != nil {
		return nil, err
	}

	// A real binary (not `go run`) so Stop() kills the server itself, not a wrapper.
	bin := filepath.Join(artifactDir, "syncyomi-e2e")
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("build server: %w\n%s", err, out)
	}

	logPath := filepath.Join(artifactDir, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, bin, "--config", dataDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start server: %w", err)
	}

	jar, _ := cookiejar.New(nil)
	s := &SyncServer{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", port),
		Port:    port,
		DataDir: dataDir,
		LogPath: logPath,
		cmd:     cmd,
		logFile: logFile,
		admin:   &http.Client{Jar: jar, Timeout: 15 * time.Second},
	}
	if err := s.waitReady(ctx); err != nil {
		s.Stop()
		return nil, err
	}
	if err := s.provision(ctx); err != nil {
		s.Stop()
		return nil, err
	}
	return s, nil
}

func (s *SyncServer) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(startupWait)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		resp, err := s.admin.Get(s.BaseURL + "/api/healthz/liveness")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("server not ready after %s (log: %s)", startupWait, s.LogPath)
}

func (s *SyncServer) provision(ctx context.Context) error {
	creds := map[string]string{"username": adminUser, "password": adminPass}
	if err := s.postJSON(ctx, "/api/auth/onboard", creds, nil); err != nil {
		return fmt.Errorf("onboard: %w", err)
	}
	if err := s.postJSON(ctx, "/api/auth/login", creds, nil); err != nil {
		return fmt.Errorf("login: %w", err)
	}
	var key struct {
		Key string `json:"key"`
	}
	req := map[string]any{"name": apiKeyName, "scopes": []string{}}
	if err := s.postJSON(ctx, "/api/keys", req, &key); err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	if key.Key == "" {
		return fmt.Errorf("create api key: empty key in response")
	}
	s.APIKey = key.Key
	return nil
}

func (s *SyncServer) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.admin.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: status %d", path, resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// AdminGet performs a session-authenticated GET and decodes the JSON response into out.
func (s *SyncServer) AdminGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.BaseURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := s.admin.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// HostURLForEmulator is the server URL as seen from inside an emulator.
func (s *SyncServer) HostURLForEmulator() string {
	return fmt.Sprintf("http://10.0.2.2:%d", s.Port)
}

func (s *SyncServer) Stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
	if s.logFile != nil {
		s.logFile.Close()
	}
}
