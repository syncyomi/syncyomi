package domain

import (
	"context"
	"time"
)

type APIRepo interface {
	Store(ctx context.Context, key *APIKey) error
	Delete(ctx context.Context, key string) error
	GetKeys(ctx context.Context) ([]APIKey, error)
	Get(ctx context.Context, key string) (*APIKey, error)
}

type APIKey struct {
	Name      string     `json:"name,omitempty"`
	Key       string     `json:"key,omitempty"`
	Scopes    []string   `json:"scopes,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}
