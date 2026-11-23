package passkey

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
)

func newTestCeremonies() (*ceremonyStore, *time.Time) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	return newCeremonyStore(func() time.Time { return now }), &now
}

func TestCeremonyStore(t *testing.T) {
	t.Run("hands a ceremony back once", func(t *testing.T) {
		store, _ := newTestCeremonies()
		store.put("a", webauthn.SessionData{Challenge: "c1"}, "Laptop")

		first, ok := store.take("a")
		_, again := store.take("a")

		if !ok || first.session.Challenge != "c1" || first.name != "Laptop" || again {
			t.Errorf("take() = %+v, %v then again = %v, want the ceremony once", first, ok, again)
		}
	})

	t.Run("does not know an unknown id", func(t *testing.T) {
		store, _ := newTestCeremonies()

		if _, ok := store.take("missing"); ok {
			t.Error("take() found a ceremony that was never stored")
		}
	})

	t.Run("expires a ceremony after the time to live", func(t *testing.T) {
		store, now := newTestCeremonies()
		store.put("a", webauthn.SessionData{Challenge: "c1"}, "")
		*now = now.Add(CeremonyTTL)

		if _, ok := store.take("a"); ok {
			t.Error("take() returned an expired ceremony")
		}
	})

	t.Run("keeps memory bounded by dropping the oldest ceremony", func(t *testing.T) {
		store, now := newTestCeremonies()
		for i := range maxCeremonies + 5 {
			store.put(fmt.Sprintf("id-%d", i), webauthn.SessionData{}, "")
			*now = now.Add(time.Millisecond)
		}

		_, oldest := store.take("id-0")
		_, newest := store.take(fmt.Sprintf("id-%d", maxCeremonies+4))
		if len(store.pending) > maxCeremonies || oldest || !newest {
			t.Errorf("tracked = %d, oldest kept = %v, newest kept = %v, want a bounded store that keeps the newest", len(store.pending), oldest, newest)
		}
	})

	t.Run("drops expired ceremonies before evicting live ones", func(t *testing.T) {
		store, now := newTestCeremonies()
		for i := range maxCeremonies {
			store.put(fmt.Sprintf("old-%d", i), webauthn.SessionData{}, "")
		}
		*now = now.Add(CeremonyTTL)
		store.put("fresh", webauthn.SessionData{}, "")

		if len(store.pending) != 1 {
			t.Errorf("tracked = %d, want only the fresh ceremony", len(store.pending))
		}
	})
}
