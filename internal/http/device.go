package http

import (
	"context"
	"github.com/go-chi/chi/v5"
	"github.com/kaiserbh/tachiyomi-sync-server/internal/domain"
	"net/http"
)

type deviceService interface {
	Store(ctx context.Context, device *domain.Device) error
	Delete(ctx context.Context, id int) error
	ListDevices(ctx context.Context) ([]domain.Device, error)
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
	//TODO implement me
	panic("implement me")
}

func (h deviceHandler) delete(w http.ResponseWriter, r *http.Request) {
	//TODO implement me
	panic("implement me")
}

func (h deviceHandler) listDevices(w http.ResponseWriter, r *http.Request) {
	//TODO implement me
	panic("implement me")
}
