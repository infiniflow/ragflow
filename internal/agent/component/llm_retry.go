// Package component — retry decorator for ChatInvoker.
//
// retryInvoker wraps any ChatInvoker and retries it on error with
// exponential backoff. It mirrors Python's
//
//	for attempt in range(max_retries + 1):
//	    try: return self._chat(...)
//	    except: time.sleep(delay * 2 ** attempt)
//
// semantics from agent/component/llm.py (the actual loop is hidden
// inside LLMBundle / Base._chat; for the Go port we implement it
// directly at the ChatInvoker boundary so every chat path
// — LLMComponent, Agent component, citation grounding — benefits
// without touching the call sites).
//
// Defaults (set by getDefaultChatInvoker): 3 retries, 2s initial
// delay. The defaults match LLMParam's zero-value semantics: a
// caller who leaves both fields unset still gets retries, mirroring
// Python's LLMBundle max_retries=5 / base_delay=2.0 closely enough
// for the Go port (we use 3/2s to keep tests snappy; per-call
// overrides flow through LLMComponent.Invoke).
package component

import (
	"context"
	"fmt"
	"time"

	"ragflow/internal/common"

	"gorm.io/gorm"
)

// retryInvoker decorates a ChatInvoker with exponential-backoff
// retries. The zero-value fields disable the loop (a maxRetries of
// 0 means "no retries, single attempt") which matches LLMParam
// semantics: 0 → call exactly once.
type retryInvoker struct {
	inner        ChatInvoker
	maxRetries   int
	initialDelay time.Duration
}

// Unwrap returns the inner ChatInvoker wrapped by this
// retryInvoker. The intended use is LLMParam's param-override
// path: when a DSL sets LLMParam.MaxRetries explicitly, the
// production boot's retryInvoker is also wrapping the
// einoChatInvoker, and the two loops would otherwise
// multiplicatively stack (boot=3, MaxRetries=5 → up to
// (3+1)*(5+1) = 24 invocations). LLMComponent.Invoke walks the
// Unwrap chain to find the bare invoker, then wraps that with
// the operator's literal MaxRetries so the absolute count is
// MaxRetries+1 regardless of the boot layer.
//
// Unwrap returns nil when the inner invoker is nil (a
// defensive call-site convenience).
func (r *retryInvoker) Unwrap() ChatInvoker {
	if r == nil {
		return nil
	}
	return r.inner
}

// retryInvokerBackoff is the default backoff used when the param
// leaves DelayAfterError at its zero value. Matches Python's
// LLM_BASE_DELAY=2.0.
const retryInvokerBackoff = 2 * time.Second

// retryInvokerDefaultRetries matches Python's LLMBundle default of
// 5, but the Go port uses 3 to keep the test suite fast while still
// demonstrating the loop. Users override via LLMParam.MaxRetries.
const retryInvokerDefaultRetries = 3

// newRetryInvoker wraps inner in a retry loop with the given
// parameters. maxRetries <= 0 yields a single attempt; initialDelay
// <= 0 results in no delay between retries.
func newRetryInvoker(inner ChatInvoker, maxRetries int, initialDelay time.Duration) *retryInvoker {
	if maxRetries < 0 {
		maxRetries = 0
	}
	if initialDelay < 0 {
		initialDelay = 0
	}
	return &retryInvoker{
		inner:        inner,
		maxRetries:   maxRetries,
		initialDelay: initialDelay,
	}
}

// unwrapChatInvoker walks the ChatInvoker chain, peeling off any
// retryInvoker layers to return the bare invoker underneath. Used
// by LLMComponent.Invoke's param-override path to install a fresh
// retryInvoker with the operator's literal MaxRetries without
// multiplicatively stacking on top of the boot retry layer.
//
// Returns the input unchanged when no retryInvoker layers are
// present. Returns the unwrapped invoker (which may itself be a
// retryInvoker wrapping another retryInvoker — production only
// installs one layer so a single-level walk is sufficient, but
// the loop handles pathological cases for safety).
func unwrapChatInvoker(inv ChatInvoker) ChatInvoker {
	for {
		if r, ok := inv.(*retryInvoker); ok && r != nil {
			if r.Unwrap() == nil {
				return inv
			}
			inv = r.Unwrap()
			continue
		}
		return inv
	}
}

// Invoke satisfies ChatInvoker. It calls the inner invoker up to
// maxRetries+1 times (one initial attempt + maxRetries retries),
// sleeping initialDelay * 2^attempt between failures. The sleep
// honours ctx cancellation: a cancelled context aborts the backoff
// and returns ctx.Err() immediately.
func (r *retryInvoker) Invoke(ctx context.Context, db *gorm.DB, req ChatInvokeRequest) (*ChatInvokeResponse, error) {
	if r.inner == nil {
		return nil, fmt.Errorf("component: retryInvoker: nil inner")
	}
	var resp *ChatInvokeResponse
	err := common.RetryWithBackoff(ctx, r.maxRetries, r.initialDelay, func() error {
		r, e := r.inner.Invoke(ctx, db, req)
		resp = r
		return e
	})
	// On failure, return nil (not the last partial response) so callers
	// that check err first never dereference a half-formed resp.
	if err != nil {
		return nil, err
	}
	return resp, nil
}
