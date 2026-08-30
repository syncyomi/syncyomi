package http

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/sync"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type mockSyncService struct {
	// v1
	snapshot    *sync.Snapshot
	snapshotErr error
	putEtag     string
	putErr      error
	putData     []byte
	putIfMatch  string
	putDevice   domain.DeviceInfo
	// v2
	mergeResp      *sync.MergeResponse
	mergeErr       error
	mergeReq       *sync.MergeRequest
	reportEventErr error

	reportedDevice domain.DeviceInfo
	accessDevice   domain.DeviceInfo
	accessWrite    bool
	accessCalls    int

	adminErr    error
	history     []domain.SyncHistoryEntry
	devices     []domain.SyncDevice
	status      *domain.SyncStatus
	restoreEtag *string
}

func (m *mockSyncService) Merge(ctx context.Context, req sync.MergeRequest) (*sync.MergeResponse, error) {
	m.mergeReq = &req
	if m.mergeErr != nil {
		return nil, m.mergeErr
	}
	return m.mergeResp, nil
}

func (m *mockSyncService) Snapshot(ctx context.Context, apiKey string, cursor int64) (*sync.Snapshot, error) {
	if m.snapshotErr != nil {
		return nil, m.snapshotErr
	}
	return m.snapshot, nil
}

func (m *mockSyncService) GetContent(ctx context.Context, apiKey string) (*sync.Snapshot, error) {
	return m.Snapshot(ctx, apiKey, 0)
}

func (m *mockSyncService) PutContent(ctx context.Context, apiKey string, dev domain.DeviceInfo, ifMatch string, data []byte) (string, error) {
	m.putData, m.putIfMatch, m.putDevice = data, ifMatch, dev
	if m.putErr != nil {
		return "", m.putErr
	}
	return m.putEtag, nil
}

func (m *mockSyncService) ReportSyncEvent(ctx context.Context, apiKey string, event string, dev domain.DeviceInfo, detailMessage string) error {
	m.reportedDevice = dev
	return m.reportEventErr
}

func (m *mockSyncService) RecordContentAccess(ctx context.Context, apiKey string, dev domain.DeviceInfo, write bool) {
	m.accessDevice = dev
	m.accessWrite = write
	m.accessCalls++
}

func (m *mockSyncService) ListHistory(ctx context.Context, apiKey string) ([]domain.SyncHistoryEntry, error) {
	return m.history, m.adminErr
}

func (m *mockSyncService) RestoreHistory(ctx context.Context, apiKey string, id int) (*string, error) {
	if m.adminErr != nil {
		return nil, m.adminErr
	}
	if m.restoreEtag == nil {
		return nil, domain.ErrNotFound
	}
	return m.restoreEtag, nil
}

func (m *mockSyncService) ListDevices(ctx context.Context, apiKey string) ([]domain.SyncDevice, error) {
	return m.devices, m.adminErr
}

func (m *mockSyncService) GetStatus(ctx context.Context, apiKey string) (*domain.SyncStatus, error) {
	return m.status, m.adminErr
}

func newRouter(mock *mockSyncService, maxBody int64) *chi.Mux {
	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		newSyncHandler(encoder{}, zerolog.Nop(), mock, maxBody).Routes(r)
	})
	return r
}

func encodedBackup(t *testing.T, urls ...string) []byte {
	t.Helper()
	b := &pb.Backup{}
	for _, u := range urls {
		b.BackupManga = append(b.BackupManga, &pb.BackupManga{Source: 1, Url: u})
	}
	data, err := backup.Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSyncHandler_getContent(t *testing.T) {
	tests := []struct {
		name        string
		ifNoneMatch string
		mock        *mockSyncService
		wantStatus  int
		wantETag    string
		wantBody    string
	}{
		{name: "no data returns 404", mock: &mockSyncService{snapshotErr: sync.ErrNoData}, wantStatus: http.StatusNotFound},
		{name: "returns data and etag", mock: &mockSyncService{snapshot: &sync.Snapshot{Data: []byte("sync-payload"), ETag: "seq=1"}}, wantStatus: http.StatusOK, wantETag: "seq=1", wantBody: "sync-payload"},
		{name: "304 when If-None-Match matches", ifNoneMatch: "seq=1", mock: &mockSyncService{snapshot: &sync.Snapshot{Data: []byte("x"), ETag: "seq=1"}}, wantStatus: http.StatusNotModified},
		{name: "200 when If-None-Match differs", ifNoneMatch: "uuid=old", mock: &mockSyncService{snapshot: &sync.Snapshot{Data: []byte("x"), ETag: "seq=1"}}, wantStatus: http.StatusOK, wantETag: "seq=1"},
		{name: "500 on error", mock: &mockSyncService{snapshotErr: errors.New("db")}, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/content", nil)
			req.Header.Set("X-API-Token", "key1")
			if tt.ifNoneMatch != "" {
				req.Header.Set("If-None-Match", tt.ifNoneMatch)
			}
			rec := httptest.NewRecorder()
			newRouter(tt.mock, 0).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if tt.wantETag != "" && rec.Header().Get("ETag") != tt.wantETag {
				t.Errorf("ETag = %q, want %q", rec.Header().Get("ETag"), tt.wantETag)
			}
			if tt.wantBody != "" && rec.Body.String() != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
			if rec.Header().Get("Deprecation") != "true" {
				t.Error("v1 response lacks Deprecation header")
			}
		})
	}
}

func TestSyncHandler_putContent(t *testing.T) {
	tests := []struct {
		name       string
		ifMatch    string
		mock       *mockSyncService
		wantStatus int
		wantETag   string
	}{
		{name: "put returns new etag", mock: &mockSyncService{putEtag: "seq=2"}, wantStatus: http.StatusOK, wantETag: "seq=2"},
		{name: "412 on etag mismatch", ifMatch: "seq=1", mock: &mockSyncService{putErr: sync.ErrPreconditionFailed}, wantStatus: http.StatusPreconditionFailed},
		{name: "400 on undecodable payload", mock: &mockSyncService{putErr: sync.ErrBadPayload}, wantStatus: http.StatusBadRequest},
		{name: "500 on error", mock: &mockSyncService{putErr: errors.New("db")}, wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader([]byte("payload")))
			req.Header.Set("X-API-Token", "key1")
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			rec := httptest.NewRecorder()
			newRouter(tt.mock, 0).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if rec.Header().Get("ETag") != tt.wantETag {
				t.Errorf("ETag = %q, want %q", rec.Header().Get("ETag"), tt.wantETag)
			}
			if tt.ifMatch != "" && tt.mock.putIfMatch != tt.ifMatch {
				t.Errorf("If-Match passed = %q", tt.mock.putIfMatch)
			}
		})
	}
}

func TestSyncHandler_putContentBodyLimit(t *testing.T) {
	r := newRouter(&mockSyncService{putEtag: "seq=1"}, 16)

	send := func(body []byte, gzipped bool) int {
		req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(body))
		req.Header.Set("X-API-Token", "key1")
		if gzipped {
			req.Header.Set("Content-Encoding", "gzip")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := send(make([]byte, 16), false); got != http.StatusOK {
		t.Errorf("under limit = %d", got)
	}
	if got := send(make([]byte, 17), false); got != http.StatusRequestEntityTooLarge {
		t.Errorf("over limit = %d", got)
	}
	if got := send(gzipBytes(t, make([]byte, 1024)), true); got != http.StatusRequestEntityTooLarge {
		t.Errorf("inflated over limit = %d", got)
	}
}

func TestSyncHandler_putContentGzip(t *testing.T) {
	mock := &mockSyncService{putEtag: "seq=1"}
	req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(gzipBytes(t, []byte("plain-payload"))))
	req.Header.Set("X-API-Token", "key1")
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	newRouter(mock, 0).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %v", rec.Code)
	}
	if !bytes.Equal(mock.putData, []byte("plain-payload")) {
		t.Errorf("stored = %q", mock.putData)
	}
}

func TestSyncHandler_deviceHeaders(t *testing.T) {
	mock := &mockSyncService{putEtag: "seq=1"}
	r := newRouter(mock, 0)

	req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader([]byte("x")))
	req.Header.Set("X-API-Token", "key1")
	req.Header.Set("X-Device-ID", "dev-1")
	req.Header.Set("X-Device-Name", "Phone")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %v", rec.Code)
	}
	want := domain.DeviceInfo{ID: "dev-1", Name: "Phone"}
	if mock.putDevice != want || mock.accessDevice != want || !mock.accessWrite {
		t.Errorf("device put=%+v access=%+v write=%v", mock.putDevice, mock.accessDevice, mock.accessWrite)
	}

	body, _ := json.Marshal(map[string]string{"event": "SYNC_SUCCESS", "device_id": "dev-2", "device_name": "Tablet"})
	req = httptest.NewRequest(http.MethodPost, "/event", bytes.NewReader(body))
	req.Header.Set("X-API-Token", "key1")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("event status = %v", rec.Code)
	}
	if mock.reportedDevice != (domain.DeviceInfo{ID: "dev-2", Name: "Tablet"}) {
		t.Errorf("event device = %+v", mock.reportedDevice)
	}
}

func TestSyncHandler_reportEvent(t *testing.T) {
	tests := []struct {
		name       string
		apiKeyIn   string
		body       any
		mock       *mockSyncService
		wantStatus int
		wantBody   string
	}{
		{name: "401 when no API key", body: map[string]string{"event": "SYNC_STARTED"}, mock: &mockSyncService{}, wantStatus: http.StatusUnauthorized},
		{name: "400 when invalid JSON", apiKeyIn: "header", body: "not json", mock: &mockSyncService{}, wantStatus: http.StatusBadRequest},
		{name: "400 when event missing", apiKeyIn: "header", body: map[string]string{}, mock: &mockSyncService{}, wantStatus: http.StatusBadRequest, wantBody: "event is required"},
		{name: "400 when invalid event", apiKeyIn: "header", body: map[string]string{"event": "INVALID_EVENT"}, mock: &mockSyncService{reportEventErr: sync.ErrInvalidSyncEvent}, wantStatus: http.StatusBadRequest, wantBody: "invalid sync event"},
		{name: "204 with header API key", apiKeyIn: "header", body: map[string]string{"event": "SYNC_STARTED"}, mock: &mockSyncService{}, wantStatus: http.StatusNoContent},
		{name: "204 with query API key", apiKeyIn: "query", body: map[string]string{"event": "SYNC_SUCCESS", "device_name": "My Phone"}, mock: &mockSyncService{}, wantStatus: http.StatusNoContent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			switch b := tt.body.(type) {
			case string:
				bodyBytes = []byte(b)
			default:
				bodyBytes, _ = json.Marshal(b)
			}
			req := httptest.NewRequest(http.MethodPost, "/event", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			switch tt.apiKeyIn {
			case "header":
				req.Header.Set("X-API-Token", "key1")
			case "query":
				req.URL.RawQuery = "apikey=key1"
			}
			rec := httptest.NewRecorder()
			newRouter(tt.mock, 0).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBody)) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestSyncHandler_adminRoutes(t *testing.T) {
	newAdmin := func(mock *mockSyncService) *chi.Mux {
		r := chi.NewRouter()
		r.Route("/", func(r chi.Router) {
			newSyncHandler(encoder{}, zerolog.Nop(), mock, 0).AdminRoutes(r)
		})
		return r
	}

	t.Run("status 404 when unknown", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newAdmin(&mockSyncService{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/key1/status", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %v", rec.Code)
		}
	})

	t.Run("status returns json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newAdmin(&mockSyncService{status: &domain.SyncStatus{LastStatus: "success", DataSize: 42}}).
			ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/key1/status", nil))
		var got domain.SyncStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.LastStatus != "success" || got.DataSize != 42 {
			t.Errorf("status = %v body = %+v err = %v", rec.Code, got, err)
		}
	})

	t.Run("devices and history return arrays", func(t *testing.T) {
		mock := &mockSyncService{devices: []domain.SyncDevice{{DeviceID: "d1"}}, history: []domain.SyncHistoryEntry{{ID: 7}}}
		for _, path := range []string{"/key1/devices", "/key1/history"} {
			rec := httptest.NewRecorder()
			newAdmin(mock).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK || !bytes.HasPrefix(bytes.TrimSpace(rec.Body.Bytes()), []byte("[")) {
				t.Errorf("%s = %v %q", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("restore", func(t *testing.T) {
		for _, tc := range []struct {
			path string
			mock *mockSyncService
			want int
		}{
			{"/key1/history/1/restore", &mockSyncService{}, http.StatusNotFound},
			{"/key1/history/abc/restore", &mockSyncService{}, http.StatusBadRequest},
			{"/key1/history/1/restore", &mockSyncService{restoreEtag: strPtr("seq=9")}, http.StatusOK},
			{"/key1/history/1/restore", &mockSyncService{adminErr: sync.ErrBadPayload}, http.StatusUnprocessableEntity},
		} {
			rec := httptest.NewRecorder()
			newAdmin(tc.mock).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, tc.path, nil))
			if rec.Code != tc.want {
				t.Errorf("%s = %v, want %v", tc.path, rec.Code, tc.want)
			}
		}
	})
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func strPtr(s string) *string { return &s }
