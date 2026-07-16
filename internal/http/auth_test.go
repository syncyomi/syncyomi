package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog"
)

// errTest stands in for any service-layer failure.
var errTest = errors.New("test error")

type mockAuthService struct {
	userCount    int
	userCountErr error
	loginUser    *domain.User
	loginErr     error
	createErr    error
}

func (m *mockAuthService) GetUserCount(ctx context.Context) (int, error) {
	if m.userCountErr != nil {
		return 0, m.userCountErr
	}
	return m.userCount, nil
}

func (m *mockAuthService) Login(ctx context.Context, username, password string) (*domain.User, error) {
	if m.loginErr != nil {
		return nil, m.loginErr
	}
	return m.loginUser, nil
}

func (m *mockAuthService) CreateUser(ctx context.Context, username, password string) error {
	return m.createErr
}

// newTestAuthRouter wires a real chi router around an authHandler, using the same
// newCookieStore the server does, so Set-Cookie is produced exactly as in production.
func newTestAuthRouter(cfg *domain.Config, svc authService) (chi.Router, *sessions.CookieStore) {
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = "test-secret" // an empty key can't sign the session
	}
	store := newCookieStore(cfg)

	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		newAuthHandler(encoder{}, zerolog.Nop(), cfg, store, svc).Routes(r)
	})
	return r, store
}

func loginRequest(fwdProto string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/login",
		strings.NewReader(`{"username":"u","password":"p"}`))
	req.Header.Set("Content-Type", "application/json")
	if fwdProto != "" {
		req.Header.Set("X-Forwarded-Proto", fwdProto)
	}
	return req
}

// The cookie must not be marked Secure over plain HTTP: browsers drop Secure
// cookies on http:// everywhere except localhost, which breaks login on IP:PORT.
func TestAuthHandler_loginCookieFlags(t *testing.T) {
	tests := []struct {
		name         string
		secureCookie bool
		fwdProto     string
		wantSecure   bool
		wantSameSite string
	}{
		{
			name:         "plain http is not secure",
			secureCookie: false,
			wantSecure:   false,
			wantSameSite: "SameSite=Lax",
		},
		{
			name:         "forwarded https is secure and strict",
			secureCookie: false,
			fwdProto:     "https",
			wantSecure:   true,
			wantSameSite: "SameSite=Strict",
		},
		{
			name:         "secureCookie config forces secure without the header",
			secureCookie: true,
			wantSecure:   true,
			wantSameSite: "SameSite=Strict",
		},
		{
			name:         "forwarded http is treated as plain",
			secureCookie: false,
			fwdProto:     "http",
			wantSecure:   false,
			wantSameSite: "SameSite=Lax",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &domain.Config{BaseURL: "/", SecureCookie: tt.secureCookie}
			r, _ := newTestAuthRouter(cfg, &mockAuthService{loginUser: &domain.User{Username: "u"}})

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, loginRequest(tt.fwdProto))

			if rec.Code != http.StatusNoContent {
				t.Fatalf("login() status = %v, want %v", rec.Code, http.StatusNoContent)
			}

			cookie := rec.Header().Get("Set-Cookie")
			if cookie == "" {
				t.Fatal("login() set no cookie")
			}
			if got := strings.Contains(cookie, "; Secure"); got != tt.wantSecure {
				t.Errorf("login() Secure = %v, want %v (cookie: %q)", got, tt.wantSecure, cookie)
			}
			if !strings.Contains(cookie, tt.wantSameSite) {
				t.Errorf("login() cookie = %q, want %q", cookie, tt.wantSameSite)
			}
			if !strings.Contains(cookie, "HttpOnly") {
				t.Errorf("login() cookie = %q, want HttpOnly", cookie)
			}
			// Without Max-Age the cookie dies on browser close.
			if !strings.Contains(cookie, "Max-Age=") {
				t.Errorf("login() cookie = %q, want Max-Age", cookie)
			}
		})
	}
}

// Regression: cookie options used to be written to the shared cookieStore, so a
// single proxied HTTPS request flipped Secure on for every later plain-HTTP client.
func TestAuthHandler_loginDoesNotLeakSchemeBetweenClients(t *testing.T) {
	cfg := &domain.Config{BaseURL: "/", SecureCookie: false}
	r, store := newTestAuthRouter(cfg, &mockAuthService{loginUser: &domain.User{Username: "u"}})

	// A client behind an HTTPS-terminating proxy logs in first.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, loginRequest("https"))
	if got := rec.Header().Get("Set-Cookie"); !strings.Contains(got, "; Secure") {
		t.Fatalf("https login cookie = %q, want Secure", got)
	}

	// A plain-HTTP client logs in immediately afterwards against the same server.
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, loginRequest(""))
	if got := rec.Header().Get("Set-Cookie"); strings.Contains(got, "; Secure") {
		t.Errorf("plain http login cookie = %q, want no Secure (the https request leaked onto it)", got)
	}

	// The shared store itself must be untouched by either request.
	if store.Options.Secure {
		t.Error("store.Options.Secure = true, want false (handler mutated shared state)")
	}
	if store.Options.SameSite != http.SameSiteLaxMode {
		t.Errorf("store.Options.SameSite = %v, want Lax (handler mutated shared state)", store.Options.SameSite)
	}
}

func TestAuthHandler_login(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		mock       *mockAuthService
		wantStatus int
	}{
		{
			name:       "success returns 204",
			body:       `{"username":"u","password":"p"}`,
			mock:       &mockAuthService{loginUser: &domain.User{Username: "u"}},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "malformed json returns 400",
			body:       `{"username":`,
			mock:       &mockAuthService{},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "bad credentials return 401",
			body:       `{"username":"u","password":"wrong"}`,
			mock:       &mockAuthService{loginErr: errTest},
			wantStatus: http.StatusUnauthorized,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &domain.Config{BaseURL: "/"}
			r, _ := newTestAuthRouter(cfg, tt.mock)

			req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("login() status = %v, want %v", rec.Code, tt.wantStatus)
			}
			// Only a successful login may hand out a session.
			if gotCookie := rec.Header().Get("Set-Cookie") != ""; gotCookie != (tt.wantStatus == http.StatusNoContent) {
				t.Errorf("login() set cookie = %v, want %v", gotCookie, tt.wantStatus == http.StatusNoContent)
			}
		})
	}
}

// logout sets no session options of its own, so it inherits the store baseline.
// If that baseline is wrong the browser drops the cookie over plain HTTP and the
// session is never cleared, so this is what guards newCookieStore.
func TestAuthHandler_logout(t *testing.T) {
	cfg := &domain.Config{BaseURL: "/"}
	r, _ := newTestAuthRouter(cfg, &mockAuthService{})

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("logout() status = %v, want %v", rec.Code, http.StatusNoContent)
	}

	cookie := rec.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("logout() set no cookie")
	}
	if strings.Contains(cookie, "; Secure") {
		t.Errorf("logout() cookie = %q, want no Secure over plain http", cookie)
	}
	if !strings.Contains(cookie, "SameSite=Lax") {
		t.Errorf("logout() cookie = %q, want SameSite=Lax", cookie)
	}
	if !strings.Contains(cookie, "HttpOnly") {
		t.Errorf("logout() cookie = %q, want HttpOnly", cookie)
	}
}

func TestAuthHandler_canOnboard(t *testing.T) {
	tests := []struct {
		name       string
		mock       *mockAuthService
		wantStatus int
	}{
		{
			name:       "no users allows onboarding",
			mock:       &mockAuthService{userCount: 0},
			wantStatus: http.StatusNoContent,
		},
		{
			name:       "existing user forbids onboarding",
			mock:       &mockAuthService{userCount: 1},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "count error returns 500",
			mock:       &mockAuthService{userCountErr: errTest},
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &domain.Config{BaseURL: "/"}
			r, _ := newTestAuthRouter(cfg, tt.mock)

			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/onboard", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("canOnboard() status = %v, want %v", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestAuthHandler_validate(t *testing.T) {
	cfg := &domain.Config{BaseURL: "/"}
	r, _ := newTestAuthRouter(cfg, &mockAuthService{})

	// No session cookie at all.
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/validate", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("validate() without session status = %v, want %v", rec.Code, http.StatusUnauthorized)
	}

	// A session minted by a real login is accepted.
	loginRec := httptest.NewRecorder()
	r.ServeHTTP(loginRec, loginRequest(""))
	sessionCookie := loginRec.Header().Get("Set-Cookie")

	req := httptest.NewRequest(http.MethodGet, "/validate", nil)
	req.Header.Set("Cookie", sessionCookie)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("validate() with session status = %v, want %v", rec.Code, http.StatusNoContent)
	}
}

func TestReadUserIP(t *testing.T) {
	tests := []struct {
		name       string
		realIP     string
		forwardFor string
		remoteAddr string
		want       string
	}{
		{
			name:       "x-real-ip wins",
			realIP:     "1.1.1.1",
			forwardFor: "2.2.2.2",
			remoteAddr: "3.3.3.3:1234",
			want:       "1.1.1.1",
		},
		{
			name:       "x-forwarded-for when no real ip",
			forwardFor: "2.2.2.2",
			remoteAddr: "3.3.3.3:1234",
			want:       "2.2.2.2",
		},
		{
			name:       "remote addr as last resort",
			remoteAddr: "3.3.3.3:1234",
			want:       "3.3.3.3:1234",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.realIP != "" {
				req.Header.Set("X-Real-Ip", tt.realIP)
			}
			if tt.forwardFor != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardFor)
			}
			if got := ReadUserIP(req); got != tt.want {
				t.Errorf("ReadUserIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
