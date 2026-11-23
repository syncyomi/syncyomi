package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SyncYomi/SyncYomi/internal/auth"
	"github.com/SyncYomi/SyncYomi/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog"
	"math"
	"mime"
	"net"
	"net/http"
	"strconv"
	"time"
)

const maxPasswordRequestBytes = 4 << 10

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

func clientKey(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}

	return r.RemoteAddr
}

func (h authHandler) rejectBlocked(w http.ResponseWriter, client string) bool {
	retryAfter, blocked := h.limiter.blockedFor(client)
	if !blocked {
		return false
	}

	w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(retryAfter.Seconds()))))
	http.Error(w, fmt.Sprintf("Too many failed attempts. Try again in %d minutes.", int(math.Ceil(retryAfter.Minutes()))), http.StatusTooManyRequests)

	return true
}

func (h authHandler) recordFailure(client string) {
	if h.limiter.recordFailure(client) {
		h.service.NotifyLoginBlocked(client)
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

// sessionOptions builds per-request options because syncyomi does not serve TLS itself:
// browsers only accept SameSite=Strict on a Secure cookie, which needs a proxy forwarding https.
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
	var (
		ctx  = r.Context()
		data domain.User
	)

	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		// encode error
		h.encoder.StatusResponse(ctx, w, nil, http.StatusBadRequest)
		return
	}

	client := clientKey(r)
	if h.rejectBlocked(w, client) {
		return
	}

	session, _ := h.cookieStore.Get(r, sessionName)
	session.Options = h.sessionOptions(r)

	user, err := h.service.Login(ctx, data.Username, data.Password)
	if err != nil {
		h.recordFailure(client)
		h.log.Error().Err(err).Msgf("Auth: Failed login attempt username: [%s] ip: %s", data.Username, ReadUserIP(r))
		h.encoder.StatusResponse(ctx, w, nil, http.StatusUnauthorized)
		return
	}

	h.limiter.recordSuccess(client)
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
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		// encode error
		h.encoder.StatusResponse(ctx, w, nil, http.StatusBadRequest)
		return
	}

	err := h.service.CreateUser(ctx, data.Username, data.Password)
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
	ctx := r.Context()
	session, _ := h.cookieStore.Get(r, sessionName)

	authenticated, err := sessionAuthenticated(ctx, session, h.service)
	if err != nil {
		h.log.Error().Err(err).Msg("Auth: could not verify session")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !authenticated {
		http.Error(w, "Forbidden", http.StatusUnauthorized)
		return
	}

	// send empty response as ok
	h.encoder.StatusResponse(ctx, w, nil, http.StatusNoContent)
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (h authHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session, _ := h.cookieStore.Get(r, sessionName)

	authenticated, err := sessionAuthenticated(ctx, session, h.service)
	if err != nil {
		h.log.Error().Err(err).Msg("Auth: could not verify session")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !authenticated {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	client := clientKey(r)
	if h.rejectBlocked(w, client) {
		return
	}

	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	var data changePasswordRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxPasswordRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := h.service.ChangePassword(ctx, data.CurrentPassword, data.NewPassword); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			h.recordFailure(client)
		}
		h.writePasswordChangeError(w, r, err)
		return
	}
	h.limiter.recordSuccess(client)

	stamp, err := h.service.CurrentPasswordStamp(ctx)
	if err != nil {
		h.log.Error().Err(err).Msg("Auth: password changed but the session could not be refreshed")
		http.Error(w, "Password changed, but the session could not be refreshed. Sign in again.", http.StatusInternalServerError)
		return
	}

	session.Options = h.sessionOptions(r)
	markSessionAuthenticated(session, stamp)
	if err := session.Save(r, w); err != nil {
		h.log.Error().Err(err).Msg("Auth: password changed but the session could not be saved")
		http.Error(w, "Password changed, but the session could not be refreshed. Sign in again.", http.StatusInternalServerError)
		return
	}

	h.log.Info().Msgf("Auth: password changed ip: %s", ReadUserIP(r))
	h.encoder.StatusResponse(ctx, w, nil, http.StatusNoContent)
}

func (h authHandler) writePasswordChangeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrPasswordTooShort):
		http.Error(w, fmt.Sprintf("New password must be at least %d characters", auth.MinPasswordLength), http.StatusBadRequest)
	case errors.Is(err, auth.ErrPasswordTooLong):
		http.Error(w, fmt.Sprintf("New password must be at most %d bytes", auth.MaxPasswordBytes), http.StatusBadRequest)
	case errors.Is(err, auth.ErrInvalidCredentials):
		h.log.Warn().Msgf("Auth: password change rejected, wrong current password ip: %s", ReadUserIP(r))
		http.Error(w, "Current password is incorrect", http.StatusForbidden)
	default:
		h.log.Error().Err(err).Msg("Auth: could not change password")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func ReadUserIP(r *http.Request) string {
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	return IPAddress
}
