package passkey

import (
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

const (
	maxCeremonies = 1000
)

type ceremony struct {
	session webauthn.SessionData
	name    string
	expires time.Time
}

type ceremonyStore struct {
	mu      sync.Mutex
	now     func() time.Time
	pending map[string]ceremony
}

func newCeremonyStore(now func() time.Time) *ceremonyStore {
	return &ceremonyStore{now: now, pending: make(map[string]ceremony)}
}

func (s *ceremonyStore) put(id string, session webauthn.SessionData, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	if len(s.pending) >= maxCeremonies {
		s.makeRoom(now)
	}

	s.pending[id] = ceremony{session: session, name: name, expires: now.Add(CeremonyTTL)}
}

func (s *ceremonyStore) take(id string) (ceremony, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	found, ok := s.pending[id]
	delete(s.pending, id)

	return found, ok && s.now().Before(found.expires)
}

func (s *ceremonyStore) makeRoom(now time.Time) {
	oldest, oldestExpiry := "", time.Time{}
	for id, entry := range s.pending {
		if !now.Before(entry.expires) {
			delete(s.pending, id)
			continue
		}
		if oldest == "" || entry.expires.Before(oldestExpiry) {
			oldest, oldestExpiry = id, entry.expires
		}
	}

	if len(s.pending) >= maxCeremonies {
		delete(s.pending, oldest)
	}
}
