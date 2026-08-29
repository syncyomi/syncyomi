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

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/sync"
	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

type mockSyncService struct {
	getETagErr         error
	getETag            *string
	getDataAndETagErr  error
	getData            []byte
	getDataETag        *string
	setDataErr         error
	setDataEtag        *string
	onSetData          func(data []byte)
	setDataIfMatchErr  error
	setDataIfMatchEtag *string
	reportEventErr     error

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

func (m *mockSyncService) GetSyncDataETag(ctx context.Context, apiKey string) (*string, error) {
	if m.getETagErr != nil {
		return nil, m.getETagErr
	}
	return m.getETag, nil
}

func (m *mockSyncService) GetSyncDataAndETag(ctx context.Context, apiKey string) ([]byte, *string, error) {
	if m.getDataAndETagErr != nil {
		return nil, nil, m.getDataAndETagErr
	}
	return m.getData, m.getDataETag, nil
}

func (m *mockSyncService) SetSyncData(ctx context.Context, apiKey string, data []byte) (*string, error) {
	if m.setDataErr != nil {
		return nil, m.setDataErr
	}
	if m.onSetData != nil {
		m.onSetData(data)
	}
	return m.setDataEtag, nil
}

func (m *mockSyncService) SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error) {
	if m.setDataIfMatchErr != nil {
		return nil, m.setDataIfMatchErr
	}
	return m.setDataIfMatchEtag, nil
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

func TestSyncHandler_getContent(t *testing.T) {
	enc := encoder{}
	tests := []struct {
		name           string
		apiKey         string
		ifNoneMatch    string
		mock           *mockSyncService
		wantStatus     int
		wantETag       string
		wantBodyPrefix string
	}{
		{
			name:       "no data returns 404",
			apiKey:     "key1",
			mock:       &mockSyncService{getData: nil, getDataETag: nil},
			wantStatus: http.StatusNotFound,
		},
		{
			name:           "returns data and etag",
			apiKey:         "key1",
			mock:           &mockSyncService{getData: []byte("sync-payload"), getDataETag: strPtr("etag-1")},
			wantStatus:     http.StatusOK,
			wantETag:       "etag-1",
			wantBodyPrefix: "sync-payload",
		},
		{
			name:        "304 when If-None-Match matches",
			apiKey:      "key1",
			ifNoneMatch: "etag-1",
			mock:        &mockSyncService{getETag: strPtr("etag-1")},
			wantStatus:  http.StatusNotModified,
		},
		{
			name:        "200 when If-None-Match does not match",
			apiKey:      "key1",
			ifNoneMatch: "old-etag",
			mock:        &mockSyncService{getETag: strPtr("etag-1"), getData: []byte("data"), getDataETag: strPtr("etag-1")},
			wantStatus:  http.StatusOK,
			wantETag:    "etag-1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Route("/", func(r chi.Router) {
				newSyncHandler(enc, zerolog.Nop(), tt.mock, 0).Routes(r)
			})
			req := httptest.NewRequest(http.MethodGet, "/content", nil)
			req.Header.Set("X-API-Token", tt.apiKey)
			if tt.ifNoneMatch != "" {
				req.Header.Set("If-None-Match", tt.ifNoneMatch)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("getContent() status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if tt.wantETag != "" && rec.Header().Get("ETag") != tt.wantETag {
				t.Errorf("ETag = %q, want %q", rec.Header().Get("ETag"), tt.wantETag)
			}
			if tt.wantBodyPrefix != "" && !bytes.HasPrefix(rec.Body.Bytes(), []byte(tt.wantBodyPrefix)) {
				t.Errorf("body = %q, want prefix %q", rec.Body.String(), tt.wantBodyPrefix)
			}
		})
	}
}

func TestSyncHandler_putContent(t *testing.T) {
	enc := encoder{}
	tests := []struct {
		name       string
		apiKey     string
		ifMatch    string
		body       []byte
		mock       *mockSyncService
		wantStatus int
		wantETag   string
	}{
		{
			name:       "put without etag returns 200 and new etag",
			apiKey:     "key1",
			body:       []byte("new-sync-data"),
			mock:       &mockSyncService{setDataEtag: strPtr("etag-new")},
			wantStatus: http.StatusOK,
			wantETag:   "etag-new",
		},
		{
			name:       "put with If-Match returns 412 when etag mismatch",
			apiKey:     "key1",
			ifMatch:    "old-etag",
			body:       []byte("new-sync-data"),
			mock:       &mockSyncService{setDataIfMatchEtag: nil},
			wantStatus: http.StatusPreconditionFailed,
		},
		{
			name:       "put with If-Match returns 200 when match",
			apiKey:     "key1",
			ifMatch:    "old-etag",
			body:       []byte("new-sync-data"),
			mock:       &mockSyncService{setDataIfMatchEtag: strPtr("etag-after")},
			wantStatus: http.StatusOK,
			wantETag:   "etag-after",
		},
		{
			name:       "repo error returns 500 only",
			apiKey:     "key1",
			body:       []byte("new-sync-data"),
			mock:       &mockSyncService{setDataErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "repo error with If-Match returns 500 not 412",
			apiKey:     "key1",
			ifMatch:    "old-etag",
			body:       []byte("new-sync-data"),
			mock:       &mockSyncService{setDataIfMatchErr: errors.New("db down")},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Route("/", func(r chi.Router) {
				newSyncHandler(enc, zerolog.Nop(), tt.mock, 0).Routes(r)
			})
			req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(tt.body))
			req.Header.Set("X-API-Token", tt.apiKey)
			if tt.ifMatch != "" {
				req.Header.Set("If-Match", tt.ifMatch)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("putContent() status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if tt.wantETag != "" && rec.Header().Get("ETag") != tt.wantETag {
				t.Errorf("ETag = %q, want %q", rec.Header().Get("ETag"), tt.wantETag)
			}
			if tt.wantStatus == http.StatusInternalServerError && rec.Header().Get("ETag") != "" {
				t.Errorf("ETag set on error response: %q", rec.Header().Get("ETag"))
			}
		})
	}
}

func TestSyncHandler_putContentBodyLimit(t *testing.T) {
	enc := encoder{}
	mock := &mockSyncService{setDataEtag: strPtr("etag-new")}
	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		newSyncHandler(enc, zerolog.Nop(), mock, 16).Routes(r)
	})

	t.Run("under limit is accepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(make([]byte, 16)))
		req.Header.Set("X-API-Token", "key1")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("status = %v, want 200", rec.Code)
		}
	})

	t.Run("over limit is rejected with 413", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(make([]byte, 17)))
		req.Header.Set("X-API-Token", "key1")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %v, want 413", rec.Code)
		}
	})

	t.Run("gzip body over limit after inflation is rejected with 413", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(gzipBytes(t, make([]byte, 1024))))
		req.Header.Set("X-API-Token", "key1")
		req.Header.Set("Content-Encoding", "gzip")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("status = %v, want 413", rec.Code)
		}
	})
}

func TestSyncHandler_putContentGzip(t *testing.T) {
	enc := encoder{}
	var stored []byte
	mock := &mockSyncService{setDataEtag: strPtr("etag-new")}
	mock.onSetData = func(data []byte) { stored = data }
	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		newSyncHandler(enc, zerolog.Nop(), mock, 0).Routes(r)
	})

	req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader(gzipBytes(t, []byte("plain-payload"))))
	req.Header.Set("X-API-Token", "key1")
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %v, want 200", rec.Code)
	}
	if !bytes.Equal(stored, []byte("plain-payload")) {
		t.Errorf("stored = %q, want inflated payload", stored)
	}
}

func TestSyncHandler_deviceHeaders(t *testing.T) {
	enc := encoder{}
	mock := &mockSyncService{setDataEtag: strPtr("etag-new")}
	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		newSyncHandler(enc, zerolog.Nop(), mock, 0).Routes(r)
	})

	req := httptest.NewRequest(http.MethodPut, "/content", bytes.NewReader([]byte("x")))
	req.Header.Set("X-API-Token", "key1")
	req.Header.Set("X-Device-ID", "dev-1")
	req.Header.Set("X-Device-Name", "Phone")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %v, want 200", rec.Code)
	}
	if mock.accessCalls != 1 || !mock.accessWrite {
		t.Errorf("RecordContentAccess calls=%d write=%v, want 1/true", mock.accessCalls, mock.accessWrite)
	}
	if mock.accessDevice != (domain.DeviceInfo{ID: "dev-1", Name: "Phone"}) {
		t.Errorf("device = %+v", mock.accessDevice)
	}

	// event: body fields are used when headers are absent
	body, _ := json.Marshal(map[string]string{"event": "SYNC_SUCCESS", "device_id": "dev-2", "device_name": "Tablet"})
	req = httptest.NewRequest(http.MethodPost, "/event", bytes.NewReader(body))
	req.Header.Set("X-API-Token", "key1")
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("event status = %v, want 204", rec.Code)
	}
	if mock.reportedDevice != (domain.DeviceInfo{ID: "dev-2", Name: "Tablet"}) {
		t.Errorf("event device = %+v", mock.reportedDevice)
	}
}

func TestSyncHandler_adminRoutes(t *testing.T) {
	enc := encoder{}
	newRouter := func(mock *mockSyncService) *chi.Mux {
		r := chi.NewRouter()
		r.Route("/", func(r chi.Router) {
			newSyncHandler(enc, zerolog.Nop(), mock, 0).AdminRoutes(r)
		})
		return r
	}

	t.Run("status 404 when unknown", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newRouter(&mockSyncService{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/key1/status", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %v, want 404", rec.Code)
		}
	})

	t.Run("status returns json", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newRouter(&mockSyncService{status: &domain.SyncStatus{LastStatus: "success", DataSize: 42}}).
			ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/key1/status", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %v, want 200", rec.Code)
		}
		var got domain.SyncStatus
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.LastStatus != "success" || got.DataSize != 42 {
			t.Errorf("body = %+v", got)
		}
	})

	t.Run("devices and history return arrays", func(t *testing.T) {
		mock := &mockSyncService{
			devices: []domain.SyncDevice{{DeviceID: "d1"}},
			history: []domain.SyncHistoryEntry{{ID: 7, ETag: "e"}},
		}
		for _, path := range []string{"/key1/devices", "/key1/history"} {
			rec := httptest.NewRecorder()
			newRouter(mock).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Errorf("%s status = %v, want 200", path, rec.Code)
			}
			if !bytes.HasPrefix(bytes.TrimSpace(rec.Body.Bytes()), []byte("[")) {
				t.Errorf("%s body = %q, want array", path, rec.Body.String())
			}
		}
	})

	t.Run("restore", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newRouter(&mockSyncService{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/key1/history/1/restore", nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("unknown id status = %v, want 404", rec.Code)
		}

		rec = httptest.NewRecorder()
		newRouter(&mockSyncService{}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/key1/history/abc/restore", nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("bad id status = %v, want 400", rec.Code)
		}

		rec = httptest.NewRecorder()
		newRouter(&mockSyncService{restoreEtag: strPtr("etag-r")}).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/key1/history/1/restore", nil))
		if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte("etag-r")) {
			t.Errorf("restore = %v %q, want 200 with etag", rec.Code, rec.Body.String())
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

func TestSyncHandler_reportEvent(t *testing.T) {
	enc := encoder{}
	tests := []struct {
		name       string
		method     string
		apiKey     string
		apiKeyIn   string
		body       interface{}
		mock       *mockSyncService
		wantStatus int
		wantBody   string
	}{
		{
			name:       "401 when no API key",
			method:     http.MethodPost,
			body:       map[string]string{"event": "SYNC_STARTED"},
			mock:       &mockSyncService{},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "400 when invalid JSON",
			method:     http.MethodPost,
			apiKey:     "key1",
			apiKeyIn:   "header",
			body:       "not json",
			mock:       &mockSyncService{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "400 when event missing",
			method:     http.MethodPost,
			apiKey:     "key1",
			apiKeyIn:   "header",
			body:       map[string]string{},
			mock:       &mockSyncService{},
			wantStatus: http.StatusBadRequest,
			wantBody:   "event is required",
		},
		{
			name:       "400 when invalid event",
			method:     http.MethodPost,
			apiKey:     "key1",
			apiKeyIn:   "header",
			body:       map[string]string{"event": "INVALID_EVENT"},
			mock:       &mockSyncService{reportEventErr: sync.ErrInvalidSyncEvent},
			wantStatus: http.StatusBadRequest,
			wantBody:   "invalid sync event",
		},
		{
			name:       "204 success with header API key",
			method:     http.MethodPost,
			apiKey:     "key1",
			apiKeyIn:   "header",
			body:       map[string]string{"event": "SYNC_STARTED"},
			mock:       &mockSyncService{},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "204 success with query API key",
			method:     http.MethodPost,
			apiKey:     "key1",
			apiKeyIn:   "query",
			body:       map[string]string{"event": "SYNC_SUCCESS", "device_name": "My Phone", "message": "done"},
			mock:       &mockSyncService{},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "204 for SYNC_CANCELLED",
			method:     http.MethodPost,
			apiKey:     "key1",
			apiKeyIn:   "header",
			body:       map[string]string{"event": "SYNC_CANCELLED", "device_name": "Tablet", "message": "User cancelled"},
			mock:       &mockSyncService{},
			wantStatus: http.StatusNoContent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Route("/", func(r chi.Router) {
				newSyncHandler(enc, zerolog.Nop(), tt.mock, 0).Routes(r)
			})
			var bodyBytes []byte
			switch b := tt.body.(type) {
			case string:
				bodyBytes = []byte(b)
			default:
				var err error
				bodyBytes, err = json.Marshal(b)
				if err != nil {
					t.Fatal(err)
				}
			}
			req := httptest.NewRequest(tt.method, "/event", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			switch tt.apiKeyIn {
			case "header":
				req.Header.Set("X-API-Token", tt.apiKey)
			case "query":
				req.URL.RawQuery = "apikey=" + tt.apiKey
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Errorf("reportEvent() status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && !bytes.Contains(rec.Body.Bytes(), []byte(tt.wantBody)) {
				t.Errorf("body %q does not contain %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
