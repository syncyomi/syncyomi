package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
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
	keys := []string{"1|/m\x1f/c1", "1|/m\x1f/c2"}
	body, err := json.Marshal(map[string][]string{"deletedChapters": keys})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		headers    map[string]string
		body       []byte
		mock       *mockSyncService
		wantStatus int
		wantAck    int
		wantKeys   []string
	}{
		{name: "missing device id", body: body, mock: &mockSyncService{}, wantStatus: http.StatusBadRequest},
		{name: "not json", headers: map[string]string{"X-Device-ID": "d"}, body: []byte{0xff}, mock: &mockSyncService{}, wantStatus: http.StatusBadRequest},
		{name: "empty body", headers: map[string]string{"X-Device-ID": "d"}, body: nil, mock: &mockSyncService{}, wantStatus: http.StatusOK},
		{name: "service error", headers: map[string]string{"X-Device-ID": "d"}, body: body, mock: &mockSyncService{deleteErr: errors.New("db")}, wantStatus: http.StatusInternalServerError},
		{name: "ok", headers: map[string]string{"X-Device-ID": "d", "X-Device-Name": "Phone"}, body: body, mock: &mockSyncService{deleteCount: 2}, wantStatus: http.StatusOK, wantAck: 2, wantKeys: keys},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v2/deletions", bytes.NewReader(tt.body))
			req.Header.Set("X-API-Token", "key1")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			newRouter(tt.mock, 0).ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %v, want %v (%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var got map[string]int
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["acknowledged"] != tt.wantAck {
				t.Errorf("body = %v err = %v", rec.Body.String(), err)
			}
			req0 := tt.mock.deleteReq
			if req0 == nil || len(req0.DeletedChapters) != len(tt.wantKeys) {
				t.Fatalf("request = %+v", req0)
			}
			for i, want := range tt.wantKeys {
				if req0.DeletedChapters[i] != want {
					t.Errorf("key[%d] = %q, want %q", i, req0.DeletedChapters[i], want)
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
	mock := &mockSyncService{deleteCount: 2}
	req := httptest.NewRequest(http.MethodPost, "/v2/deletions", bytes.NewReader(gzipBytes(t, body)))
	req.Header.Set("X-API-Token", "key1")
	req.Header.Set("X-Device-ID", "d")
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	newRouter(mock, 0).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || mock.deleteReq == nil || len(mock.deleteReq.DeletedChapters) != 2 {
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
