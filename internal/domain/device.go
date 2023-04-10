package domain

import (
	"context"
	"time"
)

type DeviceRepo interface {
	Store(ctx context.Context, device *Device) (*Device, error)
	Delete(ctx context.Context, id int) error
	ListDevices(ctx context.Context, apikey string) ([]Device, error)
	GetDeviceByDeviceId(ctx context.Context, device *Device) (*Device, error)
	GetDeviceByApiKey(ctx context.Context, device *Device) (*Device, error)
}

type Device struct {
	ID         int        `json:"id,omitempty"`
	Name       string     `json:"name,omitempty"`
	UserApiKey *APIKey    `json:"user_api_key,omitempty"`
	CreatedAt  *time.Time `json:"created_at,omitempty"`
	UpdatedAt  *time.Time `json:"updated_at,omitempty"`
}
