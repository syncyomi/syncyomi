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
		r.log.Debug().Msgf(
			"ETag mismatch detected for api_key=\"%v\". This indicates remote data has been modified since last fetched. Aborting update to avoid overwriting recent changes. Expected ETag=\"%v\", found different ETag on server.",
			apiKey, etag)
		return nil, nil

	} else {
		r.log.Debug().Msgf("Sync data replaced: api_key=\"%v\", etag=\"%v\"", apiKey, etag)
		return &newEtag, nil
	}
}
