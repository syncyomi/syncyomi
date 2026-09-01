package merge

import (
	"testing"
)

type memStore struct {
	items map[Kind]map[string]*Item
	seq   int64
}

func newMemStore() *memStore { return &memStore{items: map[Kind]map[string]*Item{}} }

func (s *memStore) Get(kind Kind, key string) *Item {
	return s.items[kind][key]
}

func (s *memStore) CategoryByName(name string) *Item {
	for _, it := range s.items[KindCategory] {
		if it.Name == name {
			return it
		}
	}
	return nil
}

// apply plays the role of the database: it assigns seq/origin and stores writes and tombstones.
func (s *memStore) apply(res *Result, device string) {
	if len(res.Writes) == 0 && len(res.Tombstones) == 0 {
		return
	}
	s.seq++
	for _, it := range res.Writes {
		cp := *it
		cp.Seq, cp.OriginDevice = s.seq, device
		if s.items[it.Kind] == nil {
			s.items[it.Kind] = map[string]*Item{}
		}
		if old := s.items[it.Kind][it.Key]; old != nil && old.Deleted && it.Kind == KindCategory {
			cp.Deleted = false
		}
		s.items[it.Kind][it.Key] = &cp
	}
	for _, key := range res.Tombstones {
		it := s.items[KindCategory][key]
		it.Deleted, it.Seq, it.OriginDevice = true, s.seq, device
	}
}

func (s *memStore) merge(device string, items ...*Item) *Result {
	res := Merge(s, Request{DeviceID: device, Items: items})
	s.apply(res, device)
	return res
}

func manga(key string, version int64, payload string, refs ...string) *Item {
	return &Item{Kind: KindManga, Key: key, Version: version, Payload: []byte(payload), Refs: refs}
}

func chapter(parent, key string, version int64, payload string) *Item {
	return &Item{Kind: KindChapter, Key: key, ParentKey: parent, Version: version, Payload: []byte(payload)}
}

func category(key, name string, version int64) *Item {
	return &Item{Kind: KindCategory, Key: key, Name: name, Version: version, Payload: []byte(name)}
}

func TestClientOnlyIsInsertedNeverDropped(t *testing.T) {
	s := newMemStore()
	res := s.merge("A", manga("1|/m", 1, "p"))
	if len(res.Writes) != 1 || res.ChangedForClient {
		t.Fatalf("res = %+v", res)
	}
	if s.Get(KindManga, "1|/m") == nil {
		t.Fatal("not stored")
	}
}

// The #1635 race: B syncs between A adding M and A uploading it. With absence-based
// deletion B would have dropped M; here M is simply unknown to B until the delta delivers it.
func TestTwoDeviceRace(t *testing.T) {
	s := newMemStore()
	// B syncs first with its own library
	s.merge("B", manga("1|/b", 1, "b"))
	// A adds M and uploads
	s.merge("A", manga("1|/b", 1, "b"), manga("1|/m", 1, "m"))
	// B syncs again without M: M must survive
	res := s.merge("B", manga("1|/b", 1, "b"))
	if len(res.Writes) != 0 {
		t.Errorf("unexpected writes %+v", res.Writes)
	}
	if s.Get(KindManga, "1|/m") == nil {
		t.Fatal("M was dropped")
	}
	// A syncs again: still there, and idempotent
	res = s.merge("A", manga("1|/b", 1, "b"), manga("1|/m", 1, "m"))
	if len(res.Writes) != 0 || res.ChangedForClient {
		t.Errorf("A resync should be a no-op, got %+v", res)
	}
}

func TestHigherVersionWinsTieServerKeeps(t *testing.T) {
	s := newMemStore()
	s.merge("A", manga("1|/m", 2, "a2"))

	res := s.merge("B", manga("1|/m", 1, "b1"))
	if len(res.Writes) != 0 || !res.ChangedForClient || len(res.ReturnKeys[KindManga]) != 1 {
		t.Errorf("older client version must lose and be returned: %+v", res)
	}
	if string(s.Get(KindManga, "1|/m").Payload) != "a2" {
		t.Error("server copy overwritten by older version")
	}

	res = s.merge("B", manga("1|/m", 2, "b2"))
	if len(res.Writes) != 0 || res.ChangedForClient {
		t.Errorf("equal version must be a no-op: %+v", res)
	}

	res = s.merge("B", manga("1|/m", 3, "b3"))
	if len(res.Writes) != 1 || string(s.Get(KindManga, "1|/m").Payload) != "b3" {
		t.Errorf("newer version must win: %+v", res)
	}
}

// Clients that never bump Version (v1-era builds leave it at 0) rely on ModifiedAt to
// break the tie; otherwise their first upload would win forever.
func TestEqualVersionModifiedAtTiebreak(t *testing.T) {
	s := newMemStore()
	old := &Item{Kind: KindManga, Key: "1|/m", Version: 0, ModifiedAt: 100, Payload: []byte("old")}
	s.merge("A", old)

	// same version, newer timestamp: client wins
	newer := &Item{Kind: KindManga, Key: "1|/m", Version: 0, ModifiedAt: 200, Payload: []byte("new")}
	res := s.merge("B", newer)
	if len(res.Writes) != 1 || string(s.Get(KindManga, "1|/m").Payload) != "new" {
		t.Errorf("newer ModifiedAt must win on a version tie: %+v", res)
	}

	// same version, older timestamp: server wins and the item is returned
	res = s.merge("A", old)
	if len(res.Writes) != 0 || !res.ChangedForClient || len(res.ReturnKeys[KindManga]) != 1 {
		t.Errorf("older ModifiedAt must lose and be returned: %+v", res)
	}

	// full tie stays a no-op
	res = s.merge("A", newer)
	if len(res.Writes) != 0 || res.ChangedForClient {
		t.Errorf("full tie must be a no-op: %+v", res)
	}

	// a higher version still beats a newer timestamp
	res = s.merge("A", &Item{Kind: KindManga, Key: "1|/m", Version: 1, ModifiedAt: 50, Payload: []byte("v1")})
	if len(res.Writes) != 1 || string(s.Get(KindManga, "1|/m").Payload) != "v1" {
		t.Errorf("version outranks ModifiedAt: %+v", res)
	}

	// same rule for categories
	s.merge("A", &Item{Kind: KindCategory, Key: "uid:1", Name: "Read", Version: 0, ModifiedAt: 100, Payload: []byte("a")})
	res = s.merge("B", &Item{Kind: KindCategory, Key: "uid:1", Name: "Read", Version: 0, ModifiedAt: 200, Payload: []byte("b")})
	if len(res.Writes) != 1 || string(s.Get(KindCategory, "uid:1").Payload) != "b" {
		t.Errorf("category tiebreak: %+v", res)
	}
}

func TestUnfavoriteTravelsAsVersionBump(t *testing.T) {
	s := newMemStore()
	s.merge("A", manga("1|/m", 5, "fav"))
	s.merge("B", manga("1|/m", 6, "unfav"))
	if string(s.Get(KindManga, "1|/m").Payload) != "unfav" {
		t.Error("unfavorite (version bump) not stored")
	}
	res := s.merge("A", manga("1|/m", 5, "fav"))
	if !res.ChangedForClient || len(res.ReturnKeys[KindManga]) != 1 {
		t.Errorf("A must receive the newer copy: %+v", res)
	}
}

func TestChaptersMergeIndependentlyOfManga(t *testing.T) {
	s := newMemStore()
	s.merge("A", manga("1|/m", 1, "a"), chapter("1|/m", "/c1", 1, "unread"), chapter("1|/m", "/c2", 1, "unread"))
	// B read c1 (version 2) and knows nothing about c2 (chapter sync off)
	res := s.merge("B", manga("1|/m", 1, "a"), chapter("1|/m", "/c1", 2, "read"))
	if len(res.Writes) != 1 || res.Writes[0].Key != "/c1" {
		t.Errorf("writes = %+v", res.Writes)
	}
	if s.Get(KindChapter, "/c2") == nil {
		t.Error("c2 dropped although B simply did not send chapters")
	}
	// A still has c1 unread at version 1: gets the read state back
	res = s.merge("A", chapter("1|/m", "/c1", 1, "unread"))
	if len(res.ReturnKeys[KindChapter]) != 1 {
		t.Errorf("A should receive c1: %+v", res)
	}
}

func TestCategoryTombstoneAndResurrect(t *testing.T) {
	s := newMemStore()
	s.merge("A", category("uid:1", "Reading", 1))
	res := Merge(s, Request{DeviceID: "B", DeletedCategories: []string{"uid:1"}})
	if len(res.Tombstones) != 1 {
		t.Fatalf("tombstones = %v", res.Tombstones)
	}
	s.apply(res, "B")
	if !s.Get(KindCategory, "uid:1").Deleted {
		t.Fatal("not tombstoned")
	}

	// A resends the category unchanged: tombstone stands, A is told to change
	res = s.merge("A", category("uid:1", "Reading", 1))
	if len(res.Writes) != 0 || !res.ChangedForClient {
		t.Errorf("stale resend must not resurrect: %+v", res)
	}
	// A renamed it after the delete (version bump): resurrect
	res = s.merge("A", category("uid:1", "Reading now", 2))
	if len(res.Writes) != 1 || s.Get(KindCategory, "uid:1").Deleted {
		t.Errorf("edit after delete must resurrect: %+v", res)
	}

	// deleting an unknown or already deleted category is a no-op
	res = Merge(s, Request{DeviceID: "B", DeletedCategories: []string{"uid:404"}})
	if len(res.Tombstones) != 0 {
		t.Errorf("tombstones = %v", res.Tombstones)
	}
}

func TestCategoryNameFallbackRemapsMangaRefs(t *testing.T) {
	s := newMemStore()
	s.merge("A", category("uid:1", "Reading", 1))
	// B has the same category under a different uid (created before uids existed)
	res := s.merge("B", category("uid:2", "Reading", 1), manga("1|/m", 1, "m", "uid:2"))
	if len(res.Writes) != 1 || res.Writes[0].Kind != KindManga {
		t.Fatalf("writes = %+v", res.Writes)
	}
	if refs := s.Get(KindManga, "1|/m").Refs; len(refs) != 1 || refs[0] != "uid:1" {
		t.Errorf("refs = %v, want remapped to uid:1", refs)
	}
	if s.Get(KindCategory, "uid:2") != nil {
		t.Error("duplicate category created")
	}
}

func TestSectionsClientWins(t *testing.T) {
	s := newMemStore()
	pref := func(v string) *Item { return &Item{Kind: KindAppPref, Key: "theme", Payload: []byte(v)} }
	s.merge("A", pref("dark"))
	res := s.merge("B", pref("light"))
	if len(res.Writes) != 1 || string(s.Get(KindAppPref, "theme").Payload) != "light" {
		t.Errorf("client must win: %+v", res)
	}
	res = s.merge("B", pref("light"))
	if len(res.Writes) != 0 {
		t.Errorf("identical section item rewritten: %+v", res)
	}
}

func TestRetryIsIdempotent(t *testing.T) {
	s := newMemStore()
	items := func() []*Item {
		return []*Item{manga("1|/m", 1, "m"), chapter("1|/m", "/c", 1, "c"), category("uid:1", "R", 1), {Kind: KindSource, Key: "1", Payload: []byte("src")}}
	}
	s.merge("A", items()...)
	seq := s.seq
	res := s.merge("A", items()...)
	if len(res.Writes) != 0 || len(res.Tombstones) != 0 || res.ChangedForClient || s.seq != seq {
		t.Errorf("retry not idempotent: %+v seq %d -> %d", res, seq, s.seq)
	}
}
