package harness

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/backup/pb"
)

// SyntheticClient speaks SyncYomi v2 directly, acting as an extra "device" so
// tests can seed server state or inject precise conflict/cursor situations
// without an emulator round-trip.
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

// Merge posts a backup (nil = "nothing changed") and returns what the server
// says this device lacks. The client's cursor advances from the response.
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
