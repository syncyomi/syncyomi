package config

import (
	"slices"
	"testing"
)

func TestNormalizeOrigins(t *testing.T) {
	t.Run("accepts exact origins and normalises case and a trailing slash", func(t *testing.T) {
		got, err := NormalizeOrigins([]string{"https://Sync.Example.com", "http://localhost:3000/", "https://sync.example.com"})

		want := []string{"https://sync.example.com", "http://localhost:3000"}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("NormalizeOrigins() = %v, %v, want %v", got, err, want)
		}
	})

	t.Run("drops a port that is the scheme default", func(t *testing.T) {
		got, err := NormalizeOrigins([]string{"https://example.com:443", "http://example.com:80", "http://example.com:8080"})

		want := []string{"https://example.com", "http://example.com", "http://example.com:8080"}
		if err != nil || !slices.Equal(got, want) {
			t.Errorf("NormalizeOrigins() = %v, %v, want %v", got, err, want)
		}
	})

	t.Run("accepts an empty list", func(t *testing.T) {
		got, err := NormalizeOrigins(nil)

		if err != nil || len(got) != 0 {
			t.Errorf("NormalizeOrigins(nil) = %v, %v, want nothing", got, err)
		}
	})

	invalid := []string{
		"*",
		"",
		"example.com",
		"//example.com",
		"ftp://example.com",
		"https://example.com/app",
		"https://example.com?x=1",
		"https://user@example.com",
		"https://",
		"https://*.example.com",
		"http://example.com:",
		"https://münchen.de",
	}
	for _, origin := range invalid {
		t.Run("rejects "+origin, func(t *testing.T) {
			if _, err := NormalizeOrigins([]string{origin}); err == nil {
				t.Errorf("NormalizeOrigins(%q) error = nil, want an error", origin)
			}
		})
	}
}
