package component

import (
	"context"
	"errors"
	"testing"
)

func TestOCRMediaAdmissionCancellationReleasesWaiter(t *testing.T) {
	admission := newOCRMediaAdmission(1)
	release, err := admission.acquire(t.Context())
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := admission.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting acquire error = %v, want context.Canceled", err)
	}

	release()
	release()
	releaseAgain, err := admission.acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	releaseAgain()
}
