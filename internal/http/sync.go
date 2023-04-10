package http

import (
	"context"
	"encoding/json"
	"github.com/go-chi/chi/v5"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"net/http"
	"strconv"
)

type syncService interface {
	Store(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, sync *domain.Sync) (*domain.Sync, error)
	ListSyncs(ctx context.Context, apiKey string) ([]domain.Sync, error)
	GetSyncByApiKey(ctx context.Context, apiKey string) (*domain.Sync, error)
	GetSyncByDeviceID(ctx context.Context, deviceID int) (*domain.Sync, error)
	SyncData(ctx context.Context, sync *domain.SyncData) (*domain.SyncData, error)
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
	r.Get("/device/{id}", h.getSyncByDeviceID)
	r.Get("/{apiKey}", h.getSyncByApiKey)
	r.Post("/data", h.sync)
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

func (h syncHandler) getSyncByDeviceID(w http.ResponseWriter, r *http.Request) {
	var (
		ctx      = r.Context()
		deviceId = chi.URLParam(r, "id")
	)

	// check if id is an integer
	id, _ := strconv.Atoi(deviceId)

	// check/try to get sync from database
	sync, err := h.syncService.GetSyncByDeviceID(ctx, id)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, sync, http.StatusOK)
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
		Device:     &domain.Device{UserApiKey: &domain.APIKey{Key: apiKey}},
	}
	sync.Data = &domain.MangaData{UserApiKey: &domain.APIKey{Key: apiKey}}
	sync.Device = &domain.Device{UserApiKey: &domain.APIKey{Key: apiKey}}

	if err := json.NewDecoder(r.Body).Decode(&sync); err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// store, check and try to sync data
	syncResult, err := h.syncService.SyncData(ctx, &sync)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !syncResult.UpdateRequired {
		h.encoder.StatusResponse(ctx, w, syncResult, http.StatusOK)
		return
	}

	h.encoder.StatusResponse(ctx, w, syncResult, http.StatusOK)
}
