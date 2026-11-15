package common

import (
	"context"
	"errors"
	"sync/atomic"

	"go.uber.org/zap"
)

// ErrTokenBudget is the cancellation cause when a run spends its whole budget.
// It is a cause rather than a plain context.Canceled so a log reader can tell
// "we stopped this run on purpose" from "the caller hung up".
var ErrTokenBudget = errors.New("agentic: per-run token budget exceeded")

// tokenBudgetKey scopes a budget to one run's context tree.
type tokenBudgetKey struct{}

// tokenBudget counts the tokens a single agentic run has spent and cancels that
// run once the budget is gone.
//
// Why a budget on top of the turn cap: a turn cap bounds how many times the
// model is asked, but says nothing about how much each asking costs. Measured
// runs show the two are not interchangeable — one question ran 23 turns on 2.2M
// tokens while another ran 15 turns on 275k, so a turn cap of 16 would have
// stopped the second and left the first far past any sensible cost. The budget
// bounds the quantity the bill is actually made of.
type tokenBudget struct {
	limit  int64
	used   atomic.Int64
	cancel context.CancelCauseFunc
	fired  atomic.Bool
}

// WithTokenBudget attaches a budget to ctx. A non-positive limit leaves ctx
// untouched, so "unset" and "no budget" are the same thing.
func WithTokenBudget(ctx context.Context, limit int64) context.Context {
	if limit <= 0 {
		return ctx
	}
	cancellable, cancel := context.WithCancelCause(ctx)
	return context.WithValue(cancellable, tokenBudgetKey{}, &tokenBudget{
		limit:  limit,
		cancel: cancel,
	})
}

func tokenBudgetFrom(ctx context.Context) *tokenBudget {
	if ctx == nil {
		return nil
	}
	b, _ := ctx.Value(tokenBudgetKey{}).(*tokenBudget)
	return b
}

// ChargeTokens records n tokens spent by the run ctx belongs to, and cancels
// that run the moment the budget is exhausted.
//
// It is a no-op outside a budgeted run, so every caller can call it
// unconditionally: the usage site lives in the model response handler, which
// serves ordinary chats and benchmark runs alike.
func ChargeTokens(ctx context.Context, n int) {
	b := tokenBudgetFrom(ctx)
	if b == nil || n <= 0 {
		return
	}
	used := b.used.Add(int64(n))
	if used < b.limit {
		return
	}
	if !b.fired.CompareAndSwap(false, true) {
		return
	}
	InfoCtx(ctx, "agentic_rag: token budget exhausted, forcing an answer",
		zap.Int64("limit", b.limit),
		zap.Int64("used", used),
	)
	// Cancelling the run is what "force an answer" means here: the agent's loop
	// unwinds, and the terminating fallback — which detaches from this very
	// cancellation — synthesizes from the evidence already gathered. Killing the
	// loop without that fallback would turn a budget stop into an empty answer.
	b.cancel(ErrTokenBudget)
}

// LogTokenBudget reports what a finished run spent, so a run that hit the
// budget is visible in the log rather than inferable from a low token count.
func LogTokenBudget(ctx context.Context) {
	b := tokenBudgetFrom(ctx)
	if b == nil {
		return
	}
	InfoCtx(ctx, "agentic_rag: token budget",
		zap.Int64("limit", b.limit),
		zap.Int64("used", b.used.Load()),
		zap.Bool("fired", b.fired.Load()),
	)
}

// TokenBudgetUsed reports the tokens charged so far, or 0 when unbudgeted.
func TokenBudgetUsed(ctx context.Context) int64 {
	if b := tokenBudgetFrom(ctx); b != nil {
		return b.used.Load()
	}
	return 0
}
