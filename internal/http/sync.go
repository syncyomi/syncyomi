package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/go-chi/chi/v5"
)

type syncService interface {
	Store(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	ListSyncs(ctx context.Context, apiKey string) ([]domain.Sync, error)
	GetSyncByApiKey(ctx context.Context, apiKey string) (*domain.Sync, error)
	GetSyncData(ctx context.Context, apiKey string) (*domain.SyncData, error)
	SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error)
	GetSyncLockFile(ctx context.Context, apiKey string) (*domain.SyncLockFile, error)
	CreateSyncLockFile(ctx context.Context, apiKey string, acquiredBy string) (*domain.SyncLockFile, error)
	UpdateSyncLockFile(ctx context.Context, syncLockFile *domain.SyncLockFile) (*domain.SyncLockFile, error)
	DeleteSyncLockFile(ctx context.Context, apiKey string) bool
}

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
	r.Post("/", h.store)
	r.Delete("/{id}", h.delete)
	r.Get("/", h.listSyncs)
	r.Get("/{apiKey}", h.getSyncByApiKey)
	r.Get("/download", h.getSyncData)
	r.Post("/upload", h.sync)
	r.Get("/lock", h.getSyncLockFile)
	r.Post("/lock", h.createSyncLockFile)
	r.Patch("/lock", h.updateSyncLockFile)
}

func (h syncHandler) store(w http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		sync domain.Sync
	)

	if err := json.NewDecoder(r.Body).Decode(&sync); err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// check/try to store sync in database
	store, err := h.syncService.Store(ctx, &sync)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, store, http.StatusOK)
}

func (h syncHandler) delete(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		syncId = chi.URLParam(r, "id")
	)

	// check if id is an integer
	id, _ := strconv.Atoi(syncId)

	// check/try to delete sync from database
	err := h.syncService.Delete(ctx, id)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, "Sync deleted", http.StatusOK)
}

func (h syncHandler) listSyncs(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = r.Header.Get("X-API-Token")
	)

	// check/try to get syncs from database
	syncs, err := h.syncService.ListSyncs(ctx, apiKey)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, syncs, http.StatusOK)
}

func (h syncHandler) getSyncByApiKey(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = chi.URLParam(r, "apiKey")
	)

	// check/try to get sync from database
	sync, err := h.syncService.GetSyncByApiKey(ctx, apiKey)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, sync, http.StatusOK)
}

func (h syncHandler) sync(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		sync   = domain.SyncData{}
		apiKey = r.Header.Get("X-API-Token")
	)

	// check if api key is set
	if apiKey == "" {
		h.encoder.StatusResponse(ctx, w, "No API key set", http.StatusBadRequest)
		return
	}

	sync.Sync = &domain.Sync{
		UserApiKey: &domain.APIKey{Key: apiKey},
	}
	sync.Data = &domain.BackupData{UserApiKey: &domain.APIKey{Key: apiKey}}

	// Read data from request body
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check if the Content-Encoding header is set to "gzip" and uncompress the data if necessary
	if r.Header.Get("Content-Encoding") == "gzip" {
		uncompressedData, err := uncompress(requestData)
		if err != nil {
			h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
			return
		}
		requestData = uncompressedData
	}

	// Decode JSON from uncompressed data
	if err := json.Unmarshal(requestData, &sync); err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// Store, check, and try to sync data
	syncResult, err := h.syncService.SyncData(ctx, &sync)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, syncResult, http.StatusOK)
}

func (h syncHandler) getSyncData(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = r.Header.Get("X-API-Token")
	)

	// check if api key is set
	if apiKey == "" {
		h.encoder.StatusResponse(ctx, w, "No API key set", http.StatusBadRequest)
		return
	}

	dataResult, err := h.syncService.GetSyncData(ctx, apiKey)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, dataResult, http.StatusOK)
}

func (h syncHandler) getSyncLockFile(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = r.Header.Get("X-API-Token")
	)

	// check if api key is set
	if apiKey == "" {
		h.encoder.StatusResponse(ctx, w, "No API key set", http.StatusBadRequest)
		return
	}

	// check/try to get sync lock file from database
	syncLockFile, err := h.syncService.GetSyncLockFile(ctx, apiKey)
	if err != nil {
		// check if error is due no rows found
		if err.Error() != "error executing query: sql: no rows in result set" {
			h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			h.encoder.StatusResponse(ctx, w, "No sync lock file found", http.StatusNotFound)
			return
		}
	}

	h.encoder.StatusResponse(ctx, w, syncLockFile, http.StatusOK)
}

func (h syncHandler) createSyncLockFile(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = r.Header.Get("X-API-Token")
	)

	// check if api key is set
	if apiKey == "" {
		h.encoder.StatusResponse(ctx, w, "No API key set", http.StatusBadRequest)
		return
	}

	// Read data from request body
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// Unmarshal JSON from request body
	var syncLockFile domain.SyncLockFile
	if err := json.Unmarshal(requestData, &syncLockFile); err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// check if sync lock exists
	lockFile, err := h.syncService.GetSyncLockFile(ctx, apiKey)
	if err != nil {
		// check if error is due no rows found
		if err.Error() != "error executing query: sql: no rows in result set" {
			h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			// create sync lock file
			lockFile, err = h.syncService.CreateSyncLockFile(ctx, apiKey, syncLockFile.AcquiredBy)
			if err != nil {
				h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
				return
			}

			h.encoder.StatusResponse(ctx, w, lockFile, http.StatusOK)
			return
		}
	}
}

func (h syncHandler) updateSyncLockFile(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = r.Header.Get("X-API-Token")
	)

	// check if api key is set
	if apiKey == "" {
		h.encoder.StatusResponse(ctx, w, "No API key set", http.StatusBadRequest)
		return
	}

	// Read data from request body
	requestData, err := io.ReadAll(r.Body)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// Unmarshal JSON from request body
	var syncLockFile domain.SyncLockFile
	if err := json.Unmarshal(requestData, &syncLockFile); err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// check if sync lock exists
	lockFile, err := h.syncService.GetSyncLockFile(ctx, apiKey)
	if err != nil {
		// check if error is due no rows found
		if err.Error() != "error executing query: sql: no rows in result set" {
			h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			h.encoder.StatusResponse(ctx, w, "No sync lock file found", http.StatusNotFound)
			return
		}
	}

	// update sync lock file
	lockFile, err = h.syncService.UpdateSyncLockFile(ctx, &syncLockFile)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, lockFile, http.StatusOK)
}
