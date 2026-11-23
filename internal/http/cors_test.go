package http

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func serveCORS(t *testing.T, allowed []string, method, origin string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	handler := newCORS(allowed)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(method, "/api/healthz/liveness", nil)
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestCORS(t *testing.T) {
	allowed := []string{"https://sync.example.com", "http://localhost:3000"}

	t.Run("reflects a listed origin and allows credentials", func(t *testing.T) {
		rec := serveCORS(t, allowed, http.MethodGet, "https://sync.example.com", nil)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://sync.example.com" {
			t.Errorf("Allow-Origin = %q, want the listed origin", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("Allow-Credentials = %q, want true", got)
		}
	})

	t.Run("lets any other origin read responses but never with credentials", func(t *testing.T) {
		rec := serveCORS(t, allowed, http.MethodGet, "https://evil.example", nil)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
			t.Errorf("Allow-Origin = %q, want *", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
			t.Errorf("Allow-Credentials = %q, want none", got)
		}
	})

	t.Run("answers a preflight from an unlisted origin without credentials", func(t *testing.T) {
		rec := serveCORS(t, allowed, http.MethodOptions, "https://evil.example", map[string]string{
			"Access-Control-Request-Method": "POST",
		})

		if rec.Header().Get("Access-Control-Allow-Origin") != "*" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("preflight headers = %v, want * and no credentials", rec.Header())
		}
	})

	t.Run("answers a preflight from a listed origin with credentials", func(t *testing.T) {
		rec := serveCORS(t, allowed, http.MethodOptions, "http://localhost:3000", map[string]string{
			"Access-Control-Request-Method": "POST",
		})

		if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" || rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("preflight headers = %v, want the listed origin with credentials", rec.Header())
		}
	})

	t.Run("treats everything as unlisted when the list is empty", func(t *testing.T) {
		rec := serveCORS(t, nil, http.MethodGet, "https://sync.example.com", nil)

		if rec.Header().Get("Access-Control-Allow-Origin") != "*" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("headers = %v, want * and no credentials", rec.Header())
		}
	})

	t.Run("adds nothing to a request without an Origin header", func(t *testing.T) {
		rec := serveCORS(t, allowed, http.MethodGet, "", nil)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Allow-Origin = %q, want none", got)
		}
	})

	t.Run("tells caches the response depends on Origin", func(t *testing.T) {
		for _, origin := range []string{"https://sync.example.com", "https://evil.example", ""} {
			rec := serveCORS(t, allowed, http.MethodGet, origin, nil)

			if !slices.Contains(rec.Header().Values("Vary"), "Origin") {
				t.Errorf("Vary for origin %q = %v, want it to include Origin", origin, rec.Header().Values("Vary"))
			}
		}
	})

	t.Run("passes the request on to the handler either way", func(t *testing.T) {
		for _, origin := range []string{"https://sync.example.com", "https://evil.example", ""} {
			if rec := serveCORS(t, allowed, http.MethodGet, origin, nil); rec.Code != http.StatusOK {
				t.Errorf("status for origin %q = %d, want 200", origin, rec.Code)
			}
		}
	})
}
