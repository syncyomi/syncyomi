package database

import (
	"context"
	"encoding/json"
	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/rs/zerolog"
	"time"
)

func NewMangaDataRepo(log logger.Logger, db *DB) domain.MangaDataRepo {
	return &MangaRepo{
		log: log.With().Str("module", "manga").Logger(),
		db:  db,
	}
}

type MangaRepo struct {
	log zerolog.Logger
	db  *DB
}

func (m MangaRepo) Store(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error) {
	// Marshal the entire mdata object
	jsonData, err := json.Marshal(mdata)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling data to JSON")
	}

	queryBuilder := m.db.squirrel.
		Insert("manga_data").
		Columns(
			"user_api_key",
			"data",
		).
		Values(
			mdata.UserApiKey.Key,
			jsonData,
		).
		Suffix("RETURNING id, created_at, updated_at").RunWith(m.db.handler)

	var id int
	var createdAt time.Time
	var updatedAt time.Time

	if err := queryBuilder.QueryRowContext(ctx).Scan(&id, &createdAt, &updatedAt); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}
	mdata.ID = id
	mdata.CreatedAt = createdAt
	mdata.UpdatedAt = updatedAt

	return mdata, nil
}

func (m MangaRepo) Delete(ctx context.Context, id int) error {
	queryBuilder := m.db.squirrel.
		Delete("manga_data").
		Where(sq.Eq{"id": id}).
		RunWith(m.db.handler)

	if _, err := queryBuilder.ExecContext(ctx); err != nil {
		return errors.Wrap(err, "error executing query")
	}

	return nil
}

func (m MangaRepo) Update(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error) {
	jsonData, err := json.Marshal(mdata)
	if err != nil {
		return nil, errors.Wrap(err, "error marshaling data to JSON")
	}

	queryBuilder := m.db.squirrel.
		Update("manga_data").
		Set("data", jsonData).
		Set("updated_at", time.Now()).
		Where(sq.Eq{"user_api_key": mdata.UserApiKey.Key}).
		Suffix("RETURNING updated_at").RunWith(m.db.handler)

	var updatedAt time.Time
	if err := queryBuilder.QueryRowContext(ctx).Scan(&updatedAt); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	mdata.UpdatedAt = updatedAt

	return mdata, nil
}

func (m MangaRepo) ListMangaData(ctx context.Context, apiKey string) ([]domain.BackupData, error) {
	queryBuilder := m.db.squirrel.
		Select(
			"id",
			"user_api_key",
			"data",
			"created_at",
			"updated_at",
		).
		From("manga_data").
		RunWith(m.db.handler)

	if apiKey != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"user_api_key": apiKey})
	}

	rows, err := queryBuilder.QueryContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	mangaData := make([]domain.BackupData, 0)
	for rows.Next() {
		var mdata domain.BackupData
		var jsonData []byte // Use a byte slice to store the JSON data from the database

		if err := rows.Scan(
			&mdata.ID,
			&mdata.UserApiKey.Key,
			&jsonData, // Scan the JSON data into the byte slice
			&mdata.CreatedAt,
			&mdata.UpdatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "error scanning row")
		}

		// Convert the JSON data back into a Go struct
		if err := json.Unmarshal(jsonData, &mdata); err != nil {
			return nil, errors.Wrap(err, "error unmarshaling JSON data")
		}

		mangaData = append(mangaData, mdata)
	}

	return mangaData, nil
}

func (m MangaRepo) GetMangaDataByApiKey(ctx context.Context, apiKey string) (*domain.BackupData, error) {
	queryBuilder := m.db.squirrel.
		Select(
			"id",
			"user_api_key",
			"data",
			"created_at",
			"updated_at",
		).
		From("manga_data").
		Where(sq.Eq{"user_api_key": apiKey}).
		RunWith(m.db.handler)

	var mdata domain.BackupData
	mdata.UserApiKey = &domain.APIKey{}
	var jsonData []byte // Use a byte slice to store the JSON data from the database

	if err := queryBuilder.QueryRowContext(ctx).Scan(
		&mdata.ID,
		&mdata.UserApiKey.Key,
		&jsonData, // Scan the JSON data into the byte slice
		&mdata.CreatedAt,
		&mdata.UpdatedAt,
	); err != nil {
		return &domain.BackupData{}, errors.Wrap(err, "error executing query")
	}

	// Convert the JSON data back into a Go struct
	if err := json.Unmarshal(jsonData, &mdata); err != nil {
		return nil, errors.Wrap(err, "error unmarshaling JSON data")
	}

	return &mdata, nil
}
