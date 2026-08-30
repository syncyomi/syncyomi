package database

import (
	"context"
	"database/sql"
	stderrors "errors"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/logger"
	"github.com/SyncYomi/SyncYomi/internal/merge"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/rs/zerolog"
)

const (
	refsSep      = "\x1f"
	sqlBatchSize = 500
	itemColumns  = "kind, key, parent_key, name, version, deleted, refs, payload, seq, origin_device"
)

func NewSyncStore(log logger.Logger, db *DB, historyLimit int) domain.SyncStore {
	return &SyncStoreRepo{
		log:     log.With().Str("repo", "syncstore").Logger(),
		db:      db,
		history: SyncRepo{log: log.With().Str("repo", "sync").Logger(), db: db, historyLimit: historyLimit},
	}
}

type SyncStoreRepo struct {
	log     zerolog.Logger
	db      *DB
	history SyncRepo
}

// Tx runs fn in one transaction holding the key's sync_state row. On Postgres the row is
// locked with FOR UPDATE so a second instance queues; SQLite serialises writers itself.
func (r *SyncStoreRepo) Tx(ctx context.Context, apiKey string, fn func(tx domain.SyncStoreTx) error) error {
	tx, err := r.db.handler.BeginTx(ctx, nil)
	if err != nil {
		return errors.Wrap(err, "error starting transaction")
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !stderrors.Is(err, sql.ErrTxDone) {
			r.log.Error().Err(err).Msg("error rolling back transaction")
		}
	}()

	st := &syncStoreTx{repo: r, tx: tx, apiKey: apiKey}
	if err := st.loadState(ctx); err != nil {
		return err
	}
	if err := fn(st); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "error committing transaction")
	}
	return nil
}

type syncStoreTx struct {
	repo   *SyncStoreRepo
	tx     *sql.Tx
	apiKey string
	seq    int64
	exists bool
}

func (t *syncStoreTx) Seq() int64   { return t.seq }
func (t *syncStoreTx) Exists() bool { return t.exists }

func (t *syncStoreTx) loadState(ctx context.Context) error {
	query := `SELECT seq FROM sync_state WHERE user_api_key = $1`
	if databaseDriver == "postgres" {
		query += " FOR UPDATE"
	}
	err := t.tx.QueryRowContext(ctx, query, t.apiKey).Scan(&t.seq)
	switch {
	case err == nil:
		t.exists = true
	case stderrors.Is(err, sql.ErrNoRows):
		// created lazily by the first Apply
	default:
		return errors.Wrap(err, "error loading sync state")
	}
	return nil
}

func (t *syncStoreTx) GetItems(ctx context.Context, kind merge.Kind, keys []string) (map[string]*merge.Item, error) {
	out := make(map[string]*merge.Item, len(keys))
	for start := 0; start < len(keys); start += sqlBatchSize {
		end := min(start+sqlBatchSize, len(keys))
		items, err := t.queryItems(ctx, sq.And{
			sq.Eq{"user_api_key": t.apiKey, "kind": string(kind)},
			sq.Eq{"key": keys[start:end]},
		})
		if err != nil {
			return nil, err
		}
		for _, it := range items {
			out[it.Key] = it
		}
	}
	return out, nil
}

func (t *syncStoreTx) Categories(ctx context.Context) ([]*merge.Item, error) {
	return t.queryItems(ctx, sq.Eq{"user_api_key": t.apiKey, "kind": string(merge.KindCategory)})
}

func (t *syncStoreTx) AllItems(ctx context.Context) ([]*merge.Item, error) {
	return t.queryItems(ctx, sq.Eq{"user_api_key": t.apiKey})
}

func (t *syncStoreTx) ItemsSince(ctx context.Context, seq int64) ([]*merge.Item, error) {
	return t.queryItems(ctx, sq.And{
		sq.Eq{"user_api_key": t.apiKey},
		sq.Or{sq.Gt{"seq": seq}, sq.Eq{"kind": string(merge.KindCategory)}},
	})
}

func (t *syncStoreTx) ItemsByKeys(ctx context.Context, keys map[merge.Kind][]string) ([]*merge.Item, error) {
	var out []*merge.Item
	for kind, ks := range keys {
		found, err := t.GetItems(ctx, kind, ks)
		if err != nil {
			return nil, err
		}
		for _, it := range found {
			out = append(out, it)
		}
	}
	return out, nil
}

func (t *syncStoreTx) ChaptersOf(ctx context.Context, mangaKeys []string) ([]*merge.Item, error) {
	var out []*merge.Item
	for start := 0; start < len(mangaKeys); start += sqlBatchSize {
		end := min(start+sqlBatchSize, len(mangaKeys))
		items, err := t.queryItems(ctx, sq.And{
			sq.Eq{"user_api_key": t.apiKey, "kind": string(merge.KindChapter)},
			sq.Eq{"parent_key": mangaKeys[start:end]},
		})
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	return out, nil
}

func (t *syncStoreTx) queryItems(ctx context.Context, where sq.Sqlizer) ([]*merge.Item, error) {
	rows, err := t.repo.db.squirrel.
		Select(strings.Split(itemColumns, ", ")...).
		From("sync_item").
		Where(where).
		OrderBy("seq", "key").
		RunWith(t.tx).
		QueryContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error querying sync items")
	}
	defer rows.Close()

	var items []*merge.Item
	for rows.Next() {
		var (
			it   merge.Item
			kind string
			refs string
		)
		if err := rows.Scan(&kind, &it.Key, &it.ParentKey, &it.Name, &it.Version, &it.Deleted, &refs, &it.Payload, &it.Seq, &it.OriginDevice); err != nil {
			return nil, errors.Wrap(err, "error scanning sync item")
		}
		it.Kind = merge.Kind(kind)
		if refs != "" {
			it.Refs = strings.Split(refs, refsSep)
		}
		items = append(items, &it)
	}
	return items, rows.Err()
}

func (t *syncStoreTx) Apply(ctx context.Context, res *merge.Result, device string) (int64, error) {
	if len(res.Writes) == 0 && len(res.Tombstones) == 0 {
		if !t.exists {
			if err := t.ensureState(ctx); err != nil {
				return 0, err
			}
		}
		return t.seq, nil
	}

	newSeq := t.seq + 1
	if err := t.writeItems(ctx, res.Writes, newSeq, device); err != nil {
		return 0, err
	}

	for _, key := range res.Tombstones {
		_, err := t.repo.db.squirrel.
			Update("sync_item").
			Set("deleted", true).
			Set("seq", newSeq).
			Set("origin_device", device).
			Where(sq.Eq{"user_api_key": t.apiKey, "kind": string(merge.KindCategory), "key": key}).
			RunWith(t.tx).
			ExecContext(ctx)
		if err != nil {
			return 0, errors.Wrap(err, "error tombstoning category")
		}
	}

	if err := t.ensureState(ctx); err != nil {
		return 0, err
	}
	_, err := t.repo.db.squirrel.
		Update("sync_state").
		Set("seq", newSeq).
		Set("updated_at", time.Now()).
		Where(sq.Eq{"user_api_key": t.apiKey}).
		RunWith(t.tx).
		ExecContext(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "error updating sync state")
	}
	t.seq = newSeq
	return newSeq, nil
}

const upsertItemSuffix = `ON CONFLICT (user_api_key, kind, key) DO UPDATE SET
	parent_key = EXCLUDED.parent_key, name = EXCLUDED.name, version = EXCLUDED.version,
	deleted = EXCLUDED.deleted, refs = EXCLUDED.refs, payload = EXCLUDED.payload,
	seq = EXCLUDED.seq, origin_device = EXCLUDED.origin_device`

// writeItems upserts rows. modernc sqlite binds thousands of parameters very slowly, so it
// gets a prepared single-row statement; Postgres is round-trip bound and gets multi-row batches.
func (t *syncStoreTx) writeItems(ctx context.Context, items []*merge.Item, seq int64, device string) error {
	if len(items) == 0 {
		return nil
	}
	if databaseDriver != "postgres" {
		stmt, err := t.tx.PrepareContext(ctx, `INSERT INTO sync_item (user_api_key, kind, key, parent_key, name, version, deleted, refs, payload, seq, origin_device)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) `+upsertItemSuffix)
		if err != nil {
			return errors.Wrap(err, "error preparing sync item upsert")
		}
		defer stmt.Close()
		for _, it := range items {
			if _, err := stmt.ExecContext(ctx, t.apiKey, string(it.Kind), it.Key, it.ParentKey, it.Name, it.Version, false, strings.Join(it.Refs, refsSep), it.Payload, seq, device); err != nil {
				return errors.Wrap(err, "error writing sync item")
			}
		}
		return nil
	}

	const batch = 100
	for start := 0; start < len(items); start += batch {
		end := min(start+batch, len(items))
		ins := t.repo.db.squirrel.
			Insert("sync_item").
			Columns("user_api_key", "kind", "key", "parent_key", "name", "version", "deleted", "refs", "payload", "seq", "origin_device")
		for _, it := range items[start:end] {
			ins = ins.Values(t.apiKey, string(it.Kind), it.Key, it.ParentKey, it.Name, it.Version, false, strings.Join(it.Refs, refsSep), it.Payload, seq, device)
		}
		if _, err := ins.Suffix(upsertItemSuffix).RunWith(t.tx).ExecContext(ctx); err != nil {
			return errors.Wrap(err, "error writing sync items")
		}
	}
	return nil
}

func (t *syncStoreTx) ensureState(ctx context.Context) error {
	if t.exists {
		return nil
	}
	now := time.Now()
	_, err := t.repo.db.squirrel.
		Insert("sync_state").
		Columns("user_api_key", "seq", "created_at", "updated_at").
		Values(t.apiKey, t.seq, now, now).
		Suffix("ON CONFLICT (user_api_key) DO NOTHING").
		RunWith(t.tx).
		ExecContext(ctx)
	if err != nil {
		return errors.Wrap(err, "error creating sync state")
	}
	t.exists = true
	return nil
}

func (t *syncStoreTx) RenderCache(ctx context.Context) (*domain.RenderCache, error) {
	var (
		rc  domain.RenderCache
		seq sql.NullInt64
	)
	err := t.repo.db.squirrel.
		Select("data", "data_etag", "rendered_seq").
		From("sync_data").
		Where(sq.Eq{"user_api_key": t.apiKey}).
		RunWith(t.tx).
		QueryRowContext(ctx).
		Scan(&rc.Data, &rc.ETag, &seq)
	if err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "error reading render cache")
	}
	if seq.Valid {
		rc.RenderedSeq = &seq.Int64
	}
	return &rc, nil
}

func (t *syncStoreTx) SetRenderCache(ctx context.Context, data []byte, etag string, seq int64) error {
	now := time.Now()
	_, err := t.repo.db.squirrel.
		Insert("sync_data").
		Columns("user_api_key", "created_at", "updated_at", "data", "data_etag", "rendered_seq").
		Values(t.apiKey, now, now, data, etag, seq).
		Suffix("ON CONFLICT (user_api_key) DO UPDATE SET data = EXCLUDED.data, data_etag = EXCLUDED.data_etag, updated_at = EXCLUDED.updated_at, rendered_seq = EXCLUDED.rendered_seq").
		RunWith(t.tx).
		ExecContext(ctx)
	if err != nil {
		return errors.Wrap(err, "error writing render cache")
	}
	return t.repo.history.recordHistory(ctx, t.tx, t.apiKey, etag, data)
}

func (t *syncStoreTx) MarkRendered(ctx context.Context, seq int64) error {
	_, err := t.repo.db.squirrel.
		Update("sync_data").
		Set("rendered_seq", seq).
		Where(sq.Eq{"user_api_key": t.apiKey}).
		RunWith(t.tx).
		ExecContext(ctx)
	if err != nil {
		return errors.Wrap(err, "error marking render cache")
	}
	return nil
}

func (t *syncStoreTx) Clear(ctx context.Context) error {
	_, err := t.repo.db.squirrel.
		Delete("sync_item").
		Where(sq.Eq{"user_api_key": t.apiKey}).
		RunWith(t.tx).
		ExecContext(ctx)
	if err != nil {
		return errors.Wrap(err, "error clearing sync items")
	}
	return nil
}

func (t *syncStoreTx) SetDeviceCursor(ctx context.Context, dc domain.DeviceCursor) error {
	deviceKey := dc.Device.Key()
	if deviceKey == "" {
		return nil
	}
	now := time.Now()
	_, err := t.tx.ExecContext(ctx, `
		INSERT INTO sync_device (user_api_key, device_id, device_name, last_seen, last_cursor, protocol, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_api_key, device_id) DO UPDATE SET
			device_name = CASE WHEN EXCLUDED.device_name = '' THEN sync_device.device_name ELSE EXCLUDED.device_name END,
			last_seen   = EXCLUDED.last_seen,
			last_cursor = EXCLUDED.last_cursor,
			protocol    = EXCLUDED.protocol`,
		t.apiKey, deviceKey, dc.Device.Name, now, dc.Cursor, dc.Protocol, now)
	if err != nil {
		return errors.Wrap(err, "error updating device cursor")
	}
	return nil
}
