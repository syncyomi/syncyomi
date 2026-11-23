package http

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ current time.Time }

func (c *fakeClock) now() time.Time          { return c.current }
func (c *fakeClock) advance(d time.Duration) { c.current = c.current.Add(d) }

func newTestLimiter() (*loginLimiter, *fakeClock) {
	clock := &fakeClock{current: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	return newLoginLimiter(clock.now), clock
}

func reserveTimes(l *loginLimiter, client string, n int) {
	for range n {
		l.reserve(client)
	}
}

func TestLoginLimiter(t *testing.T) {
	t.Run("allows attempts up to the limit", func(t *testing.T) {
		l, _ := newTestLimiter()

		for i := 1; i <= maxFailedAttempts; i++ {
			if got := l.reserve("10.0.0.1"); !got.allowed {
				t.Fatalf("reserve() attempt %d allowed = false, want true", i)
			}
		}
	})

	t.Run("refuses the attempt after the limit and reports the time left", func(t *testing.T) {
		l, clock := newTestLimiter()
		reserveTimes(l, "10.0.0.1", maxFailedAttempts)
		clock.advance(5 * time.Minute)

		got := l.reserve("10.0.0.1")

		if got.allowed || got.retryAfter != failureWindow-5*time.Minute {
			t.Errorf("reserve() = %+v, want refused with %v left", got, failureWindow-5*time.Minute)
		}
	})

	t.Run("reports the block once, on the first refused attempt", func(t *testing.T) {
		l, _ := newTestLimiter()
		var blockedOn []int

		for i := 1; i <= maxFailedAttempts+3; i++ {
			if l.reserve("10.0.0.1").justBlocked {
				blockedOn = append(blockedOn, i)
			}
		}

		if len(blockedOn) != 1 || blockedOn[0] != maxFailedAttempts+1 {
			t.Errorf("justBlocked on attempts %v, want only %d", blockedOn, maxFailedAttempts+1)
		}
	})

	t.Run("lets the client back in once the window has passed", func(t *testing.T) {
		l, clock := newTestLimiter()
		reserveTimes(l, "10.0.0.1", maxFailedAttempts+1)
		clock.advance(failureWindow)

		if got := l.reserve("10.0.0.1"); !got.allowed {
			t.Error("reserve() refused after the window elapsed, want allowed")
		}
	})

	t.Run("forgets earlier attempts after a success", func(t *testing.T) {
		l, _ := newTestLimiter()
		reserveTimes(l, "10.0.0.1", maxFailedAttempts-1)

		l.clear("10.0.0.1")

		for i := 1; i <= maxFailedAttempts; i++ {
			if got := l.reserve("10.0.0.1"); !got.allowed {
				t.Fatalf("reserve() attempt %d after clear refused, want allowed", i)
			}
		}
	})

	t.Run("gives an attempt back when it reached no verdict", func(t *testing.T) {
		l, _ := newTestLimiter()
		reserveTimes(l, "10.0.0.1", maxFailedAttempts)

		l.release("10.0.0.1")

		if got := l.reserve("10.0.0.1"); !got.allowed {
			t.Error("reserve() refused after a release, want the freed attempt usable")
		}
	})

	t.Run("tracks clients independently", func(t *testing.T) {
		l, _ := newTestLimiter()
		reserveTimes(l, "10.0.0.1", maxFailedAttempts+1)

		if got := l.reserve("10.0.0.2"); !got.allowed {
			t.Error("reserve() refused a client that never attempted, want allowed")
		}
	})

	t.Run("lets only the limit through when attempts race", func(t *testing.T) {
		l, _ := newTestLimiter()
		var (
			wg      sync.WaitGroup
			mu      sync.Mutex
			allowed int
		)

		for range 200 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if l.reserve("10.0.0.1").allowed {
					mu.Lock()
					allowed++
					mu.Unlock()
				}
			}()
		}
		wg.Wait()

		if allowed != maxFailedAttempts {
			t.Errorf("allowed attempts = %d, want %d", allowed, maxFailedAttempts)
		}
	})

	t.Run("bounds memory when many clients attempt once", func(t *testing.T) {
		l, _ := newTestLimiter()

		for i := range maxTrackedClients + 500 {
			l.reserve(fmt.Sprintf("client-%d", i))
		}

		if got := len(l.attempts); got > maxTrackedClients {
			t.Errorf("tracked clients = %d, want at most %d", got, maxTrackedClients)
		}
	})

	t.Run("drops expired clients before evicting live ones", func(t *testing.T) {
		l, clock := newTestLimiter()
		for i := range maxTrackedClients {
			l.reserve(fmt.Sprintf("old-%d", i))
		}
		clock.advance(failureWindow)

		l.reserve("fresh")

		if got := len(l.attempts); got != 1 {
			t.Errorf("tracked clients = %d, want only the fresh one", got)
		}
	})
}
