package http

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
)

type mockAPIKeyService struct {
	validKey string
	// calls records every token ValidateAPIKey was asked about, so tests can
	// assert which branch ran rather than only what it returned.
	calls []string
}

func (m *mockAPIKeyService) Get(ctx context.Context, key string) (*domain.APIKey, error) {
	return nil, nil
}
func (m *mockAPIKeyService) List(ctx context.Context) ([]domain.APIKey, error)  { return nil, nil }
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
		session    string // "", "authed", "unauthed"
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
			session:    "authed",
			wantStatus: http.StatusOK,
			wantCalls:  nil,
		},
		{
			name:       "logged out session is rejected",
			session:    "unauthed",
			wantStatus: http.StatusUnauthorized,
			wantCalls:  nil,
		},
		{
			// An invalid header must 401 outright, not quietly fall through to
			// the session branch and let a cookie rescue it.
			name:       "invalid header does not fall through to session",
			header:     "nope",
			session:    "authed",
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
			s := Server{apiService: api, cookieStore: newCookieStore(cfg)}

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
			if tt.session != "" {
				sess, _ := s.cookieStore.Get(req, "user_session")
				sess.Values["authenticated"] = tt.session == "authed"
				w := httptest.NewRecorder()
				if err := sess.Save(req, w); err != nil {
					t.Fatalf("saving session: %v", err)
				}
				req.Header.Set("Cookie", w.Header().Get("Set-Cookie"))
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
