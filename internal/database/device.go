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

func NewDeviceRepo(log logger.Logger, db *DB) domain.DeviceRepo {
	return &DeviceRepo{
		log: log.With().Str("module", "device").Logger(),
		db:  db,
	}
}

type DeviceRepo struct {
	log zerolog.Logger
	db  *DB
}

func (r DeviceRepo) Store(ctx context.Context, device *domain.Device) (*domain.Device, error) {
	queryBuilder := r.db.squirrel.
		Insert("devices").
		Columns(
			"user_api_key",
			"name",
		).
		Values(
			device.UserApiKey.Key,
			device.Name,
		).
		Suffix("RETURNING id, created_at").RunWith(r.db.handler)

	var id int
	var createdAt time.Time

	if err := queryBuilder.QueryRowContext(ctx).Scan(&id, &createdAt); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	device.ID = id
	device.CreatedAt = &createdAt

	return device, nil
}

func (r DeviceRepo) Delete(ctx context.Context, id int) error {
	queryBuilder := r.db.squirrel.
		Delete("devices").
		Where(sq.Eq{"id": id})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return errors.Wrap(err, "error building query")
	}

	_, err = r.db.handler.ExecContext(ctx, query, args...)
	if err != nil {
		return errors.Wrap(err, "error executing query")
	}

	r.db.log.Debug().Int("id", id).Msg("device deleted")

	return nil
}

func (r DeviceRepo) ListDevices(ctx context.Context, apikey string) ([]domain.Device, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"devices.id",
			"devices.name",
			"devices.created_at",
			"devices.updated_at",
			"devices.user_api_key",
			"api_key.name",
			"api_key.scopes",
			"api_key.created_at",
		).
		From("devices").
		Join("api_key ON devices.user_api_key = api_key.key")

	if apikey != "" {
		queryBuilder = queryBuilder.Where("devices.user_api_key = ?", apikey) // Filter devices by API key if provided that way we don't leak other devices
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

	devices := make([]domain.Device, 0)
	for rows.Next() {
		var device domain.Device
		// Initialize the API key to avoid a nil pointer dereference when
		device.UserApiKey = &domain.APIKey{}

		var name sql.NullString

		if err := rows.Scan(
			&device.ID,
			&name,
			&device.CreatedAt,
			&device.UpdatedAt,
			&device.UserApiKey.Key,
			&device.UserApiKey.Name,
			pq.Array(&device.UserApiKey.Scopes),
			&device.UserApiKey.CreatedAt,
		); err != nil {
			return nil, errors.Wrap(err, "error scanning row")
		}

		device.Name = name.String

		devices = append(devices, device)
	}

	return devices, nil
}

func (r DeviceRepo) GetDeviceByDeviceId(ctx context.Context, device *domain.Device) (*domain.Device, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"devices.id",
			"devices.user_api_key",
			"devices.name",
			"devices.created_at",
			"devices.updated_at",
		).
		From("devices").
		Where(sq.Eq{"devices.id": device.ID})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	var name sql.NullString

	if err := r.db.handler.QueryRowContext(ctx, query, args...).Scan(
		&device.ID,
		&device.UserApiKey.Key,
		&name,
		&device.CreatedAt,
		&device.UpdatedAt,
	); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	device.Name = name.String

	return device, nil
}

func (r DeviceRepo) GetDeviceByApiKey(ctx context.Context, device *domain.Device) (*domain.Device, error) {
	queryBuilder := r.db.squirrel.
		Select(
			"devices.id",
			"devices.user_api_key",
			"devices.name",
			"devices.created_at",
			"devices.updated_at",
		).
		From("devices").
		Where(sq.Eq{"devices.user_api_key": device.UserApiKey.Key})

	query, args, err := queryBuilder.ToSql()
	if err != nil {
		return nil, errors.Wrap(err, "error building query")
	}

	var name sql.NullString

	if err := r.db.handler.QueryRowContext(ctx, query, args...).Scan(
		&device.ID,
		&device.UserApiKey.Key,
		&name,
		&device.CreatedAt,
		&device.UpdatedAt,
	); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	device.Name = name.String

	return device, nil
}
