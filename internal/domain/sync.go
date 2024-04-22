package domain

import (
	"context"
)

type SyncRepo interface {
	// Get etag of sync data.
	// For avoid memory usage, only the etag will be returned.
	GetSyncDataETag(ctx context.Context, apiKey string) (*string, error)
	// Get sync data and etag
	GetSyncDataAndETag(ctx context.Context, apiKey string) ([]byte, *string, error)
	// Create or replace sync data, returns the new etag.
	SetSyncData(ctx context.Context, apiKey string, data []byte) (*string, error)
	// Replace sync data only if the etag matches,
	// returns the new etag if updated, or nil if not.
	SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error)
}
