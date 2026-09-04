package sync

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	gosync "sync"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/merge"
	"github.com/google/uuid"
)

const (
	ProtocolV1 = "v1"
	ProtocolV2 = "v2"

	deviceLegacy  = "legacy"
	deviceRestore = "restore"
)

var (
	ErrPreconditionFailed = errors.New("etag mismatch")
	ErrBadPayload         = errors.New("payload is not a valid backup")
	ErrNoData             = errors.New("no sync data")
)

type MergeRequest struct {
	APIKey            string
	Device            domain.DeviceInfo
	Cursor            int64
	Full              bool
	Backup            *pb.Backup
	DeletedCategories []int64
}

type MergeResponse struct {
	Backup        *pb.Backup
	Cursor        int64
	Changed       bool
	FullRequested bool
}

type Snapshot struct {
	Data   []byte
	ETag   string
	Cursor int64
}

func etagFor(seq int64) string { return "seq=" + strconv.FormatInt(seq, 10) }

// rawETag mints an etag for a raw v1 upload. The wire format ("uuid=<uuid4>", unquoted)
// matches pre-1.3 servers: v1 clients echo the value verbatim, and the prefix keeps raw
// etags distinguishable from the render cache's "seq=<n>".
func rawETag() string { return "uuid=" + uuid.NewString() }

// keyLocks serialises merges per API key inside this process; the database transaction
// covers other instances.
type keyLocks struct {
	m gosync.Map
}

// legacyWarnings remembers when a v1 device was last logged about, per key+device.
var legacyWarnings gosync.Map

func (s *service) warnLegacy(apiKey string, dev domain.DeviceInfo) {
	id := apiKey + "\x00" + dev.Key()
	now := time.Now()
	if last, ok := legacyWarnings.Load(id); ok && now.Sub(last.(time.Time)) < 24*time.Hour {
		return
	}
	legacyWarnings.Store(id, now)
	s.log.Warn().Str("device", dev.Name).Msg("device uses the deprecated v1 sync protocol; update the client to use v2")
}

func (k *keyLocks) lock(ctx context.Context, key string) (func(), error) {
	sem, _ := k.m.LoadOrStore(key, make(chan struct{}, 1))
	ch := sem.(chan struct{})
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Merge is the v2 sync: merge the client's items into the store and return what it lacks.
func (s *service) Merge(ctx context.Context, req MergeRequest) (*MergeResponse, error) {
	items, err := splitBackup(req.Backup)
	if err != nil {
		return nil, err
	}
	unlock, err := s.locks.lock(ctx, req.APIKey)
	if err != nil {
		return nil, err
	}
	defer unlock()

	var resp *MergeResponse
	err = s.store.Tx(ctx, req.APIKey, func(tx domain.SyncStoreTx) error {
		if err := s.promoteLegacy(ctx, tx); err != nil {
			return err
		}
		existed := tx.Exists()
		imported, err := s.importPending(ctx, tx)
		if err != nil {
			return err
		}
		startSeq := tx.Seq()

		cursor := req.Cursor
		fullRequested := imported && !existed && !req.Full
		if cursor > startSeq {
			s.log.Warn().Int64("cursor", cursor).Int64("seq", startSeq).Msg("client cursor ahead of server, asking for a full sync")
			cursor, fullRequested = 0, true
		}
		full := req.Full || cursor == 0

		res, err := s.mergeBackup(ctx, tx, items, req.Device.Key(), req.DeletedCategories)
		if err != nil {
			return err
		}
		newSeq, err := tx.Apply(ctx, res, req.Device.Key())
		if err != nil {
			return err
		}

		var (
			out     *pb.Backup
			changed bool
		)
		if full {
			all, err := tx.AllItems(ctx)
			if err != nil {
				return err
			}
			out, err = backup.Render(all)
			if err != nil {
				return err
			}
			changed = true
		} else {
			out, changed, err = s.delta(ctx, tx, cursor, req.Device.Key(), res)
			if err != nil {
				return err
			}
		}

		if newSeq != startSeq {
			if err := s.refreshRenderCache(ctx, tx, full, out); err != nil {
				return err
			}
		}
		if err := tx.SetDeviceCursor(ctx, domain.DeviceCursor{Device: req.Device, Cursor: newSeq, Protocol: ProtocolV2}); err != nil {
			return err
		}

		resp = &MergeResponse{Backup: out, Cursor: newSeq, Changed: changed, FullRequested: fullRequested}
		return nil
	})
	return resp, err
}

// Snapshot renders the whole store for v2 clients. ErrNoData when nothing was ever stored.
func (s *service) Snapshot(ctx context.Context, apiKey string, cursor int64) (*Snapshot, error) {
	unlock, err := s.locks.lock(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	defer unlock()

	var snap *Snapshot
	err = s.store.Tx(ctx, apiKey, func(tx domain.SyncStoreTx) error {
		if err := s.promoteLegacy(ctx, tx); err != nil {
			return err
		}
		if _, err := s.importPending(ctx, tx); err != nil {
			return err
		}
		if !tx.Exists() {
			return ErrNoData
		}
		rc, err := s.currentRender(ctx, tx)
		if err != nil {
			return err
		}
		snap = &Snapshot{Data: rc.Data, ETag: rc.ETag, Cursor: tx.Seq()}
		return nil
	})
	return snap, err
}

// GetContent serves v1 clients their own uploaded bytes verbatim while no other device
// has written since; only then does it fall back to the rendered backup. v1 clients merge
// remote and local state themselves, so fidelity of the stored bytes matters more than
// server-side merging. ErrNoData when nothing was ever stored.
func (s *service) GetContent(ctx context.Context, apiKey string) (*Snapshot, error) {
	unlock, err := s.locks.lock(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	defer unlock()

	var snap *Snapshot
	err = s.store.Tx(ctx, apiKey, func(tx domain.SyncStoreTx) error {
		if err := s.promoteLegacy(ctx, tx); err != nil {
			return err
		}
		raw, err := tx.RawBlob(ctx)
		if err != nil {
			return err
		}
		if raw != nil && raw.Seq == tx.Seq() {
			snap = &Snapshot{Data: raw.Data, ETag: raw.ETag, Cursor: tx.Seq()}
			return nil
		}
		if !tx.Exists() {
			return ErrNoData
		}
		if _, err := s.importPending(ctx, tx); err != nil {
			return err
		}
		rc, err := s.currentRender(ctx, tx)
		if err != nil {
			return err
		}
		snap = &Snapshot{Data: rc.Data, ETag: rc.ETag, Cursor: tx.Seq()}
		return nil
	})
	return snap, err
}

// v1ETag is the etag a v1 GET would return right now, "" when there is no data yet.
func (s *service) v1ETag(ctx context.Context, tx domain.SyncStoreTx) (string, error) {
	raw, err := tx.RawBlob(ctx)
	if err != nil {
		return "", err
	}
	if raw != nil && raw.Seq == tx.Seq() {
		return raw.ETag, nil
	}
	if !tx.Exists() {
		return "", nil
	}
	if _, err := s.importPending(ctx, tx); err != nil {
		return "", err
	}
	rc, err := s.currentRender(ctx, tx)
	if err != nil {
		return "", err
	}
	return rc.ETag, nil
}

// PutContent is the v1 upload: the bytes are stored verbatim as the authoritative blob
// (v1 clients merge remote and local themselves, so the upload is already the merged
// state) and answered right away. The import into the item store runs afterwards, or on
// the first v2 read that needs it, so the response time does not depend on library size.
// ErrPreconditionFailed when ifMatch is set and differs from the current ETag.
func (s *service) PutContent(ctx context.Context, apiKey string, dev domain.DeviceInfo, ifMatch string, data []byte) (string, error) {
	s.warnLegacy(apiKey, dev)
	etag, err := s.storeRaw(ctx, apiKey, dev, ifMatch, data)
	if err != nil {
		return "", err
	}
	s.scheduleImport(apiKey)
	return etag, nil
}

func (s *service) storeRaw(ctx context.Context, apiKey string, dev domain.DeviceInfo, ifMatch string, data []byte) (string, error) {
	unlock, err := s.locks.lock(ctx, apiKey)
	if err != nil {
		return "", err
	}
	defer unlock()

	var etag string
	err = s.store.Tx(ctx, apiKey, func(tx domain.SyncStoreTx) error {
		if err := s.promoteLegacy(ctx, tx); err != nil {
			return err
		}
		if ifMatch != "" {
			cur, err := s.v1ETag(ctx, tx)
			if err != nil {
				return err
			}
			if cur == "" || cur != ifMatch {
				return ErrPreconditionFailed
			}
		}

		etag = rawETag()
		if err := tx.SetRawBlob(ctx, data, etag, tx.Seq(), true); err != nil {
			return err
		}
		device := dev.Key()
		if device == "" {
			device = deviceLegacy
		}
		return tx.SetDeviceCursor(ctx, domain.DeviceCursor{Device: domain.DeviceInfo{ID: device, Name: dev.Name}, Cursor: tx.Seq(), Protocol: ProtocolV1})
	})
	return etag, err
}

// RestoreHistory rebuilds the store from a kept payload and returns the new ETag.
func (s *service) RestoreHistory(ctx context.Context, apiKey string, id int) (*string, error) {
	data, err := s.repo.GetHistoryData(ctx, apiKey, id)
	if err != nil {
		return nil, err
	}
	b, err := backup.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPayload, err)
	}
	items, err := splitBackup(b)
	if err != nil {
		return nil, err
	}

	unlock, err := s.locks.lock(ctx, apiKey)
	if err != nil {
		return nil, err
	}
	defer unlock()

	var etag string
	err = s.store.Tx(ctx, apiKey, func(tx domain.SyncStoreTx) error {
		startSeq := tx.Seq()
		if err := tx.Clear(ctx); err != nil {
			return err
		}
		res, err := s.mergeBackup(ctx, tx, items, deviceRestore, nil)
		if err != nil {
			return err
		}
		newSeq, err := tx.Apply(ctx, res, deviceRestore)
		if err != nil {
			return err
		}
		if newSeq == startSeq {
			// an empty backup writes nothing, so the seq cannot signal the change; rewrite
			// the render in place or v2 readers keep the pre-restore content
			if err := s.refreshRenderCache(ctx, tx, false, nil); err != nil {
				return err
			}
		}
		// the restored payload becomes the raw blob so v1 devices get it byte-for-byte;
		// the render refreshes lazily on the next v2 read
		etag = rawETag()
		return tx.SetRawBlob(ctx, data, etag, tx.Seq(), false)
	})
	if err != nil {
		return nil, err
	}
	return &etag, nil
}

func (s *service) promoteLegacy(ctx context.Context, tx domain.SyncStoreTx) error {
	if tx.Exists() {
		return nil
	}
	raw, err := tx.RawBlob(ctx)
	if err != nil || raw != nil {
		return err
	}
	rc, err := tx.RenderCache(ctx)
	if err != nil {
		return err
	}
	if rc == nil || rc.RenderedSeq != nil {
		return nil
	}
	return tx.SetRawBlob(ctx, rc.Data, rc.ETag, tx.Seq(), true)
}

func (s *service) importPending(ctx context.Context, tx domain.SyncStoreTx) (bool, error) {
	raw, err := tx.RawBlob(ctx)
	if err != nil || raw == nil || !raw.Pending {
		return false, err
	}

	start := time.Now()
	b, err := backup.Decode(raw.Data)
	var items []*merge.Item
	if err == nil {
		items, err = splitBackup(b)
	}
	if err != nil {
		s.log.Error().Err(err).Msg("v1 payload cannot be decoded, serving it verbatim without importing")
		return false, tx.MarkRawCurrent(ctx, tx.Seq())
	}
	res, err := s.mergeBackup(ctx, tx, items, deviceLegacy, nil)
	if err != nil {
		return false, err
	}
	seq, err := tx.Apply(ctx, res, deviceLegacy)
	if err != nil {
		return false, err
	}
	if err := tx.MarkRawCurrent(ctx, seq); err != nil {
		return false, err
	}
	s.log.Info().
		Int("manga", len(b.BackupManga)).Int("categories", len(b.BackupCategories)).
		Int("items", len(items)).Int("written", len(res.Writes)).Dur("took", time.Since(start)).
		Msg("imported v1 upload into the item store")
	return true, nil
}

func splitBackup(b *pb.Backup) ([]*merge.Item, error) {
	items, err := backup.Split(b)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadPayload, err)
	}
	return items, nil
}

func (s *service) mergeBackup(ctx context.Context, tx domain.SyncStoreTx, items []*merge.Item, device string, deletedCategories []int64) (*merge.Result, error) {
	view, err := loadStoreView(ctx, tx, items)
	if err != nil {
		return nil, err
	}

	deleted := make([]string, 0, len(deletedCategories))
	for _, uid := range deletedCategories {
		deleted = append(deleted, "uid:"+strconv.FormatInt(uid, 10))
	}

	return merge.Merge(view, merge.Request{DeviceID: device, Items: items, DeletedCategories: deleted}), nil
}

// storeView is the merge's window on the server state: the stored items matching the
// request's keys plus every category (needed for the name fallback).
type storeView struct {
	items      map[merge.Kind]map[string]*merge.Item
	categories []*merge.Item
}

func loadStoreView(ctx context.Context, tx domain.SyncStoreTx, items []*merge.Item) (*storeView, error) {
	keys := map[merge.Kind][]string{}
	for _, it := range items {
		if it.Kind != merge.KindCategory {
			keys[it.Kind] = append(keys[it.Kind], it.Key)
		}
	}
	view := &storeView{items: map[merge.Kind]map[string]*merge.Item{}}
	for kind, ks := range keys {
		found, err := lookupItems(ctx, tx, kind, ks)
		if err != nil {
			return nil, err
		}
		view.items[kind] = found
	}
	cats, err := tx.Categories(ctx)
	if err != nil {
		return nil, err
	}
	view.categories = cats
	view.items[merge.KindCategory] = make(map[string]*merge.Item, len(cats))
	for _, c := range cats {
		view.items[merge.KindCategory][c.Key] = c
	}
	return view, nil
}

const fullScanMinKeys = 500

func lookupItems(ctx context.Context, tx domain.SyncStoreTx, kind merge.Kind, keys []string) (map[string]*merge.Item, error) {
	if len(keys) > fullScanMinKeys {
		n, err := tx.CountOfKind(ctx, kind)
		if err != nil {
			return nil, err
		}
		if n <= 2*len(keys) {
			all, err := tx.ItemsOfKind(ctx, kind)
			if err != nil {
				return nil, err
			}
			out := make(map[string]*merge.Item, len(all))
			for _, it := range all {
				out[it.Key] = it
			}
			return out, nil
		}
	}
	return tx.GetItems(ctx, kind, keys)
}

func (v *storeView) Get(kind merge.Kind, key string) *merge.Item {
	return v.items[kind][key]
}

func (v *storeView) CategoryByName(name string) *merge.Item {
	for _, c := range v.categories {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// delta builds the response for a client at cursor: everything changed since by other
// devices, the items the server won during this merge, and all live categories.
func (s *service) delta(ctx context.Context, tx domain.SyncStoreTx, cursor int64, device string, res *merge.Result) (*pb.Backup, bool, error) {
	since, err := tx.ItemsSince(ctx, cursor)
	if err != nil {
		return nil, false, err
	}

	want := map[merge.Kind]map[string]*merge.Item{}
	add := func(it *merge.Item) {
		if want[it.Kind] == nil {
			want[it.Kind] = map[string]*merge.Item{}
		}
		want[it.Kind][it.Key] = it
	}

	changed := res.ChangedForClient
	for _, it := range since {
		switch {
		case it.Kind == merge.KindCategory:
			add(it)
			if it.Seq > cursor && it.OriginDevice != device {
				changed = true
			}
		case it.OriginDevice == device:
			// the client's own writes; it already has them
		default:
			add(it)
			changed = true
		}
	}

	if len(res.ReturnKeys) > 0 {
		returned, err := tx.ItemsByKeys(ctx, res.ReturnKeys)
		if err != nil {
			return nil, false, err
		}
		for _, it := range returned {
			add(it)
		}
	}

	// a changed chapter is delivered as its manga with the manga's complete chapter list, so
	// clients can restore it exactly like a full backup
	var parents []string
	for _, ch := range want[merge.KindChapter] {
		if want[merge.KindManga][ch.ParentKey] == nil {
			parents = append(parents, ch.ParentKey)
		}
	}
	if len(parents) > 0 {
		mangas, err := tx.GetItems(ctx, merge.KindManga, parents)
		if err != nil {
			return nil, false, err
		}
		for _, m := range mangas {
			add(m)
		}
	}
	if len(want[merge.KindManga]) > 0 {
		keys := make([]string, 0, len(want[merge.KindManga]))
		for key := range want[merge.KindManga] {
			keys = append(keys, key)
		}
		chapters, err := tx.ChaptersOf(ctx, keys)
		if err != nil {
			return nil, false, err
		}
		for _, ch := range chapters {
			add(ch)
		}
	}

	var items []*merge.Item
	for _, byKey := range want {
		for _, it := range byKey {
			items = append(items, it)
		}
	}
	out, err := backup.Render(items)
	if err != nil {
		return nil, false, err
	}
	return out, changed, nil
}

// refreshRenderCache stores the full backup for v1 clients and the history. rendered can be
// passed when the caller already holds a full render.
func (s *service) refreshRenderCache(ctx context.Context, tx domain.SyncStoreTx, haveFull bool, rendered *pb.Backup) error {
	if !haveFull || rendered == nil {
		all, err := tx.AllItems(ctx)
		if err != nil {
			return err
		}
		if rendered, err = backup.Render(all); err != nil {
			return err
		}
	}
	data, err := backup.Encode(rendered)
	if err != nil {
		return err
	}
	return tx.SetRenderCache(ctx, data, etagFor(tx.Seq()), tx.Seq())
}

func (s *service) currentRender(ctx context.Context, tx domain.SyncStoreTx) (*domain.RenderCache, error) {
	rc, err := tx.RenderCache(ctx)
	if err != nil {
		return nil, err
	}
	if rc != nil && rc.RenderedSeq != nil && *rc.RenderedSeq == tx.Seq() {
		return rc, nil
	}
	if err := s.refreshRenderCache(ctx, tx, false, nil); err != nil {
		return nil, err
	}
	return tx.RenderCache(ctx)
}
