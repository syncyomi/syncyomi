package sync

import (
	"context"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/device"
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
	mData.Categories = s.syncCategories(sync.Data.Categories, mData.Categories)

	// Sync Manga and Chapters
	mData.Manga = s.syncMangaAndChapters(sync.Data.Manga, mData.Manga)

	// Update the LastSynced field for the Sync record
	newSyncTime := time.Now().UTC()
	sData.LastSynced = &newSyncTime
	sync.Sync.Device.ID = d.ID
	_, err = s.repo.Update(ctx, sData)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update sync: %+v", sData)
		return nil, err
	}

	// Update the MangaData record
	_, err = s.mdataSvc.Update(ctx, mData)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update manga data: %+v", mData)
		return nil, err
	}

	return &domain.SyncData{
		Device: d,
		Sync:   sData,
		Data:   mData,
	}, nil
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

func (s service) syncCategories(clientCategories, serverCategories []domain.Category) []domain.Category {
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
	return serverCategories
}

func (s service) syncMangaAndChapters(clientManga []domain.Manga, serverManga []domain.Manga) []domain.Manga {
	clientMangaMap := make(map[string]domain.Manga)
	for _, cm := range clientManga {
		clientMangaMap[cm.URL] = cm
	}

	for i := range serverManga {
		sm := &serverManga[i]
		cm, found := clientMangaMap[sm.URL]
		if found {
			// If the Manga exists on both server and client, sync data
			clientUnixTimestampSeconds := cm.LastModifiedAt
			clientLastModifiedAt := time.Unix(clientUnixTimestampSeconds, 0)
			serverUnixTimestampSeconds := sm.LastModifiedAt
			serverLastModifiedAt := time.Unix(serverUnixTimestampSeconds, 0)

			if clientLastModifiedAt.After(serverLastModifiedAt) {
				*sm = cm
			} else {
				clientMangaMap[sm.URL] = *sm
			}

			// Sync chapters
			for _, clientChapter := range cm.Chapters {
				serverChapter := findChapter(clientChapter.URL, clientChapter.ChapterNumber, sm.Chapters)
				if serverChapter == nil {
					// If the Chapter does not exist on the server, add it
					sm.Chapters = append(sm.Chapters, clientChapter)
				} else {
					// Compare Chapter's lastModifiedAt timestamps and update if needed
					clientUnixTimestampSeconds := clientChapter.LastModifiedAt
					clientChapterLastModifiedAt := time.Unix(clientUnixTimestampSeconds, 0)
					serverUnixTimestampSeconds := serverChapter.LastModifiedAt
					serverChapterLastModifiedAt := time.Unix(serverUnixTimestampSeconds, 0)

					if clientChapterLastModifiedAt.After(serverChapterLastModifiedAt) {
						*serverChapter = clientChapter
					}
				}
			}
		} else {
			// If the Manga exists on the server but not on the client, add it to the client
			clientMangaMap[sm.URL] = *sm
		}
	}

	// Sync manga that only exists on the client
	for _, cm := range clientMangaMap {
		sm := findMangaByURL(cm.URL, serverManga)
		if sm == nil {
			serverManga = append(serverManga, cm)
		}
	}

	// Ensure the clientMangaMap is identical to serverManga
	clientMangaMap = make(map[string]domain.Manga)
	for _, sm := range serverManga {
		clientMangaMap[sm.URL] = sm
	}
	// Convert the clientMangaMap back to a slice
	syncedManga := make([]domain.Manga, 0, len(clientMangaMap))
	for _, cm := range clientMangaMap {
		syncedManga = append(syncedManga, cm)
	}

	return syncedManga
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

func findChapter(url string, chapterNumber int, chapters []domain.Chapter) *domain.Chapter {
	for i := range chapters {
		if chapters[i].URL == url && chapters[i].ChapterNumber == chapterNumber {
			return &chapters[i]
		}
	}
	return nil
}
