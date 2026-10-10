package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/SyncYomi/SyncYomi/internal/backup"
	"github.com/SyncYomi/SyncYomi/internal/sync"
	"github.com/go-chi/render"
)

const (
	headerDeviceID          = "X-Device-ID"
	headerCursor            = "X-Sync-Cursor"
	headerFull              = "X-Sync-Full"
	headerFullRequested     = "X-Sync-Full-Requested"
	headerChanged           = "X-Sync-Changed"
	headerDeletedCategories = "X-Sync-Deleted-Categories"
	maxDeviceIDLen          = 128
)

// deletionsRequest is the body of POST /api/sync/v2/deletions: chapter keys the client deleted,
// sent ahead of its merge. Keys are the raw backup.ChapterKey values, so the 0x1f separator and
// control characters inside urls are JSON-escaped instead of needing base64 to fit in a header.
type deletionsRequest struct {
	DeletedChapters []string `json:"deletedChapters"`
}

func (h syncHandler) capabilities(w http.ResponseWriter, r *http.Request) {
	render.JSON(w, r, map[string]any{
		"version":  2,
		"merge":    "/api/sync/v2/merge",
		"snapshot": "/api/sync/v2/snapshot",
	})
}

func (h syncHandler) merge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bad := func(msg string) {
		h.encoder.StatusResponse(ctx, w, map[string]string{"message": msg}, http.StatusBadRequest)
	}

	dev := deviceFromRequest(r)
	if dev.ID == "" || len(dev.ID) > maxDeviceIDLen {
		bad("missing or invalid " + headerDeviceID)
		return
	}

	cursor, err := parseCursor(r.Header.Get(headerCursor))
	if err != nil {
		bad("invalid " + headerCursor)
		return
	}

	deleted, err := parseDeletedCategories(r.Header.Get(headerDeletedCategories))
	if err != nil {
		bad("invalid " + headerDeletedCategories)
		return
	}

	// an empty body is a valid (empty) backup: "nothing changed on my side"
	data, ok := h.readBody(w, r)
	if !ok {
		return
	}
	b, err := backup.Decode(data)
	if err != nil {
		bad("body is not a valid backup")
		return
	}

	resp, err := h.syncService.Merge(ctx, sync.MergeRequest{
		APIKey:            r.Header.Get("X-API-Token"),
		Device:            dev,
		Cursor:            cursor,
		Full:              strings.EqualFold(r.Header.Get(headerFull), "true"),
		Backup:            b,
		DeletedCategories: deleted,
	})
	if err != nil {
		if errors.Is(err, sync.ErrBadPayload) {
			bad("body is not a valid backup")
			return
		}
		h.log.Error().Err(err).Msg("merge failed")
		h.encoder.StatusInternalError(w)
		return
	}

	out, err := backup.Encode(resp.Backup)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to encode merge response")
		h.encoder.StatusInternalError(w)
		return
	}

	h.syncService.RecordContentAccess(ctx, r.Header.Get("X-API-Token"), dev, true, sync.ProtocolV2)

	w.Header().Set(headerCursor, strconv.FormatInt(resp.Cursor, 10))
	w.Header().Set(headerChanged, strconv.FormatBool(resp.Changed))
	if resp.FullRequested {
		w.Header().Set(headerFullRequested, "true")
	}
	w.Header().Set("ETag", "seq="+strconv.FormatInt(resp.Cursor, 10))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(out)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(out); err != nil {
		h.log.Debug().Err(err).Msg("failed to write merge response")
	}
}

// deletions applies the chapter tombstones a client reported ahead of its merge, so the merge
// itself never needs to carry bulk deletions in a header. This route is what makes the feature
// detectable: an older server has no such route and answers 404, which keeps the client's
// deletions pending instead of silently dropping them.
func (h syncHandler) deletions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	bad := func(msg string) {
		h.encoder.StatusResponse(ctx, w, map[string]string{"message": msg}, http.StatusBadRequest)
	}

	dev := deviceFromRequest(r)
	if dev.ID == "" || len(dev.ID) > maxDeviceIDLen {
		bad("missing or invalid " + headerDeviceID)
		return
	}

	data, ok := h.readBody(w, r)
	if !ok {
		return
	}
	var req deletionsRequest
	if len(data) > 0 {
		if err := json.Unmarshal(data, &req); err != nil {
			bad("body is not a valid deletions request")
			return
		}
	}

	// a key the merge cannot resolve is a no-op there, so there is no key-level validation: a
	// client bug that produces garbage keys never wedges its pending set
	count, err := h.syncService.DeleteChapters(ctx, sync.DeleteChaptersRequest{
		APIKey:          r.Header.Get("X-API-Token"),
		Device:          dev,
		DeletedChapters: req.DeletedChapters,
	})
	if err != nil {
		h.log.Error().Err(err).Msg("failed to apply deleted chapters")
		h.encoder.StatusInternalError(w)
		return
	}

	render.JSON(w, r, map[string]int{"acknowledged": count})
}

func (h syncHandler) snapshot(w http.ResponseWriter, r *http.Request) {
	cursor, err := parseCursor(r.Header.Get(headerCursor))
	if err != nil {
		h.encoder.StatusResponse(r.Context(), w, map[string]string{"message": "invalid " + headerCursor}, http.StatusBadRequest)
		return
	}

	snap, err := h.syncService.Snapshot(r.Context(), r.Header.Get("X-API-Token"), cursor)
	if err != nil {
		if errors.Is(err, sync.ErrNoData) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.log.Error().Err(err).Msg("snapshot failed")
		h.encoder.StatusInternalError(w)
		return
	}

	h.syncService.RecordContentAccess(r.Context(), r.Header.Get("X-API-Token"), deviceFromRequest(r), false, sync.ProtocolV2)

	w.Header().Set(headerCursor, strconv.FormatInt(snap.Cursor, 10))
	w.Header().Set("ETag", snap.ETag)
	if cursor != 0 && cursor == snap.Cursor {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(snap.Data)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(snap.Data); err != nil {
		h.log.Debug().Err(err).Msg("failed to write snapshot response")
	}
}

func parseCursor(s string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return 0, errors.New("invalid cursor")
	}
	return v, nil
}

func parseDeletedCategories(s string) ([]int64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
