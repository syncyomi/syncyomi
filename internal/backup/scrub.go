package backup

import (
	"fmt"
	"strings"

	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// Fields whose text is not personal and is needed for the data to stay meaningful.
var scrubKeep = map[string]bool{
	"syncyomi.backup.v1.BackupPreference.key":               true,
	"syncyomi.backup.v1.BackupSourcePreferences.source_key": true,
	"syncyomi.backup.v1.PreferenceValue.type":               true,
	"syncyomi.backup.v1.BackupSource.name":                  true,
}

// Scrub replaces every string and bytes value with a deterministic placeholder and drops
// unknown fields, keeping all numeric fields (versions, timestamps, flags, ids) intact so
// merge tests exercise realistic data without personal content.
func Scrub(b *pb.Backup) *pb.Backup {
	out := proto.Clone(b).(*pb.Backup)
	counters := map[string]int{}
	scrubMessage(out.ProtoReflect(), counters)
	return out
}

func scrubMessage(m protoreflect.Message, counters map[string]int) {
	m.SetUnknown(nil)
	fields := m.Descriptor().Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		if !m.Has(fd) {
			continue
		}
		switch {
		case fd.IsList():
			list := m.Mutable(fd).List()
			for j := 0; j < list.Len(); j++ {
				switch fd.Kind() {
				case protoreflect.MessageKind:
					scrubMessage(list.Get(j).Message(), counters)
				case protoreflect.StringKind:
					list.Set(j, protoreflect.ValueOfString(placeholder(fd, counters)))
				case protoreflect.BytesKind:
					list.Set(j, protoreflect.ValueOfBytes(nil))
				}
			}
		case fd.Kind() == protoreflect.MessageKind:
			scrubMessage(m.Mutable(fd).Message(), counters)
		case fd.Kind() == protoreflect.StringKind:
			if scrubKeep[string(fd.FullName())] {
				continue
			}
			m.Set(fd, protoreflect.ValueOfString(placeholder(fd, counters)))
		case fd.Kind() == protoreflect.BytesKind:
			m.Set(fd, protoreflect.ValueOfBytes(scrubBytes(fd, m.Get(fd).Bytes(), m)))
		}
	}
}

func placeholder(fd protoreflect.FieldDescriptor, counters map[string]int) string {
	name := string(fd.FullName())
	counters[name]++
	n := counters[name]
	switch fd.Name() {
	case "url", "manga_url", "merge_url", "tracking_url", "thumbnail_url", "custom_thumbnail_url", "index_url", "extension_list_url", "contact_website":
		return fmt.Sprintf("/%s/%d", strings.TrimSuffix(string(fd.Name()), "_url"), n)
	default:
		return fmt.Sprintf("%s %d", fd.Name(), n)
	}
}

func scrubBytes(fd protoreflect.FieldDescriptor, value []byte, parent protoreflect.Message) []byte {
	switch fd.FullName() {
	case "syncyomi.backup.v1.PreferenceValue.value":
		// only string-valued preferences can carry personal text; scalar ones are kept
		typ := parent.Get(parent.Descriptor().Fields().ByName("type")).String()
		if strings.Contains(typ, "String") {
			return []byte{0x0a, 0x00} // field 1: empty string
		}
		return value
	default:
		if len(value) == 0 {
			return value
		}
		return []byte("{}")
	}
}
