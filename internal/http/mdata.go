package http

import (
	"context"
	"encoding/json"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

type mangaDataService interface {
	Store(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error)
	Delete(ctx context.Context, id int) error
	Update(ctx context.Context, mdata *domain.BackupData) (*domain.BackupData, error)
	ListMangaData(ctx context.Context, apiKey string) ([]domain.BackupData, error)
	GetMangaDataByApiKey(ctx context.Context, apiKey string) (*domain.BackupData, error)
}

type mangaDataHandler struct {
	encoder encoder
	service mangaDataService
}

func newMangaDataHandler(encoder encoder, service mangaDataService) *mangaDataHandler {
	return &mangaDataHandler{
		encoder: encoder,
		service: service,
	}
}

func (h mangaDataHandler) Routes(r chi.Router) {
	r.Post("/", h.store)
	r.Delete("/{id}", h.delete)
	r.Get("/", h.list)
	r.Get("/{apiKey}", h.getByApiKey)
}

func (h mangaDataHandler) store(w http.ResponseWriter, r *http.Request) {
	var (
		ctx   = r.Context()
		mdata domain.BackupData
	)

	if err := json.NewDecoder(r.Body).Decode(&mdata); err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// check/try to store manga data in database
	store, err := h.service.Store(ctx, &mdata)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, store, http.StatusOK)
}

func (h mangaDataHandler) delete(w http.ResponseWriter, r *http.Request) {
	var (
		ctx         = r.Context()
		mangaDataId = chi.URLParam(r, "id")
	)

	// check if id is an integer
	id, _ := strconv.Atoi(mangaDataId)

	// check/try to delete manga data from database
	err := h.service.Delete(ctx, id)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, "BackupManga data deleted", http.StatusOK)
}

func (h mangaDataHandler) list(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = r.Header.Get("X-API-Token")
	)

	// check/try to get manga data from database
	mdata, err := h.service.ListMangaData(ctx, apiKey)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, mdata, http.StatusOK)
}

func (h mangaDataHandler) getByApiKey(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apiKey = r.Header.Get("X-API-Token")
	)

	// check/try to get manga data from database
	mdata, err := h.service.GetMangaDataByApiKey(ctx, apiKey)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.encoder.StatusResponse(ctx, w, mdata, http.StatusOK)
}
