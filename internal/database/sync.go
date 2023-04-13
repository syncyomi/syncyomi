package database

import (
	"context"
	"database/sql"
	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
	"time"
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
	queryBuilder := r.db.squirrel.
		Insert("manga_sync").
		Columns(
			"user_api_key",
			"device_id",
			"last_sync",
			"status",
		).
		Values(
			sync.UserApiKey.Key,
			sync.Device.ID,
			sync.LastSynced,
			sync.Status,
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

	r.db.log.Debug().Msgf("Manga sync deleted: %d", id)

	return nil
}

func (r SyncRepo) Update(ctx context.Context, sync *domain.Sync) (*domain.Sync, error) {
	queryBuilder := r.db.squirrel.
		Update("manga_sync").
		Set("device_id", sync.Device.ID).
		Set("last_sync", sync.LastSynced).
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
			"manga_sync.device_id",
			"manga_sync.last_sync",
			"manga_sync.status",
			"manga_sync.created_at",
			"manga_sync.updated_at",
			"devices.id",
			"devices.name",
			"devices.created_at",
			"devices.updated_at",
			"api_key.name",
			"api_key.scopes",
			"api_key.created_at",
		).
		From("manga_sync").
		InnerJoin("devices ON devices.user_api_key = manga_sync.user_api_key").
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
		// initialize device and api key
		mangaSync.Device = &domain.Device{}
		mangaSync.UserApiKey = &domain.APIKey{}

		if err := rows.Scan(
			&mangaSync.ID,
			&mangaSync.UserApiKey.Key,
			&mangaSync.Device.ID,
			&mangaSync.LastSynced,
			&mangaSync.Status,
			&mangaSync.CreatedAt,
			&mangaSync.UpdatedAt,
			&mangaSync.Device.ID,
			&mangaSync.Device.Name,
			&mangaSync.Device.CreatedAt,
			&mangaSync.Device.UpdatedAt,
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
			"manga_sync.device_id",
			"manga_sync.last_sync",
			"manga_sync.status",
			"manga_sync.created_at",
			"manga_sync.updated_at",
			"devices.id",
			"devices.name",
			"devices.created_at",
			"devices.updated_at",
			"api_key.name",
			"api_key.scopes",
			"api_key.created_at",
		).
		From("manga_sync").
		Join("devices ON devices.user_api_key = manga_sync.user_api_key").
		Join("api_key ON api_key.key = manga_sync.user_api_key").
		Where(sq.Eq{"api_key.key": apiKey})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	var mangaSync domain.Sync
	// initialize device and api key
	mangaSync.Device = &domain.Device{}
	mangaSync.UserApiKey = &domain.APIKey{}

	if err := r.db.handler.QueryRowContext(ctx, query, args...).Scan(
		&mangaSync.ID,
		&mangaSync.UserApiKey.Key,
		&mangaSync.Device.ID,
		&mangaSync.LastSynced,
		&mangaSync.Status,
		&mangaSync.CreatedAt,
		&mangaSync.UpdatedAt,
		&mangaSync.Device.ID,
		&mangaSync.Device.Name,
		&mangaSync.Device.CreatedAt,
		&mangaSync.Device.UpdatedAt,
		&mangaSync.UserApiKey.Name,
		pq.Array(&mangaSync.UserApiKey.Scopes),
		&mangaSync.UserApiKey.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return &domain.Sync{}, errors.Wrap(err, "error executing query")
		}
		return nil, errors.Wrap(err, "error executing query")
	}

	return &mangaSync, nil
}

func (r SyncRepo) GetSyncByDeviceID(ctx context.Context, deviceID int) (*domain.Sync, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"manga_sync.id",
			"manga_sync.user_api_key",
			"manga_sync.device_id",
			"manga_sync.last_sync",
			"manga_sync.status",
			"manga_sync.created_at",
			"manga_sync.updated_at",
			"devices.id",
			"devices.name",
			"devices.created_at",
			"devices.updated_at",
			"api_key.name",
			"api_key.scopes",
			"api_key.created_at",
		).
		From("manga_sync").
		Join("devices ON devices.user_api_key = manga_sync.user_api_key").
		Join("api_key ON api_key.key = manga_sync.user_api_key").
		Where(sq.Eq{"manga_sync.device_id": deviceID})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	var mangaSync domain.Sync
	// initialize device and api key
	mangaSync.Device = &domain.Device{}
	mangaSync.UserApiKey = &domain.APIKey{}

	if err := r.db.handler.QueryRowContext(ctx, query, args...).Scan(
		&mangaSync.ID,
		&mangaSync.UserApiKey.Key,
		&mangaSync.Device.ID,
		&mangaSync.LastSynced,
		&mangaSync.Status,
		&mangaSync.CreatedAt,
		&mangaSync.UpdatedAt,
		&mangaSync.Device.ID,
		&mangaSync.Device.Name,
		&mangaSync.Device.CreatedAt,
		&mangaSync.Device.UpdatedAt,
		&mangaSync.UserApiKey.Name,
		pq.Array(&mangaSync.UserApiKey.Scopes),
		&mangaSync.UserApiKey.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return &domain.Sync{}, nil
		}
		return nil, errors.Wrap(err, "error executing query")
	}

	return &mangaSync, nil
}

func (r SyncRepo) SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error) {

	//TODO implement me
	panic("implement me")
}
