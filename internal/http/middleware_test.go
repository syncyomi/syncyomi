package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/auth"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/gorilla/sessions"
)

const currentStamp = "stamp-1"

type sessionKind string

const (
	noSession        sessionKind = ""
	authedSession    sessionKind = "authed"
	unauthedSession  sessionKind = "unauthed"
	staleSession     sessionKind = "stale"
	unstampedSession sessionKind = "unstamped"
)

var sessionValues = map[sessionKind]map[string]any{
	authedSession:    {sessionAuthenticatedKey: true, sessionStampKey: currentStamp},
	unauthedSession:  {sessionAuthenticatedKey: false, sessionStampKey: currentStamp},
	staleSession:     {sessionAuthenticatedKey: true, sessionStampKey: "old-stamp"},
	unstampedSession: {sessionAuthenticatedKey: true},
}

func attachSession(t *testing.T, store *sessions.CookieStore, req *http.Request, kind sessionKind) {
	t.Helper()
	sess, _ := store.Get(req, sessionName)
	for k, v := range sessionValues[kind] {
		sess.Values[k] = v
	}
	w := httptest.NewRecorder()
	if err := sess.Save(req, w); err != nil {
		t.Fatalf("saving session: %v", err)
	}
	req.Header.Set("Cookie", w.Header().Get("Set-Cookie"))
}

type mockAPIKeyService struct {
	validKey string
	// calls records every token ValidateAPIKey was asked about, so tests can
	// assert which branch ran rather than only what it returned.
	calls []string
}

func (m *mockAPIKeyService) Get(ctx context.Context, key string) (*domain.APIKey, error) {
	return nil, nil
}
func (m *mockAPIKeyService) List(ctx context.Context) ([]domain.APIKey, error)   { return nil, nil }
func (m *mockAPIKeyService) Store(ctx context.Context, key *domain.APIKey) error { return nil }
func (m *mockAPIKeyService) Update(ctx context.Context, key *domain.APIKey) error {
	return nil
}
func (m *mockAPIKeyService) Delete(ctx context.Context, key string) error { return nil }

func (m *mockAPIKeyService) ValidateAPIKey(ctx context.Context, token string) bool {
	m.calls = append(m.calls, token)
	return token == m.validKey
}

func TestServer_IsAuthenticated(t *testing.T) {
	const validKey = "valid-key"

	tests := []struct {
		name       string
		header     string
		query      string
		session    sessionKind
		stampErr   error
		wantStatus int
		wantCalls  []string
	}{
		{
			name:       "valid api token header passes",
			header:     validKey,
			wantStatus: http.StatusOK,
			wantCalls:  []string{validKey},
		},
		{
			name:       "invalid api token header is rejected",
			header:     "nope",
			wantStatus: http.StatusUnauthorized,
			wantCalls:  []string{"nope"},
		},
		{
			name:       "valid apikey query param passes",
			query:      validKey,
			wantStatus: http.StatusOK,
			wantCalls:  []string{validKey},
		},
		{
			name:       "invalid apikey query param is rejected",
			query:      "nope",
			wantStatus: http.StatusUnauthorized,
			wantCalls:  []string{"nope"},
		},
		{
			name:       "no credentials at all is rejected",
			wantStatus: http.StatusUnauthorized,
			wantCalls:  nil,
		},
		{
			name:       "authenticated session passes",
			session:    authedSession,
			wantStatus: http.StatusOK,
			wantCalls:  nil,
		},
		{
			name:       "logged out session is rejected",
			session:    unauthedSession,
			wantStatus: http.StatusUnauthorized,
			wantCalls:  nil,
		},
		{
			name:       "session from before a password change is rejected",
			session:    staleSession,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "session without a password stamp is rejected",
			session:    unstampedSession,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "session is rejected when no user exists",
			session:    authedSession,
			stampErr:   auth.ErrNoUser,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "a failing stamp lookup fails closed with a server error",
			session:    authedSession,
			stampErr:   errTest,
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "api key does not consult the password stamp",
			header:     validKey,
			stampErr:   errTest,
			wantStatus: http.StatusOK,
			wantCalls:  []string{validKey},
		},
		{
			// An invalid header must 401 outright, not quietly fall through to
			// the session branch and let a cookie rescue it.
			name:       "invalid header does not fall through to session",
			header:     "nope",
			session:    authedSession,
			wantStatus: http.StatusUnauthorized,
			wantCalls:  []string{"nope"},
		},
		{
			// Header is checked before query, so the query is never consulted.
			name:       "header takes precedence over query",
			header:     validKey,
			query:      "nope",
			wantStatus: http.StatusOK,
			wantCalls:  []string{validKey},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &mockAPIKeyService{validKey: validKey}
			cfg := &domain.Config{BaseURL: "/", SessionSecret: "test-secret"}
			s := Server{
				apiService:  api,
				authService: &mockAuthService{stamp: currentStamp, stampErr: tt.stampErr},
				cookieStore: newCookieStore(cfg),
			}

			var nextCalled bool
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			})

			target := "/"
			if tt.query != "" {
				target = "/?apikey=" + tt.query
			}
			req := httptest.NewRequest(http.MethodGet, target, nil)
			if tt.header != "" {
				req.Header.Set("X-API-Token", tt.header)
			}
			if tt.session != noSession {
				attachSession(t, s.cookieStore, req, tt.session)
			}

			rec := httptest.NewRecorder()
			s.IsAuthenticated(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("IsAuthenticated() status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if want := tt.wantStatus == http.StatusOK; nextCalled != want {
				t.Errorf("IsAuthenticated() called next = %v, want %v", nextCalled, want)
			}
			if len(api.calls) != len(tt.wantCalls) {
				t.Errorf("ValidateAPIKey calls = %v, want %v", api.calls, tt.wantCalls)
			}
		})
	}
}

func TestServer_IsSessionAuthenticated(t *testing.T) {
	const validKey = "valid-key"

	tests := []struct {
		name       string
		header     string
		session    sessionKind
		stampErr   error
		wantStatus int
	}{
		{name: "valid api token is rejected", header: validKey, wantStatus: http.StatusUnauthorized},
		{name: "session from before a password change is rejected", session: staleSession, wantStatus: http.StatusUnauthorized},
		{name: "session without a password stamp is rejected", session: unstampedSession, wantStatus: http.StatusUnauthorized},
		{name: "session is rejected when no user exists", session: authedSession, stampErr: auth.ErrNoUser, wantStatus: http.StatusUnauthorized},
		{name: "a failing stamp lookup fails closed with a server error", session: authedSession, stampErr: errTest, wantStatus: http.StatusInternalServerError},
		{name: "no credentials is rejected", wantStatus: http.StatusUnauthorized},
		{name: "authenticated session passes", session: authedSession, wantStatus: http.StatusOK},
		{name: "logged out session is rejected", session: unauthedSession, wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &mockAPIKeyService{validKey: validKey}
			cfg := &domain.Config{BaseURL: "/", SessionSecret: "test-secret"}
			s := Server{
				apiService:  api,
				authService: &mockAuthService{stamp: currentStamp, stampErr: tt.stampErr},
				cookieStore: newCookieStore(cfg),
			}

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("X-API-Token", tt.header)
			}
			if tt.session != noSession {
				attachSession(t, s.cookieStore, req, tt.session)
			}

			rec := httptest.NewRecorder()
			s.IsSessionAuthenticated(next).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("IsSessionAuthenticated() status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if len(api.calls) != 0 {
				t.Errorf("ValidateAPIKey must not be consulted, calls = %v", api.calls)
			}
		})
	}
}
