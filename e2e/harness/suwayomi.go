package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const SuwayomiPort = 4568

// Suwayomi is a headless Suwayomi-Server instance configured to sync against a
// SyncYomi server.
type Suwayomi struct {
	BaseURL string
	RootDir string
	LogPath string

	cmd     *exec.Cmd
	logFile *os.File
}

// StartSuwayomi boots the shadowJar with a fresh root dir, sync enabled against srv.
func StartSuwayomi(ctx context.Context, jarPath string, srv *SyncServer, artifactDir string) (*Suwayomi, error) {
	rootDir := filepath.Join(artifactDir, "suwayomi-data")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(artifactDir, "suwayomi.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}

	prop := func(k, v string) string { return "-Dsuwayomi.tachidesk.config.server." + k + "=" + v }
	cmd := exec.CommandContext(ctx, "java",
		prop("rootDir", rootDir),
		prop("port", fmt.Sprint(SuwayomiPort)),
		prop("systemTrayEnabled", "false"),
		prop("initialOpenInBrowserEnabled", "false"),
		prop("syncYomiEnabled", "true"),
		prop("syncYomiHost", srv.BaseURL),
		prop("syncYomiApiKey", srv.APIKey),
		prop("syncInterval", "0s"),
		"-jar", jarPath,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return nil, fmt.Errorf("start suwayomi: %w", err)
	}
	s := &Suwayomi{
		BaseURL: fmt.Sprintf("http://127.0.0.1:%d", SuwayomiPort),
		RootDir: rootDir,
		LogPath: logPath,
		cmd:     cmd,
		logFile: logFile,
	}
	if err := s.waitReady(ctx); err != nil {
		s.Stop()
		return nil, err
	}
	return s, nil
}

func (s *Suwayomi) waitReady(ctx context.Context) error {
	deadline := time.Now().Add(240 * time.Second)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var out struct {
			Data struct {
				AboutServer struct {
					Version string `json:"version"`
				} `json:"aboutServer"`
			} `json:"data"`
		}
		if err := s.GraphQL(ctx, `{ aboutServer { version } }`, nil, &out); err == nil && out.Data.AboutServer.Version != "" {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("suwayomi not ready (log: %s)", s.LogPath)
}

// GraphQL posts a query; out receives the full {data,errors} envelope.
func (s *Suwayomi) GraphQL(ctx context.Context, query string, variables map[string]any, out any) error {
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/api/graphql", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("graphql: status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// StartSync triggers a sync and returns the mutation result string.
func (s *Suwayomi) StartSync(ctx context.Context) (string, error) {
	var out struct {
		Data struct {
			StartSync struct {
				Result string `json:"result"`
			} `json:"startSync"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err := s.GraphQL(ctx, `mutation { startSync(input: {}) { result } }`, nil, &out)
	if err != nil {
		return "", err
	}
	if len(out.Errors) > 0 {
		return "", fmt.Errorf("startSync: %s", out.Errors[0].Message)
	}
	return out.Data.StartSync.Result, nil
}

// WaitForSyncSuccess polls lastSyncStatus until SUCCESS (or ERROR/timeout).
func (s *Suwayomi) WaitForSyncSuccess(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var out struct {
			Data struct {
				LastSyncStatus *struct {
					State        string  `json:"state"`
					ErrorMessage *string `json:"errorMessage"`
				} `json:"lastSyncStatus"`
			} `json:"data"`
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		if err := s.GraphQL(ctx, `{ lastSyncStatus { state errorMessage } }`, nil, &out); err == nil {
			if len(out.Errors) > 0 {
				return fmt.Errorf("lastSyncStatus query: %s", out.Errors[0].Message)
			}
			if st := out.Data.LastSyncStatus; st != nil {
				switch st.State {
				case "SUCCESS":
					return nil
				case "ERROR":
					msg := ""
					if st.ErrorMessage != nil {
						msg = *st.ErrorMessage
					}
					return fmt.Errorf("suwayomi sync failed: %s", msg)
				}
			}
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("suwayomi sync not successful within %s", timeout)
}

// SuwayomiManga is one library entry with the relations the tests assert on.
type SuwayomiManga struct {
	ID         int
	Title      string
	Categories []string
	ReadCount  int
	ChapterIDs []int
}

// Library returns the Suwayomi library with categories and chapter read state.
func (s *Suwayomi) Library(ctx context.Context) ([]SuwayomiManga, error) {
	var out struct {
		Data struct {
			Mangas struct {
				Nodes []struct {
					ID         int    `json:"id"`
					Title      string `json:"title"`
					Categories struct {
						Nodes []struct {
							Name string `json:"name"`
						} `json:"nodes"`
					} `json:"categories"`
					Chapters struct {
						Nodes []struct {
							ID     int  `json:"id"`
							IsRead bool `json:"isRead"`
						} `json:"nodes"`
					} `json:"chapters"`
				} `json:"nodes"`
			} `json:"mangas"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	query := `{ mangas(condition: {inLibrary: true}) { nodes {
		id title
		categories { nodes { name } }
		chapters { nodes { id isRead } }
	} } }`
	if err := s.GraphQL(ctx, query, nil, &out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("library query: %s", out.Errors[0].Message)
	}
	mangas := make([]SuwayomiManga, 0, len(out.Data.Mangas.Nodes))
	for _, n := range out.Data.Mangas.Nodes {
		m := SuwayomiManga{ID: n.ID, Title: n.Title}
		for _, c := range n.Categories.Nodes {
			m.Categories = append(m.Categories, c.Name)
		}
		for _, ch := range n.Chapters.Nodes {
			m.ChapterIDs = append(m.ChapterIDs, ch.ID)
			if ch.IsRead {
				m.ReadCount++
			}
		}
		mangas = append(mangas, m)
	}
	return mangas, nil
}

// LibraryTitles returns the titles of manga in the Suwayomi library.
func (s *Suwayomi) LibraryTitles(ctx context.Context) ([]string, error) {
	library, err := s.Library(ctx)
	if err != nil {
		return nil, err
	}
	titles := make([]string, 0, len(library))
	for _, m := range library {
		titles = append(titles, m.Title)
	}
	return titles, nil
}

// CategoryNames returns Suwayomi's categories by position (default included).
func (s *Suwayomi) CategoryNames(ctx context.Context) ([]string, error) {
	var out struct {
		Data struct {
			Categories struct {
				Nodes []struct {
					Name  string `json:"name"`
					Order int    `json:"order"`
				} `json:"nodes"`
			} `json:"categories"`
		} `json:"data"`
	}
	if err := s.GraphQL(ctx, `{ categories(orderBy: ORDER) { nodes { name order } } }`, nil, &out); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(out.Data.Categories.Nodes))
	for _, c := range out.Data.Categories.Nodes {
		names = append(names, c.Name)
	}
	return names, nil
}

// MarkChaptersRead flips all chapters of the given manga to read via GraphQL.
func (s *Suwayomi) MarkChaptersRead(ctx context.Context, title string) error {
	library, err := s.Library(ctx)
	if err != nil {
		return err
	}
	var ids []int
	for _, m := range library {
		if m.Title == title {
			ids = m.ChapterIDs
		}
	}
	if len(ids) == 0 {
		return fmt.Errorf("no chapters found for %q", title)
	}
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err = s.GraphQL(ctx,
		`mutation($ids: [Int!]!) { updateChapters(input: {ids: $ids, patch: {isRead: true}}) { chapters { id } } }`,
		map[string]any{"ids": ids}, &out)
	if err != nil {
		return err
	}
	if len(out.Errors) > 0 {
		return fmt.Errorf("updateChapters: %s", out.Errors[0].Message)
	}
	return nil
}

func (s *Suwayomi) Stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
	if s.logFile != nil {
		s.logFile.Close()
	}
}
