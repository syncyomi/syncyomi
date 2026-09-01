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

func NewSyncRepo(log logger.Logger, db *DB, historyLimit int) domain.SyncRepo {
	return &SyncRepo{
		log:          log.With().Str("repo", "sync").Logger(),
		db:           db,
		historyLimit: historyLimit,
	}
}

type SyncRepo struct {
	log          zerolog.Logger
	db           *DB
	historyLimit int
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
	tx, err := r.db.handler.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error starting transaction")
	}
	defer r.rollback(tx)

	etag, err := r.upsertSyncData(ctx, tx, apiKey, data)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "error committing transaction")
	}

	return etag, nil
}

// Returns (nil, nil) when the etag does not match, including when no data exists yet.
func (r SyncRepo) SetSyncDataIfMatch(ctx context.Context, apiKey string, etag string, data []byte) (*string, error) {
	newEtag := newETag()

	tx, err := r.db.handler.BeginTx(ctx, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error starting transaction")
	}
	defer r.rollback(tx)

	result, err := r.db.squirrel.
		Update("sync_data").
		Set("updated_at", time.Now().UTC()).
		Set("data", data).
		Set("data_etag", newEtag).
		Where(sq.Eq{"user_api_key": apiKey}).
		Where(sq.Eq{"data_etag": etag}).
		RunWith(tx).
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

	if err := r.recordHistory(ctx, tx, apiKey, newEtag, data); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, errors.Wrap(err, "error committing transaction")
	}

	return &newEtag, nil
}

func (r SyncRepo) upsertSyncData(ctx context.Context, tx *sql.Tx, apiKey string, data []byte) (*string, error) {
	now := time.Now().UTC()
	etag := newETag()

	_, err := r.db.squirrel.
		Insert("sync_data").
		Columns("user_api_key", "created_at", "updated_at", "data", "data_etag").
		Values(apiKey, now, now, data, etag).
		Suffix("ON CONFLICT (user_api_key) DO UPDATE SET data = EXCLUDED.data, data_etag = EXCLUDED.data_etag, updated_at = EXCLUDED.updated_at").
		RunWith(tx).
		ExecContext(ctx)

	if err != nil {
		return nil, errors.Wrap(err, "error upserting sync data")
	}

	if err := r.recordHistory(ctx, tx, apiKey, etag, data); err != nil {
		return nil, err
	}

	return &etag, nil
}

func (r SyncRepo) recordHistory(ctx context.Context, tx *sql.Tx, apiKey, etag string, data []byte) error {
	if r.historyLimit <= 0 {
		return nil
	}

	_, err := r.db.squirrel.
		Insert("sync_data_history").
		Columns("user_api_key", "data_etag", "size", "created_at", "data").
		Values(apiKey, etag, len(data), time.Now().UTC(), data).
		RunWith(tx).
		ExecContext(ctx)
	if err != nil {
		return errors.Wrap(err, "error recording sync history")
	}

	_, err = tx.ExecContext(ctx, `
		DELETE FROM sync_data_history
		WHERE user_api_key = $1 AND id NOT IN (
			SELECT id FROM sync_data_history WHERE user_api_key = $1 ORDER BY id DESC LIMIT $2
		)`, apiKey, r.historyLimit)
	if err != nil {
		return errors.Wrap(err, "error pruning sync history")
	}

	return nil
}

func (r SyncRepo) ListHistory(ctx context.Context, apiKey string) ([]domain.SyncHistoryEntry, error) {
	rows, err := r.db.squirrel.
		Select("id", "data_etag", "size", "created_at").
		From("sync_data_history").
		Where(sq.Eq{"user_api_key": apiKey}).
		OrderBy("id DESC").
		RunWith(r.db.handler).
		QueryContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}
	defer rows.Close()

	entries := make([]domain.SyncHistoryEntry, 0)
	for rows.Next() {
		var e domain.SyncHistoryEntry
		if err := rows.Scan(&e.ID, &e.ETag, &e.Size, &e.CreatedAt); err != nil {
			return nil, errors.Wrap(err, "error scanning row")
		}
		entries = append(entries, e)
	}

	return entries, rows.Err()
}

func (r SyncRepo) GetHistoryData(ctx context.Context, apiKey string, id int) ([]byte, error) {
	var data []byte
	err := r.db.squirrel.
		Select("data").
		From("sync_data_history").
		Where(sq.Eq{"user_api_key": apiKey, "id": id}).
		RunWith(r.db.handler).
		QueryRowContext(ctx).
		Scan(&data)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, errors.Wrap(err, "error executing query")
	}
	return data, nil
}

func (r SyncRepo) TouchDevice(ctx context.Context, apiKey string, dev domain.DeviceInfo, event, status, message, protocol string) error {
	deviceKey := dev.Key()
	if deviceKey == "" {
		return nil
	}

	now := time.Now().UTC()
	_, err := r.db.handler.ExecContext(ctx, `
		INSERT INTO sync_device (user_api_key, device_id, device_name, last_seen, last_event, last_status, last_message, protocol, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (user_api_key, device_id) DO UPDATE SET
			device_name  = CASE WHEN EXCLUDED.device_name = '' THEN sync_device.device_name ELSE EXCLUDED.device_name END,
			last_seen    = EXCLUDED.last_seen,
			last_event   = CASE WHEN EXCLUDED.last_event = '' THEN sync_device.last_event ELSE EXCLUDED.last_event END,
			last_status  = CASE WHEN EXCLUDED.last_status = '' THEN sync_device.last_status ELSE EXCLUDED.last_status END,
			last_message = CASE WHEN EXCLUDED.last_event = '' THEN sync_device.last_message ELSE EXCLUDED.last_message END,
			protocol     = CASE WHEN EXCLUDED.protocol = '' THEN sync_device.protocol ELSE EXCLUDED.protocol END`,
		apiKey, deviceKey, dev.Name, now, event, status, message, protocol, now)
	if err != nil {
		return errors.Wrap(err, "error upserting device")
	}

	return nil
}

func (r SyncRepo) DeleteDevice(ctx context.Context, apiKey string, id int) error {
	result, err := r.db.squirrel.
		Delete("sync_device").
		Where(sq.Eq{"user_api_key": apiKey, "id": id}).
		RunWith(r.db.handler).
		ExecContext(ctx)
	if err != nil {
		return errors.Wrap(err, "error deleting device")
	}
	if n, err := result.RowsAffected(); err == nil && n == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r SyncRepo) ListDevices(ctx context.Context, apiKey string) ([]domain.SyncDevice, error) {
	rows, err := r.db.squirrel.
		Select("id", "device_id", "device_name", "last_seen", "last_event", "last_status", "last_message", "protocol", "last_cursor", "created_at").
		From("sync_device").
		Where(sq.Eq{"user_api_key": apiKey}).
		OrderBy("last_seen DESC", "id DESC").
		RunWith(r.db.handler).
		QueryContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}
	defer rows.Close()

	devices := make([]domain.SyncDevice, 0)
	for rows.Next() {
		var d domain.SyncDevice
		if err := rows.Scan(&d.ID, &d.DeviceID, &d.DeviceName, &d.LastSeen, &d.LastEvent, &d.LastStatus, &d.LastMessage, &d.Protocol, &d.Cursor, &d.CreatedAt); err != nil {
			return nil, errors.Wrap(err, "error scanning row")
		}
		devices = append(devices, d)
	}

	return devices, rows.Err()
}

func (r SyncRepo) UpsertStatus(ctx context.Context, apiKey string, st domain.SyncStatus) error {
	_, err := r.db.handler.ExecContext(ctx, `
		INSERT INTO sync_status (user_api_key, last_synced_at, last_event_at, last_event, last_status, last_device, last_message, last_protocol)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_api_key) DO UPDATE SET
			last_synced_at = COALESCE(EXCLUDED.last_synced_at, sync_status.last_synced_at),
			last_event_at  = COALESCE(EXCLUDED.last_event_at, sync_status.last_event_at),
			last_event     = CASE WHEN EXCLUDED.last_event = '' THEN sync_status.last_event ELSE EXCLUDED.last_event END,
			last_status    = CASE WHEN EXCLUDED.last_status = '' THEN sync_status.last_status ELSE EXCLUDED.last_status END,
			last_device    = CASE WHEN EXCLUDED.last_device = '' THEN sync_status.last_device ELSE EXCLUDED.last_device END,
			last_message   = CASE WHEN EXCLUDED.last_event = '' THEN sync_status.last_message ELSE EXCLUDED.last_message END,
			last_protocol  = CASE WHEN EXCLUDED.last_protocol = '' THEN sync_status.last_protocol ELSE EXCLUDED.last_protocol END`,
		apiKey, nullTime(st.LastSyncedAt), nullTime(st.LastEventAt), st.LastEvent, st.LastStatus, st.LastDevice, st.LastMessage, st.LastProtocol)
	if err != nil {
		return errors.Wrap(err, "error upserting sync status")
	}

	return nil
}

func (r SyncRepo) GetStatus(ctx context.Context, apiKey string) (*domain.SyncStatus, error) {
	var (
		st    domain.SyncStatus
		found bool
	)

	var lastSynced, lastEvent sql.NullTime
	err := r.db.squirrel.
		Select("last_synced_at", "last_event_at", "last_event", "last_status", "last_device", "last_message", "last_protocol").
		From("sync_status").
		Where(sq.Eq{"user_api_key": apiKey}).
		RunWith(r.db.handler).
		QueryRowContext(ctx).
		Scan(&lastSynced, &lastEvent, &st.LastEvent, &st.LastStatus, &st.LastDevice, &st.LastMessage, &st.LastProtocol)
	switch {
	case err == nil:
		found = true
		st.LastSyncedAt = timePtr(lastSynced)
		st.LastEventAt = timePtr(lastEvent)
	case stderrors.Is(err, sql.ErrNoRows):
	default:
		return nil, errors.Wrap(err, "error executing query")
	}

	// the served payload: the raw v1 upload when one exists, else the render cache
	var updatedAt sql.NullTime
	err = r.db.squirrel.
		Select("COALESCE(length(raw_data), length(data))", "updated_at").
		From("sync_data").
		Where(sq.Eq{"user_api_key": apiKey}).
		RunWith(r.db.handler).
		QueryRowContext(ctx).
		Scan(&st.DataSize, &updatedAt)
	switch {
	case err == nil:
		found = true
		st.DataUpdatedAt = timePtr(updatedAt)
	case stderrors.Is(err, sql.ErrNoRows):
	default:
		return nil, errors.Wrap(err, "error executing query")
	}

	err = r.db.squirrel.
		Select("seq").
		From("sync_state").
		Where(sq.Eq{"user_api_key": apiKey}).
		RunWith(r.db.handler).
		QueryRowContext(ctx).
		Scan(&st.Seq)
	switch {
	case err == nil:
		found = true
	case stderrors.Is(err, sql.ErrNoRows):
	default:
		return nil, errors.Wrap(err, "error executing query")
	}

	rows, err := r.db.squirrel.
		Select("kind", "COUNT(*)").
		From("sync_item").
		Where(sq.Eq{"user_api_key": apiKey, "deleted": false}).
		GroupBy("kind").
		RunWith(r.db.handler).
		QueryContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}
	defer rows.Close()
	for rows.Next() {
		var (
			kind  string
			count int64
		)
		if err := rows.Scan(&kind, &count); err != nil {
			return nil, errors.Wrap(err, "error scanning row")
		}
		switch kind {
		case "manga":
			st.MangaCount = count
		case "chapter":
			st.ChapterCount = count
		case "category":
			st.CategoryCount = count
		}
	}
	if err := rows.Err(); err != nil {
		return nil, errors.Wrap(err, "error executing query")
	}

	if !found {
		return nil, nil
	}

	st.HistoryLimit = r.historyLimit
	return &st, nil
}

func (r SyncRepo) rollback(tx *sql.Tx) {
	if err := tx.Rollback(); err != nil && !stderrors.Is(err, sql.ErrTxDone) {
		r.log.Error().Err(err).Msg("error rolling back transaction")
	}
}

func nullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func timePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
