package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/auth"
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
	stamp        string
	stampErr     error
	changeErr    error
	changeCalls  []auth.PasswordChange
	blocked      []string
	loginCalls   int
	verifyErr    error
	verifyCalls  []string
}

func (m *mockAuthService) GetUserCount(ctx context.Context) (int, error) {
	if m.userCountErr != nil {
		return 0, m.userCountErr
	}
	return m.userCount, nil
}

func (m *mockAuthService) Login(ctx context.Context, username, password string) (*domain.User, error) {
	m.loginCalls++
	if m.loginErr != nil {
		return nil, m.loginErr
	}
	return m.loginUser, nil
}

func (m *mockAuthService) CurrentPasswordStamp(ctx context.Context) (string, error) {
	return m.stamp, m.stampErr
}

func (m *mockAuthService) ChangePassword(ctx context.Context, change auth.PasswordChange) error {
	m.changeCalls = append(m.changeCalls, change)
	if m.changeErr != nil {
		return m.changeErr
	}
	m.stamp = "stamp-after-change"
	return nil
}

func (m *mockAuthService) NotifyLoginBlocked(client string) {
	m.blocked = append(m.blocked, client)
}

func (m *mockAuthService) VerifyCurrentPassword(ctx context.Context, password string) error {
	m.verifyCalls = append(m.verifyCalls, password)
	return m.verifyErr
}

func (m *mockAuthService) CreateUser(ctx context.Context, username, password string) error {
	return m.createErr
}

// newTestAuthRouter wires a real chi router around an authHandler, using the same
// newCookieStore the server does, so Set-Cookie is produced exactly as in production.
func newTestAuthRouter(cfg *domain.Config, svc authService) (chi.Router, *sessions.CookieStore) {
	return newTestAuthRouterWithLog(cfg, svc, zerolog.Nop())
}

func newTestAuthRouterWithLog(cfg *domain.Config, svc authService, log zerolog.Logger) (chi.Router, *sessions.CookieStore) {
	if cfg.SessionSecret == "" {
		cfg.SessionSecret = "test-secret" // an empty key can't sign the session
	}
	store := newCookieStore(cfg)

	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		newAuthHandler(encoder{}, log, cfg, store, svc).Routes(r)
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
			mock:       &mockAuthService{loginErr: auth.ErrInvalidCredentials},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "a busy server returns 503",
			body:       `{"username":"u","password":"p"}`,
			mock:       &mockAuthService{loginErr: auth.ErrBusy},
			wantStatus: http.StatusServiceUnavailable,
		},
		{
			name:       "an unexpected failure returns 500, not 401",
			body:       `{"username":"u","password":"p"}`,
			mock:       &mockAuthService{loginErr: errTest},
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:       "an oversized body returns 413",
			body:       `{"username":"u","password":"` + strings.Repeat("a", 10000) + `"}`,
			mock:       &mockAuthService{},
			wantStatus: http.StatusRequestEntityTooLarge,
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
	user := &domain.User{Username: "u", Password: "stored-hash"}

	t.Run("rejects a request without a session", func(t *testing.T) {
		r, _ := newTestAuthRouter(cfg, &mockAuthService{stamp: user.PasswordStamp()})

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/validate", nil))

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("validate() status = %v, want %v", rec.Code, http.StatusUnauthorized)
		}
	})

	validateAfterLogin := func(t *testing.T, mock *mockAuthService) int {
		t.Helper()
		r, _ := newTestAuthRouter(cfg, mock)
		loginRec := httptest.NewRecorder()
		r.ServeHTTP(loginRec, loginRequest(""))

		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.Header.Set("Cookie", loginRec.Header().Get("Set-Cookie"))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("accepts a session minted by a real login", func(t *testing.T) {
		got := validateAfterLogin(t, &mockAuthService{loginUser: user, stamp: user.PasswordStamp()})

		if got != http.StatusNoContent {
			t.Errorf("validate() status = %v, want %v", got, http.StatusNoContent)
		}
	})

	t.Run("rejects a session once the password has changed", func(t *testing.T) {
		got := validateAfterLogin(t, &mockAuthService{loginUser: user, stamp: "stamp-after-change"})

		if got != http.StatusUnauthorized {
			t.Errorf("validate() status = %v, want %v", got, http.StatusUnauthorized)
		}
	})

	t.Run("fails closed when the stamp lookup errors", func(t *testing.T) {
		got := validateAfterLogin(t, &mockAuthService{loginUser: user, stampErr: errTest})

		if got != http.StatusInternalServerError {
			t.Errorf("validate() status = %v, want %v", got, http.StatusInternalServerError)
		}
	})
}

func TestAuthHandler_changePassword(t *testing.T) {
	const (
		current = "current-pass-1"
		next    = "next-pass-2345"
	)
	cfg := &domain.Config{BaseURL: "/"}
	user := &domain.User{Username: "u", Password: "stored-hash"}

	newMock := func() *mockAuthService {
		return &mockAuthService{loginUser: user, stamp: user.PasswordStamp()}
	}
	loginCookie := func(t *testing.T, r http.Handler) string {
		t.Helper()
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, loginRequest(""))
		return rec.Header().Get("Set-Cookie")
	}
	post := func(r http.Handler, cookie, contentType, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/password", strings.NewReader(body))
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		if cookie != "" {
			req.Header.Set("Cookie", cookie)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	validBody := `{"current_password":"` + current + `","new_password":"` + next + `"}`
	validate := func(r http.Handler, cookie string) int {
		req := httptest.NewRequest(http.MethodGet, "/validate", nil)
		req.Header.Set("Cookie", cookie)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	t.Run("forwards the request to remove passkeys", func(t *testing.T) {
		mock := newMock()
		r, _ := newTestAuthRouter(cfg, mock)
		body := `{"current_password":"` + current + `","new_password":"` + next + `","revoke_passkeys":true}`

		rec := post(r, loginCookie(t, r), "application/json", body)

		want := auth.PasswordChange{Current: current, Next: next, RevokePasskeys: true}
		if rec.Code != http.StatusNoContent || len(mock.changeCalls) != 1 || mock.changeCalls[0] != want {
			t.Errorf("status = %d, calls = %+v, want 204 and one call %+v", rec.Code, mock.changeCalls, want)
		}
	})

	t.Run("changes the password, keeps the caller signed in and signs out other sessions", func(t *testing.T) {
		mock := newMock()
		r, _ := newTestAuthRouter(cfg, mock)
		callerCookie := loginCookie(t, r)
		otherCookie := loginCookie(t, r)

		rec := post(r, callerCookie, "application/json", validBody)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("changePassword() status = %v, want %v", rec.Code, http.StatusNoContent)
		}
		if len(mock.changeCalls) != 1 || mock.changeCalls[0] != (auth.PasswordChange{Current: current, Next: next}) {
			t.Errorf("ChangePassword calls = %v, want one call with the submitted passwords", mock.changeCalls)
		}
		if got := validate(r, rec.Header().Get("Set-Cookie")); got != http.StatusNoContent {
			t.Errorf("validate() with the refreshed session = %v, want %v", got, http.StatusNoContent)
		}
		if got := validate(r, otherCookie); got != http.StatusUnauthorized {
			t.Errorf("validate() with an older session = %v, want %v", got, http.StatusUnauthorized)
		}
		if got := validate(r, callerCookie); got != http.StatusUnauthorized {
			t.Errorf("validate() with the pre-change cookie = %v, want %v", got, http.StatusUnauthorized)
		}
	})

	t.Run("refreshes the cookie with the same flags as login", func(t *testing.T) {
		r, _ := newTestAuthRouter(&domain.Config{BaseURL: "/", SecureCookie: true}, newMock())
		cookie := loginCookie(t, r)

		rec := post(r, cookie, "application/json", validBody)

		set := rec.Header().Get("Set-Cookie")
		for _, want := range []string{"HttpOnly", "; Secure", "SameSite=Strict", "Max-Age="} {
			if !strings.Contains(set, want) {
				t.Errorf("Set-Cookie = %q, want it to contain %q", set, want)
			}
		}
	})

	failures := []struct {
		name        string
		anonymous   bool
		contentType string
		body        string
		prepare     func(m *mockAuthService)
		wantStatus  int
		wantCalled  bool
	}{
		{name: "no session is unauthorised", anonymous: true, contentType: "application/json", body: validBody, wantStatus: http.StatusUnauthorized},
		{
			name:        "a session from before another password change is unauthorised",
			contentType: "application/json",
			body:        validBody,
			prepare:     func(m *mockAuthService) { m.stamp = "stamp-after-someone-else-changed-it" },
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "a failing stamp lookup is a server error",
			contentType: "application/json",
			body:        validBody,
			prepare:     func(m *mockAuthService) { m.stampErr = errTest },
			wantStatus:  http.StatusInternalServerError,
		},
		{name: "a non-JSON content type is unsupported", contentType: "text/plain", body: validBody, wantStatus: http.StatusUnsupportedMediaType},
		{name: "a missing content type is unsupported", body: validBody, wantStatus: http.StatusUnsupportedMediaType},
		{name: "malformed JSON is a bad request", contentType: "application/json", body: `{"current_password":`, wantStatus: http.StatusBadRequest},
		{
			name:        "an oversized body is rejected",
			contentType: "application/json",
			body:        `{"current_password":"` + strings.Repeat("a", 10000) + `"}`,
			wantStatus:  http.StatusRequestEntityTooLarge,
		},
		{
			name:        "a too short new password is a bad request",
			contentType: "application/json",
			body:        validBody,
			prepare:     func(m *mockAuthService) { m.changeErr = auth.ErrPasswordTooShort },
			wantStatus:  http.StatusBadRequest,
			wantCalled:  true,
		},
		{
			name:        "a too long new password is a bad request",
			contentType: "application/json",
			body:        validBody,
			prepare:     func(m *mockAuthService) { m.changeErr = auth.ErrPasswordTooLong },
			wantStatus:  http.StatusBadRequest,
			wantCalled:  true,
		},
		{
			name:        "a wrong current password is forbidden, never unauthorised",
			contentType: "application/json",
			body:        validBody,
			prepare:     func(m *mockAuthService) { m.changeErr = auth.ErrInvalidCredentials },
			wantStatus:  http.StatusForbidden,
			wantCalled:  true,
		},
		{
			name:        "a busy server is reported as 503",
			contentType: "application/json",
			body:        validBody,
			prepare:     func(m *mockAuthService) { m.changeErr = auth.ErrBusy },
			wantStatus:  http.StatusServiceUnavailable,
			wantCalled:  true,
		},
		{
			name:        "an unexpected failure is a server error",
			contentType: "application/json",
			body:        validBody,
			prepare:     func(m *mockAuthService) { m.changeErr = errTest },
			wantStatus:  http.StatusInternalServerError,
			wantCalled:  true,
		},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMock()
			r, _ := newTestAuthRouter(cfg, mock)
			cookie := ""
			if !tt.anonymous {
				cookie = loginCookie(t, r)
			}
			if tt.prepare != nil {
				tt.prepare(mock)
			}

			rec := post(r, cookie, tt.contentType, tt.body)

			if rec.Code != tt.wantStatus {
				t.Errorf("changePassword() status = %v, want %v", rec.Code, tt.wantStatus)
			}
			if called := len(mock.changeCalls) > 0; called != tt.wantCalled {
				t.Errorf("ChangePassword called = %v, want %v", called, tt.wantCalled)
			}
		})
	}

	t.Run("an API key alone cannot change the password", func(t *testing.T) {
		mock := newMock()
		r, _ := newTestAuthRouter(cfg, mock)
		req := httptest.NewRequest(http.MethodPost, "/password?apikey=any-key", strings.NewReader(validBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Token", "any-key")
		rec := httptest.NewRecorder()

		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized || len(mock.changeCalls) != 0 {
			t.Errorf("status = %v, calls = %v, want 401 and no call", rec.Code, mock.changeCalls)
		}
	})

	t.Run("never writes the submitted passwords to the log", func(t *testing.T) {
		logs := &bytes.Buffer{}
		mock := newMock()
		mock.changeErr = errTest
		r, _ := newTestAuthRouterWithLog(cfg, mock, zerolog.New(logs))
		cookie := loginCookie(t, r)

		post(r, cookie, "application/json", validBody)

		for _, secret := range []string{current, next} {
			if strings.Contains(logs.String(), secret) {
				t.Errorf("log output contains %q: %s", secret, logs.String())
			}
		}
	})
}

func TestAuthHandler_rateLimit(t *testing.T) {
	cfg := &domain.Config{BaseURL: "/"}
	user := &domain.User{Username: "u", Password: "stored-hash"}

	loginFrom := func(r http.Handler, remoteAddr string) *httptest.ResponseRecorder {
		req := loginRequest("")
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	t.Run("blocks a client after repeated failed logins even with the right password", func(t *testing.T) {
		mock := &mockAuthService{loginErr: auth.ErrInvalidCredentials}
		r, _ := newTestAuthRouter(cfg, mock)
		for range maxFailedAttempts {
			loginFrom(r, "203.0.113.5:1000")
		}
		mock.loginErr = nil
		mock.loginUser = user

		rec := loginFrom(r, "203.0.113.5:2000")

		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("login() status = %v, want %v", rec.Code, http.StatusTooManyRequests)
		}
		if rec.Header().Get("Retry-After") == "" {
			t.Error("login() sent no Retry-After header")
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Error("login() handed out a session to a blocked client")
		}
	})

	t.Run("notifies once when a client becomes blocked", func(t *testing.T) {
		mock := &mockAuthService{loginErr: auth.ErrInvalidCredentials}
		r, _ := newTestAuthRouter(cfg, mock)

		for range maxFailedAttempts + 3 {
			loginFrom(r, "203.0.113.5:1000")
		}

		if len(mock.blocked) != 1 || mock.blocked[0] != "203.0.113.5" {
			t.Errorf("NotifyLoginBlocked calls = %v, want one for 203.0.113.5", mock.blocked)
		}
	})

	t.Run("does not block other clients", func(t *testing.T) {
		mock := &mockAuthService{loginErr: auth.ErrInvalidCredentials}
		r, _ := newTestAuthRouter(cfg, mock)
		for range maxFailedAttempts {
			loginFrom(r, "203.0.113.5:1000")
		}
		mock.loginErr = nil
		mock.loginUser = user

		rec := loginFrom(r, "198.51.100.9:1000")

		if rec.Code != http.StatusNoContent {
			t.Errorf("login() from another client status = %v, want %v", rec.Code, http.StatusNoContent)
		}
	})

	t.Run("a successful login resets the failure count", func(t *testing.T) {
		mock := &mockAuthService{loginErr: auth.ErrInvalidCredentials}
		r, _ := newTestAuthRouter(cfg, mock)
		for range maxFailedAttempts - 1 {
			loginFrom(r, "203.0.113.5:1000")
		}
		mock.loginErr, mock.loginUser = nil, user
		loginFrom(r, "203.0.113.5:1000")
		mock.loginErr = auth.ErrInvalidCredentials

		var last int
		for range maxFailedAttempts {
			last = loginFrom(r, "203.0.113.5:1000").Code
		}

		if last != http.StatusUnauthorized {
			t.Errorf("login() status = %v, want %v (still within the limit)", last, http.StatusUnauthorized)
		}
	})

	t.Run("does not call the service for a blocked client", func(t *testing.T) {
		mock := &mockAuthService{loginErr: auth.ErrInvalidCredentials}
		r, _ := newTestAuthRouter(cfg, mock)
		for range maxFailedAttempts {
			loginFrom(r, "203.0.113.5:1000")
		}
		callsBefore := mock.loginCalls

		loginFrom(r, "203.0.113.5:1000")

		if mock.loginCalls != callsBefore {
			t.Error("Login was called for a blocked client")
		}
	})

	t.Run("does not count outcomes that are not a wrong password", func(t *testing.T) {
		outcomes := map[string]error{
			"server error":      errTest,
			"busy server":       auth.ErrBusy,
			"cancelled request": context.Canceled,
		}
		for name, loginErr := range outcomes {
			t.Run(name, func(t *testing.T) {
				mock := &mockAuthService{loginErr: loginErr}
				r, _ := newTestAuthRouter(cfg, mock)
				for range maxFailedAttempts * 2 {
					loginFrom(r, "203.0.113.5:1000")
				}
				mock.loginErr, mock.loginUser = nil, user

				if got := loginFrom(r, "203.0.113.5:1000").Code; got != http.StatusNoContent {
					t.Errorf("login() status = %v, want %v", got, http.StatusNoContent)
				}
			})
		}
	})

	t.Run("does not count malformed login bodies", func(t *testing.T) {
		mock := &mockAuthService{loginUser: user}
		r, _ := newTestAuthRouter(cfg, mock)
		for range maxFailedAttempts * 2 {
			req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(`{"username":`))
			req.RemoteAddr = "203.0.113.5:1000"
			r.ServeHTTP(httptest.NewRecorder(), req)
		}

		if got := loginFrom(r, "203.0.113.5:1000").Code; got != http.StatusNoContent {
			t.Errorf("login() status = %v, want %v after only malformed bodies", got, http.StatusNoContent)
		}
	})

	t.Run("does not count rejected password change requests", func(t *testing.T) {
		valid := `{"current_password":"x","new_password":"long-enough-1"}`
		rejections := []struct {
			name        string
			contentType string
			body        string
			changeErr   error
			wantStatus  int
		}{
			{"wrong content type", "text/plain", valid, nil, http.StatusUnsupportedMediaType},
			{"malformed JSON", "application/json", `{"current_password":`, nil, http.StatusBadRequest},
			{"policy violation", "application/json", valid, auth.ErrPasswordTooShort, http.StatusBadRequest},
			{"busy server", "application/json", valid, auth.ErrBusy, http.StatusServiceUnavailable},
		}
		for _, tt := range rejections {
			t.Run(tt.name, func(t *testing.T) {
				mock := &mockAuthService{loginUser: user, stamp: user.PasswordStamp(), changeErr: tt.changeErr}
				r, _ := newTestAuthRouter(cfg, mock)
				cookie := loginFrom(r, "203.0.113.5:1000").Header().Get("Set-Cookie")
				change := func(contentType, body string) int {
					req := httptest.NewRequest(http.MethodPost, "/password", strings.NewReader(body))
					if contentType != "" {
						req.Header.Set("Content-Type", contentType)
					}
					req.Header.Set("Cookie", cookie)
					req.RemoteAddr = "203.0.113.5:3000"
					rec := httptest.NewRecorder()
					r.ServeHTTP(rec, req)
					return rec.Code
				}
				for range maxFailedAttempts * 2 {
					if got := change(tt.contentType, tt.body); got != tt.wantStatus {
						t.Fatalf("changePassword() status = %v, want %v", got, tt.wantStatus)
					}
				}
				mock.changeErr = nil

				if got := change("application/json", valid); got != http.StatusNoContent {
					t.Errorf("changePassword() status = %v, want %v after only rejected requests", got, http.StatusNoContent)
				}
			})
		}
	})

	t.Run("treats one IPv6 /64 as a single client", func(t *testing.T) {
		mock := &mockAuthService{loginErr: auth.ErrInvalidCredentials}
		r, _ := newTestAuthRouter(cfg, mock)
		for i := range maxFailedAttempts {
			loginFrom(r, fmt.Sprintf("[2001:db8:1:2::%x]:1000", i+1))
		}

		got := loginFrom(r, "[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:1000").Code

		if got != http.StatusTooManyRequests {
			t.Errorf("login() from another address in the same /64 status = %v, want %v", got, http.StatusTooManyRequests)
		}
	})

	t.Run("wrong current passwords on the change endpoint count towards the same limit", func(t *testing.T) {
		mock := &mockAuthService{loginUser: user, stamp: user.PasswordStamp(), changeErr: auth.ErrInvalidCredentials}
		r, _ := newTestAuthRouter(cfg, mock)
		loginRec := loginFrom(r, "203.0.113.5:1000")
		cookie := loginRec.Header().Get("Set-Cookie")
		change := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/password", strings.NewReader(`{"current_password":"x","new_password":"long-enough-1"}`))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Cookie", cookie)
			req.RemoteAddr = "203.0.113.5:3000"
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			return rec
		}
		for range maxFailedAttempts {
			change()
		}
		callsBefore := len(mock.changeCalls)

		rec := change()

		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("changePassword() status = %v, want %v", rec.Code, http.StatusTooManyRequests)
		}
		if len(mock.changeCalls) != callsBefore {
			t.Error("ChangePassword was called for a blocked client")
		}
	})
}

func TestClientKey(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{"203.0.113.5:4000", "203.0.113.5"},
		{"203.0.113.5", "203.0.113.5"},
		{"[::ffff:203.0.113.5]:4000", "203.0.113.5"},
		{"[2001:db8:1:2:aaaa:bbbb:cccc:dddd]:4000", "2001:db8:1:2::/64"},
		{"2001:db8:1:2::1", "2001:db8:1:2::/64"},
		{"[fe80::1%eth0]:4000", "fe80::/64"},
		{"not-an-address", "not-an-address"},
	}
	for _, tt := range tests {
		t.Run(tt.remoteAddr, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr

			if got := clientKey(req); got != tt.want {
				t.Errorf("clientKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuthHandler_onboard(t *testing.T) {
	cfg := &domain.Config{BaseURL: "/"}
	tests := []struct {
		name       string
		mock       *mockAuthService
		wantStatus int
	}{
		{"creates the account", &mockAuthService{}, http.StatusNoContent},
		{"rejects a password that breaks the policy", &mockAuthService{createErr: auth.ErrPasswordTooShort}, http.StatusBadRequest},
		{"refuses a second account", &mockAuthService{createErr: errTest}, http.StatusForbidden},
		{"reports a busy server as 503", &mockAuthService{createErr: auth.ErrBusy}, http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := newTestAuthRouter(cfg, tt.mock)
			req := httptest.NewRequest(http.MethodPost, "/onboard", strings.NewReader(`{"username":"u","password":"long-enough-1"}`))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			r.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("onboard() status = %v, want %v", rec.Code, tt.wantStatus)
			}
		})
	}
}

func TestAuthHandler_failedLoginLogsNoCredentials(t *testing.T) {
	logs := &bytes.Buffer{}
	mock := &mockAuthService{loginErr: auth.ErrInvalidCredentials}
	r, _ := newTestAuthRouterWithLog(&domain.Config{BaseURL: "/"}, mock, zerolog.New(logs))
	body := `{"username":"pasted-secret-in-username","password":"submitted-password"}`
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(body))
	req.RemoteAddr = "203.0.113.5:1000"

	r.ServeHTTP(httptest.NewRecorder(), req)

	for _, secret := range []string{"pasted-secret-in-username", "submitted-password"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("log output contains %q: %s", secret, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "203.0.113.5") {
		t.Errorf("log output = %q, want the client address", logs.String())
	}
}
