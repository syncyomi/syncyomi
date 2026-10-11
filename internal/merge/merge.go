package merge

import "bytes"

// Merge applies the client's items to the store state and decides what to write and what to send back. Absence never means deletion: only explicit tombstones delete.
//
// Rules (same as TachiyomiSY's client merge, minus the absence inference):
//   - versioned kinds (manga, chapter, category): higher version wins; on a version tie the
//     newer ModifiedAt wins, and only a full tie keeps the server copy
//   - a tombstone stands unless the client has a genuinely newer copy: a chapter resurrects on a
//     higher version or a newer ModifiedAt (see resurrect), a category only on a higher version
//   - section kinds: client wins on conflict
//   - client-only -> insert; server-only -> not touched here, returned by the delta
func Merge(store Store, req Request) *Result {
	res := &Result{ReturnKeys: map[Kind][]string{}}
	m := &merger{store: store, req: req, res: res}

	// categories first: manga refs point at them and tombstones affect refs
	for _, key := range req.DeletedCategories {
		m.tombstone(KindCategory, key)
	}
	for _, key := range req.DeletedChapters {
		m.tombstone(KindChapter, key)
	}
	for _, it := range req.Items {
		if it.Kind == KindCategory {
			m.category(it)
		}
	}
	for _, it := range req.Items {
		switch it.Kind {
		case KindCategory:
		case KindManga:
			m.versioned(it)
		case KindChapter:
			m.versioned(it)
		default:
			if IsSectionKind(it.Kind) {
				m.section(it)
			}
		}
	}

	return res
}

type merger struct {
	store Store
	req   Request
	res   *Result
	// remapped category keys: client key -> canonical server key (name fallback)
	catKeys map[string]string
}

func (m *merger) tombstone(kind Kind, key string) {
	if cur := m.store.Get(kind, key); cur != nil && !cur.Deleted {
		m.res.Tombstones = append(m.res.Tombstones, Tombstone{Kind: kind, Key: key})
	}
}

// resurrect decides what happens to a client item whose stored copy is tombstoned. The tombstone
// stands unless the client has a genuinely newer copy: a higher version (edited after the delete
// elsewhere), or a newer ModifiedAt. The latter is what lets a chapter the source dropped and
// later re-added come back: the client re-inserts it at version 1, which loses to its own
// tombstone, but a fresh fetch gives it a newer ModifiedAt.
func (m *merger) resurrect(it, cur *Item) {
	if it.Version > cur.Version || it.ModifiedAt > cur.ModifiedAt {
		m.write(it)
	} else {
		m.res.ChangedForClient = true
	}
}

func (m *merger) category(it *Item) {
	cur := m.store.Get(KindCategory, it.Key)
	if cur == nil && it.Name != "" {
		if byName := m.store.CategoryByName(it.Name); byName != nil {
			cur = byName
			m.remapCategory(it.Key, byName.Key)
			it.Key = byName.Key
		}
	}

	switch {
	case cur == nil:
		m.write(it)
	case cur.Deleted:
		// Only a higher version resurrects a category. Categories are uploaded whole on every sync,
		// so a device that has not applied the delete yet resends its copy unchanged, and restores
		// and reorders bump its timestamp; a newer ModifiedAt must not bring the tombstone back.
		if it.Version > cur.Version {
			m.write(it)
		} else {
			m.res.ChangedForClient = true
		}
	case it.Version > cur.Version:
		m.write(it)
	case it.Version < cur.Version:
		m.returnItem(cur)
	default:
		m.tiebreak(it, cur)
	}
}

func (m *merger) remapCategory(from, to string) {
	if m.catKeys == nil {
		m.catKeys = map[string]string{}
	}
	m.catKeys[from] = to
}

func (m *merger) versioned(it *Item) {
	if it.Kind == KindManga && len(it.Refs) > 0 && m.catKeys != nil {
		for i, ref := range it.Refs {
			if to, ok := m.catKeys[ref]; ok {
				it.Refs[i] = to
			}
		}
	}

	cur := m.store.Get(it.Kind, it.Key)
	switch {
	case cur == nil:
		m.write(it)
	case cur.Deleted:
		m.resurrect(it, cur)
	case it.Version > cur.Version:
		m.write(it)
	case it.Version < cur.Version:
		m.returnItem(cur)
	default:
		m.tiebreak(it, cur)
	}
}

// tiebreak resolves an equal-version conflict by ModifiedAt. Clients whose SQL triggers
// bump Version never land here with a real difference, but clients that leave Version at
// zero (v1-era builds) rely on this to propagate anything after their first upload.
func (m *merger) tiebreak(it, cur *Item) {
	// a restore re-uploading what it received differs only by timestamp
	if m.store.SameContent(it, cur) {
		return
	}
	switch {
	case it.ModifiedAt > cur.ModifiedAt:
		m.write(it)
	case it.ModifiedAt < cur.ModifiedAt:
		m.returnItem(cur)
	}
	// full tie: server keeps its copy
}

func (m *merger) section(it *Item) {
	cur := m.store.Get(it.Kind, it.Key)
	if cur == nil || !bytes.Equal(it.Payload, cur.Payload) {
		m.write(it)
	}
}

func (m *merger) write(it *Item) {
	m.res.Writes = append(m.res.Writes, it)
}

func (m *merger) returnItem(cur *Item) {
	m.res.ReturnKeys[cur.Kind] = append(m.res.ReturnKeys[cur.Kind], cur.Key)
	m.res.ChangedForClient = true
}
