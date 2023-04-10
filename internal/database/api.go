package database

import (
	"context"
	"database/sql"
	sq "github.com/Masterminds/squirrel"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/logger"
	"github.com/kaiserbh/tachiyomi-sync-server/pkg/errors"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
	"time"
)

func NewAPIRepo(log logger.Logger, db *DB) domain.APIRepo {
	return &APIRepo{
		log: log.With().Str("repo", "api").Logger(),
		db:  db,
	}
}

type APIRepo struct {
	log   zerolog.Logger
	db    *DB
	cache map[string]domain.APIKey
}

func (r *APIRepo) Store(ctx context.Context, key *domain.APIKey) error {
	queryBuilder := r.db.squirrel.
		Insert("api_key").
		Columns(
			"name",
			"key",
			"scopes",
		).
		Values(
			key.Name,
			key.Key,
			pq.Array(key.Scopes),
		).
		Suffix("RETURNING created_at").RunWith(r.db.handler)

	var createdAt time.Time

	if err := queryBuilder.QueryRowContext(ctx).Scan(&createdAt); err != nil {
		return errors.Wrap(err, "error executing query")
	}

	key.CreatedAt = &createdAt

	return nil
}

func (r *APIRepo) Delete(ctx context.Context, key string) error {
	queryBuilder := r.db.squirrel.
		Delete("api_key").
		Where(sq.Eq{"key": key})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return errors.Wrap(err, "error building query")
	}

	_, err = r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "error executing query")
	}

	r.log.Debug().Msgf("successfully deleted: %v", key)

	return nil
}

func (r *APIRepo) GetKeys(ctx context.Context) ([]domain.APIKey, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"name",
			"key",
			"scopes",
			"created_at",
		).
		From("api_key")

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

	keys := make([]domain.APIKey, 0)
	for rows.Next() {
		var a domain.APIKey

		var name sql.NullString

		if err := rows.Scan(&name, &a.Key, pq.Array(&a.Scopes), &a.CreatedAt); err != nil {
			return nil, errors.Wrap(err, "error scanning row")

		}

		a.Name = name.String

		keys = append(keys, a)
	}

	return keys, nil
}
