package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/auth"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/passkey"
	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/rs/zerolog"
)

type mockPasskeyService struct {
	enabled       bool
	beginRegErr   error
	finishRegErr  error
	beginLoginErr error
	finishLogErr  error
	loginUser     *domain.User
	list          []domain.Passkey
	deleteOK      bool
	deleteErr     error

	registerNames []string
	ceremonyIDs   []string
	deletedIDs    []int
	loginFinishes int
}

func (m *mockPasskeyService) Enabled() bool { return m.enabled }

func (m *mockPasskeyService) BeginRegistration(_ context.Context, name string) (string, *protocol.CredentialCreation, error) {
	m.registerNames = append(m.registerNames, name)
	if m.beginRegErr != nil {
		return "", nil, m.beginRegErr
	}
	return "reg-ceremony", &protocol.CredentialCreation{}, nil
}

func (m *mockPasskeyService) FinishRegistration(_ context.Context, id string, _ *http.Request) (*domain.Passkey, error) {
	m.ceremonyIDs = append(m.ceremonyIDs, id)
	if m.finishRegErr != nil {
		return nil, m.finishRegErr
	}
	return &domain.Passkey{ID: 7, Name: "Laptop", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}, nil
}

func (m *mockPasskeyService) BeginLogin() (string, *protocol.CredentialAssertion, error) {
	if m.beginLoginErr != nil {
		return "", nil, m.beginLoginErr
	}
	return "login-ceremony", &protocol.CredentialAssertion{}, nil
}

func (m *mockPasskeyService) FinishLogin(_ context.Context, id string, _ *http.Request) (*domain.User, error) {
	m.loginFinishes++
	m.ceremonyIDs = append(m.ceremonyIDs, id)
	if m.finishLogErr != nil {
		return nil, m.finishLogErr
	}
	return m.loginUser, nil
}

func (m *mockPasskeyService) List(context.Context) ([]domain.Passkey, error) { return m.list, nil }

func (m *mockPasskeyService) Delete(_ context.Context, id int) (bool, error) {
	m.deletedIDs = append(m.deletedIDs, id)
	return m.deleteOK, m.deleteErr
}

type passkeyRig struct {
	router   chi.Router
	authSvc  *mockAuthService
	passkeys *mockPasskeyService
	cookie   string
}

func newPasskeyRig(t *testing.T) *passkeyRig {
	t.Helper()

	user := &domain.User{Username: "u", Password: "stored-hash"}
	authSvc := &mockAuthService{loginUser: user, stamp: user.PasswordStamp()}
	passkeys := &mockPasskeyService{enabled: true, loginUser: user, deleteOK: true}
	cfg := &domain.Config{BaseURL: "/", SessionSecret: "test-secret"}
	store := newCookieStore(cfg)

	r := chi.NewRouter()
	r.Route("/", func(r chi.Router) {
		handler := newAuthHandler(encoder{}, zerolog.Nop(), cfg, store, authSvc)
		handler.Routes(r)
		newPasskeyHandler(handler, passkeys).Routes(r)
	})

	rig := &passkeyRig{router: r, authSvc: authSvc, passkeys: passkeys}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, loginRequest(""))
	rig.cookie = rec.Header().Get("Set-Cookie")

	return rig
}

type call struct {
	method      string
	path        string
	body        string
	contentType string
	session     bool
	ceremony    string
	remoteAddr  string
}

func (rig *passkeyRig) do(c call) *httptest.ResponseRecorder {
	req := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
	if c.contentType != "" {
		req.Header.Set("Content-Type", c.contentType)
	}
	var cookies []string
	if c.session {
		cookies = append(cookies, strings.SplitN(rig.cookie, ";", 2)[0])
	}
	if c.ceremony != "" {
		cookies = append(cookies, "passkey_ceremony="+c.ceremony)
	}
	if len(cookies) > 0 {
		req.Header.Set("Cookie", strings.Join(cookies, "; "))
	}
	req.RemoteAddr = "203.0.113.9:4000"
	if c.remoteAddr != "" {
		req.RemoteAddr = c.remoteAddr
	}
	rec := httptest.NewRecorder()
	rig.router.ServeHTTP(rec, req)

	return rec
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

const validRegisterBody = `{"name":"Laptop","current_password":"right-password-1"}`

func TestPasskeyHandler_capabilities(t *testing.T) {
	t.Run("reports whether passkeys are enabled", func(t *testing.T) {
		for _, enabled := range []bool{true, false} {
			rig := newPasskeyRig(t)
			rig.passkeys.enabled = enabled

			rec := rig.do(call{method: http.MethodGet, path: "/capabilities"})

			var got struct{ Passkeys bool }
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || rec.Code != http.StatusOK || got.Passkeys != enabled {
				t.Errorf("capabilities = %d %+v %v, want passkeys=%v", rec.Code, got, err, enabled)
			}
		}
	})

	t.Run("reports passkeys off when no passkey service is configured", func(t *testing.T) {
		rig := newPasskeyRig(t)
		cfg := &domain.Config{BaseURL: "/", SessionSecret: "test-secret"}
		router := chi.NewRouter()
		handler := newAuthHandler(encoder{}, zerolog.Nop(), cfg, newCookieStore(cfg), rig.authSvc)
		newPasskeyHandler(handler, nil).Routes(router)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/capabilities", nil))

		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"passkeys":false`) {
			t.Errorf("capabilities = %d %q, want passkeys false", rec.Code, rec.Body.String())
		}
	})
}

func TestPasskeyHandler_loginBegin(t *testing.T) {
	t.Run("returns the options and hands the browser a short-lived ceremony cookie", func(t *testing.T) {
		rig := newPasskeyRig(t)

		rec := rig.do(call{method: http.MethodPost, path: "/passkey/login/begin"})

		cookie := cookieNamed(rec, "passkey_ceremony")
		if rec.Code != http.StatusOK || cookie == nil || cookie.Value != "login-ceremony" || !cookie.HttpOnly || cookie.MaxAge != 300 {
			t.Errorf("status = %d, cookie = %+v, want 200 and a 5 minute HttpOnly ceremony cookie", rec.Code, cookie)
		}
	})

	t.Run("answers 404 when passkeys are off", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.passkeys.enabled = false

		rec := rig.do(call{method: http.MethodPost, path: "/passkey/login/begin"})

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
}

func TestPasskeyHandler_loginFinish(t *testing.T) {
	finish := func(rig *passkeyRig, c call) *httptest.ResponseRecorder {
		c.method, c.path = http.MethodPost, "/passkey/login/finish"
		if c.contentType == "" {
			c.contentType = "application/json"
		}
		if c.ceremony == "" {
			c.ceremony = "login-ceremony"
		}
		if c.body == "" {
			c.body = `{}`
		}
		return rig.do(c)
	}

	t.Run("signs in, ends the ceremony and starts a session that stays valid", func(t *testing.T) {
		rig := newPasskeyRig(t)

		rec := finish(rig, call{})

		var body struct{ Username string }
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil || rec.Code != http.StatusOK || body.Username != "u" {
			t.Fatalf("status = %d, body = %+v, %v, want 200 and the username", rec.Code, body, err)
		}
		session := cookieNamed(rec, "user_session")
		cleared := cookieNamed(rec, "passkey_ceremony")
		if session == nil || cleared == nil || cleared.MaxAge >= 0 {
			t.Fatalf("session = %+v, ceremony cookie = %+v, want a session and a cleared ceremony cookie", session, cleared)
		}
		check := httptest.NewRequest(http.MethodGet, "/validate", nil)
		check.AddCookie(session)
		valid := httptest.NewRecorder()
		rig.router.ServeHTTP(valid, check)
		if valid.Code != http.StatusNoContent {
			t.Errorf("validate with the new session = %d, want 204", valid.Code)
		}
	})

	t.Run("answers 404 when passkeys are off", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.passkeys.enabled = false

		if rec := finish(rig, call{}); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("counts rejected passkeys towards the shared limit and then blocks even a valid one", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.passkeys.finishLogErr = passkey.ErrRejected
		for range maxFailedAttempts {
			if rec := finish(rig, call{}); rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401 for a rejected passkey", rec.Code)
			}
		}
		rig.passkeys.finishLogErr = nil
		callsBefore := rig.passkeys.loginFinishes

		rec := finish(rig, call{})

		if rec.Code != http.StatusTooManyRequests || rig.passkeys.loginFinishes != callsBefore || rec.Header().Get("Retry-After") == "" {
			t.Errorf("status = %d, service calls = %d (was %d), want 429 without calling the service", rec.Code, rig.passkeys.loginFinishes, callsBefore)
		}
		if len(rig.authSvc.blocked) != 1 {
			t.Errorf("NotifyLoginBlocked calls = %v, want one", rig.authSvc.blocked)
		}
	})

	t.Run("shares the budget with password logins", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.authSvc.loginErr = auth.ErrInvalidCredentials
		for range maxFailedAttempts {
			rec := httptest.NewRecorder()
			req := loginRequest("")
			req.RemoteAddr = "203.0.113.9:1000"
			rig.router.ServeHTTP(rec, req)
		}

		rec := finish(rig, call{})

		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("status = %d, want 429 after password failures from the same address", rec.Code)
		}
	})

	t.Run("refuses a cloned authenticator", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.passkeys.finishLogErr = passkey.ErrCloned

		if rec := finish(rig, call{}); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("does not count outcomes that are not a bad passkey", func(t *testing.T) {
		outcomes := map[string]struct {
			err        error
			call       call
			wantStatus int
		}{
			"expired ceremony":   {passkey.ErrCeremonyNotFound, call{}, http.StatusBadRequest},
			"service failure":    {errTest, call{}, http.StatusInternalServerError},
			"missing ceremony":   {nil, call{ceremony: "none"}, http.StatusBadRequest},
			"wrong content type": {nil, call{contentType: "text/plain"}, http.StatusUnsupportedMediaType},
			"oversized body":     {nil, call{body: `{"x":"` + strings.Repeat("a", maxPasskeyBodyBytes) + `"}`}, http.StatusRequestEntityTooLarge},
		}
		for name, tc := range outcomes {
			t.Run(name, func(t *testing.T) {
				rig := newPasskeyRig(t)
				rig.passkeys.finishLogErr = tc.err
				c := tc.call
				if c.ceremony == "none" {
					c.ceremony = ""
					c.method, c.path, c.contentType, c.body = http.MethodPost, "/passkey/login/finish", "application/json", `{}`
					for range maxFailedAttempts * 2 {
						if rec := rig.do(c); rec.Code != tc.wantStatus {
							t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
						}
					}
				} else {
					for range maxFailedAttempts * 2 {
						if rec := finish(rig, c); rec.Code != tc.wantStatus {
							t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
						}
					}
				}
				rig.passkeys.finishLogErr = nil

				if rec := finish(rig, call{}); rec.Code != http.StatusOK {
					t.Errorf("status after the rejected attempts = %d, want 200 (none of them should have counted)", rec.Code)
				}
			})
		}
	})
}

func TestPasskeyHandler_registerBegin(t *testing.T) {
	begin := func(rig *passkeyRig, c call) *httptest.ResponseRecorder {
		c.method, c.path = http.MethodPost, "/passkeys/register/begin"
		if c.contentType == "" {
			c.contentType = "application/json"
		}
		if c.body == "" {
			c.body = validRegisterBody
		}
		return rig.do(c)
	}

	t.Run("checks the password, then returns the options and the ceremony cookie", func(t *testing.T) {
		rig := newPasskeyRig(t)

		rec := begin(rig, call{session: true})

		cookie := cookieNamed(rec, "passkey_ceremony")
		if rec.Code != http.StatusOK || cookie == nil || cookie.Value != "reg-ceremony" {
			t.Fatalf("status = %d, cookie = %+v, want 200 and the ceremony cookie", rec.Code, cookie)
		}
		if len(rig.authSvc.verifyCalls) != 1 || rig.authSvc.verifyCalls[0] != "right-password-1" || rig.passkeys.registerNames[0] != "Laptop" {
			t.Errorf("verify calls = %v, names = %v, want the submitted password and name", rig.authSvc.verifyCalls, rig.passkeys.registerNames)
		}
	})

	t.Run("needs a web session, not just an API key", func(t *testing.T) {
		rig := newPasskeyRig(t)

		rec := begin(rig, call{})

		if rec.Code != http.StatusUnauthorized || len(rig.authSvc.verifyCalls) != 0 {
			t.Errorf("status = %d, verify calls = %v, want 401 before any password check", rec.Code, rig.authSvc.verifyCalls)
		}
	})

	t.Run("answers 404 when passkeys are off", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.passkeys.enabled = false

		if rec := begin(rig, call{session: true}); rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})

	t.Run("answers 403 for a wrong password and counts it towards the limit", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.authSvc.verifyErr = auth.ErrInvalidCredentials
		for range maxFailedAttempts {
			if rec := begin(rig, call{session: true}); rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", rec.Code)
			}
		}

		rec := begin(rig, call{session: true})

		if rec.Code != http.StatusTooManyRequests || len(rig.passkeys.registerNames) != 0 {
			t.Errorf("status = %d, begin calls = %v, want 429 and no ceremony", rec.Code, rig.passkeys.registerNames)
		}
	})

	t.Run("maps service outcomes without counting them", func(t *testing.T) {
		outcomes := map[string]struct {
			err        error
			wantStatus int
		}{
			"invalid name":  {passkey.ErrInvalidName, http.StatusBadRequest},
			"limit reached": {passkey.ErrLimit, http.StatusConflict},
			"server error":  {errTest, http.StatusInternalServerError},
		}
		for name, tc := range outcomes {
			t.Run(name, func(t *testing.T) {
				rig := newPasskeyRig(t)
				rig.passkeys.beginRegErr = tc.err
				for range maxFailedAttempts * 2 {
					if rec := begin(rig, call{session: true}); rec.Code != tc.wantStatus {
						t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
					}
				}
				rig.passkeys.beginRegErr = nil

				if rec := begin(rig, call{session: true}); rec.Code != http.StatusOK {
					t.Errorf("status afterwards = %d, want 200", rec.Code)
				}
			})
		}
	})

	t.Run("rejects a wrong content type and malformed JSON without counting", func(t *testing.T) {
		rig := newPasskeyRig(t)

		for range maxFailedAttempts * 2 {
			if rec := begin(rig, call{session: true, contentType: "text/plain"}); rec.Code != http.StatusUnsupportedMediaType {
				t.Fatalf("status = %d, want 415", rec.Code)
			}
			if rec := begin(rig, call{session: true, body: `{"name":`}); rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
		}

		if rec := begin(rig, call{session: true}); rec.Code != http.StatusOK {
			t.Errorf("status afterwards = %d, want 200", rec.Code)
		}
	})
}

func TestPasskeyHandler_registerFinish(t *testing.T) {
	finish := func(rig *passkeyRig, c call) *httptest.ResponseRecorder {
		c.method, c.path = http.MethodPost, "/passkeys/register/finish"
		if c.contentType == "" {
			c.contentType = "application/json"
		}
		if c.ceremony == "" {
			c.ceremony = "reg-ceremony"
		}
		if c.body == "" {
			c.body = `{}`
		}
		return rig.do(c)
	}

	t.Run("stores the passkey and returns it", func(t *testing.T) {
		rig := newPasskeyRig(t)

		rec := finish(rig, call{session: true})

		var got struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil || rec.Code != http.StatusCreated || got.ID != 7 || got.Name != "Laptop" {
			t.Errorf("status = %d, body = %+v, %v, want 201 and the new passkey", rec.Code, got, err)
		}
		if cleared := cookieNamed(rec, "passkey_ceremony"); cleared == nil || cleared.MaxAge >= 0 {
			t.Errorf("ceremony cookie = %+v, want it cleared", cleared)
		}
	})

	t.Run("needs a web session", func(t *testing.T) {
		rig := newPasskeyRig(t)

		if rec := finish(rig, call{}); rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("maps service outcomes", func(t *testing.T) {
		outcomes := map[string]struct {
			err        error
			wantStatus int
		}{
			"expired ceremony": {passkey.ErrCeremonyNotFound, http.StatusBadRequest},
			"rejected":         {passkey.ErrRejected, http.StatusBadRequest},
			"limit reached":    {passkey.ErrLimit, http.StatusConflict},
			"server error":     {errTest, http.StatusInternalServerError},
		}
		for name, tc := range outcomes {
			t.Run(name, func(t *testing.T) {
				rig := newPasskeyRig(t)
				rig.passkeys.finishRegErr = tc.err

				if rec := finish(rig, call{session: true}); rec.Code != tc.wantStatus {
					t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
				}
			})
		}
	})

	t.Run("needs the ceremony cookie and a JSON body", func(t *testing.T) {
		rig := newPasskeyRig(t)

		noCeremony := rig.do(call{method: http.MethodPost, path: "/passkeys/register/finish", contentType: "application/json", body: `{}`, session: true})
		wrongType := finish(rig, call{session: true, contentType: "text/plain"})

		if noCeremony.Code != http.StatusBadRequest || wrongType.Code != http.StatusUnsupportedMediaType {
			t.Errorf("statuses = %d and %d, want 400 and 415", noCeremony.Code, wrongType.Code)
		}
	})
}

func TestPasskeyHandler_listAndDelete(t *testing.T) {
	t.Run("lists the passkeys without credential data", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.passkeys.list = []domain.Passkey{
			{ID: 1, Name: "Laptop", CredentialJSON: `{"secret":"x"}`, CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
			{ID: 2, Name: "Phone", LastUsedAt: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)},
		}

		rec := rig.do(call{method: http.MethodGet, path: "/passkeys", session: true})

		body := rec.Body.String()
		if rec.Code != http.StatusOK || strings.Contains(body, "secret") || strings.Contains(body, "CredentialJSON") ||
			!strings.Contains(body, `"name":"Laptop"`) || !strings.Contains(body, `"last_used_at":null`) {
			t.Errorf("status = %d, body = %s, want the names, null for an unused passkey and no credential data", rec.Code, body)
		}
	})

	t.Run("returns an empty array when there are none", func(t *testing.T) {
		rig := newPasskeyRig(t)

		rec := rig.do(call{method: http.MethodGet, path: "/passkeys", session: true})

		if strings.TrimSpace(rec.Body.String()) != "[]" {
			t.Errorf("body = %q, want []", rec.Body.String())
		}
	})

	t.Run("listing and deleting need a web session", func(t *testing.T) {
		rig := newPasskeyRig(t)

		list := rig.do(call{method: http.MethodGet, path: "/passkeys"})
		del := rig.do(call{method: http.MethodDelete, path: "/passkeys/1"})

		if list.Code != http.StatusUnauthorized || del.Code != http.StatusUnauthorized || len(rig.passkeys.deletedIDs) != 0 {
			t.Errorf("statuses = %d and %d, deleted = %v, want 401, 401 and nothing deleted", list.Code, del.Code, rig.passkeys.deletedIDs)
		}
	})

	t.Run("deletes a passkey", func(t *testing.T) {
		rig := newPasskeyRig(t)

		rec := rig.do(call{method: http.MethodDelete, path: "/passkeys/3", session: true})

		if rec.Code != http.StatusNoContent || len(rig.passkeys.deletedIDs) != 1 || rig.passkeys.deletedIDs[0] != 3 {
			t.Errorf("status = %d, deleted = %v, want 204 for id 3", rec.Code, rig.passkeys.deletedIDs)
		}
	})

	t.Run("answers 404 for an unknown passkey and 400 for a bad id", func(t *testing.T) {
		rig := newPasskeyRig(t)
		rig.passkeys.deleteOK = false

		unknown := rig.do(call{method: http.MethodDelete, path: "/passkeys/3", session: true})
		bad := rig.do(call{method: http.MethodDelete, path: "/passkeys/abc", session: true})

		if unknown.Code != http.StatusNotFound || bad.Code != http.StatusBadRequest {
			t.Errorf("statuses = %d and %d, want 404 and 400", unknown.Code, bad.Code)
		}
	})
}
