package domain

import "context"

type UserRepo interface {
	GetUserCount(ctx context.Context) (int, error)
	FindByUsername(ctx context.Context, username string) (*User, error)
	FindFirst(ctx context.Context) (*User, error)
	Store(ctx context.Context, user User) error
	Update(ctx context.Context, user User) error
	UpdatePassword(ctx context.Context, id int, hash string) error
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"password"`
}
