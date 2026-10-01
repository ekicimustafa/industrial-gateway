package connector

import "time"

// Breaker is the connector circuit breaker (Python: _cb_record_success /
// _cb_record_error). Below Threshold consecutive errors it asks for a fixed
// RetryDelay; from Threshold on it opens and backs off exponentially:
// BaseDelay, 2×, 4×, … capped at MaxDelay.
//
// Not safe for concurrent use: it is owned by the connector's run loop.
type Breaker struct {
	Threshold  int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	RetryDelay time.Duration

	now       func() time.Time
	streak    int
	openUntil time.Time
}

// NewBreaker returns a breaker with the Python defaults for the delays.
func NewBreaker(threshold int, baseDelay time.Duration) *Breaker {
	return &Breaker{
		Threshold:  max(threshold, 1),
		BaseDelay:  baseDelay,
		MaxDelay:   10 * time.Minute,
		RetryDelay: 10 * time.Second,
		now:        time.Now,
	}
}

// RecordSuccess resets the breaker and returns the streak it cleared.
func (b *Breaker) RecordSuccess() int {
	prev := b.streak
	b.streak = 0
	b.openUntil = time.Time{}
	return prev
}

// RecordError counts a failure and returns how long to wait before the next
// attempt; opened is true when the breaker is in back-off.
func (b *Breaker) RecordError() (delay time.Duration, opened bool) {
	b.streak++
	if b.streak < b.Threshold {
		return b.RetryDelay, false
	}
	delay = b.BaseDelay
	for i := b.Threshold; i < b.streak && delay < b.MaxDelay; i++ {
		delay *= 2
	}
	delay = min(delay, b.MaxDelay)
	b.openUntil = b.now().Add(delay)
	return delay, true
}

// Remaining is how long the breaker stays open; zero when closed.
func (b *Breaker) Remaining() time.Duration {
	return max(b.openUntil.Sub(b.now()), 0)
}

// Streak is the current number of consecutive errors.
func (b *Breaker) Streak() int {
	return b.streak
}
