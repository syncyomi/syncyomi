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
	CreateSyncLockFile(ctx context.Context, apiKey string, acquiredBy string) (*SyncLockFile, error)
	GetSyncLockFile(ctx context.Context, apiKey string) (*SyncLockFile, error)
	UpdateSyncLockFile(ctx context.Context, syncLockFile *SyncLockFile) (*SyncLockFile, error)
	DeleteSyncLockFile(ctx context.Context, apiKey string) bool
}

type Sync struct {
	ID              int        `json:"id,omitempty"`
	LastSynced      *time.Time `json:"last_synced,omitempty"`
	Status          SyncStatus `json:"status,omitempty"`
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
	Sync *Sync       `json:"sync,omitempty"`
	Data *BackupData `json:"backup,omitempty"`
}

type SyncLockFile struct {
	ID         int        `json:"id,omitempty"`
	UserApiKey string     `json:"user_api_key,omitempty"`
	AcquiredBy string     `json:"acquired_by,omitempty"`
	LastSynced *time.Time `json:"last_synced,omitempty"`
	Status     SyncStatus `json:"status,omitempty"`
	RetryCount int        `json:"retry_count,omitempty"`
	AcquiredAt *time.Time `json:"acquired_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}
