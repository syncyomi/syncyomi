package http

import (
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
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(out); err != nil {
		h.log.Debug().Err(err).Msg("failed to write merge response")
	}
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
