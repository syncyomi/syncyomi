package domain

import (
	"context"
	"time"
)

type SyncRepo interface {
	Store(ctx context.Context, sync *Sync) (*Sync, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, sync *Sync) (*Sync, error)
	ListSyncs(ctx context.Context, apiKey string) ([]Sync, error)
	GetSyncByApiKey(ctx context.Context, apiKey string) (*Sync, error)
	GetSyncByDeviceID(ctx context.Context, deviceID int) (*Sync, error)
	SyncData(ctx context.Context, sync *SyncData) (*SyncData, error)
}

type Sync struct {
	ID              int        `json:"id,omitempty"`
	LastSynced      *time.Time `json:"last_synced,omitempty"`
	Status          SyncStatus `json:"status,omitempty"`
	Device          *Device    `json:"device,omitempty"`
	UserApiKey      *APIKey    `json:"user_api_key,omitempty"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
	UpdatedAt       *time.Time `json:"updated_at,omitempty"`
	LastSyncedEpoch int64      `json:"last_synced_epoch,omitempty"`
}

type SyncStatus string

const (
	SyncStatusPending SyncStatus = "pending"
	SyncStatusSyncing SyncStatus = "syncing"
	SyncStatusSuccess SyncStatus = "success"
	SyncStatusError   SyncStatus = "error"
)

type SyncData struct {
	Sync   *Sync      `json:"sync,omitempty"`
	Data   *MangaData `json:"backup,omitempty"`
	Device *Device    `json:"device,omitempty"`
}
