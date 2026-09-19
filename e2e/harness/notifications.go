package harness

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

var notificationWhenRe = regexp.MustCompile(`when=(\d+)`)

func (e *Emulator) LastRestoreCompleteNotification(ctx context.Context) (int64, error) {
	out, err := e.AdbShellStdin(ctx, "", "dumpsys notification --noredact")
	if err != nil {
		return 0, err
	}
	var best int64
	for _, block := range strings.Split(out, "NotificationRecord(") {
		if !strings.Contains(block, "Library sync complete") {
			continue
		}
		for _, m := range notificationWhenRe.FindAllStringSubmatch(block, -1) {
			if v, err := strconv.ParseInt(m[1], 10, 64); err == nil && v > best {
				best = v
			}
		}
	}
	return best, nil
}
