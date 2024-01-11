package mdata

import (
	"context"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/rs/zerolog"
)

type Service interface {
	Store(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error)
	ListMangaData(ctx context.Context, apiKey string) ([]domain.BackupData, error)
	GetMangaDataByApiKey(ctx context.Context, apiKey string) (*domain.BackupData, error)
}

func NewService(log logger.Logger, repo domain.MangaDataRepo) Service {
	return &service{
		log:  log.With().Str("module", "mdata").Logger(),
		repo: repo,
	}
}

type service struct {
	log  zerolog.Logger
	repo domain.MangaDataRepo
}

func (s service) Store(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error) {
	data, err := s.repo.Store(ctx, mdata)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not store manga data: %+v", mdata)
		return nil, err
	}

	return data, nil
}

func (s service) Delete(ctx context.Context, id int) error {
	err := s.repo.Delete(ctx, id)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not delete manga data with id: %v", id)
		return err
	}

	return nil
}

func (s service) Update(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error) {
	data, err := s.repo.Update(ctx, mdata)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not update manga data: %+v", mdata)
		return nil, err
	}

	return data, nil
}

func (s service) ListMangaData(ctx context.Context, apiKey string) ([]domain.BackupData, error) {
	data, err := s.repo.ListMangaData(ctx, apiKey)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not list manga data for api key: %v", apiKey)
		return nil, err
	}

	return data, nil
}

func (s service) GetMangaDataByApiKey(ctx context.Context, apiKey string) (*domain.BackupData, error) {
	data, err := s.repo.GetMangaDataByApiKey(ctx, apiKey)
	if err != nil {
		s.log.Error().Err(err).Msgf("could not get manga data for api key: %v", apiKey)
		return nil, err
	}

	return data, nil
}
