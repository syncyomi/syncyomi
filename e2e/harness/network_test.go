package harness

import "testing"

func TestParseHostProbe(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   hostProbe
	}{
		{"an http answer proves the path to the host", "HTTP/1.0 200 OK\r\nContent-Type: text/plain\r\n\r\nOK", hostReachable},
		{"any http status proves the path", "HTTP/1.1 503 Service Unavailable\r\n\r\n", hostReachable},
		{"a refused connection means nothing listens on the port", "nc: connect: Connection refused\n", serverRefused},
		{"no route means the guest network is down", "nc: connect: Network is unreachable\n", hostUnreachable},
		{"no output means the probe timed out", "", hostUnreachable},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseHostProbe(tc.output)

			if got != tc.want {
				t.Errorf("parseHostProbe(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}
