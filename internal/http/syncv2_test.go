package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/merge"
	"github.com/SyncYomi/SyncYomi/internal/sync"
)

func TestSyncV2_capabilities(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(&mockSyncService{}, 0).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v2/capabilities", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %v", rec.Code)
	}
	var caps map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &caps); err != nil || caps["version"] != float64(2) || caps["merge"] == "" {
		t.Errorf("caps = %v err = %v", caps, err)
	}
	if caps["deletions"] != "/api/sync/v2/deletions" || caps["snapshot"] != "/api/sync/v2/snapshot" {
		t.Errorf("routes not advertised: %v", caps)
	}
}

func TestSyncV2_merge(t *testing.T) {
	okResp := &sync.MergeResponse{Backup: &pb.Backup{BackupManga: []*pb.BackupManga{{Source: 1, Url: "/r"}}}, Cursor: 7, Changed: true, FullRequested: true}

	tests := []struct {
		name       string
		headers    map[string]string
		body       []byte
		mock       *mockSyncService
		wantStatus int
	}{
		{name: "missing device id", headers: map[string]string{}, body: encodedBackup(t, "/m"), mock: &mockSyncService{mergeResp: okResp}, wantStatus: http.StatusBadRequest},
		{name: "bad cursor", headers: map[string]string{"X-Device-ID": "d", "X-Sync-Cursor": "x"}, body: encodedBackup(t, "/m"), mock: &mockSyncService{mergeResp: okResp}, wantStatus: http.StatusBadRequest},
		{name: "bad deleted categories", headers: map[string]string{"X-Device-ID": "d", "X-Sync-Deleted-Categories": "1,a"}, body: encodedBackup(t, "/m"), mock: &mockSyncService{mergeResp: okResp}, wantStatus: http.StatusBadRequest},
		{name: "empty body is an empty backup", headers: map[string]string{"X-Device-ID": "d"}, body: nil, mock: &mockSyncService{mergeResp: &sync.MergeResponse{Backup: &pb.Backup{}}}, wantStatus: http.StatusOK},
		{name: "invalid protobuf", headers: map[string]string{"X-Device-ID": "d"}, body: []byte{0xff, 0xff, 0xff}, mock: &mockSyncService{mergeResp: okResp}, wantStatus: http.StatusBadRequest},
		{name: "service error", headers: map[string]string{"X-Device-ID": "d"}, body: encodedBackup(t, "/m"), mock: &mockSyncService{mergeErr: errors.New("db")}, wantStatus: http.StatusInternalServerError},
		{name: "ok", headers: map[string]string{"X-Device-ID": "d", "X-Device-Name": "Phone", "X-Sync-Cursor": "3", "X-Sync-Full": "true", "X-Sync-Deleted-Categories": "10, 20"}, body: encodedBackup(t, "/m"), mock: &mockSyncService{mergeResp: okResp}, wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v2/merge", bytes.NewReader(tt.body))
			req.Header.Set("X-API-Token", "key1")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			newRouter(tt.mock, 0).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %v, want %v (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK || tt.name == "empty body is an empty backup" {
				return
			}
			h := rec.Header()
			if h.Get("X-Sync-Cursor") != "7" || h.Get("X-Sync-Changed") != "true" || h.Get("X-Sync-Full-Requested") != "true" || h.Get("ETag") != "seq=7" {
				t.Errorf("headers = %v", h)
			}
			got, err := backup.Decode(rec.Body.Bytes())
			if err != nil || len(got.BackupManga) != 1 || got.BackupManga[0].Url != "/r" {
				t.Errorf("body = %v err = %v", got, err)
			}
			req0 := tt.mock.mergeReq
			if req0.Device.ID != "d" || req0.Device.Name != "Phone" || req0.Cursor != 3 || !req0.Full || len(req0.DeletedCategories) != 2 || req0.DeletedCategories[1] != 20 || len(req0.Backup.BackupManga) != 1 {
				t.Errorf("request = %+v", req0)
			}
			if tt.mock.accessCalls != 1 || !tt.mock.accessWrite {
				t.Errorf("access recorded = %d write=%v", tt.mock.accessCalls, tt.mock.accessWrite)
			}
		})
	}
}

func TestSyncV2_deletions(t *testing.T) {
	type result struct {
		Acknowledged int `json:"acknowledged"`
		Applied      int `json:"applied"`
	}

	tests := []struct {
		name        string
		device      string
		body        string
		applied     int
		serviceErr  error
		wantStatus  int
		wantAck     int
		wantApplied int
		wantKinds   []merge.Kind
		wantKeys    []string
		wantNoCall  bool
	}{
		{
			name:        "canonical items",
			device:      "d",
			body:        `{"items":[{"kind":"chapter","key":"1|/m\u001f/c"},{"kind":"source","key":"1"}]}`,
			applied:     1,
			wantStatus:  http.StatusOK,
			wantAck:     2,
			wantApplied: 1,
			wantKinds:   []merge.Kind{merge.KindChapter, merge.KindSource},
			wantKeys:    []string{"1|/m\x1f/c", "1"},
		},
		{
			name:        "history kind",
			device:      "d",
			body:        `{"items":[{"kind":"history","key":"1|/m\u001f/h"}]}`,
			applied:     1,
			wantStatus:  http.StatusOK,
			wantAck:     1,
			wantApplied: 1,
			wantKinds:   []merge.Kind{merge.KindHistory},
			wantKeys:    []string{"1|/m\x1f/h"},
		},
		{
			name:        "legacy deleted chapters",
			device:      "d",
			body:        `{"deletedChapters":["a","b"]}`,
			applied:     2,
			wantStatus:  http.StatusOK,
			wantAck:     2,
			wantApplied: 2,
			wantKinds:   []merge.Kind{merge.KindChapter, merge.KindChapter},
			wantKeys:    []string{"a", "b"},
		},
		{
			name:        "legacy deleted categories",
			device:      "d",
			body:        `{"deletedCategories":[17,18]}`,
			applied:     2,
			wantStatus:  http.StatusOK,
			wantAck:     2,
			wantApplied: 2,
			wantKinds:   []merge.Kind{merge.KindCategory, merge.KindCategory},
			wantKeys:    []string{"uid:17", "uid:18"},
		},
		{
			name:        "mixed shapes dedupe by kind and key",
			device:      "d",
			body:        `{"items":[{"kind":"category","key":"uid:17"},{"kind":"chapter","key":"c"}],"deletedCategories":[17,18],"deletedChapters":["c"]}`,
			applied:     3,
			wantStatus:  http.StatusOK,
			wantAck:     5,
			wantApplied: 3,
			wantKinds:   []merge.Kind{merge.KindCategory, merge.KindChapter, merge.KindCategory},
			wantKeys:    []string{"uid:17", "c", "uid:18"},
		},
		{
			name:        "unknown key is acknowledged but not applied",
			device:      "d",
			body:        `{"items":[{"kind":"manga","key":"9|/missing"}]}`,
			applied:     0,
			wantStatus:  http.StatusOK,
			wantAck:     1,
			wantApplied: 0,
			wantKinds:   []merge.Kind{merge.KindManga},
			wantKeys:    []string{"9|/missing"},
		},
		{
			name:        "empty body is a no-op",
			device:      "d",
			body:        ``,
			wantStatus:  http.StatusOK,
			wantAck:     0,
			wantApplied: 0,
			wantNoCall:  true,
		},
		{
			name:       "missing device id",
			body:       `{"deletedChapters":["a"]}`,
			wantStatus: http.StatusBadRequest,
			wantNoCall: true,
		},
		{
			name:       "unknown kind",
			device:     "d",
			body:       `{"items":[{"kind":"nope","key":"k"}]}`,
			wantStatus: http.StatusBadRequest,
			wantNoCall: true,
		},
		{
			name:       "empty key",
			device:     "d",
			body:       `{"items":[{"kind":"chapter","key":""}]}`,
			wantStatus: http.StatusBadRequest,
			wantNoCall: true,
		},
		{
			name:       "empty legacy chapter key",
			device:     "d",
			body:       `{"deletedChapters":[""]}`,
			wantStatus: http.StatusBadRequest,
			wantNoCall: true,
		},
		{
			name:       "key too long",
			device:     "d",
			body:       `{"items":[{"kind":"chapter","key":"` + strings.Repeat("x", 4097) + `"}]}`,
			wantStatus: http.StatusBadRequest,
			wantNoCall: true,
		},
		{
			name:       "malformed JSON",
			device:     "d",
			body:       `{"items":`,
			wantStatus: http.StatusBadRequest,
			wantNoCall: true,
		},
		{
			name:       "service error",
			device:     "d",
			body:       `{"deletedChapters":["a"]}`,
			serviceErr: errors.New("db"),
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockSyncService{deleteApplied: tt.applied, deleteErr: tt.serviceErr}
			req := httptest.NewRequest(http.MethodPost, "/v2/deletions", strings.NewReader(tt.body))
			req.Header.Set("X-API-Token", "key1")
			if tt.device != "" {
				req.Header.Set("X-Device-ID", tt.device)
				req.Header.Set("X-Device-Name", "Phone")
			}
			rec := httptest.NewRecorder()
			newRouter(mock, 0).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %v, want %v (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantNoCall {
				if mock.deleteReq != nil {
					t.Errorf("service called with %v", mock.deleteReq)
				}
				return
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var got result
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Acknowledged != tt.wantAck || got.Applied != tt.wantApplied {
				t.Errorf("response = %+v, want acknowledged=%d applied=%d", got, tt.wantAck, tt.wantApplied)
			}
			if mock.deleteDevice != (domain.DeviceInfo{ID: "d", Name: "Phone"}) || mock.deleteAPIKey != "key1" {
				t.Errorf("device=%+v apikey=%q", mock.deleteDevice, mock.deleteAPIKey)
			}
			if len(mock.deleteReq) != len(tt.wantKinds) {
				t.Fatalf("submitted tombstones = %v", mock.deleteReq)
			}
			for i, tb := range mock.deleteReq {
				if tb.Kind != tt.wantKinds[i] || tb.Key != tt.wantKeys[i] {
					t.Errorf("tombstone[%d] = %+v, want {%s %s}", i, tb, tt.wantKinds[i], tt.wantKeys[i])
				}
			}
		})
	}
}

func TestSyncV2_deletionsAcceptsGzip(t *testing.T) {
	keys := []string{backup.ChapterKey("1|/m", "/c1"), backup.ChapterKey("1|/m", "/c2")}
	body, err := json.Marshal(map[string][]string{"deletedChapters": keys})
	if err != nil {
		t.Fatal(err)
	}
	mock := &mockSyncService{deleteApplied: 2}
	req := httptest.NewRequest(http.MethodPost, "/v2/deletions", bytes.NewReader(gzipBytes(t, body)))
	req.Header.Set("X-API-Token", "key1")
	req.Header.Set("X-Device-ID", "d")
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	newRouter(mock, 0).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(mock.deleteReq) != 2 ||
		mock.deleteReq[0].Kind != merge.KindChapter || mock.deleteReq[0].Key != keys[0] ||
		mock.deleteReq[1].Kind != merge.KindChapter || mock.deleteReq[1].Key != keys[1] {
		t.Errorf("status = %v req = %+v", rec.Code, mock.deleteReq)
	}
}

func TestSyncV2_mergeAcceptsGzip(t *testing.T) {
	mock := &mockSyncService{mergeResp: &sync.MergeResponse{Backup: &pb.Backup{}}}
	req := httptest.NewRequest(http.MethodPost, "/v2/merge", bytes.NewReader(gzipBytes(t, encodedBackup(t, "/m"))))
	req.Header.Set("X-API-Token", "key1")
	req.Header.Set("X-Device-ID", "d")
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	newRouter(mock, 0).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || len(mock.mergeReq.Backup.BackupManga) != 1 {
		t.Errorf("status = %v req = %+v", rec.Code, mock.mergeReq)
	}
}

func TestSyncV2_snapshot(t *testing.T) {
	snap := &sync.Snapshot{Data: []byte("full"), ETag: "seq=5", Cursor: 5}
	tests := []struct {
		name       string
		cursor     string
		mock       *mockSyncService
		wantStatus int
	}{
		{name: "no data", mock: &mockSyncService{snapshotErr: sync.ErrNoData}, wantStatus: http.StatusNotFound},
		{name: "bad cursor", cursor: "-1", mock: &mockSyncService{snapshot: snap}, wantStatus: http.StatusBadRequest},
		{name: "current cursor is 304", cursor: "5", mock: &mockSyncService{snapshot: snap}, wantStatus: http.StatusNotModified},
		{name: "older cursor gets data", cursor: "2", mock: &mockSyncService{snapshot: snap}, wantStatus: http.StatusOK},
		{name: "no cursor gets data", mock: &mockSyncService{snapshot: snap}, wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v2/snapshot", nil)
			req.Header.Set("X-API-Token", "key1")
			if tt.cursor != "" {
				req.Header.Set("X-Sync-Cursor", tt.cursor)
			}
			rec := httptest.NewRecorder()
			newRouter(tt.mock, 0).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && (rec.Body.String() != "full" || rec.Header().Get("X-Sync-Cursor") != "5") {
				t.Errorf("body = %q headers = %v", rec.Body.String(), rec.Header())
			}
		})
	}
}
