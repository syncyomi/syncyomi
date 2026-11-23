package config

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
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

		entry := scheme + "://" + strings.ToLower(parsed.Host)
		if !slices.Contains(normalized, entry) {
			normalized = append(normalized, entry)
		}
	}

	return normalized, nil
}
