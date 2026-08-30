package backup

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"google.golang.org/protobuf/proto"
)

var gzipMagic = []byte{0x1f, 0x8b}

// Decode parses a Tachiyomi backup, transparently inflating gzip (.tachibk files are gzipped,
// sync payloads are raw).
func Decode(data []byte) (*pb.Backup, error) {
	if bytes.HasPrefix(data, gzipMagic) {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("gunzip backup: %w", err)
		}
		defer gz.Close()
		if data, err = io.ReadAll(gz); err != nil {
			return nil, fmt.Errorf("gunzip backup: %w", err)
		}
	}

	b := &pb.Backup{}
	if err := proto.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("decode backup: %w", err)
	}
	return b, nil
}

func Encode(b *pb.Backup) ([]byte, error) {
	return proto.MarshalOptions{Deterministic: true}.Marshal(b)
}

func EncodeGzip(b *pb.Backup) ([]byte, error) {
	raw, err := Encode(b)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func MangaKey(source int64, url string) string {
	return fmt.Sprintf("%d|%s", source, url)
}

// IsFavorite honours the Kotlin default: an absent field means true.
func IsFavorite(m *pb.BackupManga) bool {
	return m.Favorite == nil || *m.Favorite
}

func SetFavorite(m *pb.BackupManga, favorite bool) {
	m.Favorite = proto.Bool(favorite)
}
