package merge

import (
	"bytes"
	"fmt"
	"slices"
	"testing"
	"time"
)

type memStore struct {
	items map[Kind]map[string]*Item
	seq   int64
	// now is the tombstone clock. syncstore.Apply stamps deleted rows with the server
	// clock, so a resurrecting write must beat a real timestamp rather than zero.
	now int64
}

func newMemStore() *memStore {
	return &memStore{items: map[Kind]map[string]*Item{}, now: time.Now().Unix()}
}

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

func (s *memStore) SameContent(a, b *Item) bool {
	return bytes.Equal(a.Payload, b.Payload) && slices.Equal(a.Refs, b.Refs)
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
		if old := s.items[it.Kind][it.Key]; old != nil && old.Deleted {
			cp.Deleted = false
		}
		s.items[it.Kind][it.Key] = &cp
	}
	for _, tb := range res.Tombstones {
		it := s.items[tb.Kind][tb.Key]
		if it == nil {
			continue
		}
		it.Deleted, it.Seq, it.OriginDevice = true, s.seq, device
		it.ModifiedAt = s.now
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

func TestSameContentTiebreakIsNoop(t *testing.T) {
	s := newMemStore()
	s.merge("A", &Item{Kind: KindManga, Key: "1|/m", Version: 3, ModifiedAt: 100, Payload: []byte("p"), Refs: []string{"uid:1"}})

	res := s.merge("B", &Item{Kind: KindManga, Key: "1|/m", Version: 3, ModifiedAt: 200, Payload: []byte("p"), Refs: []string{"uid:1"}})
	if len(res.Writes) != 0 || res.ChangedForClient {
		t.Errorf("restore echo must be a no-op: %+v", res)
	}
	if s.seq != 1 {
		t.Errorf("seq = %d, want 1", s.seq)
	}

	res = s.merge("C", &Item{Kind: KindManga, Key: "1|/m", Version: 3, ModifiedAt: 50, Payload: []byte("p"), Refs: []string{"uid:1"}})
	if len(res.Writes) != 0 || res.ChangedForClient {
		t.Errorf("an older timestamp with the same content is not worth returning: %+v", res)
	}

	res = s.merge("B", &Item{Kind: KindManga, Key: "1|/m", Version: 3, ModifiedAt: 200, Payload: []byte("q"), Refs: []string{"uid:1"}})
	if len(res.Writes) != 1 {
		t.Errorf("different content must still win the tiebreak: %+v", res)
	}

	res = s.merge("B", &Item{Kind: KindManga, Key: "1|/m", Version: 3, ModifiedAt: 300, Payload: []byte("q"), Refs: []string{"uid:2"}})
	if len(res.Writes) != 1 {
		t.Errorf("a category move must still win the tiebreak: %+v", res)
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
	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindCategory, Key: "uid:1"}}})
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
	resent := category("uid:1", "Reading", 1)
	resent.ModifiedAt = 200
	res = s.merge("A", resent)
	if len(res.Writes) != 0 || !res.ChangedForClient {
		t.Errorf("a newer timestamp must not resurrect a category: %+v", res)
	}
	// A renamed it after the delete (version bump): resurrect
	res = s.merge("A", category("uid:1", "Reading now", 2))
	if len(res.Writes) != 1 || s.Get(KindCategory, "uid:1").Deleted {
		t.Errorf("edit after delete must resurrect: %+v", res)
	}

	// deleting an unknown or already deleted category is a no-op
	res = Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindCategory, Key: "uid:404"}}})
	if len(res.Tombstones) != 0 {
		t.Errorf("tombstones = %v", res.Tombstones)
	}
}

// A client whose categories are name-keyed deletes name:<name>, while another device may
// have stored the same category under a uid: the tombstone must resolve to the stored key
// instead of silently doing nothing.
func TestCategoryNameTombstoneResolvesStoredUID(t *testing.T) {
	s := newMemStore()
	s.merge("A", category("uid:17", "Reading", 1))

	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindCategory, Key: "name:Reading"}}})
	if len(res.Tombstones) != 1 || res.Tombstones[0].Key != "uid:17" {
		t.Fatalf("tombstones = %v, want the stored uid:17 row", res.Tombstones)
	}
	s.apply(res, "B")
	if !s.Get(KindCategory, "uid:17").Deleted {
		t.Fatal("uid:17 not tombstoned")
	}

	// a name that matches no stored row is still a no-op
	res = Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindCategory, Key: "name:Missing"}}})
	if len(res.Tombstones) != 0 {
		t.Errorf("unknown name tombstone = %v", res.Tombstones)
	}
}

func TestTombstoneKindTargetsOnlyItsKind(t *testing.T) {
	// every kind uses the same key, so a helper that hardcodes KindCategory would
	// tombstone the category instead of the requested row
	seed := func() *memStore {
		s := newMemStore()
		s.merge("A", manga("k", 1, "m"), chapter("k", "k", 1, "c"), category("k", "R", 1))
		return s
	}

	for _, kind := range []Kind{KindManga, KindChapter, KindCategory} {
		t.Run(string(kind), func(t *testing.T) {
			s := seed()
			res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: kind, Key: "k"}}})
			if len(res.Tombstones) != 1 || res.Tombstones[0].Kind != kind || res.Tombstones[0].Key != "k" {
				t.Fatalf("tombstones = %v", res.Tombstones)
			}
			s.apply(res, "B")
			if got := s.Get(kind, "k"); got == nil || !got.Deleted || got.Seq != 2 || got.OriginDevice != "B" {
				t.Errorf("after tombstone: %+v", got)
			}
			for other, items := range s.items {
				if other == kind {
					continue
				}
				if it := items["k"]; it != nil && it.Deleted {
					t.Errorf("%s row was tombstoned too", other)
				}
			}

			// an already deleted row is a no-op
			res = Merge(s, Request{DeviceID: "C", Tombstones: []Tombstone{{Kind: kind, Key: "k"}}})
			if len(res.Tombstones) != 0 {
				t.Errorf("repeat tombstone = %v", res.Tombstones)
			}
		})
	}

	// an unknown key is a no-op
	s := seed()
	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindManga, Key: "missing"}}})
	if len(res.Tombstones) != 0 {
		t.Errorf("unknown key tombstone = %v", res.Tombstones)
	}
}

// Every kind can be tombstoned, and the tombstone targets only that row.
func TestTombstoneAllKinds(t *testing.T) {
	cases := []struct {
		kind Kind
		item *Item
	}{
		{KindManga, manga("m", 1, "m")},
		{KindChapter, chapter("m", "c", 1, "c")},
		{KindCategory, category("cat", "R", 1)},
		{KindSource, &Item{Kind: KindSource, Key: "k", Payload: []byte("v")}},
		{KindAppPref, &Item{Kind: KindAppPref, Key: "k", Payload: []byte("v")}},
		{KindSourcePref, &Item{Kind: KindSourcePref, Key: "k", ParentKey: "s", Payload: []byte("v")}},
		{KindExtStore, &Item{Kind: KindExtStore, Key: "k", Payload: []byte("v")}},
		{KindSavedSearch, &Item{Kind: KindSavedSearch, Key: "k", Payload: []byte("v")}},
		{KindExtra, &Item{Kind: KindExtra, Key: "k", Payload: []byte("v")}},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			s := newMemStore()
			s.merge("A", tc.item)
			res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: tc.kind, Key: tc.item.Key}}})
			if len(res.Tombstones) != 1 || res.Tombstones[0].Kind != tc.kind || res.Tombstones[0].Key != tc.item.Key {
				t.Fatalf("tombstones = %v", res.Tombstones)
			}
			s.apply(res, "B")
			if got := s.Get(tc.kind, tc.item.Key); got == nil || !got.Deleted || got.Seq != 2 || got.OriginDevice != "B" {
				t.Errorf("after tombstone: %+v", got)
			}
		})
	}
}

func TestVersionedResurrectRules(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		make func(version, modified int64, payload string) *Item
		key  string
	}{
		{"manga", KindManga, func(version, modified int64, payload string) *Item {
			return &Item{Kind: KindManga, Key: "m", Version: version, ModifiedAt: modified, Payload: []byte(payload)}
		}, "m"},
		{"chapter", KindChapter, func(version, modified int64, payload string) *Item {
			return &Item{Kind: KindChapter, Key: "c", ParentKey: "m", Version: version, ModifiedAt: modified, Payload: []byte(payload)}
		}, "c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newMemStore()
			s.now = 150 // the tombstone stamp for this case
			s.merge("A", tc.make(1, 100, "old"))
			tombstone := func() {
				t.Helper()
				res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: tc.kind, Key: tc.key}}})
				if len(res.Tombstones) != 1 {
					t.Fatalf("tombstones = %v", res.Tombstones)
				}
				s.apply(res, "B")
			}
			tombstone()

			// a stale resend (equal version, not newer) must stay deleted
			res := s.merge("A", tc.make(1, 100, "old"))
			if len(res.Writes) != 0 || !s.Get(tc.kind, tc.key).Deleted {
				t.Errorf("stale resend resurrected: %+v", res)
			}

			// equal version with a newer modified_at resurrects
			res = s.merge("A", tc.make(1, 200, "new"))
			if len(res.Writes) != 1 || s.Get(tc.kind, tc.key).Deleted {
				t.Errorf("newer modified_at did not resurrect: %+v", res)
			}

			// an equal-version touch newer than the delete resurrects even with identical content
			tombstone()
			res = s.merge("A", tc.make(1, 250, "new"))
			if len(res.Writes) != 1 || s.Get(tc.kind, tc.key).Deleted {
				t.Errorf("newer timestamp with identical content did not resurrect: %+v", res)
			}

			// a higher version resurrects even with an older timestamp
			tombstone()
			res = s.merge("A", tc.make(2, 50, "v2"))
			if len(res.Writes) != 1 || s.Get(tc.kind, tc.key).Deleted {
				t.Errorf("higher version did not resurrect: %+v", res)
			}
		})
	}
}

// A chapter re-added after its tombstone is a brand-new client row: the version restarts
// at 1 while the tombstoned server row may sit at a higher version, so only a modified_at
// newer than the deletion may resurrect it. A re-add older than the deletion stays dead.
func TestChapterReAddResurrectsTombstone(t *testing.T) {
	testReAddResurrectsTombstone(t, KindChapter, "1|/m\x1f/c")
}

func TestMangaReAddResurrectsTombstone(t *testing.T) {
	testReAddResurrectsTombstone(t, KindManga, "1|/m")
}

func testReAddResurrectsTombstone(t *testing.T, kind Kind, key string) {
	t.Helper()
	const t0 int64 = 1000
	makeItem := func(version, modified int64, payload string) *Item {
		it := &Item{Kind: kind, Key: key, Version: version, ModifiedAt: modified, Payload: []byte(payload)}
		if kind == KindChapter {
			it.ParentKey = "1|/m"
		}
		return it
	}

	s := newMemStore()
	s.now = t0 + 100 // the deletion stamp

	// stored at version 3, then deleted
	s.merge("A", makeItem(3, t0, "read"))
	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: kind, Key: key}}})
	if len(res.Tombstones) != 1 {
		t.Fatalf("tombstones = %v", res.Tombstones)
	}
	s.apply(res, "B")
	if got := s.Get(kind, key); !got.Deleted || got.ModifiedAt != t0+100 {
		t.Fatalf("after tombstone: %+v", got)
	}

	// re-add at version 1 with a timestamp newer than the deletion: resurrects (live, so
	// it renders again)
	res = s.merge("A", makeItem(1, t0+200, "read again"))
	if len(res.Writes) != 1 || s.Get(kind, key).Deleted {
		t.Fatalf("newer re-add did not resurrect: writes=%v item=%+v", res.Writes, s.Get(kind, key))
	}
	if s.Get(kind, key).Version != 1 {
		t.Errorf("resurrected version = %d, want 1", s.Get(kind, key).Version)
	}

	// delete again, then a re-add older than the deletion stays dead (does not render)
	s.now = t0 + 300
	res = Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: kind, Key: key}}})
	if len(res.Tombstones) != 1 {
		t.Fatalf("second tombstone = %v", res.Tombstones)
	}
	s.apply(res, "B")
	res = s.merge("A", makeItem(1, t0+50, "stale"))
	if len(res.Writes) != 0 || !s.Get(kind, key).Deleted {
		t.Fatalf("older re-add resurrected: writes=%v item=%+v", res.Writes, s.Get(kind, key))
	}
}

// A category is uploaded whole on every sync, so a device that has not applied the delete
// resends it unchanged and restores/reorders bump last_modified_at. A newer timestamp must
// not resurrect a deleted category; only a higher version may.
func TestCategoryTombstoneIgnoresNewerTimestamp(t *testing.T) {
	s := newMemStore()
	s.now = 150 // the tombstone stamp
	s.merge("A", category("uid:1", "Reading", 1))

	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindCategory, Key: "uid:1"}}})
	if len(res.Tombstones) != 1 {
		t.Fatalf("tombstones = %v", res.Tombstones)
	}
	s.apply(res, "B")

	// equal version with a newer modified_at stays deleted
	newer := category("uid:1", "Reading", 1)
	newer.ModifiedAt = 200
	res = s.merge("A", newer)
	if len(res.Writes) != 0 || !s.Get(KindCategory, "uid:1").Deleted {
		t.Errorf("newer timestamp resurrected a deleted category: %+v", res)
	}

	// a higher version resurrects
	res = s.merge("A", category("uid:1", "Reading", 2))
	if len(res.Writes) != 1 || s.Get(KindCategory, "uid:1").Deleted {
		t.Errorf("higher version did not resurrect: %+v", res)
	}
}

// A request carrying both the name key and the uid key for one category resolves both to the
// same row: the tombstone must be emitted once so applied does not double-count.
func TestCategoryNameAndUIDTombstonesCountOnce(t *testing.T) {
	s := newMemStore()
	s.merge("A", category("uid:17", "Reading", 1))

	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{
		{Kind: KindCategory, Key: "name:Reading"},
		{Kind: KindCategory, Key: "uid:17"},
	}})
	if len(res.Tombstones) != 1 || res.Tombstones[0].Key != "uid:17" {
		t.Fatalf("tombstones = %v, want one uid:17 tombstone", res.Tombstones)
	}
}

func TestSectionTombstoneResurrectRules(t *testing.T) {
	kinds := []Kind{KindSource, KindAppPref, KindSourcePref, KindExtStore, KindSavedSearch, KindExtra}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			s := newMemStore()
			s.merge("A", &Item{Kind: kind, Key: "k", Payload: []byte("payload")})
			res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: kind, Key: "k"}}})
			if len(res.Tombstones) != 1 {
				t.Fatalf("tombstones = %v", res.Tombstones)
			}
			s.apply(res, "B")

			// byte-identical stale re-upload stays deleted
			res = s.merge("A", &Item{Kind: kind, Key: "k", Payload: []byte("payload")})
			if len(res.Writes) != 0 || !s.Get(kind, "k").Deleted {
				t.Errorf("identical re-upload resurrected: writes=%v", res.Writes)
			}

			// a different payload resurrects
			res = s.merge("A", &Item{Kind: kind, Key: "k", Payload: []byte("payload2")})
			if len(res.Writes) != 1 || s.Get(kind, "k").Deleted {
				t.Errorf("changed payload did not resurrect: %+v", res)
			}
		})
	}
}

func TestChapterTombstoneAndResurrect(t *testing.T) {
	s := newMemStore()
	s.merge("A", manga("1|/m", 1, "m"), chapter("1|/m", "1|/m\x1f/c1", 1, "c1"))

	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindChapter, Key: "1|/m\x1f/c1"}}})
	if len(res.Tombstones) != 1 || res.Tombstones[0].Kind != KindChapter || res.Tombstones[0].Key != "1|/m\x1f/c1" {
		t.Fatalf("tombstones = %+v", res.Tombstones)
	}
	s.apply(res, "B")
	if !s.Get(KindChapter, "1|/m\x1f/c1").Deleted {
		t.Fatal("not tombstoned")
	}

	res = s.merge("A", manga("1|/m", 1, "m"), chapter("1|/m", "1|/m\x1f/c1", 1, "c1"))
	if len(res.Writes) != 0 || !res.ChangedForClient {
		t.Errorf("stale resend must not resurrect: %+v", res)
	}
	res = s.merge("A", chapter("1|/m", "1|/m\x1f/c1", 2, "c1b"))
	if len(res.Writes) != 1 || s.Get(KindChapter, "1|/m\x1f/c1").Deleted {
		t.Errorf("edit after delete must resurrect: %+v", res)
	}

	res = Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindChapter, Key: "1|/m\x1f/nope"}}})
	if len(res.Tombstones) != 0 {
		t.Errorf("tombstones = %+v", res.Tombstones)
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

func history(parent, key string, lastRead int64) *Item {
	return &Item{Kind: KindHistory, Key: key, ParentKey: parent, ModifiedAt: lastRead, Payload: fmt.Appendf(nil, "%s@%d", key, lastRead)}
}

func TestHistoryNewerReadWinsWithoutMangaBump(t *testing.T) {
	s := newMemStore()
	s.merge("A", manga("1|/m", 5, "m"), history("1|/m", "/c1", 100))

	res := s.merge("A", manga("1|/m", 5, "m"), history("1|/m", "/c1", 200), history("1|/m", "/c2", 150))

	if len(res.Writes) != 2 {
		t.Fatalf("writes = %+v, want the two history entries", res.Writes)
	}
	if got := s.Get(KindHistory, "/c1").ModifiedAt; got != 200 {
		t.Errorf("c1 last read = %d, want 200", got)
	}
}

func TestHistoryOlderReadGetsServerCopyBack(t *testing.T) {
	s := newMemStore()
	s.merge("A", history("1|/m", "/c1", 200))

	res := s.merge("B", history("1|/m", "/c1", 100))

	if len(res.Writes) != 0 || len(res.ReturnKeys[KindHistory]) != 1 {
		t.Errorf("B must receive the newer entry: %+v", res)
	}
}

// A history re-upload carries the client's millisecond last_read. Normalized to seconds it
// must not resurrect a tombstone stamped in seconds; a genuinely newer read does.
func TestHistoryTombstoneIgnoresStaleMillisecondReupload(t *testing.T) {
	const tombstone = int64(1_700_000_000)
	s := newMemStore()
	s.now = tombstone
	s.merge("A", history("1|/m", "/c1", tombstone-10))

	res := Merge(s, Request{DeviceID: "B", Tombstones: []Tombstone{{Kind: KindHistory, Key: "/c1"}}})
	if len(res.Tombstones) != 1 {
		t.Fatalf("tombstones = %v", res.Tombstones)
	}
	s.apply(res, "B")

	// 1_600_000_000_123 ms is 1_600_000_000 s, older than the 1_700_000_000 s tombstone
	res = s.merge("A", history("1|/m", "/c1", NormalizeTimestamp(1_600_000_000_123)))
	if len(res.Writes) != 0 || !s.Get(KindHistory, "/c1").Deleted {
		t.Errorf("stale millisecond re-upload resurrected the tombstone: %+v", res)
	}

	// 1_800_000_000_123 ms is 1_800_000_000 s, newer than the tombstone
	res = s.merge("A", history("1|/m", "/c1", NormalizeTimestamp(1_800_000_000_123)))
	if len(res.Writes) != 1 || s.Get(KindHistory, "/c1").Deleted {
		t.Errorf("newer millisecond read did not resurrect: %+v", res)
	}
}
