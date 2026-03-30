package notify

import (
	"sync"
	"time"
)

// RateLimiter limits notification sending with retry support.
type RateLimiter struct {
	mu         sync.Mutex
	interval   time.Duration
	maxRetries int
	lastSent   time.Time
	queue      []pendingNotification
}

type pendingNotification struct {
	title   string
	body    string
	retries int
}

// NewRateLimiter creates a rate limiter with the given minimum interval between sends.
func NewRateLimiter(interval time.Duration, maxRetries int) *RateLimiter {
	return &RateLimiter{
		interval:   interval,
		maxRetries: maxRetries,
	}
}

// TrySend attempts to send a notification, respecting rate limits.
// Returns true if the notification was sent, false if rate limited.
// Failed sends are queued for retry.
func (rl *RateLimiter) TrySend(title, body string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if now.Sub(rl.lastSent) < rl.interval {
		rl.queue = append(rl.queue, pendingNotification{title: title, body: body})
		return false
	}

	if err := Send(title, body); err != nil {
		rl.queue = append(rl.queue, pendingNotification{title: title, body: body})
		return false
	}

	rl.lastSent = now
	return true
}

// Flush attempts to send all queued notifications.
// Returns the number of successfully sent notifications.
func (rl *RateLimiter) Flush() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if len(rl.queue) == 0 {
		return 0
	}

	sent := 0
	var remaining []pendingNotification
	now := time.Now()

	for _, n := range rl.queue {
		if now.Sub(rl.lastSent) < rl.interval {
			remaining = append(remaining, n)
			continue
		}
		if err := Send(n.title, n.body); err != nil {
			n.retries++
			if n.retries < rl.maxRetries {
				remaining = append(remaining, n)
			}
			// else: drop after max retries
		} else {
			sent++
			rl.lastSent = now
		}
	}

	rl.queue = remaining
	return sent
}

// QueueLen returns the number of pending notifications.
func (rl *RateLimiter) QueueLen() int {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return len(rl.queue)
}
