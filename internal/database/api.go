package database

import (
	"context"
	"database/sql"
	stderrors "errors"

	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
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
	log zerolog.Logger
	db  *DB
}

func (r *APIRepo) Get(ctx context.Context, key string) (*domain.APIKey, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"name",
			"key",
			"scopes",
			"created_at",
		).
		From("api_key")

	queryBuilder = queryBuilder.Where(sq.Eq{"key": key})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	row := r.db.handler.QueryRowContext(ctx, query, args...)

	var a domain.APIKey

	var name sql.NullString

	if err := row.Scan(&name, &a.Key, pq.Array(&a.Scopes), &a.CreatedAt); err != nil {
		return nil, errors.Wrap(err, "error scanning row")
	}

	a.Name = name.String

	return &a, nil
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

// SQLite runs with foreign keys off, so the ON DELETE CASCADE is done by hand.
func (r *APIRepo) Delete(ctx context.Context, key string) error {
	tx, err := r.db.handler.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "error starting transaction")
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !stderrors.Is(err, sql.ErrTxDone) {
			r.log.Error().Err(err).Msg("error rolling back transaction")
		}
	}()

	for _, table := range apiKeyDependentTables {
		query, args, err := r.db.squirrel.
			Delete(table).
			Where(sq.Eq{"user_api_key": key}).
			ToSql()
		if err != nil {
			return errors.Wrap(err, "error building query")
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return errors.Wrap(err, "error deleting from %s", table)
		}
	}

	query, args, err := r.db.squirrel.
		Delete("api_key").
		Where(sq.Eq{"key": key}).
		ToSql()
	if err != nil {
		return errors.Wrap(err, "error building query")
	}

	if _, err = tx.ExecContext(ctx, query, args...); err != nil {
		return errors.Wrap(err, "error executing query")
	}

	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "error committing transaction")
	}

	r.log.Debug().Msgf("successfully deleted: %v", key)

	return nil
}

var apiKeyDependentTables = []string{"sync_item", "sync_state", "sync_data", "sync_data_history", "sync_device", "sync_status"}

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
