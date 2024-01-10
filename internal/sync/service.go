package sync

import (
	"context"
	"fmt"
	"github.com/SyncYomi/SyncYomi/internal/notification"
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
	GetSyncData(ctx context.Context, apiKey string) (*domain.SyncData, error)
	SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error)
	GetSyncLockFile(ctx context.Context, apiKey string) (*domain.SyncLockFile, error)
	CreateSyncLockFile(ctx context.Context, apiKey string, acquiredBy string) (*domain.SyncLockFile, error)
	UpdateSyncLockFile(ctx context.Context, syncLockFile *domain.SyncLockFile) (*domain.SyncLockFile, error)
	DeleteSyncLockFile(ctx context.Context, apiKey string) bool
}

func NewService(log logger.Logger, repo domain.SyncRepo, mdata mdata.Service, notificationSvc notification.Service, apiRepo domain.APIRepo) Service {
	return &service{
		log:                 log.With().Str("module", "sync").Logger(),
		repo:                repo,
		mdataSvc:            mdata,
		notificationService: notificationSvc,
		apiRepo:             apiRepo,
	}
}

type service struct {
	log                 zerolog.Logger
	repo                domain.SyncRepo
	mdataSvc            mdata.Service
	notificationService notification.Service
	apiRepo             domain.APIRepo
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

func (s service) GetSyncData(ctx context.Context, apiKey string) (*domain.SyncData, error) {
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
	user, err := s.apiRepo.Get(ctx, sync.Sync.UserApiKey.Key)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not get user by api key: %v", sync.Sync.UserApiKey.Key)
		return nil, err
	}

	err = s.updateSyncLockFile(ctx, domain.SyncStatusSyncing, sync.Sync.UserApiKey.Key)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update sync lock file")
		return nil, err
	}

	s.notifySyncStarted(user.Name)

	// Ensure sync record exists
	sData, err := s.ensureSyncRecordExists(ctx, sync)
	if err != nil {
		return nil, err
	}

	// Ensure manga data exists
	mData, err := s.ensureMangaDataExists(ctx, sync)
	if err != nil {
		s.notifySyncFailed(user.Name, err.Error())
		return nil, err
	}

	// Update the LastSynced field for the Sync record
	newSyncTime := time.Now().UTC()
	sData.LastSynced = &newSyncTime
	_, err = s.repo.Update(ctx, sync.Sync)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update sync: %+v", sync.Sync)
		s.notifySyncError(user.Name, err.Error())
		return nil, err
	}

	// Update the MangaData record
	_, err = s.mdataSvc.Update(ctx, sync.Data)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update manga data: %+v", mData)
		s.notifySyncError(user.Name, err.Error())
		return nil, err
	}

	err = s.updateSyncLockFile(ctx, domain.SyncStatusSuccess, sync.Sync.UserApiKey.Key)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update sync lock file")
		return nil, err
	}

	// Notify success
	s.notifySyncSuccess(user.Name)

	return &domain.SyncData{
		Sync: sData,
		Data: mData,
	}, nil
}

func (s service) GetSyncLockFile(ctx context.Context, apiKey string) (*domain.SyncLockFile, error) {
	lockFile, err := s.repo.GetSyncLockFile(ctx, apiKey)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not get sync lock file by api key: %v", apiKey)
		return nil, err
	}

	return lockFile, nil
}

func (s service) CreateSyncLockFile(ctx context.Context, apiKey string, acquiredBy string) (*domain.SyncLockFile, error) {
	lockFile, err := s.repo.CreateSyncLockFile(ctx, apiKey, acquiredBy)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not create sync lock file by api key: %v", apiKey)
		return nil, err
	}

	return lockFile, nil
}

func (s service) UpdateSyncLockFile(ctx context.Context, syncLockFile *domain.SyncLockFile) (*domain.SyncLockFile, error) {
	lockFile, err := s.repo.UpdateSyncLockFile(ctx, syncLockFile)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update sync lock file by api key: %v", syncLockFile.UserApiKey)
		return nil, err
	}

	return lockFile, nil
}

func (s service) DeleteSyncLockFile(ctx context.Context, apiKey string) bool {
	return s.repo.DeleteSyncLockFile(ctx, apiKey)
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

func (s service) notifySyncStarted(apiKeyName string) {
	s.notificationService.Send(domain.NotificationEventSyncStarted, domain.NotificationPayload{
		Subject: "Data Transmission Initiated",
		Message: fmt.Sprintf("A data transmission between your Tachiyomi library and user **%s** has been initiated. "+
			"Please wait for the process to complete.", apiKeyName),
	})
}

func (s service) notifySyncSuccess(apiKeyName string) {
	s.notificationService.Send(domain.NotificationEventSyncSuccess, domain.NotificationPayload{
		Subject: "Data Send Successful",
		Message: fmt.Sprintf("Your Tachiyomi library data has been successfully sent to user **%s**.", apiKeyName),
	})
}

func (s service) notifySyncFailed(apiKeyName string, errMsg string) {
	s.notificationService.Send(domain.NotificationEventSyncFailed, domain.NotificationPayload{
		Subject: "Sync Operation Failed",
		Message: fmt.Sprintf("The synchronization with Tachiyomi failed for user **%s**. Error: %s", apiKeyName, errMsg),
	})
}

func (s service) notifySyncError(apiKeyName string, errMsg string) {
	s.notificationService.Send(domain.NotificationEventSyncError, domain.NotificationPayload{
		Subject: "Error During Sync",
		Message: fmt.Sprintf("An error occurred during synchronization with Tachiyomi for user **%s**. Error: %s", apiKeyName, errMsg),
	})
}

func (s service) updateSyncLockFile(ctx context.Context, status domain.SyncStatus, apiKey string) error {
	now := time.Now().UTC()
	expiresAt := now.Add(time.Minute * 5)

	syncLockFile := &domain.SyncLockFile{
		UserApiKey: apiKey,
		LastSynced: &now,
		Status:     status,
		RetryCount: 0,
		AcquiredAt: &now,
		ExpiresAt:  &expiresAt,
		UpdatedAt:  &now,
	}

	_, err := s.UpdateSyncLockFile(ctx, syncLockFile)
	return err
}
