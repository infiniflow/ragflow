package doctype

import (
	"context"
	"testing"
	"time"
)

// Tests for the in-process (local) session gate and its capacity derivation.
// Two-layer budget: the outer gate (AcquireInference) bounds ALL inference calls
// at K (including remote HTTP clients); this inner local gate bounds only
// in-process ONNX Runs at min(K, cores) so local inference never oversubscribes
// the machine. Remote clients never pass through the local gate.

func TestLocalInferenceCapacityClampsToCores(t *testing.T) {
	cases := []struct {
		name  string
		k     int
		cores int
		want  int
	}{
		{"K exceeds cores -> clamp to cores", 32, 16, 16},
		{"K below cores -> use K", 8, 16, 8},
		{"K equals cores", 16, 16, 16},
		{"zero K means all cores", 0, 16, 16},
		{"zero cores floor to one", 32, 0, 1},
		{"K one stays one", 1, 16, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LocalInferenceCapacity(tc.k, tc.cores); got != tc.want {
				t.Fatalf("LocalInferenceCapacity(%d, %d) = %d, want %d", tc.k, tc.cores, got, tc.want)
			}
		})
	}
}

// Adversarial: a full local gate must NOT block the outer/remote gate, and vice
// versa. A single shared gate (the pre-change behaviour) fails here.
func TestLocalAndRemoteGatesAreIndependent(t *testing.T) {
	restoreLocal := swapLocalLimiter(newInferenceLimiter(1))
	restoreRemote := swapProcessInferenceLimiter(newInferenceLimiter(1))
	defer restoreLocal()
	defer restoreRemote()
	ctx := context.Background()

	// Hold the only LOCAL slot.
	if err := AcquireLocal(ctx); err != nil {
		t.Fatalf("hold local slot: %v", err)
	}
	// The REMOTE gate must still be acquirable (its own slot is free).
	remoteAcquired := make(chan error, 1)
	go func() { remoteAcquired <- AcquireInference(ctx) }()
	select {
	case err := <-remoteAcquired:
		if err != nil {
			t.Fatalf("remote acquire while local gate full: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("remote acquire blocked while local gate was full (gates not independent)")
	}
	ReleaseInference()
	ReleaseLocal()
}

// Adversarial: with the only local slot held, a second local acquire MUST block.
func TestLocalGateQueuesWhenExhausted(t *testing.T) {
	restore := swapLocalLimiter(newInferenceLimiter(1))
	defer restore()
	ctx := context.Background()
	if err := AcquireLocal(ctx); err != nil {
		t.Fatalf("first local acquire: %v", err)
	}
	second := make(chan error, 1)
	go func() { second <- AcquireLocal(ctx) }()
	select {
	case err := <-second:
		ReleaseLocal()
		t.Fatalf("second local acquire returned (%v) while only slot held", err)
	case <-time.After(200 * time.Millisecond):
	}
	ReleaseLocal()
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second local acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second local acquire did not proceed after release")
	}
	ReleaseLocal()
}

// Adversarial (error path): blocked local acquire aborts on ctx cancellation.
func TestLocalGateHonorsContextCancellation(t *testing.T) {
	restore := swapLocalLimiter(newInferenceLimiter(1))
	defer restore()
	if err := AcquireLocal(context.Background()); err != nil {
		t.Fatalf("hold local slot: %v", err)
	}
	defer ReleaseLocal()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- AcquireLocal(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled local acquire returned nil, want error")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled local acquire did not return")
	}
}

// Adversarial (once semantics, mirrors the outer gate): repeated
// SetLocalInferenceLimit must not change an already-registered capacity.
// Without once, a stray late caller could silently enlarge the local gate and
// break the no-oversubscription guarantee.
func TestLocalGateCapacityFixedAfterFirstSet(t *testing.T) {
	// Isolate from other tests by swapping to nil first; then drive the real
	// setter path. Note: sync.Once is process-global, so this test assumes it
	// runs before any other test calls SetLocalInferenceLimit (none do).
	restore := swapLocalLimiter(nil)
	defer restore()
	SetLocalInferenceLimit(5)
	first := localCapacity()
	SetLocalInferenceLimit(20)
	second := localCapacity()
	if first != 5 || second != 5 {
		t.Fatalf("local capacity changed across SetLocalInferenceLimit: first=%d second=%d, want 5/5", first, second)
	}
}

func localCapacity() int {
	if localLimiterInst == nil {
		return 0
	}
	return cap(localLimiterInst.slots)
}
