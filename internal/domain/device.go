package domain

import (
	"context"
	"time"
)

type DeviceRepo interface {
	Store(ctx context.Context, device *Device) error
	Delete(ctx context.Context, id int) error
	ListDevices(ctx context.Context, apikey string) ([]Device, error)
}

type Device struct {
	ID         int       `json:"id"`
	Name       string    `json:"name"`
	UserApiKey *APIKey   `json:"user_api_key"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
