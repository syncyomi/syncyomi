package backup

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"github.com/SyncYomi/SyncYomi/internal/merge"
	"google.golang.org/protobuf/proto"
)

const keySep = "\x1f"

func CategoryKey(c *pb.BackupCategory) string {
	if c.Uid != 0 {
		return "uid:" + strconv.FormatInt(c.Uid, 10)
	}
	return "name:" + c.Name
}

func ChapterKey(mangaKey, url string) string {
	return mangaKey + keySep + url
}

func SourcePrefKey(sourceKey, prefKey string) string {
	return sourceKey + keySep + prefKey
}

func SavedSearchKey(s *pb.BackupSavedSearch) string {
	return s.Name + "|" + strconv.FormatInt(s.Source, 10)
}

// Split turns a Backup into merge items. Manga payloads carry neither chapters nor
// categories: chapters are items of their own and category membership lives in Refs
// (category keys), so it survives differing category orders between devices.
func Split(b *pb.Backup) ([]*merge.Item, error) {
	var items []*merge.Item

	orderToKey := map[int64]string{}
	for _, c := range b.BackupCategories {
		key := CategoryKey(c)
		orderToKey[c.Order] = key
		payload, err := encodeMsg(c)
		if err != nil {
			return nil, err
		}
		items = append(items, &merge.Item{Kind: merge.KindCategory, Key: key, Name: c.Name, Version: c.Version, ModifiedAt: c.GetLastModifiedAt(), Payload: payload})
	}

	for _, m := range b.BackupManga {
		if m.Url == "" {
			continue
		}
		key := MangaKey(m.Source, m.Url)
		var refs []string
		for _, order := range m.Categories {
			if k, ok := orderToKey[order]; ok {
				refs = append(refs, k)
			}
		}
		stripped := proto.Clone(m).(*pb.BackupManga)
		stripped.Chapters = nil
		stripped.Categories = nil
		payload, err := encodeMsg(stripped)
		if err != nil {
			return nil, err
		}
		items = append(items, &merge.Item{Kind: merge.KindManga, Key: key, Version: m.Version, ModifiedAt: m.GetLastModifiedAt(), Refs: refs, Payload: payload})

		for _, ch := range m.Chapters {
			if ch.Url == "" {
				continue
			}
			payload, err := encodeMsg(ch)
			if err != nil {
				return nil, err
			}
			items = append(items, &merge.Item{Kind: merge.KindChapter, Key: ChapterKey(key, ch.Url), ParentKey: key, Version: ch.Version, ModifiedAt: ch.GetLastModifiedAt(), Payload: payload})
		}
	}

	for _, s := range b.BackupSources {
		payload, err := encodeMsg(s)
		if err != nil {
			return nil, err
		}
		items = append(items, &merge.Item{Kind: merge.KindSource, Key: strconv.FormatInt(s.SourceId, 10), Payload: payload})
	}
	for _, p := range b.BackupPreferences {
		payload, err := encodeMsg(p)
		if err != nil {
			return nil, err
		}
		items = append(items, &merge.Item{Kind: merge.KindAppPref, Key: p.Key, Payload: payload})
	}
	for _, sp := range b.BackupSourcePreferences {
		for _, p := range sp.Prefs {
			payload, err := encodeMsg(p)
			if err != nil {
				return nil, err
			}
			items = append(items, &merge.Item{Kind: merge.KindSourcePref, Key: SourcePrefKey(sp.SourceKey, p.Key), ParentKey: sp.SourceKey, Payload: payload})
		}
	}
	for _, e := range b.BackupExtensionStores {
		payload, err := encodeMsg(e)
		if err != nil {
			return nil, err
		}
		items = append(items, &merge.Item{Kind: merge.KindExtStore, Key: e.IndexUrl, Payload: payload})
	}
	for _, s := range b.BackupSavedSearches {
		payload, err := encodeMsg(s)
		if err != nil {
			return nil, err
		}
		items = append(items, &merge.Item{Kind: merge.KindSavedSearch, Key: SavedSearchKey(s), Payload: payload})
	}
	if extra := b.ProtoReflect().GetUnknown(); len(extra) > 0 {
		items = append(items, &merge.Item{Kind: merge.KindExtra, Key: merge.ExtraItemKey, Payload: append([]byte(nil), extra...)})
	}

	return dedupe(items), nil
}

// dedupe keeps one item per (kind, key): backups can repeat a chapter url or a category,
// and a single write must not touch the same row twice. The highest version wins, version
// ties fall to the newer ModifiedAt, and full ties go to the later occurrence.
func dedupe(items []*merge.Item) []*merge.Item {
	type id struct {
		kind merge.Kind
		key  string
	}
	index := make(map[id]int, len(items))
	out := items[:0]
	for _, it := range items {
		k := id{it.Kind, it.Key}
		if i, ok := index[k]; ok {
			cur := out[i]
			if it.Version > cur.Version || (it.Version == cur.Version && it.ModifiedAt >= cur.ModifiedAt) {
				out[i] = it
			}
			continue
		}
		index[k] = len(out)
		out = append(out, it)
	}
	return out
}

// Render assembles a Backup from items. Tombstoned categories are skipped and manga
// refs pointing at them are dropped. Chapters are attached to their manga; a chapter
// whose manga is not in the list is ignored.
func Render(items []*merge.Item) (*pb.Backup, error) {
	b := &pb.Backup{}

	var categories []*pb.BackupCategory
	keyToOrder := map[string]int64{}
	for _, it := range items {
		if it.Kind != merge.KindCategory || it.Deleted {
			continue
		}
		c := &pb.BackupCategory{}
		if err := proto.Unmarshal(it.Payload, c); err != nil {
			return nil, fmt.Errorf("category %s: %w", it.Key, err)
		}
		categories = append(categories, c)
	}
	sort.SliceStable(categories, func(i, j int) bool { return categories[i].Order < categories[j].Order })
	for _, c := range categories {
		keyToOrder[CategoryKey(c)] = c.Order
	}
	b.BackupCategories = categories

	chaptersByManga := map[string][]*pb.BackupChapter{}
	for _, it := range items {
		if it.Kind != merge.KindChapter {
			continue
		}
		ch := &pb.BackupChapter{}
		if err := proto.Unmarshal(it.Payload, ch); err != nil {
			return nil, fmt.Errorf("chapter %s: %w", it.Key, err)
		}
		chaptersByManga[it.ParentKey] = append(chaptersByManga[it.ParentKey], ch)
	}

	sourcesSeen := map[int64]bool{}
	var mangaSources []int64
	for _, it := range items {
		if it.Kind != merge.KindManga {
			continue
		}
		m := &pb.BackupManga{}
		if err := proto.Unmarshal(it.Payload, m); err != nil {
			return nil, fmt.Errorf("manga %s: %w", it.Key, err)
		}
		m.Categories = nil
		for _, ref := range it.Refs {
			if order, ok := keyToOrder[ref]; ok {
				m.Categories = append(m.Categories, order)
			}
		}
		chapters := chaptersByManga[it.Key]
		sort.SliceStable(chapters, func(i, j int) bool {
			if chapters[i].SourceOrder != chapters[j].SourceOrder {
				return chapters[i].SourceOrder < chapters[j].SourceOrder
			}
			return chapters[i].Url < chapters[j].Url
		})
		m.Chapters = chapters
		b.BackupManga = append(b.BackupManga, m)
		if !sourcesSeen[m.Source] {
			sourcesSeen[m.Source] = true
			mangaSources = append(mangaSources, m.Source)
		}
	}

	sourceListed := map[int64]bool{}
	sourcePrefs := map[string]*pb.BackupSourcePreferences{}
	var sourcePrefOrder []string
	var extra []byte
	for _, it := range items {
		switch it.Kind {
		case merge.KindSource:
			s := &pb.BackupSource{}
			if err := proto.Unmarshal(it.Payload, s); err != nil {
				return nil, fmt.Errorf("source %s: %w", it.Key, err)
			}
			sourceListed[s.SourceId] = true
			b.BackupSources = append(b.BackupSources, s)
		case merge.KindAppPref:
			p := &pb.BackupPreference{}
			if err := proto.Unmarshal(it.Payload, p); err != nil {
				return nil, fmt.Errorf("preference %s: %w", it.Key, err)
			}
			b.BackupPreferences = append(b.BackupPreferences, p)
		case merge.KindSourcePref:
			p := &pb.BackupPreference{}
			if err := proto.Unmarshal(it.Payload, p); err != nil {
				return nil, fmt.Errorf("source preference %s: %w", it.Key, err)
			}
			group, ok := sourcePrefs[it.ParentKey]
			if !ok {
				group = &pb.BackupSourcePreferences{SourceKey: it.ParentKey}
				sourcePrefs[it.ParentKey] = group
				sourcePrefOrder = append(sourcePrefOrder, it.ParentKey)
			}
			group.Prefs = append(group.Prefs, p)
		case merge.KindExtStore:
			e := &pb.BackupExtensionStore{}
			if err := proto.Unmarshal(it.Payload, e); err != nil {
				return nil, fmt.Errorf("extension store %s: %w", it.Key, err)
			}
			b.BackupExtensionStores = append(b.BackupExtensionStores, e)
		case merge.KindSavedSearch:
			s := &pb.BackupSavedSearch{}
			if err := proto.Unmarshal(it.Payload, s); err != nil {
				return nil, fmt.Errorf("saved search %s: %w", it.Key, err)
			}
			b.BackupSavedSearches = append(b.BackupSavedSearches, s)
		case merge.KindExtra:
			extra = it.Payload
		}
	}
	for _, key := range sourcePrefOrder {
		b.BackupSourcePreferences = append(b.BackupSourcePreferences, sourcePrefs[key])
	}
	for _, id := range mangaSources {
		if !sourceListed[id] {
			b.BackupSources = append(b.BackupSources, &pb.BackupSource{SourceId: id})
		}
	}
	if len(extra) > 0 {
		b.ProtoReflect().SetUnknown(extra)
	}

	return b, nil
}

func encodeMsg(m proto.Message) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(m)
}

// MangaKeyParts splits a manga key back into source id and url.
func MangaKeyParts(key string) (int64, string, bool) {
	i := strings.IndexByte(key, '|')
	if i < 0 {
		return 0, "", false
	}
	source, err := strconv.ParseInt(key[:i], 10, 64)
	if err != nil {
		return 0, "", false
	}
	return source, key[i+1:], true
}
