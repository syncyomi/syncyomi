package config

import "testing"

func TestWebauthnOriginNormalisation(t *testing.T) {
	t.Run("treats an empty origin as passkeys switched off", func(t *testing.T) {
		got, err := normalizeWebauthnOrigin("")

		if got != "" || err != nil {
			t.Errorf("normalizeWebauthnOrigin(\"\") = %q, %v, want empty and no error", got, err)
		}
	})

	valid := map[string]string{
		"https://Sync.Example.com":       "https://sync.example.com",
		"https://sync.example.com:8443/": "https://sync.example.com:8443",
		"https://sync.example.com:443":   "https://sync.example.com",
		"http://localhost:8282":          "http://localhost:8282",
		"http://app.localhost:8282":      "http://app.localhost:8282",
	}
	for in, want := range valid {
		t.Run("accepts "+in, func(t *testing.T) {
			got, err := normalizeWebauthnOrigin(in)

			if err != nil || got != want {
				t.Errorf("normalizeWebauthnOrigin(%q) = %q, %v, want %q", in, got, err, want)
			}
		})
	}

	invalid := []string{
		"http://sync.example.com",
		"http://192.168.1.10:8282",
		"https://192.168.1.10",
		"https://[::1]",
		"http://127.0.0.1:8282",
		"sync.example.com",
		"https://sync.example.com/app",
		"https://*.example.com",
		"https://user@sync.example.com",
	}
	for _, in := range invalid {
		t.Run("rejects "+in, func(t *testing.T) {
			if _, err := normalizeWebauthnOrigin(in); err == nil {
				t.Errorf("normalizeWebauthnOrigin(%q) error = nil, want an error", in)
			}
		})
	}
}
