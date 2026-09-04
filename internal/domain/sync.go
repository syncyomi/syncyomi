package domain

import (
	"context"
	"errors"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/merge"
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
	Protocol    string    `json:"protocol"` // "", v1 or v2
	Cursor      int64     `json:"cursor"`
	CreatedAt   time.Time `json:"created_at"`
}

type SyncStatus struct {
	LastSyncedAt  *time.Time `json:"last_synced_at"`
	LastEventAt   *time.Time `json:"last_event_at"`
	LastEvent     string     `json:"last_event"`
	LastStatus    string     `json:"last_status"`
	LastDevice    string     `json:"last_device"`
	LastMessage   string     `json:"last_message"`
	LastProtocol  string     `json:"last_protocol"` // "", v1 or v2
	DataSize      int64      `json:"data_size"`
	DataUpdatedAt *time.Time `json:"data_updated_at"`
	// Seq is the server's change counter; a device whose cursor is behind it has
	// that many merges still to pull.
	Seq           int64 `json:"seq"`
	MangaCount    int64 `json:"manga_count"`
	ChapterCount  int64 `json:"chapter_count"`
	CategoryCount int64 `json:"category_count"`
	HistoryLimit  int   `json:"history_limit"`
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
	// GetHistoryData returns the stored payload of a history entry. ErrNotFound if id is unknown.
	GetHistoryData(ctx context.Context, apiKey string, id int) ([]byte, error)

	// Empty fields never overwrite stored values, and protocol is only filled when the row
	// has none yet (SetDeviceCursor owns updates). No-op when dev has no ID and no Name.
	TouchDevice(ctx context.Context, apiKey string, dev DeviceInfo, event, status, message, protocol string) error
	ListDevices(ctx context.Context, apiKey string) ([]SyncDevice, error)
	// DeleteDevice removes a device row. ErrNotFound if id is unknown for the key.
	DeleteDevice(ctx context.Context, apiKey string, id int) error

	// Nil timestamps and empty strings never overwrite stored values.
	UpsertStatus(ctx context.Context, apiKey string, st SyncStatus) error
	// Returns (nil, nil) when nothing is known about the key.
	GetStatus(ctx context.Context, apiKey string) (*SyncStatus, error)
}

// RenderCache is the last full backup rendered from the item store, served to v1 clients
// only when no raw client blob is current. RenderedSeq is nil for a blob written by a
// pre-v2 server that has not been promoted to a raw blob yet.
type RenderCache struct {
	Data        []byte
	ETag        string
	RenderedSeq *int64
}

// RawBlob is the last v1 client upload, kept byte-for-byte. It is served back verbatim
// while Seq still equals the store's seq (no other device has written since).
type RawBlob struct {
	Data []byte
	ETag string
	Seq  int64
}

type DeviceCursor struct {
	Device   DeviceInfo
	Cursor   int64
	Protocol string
}

// SyncStoreTx is one transaction over an API key's item store. Store.Tx serialises
// transactions per key.
type SyncStoreTx interface {
	Seq() int64
	// Exists reports whether the item store has ever been written for this key.
	Exists() bool
	GetItems(ctx context.Context, kind merge.Kind, keys []string) (map[string]*merge.Item, error)
	// Categories returns every category item, tombstoned ones included.
	Categories(ctx context.Context) ([]*merge.Item, error)
	CountOfKind(ctx context.Context, kind merge.Kind) (int, error)
	ItemsOfKind(ctx context.Context, kind merge.Kind) ([]*merge.Item, error)
	// Apply writes the merge result and returns the new seq (unchanged when nothing was written).
	Apply(ctx context.Context, res *merge.Result, device string) (int64, error)
	AllItems(ctx context.Context) ([]*merge.Item, error)
	// ItemsSince returns items changed after seq. Categories are always returned in full.
	ItemsSince(ctx context.Context, seq int64) ([]*merge.Item, error)
	ItemsByKeys(ctx context.Context, keys map[merge.Kind][]string) ([]*merge.Item, error)
	// ChaptersOf returns every chapter of the given manga keys.
	ChaptersOf(ctx context.Context, mangaKeys []string) ([]*merge.Item, error)
	RenderCache(ctx context.Context) (*RenderCache, error)
	SetRenderCache(ctx context.Context, data []byte, etag string, seq int64) error
	// MarkRendered stamps the existing cache as matching seq without rewriting it.
	MarkRendered(ctx context.Context, seq int64) error
	// RawBlob returns the last v1 upload, or nil when none was ever stored.
	RawBlob(ctx context.Context) (*RawBlob, error)
	// SetRawBlob stores a v1 upload verbatim and records it in the history.
	SetRawBlob(ctx context.Context, data []byte, etag string, seq int64) error
	// MarkRawCurrent stamps the existing raw blob as matching seq without rewriting it.
	MarkRawCurrent(ctx context.Context, seq int64) error
	// Clear removes every item so the store can be rebuilt from a backup.
	Clear(ctx context.Context) error
	SetDeviceCursor(ctx context.Context, dc DeviceCursor) error
}

type SyncStore interface {
	Tx(ctx context.Context, apiKey string, fn func(tx SyncStoreTx) error) error
}
