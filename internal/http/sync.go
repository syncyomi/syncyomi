package http

import (
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/sync"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"github.com/rs/zerolog"
)

type syncService = sync.Service

type syncHandler struct {
	encoder      encoder
	log          zerolog.Logger
	syncService  syncService
	maxBodyBytes int64
}

func newSyncHandler(encoder encoder, log zerolog.Logger, syncService syncService, maxBodyBytes int64) *syncHandler {
	return &syncHandler{
		encoder:      encoder,
		log:          log.With().Str("handler", "sync").Logger(),
		syncService:  syncService,
		maxBodyBytes: maxBodyBytes,
	}
}

// syncEventRequest is the body for POST /api/sync/event (device-reported sync status).
type syncEventRequest struct {
	Event      string `json:"event"`
	DeviceID   string `json:"device_id"`
	DeviceName string `json:"device_name"`
	Message    string `json:"message"`
}

// Routes are the client-facing sync endpoints (API key auth).
func (h syncHandler) Routes(r chi.Router) {
	r.With(middleware.Compress(5, "application/octet-stream")).Get("/content", h.getContent)
	r.Put("/content", h.putContent)
	r.Post("/event", h.reportEvent)
}

// AdminRoutes expose per-key status for the web UI and must be mounted behind session auth only.
func (h syncHandler) AdminRoutes(r chi.Router) {
	r.Get("/{apikey}/status", h.getStatus)
	r.Get("/{apikey}/devices", h.listDevices)
	r.Get("/{apikey}/history", h.listHistory)
	r.Post("/{apikey}/history/{id}/restore", h.restoreHistory)
}

func deviceFromRequest(r *http.Request) domain.DeviceInfo {
	return domain.DeviceInfo{
		ID:   r.Header.Get("X-Device-ID"),
		Name: r.Header.Get("X-Device-Name"),
	}
}

func (h syncHandler) getContent(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Token")
	etag := r.Header.Get("If-None-Match")

	if etag != "" {
		etagInDb, err := h.syncService.GetSyncDataETag(r.Context(), apiKey)
		if err != nil {
			h.log.Error().Err(err).Msg("failed to read sync data etag")
			h.encoder.StatusInternalError(w)
			return
		}

		if etagInDb != nil && etag == *etagInDb {
			// see: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/If-None-Match
			h.syncService.RecordContentAccess(r.Context(), apiKey, deviceFromRequest(r), false)
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	syncData, syncDataETag, err := h.syncService.GetSyncDataAndETag(r.Context(), apiKey)
	if err != nil {
		h.log.Error().Err(err).Msg("failed to read sync data")
		h.encoder.StatusInternalError(w)
		return
	}

	if syncData == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	h.syncService.RecordContentAccess(r.Context(), apiKey, deviceFromRequest(r), false)

	if syncDataETag != nil {
		w.Header().Set("ETag", *syncDataETag)
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(syncData); err != nil {
		h.log.Debug().Err(err).Msg("failed to write sync data response")
	}
}

func (h syncHandler) putContent(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Token")
	etag := r.Header.Get("If-Match")

	requestData, err := h.readBody(w, r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) || errors.Is(err, errBodyTooLarge) {
			h.encoder.StatusResponse(r.Context(), w, map[string]string{"message": "request body too large"}, http.StatusRequestEntityTooLarge)
			return
		}
		h.encoder.StatusResponse(r.Context(), w, map[string]string{"message": err.Error()}, http.StatusBadRequest)
		return
	}

	var newEtag *string
	if etag != "" {
		newEtag, err = h.syncService.SetSyncDataIfMatch(r.Context(), apiKey, etag, requestData)
	} else {
		newEtag, err = h.syncService.SetSyncData(r.Context(), apiKey, requestData)
	}
	if err != nil {
		h.log.Error().Err(err).Msg("failed to store sync data")
		h.encoder.StatusInternalError(w)
		return
	}

	if newEtag == nil {
		// see: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/If-Match
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	h.syncService.RecordContentAccess(r.Context(), apiKey, deviceFromRequest(r), true)

	w.Header().Set("ETag", *newEtag)
	w.WriteHeader(http.StatusOK)
}

var errBodyTooLarge = errors.New("request body too large")

func (h syncHandler) readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	var body io.Reader = r.Body
	if h.maxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
		body = r.Body
	}

	if r.Header.Get("Content-Encoding") == "gzip" {
		gz, err := gzip.NewReader(body)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		body = gz
		if h.maxBodyBytes > 0 {
			body = io.LimitReader(gz, h.maxBodyBytes+1)
		}
	}

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if h.maxBodyBytes > 0 && int64(len(data)) > h.maxBodyBytes {
		return nil, errBodyTooLarge
	}

	return data, nil
}

func (h syncHandler) reportEvent(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Token")
	if apiKey == "" {
		apiKey = r.URL.Query().Get("apikey")
	}
	if apiKey == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	var body syncEventRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		h.encoder.StatusResponse(r.Context(), w, map[string]string{"message": "invalid JSON body"}, http.StatusBadRequest)
		return
	}
	if body.Event == "" {
		h.encoder.StatusResponse(r.Context(), w, map[string]string{"message": "event is required"}, http.StatusBadRequest)
		return
	}

	dev := deviceFromRequest(r)
	if dev.ID == "" {
		dev.ID = body.DeviceID
	}
	if dev.Name == "" {
		dev.Name = body.DeviceName
	}

	if err := h.syncService.ReportSyncEvent(r.Context(), apiKey, body.Event, dev, body.Message); err != nil {
		if errors.Is(err, sync.ErrInvalidSyncEvent) {
			h.encoder.StatusResponse(r.Context(), w, map[string]string{"message": "invalid sync event"}, http.StatusBadRequest)
			return
		}
		h.log.Error().Err(err).Msg("failed to report sync event")
		h.encoder.StatusInternalError(w)
		return
	}

	h.encoder.NoContent(w)
}

func (h syncHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.syncService.GetStatus(r.Context(), chi.URLParam(r, "apikey"))
	if err != nil {
		h.log.Error().Err(err).Msg("failed to read sync status")
		h.encoder.StatusInternalError(w)
		return
	}
	if status == nil {
		h.encoder.StatusNotFound(r.Context(), w)
		return
	}

	render.JSON(w, r, status)
}

func (h syncHandler) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := h.syncService.ListDevices(r.Context(), chi.URLParam(r, "apikey"))
	if err != nil {
		h.log.Error().Err(err).Msg("failed to list devices")
		h.encoder.StatusInternalError(w)
		return
	}

	render.JSON(w, r, devices)
}

func (h syncHandler) listHistory(w http.ResponseWriter, r *http.Request) {
	history, err := h.syncService.ListHistory(r.Context(), chi.URLParam(r, "apikey"))
	if err != nil {
		h.log.Error().Err(err).Msg("failed to list sync history")
		h.encoder.StatusInternalError(w)
		return
	}

	render.JSON(w, r, history)
}

func (h syncHandler) restoreHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		h.encoder.StatusResponse(r.Context(), w, map[string]string{"message": "invalid history id"}, http.StatusBadRequest)
		return
	}

	etag, err := h.syncService.RestoreHistory(r.Context(), chi.URLParam(r, "apikey"), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			h.encoder.StatusNotFound(r.Context(), w)
			return
		}
		h.log.Error().Err(err).Msg("failed to restore sync history")
		h.encoder.StatusInternalError(w)
		return
	}

	render.JSON(w, r, map[string]string{"etag": *etag})
}
