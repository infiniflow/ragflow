package sandbox

import (
	"testing"
	"time"
)

// TestBackoffCharacterization pins the current contract: the capped
// exponential base*2^(attempt-1) is computed first (attempt is 1-indexed and
// clamped to 1), then FULL JITTER is applied via rand.Int64N(d+1). The jitter
// is load-bearing - callers rely on it to avoid a thundering herd - so it must
// survive a refactor to the shared ExponentialBackoff primitive.
func TestBackoffCharacterization(t *testing.T) {
	base := 100 * time.Millisecond
	max := time.Second

	checkRange := func(name string, attempt int, ceiling time.Duration) {
		t.Helper()
		seen := make(map[time.Duration]bool)
		for i := 0; i < 300; i++ {
			got := backoff(base, max, attempt)
			if got < 0 || got > ceiling {
				t.Fatalf("%s: backoff(_, _, %d) = %v out of [0,%v]", name, attempt, got, ceiling)
			}
			seen[got] = true
		}
		if len(seen) < 2 {
			t.Fatalf("%s: backoff must apply full jitter, got values %v", name, seen)
		}
	}

	checkRange("attempt=1", 1, base)
	checkRange("attempt=2", 2, 2*base)
	checkRange("attempt clamped to 1", 0, base)
	checkRange("capped at max", 10, max)

	if got := backoff(0, max, 1); got != 0 {
		t.Fatalf("zero base = %v, want 0", got)
	}
}
