package sync

import (
	"context"
	"github.com/SyncYomi/SyncYomi/internal/device"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/mdata"
	"github.com/rs/zerolog"
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
	// Ensure device exists
	d, err := s.ensureDeviceExists(ctx, sync)
	if err != nil {
		return nil, err
	}

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

	// Granular sync

	// Sync Categories
	s.syncCategories(sync.Data.Categories, mData.Categories)

	// Sync Manga and Chapters
	s.syncMangaAndChapters(sync.Data.Manga, mData.Manga)

	// Compare sync times and update data accordingly
	result, err := s.compareSyncTimesAndUpdate(ctx, sync, d, sData, mData)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s service) ensureDeviceExists(ctx context.Context, sync *domain.SyncData) (*domain.Device, error) {
	d, err := s.deviceService.GetDeviceByDeviceId(ctx, sync.Device)
	if err != nil {
		if err.Error() == "error executing query: sql: no rows in result set" {
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

	return d, nil
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

func (s service) syncCategories(clientCategories, serverCategories []domain.Category) {
	for _, clientCategory := range clientCategories {
		serverCategory := findCategoryByName(clientCategory.Name, serverCategories)
		if serverCategory == nil {
			// If the category does not exist on the server, add it
			serverCategories = append(serverCategories, clientCategory)
		} else {
			// Directly update the server category with the client category data
			*serverCategory = clientCategory
		}
	}
}

func (s service) syncMangaAndChapters(clientManga []domain.Manga, serverManga []domain.Manga) {
	for _, cm := range clientManga {
		sm := findMangaByURL(cm.URL, serverManga)
		if sm == nil {
			// If the Manga does not exist on the server, add it
			serverManga = append(serverManga, cm)
			continue
		}

		// Compare Manga's lastModifiedAt timestamps and update if needed
		if cm.LastModifiedAt > sm.LastModifiedAt {
			*sm = cm
		}

		// Sync chapters
		for _, clientChapter := range cm.Chapters {
			serverChapter := findChapterByID(clientChapter.Id, sm.Chapters)
			if serverChapter == nil {
				// If the Chapter does not exist on the server, add it
				sm.Chapters = append(sm.Chapters, clientChapter)
				continue
			}

			// Compare Chapter's lastModifiedAt timestamps and update if needed
			if clientChapter.LastModifiedAt > serverChapter.LastModifiedAt {
				*serverChapter = clientChapter
			}
		}
	}
}

func (s service) compareSyncTimesAndUpdate(ctx context.Context, sync *domain.SyncData, d *domain.Device, sData *domain.Sync, mData *domain.MangaData) (*domain.SyncData, error) {
	epochTime := time.Unix(0, sync.Sync.LastSyncedEpoch*int64(time.Millisecond)).UTC()
	utcLastSynced := sData.LastSynced.UTC()
	utcEpochTime := epochTime.UTC()
	if utcEpochTime.After(utcLastSynced) {
		s.log.Info().Msgf("user's last local changes is newer: %v than ours: %v, therefore our data is old and should be updated for next time", utcEpochTime, utcLastSynced)
		newSyncTime := time.Now().UTC()
		sData.LastSynced = &newSyncTime
		sync.Sync.Device.ID = d.ID
		_, err := s.mdataSvc.Update(ctx, sync.Data)
		if err != nil {
			s.log.Error().Err(err).Msgf("could not update manga data: %+v", sync.Data)
			return nil, err
		}

		sync.Sync.Device.ID = d.ID
		_, err = s.repo.Update(ctx, sync.Sync)
		if err != nil {
			s.log.Error().Err(err).Msgf("could not update sync: %+v", sync.Sync)
			return nil, err
		}

		return &domain.SyncData{
			UpdateRequired: false,
			Device:         d,
		}, nil
	} else {
		s.log.Info().Msgf("user's last local changes is old: %v than our last sync at: %v, therefore their data is outdated updating sync!", utcEpochTime, utcLastSynced)
		newSyncTime := time.Now().UTC()
		sData.LastSynced = &newSyncTime
		sync.Sync.Device.ID = d.ID
		sData, err := s.repo.Update(ctx, sync.Sync)
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

func findCategoryByName(name string, categories []domain.Category) *domain.Category {
	for i := range categories {
		if categories[i].Name == name {
			return &categories[i]
		}
	}
	return nil
}

func findMangaByURL(url string, mangas []domain.Manga) *domain.Manga {
	for i := range mangas {
		if mangas[i].URL == url {
			return &mangas[i]
		}
	}
	return nil
}

func findChapterByID(id int64, chapters []domain.Chapter) *domain.Chapter {
	for i := range chapters {
		if chapters[i].Id == id {
			return &chapters[i]
		}
	}
	return nil
}
