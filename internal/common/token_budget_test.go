package common

import (
	"context"
	"errors"
	"testing"
)

// TestChargeTokensWithoutBudgetIsNoop pins that the shared response handler can
// charge unconditionally: ordinary chats carry no budget and must be unaffected.
func TestChargeTokensWithoutBudgetIsNoop(t *testing.T) {
	ctx := context.Background()
	ChargeTokens(ctx, 1_000_000)
	ChargeTokens(context.TODO(), 5)
	if got := TokenBudgetUsed(ctx); got != 0 {
		t.Fatalf("TokenBudgetUsed = %d, want 0 outside a budget", got)
	}
	LogTokenBudget(ctx) // must not panic
}

// TestChargeTokensCancelsAtLimit is the whole contract: the run stops the moment
// the budget is gone, and it stops with a cause that says why.
func TestChargeTokensCancelsAtLimit(t *testing.T) {
	parent := context.Background()
	ctx := WithTokenBudget(parent, 100)

	ChargeTokens(ctx, 60)
	if err := ctx.Err(); err != nil {
		t.Fatalf("cancelled at 60/100: %v", err)
	}
	ChargeTokens(ctx, 40) // exactly at the limit
	select {
	case <-ctx.Done():
	default:
		t.Fatal("budget exhausted but the run was not cancelled")
	}
	if cause := context.Cause(ctx); !errors.Is(cause, ErrTokenBudget) {
		t.Fatalf("cause = %v, want ErrTokenBudget", cause)
	}
	if got := TokenBudgetUsed(ctx); got != 100 {
		t.Fatalf("TokenBudgetUsed = %d, want 100", got)
	}
	// The parent must be untouched: the budget stops one run, not the caller.
	if err := parent.Err(); err != nil {
		t.Fatalf("parent context was cancelled: %v", err)
	}
}

// TestChargeTokensOvershootCountsOnce guards the log: one exhausted budget must
// not emit a line per following token charge.
func TestChargeTokensOvershootCountsOnce(t *testing.T) {
	ctx := WithTokenBudget(context.Background(), 50)
	ChargeTokens(ctx, 50)
	for i := 0; i < 10; i++ {
		ChargeTokens(ctx, 10)
	}
	if got := TokenBudgetUsed(ctx); got != 150 {
		t.Fatalf("TokenBudgetUsed = %d, want 150", got)
	}
	if !errors.Is(context.Cause(ctx), ErrTokenBudget) {
		t.Fatal("cause should stay ErrTokenBudget after repeated charges")
	}
}

// TestWithTokenBudgetNonPositiveIsUnset keeps "0 disables" true, so a config
// that means "no ceiling" cannot quietly install a zero-token ceiling that
// cancels every run on its first model call.
func TestWithTokenBudgetNonPositiveIsUnset(t *testing.T) {
	for _, limit := range []int64{0, -1} {
		ctx := WithTokenBudget(context.Background(), limit)
		ChargeTokens(ctx, 10_000)
		if err := ctx.Err(); err != nil {
			t.Fatalf("limit %d cancelled the run: %v", limit, err)
		}
		if got := TokenBudgetUsed(ctx); got != 0 {
			t.Fatalf("limit %d recorded %d tokens, want 0", limit, got)
		}
	}
}
