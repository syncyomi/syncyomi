package harness

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func deletionClient(t *testing.T, srv *httptest.Server) *SyntheticClient {
	t.Helper()
	return &SyntheticClient{
		Server:     &SyncServer{BaseURL: srv.URL, APIKey: "test-token"},
		DeviceID:   "test-device",
		DeviceName: "e2e-synthetic",
	}
}

func assertDeletionRequest(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("method = %s, want POST", r.Method)
	}
	if r.URL.Path != "/api/sync/v2/deletions" {
		t.Errorf("path = %s, want /api/sync/v2/deletions", r.URL.Path)
	}
	if got := r.Header.Get("X-API-Token"); got != "test-token" {
		t.Errorf("X-API-Token = %q, want test-token", got)
	}
	if got := r.Header.Get("X-Device-ID"); got != "test-device" {
		t.Errorf("X-Device-ID = %q, want test-device", got)
	}
	if got := r.Header.Get("X-Device-Name"); got != "e2e-synthetic" {
		t.Errorf("X-Device-Name = %q, want e2e-synthetic", got)
	}
}

func TestSyntheticClientDeletionsCanonical(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertDeletionRequest(t, r)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Errorf("body %q is not JSON: %v", data, err)
		}
		got = payload
		_, _ = io.WriteString(w, `{"acknowledged":1,"applied":1}`)
	}))
	defer srv.Close()

	c := deletionClient(t, srv)
	res, err := c.Deletions(context.Background(), DeletionsOptions{
		Items: []DeletionItem{{Kind: "chapter", Key: "1|/m\x1f/c"}},
	})
	if err != nil {
		t.Fatalf("Deletions: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}

	want := map[string]any{
		"items": []any{map[string]any{"kind": "chapter", "key": "1|/m\u001f/c"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %#v, want %#v", got, want)
	}
}

func TestSyntheticClientDeletionsLegacy(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertDeletionRequest(t, r)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Errorf("body %q is not JSON: %v", data, err)
		}
		got = payload
		_, _ = io.WriteString(w, `{"acknowledged":2,"applied":1}`)
	}))
	defer srv.Close()

	c := deletionClient(t, srv)
	_, err := c.Deletions(context.Background(), DeletionsOptions{
		DeletedChapters:   []string{"k"},
		DeletedCategories: []int64{7},
	})
	if err != nil {
		t.Fatalf("Deletions: %v", err)
	}

	want := map[string]any{
		"deletedChapters":   []any{"k"},
		"deletedCategories": []any{float64(7)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("request body = %#v, want %#v", got, want)
	}
}

func TestSyntheticClientDeletionsResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"acknowledged":2,"applied":1}`)
	}))
	defer srv.Close()

	c := deletionClient(t, srv)
	res, err := c.Deletions(context.Background(), DeletionsOptions{
		Items: []DeletionItem{{Kind: "manga", Key: "1|/m"}},
	})
	if err != nil {
		t.Fatalf("Deletions: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if res.Acknowledged != 2 || res.Applied != 1 {
		t.Errorf("result = ack %d applied %d, want ack 2 applied 1", res.Acknowledged, res.Applied)
	}
}

func TestSyntheticClientDeletionsNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"message":"unknown deletion kind"}`)
	}))
	defer srv.Close()

	c := deletionClient(t, srv)
	res, err := c.Deletions(context.Background(), DeletionsOptions{
		Items: []DeletionItem{{Kind: "bogus", Key: "x"}},
	})
	if err != nil {
		t.Fatalf("Deletions returned an error for a 400: %v", err)
	}
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
	if string(res.Body) != `{"message":"unknown deletion kind"}` {
		t.Errorf("body = %q, want the 400 response body", res.Body)
	}
	if res.Acknowledged != 0 || res.Applied != 0 {
		t.Errorf("result = ack %d applied %d, want 0/0", res.Acknowledged, res.Applied)
	}
}

func TestSyntheticClientDeletionsUnparseableResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `not-json`)
	}))
	defer srv.Close()

	c := deletionClient(t, srv)
	res, err := c.Deletions(context.Background(), DeletionsOptions{
		Items: []DeletionItem{{Kind: "manga", Key: "1|/m"}},
	})
	if err == nil {
		t.Fatalf("Deletions returned ack %d applied %d and no error for a 200 body that is not JSON; want an error", res.Acknowledged, res.Applied)
	}
	if !strings.Contains(err.Error(), "deletions: decode response") {
		t.Errorf("err = %v, want it to mention the decode failure", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if string(res.Body) != "not-json" {
		t.Errorf("body = %q, want not-json", res.Body)
	}
}

func TestSyntheticClientDeletionsEmptyBody(t *testing.T) {
	var gotLen int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		gotLen = len(data)
		_, _ = io.WriteString(w, `{"acknowledged":0,"applied":0}`)
	}))
	defer srv.Close()

	c := deletionClient(t, srv)
	res, err := c.Deletions(context.Background(), DeletionsOptions{RawBody: []byte{}})
	if err != nil {
		t.Fatalf("Deletions: %v", err)
	}
	if gotLen != 0 {
		t.Errorf("server saw a %d-byte body, want 0", gotLen)
	}
	if res.StatusCode != http.StatusOK || res.Acknowledged != 0 || res.Applied != 0 {
		t.Errorf("result = %d ack %d applied %d, want 200 0/0", res.StatusCode, res.Acknowledged, res.Applied)
	}
}
