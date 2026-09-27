package harness

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

var notificationWhenRe = regexp.MustCompile(`when=(\d+)`)

func (e *Emulator) LastRestoreCompleteNotification(ctx context.Context) (int64, error) {
	dump, err := e.notificationDump(ctx)
	if err != nil {
		return 0, err
	}
	var best int64
	for _, record := range splitNotificationRecords(dump) {
		if strings.Contains(record, "Library sync complete") {
			best = max(best, latestWhen(record))
		}
	}
	return best, nil
}

func (e *Emulator) notificationDump(ctx context.Context) (string, error) {
	return e.AdbShellStdin(ctx, "", "dumpsys notification --noredact")
}

func splitNotificationRecords(dump string) []string {
	return strings.Split(dump, "NotificationRecord(")
}

func latestWhen(record string) int64 {
	var latest int64
	for _, m := range notificationWhenRe.FindAllStringSubmatch(record, -1) {
		if v, err := strconv.ParseInt(m[1], 10, 64); err == nil {
			latest = max(latest, v)
		}
	}
	return latest
}
