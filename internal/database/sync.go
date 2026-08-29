package database

import (
	"context"
	"database/sql"
	stderrors "errors"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

func NewSyncRepo(log logger.Logger, db *DB) domain.SyncRepo {
	return &SyncRepo{
		log: log.With().Str("repo", "sync").Logger(),
		db:  db,
	}
}

type SyncRepo struct {
	log zerolog.Logger
	db  *DB
}

// Wire format ("uuid=<uuid4>", unquoted) is part of the client contract:
// TachiyomiSY and Suwayomi echo the header value verbatim. Do not change it.
func newETag() string {
	return "uuid=" + uuid.NewString()
}

func (r SyncRepo) GetSyncDataETag(ctx context.Context, apiKey string) (*string, error) {
	var etag string

	err := r.db.squirrel.
		Select("data_etag").
		From("sync_data").
		Where(sq.Eq{"user_api_key": apiKey}).
		Limit(1).
		RunWith(r.db.handler).
		QueryRowContext(ctx).
		Scan(&etag)

	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "error executing query")
	}

	return &etag, nil
}

func (r SyncRepo) GetSyncDataAndETag(ctx context.Context, apiKey string) ([]byte, *string, error) {
	var etag string
	var data []byte

	err := r.db.squirrel.
		Select("data", "data_etag").
		From("sync_data").
		Where(sq.Eq{"user_api_key": apiKey}).
		Limit(1).
		RunWith(r.db.handler).
		QueryRowContext(ctx).
		Scan(&data, &etag)

	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, nil, nil
		}
		return nil, nil, errors.Wrap(err, "error executing query")
	}

	return data, &etag, nil
}

func (r SyncRepo) SetSyncData(ctx context.Context, apiKey string, data []byte) (*string, error) {
	now := time.Now()
	etag := newETag()

	_, err := r.db.squirrel.
		Insert("sync_data").
		Columns("user_api_key", "created_at", "updated_at", "data", "data_etag").
		Values(apiKey, now, now, data, etag).
		Suffix("ON CONFLICT (user_api_key) DO UPDATE SET data = EXCLUDED.data, data_etag = EXCLUDED.data_etag, updated_at = EXCLUDED.updated_at").
		RunWith(r.db.handler).
		ExecContext(ctx)

	if err != nil {
		r.log.Err(err).Msg("error upserting sync data")
		return nil, errors.Wrap(err, "error executing query")
	}

	return &etag, nil
}

// Returns (nil, nil) when the etag does not match, including when no data exists yet.
func (r SyncRepo) SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error) {
	now := time.Now()
	newEtag := newETag()

	result, err := r.db.squirrel.
		Update("sync_data").
		Set("updated_at", now).
		Set("data", data).
		Set("data_etag", newEtag).
		Where(sq.Eq{"user_api_key": apiKey}).
		Where(sq.Eq{"data_etag": etag}).
		RunWith(r.db.handler).
		ExecContext(ctx)

	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	if rowsAffected == 0 {
		r.log.Debug().Msgf("ETag mismatch, aborting update. Expected ETag=%q", etag)
		return nil, nil
	}

	return &newEtag, nil
}
