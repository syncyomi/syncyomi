package http

import (
	"context"
	"crypto/subtle"
	"net/http"

	"github.com/SyncYomi/SyncYomi/internal/auth"
	"github.com/SyncYomi/SyncYomi/pkg/errors"
	"github.com/gorilla/sessions"
	"github.com/rs/zerolog"
)

const (
	sessionName             = "user_session"
	sessionAuthenticatedKey = "authenticated"
	sessionStampKey         = "pwv"
)

type stampSource interface {
	CurrentPasswordStamp(ctx context.Context) (string, error)
}

func sessionAuthenticated(ctx context.Context, session *sessions.Session, source stampSource) (bool, error) {
	if authenticated, ok := session.Values[sessionAuthenticatedKey].(bool); !ok || !authenticated {
		return false, nil
	}

	stamp, ok := session.Values[sessionStampKey].(string)
	if !ok {
		return false, nil
	}

	current, err := source.CurrentPasswordStamp(ctx)
	if errors.Is(err, auth.ErrNoUser) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return subtle.ConstantTimeCompare([]byte(stamp), []byte(current)) == 1, nil
}

func markSessionAuthenticated(session *sessions.Session, stamp string) {
	session.Values[sessionAuthenticatedKey] = true
	session.Values[sessionStampKey] = stamp
}

func requireSession(w http.ResponseWriter, r *http.Request, store *sessions.CookieStore, source stampSource, log zerolog.Logger) (*sessions.Session, bool) {
	session, _ := store.Get(r, sessionName)

	authenticated, err := sessionAuthenticated(r.Context(), session, source)
	if err != nil {
		log.Error().Err(err).Msg("could not verify session")
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return nil, false
	}
	if !authenticated {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return nil, false
	}

	return session, true
}
