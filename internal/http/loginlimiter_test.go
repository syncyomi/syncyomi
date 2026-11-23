package http

import (
	"fmt"
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

func failTimes(l *loginLimiter, client string, n int) {
	for range n {
		l.recordFailure(client)
	}
}

func TestLoginLimiter(t *testing.T) {
	t.Run("allows attempts below the failure limit", func(t *testing.T) {
		l, _ := newTestLimiter()

		failTimes(l, "10.0.0.1", maxFailedAttempts-1)

		if _, blocked := l.blockedFor("10.0.0.1"); blocked {
			t.Error("blockedFor() = true after fewer failures than the limit, want false")
		}
	})

	t.Run("blocks at the failure limit and reports the time left", func(t *testing.T) {
		l, clock := newTestLimiter()
		failTimes(l, "10.0.0.1", maxFailedAttempts)
		clock.advance(5 * time.Minute)

		retryAfter, blocked := l.blockedFor("10.0.0.1")

		if !blocked || retryAfter != failureWindow-5*time.Minute {
			t.Errorf("blockedFor() = %v, %v, want %v, true", retryAfter, blocked, failureWindow-5*time.Minute)
		}
	})

	t.Run("reports the block only on the failure that causes it", func(t *testing.T) {
		l, _ := newTestLimiter()
		var blockedOn []int

		for i := 1; i <= maxFailedAttempts+2; i++ {
			if l.recordFailure("10.0.0.1") {
				blockedOn = append(blockedOn, i)
			}
		}

		if len(blockedOn) != 1 || blockedOn[0] != maxFailedAttempts {
			t.Errorf("recordFailure() reported a block on attempts %v, want only %d", blockedOn, maxFailedAttempts)
		}
	})

	t.Run("lets the client back in once the window has passed", func(t *testing.T) {
		l, clock := newTestLimiter()
		failTimes(l, "10.0.0.1", maxFailedAttempts)
		clock.advance(failureWindow)

		if _, blocked := l.blockedFor("10.0.0.1"); blocked {
			t.Error("blockedFor() = true after the window elapsed, want false")
		}
		if l.recordFailure("10.0.0.1") {
			t.Error("recordFailure() reported a block on the first failure of a new window")
		}
	})

	t.Run("forgets earlier failures after a success", func(t *testing.T) {
		l, _ := newTestLimiter()
		failTimes(l, "10.0.0.1", maxFailedAttempts-1)

		l.recordSuccess("10.0.0.1")
		failTimes(l, "10.0.0.1", maxFailedAttempts-1)

		if _, blocked := l.blockedFor("10.0.0.1"); blocked {
			t.Error("blockedFor() = true, want a success to reset the failure count")
		}
	})

	t.Run("tracks clients independently", func(t *testing.T) {
		l, _ := newTestLimiter()

		failTimes(l, "10.0.0.1", maxFailedAttempts)

		if _, blocked := l.blockedFor("10.0.0.2"); blocked {
			t.Error("blockedFor() = true for a client that never failed, want false")
		}
	})

	t.Run("bounds memory when many clients fail once", func(t *testing.T) {
		l, _ := newTestLimiter()

		for i := range maxTrackedClients + 500 {
			l.recordFailure(fmt.Sprintf("client-%d", i))
		}

		if got := len(l.failures); got > maxTrackedClients {
			t.Errorf("tracked clients = %d, want at most %d", got, maxTrackedClients)
		}
	})

	t.Run("drops expired clients before evicting live ones", func(t *testing.T) {
		l, clock := newTestLimiter()
		for i := range maxTrackedClients {
			l.recordFailure(fmt.Sprintf("old-%d", i))
		}
		clock.advance(failureWindow)

		l.recordFailure("fresh")

		if got := len(l.failures); got != 1 {
			t.Errorf("tracked clients = %d, want only the fresh one", got)
		}
	})
}
