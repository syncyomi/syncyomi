package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

func TestDecodeFavoriteDefault(t *testing.T) {
	// kotlinx omits favorite=true (the default) and writes 100:0 for false
	var absent []byte
	absent = protowire.AppendTag(absent, 2, protowire.BytesType)
	absent = protowire.AppendString(absent, "/m/1")

	explicitFalse := append([]byte(nil), absent...)
	explicitFalse = protowire.AppendTag(explicitFalse, 100, protowire.VarintType)
	explicitFalse = protowire.AppendVarint(explicitFalse, 0)

	for _, tc := range []struct {
		name string
		raw  []byte
		want bool
	}{
		{"absent means favorite", absent, true},
		{"explicit false", explicitFalse, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &pb.BackupManga{}
			if err := proto.Unmarshal(tc.raw, m); err != nil {
				t.Fatal(err)
			}
			if got := IsFavorite(m); got != tc.want {
				t.Errorf("IsFavorite = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDecodeCategoriesPackedOrNot(t *testing.T) {
	var unpacked []byte
	for _, v := range []uint64{1, 3} {
		unpacked = protowire.AppendTag(unpacked, 17, protowire.VarintType)
		unpacked = protowire.AppendVarint(unpacked, v)
	}
	var packed []byte
	packed = protowire.AppendTag(packed, 17, protowire.BytesType)
	packed = protowire.AppendBytes(packed, []byte{1, 3})

	for name, raw := range map[string][]byte{"unpacked": unpacked, "packed": packed} {
		m := &pb.BackupManga{}
		if err := proto.Unmarshal(raw, m); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(m.Categories) != 2 || m.Categories[0] != 1 || m.Categories[1] != 3 {
			t.Errorf("%s: categories = %v", name, m.Categories)
		}
	}

	// and we write them unpacked, like kotlinx
	enc, err := Encode(&pb.Backup{BackupManga: []*pb.BackupManga{{Url: "/m", Categories: []int64{1, 3}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(enc, unpacked) {
		t.Errorf("categories not encoded unpacked: %x", enc)
	}
}

func TestDecodeGzipAndRaw(t *testing.T) {
	b := &pb.Backup{BackupManga: []*pb.BackupManga{{Source: 1, Url: "/m/1", Title: "t"}}}
	raw, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	gz, err := EncodeGzip(b)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"raw": raw, "gzip": gz} {
		got, err := Decode(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !proto.Equal(got, b) {
			t.Errorf("%s: round trip mismatch", name)
		}
	}
	if _, err := Decode([]byte{0x1f, 0x8b, 0xff}); err == nil {
		t.Error("corrupt gzip accepted")
	}
}

func TestUnknownFieldsSurvive(t *testing.T) {
	var raw []byte
	raw = protowire.AppendTag(raw, 2, protowire.BytesType)
	raw = protowire.AppendString(raw, "/m/1")
	raw = protowire.AppendTag(raw, 9000, protowire.BytesType) // Suwayomi-style extra field
	raw = protowire.AppendString(raw, "extra")

	m := &pb.BackupManga{}
	if err := proto.Unmarshal(raw, m); err != nil {
		t.Fatal(err)
	}
	out, err := proto.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("extra")) {
		t.Error("unknown field dropped on re-encode")
	}
}

func loadFixture(t *testing.T) *pb.Backup {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "backup_scrubbed.tachibk"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFixtureRoundTrip(t *testing.T) {
	b := loadFixture(t)
	if len(b.BackupManga) == 0 || len(b.BackupCategories) == 0 {
		t.Fatal("fixture is empty")
	}
	enc, err := Encode(b)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Decode(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(b, again) {
		t.Error("fixture does not round trip")
	}
}

// Runs only when a real backup is present locally; it must never be committed.
func TestPrivateFixtureRoundTrip(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "private", "*.tachibk"))
	if len(files) == 0 {
		t.Skip("no private fixture")
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		b, err := Decode(raw)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		enc, err := Encode(b)
		if err != nil {
			t.Fatal(err)
		}
		again, err := Decode(enc)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(b, again) {
			t.Errorf("%s: round trip mismatch", f)
		}
		for _, m := range b.BackupManga {
			if len(m.ProtoReflect().GetUnknown()) > 0 {
				t.Logf("%s: manga %q has unknown fields (fork-specific data?)", f, m.Url)
				break
			}
		}
	}
}

func TestScrubRemovesText(t *testing.T) {
	b := &pb.Backup{
		BackupManga: []*pb.BackupManga{{
			Source: 7, Url: "/secret/manga", Title: "Secret Title", Author: proto.String("Someone"),
			Version: 42, LastModifiedAt: 99, Memo: []byte(`{"note":"private"}`),
			Chapters: []*pb.BackupChapter{{Url: "/secret/ch1", Name: "Chapter 1", Version: 3}},
			History:  []*pb.BackupHistory{{Url: "/secret/ch1", LastRead: 5}},
		}},
		BackupCategories:  []*pb.BackupCategory{{Name: "My Category", Order: 2, Uid: 11}},
		BackupPreferences: []*pb.BackupPreference{{Key: "theme", Value: &pb.PreferenceValue{Type: "StringPreferenceValue", Value: []byte("\x0a\x05dark!")}}},
	}
	b.BackupManga[0].ProtoReflect().SetUnknown([]byte{0xc2, 0x3e, 0x01, 0x41}) // field 1000, bytes "A"

	s := Scrub(b)
	enc, _ := Encode(s)
	for _, secret := range []string{"secret", "Secret Title", "Someone", "private", "Chapter 1", "My Category", "dark!"} {
		if bytes.Contains(enc, []byte(secret)) {
			t.Errorf("%q survived scrubbing", secret)
		}
	}
	m := s.BackupManga[0]
	if m.Source != 7 || m.Version != 42 || m.LastModifiedAt != 99 || m.Chapters[0].Version != 3 || s.BackupCategories[0].Uid != 11 {
		t.Error("numeric fields changed")
	}
	if m.Url == "" || m.Chapters[0].Url == "" || s.BackupCategories[0].Name == "" {
		t.Error("keys blanked instead of replaced")
	}
	if s.BackupPreferences[0].Key != "theme" {
		t.Error("preference key should be kept")
	}
	if len(m.ProtoReflect().GetUnknown()) != 0 {
		t.Error("unknown fields kept")
	}
	if proto.Equal(b, s) || b.BackupManga[0].Title != "Secret Title" {
		t.Error("Scrub must not modify its input")
	}
}
