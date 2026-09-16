package infinity

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"ragflow/internal/common"
)

// TestPoolExhaustedError_Contract exercises the real poolExhaustedError type
// that checkoutConn returns when the connection pool is saturated. It is the
// exact value the admin PingEngine handler surfaces as HTTP 503, so it must
// keep reporting CodeConnectionPoolExhausted. No live Infinity is required.
func TestPoolExhaustedError_Contract(t *testing.T) {
	pe := &poolExhaustedError{caller: "SearchMetadata"}

	if pe.Code() != common.CodeConnectionPoolExhausted {
		t.Fatalf("Code() = %v, want %v", pe.Code(), common.CodeConnectionPoolExhausted)
	}

	wantMsg := "Infinity connection pool exhausted while SearchMetadata"
	if pe.Error() != wantMsg {
		t.Fatalf("Error() = %q, want %q", pe.Error(), wantMsg)
	}
	if pe.Message() != wantMsg {
		t.Fatalf("Message() = %q, want %q", pe.Message(), wantMsg)
	}

	// When wrapped, errors.As must still extract it and keep the same code.
	wrapped := fmt.Errorf("outer: %w", pe)
	var extracted *poolExhaustedError
	if !errors.As(wrapped, &extracted) {
		t.Fatalf("errors.As failed to extract *poolExhaustedError from %v", wrapped)
	}
	if extracted.Code() != common.CodeConnectionPoolExhausted {
		t.Fatalf("extracted Code() = %v, want %v", extracted.Code(), common.CodeConnectionPoolExhausted)
	}
}

// TestEnsureDeadline covers the core of the bounded-latency fix: callers that
// pass context.Background() (no deadline) must get one applied, while callers
// that already carry a deadline are left untouched and their cancel is not
// hijacked by the no-op cancel returned for them.
func TestEnsureDeadline(t *testing.T) {
	// nil ctx -> gets a deadline and a real cancel func.
	ctx, cancel := ensureDeadline(nil, 50*time.Millisecond)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("expected deadline on ctx built from nil")
	}

	// ctx already carrying a deadline -> returned unchanged with a no-op cancel.
	base, baseCancel := context.WithTimeout(context.Background(), time.Second)
	defer baseCancel()
	got, noop := ensureDeadline(base, 50*time.Millisecond)
	if got != base {
		t.Fatal("expected same context when a deadline already exists")
	}
	noop() // must be safe to call and must not cancel the caller's context.
	select {
	case <-got.Done():
		t.Fatal("no-op cancel must not cancel the caller's context")
	default:
	}

	// plain Background -> gets a deadline applied.
	bctx, bcancel := ensureDeadline(context.Background(), 50*time.Millisecond)
	defer bcancel()
	if _, ok := bctx.Deadline(); !ok {
		t.Fatal("expected deadline on Background-derived ctx")
	}
}
