package device

import (
	"context"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/logger"
	"github.com/rs/zerolog"
)

type Service interface {
	Store(ctx context.Context, device *domain.Device) (*domain.Device, error)
	Delete(ctx context.Context, id int) error
	ListDevices(ctx context.Context, apikey string) ([]domain.Device, error)
	GetDeviceByDeviceId(ctx context.Context, device *domain.Device) (*domain.Device, error)
	GetDeviceByApiKey(ctx context.Context, device *domain.Device) (*domain.Device, error)
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

func (s service) Store(ctx context.Context, device *domain.Device) (*domain.Device, error) {
	d, err := s.repo.Store(ctx, device)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not store device: %+v", device)
		return nil, err
	}

	return d, nil
}

func (s service) Delete(ctx context.Context, id int) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not delete device with id: %v", id)
		return err
	}

	return nil
}

func (s service) ListDevices(ctx context.Context, apikey string) ([]domain.Device, error) {
	devices, err := s.repo.ListDevices(ctx, apikey)
	if err != nil {
		s.log.Error().Err(err).Msg("could not list devices")
		return nil, err
	}

	return devices, nil
}

func (s service) GetDeviceByDeviceId(ctx context.Context, device *domain.Device) (*domain.Device, error) {
	d, err := s.repo.GetDeviceByDeviceId(ctx, device)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not get device with id: %v", device.ID)
		return nil, err
	}

	return d, nil
}

func (s service) GetDeviceByApiKey(ctx context.Context, device *domain.Device) (*domain.Device, error) {
	d, err := s.repo.GetDeviceByApiKey(ctx, device)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not get device with apikey: %v", device.UserApiKey.Key)
		return nil, err
	}

	return d, nil
}
