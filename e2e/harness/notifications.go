package harness

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

const syncFailedTitle = "android.title=String (Syncing library failed)"

var (
	notificationWhenRe = regexp.MustCompile(`when=(\d+)`)
	notificationTextRe = regexp.MustCompile(`(?m)android\.text=String \((.*)\)\s*$`)
	connectFailureRe   = regexp.MustCompile(`(?i)^failed to connect to `)
)

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

type SyncError struct {
	When int64
	Text string
}

func (s SyncError) Transient() bool {
	return connectFailureRe.MatchString(s.Text)
}

func (e *Emulator) LastSyncError(ctx context.Context) (SyncError, error) {
	dump, err := e.notificationDump(ctx)
	if err != nil {
		return SyncError{}, err
	}
	return parseSyncError(dump), nil
}

func parseSyncError(dump string) SyncError {
	var last SyncError
	for _, record := range splitNotificationRecords(dump) {
		if !strings.Contains(record, syncFailedTitle) {
			continue
		}
		when := latestWhen(record)
		if when <= last.When {
			continue
		}
		last = SyncError{When: when}
		if m := notificationTextRe.FindStringSubmatch(record); m != nil {
			last.Text = m[1]
		}
	}
	return last
}
