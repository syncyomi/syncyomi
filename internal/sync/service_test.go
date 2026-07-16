package sync

import (
	"errors"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
)

func TestParseSyncEvent(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		want    domain.NotificationEvent
		wantErr bool
	}{
		{name: "started", event: "SYNC_STARTED", want: domain.NotificationEventSyncStarted},
		{name: "success", event: "SYNC_SUCCESS", want: domain.NotificationEventSyncSuccess},
		{name: "failed", event: "SYNC_FAILED", want: domain.NotificationEventSyncFailed},
		{name: "error", event: "SYNC_ERROR", want: domain.NotificationEventSyncError},
		{name: "cancelled", event: "SYNC_CANCELLED", want: domain.NotificationEventSyncCancelled},
		{name: "unknown is rejected", event: "SYNC_WHATEVER", wantErr: true},
		{name: "empty is rejected", event: "", wantErr: true},
		{name: "non-sync event is rejected", event: "TEST", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSyncEvent(tt.event)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidSyncEvent) {
					t.Errorf("parseSyncEvent(%q) err = %v, want ErrInvalidSyncEvent", tt.event, err)
				}
				return
			}
			if err != nil {
				t.Errorf("parseSyncEvent(%q) unexpected error: %v", tt.event, err)
			}
			if got != tt.want {
				t.Errorf("parseSyncEvent(%q) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}
