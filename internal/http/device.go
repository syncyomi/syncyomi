package http

import (
	"context"
	"encoding/json"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

type deviceService interface {
	Store(ctx context.Context, device *domain.Device) (*domain.Device, error)
	Delete(ctx context.Context, id int) error
	ListDevices(ctx context.Context, apikey string) ([]domain.Device, error)
	GetDeviceByDeviceId(ctx context.Context, device *domain.Device) (*domain.Device, error)
	GetDeviceByApiKey(ctx context.Context, device *domain.Device) (*domain.Device, error)
}

type deviceHandler struct {
	encoder encoder
	service deviceService
}

func newDeviceHandler(encoder encoder, service deviceService) *deviceHandler {
	return &deviceHandler{
		encoder: encoder,
		service: service,
	}
}

func (h deviceHandler) Routes(r chi.Router) {
	r.Post("/", h.store)
	r.Delete("/{id}", h.delete)
	r.Get("/", h.listDevices)
}

func (h deviceHandler) store(w http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		data domain.Device
	)

	// check if X-API-Token is present
	if r.Header.Get("X-API-Token") == "" {
		h.encoder.StatusResponse(ctx, w, "X-API-Token header is missing", http.StatusBadRequest)
		return
	}

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		// encode error
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	// check/try to store device in database
	d, err := h.service.Store(ctx, &data)
	if err != nil {
		h.encoder.StatusResponse(ctx, w, err.Error(), http.StatusBadRequest)
		return
	}

	h.encoder.StatusResponse(ctx, w, d, http.StatusCreated)
}

func (h deviceHandler) delete(w http.ResponseWriter, r *http.Request) {
	var (
		ctx      = r.Context()
		deviceId = chi.URLParam(r, "id")
	)

	id, _ := strconv.Atoi(deviceId)

	if err := h.service.Delete(ctx, id); err != nil {
		h.encoder.StatusResponse(ctx, w, "Failed deleting device", http.StatusBadRequest)
		return
	}

	h.encoder.StatusResponse(ctx, w, nil, http.StatusOK)
}

func (h deviceHandler) listDevices(w http.ResponseWriter, r *http.Request) {
	var (
		ctx    = r.Context()
		apikey = r.Header.Get("X-API-Token")
	)

	list, err := h.service.ListDevices(ctx, apikey)
	if err != nil {
		h.encoder.StatusNotFound(ctx, w)
		return
	}

	h.encoder.StatusResponse(ctx, w, list, http.StatusOK)
}
