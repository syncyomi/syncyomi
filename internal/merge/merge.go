package merge

import "bytes"

// Merge applies the client's items to the store state and decides what to write and what to send back. Absence never means deletion: only explicit tombstones delete.
//
// Rules (same as TachiyomiSY's client merge, minus the absence inference):
//   - versioned kinds (manga, chapter, category): higher version wins, tie -> server keeps
//   - section kinds: client wins on conflict
//   - client-only -> insert; server-only -> not touched here, returned by the delta
func Merge(store Store, req Request) *Result {
	res := &Result{ReturnKeys: map[Kind][]string{}}
	m := &merger{store: store, req: req, res: res}

	// categories first: manga refs point at them and tombstones affect refs
	for _, key := range req.DeletedCategories {
		m.tombstone(key)
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

func (m *merger) tombstone(key string) {
	if cur := m.store.Get(KindCategory, key); cur != nil && !cur.Deleted {
		m.res.Tombstones = append(m.res.Tombstones, key)
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
		if it.Version > cur.Version {
			m.write(it) // edited after the delete elsewhere: resurrect
		} else {
			m.res.ChangedForClient = true
		}
	case it.Version > cur.Version:
		m.write(it)
	case it.Version < cur.Version:
		m.returnItem(cur)
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
	case it.Version > cur.Version:
		m.write(it)
	case it.Version < cur.Version:
		m.returnItem(cur)
	}
	// equal version: server keeps its copy; differences without a version bump are
	// not synced (the client's SQL triggers bump the version for anything that matters)
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

func sameRefs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, x := range a {
		seen[x]++
	}
	for _, x := range b {
		if seen[x] == 0 {
			return false
		}
		seen[x]--
	}
	return true
}
