package database

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/merge"
	"github.com/rs/zerolog"
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
		if _, err := tx.Apply(ctx, &merge.Result{Tombstones: []string{"uid:1"}, ReturnKeys: map[merge.Kind][]string{}}, "B"); err != nil {
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

func TestSyncStore_RenderCacheAndHistory(t *testing.T) {
	store, db := newTestStore(t)
	ctx := context.Background()

	// a pre-v2 blob has no rendered_seq
	if _, err := db.handler.Exec(`INSERT INTO sync_data (user_api_key, data, data_etag) VALUES ('key1', X'01', 'uuid=old')`); err != nil {
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
