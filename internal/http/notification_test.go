package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SyncYomi/SyncYomi/internal/domain"
)

type fakeNotificationService struct {
	storeErr  error
	updateErr error
	deleteErr error
}

func (f *fakeNotificationService) Find(context.Context, domain.NotificationQueryParams) ([]domain.Notification, int, error) {
	return nil, 0, nil
}

func (f *fakeNotificationService) FindByID(context.Context, int) (*domain.Notification, error) {
	return nil, nil
}

func (f *fakeNotificationService) Store(context.Context, domain.Notification) (*domain.Notification, error) {
	return nil, f.storeErr
}

func (f *fakeNotificationService) Update(context.Context, domain.Notification) (*domain.Notification, error) {
	return nil, f.updateErr
}

func (f *fakeNotificationService) Delete(context.Context, int) error {
	return f.deleteErr
}

func (f *fakeNotificationService) Test(context.Context, domain.Notification) error {
	return nil
}

func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}

	var body errorResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	if body.Message != errTest.Error() {
		t.Errorf("message: got %q, want %q", body.Message, errTest.Error())
	}
}

func TestNotificationHandler_storeReturnsServiceError(t *testing.T) {
	h := newNotificationHandler(encoder{}, &fakeNotificationService{storeErr: errTest})
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"n","type":"WEBHOOK"}`))
	rec := httptest.NewRecorder()

	h.store(rec, req)

	assertErrorResponse(t, rec)
}

func TestNotificationHandler_updateReturnsServiceError(t *testing.T) {
	h := newNotificationHandler(encoder{}, &fakeNotificationService{updateErr: errTest})
	req := httptest.NewRequest(http.MethodPut, "/1", strings.NewReader(`{"id":1,"name":"n","type":"WEBHOOK"}`))
	rec := httptest.NewRecorder()

	h.update(rec, req)

	assertErrorResponse(t, rec)
}

func TestNotificationHandler_deleteReturnsServiceError(t *testing.T) {
	h := newNotificationHandler(encoder{}, &fakeNotificationService{deleteErr: errTest})
	req := httptest.NewRequest(http.MethodDelete, "/1", nil)
	rec := httptest.NewRecorder()

	h.delete(rec, req)

	assertErrorResponse(t, rec)
}
