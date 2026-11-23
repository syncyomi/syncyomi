package http

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/SyncYomi/SyncYomi/internal/auth"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog"
	"net/http"
	"time"
)

const maxAuthRequestBytes = 4 << 10

type authService interface {
	GetUserCount(ctx context.Context) (int, error)
	Login(ctx context.Context, username, password string) (*domain.User, error)
	CreateUser(ctx context.Context, username, password string) error
	CurrentPasswordStamp(ctx context.Context) (string, error)
	ChangePassword(ctx context.Context, current, next string) error
	NotifyLoginBlocked(client string)
}

type authHandler struct {
	log     zerolog.Logger
	encoder encoder
	config  *domain.Config
	service authService

	cookieStore *sessions.CookieStore
	limiter     *loginLimiter
}

func newAuthHandler(encoder encoder, log zerolog.Logger, config *domain.Config, cookieStore *sessions.CookieStore, service authService) *authHandler {
	return &authHandler{
		log:         log,
		encoder:     encoder,
		config:      config,
		service:     service,
		cookieStore: cookieStore,
		limiter:     newLoginLimiter(time.Now),
	}
}

func (h authHandler) Routes(r chi.Router) {
	r.Post("/login", h.login)
	r.Post("/logout", h.logout)
	r.Post("/onboard", h.onboard)
	r.Get("/onboard", h.canOnboard)
	r.Get("/validate", h.validate)
	r.Post("/password", h.changePassword)
}

// sessionOptions is per request because browsers only accept SameSite=Strict on a Secure cookie, which needs a proxy forwarding https.
func (h authHandler) sessionOptions(r *http.Request) *sessions.Options {
	secure := h.config.SecureCookie || r.Header.Get("X-Forwarded-Proto") == "https"
	sameSite := http.SameSiteLaxMode
	if secure {
		sameSite = http.SameSiteStrictMode
	}

	return &sessions.Options{
		Path:     h.config.BaseURL,
		MaxAge:   86400 * 30,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	}
}

func (h authHandler) login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	client := clientKey(r)
	if !h.reserveAttempt(w, client) {
		return
	}

	var data domain.User
	if !decodeBody(w, r, &data) {
		h.limiter.release(client)
		return
	}

	session, _ := h.cookieStore.Get(r, sessionName)
	session.Options = h.sessionOptions(r)

	user, err := h.service.Login(ctx, data.Username, data.Password)
	switch {
	case err == nil:
		h.limiter.clear(client)
	case errors.Is(err, auth.ErrInvalidCredentials):
		h.log.Warn().Msgf("Auth: Failed login attempt username: [%s] ip: %s", data.Username, client)
		h.encoder.StatusResponse(ctx, w, nil, http.StatusUnauthorized)
		return
	case errors.Is(err, auth.ErrBusy):
		h.limiter.release(client)
		http.Error(w, "Server busy, try again shortly", http.StatusServiceUnavailable)
		return
	default:
		h.limiter.release(client)
		h.log.Error().Err(err).Msg("Auth: login failed")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	markSessionAuthenticated(session, user.PasswordStamp())
	session.Save(r, w)

	h.encoder.StatusResponse(ctx, w, nil, http.StatusNoContent)
}

func (h authHandler) logout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	session, _ := h.cookieStore.Get(r, sessionName)

	// Revoke users authentication
	session.Values[sessionAuthenticatedKey] = false
	session.Save(r, w)

	h.encoder.StatusResponse(ctx, w, nil, http.StatusNoContent)
}

func (h authHandler) onboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, _ := h.cookieStore.Get(r, sessionName)

	// Don't proceed if user is authenticated
	if _, ok := session.Values[sessionAuthenticatedKey].(bool); ok {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var data domain.User
	if !decodeBody(w, r, &data) {
		return
	}

	err := h.service.CreateUser(ctx, data.Username, data.Password)
	if message, ok := auth.PolicyMessage(err); ok {
		http.Error(w, message, http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// send empty response as ok
	h.encoder.StatusResponse(ctx, w, nil, http.StatusNoContent)
}

func (h authHandler) canOnboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userCount, err := h.service.GetUserCount(ctx)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if userCount > 0 {
		// send 503 service onboarding unavailable
		http.Error(w, "Onboarding unavailable", http.StatusForbidden)
		return
	}

	// send empty response as ok
	// (client can proceed with redirection to onboarding page)
	h.encoder.StatusResponse(ctx, w, nil, http.StatusNoContent)
}

func (h authHandler) validate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireSession(w, r, h.cookieStore, h.service, h.log); !ok {
		return
	}

	h.encoder.StatusResponse(r.Context(), w, nil, http.StatusNoContent)
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxAuthRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return false
		}
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return false
	}

	return true
}
