package domain

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
)

const passwordStampLength = 16

type UserRepo interface {
	GetUserCount(ctx context.Context) (int, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindFirst(ctx context.Context) (*User, error)
	Store(ctx context.Context, user User) error
	UpdatePassword(ctx context.Context, id int, hash, stamp string) error
	RehashPassword(ctx context.Context, id int, oldHash, newHash string) error
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"`
	Stamp    string `json:"-"`
}

func (u User) PasswordStamp() string {
	if u.Stamp != "" {
		return u.Stamp
	}

	return u.LegacyPasswordStamp()
}

func (u User) LegacyPasswordStamp() string {
	sum := sha256.Sum256([]byte(u.Password))
	return hex.EncodeToString(sum[:])[:passwordStampLength]
}
