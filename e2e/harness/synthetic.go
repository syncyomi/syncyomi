package harness

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

const LegacyClientTimeout = 10 * time.Second

var legacyClient = &http.Client{Timeout: LegacyClientTimeout}

type SyntheticClient struct {
	Server     *SyncServer
	DeviceID   string
	DeviceName string
	Cursor     int64
}

func NewSyntheticClient(s *SyncServer, deviceID string) *SyntheticClient {
	return &SyntheticClient{Server: s, DeviceID: deviceID, DeviceName: "e2e-synthetic"}
}

type MergeOptions struct {
	Full              bool
	DeletedCategories []int64
	Streamed          bool
	Gzip              bool
}

func (c *SyntheticClient) Merge(ctx context.Context, b *pb.Backup, opts MergeOptions) (*pb.Backup, error) {
	body, err := requestBody(b, opts.Streamed, opts.Gzip)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.Server.BaseURL+"/api/sync/v2/merge", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-API-Token", c.Server.APIKey)
	req.Header.Set("X-Device-ID", c.DeviceID)
	req.Header.Set("X-Device-Name", c.DeviceName)
	req.Header.Set("X-Sync-Cursor", strconv.FormatInt(c.Cursor, 10))
	if opts.Full {
		req.Header.Set("X-Sync-Full", "true")
	}
	if opts.Gzip {
		req.Header.Set("Content-Encoding", "gzip")
	}
	if len(opts.DeletedCategories) > 0 {
		uids := make([]string, len(opts.DeletedCategories))
		for i, uid := range opts.DeletedCategories {
			uids[i] = strconv.FormatInt(uid, 10)
		}
		req.Header.Set("X-Sync-Deleted-Categories", strings.Join(uids, ","))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("merge: status %d: %s", resp.StatusCode, payload)
	}
	if v := resp.Header.Get("X-Sync-Cursor"); v != "" {
		if cur, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.Cursor = cur
		}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return &pb.Backup{}, nil
	}
	return backup.Decode(data)
}

func requestBody(b *pb.Backup, streamed, gzipBody bool) (io.Reader, error) {
	if b == nil {
		return bytes.NewReader(nil), nil
	}
	if streamed {
		pr, pw := io.Pipe()
		go func() {
			var w io.Writer = pw
			var gz *gzip.Writer
			if gzipBody {
				gz = gzip.NewWriter(pw)
				w = gz
			}
			err := writeSplit(w, b)
			if err == nil && gz != nil {
				err = gz.Close()
			}
			pw.CloseWithError(err)
		}()
		return pr, nil
	}
	raw, err := backup.Encode(b)
	if err != nil {
		return nil, err
	}
	if !gzipBody {
		return bytes.NewReader(raw), nil
	}
	compressed, err := gzipBytes(raw)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(compressed), nil
}

func writeSplit(w io.Writer, b *pb.Backup) error {
	for _, m := range b.BackupManga {
		raw, err := proto.Marshal(m)
		if err != nil {
			return err
		}
		var record []byte
		record = protowire.AppendTag(record, 1, protowire.BytesType)
		record = protowire.AppendBytes(record, raw)
		if _, err := w.Write(record); err != nil {
			return err
		}
	}
	rest := proto.Clone(b).(*pb.Backup)
	rest.BackupManga = nil
	encoded, err := backup.Encode(rest)
	if err != nil {
		return err
	}
	_, err = w.Write(encoded)
	return err
}

func gzipBytes(raw []byte) ([]byte, error) {
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

func (c *SyntheticClient) PutV1(ctx context.Context, raw []byte, ifMatch string, gzipBody bool) (etag string, status int, err error) {
	body := raw
	if gzipBody {
		if body, err = gzipBytes(raw); err != nil {
			return "", 0, err
		}
	}
	return c.putV1(ctx, bytes.NewReader(body), ifMatch, gzipBody)
}

func (c *SyntheticClient) PutV1Streamed(ctx context.Context, b *pb.Backup, ifMatch string, gzipBody bool) (etag string, status int, err error) {
	body, err := requestBody(b, true, gzipBody)
	if err != nil {
		return "", 0, err
	}
	return c.putV1(ctx, body, ifMatch, gzipBody)
}

func (c *SyntheticClient) putV1(ctx context.Context, body io.Reader, ifMatch string, gzipBody bool) (etag string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.Server.BaseURL+"/api/sync/content", body)
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-API-Token", c.Server.APIKey)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	if gzipBody {
		req.Header.Set("Content-Encoding", "gzip")
	}
	resp, err := legacyClient.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Header.Get("ETag"), resp.StatusCode, nil
}

func (c *SyntheticClient) GetV1(ctx context.Context, ifNoneMatch string) (data []byte, etag string, status int, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.Server.BaseURL+"/api/sync/content", nil)
	if err != nil {
		return nil, "", 0, err
	}
	req.Header.Set("X-API-Token", c.Server.APIKey)
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	resp, err := legacyClient.Do(req)
	if err != nil {
		return nil, "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, resp.Header.Get("ETag"), resp.StatusCode, nil
	}
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", 0, err
	}
	return data, resp.Header.Get("ETag"), resp.StatusCode, nil
}

func (c *SyntheticClient) ReportEvent(ctx context.Context, event, message string) (status int, err error) {
	body, err := json.Marshal(map[string]string{
		"event":       event,
		"device_id":   c.DeviceID,
		"device_name": c.DeviceName,
		"message":     message,
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.Server.BaseURL+"/api/sync/event", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Token", c.Server.APIKey)
	resp, err := legacyClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
