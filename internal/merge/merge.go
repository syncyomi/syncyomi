package merge

import (
	"bytes"
	"strings"
)

// Merge applies the client's items to the store state and decides what to write and what to send back. Absence never means deletion: only explicit tombstones delete.
//
// Rules (same as TachiyomiSY's client merge, minus the absence inference):
//   - versioned kinds (manga, chapter, category): higher version wins; on a version tie the
//     newer ModifiedAt wins, and only a full tie keeps the server copy
//   - a tombstoned row's resurrection rule differs by kind: manga and chapter come back on a
//     higher version OR a ModifiedAt newer than the deletion, because a re-added item
//     restarts at version 1; a category comes back only on a higher version, because
//     categories are uploaded whole on every sync, so an unchanged resend or a
//     restore/reorder that bumps last_modified_at must not restore a deleted category
//   - section kinds: client wins on conflict
//   - client-only -> insert; server-only -> not touched here, returned by the delta
func Merge(store Store, req Request) *Result {
	res := &Result{ReturnKeys: map[Kind][]string{}}
	m := &merger{store: store, req: req, res: res, emitted: map[tombstoneKey]struct{}{}}

	// tombstones first: manga refs point at categories and tombstones affect refs
	for _, tb := range req.Tombstones {
		m.tombstone(tb.Kind, tb.Key)
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
	// (kind, resolved key) pairs already emitted as tombstones this merge, so two request
	// entries that resolve to the same row are counted once
	emitted map[tombstoneKey]struct{}
}

type tombstoneKey struct {
	kind Kind
	key  string
}

// tombstone marks a live row deleted. The tombstone stands until a later write proves it
// is newer, which differs by kind:
//   - manga and chapter resurrect on a higher Version, or on a ModifiedAt newer than the
//     deletion, regardless of Version (a re-added item restarts at version 1, so its
//     timestamp must be able to beat the tombstoned version)
//   - a category resurrects only on a higher Version: categories are uploaded whole on every
//     sync, so a device that has not applied the delete resends its unchanged copy, and
//     restores/reorders bump last_modified_at; a newer timestamp must not bring it back
//   - unversioned section kinds resurrect only when the incoming payload differs from the
//     retained stored payload; a byte-identical re-upload from a stale device stays deleted
func (m *merger) tombstone(kind Kind, key string) {
	// a name-keyed client deletes the category it knows, while another device may have
	// stored the same category under its uid; resolve through the name before giving up
	if kind == KindCategory && m.store.Get(kind, key) == nil {
		if name, ok := strings.CutPrefix(key, "name:"); ok {
			if byName := m.store.CategoryByName(name); byName != nil {
				key = byName.Key
			}
		}
	}
	// two request entries (name:<name> and uid:<uid>) can resolve to the same row; emit the
	// tombstone once so the applied count never exceeds the rows that actually transition
	if _, ok := m.emitted[tombstoneKey{kind, key}]; ok {
		return
	}
	if cur := m.store.Get(kind, key); cur != nil && !cur.Deleted {
		m.res.Tombstones = append(m.res.Tombstones, Tombstone{Kind: kind, Key: key})
		m.emitted[tombstoneKey{kind, key}] = struct{}{}
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
		// a tombstoned category resurrects only on a higher version: categories are
		// uploaded whole on every sync, so a device that has not applied the delete
		// resends its unchanged copy, and restores/reorders bump last_modified_at; a newer
		// timestamp must not bring a deleted category back
		if it.Version > cur.Version {
			m.write(it)
		} else {
			m.returnItem(cur)
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
		// a tombstoned manga or chapter resurrects on a higher version, or on a modified_at
		// newer than the deletion regardless of version: a re-added manga or chapter
		// restarts at version 1, so the timestamp must be able to beat the tombstoned version
		if it.Version > cur.Version || it.ModifiedAt > cur.ModifiedAt {
			m.write(it)
		} else {
			m.returnItem(cur)
		}
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

// section writes the client payload when it differs from the stored one. A tombstoned
// row keeps its payload, so a byte-identical stale re-upload is not written and stays
// deleted; any changed payload resurrects the row.
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
