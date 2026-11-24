package doctype

import (
	"context"
	"testing"
)

// BenchmarkInferenceGateAcquireRelease guards the hot-path cost of the
// process-wide gate. Every inference call (DLA/TSR/OCR, in-process or remote)
// acquires and releases one slot, so this must stay allocation-free and O(1).
// Capacity 16 matches a typical budget; the acquire path is uncontended here,
// measuring the steady-state enqueue/dequeue cost.
func BenchmarkInferenceGateAcquireRelease(b *testing.B) {
	restore := swapProcessInferenceLimiter(newInferenceLimiter(16))
	defer restore()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := AcquireInference(ctx); err != nil {
			b.Fatal(err)
		}
		ReleaseInference()
	}
}
