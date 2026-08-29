package domain

import (
	"context"
	"errors"
	"time"
)

var ErrNotFound = errors.New("not found")

type SyncHistoryEntry struct {
	ID        int       `json:"id"`
	ETag      string    `json:"etag"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

type SyncDevice struct {
	ID          int       `json:"id"`
	DeviceID    string    `json:"device_id"`
	DeviceName  string    `json:"device_name"`
	LastSeen    time.Time `json:"last_seen"`
	LastEvent   string    `json:"last_event"`
	LastStatus  string    `json:"last_status"`
	LastMessage string    `json:"last_message"`
	CreatedAt   time.Time `json:"created_at"`
}

type SyncStatus struct {
	LastSyncedAt  *time.Time `json:"last_synced_at"`
	LastEventAt   *time.Time `json:"last_event_at"`
	LastEvent     string     `json:"last_event"`
	LastStatus    string     `json:"last_status"`
	LastDevice    string     `json:"last_device"`
	LastMessage   string     `json:"last_message"`
	DataSize      int64      `json:"data_size"`
	DataUpdatedAt *time.Time `json:"data_updated_at"`
}

// DeviceInfo is what a client optionally identifies itself with. All fields may be empty.
type DeviceInfo struct {
	ID   string
	Name string
}

func (d DeviceInfo) Key() string {
	if d.ID != "" {
		return d.ID
	}
	return d.Name
}

type SyncRepo interface {
	// Returns (nil, nil) when no data exists.
	GetSyncDataETag(ctx context.Context, apiKey string) (*string, error)
	GetSyncDataAndETag(ctx context.Context, apiKey string) ([]byte, *string, error)
	// Create or replace sync data, returns the new etag.
	SetSyncData(ctx context.Context, apiKey string, data []byte) (*string, error)
	// Returns the new etag if the stored etag matched, or (nil, nil) if not.
	SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error)

	ListHistory(ctx context.Context, apiKey string) ([]SyncHistoryEntry, error)
	// Puts a history entry back as the current sync data and returns the new etag. ErrNotFound if id is unknown.
	RestoreHistory(ctx context.Context, apiKey string, id int) (*string, error)

	// Empty fields never overwrite stored values. No-op when dev has no ID and no Name.
	TouchDevice(ctx context.Context, apiKey string, dev DeviceInfo, event, status, message string) error
	ListDevices(ctx context.Context, apiKey string) ([]SyncDevice, error)

	// Nil timestamps and empty strings never overwrite stored values.
	UpsertStatus(ctx context.Context, apiKey string, st SyncStatus) error
	// Returns (nil, nil) when nothing is known about the key.
	GetStatus(ctx context.Context, apiKey string) (*SyncStatus, error)
}
