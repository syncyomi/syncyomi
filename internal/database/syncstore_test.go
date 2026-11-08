package database

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/merge"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

func newTestStore(t *testing.T) (*SyncStoreRepo, *DB) {
	t.Helper()
	db := newTestDB(t)
	insertTestAPIKey(t, db, "key1")
	return &SyncStoreRepo{
		log:     zerolog.Nop(),
		db:      db,
		history: SyncRepo{log: zerolog.Nop(), db: db, historyLimit: 3},
	}, db
}

func write(items ...*merge.Item) *merge.Result {
	return &merge.Result{Writes: items, ReturnKeys: map[merge.Kind][]string{}}
}

func TestSyncStore_ApplyAndRead(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if tx.Exists() || tx.Seq() != 0 {
			t.Errorf("fresh store exists=%v seq=%d", tx.Exists(), tx.Seq())
		}
		seq, err := tx.Apply(ctx, write(
			&merge.Item{Kind: merge.KindCategory, Key: "uid:1", Name: "Read", Version: 1, Payload: []byte("c")},
			&merge.Item{Kind: merge.KindManga, Key: "1|/m", Version: 2, Refs: []string{"uid:1", "uid:2"}, Payload: []byte("m")},
			&merge.Item{Kind: merge.KindChapter, Key: "1|/m\x1f/c", ParentKey: "1|/m", Version: 3, Payload: []byte("ch")},
		), "A")
		if err != nil {
			return err
		}
		if seq != 1 {
			t.Errorf("seq = %d, want 1", seq)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if !tx.Exists() || tx.Seq() != 1 {
			t.Errorf("exists=%v seq=%d", tx.Exists(), tx.Seq())
		}
		got, err := tx.GetItems(ctx, merge.KindManga, []string{"1|/m", "missing"})
		if err != nil {
			return err
		}
		m := got["1|/m"]
		if m == nil || m.Version != 2 || len(m.Refs) != 2 || m.Refs[1] != "uid:2" || m.Seq != 1 || m.OriginDevice != "A" || !bytes.Equal(m.Payload, []byte("m")) {
			t.Errorf("manga = %+v", m)
		}
		if got["missing"] != nil {
			t.Error("unknown key returned")
		}

		all, err := tx.AllItems(ctx)
		if err != nil {
			return err
		}
		if len(all) != 3 {
			t.Errorf("all = %d items", len(all))
		}

		// second write bumps seq only for touched rows
		seq, err := tx.Apply(ctx, write(&merge.Item{Kind: merge.KindManga, Key: "1|/m", Version: 5, Payload: []byte("m2")}), "B")
		if err != nil {
			return err
		}
		if seq != 2 {
			t.Errorf("seq = %d, want 2", seq)
		}
		since, err := tx.ItemsSince(ctx, 1)
		if err != nil {
			return err
		}
		kinds := map[merge.Kind]int{}
		for _, it := range since {
			kinds[it.Kind]++
		}
		if kinds[merge.KindManga] != 1 || kinds[merge.KindChapter] != 0 || kinds[merge.KindCategory] != 1 {
			t.Errorf("ItemsSince(1) kinds = %v (categories always included)", kinds)
		}

		// no-op apply keeps seq
		seq, err = tx.Apply(ctx, write(), "B")
		if err != nil || seq != 2 {
			t.Errorf("empty apply seq=%d err=%v", seq, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncStore_TombstoneAndResurrect(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(&merge.Item{Kind: merge.KindCategory, Key: "uid:1", Name: "R", Version: 1, Payload: []byte("c")}), "A"); err != nil {
			return err
		}
		if _, err := tx.Apply(ctx, &merge.Result{Tombstones: []merge.Tombstone{{Kind: merge.KindCategory, Key: "uid:1"}}, ReturnKeys: map[merge.Kind][]string{}}, "B"); err != nil {
			return err
		}
		cats, err := tx.Categories(ctx)
		if err != nil {
			return err
		}
		if len(cats) != 1 || !cats[0].Deleted || cats[0].Seq != 2 || cats[0].OriginDevice != "B" {
			t.Errorf("after tombstone: %+v", cats[0])
		}
		if _, err := tx.Apply(ctx, write(&merge.Item{Kind: merge.KindCategory, Key: "uid:1", Name: "R2", Version: 2, Payload: []byte("c2")}), "A"); err != nil {
			return err
		}
		cats, _ = tx.Categories(ctx)
		if cats[0].Deleted || cats[0].Name != "R2" {
			t.Errorf("after resurrect: %+v", cats[0])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// mergeStore adapts a live store transaction to merge.Store, so a test drives the
// production path (merge.Merge -> tx.Apply) instead of hand-building a Result.
type mergeStore struct {
	ctx context.Context
	tx  domain.SyncStoreTx
}

func (s *mergeStore) Get(kind merge.Kind, key string) *merge.Item {
	items, err := s.tx.GetItems(s.ctx, kind, []string{key})
	if err != nil {
		return nil
	}
	return items[key]
}

func (s *mergeStore) CategoryByName(name string) *merge.Item {
	cats, err := s.tx.Categories(s.ctx)
	if err != nil {
		return nil
	}
	for _, c := range cats {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func (s *mergeStore) SameContent(a, b *merge.Item) bool {
	return bytes.Equal(a.Payload, b.Payload) && slices.Equal(a.Refs, b.Refs)
}

func renderChapterCount(t *testing.T, tx domain.SyncStoreTx) int {
	t.Helper()
	all, err := tx.AllItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := backup.Render(all)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.BackupManga) != 1 {
		t.Fatalf("rendered manga = %d, want 1", len(b.BackupManga))
	}
	return len(b.BackupManga[0].Chapters)
}

// A chapter re-added after a tombstone restarts at version 1. Through the real Apply path
// the deletion stamps modified_at with the server clock, so a newer re-add must resurrect
// the row and render, while an older one must stay deleted and not render.
func TestSyncStore_ChapterReAddResurrectsTombstone(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	const (
		mangaKey         = "1|/m"
		chapterKey       = "1|/m\x1f/c"
		t0         int64 = 1000
	)
	mangaItem := &merge.Item{Kind: merge.KindManga, Key: mangaKey, Version: 1, ModifiedAt: t0, Payload: pbPayload(t, &pb.BackupManga{Source: 1, Url: "/m", Title: "/m"})}
	chapterItem := &merge.Item{Kind: merge.KindChapter, Key: chapterKey, ParentKey: mangaKey, Version: 3, ModifiedAt: t0, Payload: pbPayload(t, &pb.BackupChapter{Url: "/c", Name: "/c"})}

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		view := &mergeStore{ctx: ctx, tx: tx}
		if _, err := tx.Apply(ctx, write(mangaItem, chapterItem), "A"); err != nil {
			return err
		}

		tombstone := func() (int64, error) {
			res := merge.Merge(view, merge.Request{DeviceID: "B", Tombstones: []merge.Tombstone{{Kind: merge.KindChapter, Key: chapterKey}}})
			if _, err := tx.Apply(ctx, res, "B"); err != nil {
				return 0, err
			}
			items, err := tx.GetItems(ctx, merge.KindChapter, []string{chapterKey})
			if err != nil {
				return 0, err
			}
			if !items[chapterKey].Deleted {
				t.Fatalf("chapter not tombstoned: %+v", items[chapterKey])
			}
			return items[chapterKey].ModifiedAt, nil
		}

		deletedAt, err := tombstone()
		if err != nil {
			return err
		}
		now := time.Now().Unix()
		if deletedAt < now-5 || deletedAt > now+5 {
			t.Fatalf("tombstone modified_at = %d, want the server clock near %d", deletedAt, now)
		}
		if got := renderChapterCount(t, tx); got != 0 {
			t.Errorf("tombstoned chapter rendered: %d chapters", got)
		}

		// version 1 with a newer timestamp resurrects and renders
		readd := &merge.Item{Kind: merge.KindChapter, Key: chapterKey, ParentKey: mangaKey, Version: 1, ModifiedAt: deletedAt + 1, Payload: pbPayload(t, &pb.BackupChapter{Url: "/c", Name: "/c", Read: true})}
		res := merge.Merge(view, merge.Request{DeviceID: "A", Items: []*merge.Item{readd}})
		if len(res.Writes) != 1 {
			t.Fatalf("re-add writes = %v, want 1", res.Writes)
		}
		if _, err := tx.Apply(ctx, res, "A"); err != nil {
			return err
		}
		if got := renderChapterCount(t, tx); got != 1 {
			t.Errorf("resurrected chapter did not render: %d chapters", got)
		}

		// a re-add older than the fresh deletion stays dead and does not render
		if _, err := tombstone(); err != nil {
			return err
		}
		stale := &merge.Item{Kind: merge.KindChapter, Key: chapterKey, ParentKey: mangaKey, Version: 1, ModifiedAt: deletedAt - 100, Payload: pbPayload(t, &pb.BackupChapter{Url: "/c", Name: "/c", Read: true})}
		res = merge.Merge(view, merge.Request{DeviceID: "A", Items: []*merge.Item{stale}})
		if len(res.Writes) != 0 {
			t.Fatalf("older re-add wrote: %v", res.Writes)
		}
		if _, err := tx.Apply(ctx, res, "A"); err != nil {
			return err
		}
		if got := renderChapterCount(t, tx); got != 0 {
			t.Errorf("older re-add rendered: %d chapters", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func pbPayload(t *testing.T, m proto.Message) []byte {
	t.Helper()
	b, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// A tombstone names its kind, so Apply marks the requested row and only that one.
func TestSyncStore_TombstoneKinds(t *testing.T) {
	const (
		mangaKey   = "1|/m"
		chapterKey = "1|/m\x1f/c"
		catKey     = "uid:7"
	)
	mangaItem := &merge.Item{Kind: merge.KindManga, Key: mangaKey, Payload: pbPayload(t, &pb.BackupManga{Source: 1, Url: "/m", Title: "/m"})}

	cases := []struct {
		name    string
		kind    merge.Kind
		key     string
		seed    []*merge.Item
		omitted func(t *testing.T, b *pb.Backup)
	}{
		{
			name: "manga",
			kind: merge.KindManga,
			key:  mangaKey,
			seed: []*merge.Item{mangaItem},
			omitted: func(t *testing.T, b *pb.Backup) {
				if len(b.BackupManga) != 0 {
					t.Errorf("render kept the tombstoned manga: %v", b.BackupManga)
				}
			},
		},
		{
			name: "chapter",
			kind: merge.KindChapter,
			key:  chapterKey,
			seed: []*merge.Item{
				mangaItem,
				{Kind: merge.KindChapter, Key: chapterKey, ParentKey: mangaKey, Payload: pbPayload(t, &pb.BackupChapter{Url: "/c", Name: "/c"})},
			},
			omitted: func(t *testing.T, b *pb.Backup) {
				if len(b.BackupManga) != 1 || len(b.BackupManga[0].Chapters) != 0 {
					t.Errorf("render kept the tombstoned chapter: %v", b.BackupManga)
				}
			},
		},
		{
			name: "category",
			kind: merge.KindCategory,
			key:  catKey,
			seed: []*merge.Item{{Kind: merge.KindCategory, Key: catKey, Name: "Reading", Payload: pbPayload(t, &pb.BackupCategory{Name: "Reading", Uid: 7})}},
			omitted: func(t *testing.T, b *pb.Backup) {
				if len(b.BackupCategories) != 0 {
					t.Errorf("render kept the tombstoned category: %v", b.BackupCategories)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, _ := newTestStore(t)
			ctx := context.Background()

			err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
				if _, err := tx.Apply(ctx, write(tc.seed...), "A"); err != nil {
					return err
				}
				seq, err := tx.Apply(ctx, &merge.Result{Tombstones: []merge.Tombstone{{Kind: tc.kind, Key: tc.key}}}, "B")
				if err != nil {
					return err
				}
				if seq != 2 {
					t.Errorf("apply seq = %d, want 2 (one increment per Apply)", seq)
				}

				got, err := tx.GetItems(ctx, tc.kind, []string{tc.key})
				if err != nil {
					return err
				}
				it := got[tc.key]
				if it == nil {
					t.Fatalf("%s %s missing after tombstone", tc.kind, tc.key)
				}
				if !it.Deleted || it.Seq != 2 || it.OriginDevice != "B" {
					t.Errorf("tombstoned row = kind=%s key=%s deleted=%v seq=%d origin=%s", it.Kind, it.Key, it.Deleted, it.Seq, it.OriginDevice)
				}
				now := time.Now().Unix()
				if it.ModifiedAt <= 0 || it.ModifiedAt > now || it.ModifiedAt < now-60 {
					t.Errorf("modified_at = %d, want unix seconds near %d", it.ModifiedAt, now)
				}

				since, err := tx.ItemsSince(ctx, 1)
				if err != nil {
					return err
				}
				var seen *merge.Item
				for _, s := range since {
					if s.Kind == tc.kind && s.Key == tc.key {
						seen = s
					}
				}
				if seen == nil || seen.Seq != 2 {
					t.Errorf("ItemsSince(1) = %+v, want the tombstoned row at seq 2", seen)
				}

				all, err := tx.AllItems(ctx)
				if err != nil {
					return err
				}
				b, err := backup.Render(all)
				if err != nil {
					return err
				}
				tc.omitted(t, b)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// An unknown key matches no row, so a tombstone for it leaves the store content alone.
func TestSyncStore_TombstoneUnknownKeyIsNoop(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(&merge.Item{Kind: merge.KindManga, Key: "1|/m", ModifiedAt: 100, Payload: []byte("m")}), "A"); err != nil {
			return err
		}
		if _, err := tx.Apply(ctx, &merge.Result{Tombstones: []merge.Tombstone{{Kind: merge.KindManga, Key: "missing"}}}, "B"); err != nil {
			return err
		}
		all, err := tx.AllItems(ctx)
		if err != nil {
			return err
		}
		if len(all) != 1 || all[0].Deleted || all[0].Seq != 1 || all[0].OriginDevice != "A" || all[0].ModifiedAt != 100 {
			t.Errorf("unknown key tombstone touched rows: %+v", all)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A deletion matching no row must not advance the seq: otherwise a no-op request wakes
// every device.
func TestSyncStore_NoOpTombstoneKeepsSeq(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(&merge.Item{Kind: merge.KindManga, Key: "1|/m", Payload: []byte("m")}), "A"); err != nil {
			return err
		}
		seq, err := tx.Apply(ctx, &merge.Result{Tombstones: []merge.Tombstone{{Kind: merge.KindManga, Key: "missing"}}}, "B")
		if err != nil {
			return err
		}
		if seq != 1 || tx.Seq() != 1 {
			t.Errorf("seq = %d tx.seq = %d, want 1 (no bump for an unmatched tombstone)", seq, tx.Seq())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// the persisted state is unchanged too
	if err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if tx.Seq() != 1 {
			t.Errorf("reloaded seq = %d, want 1", tx.Seq())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Several tombstones in one Apply share a single seq increment.
func TestSyncStore_MultipleTombstonesBumpSeqOnce(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(
			&merge.Item{Kind: merge.KindManga, Key: "m1", Payload: []byte("a")},
			&merge.Item{Kind: merge.KindManga, Key: "m2", Payload: []byte("b")},
			&merge.Item{Kind: merge.KindSource, Key: "s", Payload: []byte("c")},
		), "A"); err != nil {
			return err
		}
		seq, err := tx.Apply(ctx, &merge.Result{Tombstones: []merge.Tombstone{
			{Kind: merge.KindManga, Key: "m1"},
			{Kind: merge.KindManga, Key: "m2"},
			{Kind: merge.KindSource, Key: "s"},
		}}, "B")
		if err != nil {
			return err
		}
		if seq != 2 {
			t.Errorf("seq = %d, want 2 (one increment for the whole call)", seq)
		}
		for _, kind := range []merge.Kind{merge.KindManga, merge.KindSource} {
			keys := []string{"m1", "m2"}
			if kind == merge.KindSource {
				keys = []string{"s"}
			}
			got, err := tx.GetItems(ctx, kind, keys)
			if err != nil {
				return err
			}
			for _, key := range keys {
				it := got[key]
				if it == nil || !it.Deleted || it.Seq != 2 || it.OriginDevice != "B" {
					t.Errorf("%s %s after tombstone = %+v", kind, key, it)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncStore_RenderCacheAndHistory(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	// a pre-v2 blob has no rendered_seq
	if _, err := db.handler.Exec(`INSERT INTO sync_data (user_api_key, data, data_etag) VALUES ('key1', $1, 'uuid=old')`, []byte{1}); err != nil {
		t.Fatal(err)
	}

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		rc, err := tx.RenderCache(ctx)
		if err != nil {
			return err
		}
		if rc == nil || rc.RenderedSeq != nil || rc.ETag != "uuid=old" {
			t.Errorf("legacy cache = %+v", rc)
		}
		if err := tx.SetRenderCache(ctx, []byte("rendered"), "seq=3", 3); err != nil {
			return err
		}
		rc, _ = tx.RenderCache(ctx)
		if rc == nil || rc.RenderedSeq == nil || *rc.RenderedSeq != 3 || !bytes.Equal(rc.Data, []byte("rendered")) {
			t.Errorf("cache = %+v", rc)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	history, err := store.history.ListHistory(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ETag != "seq=3" {
		t.Errorf("history = %+v", history)
	}

	// nothing stored at all
	insertTestAPIKey(t, db, "key2")
	err = store.Tx(ctx, "key2", func(tx domain.SyncStoreTx) error {
		rc, err := tx.RenderCache(ctx)
		if err != nil || rc != nil {
			t.Errorf("empty cache = %+v, %v", rc, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncStore_RawBlob(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		raw, err := tx.RawBlob(ctx)
		if err != nil || raw != nil {
			t.Errorf("empty raw blob = %+v, %v", raw, err)
		}
		if err := tx.SetRawBlob(ctx, []byte("client bytes"), "uuid=a", 2, true); err != nil {
			return err
		}
		raw, err = tx.RawBlob(ctx)
		if err != nil {
			return err
		}
		if raw == nil || !bytes.Equal(raw.Data, []byte("client bytes")) || raw.ETag != "uuid=a" || raw.Seq != 2 || !raw.Pending {
			t.Errorf("raw blob = %+v", raw)
		}

		// renders and raw blobs live in the same row without clobbering each other
		if err := tx.SetRenderCache(ctx, []byte("rendered"), "seq=3", 3); err != nil {
			return err
		}
		raw, _ = tx.RawBlob(ctx)
		if raw == nil || !bytes.Equal(raw.Data, []byte("client bytes")) || raw.Seq != 2 {
			t.Errorf("raw blob after render = %+v", raw)
		}
		if err := tx.SetRawBlob(ctx, []byte("newer"), "uuid=b", 4, true); err != nil {
			return err
		}
		rc, _ := tx.RenderCache(ctx)
		if rc == nil || !bytes.Equal(rc.Data, []byte("rendered")) || *rc.RenderedSeq != 3 {
			t.Errorf("render cache after raw write = %+v", rc)
		}

		if err := tx.MarkRawCurrent(ctx, 9); err != nil {
			return err
		}
		raw, _ = tx.RawBlob(ctx)
		if raw == nil || raw.Seq != 9 || raw.Pending || !bytes.Equal(raw.Data, []byte("newer")) {
			t.Errorf("raw blob after mark = %+v", raw)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// raw uploads are recorded in the history
	history, err := store.history.ListHistory(ctx, "key1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 || history[0].ETag != "uuid=b" {
		t.Errorf("history = %+v", history)
	}

	// a row whose raw columns are NULL (render-only key) yields nil
	insertTestAPIKey(t, db, "key2")
	err = store.Tx(ctx, "key2", func(tx domain.SyncStoreTx) error {
		if err := tx.SetRenderCache(ctx, []byte("r"), "seq=1", 1); err != nil {
			return err
		}
		raw, err := tx.RawBlob(ctx)
		if err != nil || raw != nil {
			t.Errorf("render-only raw blob = %+v, %v", raw, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncStore_DeviceCursor(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if err := tx.SetDeviceCursor(ctx, domain.DeviceCursor{Device: domain.DeviceInfo{ID: "d1", Name: "Phone"}, Cursor: 4, Protocol: "v2"}); err != nil {
			return err
		}
		return tx.SetDeviceCursor(ctx, domain.DeviceCursor{Device: domain.DeviceInfo{ID: "d1"}, Cursor: 5, Protocol: "v2"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var name, protocol string
	var cursor int64
	if err := store.db.handler.QueryRow(`SELECT device_name, last_cursor, protocol FROM sync_device WHERE user_api_key = 'key1' AND device_id = 'd1'`).Scan(&name, &cursor, &protocol); err != nil {
		t.Fatal(err)
	}
	if name != "Phone" || cursor != 5 || protocol != "v2" {
		t.Errorf("device = %s %d %s", name, cursor, protocol)
	}
}

func TestSyncStore_RollbackOnError(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(&merge.Item{Kind: merge.KindManga, Key: "1|/m", Payload: []byte("m")}), "A"); err != nil {
			return err
		}
		return fmt.Errorf("boom")
	})
	if err == nil {
		t.Fatal("error swallowed")
	}
	store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if tx.Exists() || tx.Seq() != 0 {
			t.Errorf("rolled back tx left state exists=%v seq=%d", tx.Exists(), tx.Seq())
		}
		return nil
	})
}

func TestSyncStore_LargeBatch(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	var items []*merge.Item
	for i := 0; i < 1234; i++ {
		items = append(items, &merge.Item{Kind: merge.KindChapter, Key: fmt.Sprintf("1|/m\x1f/c%d", i), ParentKey: "1|/m", Payload: []byte{byte(i)}})
	}
	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(items...), "A"); err != nil {
			return err
		}
		keys := make([]string, 0, len(items))
		for _, it := range items {
			keys = append(keys, it.Key)
		}
		got, err := tx.GetItems(ctx, merge.KindChapter, keys)
		if err != nil {
			return err
		}
		if len(got) != len(items) {
			t.Errorf("got %d of %d", len(got), len(items))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncStore_KindQueries(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(
			&merge.Item{Kind: merge.KindManga, Key: "1|/a", Payload: []byte("a")},
			&merge.Item{Kind: merge.KindManga, Key: "1|/b", Payload: []byte("b")},
			&merge.Item{Kind: merge.KindChapter, Key: "1|/a\x1f/c", ParentKey: "1|/a", Payload: []byte("c")},
		), "A"); err != nil {
			return err
		}
		for kind, want := range map[merge.Kind]int{merge.KindManga: 2, merge.KindChapter: 1, merge.KindCategory: 0} {
			n, err := tx.CountOfKind(ctx, kind)
			if err != nil {
				return err
			}
			items, err := tx.ItemsOfKind(ctx, kind)
			if err != nil {
				return err
			}
			if n != want || len(items) != want {
				t.Errorf("%s: count=%d items=%d, want %d", kind, n, len(items), want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSyncStore_ReadTxDoesNotWaitForWriters(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	err := store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
		if _, err := tx.Apply(ctx, write(&merge.Item{Kind: merge.KindManga, Key: "1|/m", Payload: []byte("m")}), "A"); err != nil {
			return err
		}
		return tx.SetRawBlob(ctx, []byte("raw"), "uuid=a", 1, true)
	})
	if err != nil {
		t.Fatal(err)
	}

	release := make(chan struct{})
	writing := make(chan struct{})
	go func() {
		_ = store.Tx(ctx, "key1", func(tx domain.SyncStoreTx) error {
			close(writing)
			<-release
			return nil
		})
	}()
	<-writing
	defer close(release)

	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	err = store.ReadTx(readCtx, "key1", func(tx domain.SyncStoreReader) error {
		raw, err := tx.RawBlob(readCtx)
		if err != nil {
			return err
		}
		if !tx.Exists() || tx.Seq() != 1 || raw == nil || !raw.Pending || raw.Seq != 1 {
			t.Errorf("read tx state: exists=%v seq=%d raw=%+v", tx.Exists(), tx.Seq(), raw)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx while a writer holds the store: %v", err)
	}
}
