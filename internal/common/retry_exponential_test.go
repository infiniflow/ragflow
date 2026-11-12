package common

import (
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	t.Parallel()
	const max = time.Minute
	cases := []struct {
		name    string
		base    time.Duration
		attempt int
		want    time.Duration
	}{
		{"attempt1", 100 * time.Millisecond, 1, 100 * time.Millisecond},
		{"attempt2", 100 * time.Millisecond, 2, 200 * time.Millisecond},
		{"attempt3", 100 * time.Millisecond, 3, 400 * time.Millisecond},
		{"attempt4", 100 * time.Millisecond, 4, 800 * time.Millisecond},
		// 2^10*100ms = 102.4s exceeds the 1-minute cap, so it must be clamped.
		{"attempt11_capped", 100 * time.Millisecond, 11, max},
		// A very large attempt must stay at the cap and never overflow.
		{"attempt1000_capped", 100 * time.Millisecond, 1000, max},
		// attempt <= 0 is treated as attempt 1.
		{"attempt0", 100 * time.Millisecond, 0, 100 * time.Millisecond},
		{"attemptNeg", 100 * time.Millisecond, -5, 100 * time.Millisecond},
		// base larger than max is clamped immediately.
		{"baseOverMax", 2 * time.Minute, 1, max},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExponentialBackoff(c.base, max, c.attempt)
			if got != c.want {
				t.Errorf("ExponentialBackoff(%v, %v, %d) = %v, want %v", c.base, max, c.attempt, got, c.want)
			}
			if got > max {
				t.Errorf("ExponentialBackoff returned %v > max %v (uncapped backoff bug)", got, max)
			}
		})
	}
}

// TestExponentialBackoff_UnboundedBackoffBug reproduces the tool_invoke.go
// regression where `backoff *= 2` grew without a ceiling: no attempt index may
// exceed the cap.
func TestExponentialBackoff_UnboundedBackoffBug(t *testing.T) {
	t.Parallel()
	for attempt := 1; attempt <= 100; attempt++ {
		if got := ExponentialBackoff(time.Millisecond, time.Minute, attempt); got > time.Minute {
			t.Fatalf("attempt %d returned %v > 1-minute cap", attempt, got)
		}
	}
}
