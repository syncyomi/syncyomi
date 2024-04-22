package http

import (
	"io"
	"log"
	"net/http"

	"github.com/SyncYomi/SyncYomi/internal/sync"
	"github.com/go-chi/chi/v5"
)

type syncService = sync.Service

type syncHandler struct {
	encoder     encoder
	syncService syncService
}

func newSyncHandler(encoder encoder, syncService syncService) *syncHandler {
	return &syncHandler{
		encoder:     encoder,
		syncService: syncService,
	}
}

func (h syncHandler) Routes(r chi.Router) {
	r.Get("/content", h.getContent)
	r.Put("/content", h.putContent)
}

func (h syncHandler) getContent(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Token")
	etag := r.Header.Get("If-None-Match")

	if etag != "" {
		etagInDb, err := h.syncService.GetSyncDataETag(r.Context(), apiKey)
		if err != nil {
			log.Println(err)
			h.encoder.StatusInternalError(w)
			return
		}

		if etagInDb != nil && etag == *etagInDb {
			// nothing changed after last request
			// see: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/If-None-Match
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}

	syncData, syncDataETag, err := h.syncService.GetSyncDataAndETag(r.Context(), apiKey)

	if err != nil {
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
	w.Write(syncData)
	w.WriteHeader(http.StatusOK)
}

func (h syncHandler) putContent(w http.ResponseWriter, r *http.Request) {
	apiKey := r.Header.Get("X-API-Token")
	etag := r.Header.Get("If-Match")

	// Read data from request body
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		h.encoder.StatusResponse(r.Context(), w, err.Error(), http.StatusBadRequest)
		return
	}

	var newEtag *string
	if etag != "" {
		newEtag, err = h.syncService.SetSyncDataIfMatch(r.Context(), apiKey, etag, requestData)
	} else {
		newEtag, err = h.syncService.SetSyncData(r.Context(), apiKey, requestData)
	}
	if err != nil {
		h.encoder.StatusInternalError(w)
	}

	if newEtag == nil {
		// syncdata was changed from other clients
		// see: https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/If-Match
		w.WriteHeader(http.StatusPreconditionFailed)
	} else {
		w.Header().Set("ETag", *newEtag)
		w.WriteHeader(http.StatusOK)
	}
}
