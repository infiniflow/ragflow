//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

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
		lowQuarter, highQuarter := ceiling/4, ceiling*3/4
		var reachedLow, reachedHigh bool
		for i := 0; i < 500; i++ {
			got := backoff(base, max, attempt)
			if got < 0 || got > ceiling {
				t.Fatalf("%s: backoff(_, _, %d) = %v out of [0,%v]", name, attempt, got, ceiling)
			}
			if got <= lowQuarter {
				reachedLow = true
			}
			if got >= highQuarter {
				reachedHigh = true
			}
			seen[got] = true
		}
		if len(seen) < 2 {
			t.Fatalf("%s: backoff must apply full jitter, got values %v", name, seen)
		}
		// Statistical envelope: a loss of full jitter (e.g. upper-half-only)
		// or a wrong exponential ceiling would leave one quarter unreachable.
		if !reachedLow || !reachedHigh {
			t.Fatalf("%s: jitter must span both quarters of [0,%v]; reachedLow=%v reachedHigh=%v",
				name, ceiling, reachedLow, reachedHigh)
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
