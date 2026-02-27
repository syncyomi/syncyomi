package http

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/sync"
	"github.com/go-chi/chi/v5"
)

type mockSyncService struct {
	getETagErr         error
	getETag            *string
	getDataAndETagErr  error
	getData            []byte
	getDataETag        *string
	setDataErr         error
	setDataEtag        *string
	setDataIfMatchErr  error
	setDataIfMatchEtag *string
	reportEventErr     error
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
	return m.setDataEtag, nil
}

func (m *mockSyncService) SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error) {
	if m.setDataIfMatchErr != nil {
		return nil, m.setDataIfMatchErr
	}
	return m.setDataIfMatchEtag, nil
}

func (m *mockSyncService) ReportSyncEvent(ctx context.Context, apiKey string, event string, deviceName string, detailMessage string) error {
	return m.reportEventErr
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
				newSyncHandler(enc, tt.mock).Routes(r)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := chi.NewRouter()
			r.Route("/", func(r chi.Router) {
				newSyncHandler(enc, tt.mock).Routes(r)
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
		})
	}
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
				newSyncHandler(enc, tt.mock).Routes(r)
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
