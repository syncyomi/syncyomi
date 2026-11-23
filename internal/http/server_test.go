package http

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/config"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/r3labs/sse/v2"
	"github.com/rs/zerolog"
)

const routingTestKey = "device-key"

func newRoutingTestServer() Server {
	cfg := &domain.Config{BaseURL: "/", SessionSecret: "test-secret"}
	return Server{
		log:                 zerolog.Nop(),
		sse:                 sse.New(),
		config:              &config.AppConfig{Config: cfg},
		cookieStore:         newCookieStore(cfg),
		authService:         &mockAuthService{stamp: currentStamp},
		apiService:          &mockAPIKeyService{validKey: routingTestKey},
		notificationService: &fakeNotificationService{},
		syncService:         &mockSyncService{},
	}
}

func authedSessionCookie(t *testing.T, s Server) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess, _ := s.cookieStore.Get(req, sessionName)
	markSessionAuthenticated(sess, currentStamp)
	rec := httptest.NewRecorder()
	if err := sess.Save(req, rec); err != nil {
		t.Fatalf("saving session: %v", err)
	}
	return rec.Header().Get("Set-Cookie")
}

func TestServer_HandlerRejectsAPIKeysOnAdminRoutes(t *testing.T) {
	adminRoutes := []struct{ method, path string }{
		{http.MethodGet, "/api/keys"},
		{http.MethodPost, "/api/keys"},
		{http.MethodDelete, "/api/keys/other-key"},
		{http.MethodGet, "/api/notification"},
		{http.MethodPost, "/api/notification"},
		{http.MethodGet, "/api/config"},
		{http.MethodPatch, "/api/config"},
		{http.MethodGet, "/api/logs/files"},
		{http.MethodGet, "/api/updates/latest"},
		{http.MethodGet, "/api/events?stream=logs"},
	}
	credentials := []struct {
		name  string
		apply func(*http.Request)
	}{
		{"header", func(r *http.Request) { r.Header.Set("X-API-Token", routingTestKey) }},
		{"query", func(r *http.Request) {
			q := r.URL.Query()
			q.Set("apikey", routingTestKey)
			r.URL.RawQuery = q.Encode()
		}},
	}
	handler := newRoutingTestServer().Handler()

	for _, route := range adminRoutes {
		for _, cred := range credentials {
			t.Run(route.method+" "+route.path+" via "+cred.name, func(t *testing.T) {
				req := httptest.NewRequest(route.method, route.path, nil)
				cred.apply(req)
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusUnauthorized {
					t.Errorf("got status %v, want %v", rec.Code, http.StatusUnauthorized)
				}
			})
		}
	}
}

func TestServer_HandlerServesAdminRoutesToSession(t *testing.T) {
	s := newRoutingTestServer()
	handler := s.Handler()
	cookie := authedSessionCookie(t, s)

	for _, path := range []string{"/api/keys", "/api/notification"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Cookie", cookie)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Errorf("got status %v, want %v", rec.Code, http.StatusOK)
			}
		})
	}
}

func TestServer_HandlerServesSyncRoutesToAPIKey(t *testing.T) {
	handler := newRoutingTestServer().Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/sync/v2/capabilities", nil)
	req.Header.Set("X-API-Token", routingTestKey)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("got status %v, want %v", rec.Code, http.StatusOK)
	}
}

type switchableStamp struct {
	*mockAuthService
	mu    sync.Mutex
	stamp string
	err   error
}

func (s *switchableStamp) CurrentPasswordStamp(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stamp, s.err
}

func (s *switchableStamp) setErr(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (s *switchableStamp) set(stamp string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stamp = stamp
}

func TestServer_EventStreamEndsWhenThePasswordChanges(t *testing.T) {
	auth := &switchableStamp{mockAuthService: &mockAuthService{}, stamp: currentStamp}
	s := newRoutingTestServer()
	s.authService = auth
	s.streamRecheckInterval = 10 * time.Millisecond
	s.sse.CreateStream("logs")
	cookie := authedSessionCookie(t, s)
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/events?stream=logs", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	ended := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		close(ended)
	}()

	select {
	case <-ended:
		t.Fatal("stream ended while the session was still valid")
	case <-time.After(100 * time.Millisecond):
	}

	auth.set("stamp-after-password-change")

	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("stream still open after the password changed")
	}
}

func TestServer_EventStreamSurvivesAFailedSessionRecheck(t *testing.T) {
	auth := &switchableStamp{mockAuthService: &mockAuthService{}, stamp: currentStamp}
	logs := &lockedBuffer{}
	s := newRoutingTestServer()
	s.log = zerolog.New(logs)
	s.authService = auth
	s.streamRecheckInterval = 10 * time.Millisecond
	s.sse.CreateStream("logs")
	cookie := authedSessionCookie(t, s)
	server := httptest.NewServer(s.Handler())
	t.Cleanup(server.Close)

	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/events?stream=logs", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Cookie", cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ended := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		close(ended)
	}()

	auth.setErr(errTest)
	deadline := time.After(2 * time.Second)
	for !strings.Contains(logs.String(), "could not re-check the session") {
		select {
		case <-deadline:
			t.Fatal("no warning was logged for the failed re-check")
		case <-time.After(5 * time.Millisecond):
		}
	}

	select {
	case <-ended:
		t.Fatal("stream ended because of a transient lookup error")
	case <-time.After(100 * time.Millisecond):
	}

	resp.Body.Close()
	<-ended
	before := strings.Count(logs.String(), "could not re-check the session")
	time.Sleep(60 * time.Millisecond)
	if after := strings.Count(logs.String(), "could not re-check the session"); after != before {
		t.Errorf("warnings after the client left = %d, want %d (no log for a cancelled re-check)", after, before)
	}
}
