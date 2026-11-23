package http

import (
	"errors"
	"mime"
	"net/http"

	"github.com/SyncYomi/SyncYomi/internal/auth"
	"github.com/gorilla/sessions"
)

const sessionRefreshFailed = "Password changed, but the session could not be refreshed. Sign in again."

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
	RevokePasskeys  bool   `json:"revoke_passkeys"`
}

func (h authHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	session, ok := requireSession(w, r, h.cookieStore, h.service, h.log)
	if !ok {
		return
	}

	client := clientKey(r)
	if !h.reserveAttempt(w, client) {
		return
	}

	var data changePasswordRequest
	if !requireJSON(w, r) || !decodeBody(w, r, &data) {
		h.limiter.release(client)
		return
	}

	err := h.service.ChangePassword(r.Context(), auth.PasswordChange{
		Current:        data.CurrentPassword,
		Next:           data.NewPassword,
		RevokePasskeys: data.RevokePasskeys,
	})
	switch {
	case err == nil:
		h.limiter.clear(client)
	case errors.Is(err, auth.ErrInvalidCredentials):
		h.log.Warn().Msgf("Auth: password change rejected, wrong current password ip: %s", client)
		http.Error(w, "Current password is incorrect", http.StatusForbidden)
		return
	default:
		h.limiter.release(client)
		h.writePasswordChangeError(w, err)
		return
	}

	if !h.refreshSession(w, r, session) {
		return
	}

	h.log.Info().Msgf("Auth: password changed ip: %s", client)
	h.encoder.StatusResponse(r.Context(), w, nil, http.StatusNoContent)
}

func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return false
	}

	return true
}

func (h authHandler) refreshSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) bool {
	stamp, err := h.service.CurrentPasswordStamp(r.Context())
	if err != nil {
		h.log.Error().Err(err).Msg("Auth: password changed but the session could not be refreshed")
		http.Error(w, sessionRefreshFailed, http.StatusInternalServerError)
		return false
	}

	session.Options = h.sessionOptions(r)
	markSessionAuthenticated(session, stamp)
	if err := session.Save(r, w); err != nil {
		h.log.Error().Err(err).Msg("Auth: password changed but the session could not be saved")
		http.Error(w, sessionRefreshFailed, http.StatusInternalServerError)
		return false
	}

	return true
}

func (h authHandler) writePasswordChangeError(w http.ResponseWriter, err error) {
	if message, ok := auth.PolicyMessage(err); ok {
		http.Error(w, message, http.StatusBadRequest)
		return
	}

	h.writeAuthFailure(w, err, "could not change password")
}
