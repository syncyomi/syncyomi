package database

import (
	"context"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/logger"
	"github.com/rs/zerolog"
)

func NewDeviceRepo(log logger.Logger, db *DB) domain.DeviceRepo {
	return &DeviceRepo{
		log: log.With().Str("module", "device").Logger(),
		db:  db,
	}
}

type DeviceRepo struct {
	log zerolog.Logger
	db  *DB
}

func (d DeviceRepo) Store(ctx context.Context, device *domain.Device) error {
	//TODO implement me
	panic("implement me")
}

func (d DeviceRepo) Delete(ctx context.Context, id int) error {
	//TODO implement me
	panic("implement me")
}

func (d DeviceRepo) ListDevices(ctx context.Context) ([]domain.Device, error) {
	//TODO implement me
	panic("implement me")
}
