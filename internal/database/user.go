package database

import (
	"context"
	"database/sql"
	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/rs/zerolog"
)

type UserRepo struct {
	log zerolog.Logger
	db  *DB
}

func NewUserRepo(log logger.Logger, db *DB) domain.UserRepo {
	return &UserRepo{
		log: log.With().Str("repo", "user").Logger(),
		db:  db,
	}
}

func (r *UserRepo) GetUserCount(ctx context.Context) (int, error) {
	queryBuilder := r.db.squirrel.Select("count(*)").From("users")

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return 0, errors.Wrap(err, "error building query")
	}

	row := r.db.handler.QueryRowContext(ctx, query, args...)
	if err := row.Err(); err != nil {
		return 0, errors.Wrap(err, "error executing query")
	}

	result := 0
	if err := row.Scan(&result); err != nil {
		return 0, errors.Wrap(err, "error scanning row")
	}

	return result, nil
}

func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	queryBuilder := r.db.squirrel.
		Select("id", "username", "password", "password_stamp").
		From("users").
		Where(sq.Eq{"username": username})

	return r.findOne(ctx, queryBuilder)
}

func (r *UserRepo) findOne(ctx context.Context, queryBuilder sq.SelectBuilder) (*domain.User, error) {
	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	var user domain.User

	row := r.db.handler.QueryRowContext(ctx, query, args...)
	if err := row.Scan(&user.ID, &user.Username, &user.Password, &user.Stamp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}

		return nil, errors.Wrap(err, "error scanning row")
	}

	return &user, nil
}

func (r *UserRepo) Store(ctx context.Context, user domain.User) error {

	var err error

	queryBuilder := r.db.squirrel.
		Insert("users").
		Columns("username", "password", "password_stamp").
		Values(user.Username, user.Password, user.Stamp)

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return errors.Wrap(err, "error building query")
	}

	_, err = r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "error executing query")
	}

	return err
}

func (r *UserRepo) FindFirst(ctx context.Context) (*domain.User, error) {
	queryBuilder := r.db.squirrel.
		Select("id", "username", "password", "password_stamp").
		From("users").
		OrderBy("id ASC").
		Limit(1)

	return r.findOne(ctx, queryBuilder)
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id int, hash, stamp string) error {
	queryBuilder := r.db.squirrel.
		Update("users").
		Set("password", hash).
		Set("password_stamp", stamp).
		Where(sq.Eq{"id": id})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return errors.Wrap(err, "error building query")
	}

	result, err := r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "error executing query")
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return errors.Wrap(err, "error reading affected rows")
	}
	if affected == 0 {
		return errors.New("no user with id %d", id)
	}

	return nil
}

func (r *UserRepo) RehashPassword(ctx context.Context, id int, oldHash, newHash string) (bool, error) {
	legacyStamp := domain.User{Password: oldHash}.LegacyPasswordStamp()

	queryBuilder := r.db.squirrel.
		Update("users").
		Set("password", newHash).
		Set("password_stamp", sq.Expr("CASE WHEN password_stamp = '' THEN ? ELSE password_stamp END", legacyStamp)).
		Where(sq.Eq{"id": id, "password": oldHash})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return false, errors.Wrap(err, "error building query")
	}

	result, err := r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		return false, errors.Wrap(err, "error executing query")
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, errors.Wrap(err, "error reading affected rows")
	}

	return affected > 0, nil
}
