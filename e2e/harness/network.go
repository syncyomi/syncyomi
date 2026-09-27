package harness

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

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
