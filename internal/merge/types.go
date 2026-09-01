// Package merge implements the server-side sync merge on a generic item model.
// It never inspects payloads, so the same rules serve plaintext and encrypted items.
package merge

type Kind string

const (
	KindManga       Kind = "manga"
	KindChapter     Kind = "chapter"
	KindCategory    Kind = "category"
	KindSource      Kind = "source"
	KindAppPref     Kind = "app_pref"
	KindSourcePref  Kind = "source_pref"
	KindExtStore    Kind = "ext_store"
	KindSavedSearch Kind = "saved_search"
	KindExtra       Kind = "extra" // unknown top-level Backup fields, single item
	ExtraItemKey         = "extra"
)

// SectionKinds have no version; on conflict the client wins.
var SectionKinds = []Kind{KindSource, KindAppPref, KindSourcePref, KindExtStore, KindSavedSearch, KindExtra}

func IsSectionKind(k Kind) bool {
	for _, s := range SectionKinds {
		if s == k {
			return true
		}
	}
	return false
}

type Item struct {
	Kind      Kind
	Key       string
	ParentKey string // chapter -> manga key, source_pref -> source key
	Name      string // category display name, used for the uid-less fallback match
	Version   int64
	// ModifiedAt breaks equal-version ties (clients that never bump Version would
	// otherwise be stuck at "first write wins forever"). Extracted by the adapter so the
	// merge itself still never inspects payloads.
	ModifiedAt int64
	Deleted    bool     // category tombstone
	Refs       []string // manga -> category keys
	Payload    []byte

	// set by the store
	Seq          int64
	OriginDevice string
}

// Store is what the merge needs to know about the server side for the keys in a request.
type Store interface {
	// Get returns the stored item or nil.
	Get(kind Kind, key string) *Item
	// CategoryByName returns a live or tombstoned category with that name, or nil.
	CategoryByName(name string) *Item
}

type Request struct {
	DeviceID          string
	Items             []*Item
	DeletedCategories []string // category keys the client deleted
}

type Result struct {
	// Writes are the items to upsert; the store assigns Seq and OriginDevice.
	Writes []*Item
	// Tombstones are category keys to mark deleted.
	Tombstones []string
	// ReturnKeys are items the client must receive because the server version won.
	ReturnKeys map[Kind][]string
	// ChangedForClient is true when the client's state differs from the server's after the merge.
	ChangedForClient bool
}
