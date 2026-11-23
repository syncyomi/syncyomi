package config

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

func NormalizeOrigins(origins []string) ([]string, error) {
	normalized := make([]string, 0, len(origins))

	for _, origin := range origins {
		parsed, err := url.Parse(strings.TrimSpace(origin))
		if err != nil {
			return nil, fmt.Errorf("corsAllowedOrigins: %q is not a valid URL: %w", origin, err)
		}

		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("corsAllowedOrigins: %q must start with http:// or https://", origin)
		}
		if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return nil, fmt.Errorf("corsAllowedOrigins: %q must be scheme://host[:port] with no path, query or credentials", origin)
		}

		host := strings.ToLower(parsed.Host)
		if strings.ContainsAny(host, "*") || strings.HasSuffix(host, ":") || !isASCII(host) {
			return nil, fmt.Errorf("corsAllowedOrigins: %q must be an exact ASCII host (no wildcard, empty port or non-ASCII characters; use the punycode form)", origin)
		}
		if parsed.Port() == defaultPorts[scheme] {
			host = strings.TrimSuffix(host, ":"+parsed.Port())
		}

		entry := scheme + "://" + host
		if !slices.Contains(normalized, entry) {
			normalized = append(normalized, entry)
		}
	}

	return normalized, nil
}

var defaultPorts = map[string]string{"http": "80", "https": "443"}

func isASCII(text string) bool {
	for i := 0; i < len(text); i++ {
		if text[i] >= utf8.RuneSelf {
			return false
		}
	}

	return true
}
