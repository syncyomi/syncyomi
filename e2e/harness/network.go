package harness

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

type hostProbe int

const (
	hostUnreachable hostProbe = iota
	hostReachable
	serverRefused
)

const (
	hostProbeTimeout  = 10 * time.Second
	hostProbeInterval = 2 * time.Second
)

var (
	httpStatusLineRe = regexp.MustCompile(`(?m)^HTTP/1\.[01] \d{3} `)
	connRefusedRe    = regexp.MustCompile(`(?i)connection refused`)
)

type networkRepair struct {
	name    string
	command string
	wait    time.Duration
}

var networkRepairs = []networkRepair{
	{name: "waiting", wait: 10 * time.Second},
	{name: "a wifi toggle", command: "svc wifi disable; sleep 2; svc wifi enable", wait: 30 * time.Second},
	{name: "an airplane-mode toggle", command: "cmd connectivity airplane-mode enable; sleep 2; cmd connectivity airplane-mode disable", wait: 45 * time.Second},
}

func parseHostProbe(output string) hostProbe {
	switch {
	case httpStatusLineRe.MatchString(output):
		return hostReachable
	case connRefusedRe.MatchString(output):
		return serverRefused
	default:
		return hostUnreachable
	}
}

func (e *Emulator) EnsureHostReachable(ctx context.Context, port int, logf func(format string, args ...any)) error {
	target := fmt.Sprintf("10.0.2.2:%d", port)
	out, probe := e.probeHost(ctx, port)
	if probe == hostReachable {
		return nil
	}
	started := time.Now()
	logf("%s cannot reach %s: %s", e.AVD, target, strings.TrimSpace(out))
	for _, repair := range networkRepairs {
		if probe == serverRefused {
			break
		}
		if repair.command != "" {
			logf("%s: trying %s", e.AVD, repair.name)
			if _, err := e.Adb(ctx, "shell", repair.command); err != nil {
				return fmt.Errorf("%s: %s: %w", e.AVD, repair.name, err)
			}
		}
		out, probe = e.waitForHost(ctx, port, repair.wait)
		if probe == hostReachable {
			logf("%s reached %s again after %s, by %s", e.AVD, target, time.Since(started).Round(time.Second), repair.name)
			return nil
		}
	}
	if probe == serverRefused {
		return fmt.Errorf("%s: nothing listens on %s: %s", e.AVD, target, strings.TrimSpace(out))
	}
	return fmt.Errorf("%s cannot reach %s after a wifi and an airplane-mode toggle: %s", e.AVD, target, strings.TrimSpace(out))
}

func (e *Emulator) waitForHost(ctx context.Context, port int, budget time.Duration) (string, hostProbe) {
	deadline := time.Now().Add(budget)
	for {
		out, probe := e.probeHost(ctx, port)
		if probe != hostUnreachable || !time.Now().Before(deadline) || ctx.Err() != nil {
			return out, probe
		}
		time.Sleep(hostProbeInterval)
	}
}

func (e *Emulator) probeHost(ctx context.Context, port int) (string, hostProbe) {
	ctx, cancel := context.WithTimeout(ctx, hostProbeTimeout)
	defer cancel()
	request := fmt.Sprintf(`printf 'GET /api/healthz/liveness HTTP/1.0\r\n\r\n' | nc -w 5 -W 5 10.0.2.2 %d`, port)
	out, _ := e.adbOnce(ctx, "shell", request)
	return out, parseHostProbe(out)
}

var networkReportCommands = []string{
	"ip addr",
	"ip rule",
	"ip route show table all",
	"cmd wifi status",
	"settings get global airplane_mode_on",
	"dumpsys connectivity",
	"dumpsys wifi",
}

func (e *Emulator) networkReport(ctx context.Context) string {
	var b strings.Builder
	for _, command := range networkReportCommands {
		out, err := e.Adb(ctx, "shell", command)
		fmt.Fprintf(&b, "$ %s\n%s\n", command, out)
		if err != nil {
			fmt.Fprintf(&b, "exit: %v\n", errors.Unwrap(err))
		}
	}
	return b.String()
}
