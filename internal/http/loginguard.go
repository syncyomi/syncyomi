package http

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"time"
)

const ipv6PrefixBits = 64

func clientKey(r *http.Request) string {
	host := r.RemoteAddr
	if withoutPort, _, err := net.SplitHostPort(host); err == nil {
		host = withoutPort
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return host
	}

	addr = addr.WithZone("").Unmap()
	if !addr.Is6() {
		return addr.String()
	}

	prefix, err := addr.Prefix(ipv6PrefixBits)
	if err != nil {
		return addr.String()
	}

	return prefix.String()
}

func (h authHandler) reserveAttempt(w http.ResponseWriter, client string) bool {
	reservation := h.limiter.reserve(client)
	if reservation.allowed {
		return true
	}

	if reservation.justBlocked {
		h.service.NotifyLoginBlocked(client)
	}

	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(reservation.retryAfter.Seconds()))))
	http.Error(w, fmt.Sprintf("Too many failed attempts. Try again in %s.", describeWait(reservation.retryAfter)), http.StatusTooManyRequests)

	return false
}

func describeWait(wait time.Duration) string {
	minutes := int(math.Ceil(wait.Minutes()))
	if minutes <= 1 {
		return "a minute"
	}

	return fmt.Sprintf("%d minutes", minutes)
}
