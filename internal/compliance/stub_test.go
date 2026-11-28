//go:build !ee

package compliance

import (
	"context"
	"errors"
	"testing"
)

func TestOutputFilterHooksAreNoops(t *testing.T) {
	ctx := t.Context()
	if got, err := WithOutputFilter(ctx); got != ctx || err != nil {
		t.Fatalf("WithOutputFilter changed the context: %v", err)
	}
	if got, err := FilterOutput(ctx, "secret 机密"); got != "secret 机密" || err != nil {
		t.Fatalf("FilterOutput = %q, %v", got, err)
	}
	if FilterText(ctx, "secret 机密") != "secret 机密" || OutputFilterEnabled(ctx) {
		t.Fatal("community output filtering must pass text through")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if got, err := WithOutputFilter(canceled); got != canceled || !errors.Is(err, context.Canceled) {
		t.Fatalf("WithOutputFilter lost cancellation: %v", err)
	}
	if _, err := FilterOutput(canceled, "secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("FilterOutput lost cancellation: %v", err)
	}
}
