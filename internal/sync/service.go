package sync

import (
	"context"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/device"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/logger"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/mdata"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"time"
)

type Service interface {
	Store(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	ListSyncs(ctx context.Context, apiKey string) ([]domain.Sync, error)
	GetSyncByApiKey(ctx context.Context, apiKey string) (*domain.Sync, error)
	GetSyncByDeviceID(ctx context.Context, deviceID int) (*domain.Sync, error)
	SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error)
}

func NewService(log logger.Logger, repo domain.SyncRepo, mdata mdata.Service, device device.Service) Service {
	return &service{
		log:           log.With().Str("module", "sync").Logger(),
		repo:          repo,
		mdataSvc:      mdata,
		deviceService: device,
	}
}

type service struct {
	log           zerolog.Logger
	repo          domain.SyncRepo
	mdataSvc      mdata.Service
	deviceService device.Service
}

func (s service) Store(ctx context.Context, sync *domain.Sync) (*domain.Sync, error) {
	msync, err := s.repo.Store(ctx, sync)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not store sync: %+v", sync)
		return nil, err
	}

	return msync, nil
}

func (s service) Delete(ctx context.Context, id int) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not delete sync with id: %v", id)
		return err
	}

	return nil
}

func (s service) Update(ctx context.Context, sync *domain.Sync) (*domain.Sync, error) {
	msync, err := s.repo.Update(ctx, sync)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update sync: %+v", sync)
		return nil, err
	}

	return msync, nil
}

func (s service) ListSyncs(ctx context.Context, apiKey string) ([]domain.Sync, error) {
	syncs, err := s.repo.ListSyncs(ctx, apiKey)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not list syncs")
		return nil, err
	}

	return syncs, nil
}

func (s service) GetSyncByApiKey(ctx context.Context, apiKey string) (*domain.Sync, error) {
	msync, err := s.repo.GetSyncByApiKey(ctx, apiKey)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not get sync by api key: %v", apiKey)
		return nil, err
	}

	return msync, nil
}

func (s service) GetSyncByDeviceID(ctx context.Context, deviceID int) (*domain.Sync, error) {
	msync, err := s.repo.GetSyncByDeviceID(ctx, deviceID)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not get sync by device id: %v", deviceID)
		return nil, err
	}

	return msync, nil
}

func (s service) SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error) {
	// Check if the device exists in the database
	d, err := s.deviceService.GetDeviceByDeviceId(ctx, sync.Device)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
			// If the device does not exist, it might be the first time syncing, so create the device
			s.log.Info().Msgf("device does not exist maybe it's first time, creating it")
			d, err = s.deviceService.Store(ctx, sync.Device)
			if err != nil {
				s.log.Error().Err(err).Msgf("could not store device: %+v", sync.Device)
				return nil, err
			}
		} else {
			s.log.Error().Err(err).Msgf("could not get device: %+v", sync.Device)
			return nil, err
		}
	}

	// Check if a sync record exists for the user
	sData, err := s.repo.GetSyncByApiKey(ctx, sync.Sync.UserApiKey.Key)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
			// If a sync record does not exist, it might be the first time syncing, so create the sync record
			s.log.Info().Msgf("sync does not exist maybe it's first time, creating it")
			now := time.Now().UTC()
			if sync.Sync.LastSynced == nil {
				sync.Sync.LastSynced = &now
			}
			if sync.Sync.Device.ID == 0 {
				sync.Sync.Device.ID = sync.Device.ID
			}
			sData, err = s.repo.Store(ctx, sync.Sync)
			if err != nil {
				s.log.Error().Err(err).Msgf("could not store sync: %+v", sync.Sync)
				return nil, err
			}
		} else {
			s.log.Error().Err(err).Msgf("could not get sync by api key: %v", sync.Sync.UserApiKey.Key)
			return nil, err
		}
	}

	// Check if manga data exists for the user
	mData, err := s.mdataSvc.GetMangaDataByApiKey(ctx, sync.Sync.UserApiKey.Key)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
			// If manga data does not exist, it might be the first time syncing, so create the manga data
			s.log.Info().Msgf("manga data does not exist maybe it's first time, creating it")
			mData, err = s.mdataSvc.Store(ctx, sync.Data)
			if err != nil {
				s.log.Error().Err(err).Msgf("could not store manga data: %+v", sync.Data)
				return nil, err
			}
		} else {
			s.log.Error().Err(err).Msgf("could not get manga data by api key: %v", sync.Sync.UserApiKey.Key)
			return nil, err
		}
	}

	// Compare the user's last sync time with the current sync data
	// Convert time to UTC for comparison
	epochTime := time.Unix(0, sync.Sync.LastSyncedEpoch*int64(time.Millisecond)).UTC()
	utcLastSynced := sData.LastSynced.UTC()
	utcEpochTime := epochTime.UTC()
	log.Info().Msgf("last synced: %v", utcLastSynced)
	log.Info().Msgf("epoch time: %v", utcEpochTime)
	if utcEpochTime.After(utcLastSynced) {
		// If the user's last sync is newer than the current sync data, update the server's data
		s.log.Info().Msgf("user's last local changes is newer: %v than ours: %v, therefore our data is old and should be updated for next time", utcEpochTime, utcLastSynced)
		// update the last device id it was synced with
		sync.Sync.Device.ID = d.ID
		mData, err = s.mdataSvc.Update(ctx, sync.Data)
		if err != nil {
			s.log.Error().Err(err).Msgf("could not update manga data: %+v", sync.Data)
			return nil, err
		}

		return &domain.SyncData{
			UpdateRequired: false,
		}, nil
	} else {
		// if user's last local changes is older than current sync data, update sync data (send back to client)
		s.log.Info().Msgf("user's last local changes is old: %v than our last sync at: %v, therefore their data is outdated updating sync!", utcEpochTime, utcLastSynced)
		// set the last synced time immediately
		newSyncTime := time.Now().UTC()
		sData.LastSynced = &newSyncTime
		// update the last device id it was synced with
		sync.Sync.Device.ID = d.ID
		sData, err = s.repo.Update(ctx, sync.Sync)
		if err != nil {
			s.log.Error().Err(err).Msgf("could not update sync: %+v", sync.Sync)
			return nil, err
		}

		return &domain.SyncData{
			UpdateRequired: true,
			Sync:           sData,
			Data:           mData,
			Device:         d,
		}, nil
	}
}
