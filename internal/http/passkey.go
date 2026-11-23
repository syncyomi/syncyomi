package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/SyncYomi/SyncYomi/internal/auth"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/SyncYomi/SyncYomi/internal/passkey"
	"github.com/go-chi/chi/v5"
	"github.com/go-webauthn/webauthn/protocol"
)

const (
	maxPasskeyBodyBytes = 64 << 10
	ceremonyCookieName  = "passkey_ceremony"
)

type passkeyService interface {
	Enabled() bool
	BeginRegistration(ctx context.Context, name string) (string, *protocol.CredentialCreation, error)
	FinishRegistration(ctx context.Context, ceremonyID string, r *http.Request) (*domain.Passkey, error)
	BeginLogin() (string, *protocol.CredentialAssertion, error)
	FinishLogin(ctx context.Context, ceremonyID string, r *http.Request) (*domain.User, error)
	List(ctx context.Context) ([]domain.Passkey, error)
	Delete(ctx context.Context, id int) (bool, error)
}

type passkeyHandler struct {
	auth    *authHandler
	service passkeyService
}

func newPasskeyHandler(auth *authHandler, service passkeyService) *passkeyHandler {
	return &passkeyHandler{auth: auth, service: service}
}

func (h passkeyHandler) Routes(r chi.Router) {
	r.Get("/capabilities", h.capabilities)
	r.Post("/passkey/login/begin", h.loginBegin)
	r.Post("/passkey/login/finish", h.loginFinish)
	r.Get("/passkeys", h.list)
	r.Post("/passkeys/register/begin", h.registerBegin)
	r.Post("/passkeys/register/finish", h.registerFinish)
	r.Delete("/passkeys/{id}", h.remove)
}

func (h passkeyHandler) enabled() bool {
	return h.service != nil && h.service.Enabled()
}

func (h passkeyHandler) capabilities(w http.ResponseWriter, r *http.Request) {
	h.auth.encoder.StatusResponse(r.Context(), w, map[string]bool{"passkeys": h.enabled()}, http.StatusOK)
}

func (h passkeyHandler) loginBegin(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}

	id, assertion, err := h.service.BeginLogin()
	if err != nil {
		h.fail(w, err, "could not start a passkey login")
		return
	}

	h.setCeremonyCookie(w, r, id, int(passkey.CeremonyTTL.Seconds()))
	h.auth.encoder.StatusResponse(r.Context(), w, assertion, http.StatusOK)
}

func (h passkeyHandler) loginFinish(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}

	client := clientKey(r)
	if !h.auth.reserveAttempt(w, client) {
		return
	}

	ceremonyID, ok := h.readCeremony(w, r)
	if !ok || !h.readJSONBody(w, r) {
		h.auth.limiter.release(client)
		return
	}

	session, _ := h.auth.cookieStore.Get(r, sessionName)
	session.Options = h.auth.sessionOptions(r)

	user, err := h.service.FinishLogin(r.Context(), ceremonyID, r)
	switch {
	case err == nil:
		h.auth.limiter.clear(client)
	case errors.Is(err, passkey.ErrRejected), errors.Is(err, passkey.ErrCloned):
		h.auth.log.Warn().Msgf("Auth: failed passkey login ip: %s", client)
		h.clearCeremonyCookie(w, r)
		http.Error(w, "Passkey sign-in failed", http.StatusUnauthorized)
		return
	case errors.Is(err, passkey.ErrCeremonyNotFound):
		h.auth.limiter.release(client)
		h.clearCeremonyCookie(w, r)
		http.Error(w, "The passkey request expired, try again", http.StatusBadRequest)
		return
	default:
		h.auth.limiter.release(client)
		h.fail(w, err, "passkey login failed")
		return
	}

	markSessionAuthenticated(session, user.PasswordStamp())
	if err := session.Save(r, w); err != nil {
		h.fail(w, err, "could not save the session")
		return
	}

	h.clearCeremonyCookie(w, r)
	h.auth.encoder.StatusResponse(r.Context(), w, map[string]string{"username": user.Username}, http.StatusOK)
}

type registerBeginRequest struct {
	Name            string `json:"name"`
	CurrentPassword string `json:"current_password"`
}

func (h passkeyHandler) registerBegin(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}
	if _, ok := requireSession(w, r, h.auth.cookieStore, h.auth.service, h.auth.log); !ok {
		return
	}

	client := clientKey(r)
	if !h.auth.reserveAttempt(w, client) {
		return
	}

	var data registerBeginRequest
	if !requireJSON(w, r) || !decodeBody(w, r, &data) {
		h.auth.limiter.release(client)
		return
	}

	err := h.auth.service.VerifyCurrentPassword(r.Context(), data.CurrentPassword)
	switch {
	case err == nil:
		h.auth.limiter.clear(client)
	case errors.Is(err, auth.ErrInvalidCredentials):
		h.auth.log.Warn().Msgf("Auth: passkey registration rejected, wrong current password ip: %s", client)
		http.Error(w, "Current password is incorrect", http.StatusForbidden)
		return
	default:
		h.auth.limiter.release(client)
		h.auth.writeAuthFailure(w, err, "could not verify the password")
		return
	}

	id, creation, err := h.service.BeginRegistration(r.Context(), data.Name)
	if err != nil {
		h.fail(w, err, "could not start a passkey registration")
		return
	}

	h.setCeremonyCookie(w, r, id, int(passkey.CeremonyTTL.Seconds()))
	h.auth.encoder.StatusResponse(r.Context(), w, creation, http.StatusOK)
}

type passkeyView struct {
	ID         int        `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func viewOf(key domain.Passkey) passkeyView {
	view := passkeyView{ID: key.ID, Name: key.Name, CreatedAt: key.CreatedAt}
	if !key.LastUsedAt.IsZero() {
		view.LastUsedAt = &key.LastUsedAt
	}

	return view
}

func (h passkeyHandler) registerFinish(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}
	if _, ok := requireSession(w, r, h.auth.cookieStore, h.auth.service, h.auth.log); !ok {
		return
	}

	ceremonyID, ok := h.readCeremony(w, r)
	if !ok || !h.readJSONBody(w, r) {
		return
	}

	created, err := h.service.FinishRegistration(r.Context(), ceremonyID, r)
	h.clearCeremonyCookie(w, r)
	if err != nil {
		h.fail(w, err, "could not register the passkey")
		return
	}

	h.auth.encoder.StatusCreatedData(w, viewOf(*created))
}

func (h passkeyHandler) list(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}
	if _, ok := requireSession(w, r, h.auth.cookieStore, h.auth.service, h.auth.log); !ok {
		return
	}

	passkeys, err := h.service.List(r.Context())
	if err != nil {
		h.fail(w, err, "could not list passkeys")
		return
	}

	views := make([]passkeyView, 0, len(passkeys))
	for _, key := range passkeys {
		views = append(views, viewOf(key))
	}
	h.auth.encoder.StatusResponse(r.Context(), w, views, http.StatusOK)
}

func (h passkeyHandler) remove(w http.ResponseWriter, r *http.Request) {
	if !h.enabled() {
		http.NotFound(w, r)
		return
	}
	if _, ok := requireSession(w, r, h.auth.cookieStore, h.auth.service, h.auth.log); !ok {
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid passkey id", http.StatusBadRequest)
		return
	}

	deleted, err := h.service.Delete(r.Context(), id)
	if err != nil {
		h.fail(w, err, "could not delete the passkey")
		return
	}
	if !deleted {
		http.NotFound(w, r)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h passkeyHandler) fail(w http.ResponseWriter, err error, action string) {
	switch {
	case errors.Is(err, passkey.ErrDisabled):
		http.Error(w, "Passkeys are not enabled", http.StatusNotFound)
	case errors.Is(err, passkey.ErrInvalidName):
		http.Error(w, fmt.Sprintf("Give the passkey a name of up to %d characters", passkey.MaxNameLength), http.StatusBadRequest)
	case errors.Is(err, passkey.ErrLimit):
		http.Error(w, "The passkey limit is reached, remove one first", http.StatusConflict)
	case errors.Is(err, passkey.ErrCeremonyNotFound):
		http.Error(w, "The passkey request expired, try again", http.StatusBadRequest)
	case errors.Is(err, passkey.ErrRejected):
		http.Error(w, "The passkey could not be verified", http.StatusBadRequest)
	default:
		h.auth.log.Error().Err(err).Msgf("Auth: %s", action)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (h passkeyHandler) readCeremony(w http.ResponseWriter, r *http.Request) (string, bool) {
	cookie, err := r.Cookie(ceremonyCookieName)
	if err != nil || cookie.Value == "" {
		http.Error(w, "The passkey request expired, try again", http.StatusBadRequest)
		return "", false
	}

	return cookie.Value, true
}

func (h passkeyHandler) readJSONBody(w http.ResponseWriter, r *http.Request) bool {
	if !requireJSON(w, r) {
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPasskeyBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return false
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	return true
}

func (h passkeyHandler) setCeremonyCookie(w http.ResponseWriter, r *http.Request, value string, maxAge int) {
	options := h.auth.sessionOptions(r)
	http.SetCookie(w, &http.Cookie{
		Name:     ceremonyCookieName,
		Value:    value,
		Path:     options.Path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   options.Secure,
		SameSite: options.SameSite,
	})
}

func (h passkeyHandler) clearCeremonyCookie(w http.ResponseWriter, r *http.Request) {
	h.setCeremonyCookie(w, r, "", -1)
}
