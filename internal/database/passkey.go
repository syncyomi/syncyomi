package database

import (
	"context"
	"database/sql"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/rs/zerolog"
)

type PasskeyRepo struct {
	log zerolog.Logger
	db  *DB
}

func NewPasskeyRepo(log logger.Logger, db *DB) domain.PasskeyRepo {
	return &PasskeyRepo{
		log: log.With().Str("repo", "passkey").Logger(),
		db:  db,
	}
}

var passkeyColumns = []string{"id", "user_id", "credential_id", "name", "credential_json", "created_at", "last_used_at"}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPasskey(row rowScanner) (domain.Passkey, error) {
	var (
		passkey    domain.Passkey
		createdAt  sql.NullTime
		lastUsedAt sql.NullTime
	)

	if err := row.Scan(&passkey.ID, &passkey.UserID, &passkey.CredentialID, &passkey.Name, &passkey.CredentialJSON, &createdAt, &lastUsedAt); err != nil {
		return domain.Passkey{}, err
	}

	passkey.CreatedAt = createdAt.Time
	passkey.LastUsedAt = lastUsedAt.Time

	return passkey, nil
}

func (r *PasskeyRepo) List(ctx context.Context, userID int) ([]domain.Passkey, error) {
	query, args, err := r.db.squirrel.
		Select(passkeyColumns...).
		From("passkeys").
		Where(sq.Eq{"user_id": userID}).
		OrderBy("id ASC").
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	rows, err := r.db.handler.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}
	defer rows.Close()

	passkeys := []domain.Passkey{}
	for rows.Next() {
		passkey, err := scanPasskey(rows)
		if err != nil {
			return nil, errors.Wrap(err, "error scanning row")
		}
		passkeys = append(passkeys, passkey)
	}

	return passkeys, errors.Wrap(rows.Err(), "error reading rows")
}

func (r *PasskeyRepo) FindByCredentialID(ctx context.Context, credentialID []byte) (*domain.Passkey, error) {
	query, args, err := r.db.squirrel.
		Select(passkeyColumns...).
		From("passkeys").
		Where(sq.Eq{"credential_id": credentialID}).
		ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	passkey, err := scanPasskey(r.db.handler.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "error scanning row")
	}

	return &passkey, nil
}

func (r *PasskeyRepo) Store(ctx context.Context, passkey domain.Passkey) (int, error) {
	query, args, err := r.db.squirrel.
		Insert("passkeys").
		Columns("user_id", "credential_id", "name", "credential_json").
		Values(passkey.UserID, passkey.CredentialID, passkey.Name, passkey.CredentialJSON).
		Suffix("RETURNING id").
		ToSql()
	if err != nil {
		return 0, errors.Wrap(err, "error building query")
	}

	var id int
	if err := r.db.handler.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		return 0, errors.Wrap(err, "error executing query")
	}

	return id, nil
}

func (r *PasskeyRepo) UpdateCredential(ctx context.Context, id int, credentialJSON string, usedAt time.Time) error {
	query, args, err := r.db.squirrel.
		Update("passkeys").
		Set("credential_json", credentialJSON).
		Set("last_used_at", usedAt.UTC()).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return errors.Wrap(err, "error building query")
	}

	if _, err := r.db.handler.ExecContext(ctx, query, args...); err != nil {
		return errors.Wrap(err, "error executing query")
	}

	return nil
}

func (r *PasskeyRepo) Delete(ctx context.Context, userID, id int) (bool, error) {
	affected, err := r.deleteWhere(ctx, sq.Eq{"user_id": userID, "id": id})

	return affected > 0, err
}

func (r *PasskeyRepo) DeleteAll(ctx context.Context, userID int) (int, error) {
	return r.deleteWhere(ctx, sq.Eq{"user_id": userID})
}

func (r *PasskeyRepo) deleteWhere(ctx context.Context, where sq.Eq) (int, error) {
	query, args, err := r.db.squirrel.Delete("passkeys").Where(where).ToSql()
	if err != nil {
		return 0, errors.Wrap(err, "error building query")
	}

	result, err := r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, errors.Wrap(err, "error executing query")
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, errors.Wrap(err, "error reading affected rows")
	}

	return int(affected), nil
}
