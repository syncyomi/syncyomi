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

type failureRecord struct {
	count int
	start time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	now      func() time.Time
	failures map[string]failureRecord
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{now: now, failures: make(map[string]failureRecord)}
}

func (l *loginLimiter) blockedFor(client string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	record, ok := l.failures[client]
	if !ok || record.count < maxFailedAttempts {
		return 0, false
	}

	remaining := record.start.Add(failureWindow).Sub(l.now())
	if remaining <= 0 {
		delete(l.failures, client)
		return 0, false
	}

	return remaining, true
}

func (l *loginLimiter) recordFailure(client string) (justBlocked bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	record, ok := l.failures[client]
	if !ok || !now.Before(record.start.Add(failureWindow)) {
		l.makeRoom(now)
		record = failureRecord{start: now}
	}

	record.count++
	l.failures[client] = record

	return record.count == maxFailedAttempts
}

func (l *loginLimiter) recordSuccess(client string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	delete(l.failures, client)
}

func (l *loginLimiter) makeRoom(now time.Time) {
	if len(l.failures) < maxTrackedClients {
		return
	}

	oldest, oldestStart := "", time.Time{}
	for client, record := range l.failures {
		if !now.Before(record.start.Add(failureWindow)) {
			delete(l.failures, client)
			continue
		}
		if oldest == "" || record.start.Before(oldestStart) {
			oldest, oldestStart = client, record.start
		}
	}

	if len(l.failures) >= maxTrackedClients {
		delete(l.failures, oldest)
	}
}
