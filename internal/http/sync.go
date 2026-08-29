package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/SyncYomi/SyncYomi/internal/sync"
	"github.com/go-chi/chi/v5"
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
	DeviceName string `json:"device_name"`
	Message    string `json:"message"`
}

func (h syncHandler) Routes(r chi.Router) {
	r.Get("/content", h.getContent)
	r.Put("/content", h.putContent)
	r.Post("/event", h.reportEvent)
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

	if h.maxBodyBytes > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, h.maxBodyBytes)
	}

	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
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

	w.Header().Set("ETag", *newEtag)
	w.WriteHeader(http.StatusOK)
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

	if err := h.syncService.ReportSyncEvent(r.Context(), apiKey, body.Event, body.DeviceName, body.Message); err != nil {
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
