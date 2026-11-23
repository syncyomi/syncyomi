package domain

import (
	"context"
	"time"
)

type Passkey struct {
	ID             int
	UserID         int
	CredentialID   []byte
	Name           string
	CredentialJSON string
	CreatedAt      time.Time
	LastUsedAt     time.Time
}

type PasskeyRepo interface {
	List(ctx context.Context, userID int) ([]Passkey, error)
	FindByCredentialID(ctx context.Context, credentialID []byte) (*Passkey, error)
	Store(ctx context.Context, passkey Passkey) (int, error)
	UpdateCredential(ctx context.Context, id int, credentialJSON string, usedAt time.Time) error
	Delete(ctx context.Context, userID, id int) (bool, error)
	DeleteAll(ctx context.Context, userID int) (int, error)
}
