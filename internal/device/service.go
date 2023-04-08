package device

import (
	"context"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/logger"
	"github.com/rs/zerolog"
)

type Service interface {
	Store(ctx context.Context, device *domain.Device) error
	Delete(ctx context.Context, id int) error
	ListDevices(ctx context.Context) ([]domain.Device, error)
}

func NewService(log logger.Logger, repo domain.DeviceRepo) Service {
	return &service{
		log:  log.With().Str("module", "device").Logger(),
		repo: repo,
	}
}

type service struct {
	log  zerolog.Logger
	repo domain.DeviceRepo
}

func (s service) Store(ctx context.Context, device *domain.Device) error {
	//TODO implement me
	panic("implement me")
}

func (s service) Delete(ctx context.Context, id int) error {
	//TODO implement me
	panic("implement me")
}

func (s service) ListDevices(ctx context.Context) ([]domain.Device, error) {
	//TODO implement me
	panic("implement me")
}
