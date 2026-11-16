package harness

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const restoreTestQuery = `query($id: String!) { restoreStatus(id: $id) { state } }`

func restoreTestGzip() []byte {
	return []byte("\x1f\x8b\x08\x00\x00\x00\x00\x00\x00\xff\xaa\x01\x04\x00\x00\x00\xff\xffS22-tachibk")
}

func isMultipart(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data")
}

// assertRestoreUpload checks the GraphQL multipart request the server's upload parser
// requires: operations, map, and a file part bound to the top-level backup variable.
func assertRestoreUpload(t *testing.T, r *http.Request, gz []byte) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", r.Method)
	}
	if r.URL.Path != "/api/graphql" {
		t.Errorf("path = %s, want /api/graphql", r.URL.Path)
	}
	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" || params["boundary"] == "" {
		t.Fatalf("content type %q is not multipart/form-data with a boundary: %v", r.Header.Get("Content-Type"), err)
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("parse multipart body: %v", err)
	}

	wantOperations := `{"query":"mutation($backup: Upload!) { restoreBackup(input: {backup: $backup}) { id } }","variables":{"backup":null}}`
	if got := r.FormValue("operations"); got != wantOperations {
		t.Errorf("operations = %s, want %s", got, wantOperations)
	}
	wantMap := `{"0":["variables.backup"]}`
	if got := r.FormValue("map"); got != wantMap {
		t.Errorf("map = %s, want %s", got, wantMap)
	}

	headers := r.MultipartForm.File["0"]
	if len(headers) != 1 {
		t.Fatalf("file parts = %d, want 1 part named \"0\"", len(headers))
	}
	if got := headers[0].Filename; got != "backup.tachibk" {
		t.Errorf("filename = %q, want backup.tachibk", got)
	}
	f, err := headers[0].Open()
	if err != nil {
		t.Fatalf("open file part: %v", err)
	}
	defer f.Close()
	body, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("read file part: %v", err)
	}
	if string(body) != string(gz) {
		t.Errorf("file part = %q, want %q", body, gz)
	}
}

// statusRecorder answers restoreStatus with the given states and records the ids asked for.
type statusRecorder struct {
	mu     sync.Mutex
	states []string
	ids    []string
}

func (s *statusRecorder) handle(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		t.Errorf("read status body: %v", err)
	}
	var req struct {
		Query     string `json:"query"`
		Variables struct {
			ID string `json:"id"`
		} `json:"variables"`
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		t.Errorf("status body %q is not JSON: %v", payload, err)
	}
	if req.Query != restoreTestQuery {
		t.Errorf("status query = %q, want %q", req.Query, restoreTestQuery)
	}

	s.mu.Lock()
	s.ids = append(s.ids, req.Variables.ID)
	idx := len(s.ids) - 1
	if idx >= len(s.states) {
		idx = len(s.states) - 1
	}
	state := s.states[idx]
	s.mu.Unlock()

	_, _ = io.WriteString(w, `{"data":{"restoreStatus":{"state":"`+state+`"}}}`)
}

func (s *statusRecorder) calls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.ids...)
}

func TestImportBackupMultipartUpload(t *testing.T) {
	gz := restoreTestGzip()
	status := &statusRecorder{states: []string{"SUCCESS"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMultipart(r) {
			status.handle(t, w, r)
			return
		}
		assertRestoreUpload(t, r, gz)
		_, _ = io.WriteString(w, `{"data":{"restoreBackup":{"id":"r-1"}}}`)
	}))
	defer srv.Close()

	s := &Suwayomi{BaseURL: srv.URL}
	if err := s.ImportBackup(context.Background(), gz); err != nil {
		t.Fatalf("ImportBackup: %v", err)
	}
	if ids := status.calls(); len(ids) != 1 || ids[0] != "r-1" {
		t.Errorf("restoreStatus ids = %v, want [r-1]", ids)
	}
}

func TestImportBackupPollsUntilRestoreSucceeds(t *testing.T) {
	gz := restoreTestGzip()
	status := &statusRecorder{states: []string{"IDLE", "RESTORING_MANGA", "SUCCESS"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMultipart(r) {
			status.handle(t, w, r)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"restoreBackup":{"id":"r-2"}}}`)
	}))
	defer srv.Close()

	s := &Suwayomi{BaseURL: srv.URL}
	if err := s.ImportBackup(context.Background(), gz); err != nil {
		t.Fatalf("ImportBackup: %v", err)
	}
	if ids := status.calls(); len(ids) != 3 {
		t.Errorf("restoreStatus called %d times, want 3 (IDLE, RESTORING_MANGA, SUCCESS): %v", len(ids), ids)
	}
}

func TestImportBackupRestoreFailureTerminatesPoll(t *testing.T) {
	gz := restoreTestGzip()
	status := &statusRecorder{states: []string{"RESTORING_MANGA", "FAILURE"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMultipart(r) {
			status.handle(t, w, r)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"restoreBackup":{"id":"r-3"}}}`)
	}))
	defer srv.Close()

	s := &Suwayomi{BaseURL: srv.URL}
	err := s.ImportBackup(context.Background(), gz)
	if err == nil {
		t.Fatal("ImportBackup succeeded for a FAILURE restore")
	}
	if want := "suwayomi restore r-3: state FAILURE"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if ids := status.calls(); len(ids) != 2 {
		t.Errorf("restoreStatus called %d times, want 2 (poll must stop at FAILURE): %v", len(ids), ids)
	}
}

func TestImportBackupMutationErrorSkipsPoll(t *testing.T) {
	gz := restoreTestGzip()
	status := &statusRecorder{states: []string{"SUCCESS"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMultipart(r) {
			status.handle(t, w, r)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"restoreBackup":null},"errors":[{"message":"backup: unknown source"}]}`)
	}))
	defer srv.Close()

	s := &Suwayomi{BaseURL: srv.URL}
	err := s.ImportBackup(context.Background(), gz)
	if err == nil {
		t.Fatal("ImportBackup succeeded for a mutation error response")
	}
	if want := "restoreBackup: backup: unknown source"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if ids := status.calls(); len(ids) != 0 {
		t.Errorf("restoreStatus polled %v with no restore id", ids)
	}
}

func TestImportBackupMissingRestoreID(t *testing.T) {
	gz := restoreTestGzip()
	status := &statusRecorder{states: []string{"SUCCESS"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMultipart(r) {
			status.handle(t, w, r)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"restoreBackup":{"id":""}}}`)
	}))
	defer srv.Close()

	s := &Suwayomi{BaseURL: srv.URL}
	err := s.ImportBackup(context.Background(), gz)
	if err == nil {
		t.Fatal("ImportBackup succeeded without a restore id")
	}
	if want := "restoreBackup: response carried no restore id"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
}

func TestImportBackupCleanedUpRestoreStateFailsFast(t *testing.T) {
	gz := restoreTestGzip()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMultipart(r) {
			_, _ = io.WriteString(w, `{"data":{"restoreBackup":{"id":"r-4"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"restoreStatus":null}}`)
	}))
	defer srv.Close()

	s := &Suwayomi{BaseURL: srv.URL}
	start := time.Now()
	err := s.ImportBackup(context.Background(), gz)
	if err == nil {
		t.Fatal("ImportBackup succeeded when restoreStatus returned null")
	}
	if want := "suwayomi restore r-4: server has no state for the id (restore finished and was cleaned up)"; err.Error() != want {
		t.Errorf("error = %q, want %q", err, want)
	}
	if waited := time.Since(start); waited > 10*time.Second {
		t.Errorf("took %s, want a failure instead of polling the whole timeout", waited)
	}
}

func TestImportBackupPollStopsWhenContextExpires(t *testing.T) {
	gz := restoreTestGzip()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isMultipart(r) {
			_, _ = io.WriteString(w, `{"data":{"restoreBackup":{"id":"r-5"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":{"restoreStatus":{"state":"RESTORING_MANGA"}}}`)
	}))
	defer srv.Close()

	s := &Suwayomi{BaseURL: srv.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := s.ImportBackup(ctx, gz)
	if err == nil {
		t.Fatal("ImportBackup succeeded when the context expired mid-restore")
	}
	if !strings.Contains(err.Error(), "suwayomi restore r-5") {
		t.Errorf("error = %q, want it to name restore r-5", err)
	}
	if waited := time.Since(start); waited > 10*time.Second {
		t.Errorf("took %s, want the poll to stop at the context deadline", waited)
	}
}
