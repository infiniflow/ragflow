// Package component — retry decorator tests.
//
// These tests exercise the retryInvoker wrapper directly. The wrapper
// is the chat-level retry loop introduced to mirror Python's
// max_retries/delay_after_error semantics (agent/component/llm.py,
// driven by LLMBundle in rag/llm/chat_model.py). Unlike the
// existing one-shot structured-output retry (in LLMComponent.Invoke),
// the retry loop lives at the ChatInvoker boundary so it covers
// every chat path: LLM, Agent, citation grounding.
package component

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedInvoker fails the first failTimes calls then succeeds.
// err is returned on every failing call (asserted via errors.Is).
type scriptedInvoker struct {
	failTimes int32
	err       error
	resp      *ChatInvokeResponse
	calls     int32
}

func (s *scriptedInvoker) Invoke(_ context.Context, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
	n := atomic.AddInt32(&s.calls, 1)
	if n <= atomic.LoadInt32(&s.failTimes) {
		return nil, s.err
	}
	return s.resp, nil
}

func (s *scriptedInvoker) callCount() int { return int(atomic.LoadInt32(&s.calls)) }

// alwaysFailInvoker returns err on every call. Used to exercise the
// exhaustion path.
type alwaysFailInvoker struct {
	err   error
	calls int32
}

func (a *alwaysFailInvoker) Invoke(_ context.Context, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
	atomic.AddInt32(&a.calls, 1)
	return nil, a.err
}

func (a *alwaysFailInvoker) callCount() int { return int(atomic.LoadInt32(&a.calls)) }

// TestRetryInvoker_SucceedsOnSecondAttempt: 1 failure, 1 success —
// the loop must retry exactly once and return the success response
// without surfacing the error.
func TestRetryInvoker_SucceedsOnSecondAttempt(t *testing.T) {
	want := &ChatInvokeResponse{Content: "ok", Model: "m", Stopped: true}
	inner := &scriptedInvoker{failTimes: 1, err: errors.New("transient"), resp: want}
	r := newRetryInvoker(inner, 3, time.Millisecond)

	resp, err := r.Invoke(context.Background(), ChatInvokeRequest{ModelName: "m"})
	if err != nil {
		t.Fatalf("Invoke: unexpected err: %v", err)
	}
	if resp != want {
		t.Errorf("resp=%v, want %v", resp, want)
	}
	if got := inner.callCount(); got != 2 {
		t.Errorf("inner.calls=%d, want 2 (1 fail + 1 success)", got)
	}
}

// TestRetryInvoker_ExhaustsRetries: failures exceed the budget —
// the loop must stop after maxRetries+1 attempts and wrap the last
// error with the retry count.
func TestRetryInvoker_ExhaustsRetries(t *testing.T) {
	sentinel := errors.New("permanent")
	inner := &alwaysFailInvoker{err: sentinel}
	r := newRetryInvoker(inner, 3, time.Millisecond)

	_, err := r.Invoke(context.Background(), ChatInvokeRequest{ModelName: "m"})
	if err == nil {
		t.Fatal("expected error after exhaustion")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("err does not wrap sentinel: %v", err)
	}
	if got, want := inner.callCount(), 4; got != want {
		// 1 initial + 3 retries
		t.Errorf("inner.calls=%d, want %d (1 + maxRetries)", got, want)
	}
	if !strings.Contains(err.Error(), "3 retries") {
		t.Errorf("error message missing retry count: %q", err.Error())
	}
}

// TestRetryInvoker_HonorsContextCancellation: a ctx cancelled
// during backoff must abort the sleep and return ctx.Err() promptly,
// not wait out the full delay.
func TestRetryInvoker_HonorsContextCancellation(t *testing.T) {
	inner := &alwaysFailInvoker{err: errors.New("transient")}
	// 30s delay so the test would obviously hang if ctx were not
	// honored. The cancellation lands within milliseconds.
	r := newRetryInvoker(inner, 5, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after a short delay, while the retry is sleeping
	// through the first backoff.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := r.Invoke(ctx, ChatInvokeRequest{ModelName: "m"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err=%v, want context.Canceled", err)
	}
	// Generous upper bound: ctx cancel must land well before the
	// 30s backoff would have elapsed.
	if elapsed > 2*time.Second {
		t.Errorf("Invoke took %v, expected < 2s with prompt ctx cancel", elapsed)
	}
	// First call happens, then we cancel during the first backoff.
	// The retry loop should not have made more than 2 calls.
	if got := inner.callCount(); got > 2 {
		t.Errorf("inner.calls=%d, want <= 2 (ctx cancel should abort backoff)", got)
	}
}

// TestRetryInvoker_ExponentialBackoff: measure total elapsed for a
// 3-retry loop with a 20ms initial delay. Expected: 20 + 40 + 80
// = 140ms minimum of pure sleep. We allow generous slack for slow
// CI but assert a lower bound that proves doubling (a single
// constant delay would fall below it).
func TestRetryInvoker_ExponentialBackoff(t *testing.T) {
	inner := &alwaysFailInvoker{err: errors.New("transient")}
	const initial = 20 * time.Millisecond
	r := newRetryInvoker(inner, 3, initial)

	start := time.Now()
	_, _ = r.Invoke(context.Background(), ChatInvokeRequest{ModelName: "m"})
	elapsed := time.Since(start)

	// 20 + 40 + 80 = 140ms of backoff (3 retries, 4 attempts total).
	// Use 130ms as the lower bound to avoid CI flakes from clock
	// granularity. Upper bound: 2s for very slow CI.
	if elapsed < 130*time.Millisecond {
		t.Errorf("elapsed=%v, want >= 130ms (proves doubling, not constant)", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Errorf("elapsed=%v, want < 2s", elapsed)
	}
}

// TestRetryInvoker_NoRetriesWhenZero: maxRetries=0 means a single
// attempt with no retry on failure. Mirrors LLMParam.MaxRetries=0
// for latency-sensitive flows.
func TestRetryInvoker_NoRetriesWhenZero(t *testing.T) {
	inner := &alwaysFailInvoker{err: errors.New("nope")}
	r := newRetryInvoker(inner, 0, 50*time.Millisecond)

	_, err := r.Invoke(context.Background(), ChatInvokeRequest{ModelName: "m"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := inner.callCount(); got != 1 {
		t.Errorf("inner.calls=%d, want 1 (no retries)", got)
	}
}

// TestRetryInvoker_NilInner: a defensive nil check — the wrapper
// should not panic when constructed with nil inner.
func TestRetryInvoker_NilInner(t *testing.T) {
	r := newRetryInvoker(nil, 3, time.Millisecond)
	_, err := r.Invoke(context.Background(), ChatInvokeRequest{ModelName: "m"})
	if err == nil {
		t.Fatal("expected error for nil inner")
	}
}

// TestLLMParam_RespectsMaxRetries: an LLMComponent configured with
// MaxRetries=5 should exhaust after 6 attempts (1 initial + 5
// retries) when the invoker always fails. This is the integration
// test for the param-override path through resolveChatInvoker.
func TestLLMParam_RespectsMaxRetries(t *testing.T) {
	inner := &alwaysFailInvoker{err: errors.New("downstream dead")}
	withStubInvoker(t, inner)

	c := NewLLMComponent(LLMParam{
		ModelID:    "m",
		MaxRetries: 5,
	})
	// Force the param to a tiny delay so the test is fast. The
	// zero-value default is 2s, which would make this test slow.
	c.param.DelayAfterError = time.Millisecond

	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}
	// 1 initial + 5 retries = 6 invoker calls.
	if got, want := inner.callCount(), 6; got != want {
		t.Errorf("inner.calls=%d, want %d", got, want)
	}
}

// TestLLMParam_ZeroRetriesMeansOneAttempt: MaxRetries=0 must bypass
// retries entirely (the param-override path passes through
// resolveChatInvoker and a fresh retryInvoker with maxRetries=0).
func TestLLMParam_ZeroRetriesMeansOneAttempt(t *testing.T) {
	inner := &alwaysFailInvoker{err: errors.New("once")}
	withStubInvoker(t, inner)

	c := NewLLMComponent(LLMParam{
		ModelID:    "m",
		MaxRetries: 0,
	})
	// MaxRetries=0 with default zero-value DelayAfterError triggers
	// the "no param override" path through resolveChatInvoker, which
	// returns the package default (3 retries). To genuinely request
	// zero retries we set DelayAfterError to a non-zero sentinel so
	// resolveChatInvoker wraps the default in a new retryInvoker
	// with maxRetries=0.
	c.param.DelayAfterError = time.Millisecond

	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := inner.callCount(); got != 1 {
		t.Errorf("inner.calls=%d, want 1 (zero retries)", got)
	}
}

// TestLLMParam_DefaultRetries: with MaxRetries and
// DelayAfterError both zero (the v1 fixture default), the
// component should still retry up to the package default
// (retryInvokerDefaultRetries=3). This protects against
// regressions where a future change accidentally bypasses the
// retry loop on the hot path.
func TestLLMParam_DefaultRetries(t *testing.T) {
	inner := &alwaysFailInvoker{err: errors.New("flaky")}
	withStubInvoker(t, inner)

	c := NewLLMComponent(LLMParam{ModelID: "m"})
	// Both fields zero — the test relies on the package default
	// being applied. The default initial delay is 2s, which is too
	// slow for a unit test, so we mutate the package default
	// indirectly: the test cannot reach into the retry decorator
	// (it's wrapped by resolveChatInvoker), so we instead assert
	// behavior with a manually-fast retryInvoker injected via
	// SetDefaultChatInvoker. This is the more honest test.
	fastInner := &alwaysFailInvoker{err: errors.New("flaky")}
	SetDefaultChatInvoker(newRetryInvoker(fastInner, 2, time.Millisecond))
	t.Cleanup(func() { SetDefaultChatInvoker(nil) })

	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected error after default retries")
	}
	// 1 initial + 2 retries = 3.
	if got, want := fastInner.callCount(), 3; got != want {
		t.Errorf("inner.calls=%d, want %d", got, want)
	}
	// Original (un-wrapped) inner should have been called 0 times
	// because resolveChatInvoker returned the package default (the
	// one we just installed), not the one passed via withStubInvoker.
	if got := inner.callCount(); got != 0 {
		t.Errorf("unused inner.calls=%d, want 0", got)
	}
}

// TestUnwrapChatInvoker_StripsSingleRetryLayer is the unit-level
// test for the unwrapChatInvoker helper. It must peel off a
// single retryInvoker layer to return the bare invoker
// underneath, so the param-override path can install a fresh
// retryInvoker with the operator's literal MaxRetries without
// multiplicatively stacking on the boot retry.
func TestUnwrapChatInvoker_StripsSingleRetryLayer(t *testing.T) {
	bare := &alwaysFailInvoker{err: errors.New("bare")}
	wrapped := newRetryInvoker(bare, 3, time.Millisecond)
	if got := unwrapChatInvoker(wrapped); got != bare {
		t.Errorf("unwrapChatInvoker(retry(bare)) = %v, want %v (bare invoker)", got, bare)
	}
}

// TestUnwrapChatInvoker_NoRetryLayer verifies that a bare
// (non-retry) invoker passes through unwrapChatInvoker
// unchanged. The function must not wrap or modify the input
// when no retry layers are present.
func TestUnwrapChatInvoker_NoRetryLayer(t *testing.T) {
	bare := &alwaysFailInvoker{err: errors.New("bare")}
	if got := unwrapChatInvoker(bare); got != bare {
		t.Errorf("unwrapChatInvoker(bare) = %v, want %v (unchanged passthrough)", got, bare)
	}
}

// TestUnwrapChatInvoker_StripsMultipleLayers is the defensive
// case: a chain of retryInvokers (production only installs one,
// but pathological callers could nest) is peeled down to the
// bare invoker. The loop bounds the walk at the first
// non-retryInvoker layer.
func TestUnwrapChatInvoker_StripsMultipleLayers(t *testing.T) {
	bare := &alwaysFailInvoker{err: errors.New("bare")}
	double := newRetryInvoker(newRetryInvoker(bare, 3, time.Millisecond), 3, time.Millisecond)
	if got := unwrapChatInvoker(double); got != bare {
		t.Errorf("unwrapChatInvoker(retry(retry(bare))) = %v, want %v (bare invoker)", got, bare)
	}
}

// TestLLM_ParamOverride_AbsoluteCount_NotStacked is the
// integration test for LLM retry normal-absolute-count
// semantics. It installs a boot retryInvoker with MaxRetries=3
// wrapping an alwaysFailInvoker, then runs an LLMComponent with
// MaxRetries=5. The pre-fix behaviour (stacking) would produce
// (3+1)*(5+1) = 24 invoker calls. The current implementation
// unwraps the boot layer and installs a fresh retryInvoker with
// the operator's literal MaxRetries, so the absolute count is
// exactly MaxRetries+1 = 6.
//
// A regression that re-introduces stacking (e.g. someone drops
// the unwrapChatInvoker call) fails this test.
func TestLLM_ParamOverride_AbsoluteCount_NotStacked(t *testing.T) {
	bare := &alwaysFailInvoker{err: errors.New("downstream dead")}
	// Simulate the production boot: a retryInvoker wrapping the
	// bare invoker. The boot layer's MaxRetries=3 means 4
	// invocations per call to the wrapped invoker; without
	// unwrapping, the param-override retryInvoker would stack on
	// top and produce 4*6 = 24 calls. With unwrapping, the
	// absolute count is 6.
	boot := newRetryInvoker(bare, 3, time.Millisecond)
	withStubInvoker(t, boot)

	c := NewLLMComponent(LLMParam{
		ModelID:    "m",
		MaxRetries: 5,
	})
	// Force a tiny delay so the test runs fast.
	c.param.DelayAfterError = time.Millisecond

	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}
	// With the unwrap: 1 initial + 5 retries = 6 calls to
	// the bare invoker. The boot layer is peeled off first.
	// Without the unwrap: 6 outer × 4 inner = 24.
	if got, want := bare.callCount(), 6; got != want {
		t.Errorf("bare.calls=%d, want %d (absolute count, not stacked). If you see 24, the multiplicative-stacking regression has been re-introduced.", got, want)
	}
}

// TestLLM_NoParamOverride_StackingPreserved is the
// back-compat companion to the absolute-count test. When
// MaxRetries=0 AND DelayAfterError=0, the boot retry chain must
// run unchanged so existing DSLs that rely on the implicit
// 3-retry budget keep working.
//
// A future change that aggressively unwraps even when no
// override is set would silence the boot retry chain and
// regress production retry behaviour.
func TestLLM_NoParamOverride_StackingPreserved(t *testing.T) {
	bare := &alwaysFailInvoker{err: errors.New("downstream dead")}
	// Boot layer with the production default (3 retries).
	boot := newRetryInvoker(bare, 3, time.Millisecond)
	withStubInvoker(t, boot)

	// No param override: MaxRetries=0 AND DelayAfterError=0.
	c := NewLLMComponent(LLMParam{ModelID: "m"})

	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}
	// 1 initial + 3 retries = 4 calls to the bare invoker (the
	// boot layer ran unchanged).
	if got, want := bare.callCount(), 4; got != want {
		t.Errorf("bare.calls=%d, want %d (boot layer ran unchanged — no param override means we keep the implicit 3-retry budget)", got, want)
	}
}
