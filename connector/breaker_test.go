package connector

import (
	"testing"
	"time"
)

func TestBreakerBackoff(t *testing.T) {
	now := time.Unix(0, 0)
	b := NewBreaker(3, 30*time.Second)
	b.now = func() time.Time { return now }

	// Below the threshold: fixed retry delay, breaker stays closed.
	for i := 1; i < 3; i++ {
		delay, opened := b.RecordError()
		if opened || delay != 10*time.Second {
			t.Fatalf("error %d: got (%v, %v), want (10s, false)", i, delay, opened)
		}
	}

	// From the threshold on: 30s, 60s, 120s, 240s, 480s, then capped at 10m.
	want := []time.Duration{30, 60, 120, 240, 480, 600, 600}
	for i, w := range want {
		delay, opened := b.RecordError()
		if !opened || delay != w*time.Second {
			t.Fatalf("open step %d: got (%v, %v), want (%v, true)", i, delay, opened, w*time.Second)
		}
	}
	if got := b.Remaining(); got != 10*time.Minute {
		t.Fatalf("Remaining() = %v, want 10m", got)
	}

	now = now.Add(4 * time.Minute)
	if got := b.Remaining(); got != 6*time.Minute {
		t.Fatalf("Remaining() after 4m = %v, want 6m", got)
	}

	if prev := b.RecordSuccess(); prev != 9 {
		t.Fatalf("RecordSuccess() = %d, want 9", prev)
	}
	if got := b.Remaining(); got != 0 {
		t.Fatalf("Remaining() after success = %v, want 0", got)
	}
}

func TestBreakerLongStreakDoesNotOverflow(t *testing.T) {
	b := NewBreaker(1, 30*time.Second)
	var delay time.Duration
	for range 500 {
		delay, _ = b.RecordError()
	}
	if delay != 10*time.Minute {
		t.Fatalf("delay after 500 errors = %v, want 10m", delay)
	}
}
