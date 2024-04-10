package database

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

func NewSyncRepo(log logger.Logger, db *DB) domain.SyncRepo {
	return &SyncRepo{
		log: log.With().Str("module", "device").Logger(),
		db:  db,
	}
}

type SyncRepo struct {
	log zerolog.Logger
	db  *DB
}

func (r SyncRepo) Store(ctx context.Context, sync *domain.Sync) (*domain.Sync, error) {
	// Check if LastSynced is nil and set it to current time if it is
	if sync.LastSynced == nil {
		now := time.Now()
		sync.LastSynced = &now
	}

	sync.Status = domain.SyncStatusSuccess

	queryBuilder := r.db.squirrel.
		Insert("manga_sync").
		Columns(
			"user_api_key",
			"last_sync",
			"status",
			"device_id",
		).
		Values(
			sync.UserApiKey.Key,
			sync.LastSynced,
			sync.Status,
			sync.DeviceId,
		).
		Suffix("RETURNING id, created_at, updated_at").RunWith(r.db.handler)

	var id int
	var createdAt time.Time
	var updatedAt time.Time

	if err := queryBuilder.QueryRowContext(ctx).Scan(&id, &createdAt, &updatedAt); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	sync.ID = id
	sync.CreatedAt = &createdAt
	sync.UpdatedAt = &updatedAt

	return sync, nil
}

func (r SyncRepo) Delete(ctx context.Context, id int) error {
	queryBuilder := r.db.squirrel.
		Delete("manga_sync").
		Where(sq.Eq{"id": id})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return errors.Wrap(err, "error building query")
	}
	_, err = r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "error executing query")
	}

	r.db.log.Debug().Msgf("BackupManga sync deleted: %d", id)

	return nil
}

func (r SyncRepo) Update(ctx context.Context, sync *domain.Sync) (*domain.Sync, error) {
	// Check if LastSynced is nil and set it to current time if it is
	if sync.LastSynced == nil {
		now := time.Now()
		sync.LastSynced = &now
	}

	sync.Status = domain.SyncStatusSuccess

	queryBuilder := r.db.squirrel.
		Update("manga_sync").
		Set("last_sync", sync.LastSynced).
		Set("device_id", sync.DeviceId).
		Set("status", sync.Status).
		Where(sq.Eq{"user_api_key": sync.UserApiKey.Key}).
		Suffix("RETURNING updated_at").RunWith(r.db.handler)

	var updatedAt time.Time

	if err := queryBuilder.QueryRowContext(ctx).Scan(&updatedAt); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	sync.UpdatedAt = &updatedAt

	return sync, nil
}

func (r SyncRepo) ListSyncs(ctx context.Context, apiKey string) ([]domain.Sync, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"manga_sync.id",
			"manga_sync.user_api_key",
			"manga_sync.last_sync",
			"manga_sync.status",
			"manga_sync.device_id",
			"manga_sync.created_at",
			"manga_sync.updated_at",
			"api_key.name",
			"api_key.scopes",
			"api_key.created_at",
		).
		From("manga_sync").
		InnerJoin("api_key ON api_key.key = manga_sync.user_api_key")

	if apiKey != "" {
		queryBuilder = queryBuilder.Where(sq.Eq{"api_key.key": apiKey})
	}

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	rows, err := r.db.handler.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			r.db.log.Error().Msgf("error closing rows: %v", err)
		}
	}(rows)

	mangaSyncs := make([]domain.Sync, 0)
	for rows.Next() {
		var mangaSync domain.Sync
		mangaSync.UserApiKey = &domain.APIKey{}

		if err := rows.Scan(
			&mangaSync.ID,
			&mangaSync.UserApiKey.Key,
			&mangaSync.LastSynced,
			&mangaSync.Status,
			&mangaSync.DeviceId,
			&mangaSync.CreatedAt,
			&mangaSync.UpdatedAt,
			&mangaSync.UserApiKey.Name,
			pq.Array(&mangaSync.UserApiKey.Scopes),
			&mangaSync.UserApiKey.CreatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "error scanning row")
		}

		mangaSyncs = append(mangaSyncs, mangaSync)
	}

	return mangaSyncs, nil
}

func (r SyncRepo) GetSyncByApiKey(ctx context.Context, apiKey string) (*domain.Sync, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"manga_sync.id",
			"manga_sync.user_api_key",
			"manga_sync.last_sync",
			"manga_sync.status",
			"manga_sync.device_id",
			"manga_sync.created_at",
			"manga_sync.updated_at",
			"api_key.name",
			"api_key.scopes",
			"api_key.created_at",
		).
		From("manga_sync").
		Join("api_key ON api_key.key = manga_sync.user_api_key").
		Where(sq.Eq{"api_key.key": apiKey})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	var mangaSync domain.Sync
	// initialize device and api key
	mangaSync.UserApiKey = &domain.APIKey{}

	if err := r.db.handler.QueryRowContext(ctx, query, args...).Scan(
		&mangaSync.ID,
		&mangaSync.UserApiKey.Key,
		&mangaSync.LastSynced,
		&mangaSync.Status,
		&mangaSync.DeviceId,
		&mangaSync.CreatedAt,
		&mangaSync.UpdatedAt,
		&mangaSync.UserApiKey.Name,
		pq.Array(&mangaSync.UserApiKey.Scopes),
		&mangaSync.UserApiKey.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.Sync{}, errors.Wrap(err, "error executing query")
		}
		return nil, errors.Wrap(err, "error executing query")
	}

	return &mangaSync, nil
}

func (r SyncRepo) GetSyncLockFile(ctx context.Context, apiKey string) (*domain.SyncLockFile, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"id",
			"user_api_key",
			"acquired_by",
			"last_sync",
			"status",
			"retry_count",
			"acquired_at",
			"expires_at",
		).
		From("sync_lock").
		Where(sq.Eq{"user_api_key": apiKey})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	var syncLockFile domain.SyncLockFile

	if err := r.db.handler.QueryRowContext(ctx, query, args...).Scan(
		&syncLockFile.ID,
		&syncLockFile.UserApiKey,
		&syncLockFile.AcquiredBy,
		&syncLockFile.LastSynced,
		&syncLockFile.Status,
		&syncLockFile.RetryCount,
		&syncLockFile.AcquiredAt,
		&syncLockFile.ExpiresAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &domain.SyncLockFile{}, errors.Wrap(err, "error executing query")
		}
		return nil, errors.Wrap(err, "error executing query")
	}

	return &syncLockFile, nil
}

func (r SyncRepo) CreateSyncLockFile(ctx context.Context, apiKey string, acquiredBy string) (*domain.SyncLockFile, error) {
	queryBuilder := r.db.squirrel.
		Insert("sync_lock").
		Columns(
			"user_api_key",
			"acquired_by",
			"last_sync",
			"status",
			"retry_count",
			"acquired_at",
			"expires_at",
		).
		Values(
			apiKey,
			acquiredBy,
			time.Now(),
			domain.SyncStatusSuccess,
			0,
			time.Now(),
			time.Now().Add(time.Minute*5),
		).
		Suffix("RETURNING id, created_at, updated_at").RunWith(r.db.handler)

	var id int
	var createdAt time.Time
	var updatedAt time.Time

	if err := queryBuilder.QueryRowContext(ctx).Scan(&id, &createdAt, &updatedAt); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	syncLockFile := &domain.SyncLockFile{
		ID:         id,
		UserApiKey: apiKey,
		AcquiredBy: acquiredBy,
		LastSynced: &createdAt,
		Status:     domain.SyncStatusPending,
		RetryCount: 0,
		AcquiredAt: &createdAt,
		ExpiresAt:  &updatedAt,
	}

	return syncLockFile, nil
}

func (r SyncRepo) UpdateSyncLockFile(ctx context.Context, syncLockFile *domain.SyncLockFile) (*domain.SyncLockFile, error) {
	// Start building the query.
	queryBuilder := r.db.squirrel.
		Update("sync_lock").
		Where(sq.Eq{"user_api_key": syncLockFile.UserApiKey})

	// Dynamically add fields that are present.
	if syncLockFile.AcquiredBy != "" {
		queryBuilder = queryBuilder.Set("acquired_by", syncLockFile.AcquiredBy)
	}
	if syncLockFile.LastSynced != nil {
		queryBuilder = queryBuilder.Set("last_sync", syncLockFile.LastSynced)
	}
	if syncLockFile.Status != "" {
		queryBuilder = queryBuilder.Set("status", syncLockFile.Status)
	}
	if syncLockFile.RetryCount != 0 {
		queryBuilder = queryBuilder.Set("retry_count", syncLockFile.RetryCount)
	}
	if syncLockFile.AcquiredAt != nil {
		queryBuilder = queryBuilder.Set("acquired_at", syncLockFile.AcquiredAt)
	}
	if syncLockFile.ExpiresAt != nil {
		queryBuilder = queryBuilder.Set("expires_at", syncLockFile.ExpiresAt)
	}

	queryBuilder = queryBuilder.Suffix("RETURNING updated_at").RunWith(r.db.handler)

	var updatedAt time.Time
	if err := queryBuilder.QueryRowContext(ctx).Scan(&updatedAt); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	syncLockFile.UpdatedAt = &updatedAt

	return syncLockFile, nil
}

func (r SyncRepo) DeleteSyncLockFile(ctx context.Context, apiKey string) bool {
	queryBuilder := r.db.squirrel.
		Delete("sync_lock").
		Where(sq.Eq{"user_api_key": apiKey})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		r.db.log.Error().Err(err).Msgf("error building query")
		return false
	}

	_, err = r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		r.db.log.Error().Err(err).Msgf("error executing query")
		return false
	}

	r.db.log.Debug().Msgf("Sync lock file deleted: %v", apiKey)

	return true
}

// Get etag of sync data.
// For avoid memory usage, only the etag will be returned.
func (r SyncRepo) GetSyncDataETag(ctx context.Context, apiKey string) (*string, error) {
	var etag string

	err := r.db.squirrel.
		Select("data_etag").
		From("sync_data").
		Where(sq.Eq{"user_api_key": apiKey}).
		Limit(1).
		RunWith(r.db.handler).
		Scan(&etag)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, errors.Wrap(err, "error executing query")
	}

	return &etag, nil
}

// Get sync data and etag
func (r SyncRepo) GetSyncDataAndETag(ctx context.Context, apiKey string) ([]byte, *string, error) {
	var etag string
	var data []byte

	err := r.db.squirrel.
		Select("data", "data_etag").
		From("sync_data").
		Where(sq.Eq{"user_api_key": apiKey}).
		Limit(1).
		RunWith(r.db.handler).
		Scan(&data, &etag)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, errors.Wrap(err, "error executing query")
	}

	return data, &etag, nil
}

// Create or replace sync data, returns the new etag.
func (r SyncRepo) SetSyncData(ctx context.Context, apiKey string, data []byte) (*string, error) {
	now := time.Now()
	// the better way is use hash like sha1
	// but uuid is faster than sha1
	newEtag := "uuid=" + uuid.NewString()

	updateResult, err := r.db.squirrel.
		Update("sync_data").
		Set("updated_at", now).
		Set("data", data).
		Set("data_etag", newEtag).
		Where(sq.Eq{"user_api_key": apiKey}).
		RunWith(r.db.handler).ExecContext(ctx)

	if err != nil {
		r.log.Err(err).Msgf("Error when updating sync data")
		return nil, errors.Wrap(err, "error executing query")
	}

	if rowsAffected, err := updateResult.RowsAffected(); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	} else if rowsAffected == 0 {
		// new item
		insertResult, err := r.db.squirrel.
			Insert("sync_data").
			Columns(
				"user_api_key",
				"updated_at",
				"data",
				"data_etag",
			).
			Values(apiKey, now, data, newEtag).
			RunWith(r.db.handler).ExecContext(ctx)

		if err != nil {
			r.log.Err(err).Msgf("Error when inserting sync data")
			return nil, errors.Wrap(err, "error executing query")
		}

		if rowsAffected, err := insertResult.RowsAffected(); err != nil {
			return nil, errors.Wrap(err, "error executing query")

		} else if rowsAffected == 0 {
			// multi devices race condition
			return nil, errors.New("no rows affected")
		}
	}

	r.log.Debug().Msgf("Sync data upsert: api_key=\"%v\"", apiKey)
	return &newEtag, nil
}

// Replace sync data only if the etag matches,
// returns the new etag if updated, or nil if not.
func (r SyncRepo) SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error) {
	now := time.Now()
	// the better way is use hash like sha1
	// but uuid is faster than sha1
	newEtag := "uuid=" + uuid.NewString()

	result, err := r.db.squirrel.
		Update("sync_data").
		Set("updated_at", now).
		Set("data", data).
		Set("data_etag", newEtag).
		Where(sq.Eq{"user_api_key": apiKey}).
		Where(sq.Eq{"data_etag": etag}).
		RunWith(r.db.handler).ExecContext(ctx)

	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	if rowsAffected, err := result.RowsAffected(); err != nil {
		return nil, errors.Wrap(err, "error executing query")

	} else if rowsAffected == 0 {
		r.log.Debug().Msgf("ETag mismatch for api_key=\"%v\". Concurrent modification detected, aborting sync to prevent data overwrite. ETag=\"%v\"", apiKey, etag)
		return nil, nil

	} else {
		r.log.Debug().Msgf("Sync data replaced: api_key=\"%v\", etag=\"%v\"", apiKey, etag)
		return &newEtag, nil
	}
}
