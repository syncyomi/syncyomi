package http

import (
	"sync"
	"time"
)

const (
	maxFailedAttempts = 5
	failureWindow     = 15 * time.Minute
	maxTrackedClients = 10000
)

type attemptRecord struct {
	count    int
	start    time.Time
	notified bool
}

type loginLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	attempts map[string]attemptRecord
}

type reservation struct {
	allowed     bool
	retryAfter  time.Duration
	justBlocked bool
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{now: now, attempts: make(map[string]attemptRecord)}
}

func (l *loginLimiter) reserve(client string) reservation {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	record, ok := l.attempts[client]
	if !ok || !now.Before(record.start.Add(failureWindow)) {
		l.makeRoom(now)
		record = attemptRecord{start: now}
	}

	if record.count >= maxFailedAttempts {
		justBlocked := !record.notified
		record.notified = true
		l.attempts[client] = record

		return reservation{retryAfter: record.start.Add(failureWindow).Sub(now), justBlocked: justBlocked}
	}

	record.count++
	l.attempts[client] = record

	return reservation{allowed: true}
}

func (l *loginLimiter) release(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if record, ok := l.attempts[client]; ok && record.count > 0 {
		record.count--
		l.attempts[client] = record
	}
}

func (l *loginLimiter) clear(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.attempts, client)
}

func (l *loginLimiter) makeRoom(now time.Time) {
	if len(l.attempts) < maxTrackedClients {
		return
	}

	oldest, oldestStart := "", time.Time{}
	for client, record := range l.attempts {
		if !now.Before(record.start.Add(failureWindow)) {
			delete(l.attempts, client)
			continue
		}
		if oldest == "" || record.start.Before(oldestStart) {
			oldest, oldestStart = client, record.start
		}
	}

	if len(l.attempts) >= maxTrackedClients {
		delete(l.attempts, oldest)
	}
}
