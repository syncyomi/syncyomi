package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

func normalizeWebauthnOrigin(origin string) (string, error) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return "", nil
	}

	parsed, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("webauthnOrigin: %q is not a valid URL: %w", origin, err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	host := strings.ToLower(parsed.Hostname())
	if (scheme != "http" && scheme != "https") || host == "" || strings.Contains(host, "*") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("webauthnOrigin: %q must be scheme://host[:port] with no path, query, credentials or wildcard", origin)
	}

	if _, err := netip.ParseAddr(host); err == nil {
		return "", fmt.Errorf("webauthnOrigin: %q uses an IP address, but passkeys need a domain name", origin)
	}

	if scheme == "http" && host != "localhost" && !strings.HasSuffix(host, ".localhost") {
		return "", fmt.Errorf("webauthnOrigin: %q must use https (plain http only works for localhost)", origin)
	}

	if parsed.Port() == "" || parsed.Port() == defaultPort(scheme) {
		return scheme + "://" + host, nil
	}

	return scheme + "://" + net.JoinHostPort(host, parsed.Port()), nil
}

func defaultPort(scheme string) string {
	if scheme == "https" {
		return "443"
	}

	return "80"
}
