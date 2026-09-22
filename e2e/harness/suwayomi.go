package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const SuwayomiPort = 4568

type Suwayomi struct {
	BaseURL string
	RootDir string
	LogPath string

	cmd     *exec.Cmd
	logFile *os.File
}

func StartSuwayomi(ctx context.Context, jarPath string, srv *SyncServer, artifactDir string, jvmArgs ...string) (*Suwayomi, error) {
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
	args := []string{
		prop("rootDir", rootDir),
		prop("port", fmt.Sprint(SuwayomiPort)),
		prop("systemTrayEnabled", "false"),
		prop("initialOpenInBrowserEnabled", "false"),
		prop("syncYomiEnabled", "true"),
		prop("syncYomiHost", srv.BaseURL),
		prop("syncYomiApiKey", srv.APIKey),
		prop("syncInterval", "0s"),
	}
	args = append(args, jvmArgs...)
	args = append(args, "-jar", jarPath)
	cmd := exec.CommandContext(ctx, "java", args...)
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

func (s *Suwayomi) ImportBackup(ctx context.Context, gz []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.BaseURL+"/api/v1/backup/import", bytes.NewReader(gz))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("import backup: status %d: %s", resp.StatusCode, payload)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

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

type SuwayomiManga struct {
	ID         int
	Title      string
	Categories []string
	ReadCount  int
	ChapterIDs []int
}

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

func (s *Suwayomi) LibraryMangaCount(ctx context.Context) (int, error) {
	var out struct {
		Data struct {
			Mangas struct {
				TotalCount int `json:"totalCount"`
			} `json:"mangas"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	query := `{ mangas(condition: {inLibrary: true}) { totalCount } }`
	if err := s.GraphQL(ctx, query, nil, &out); err != nil {
		return 0, err
	}
	if len(out.Errors) > 0 {
		return 0, fmt.Errorf("manga count query: %s", out.Errors[0].Message)
	}
	return out.Data.Mangas.TotalCount, nil
}

func (s *Suwayomi) SampleMangaIDs(ctx context.Context, n int) ([]int, error) {
	var out struct {
		Data struct {
			Mangas struct {
				Nodes []struct {
					ID int `json:"id"`
				} `json:"nodes"`
			} `json:"mangas"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	query := `query($n: Int!) { mangas(condition: {inLibrary: true}, first: $n) { nodes { id } } }`
	if err := s.GraphQL(ctx, query, map[string]any{"n": n}, &out); err != nil {
		return nil, err
	}
	if len(out.Errors) > 0 {
		return nil, fmt.Errorf("manga sample query: %s", out.Errors[0].Message)
	}
	ids := make([]int, 0, len(out.Data.Mangas.Nodes))
	for _, m := range out.Data.Mangas.Nodes {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

func (s *Suwayomi) ChapterCount(ctx context.Context, mangaID int) (int, error) {
	var out struct {
		Data struct {
			Chapters struct {
				TotalCount int `json:"totalCount"`
			} `json:"chapters"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	query := `query($id: Int!) { chapters(condition: {mangaId: $id}) { totalCount } }`
	if err := s.GraphQL(ctx, query, map[string]any{"id": mangaID}, &out); err != nil {
		return 0, err
	}
	if len(out.Errors) > 0 {
		return 0, fmt.Errorf("chapter count query: %s", out.Errors[0].Message)
	}
	return out.Data.Chapters.TotalCount, nil
}

func (s *Suwayomi) OOMCount() int {
	data, err := os.ReadFile(s.LogPath)
	if err != nil {
		return 0
	}
	return strings.Count(string(data), "OutOfMemoryError")
}

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

func (s *Suwayomi) CreateCategoryAt(ctx context.Context, name string, position int) (int, error) {
	var out struct {
		Data struct {
			CreateCategory struct {
				Category struct {
					ID int `json:"id"`
				} `json:"category"`
			} `json:"createCategory"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err := s.GraphQL(ctx,
		`mutation($name: String!, $order: Int) { createCategory(input: {name: $name, order: $order}) { category { id } } }`,
		map[string]any{"name": name, "order": position}, &out)
	if err != nil {
		return 0, err
	}
	if len(out.Errors) > 0 {
		return 0, fmt.Errorf("createCategory: %s", out.Errors[0].Message)
	}
	return out.Data.CreateCategory.Category.ID, nil
}

func (s *Suwayomi) AddMangaToCategory(ctx context.Context, title, category string) error {
	library, err := s.Library(ctx)
	if err != nil {
		return err
	}
	mangaID := 0
	for _, m := range library {
		if m.Title == title {
			mangaID = m.ID
		}
	}
	if mangaID == 0 {
		return fmt.Errorf("no library manga titled %q", title)
	}
	var cats struct {
		Data struct {
			Categories struct {
				Nodes []struct {
					ID   int    `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"categories"`
		} `json:"data"`
	}
	if err := s.GraphQL(ctx, `{ categories { nodes { id name } } }`, nil, &cats); err != nil {
		return err
	}
	categoryID := -1
	for _, c := range cats.Data.Categories.Nodes {
		if c.Name == category {
			categoryID = c.ID
		}
	}
	if categoryID < 0 {
		return fmt.Errorf("no category named %q", category)
	}
	var out struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	err = s.GraphQL(ctx,
		`mutation($id: Int!, $cats: [Int!]!) { updateMangaCategories(input: {id: $id, patch: {addToCategories: $cats}}) { clientMutationId } }`,
		map[string]any{"id": mangaID, "cats": []int{categoryID}}, &out)
	if err != nil {
		return err
	}
	if len(out.Errors) > 0 {
		return fmt.Errorf("updateMangaCategories: %s", out.Errors[0].Message)
	}
	return nil
}

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
		_ = s.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			_ = s.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			_ = s.cmd.Process.Kill()
			<-done
		}
	}
	if s.logFile != nil {
		s.logFile.Close()
	}
}
