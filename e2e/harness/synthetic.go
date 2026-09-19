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
}

func (c *SyntheticClient) Merge(ctx context.Context, b *pb.Backup, opts MergeOptions) (*pb.Backup, error) {
	var body []byte
	if b != nil {
		var err error
		if body, err = backup.Encode(b); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.Server.BaseURL+"/api/sync/v2/merge", bytes.NewReader(body))
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

func (c *SyntheticClient) PutV1(ctx context.Context, raw []byte, ifMatch string, gzipBody bool) (etag string, status int, err error) {
	body := raw
	if gzipBody {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(raw); err != nil {
			return "", 0, err
		}
		if err := gz.Close(); err != nil {
			return "", 0, err
		}
		body = buf.Bytes()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.Server.BaseURL+"/api/sync/content", bytes.NewReader(body))
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
