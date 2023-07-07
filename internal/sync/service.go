package sync

import (
	"context"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/mdata"
	"github.com/rs/zerolog"
)

type Service interface {
	Store(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	ListSyncs(ctx context.Context, apiKey string) ([]domain.Sync, error)
	GetSyncByApiKey(ctx context.Context, apiKey string) (*domain.Sync, error)
	GetSyncData(ctx context.Context, apiKey string, deviceID int) (*domain.SyncData, error)
	SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error)
}

func NewService(log logger.Logger, repo domain.SyncRepo, mdata mdata.Service) Service {
	return &service{
		log:      log.With().Str("module", "sync").Logger(),
		repo:     repo,
		mdataSvc: mdata,
	}
}

type service struct {
	log      zerolog.Logger
	repo     domain.SyncRepo
	mdataSvc mdata.Service
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

func (s service) GetSyncData(ctx context.Context, apiKey string, deviceID int) (*domain.SyncData, error) {
	sData, err := s.repo.GetSyncByApiKey(ctx, apiKey)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
			sData = nil
		}
	}

	mData, err := s.mdataSvc.GetMangaDataByApiKey(ctx, apiKey)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
			mData = nil
		}
	}

	return &domain.SyncData{
		Sync: sData,
		Data: mData,
	}, nil
}

func (s service) SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error) {
	// Ensure sync record exists
	sData, err := s.ensureSyncRecordExists(ctx, sync)
	if err != nil {
		return nil, err
	}

	// Ensure manga data exists
	mData, err := s.ensureMangaDataExists(ctx, sync)
	if err != nil {
		return nil, err
	}

	// Update the LastSynced field for the Sync record
	newSyncTime := time.Now().UTC()
	sData.LastSynced = &newSyncTime
	_, err = s.repo.Update(ctx, sync.Sync)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update sync: %+v", sync.Sync)
		return nil, err
	}

	// Update the MangaData record
	_, err = s.mdataSvc.Update(ctx, sync.Data)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update manga data: %+v", mData)
		return nil, err
	}

	return &domain.SyncData{
		Sync: sData,
		Data: mData,
	}, nil
}

func (s service) ensureSyncRecordExists(ctx context.Context, sync *domain.SyncData) (*domain.Sync, error) {
	sData, err := s.repo.GetSyncByApiKey(ctx, sync.Sync.UserApiKey.Key)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
			s.log.Info().Msgf("sync does not exist maybe it's first time, creating it")
			now := time.Now().UTC()
			if sync.Sync.LastSynced == nil {
				sync.Sync.LastSynced = &now
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

	return sData, nil
}

func (s service) ensureMangaDataExists(ctx context.Context, sync *domain.SyncData) (*domain.MangaData, error) {
	mData, err := s.mdataSvc.GetMangaDataByApiKey(ctx, sync.Sync.UserApiKey.Key)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
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

	return mData, nil
}
