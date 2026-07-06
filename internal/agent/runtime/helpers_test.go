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

package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// recordingCallback is a thread-safe ProgressCallback recorder used by
// the TrackProgress tests. progress/message pairs are appended in
// invocation order so tests can assert the exact call sequence.
type recordingCallback struct {
	mu      sync.Mutex
	calls   []recordedCall
	started bool
}

type recordedCall struct {
	progress int
	message  string
}

func (r *recordingCallback) callback(progress int, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, recordedCall{progress: progress, message: message})
}

func (r *recordingCallback) callsCopy() []recordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// --- TrackProgress ---

func TestTrackProgress_Success(t *testing.T) {
	rec := &recordingCallback{}
	err := TrackProgress("Parser", rec.callback, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	calls := rec.callsCopy()
	if len(calls) != 2 {
		t.Fatalf("expected 2 callback invocations, got %d: %+v", len(calls), calls)
	}
	if calls[0].progress != 0 || calls[0].message != "Parser Started" {
		t.Errorf("first call = %+v, want progress=0 message=%q", calls[0], "Parser Started")
	}
	if calls[1].progress != 1 || calls[1].message != "Parser Done" {
		t.Errorf("second call = %+v, want progress=1 message=%q", calls[1], "Parser Done")
	}
}

func TestTrackProgress_Failure(t *testing.T) {
	rec := &recordingCallback{}
	wantErr := errors.New("boom")
	err := TrackProgress("Tokenizer", rec.callback, func() error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
	calls := rec.callsCopy()
	if len(calls) != 2 {
		t.Fatalf("expected 2 callback invocations, got %d", len(calls))
	}
	if calls[0].progress != 0 || calls[0].message != "Tokenizer Started" {
		t.Errorf("first call = %+v, want progress=0 message=%q", calls[0], "Tokenizer Started")
	}
	if calls[1].progress != -1 {
		t.Errorf("second call progress = %d, want -1", calls[1].progress)
	}
	if !strings.Contains(calls[1].message, "Tokenizer") || !strings.Contains(calls[1].message, "boom") {
		t.Errorf("second call message = %q, want it to contain both %q and %q", calls[1].message, "Tokenizer", "boom")
	}
}

func TestTrackProgress_NilCallback(t *testing.T) {
	// Must not panic with a nil callback; must still pass fn's result through.
	called := false
	if err := TrackProgress("File", nil, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("unexpected error from nil-cb success path: %v", err)
	}
	if !called {
		t.Fatal("fn was not invoked")
	}

	wantErr := errors.New("nil-cb err")
	got := TrackProgress("File", nil, func() error {
		return wantErr
	})
	if !errors.Is(got, wantErr) {
		t.Fatalf("nil-cb failure path: got %v, want %v", got, wantErr)
	}
}

// TestTrackProgress_PassesThroughReturnValue covers the documented contract
// that the error returned to the caller is fn's error verbatim (wrapped
// only by the message-formatting for the callback, not for the return).
func TestTrackProgress_PassesThroughReturnValue(t *testing.T) {
	rec := &recordingCallback{}

	// nil path
	if err := TrackProgress("Foo", rec.callback, func() error { return nil }); err != nil {
		t.Fatalf("nil error not propagated as nil: %v", err)
	}

	// err path — exact identity preserved
	want := errors.New("exact")
	got := TrackProgress("Foo", rec.callback, func() error { return want })
	if got != want {
		t.Fatalf("err not propagated by identity: got %v (%T), want %v (%T)", got, got, want, want)
	}

	// cb saw the failure with progress=-1
	var last recordedCall
	for _, c := range rec.callsCopy() {
		last = c
	}
	if last.progress != -1 {
		t.Errorf("final cb call progress = %d, want -1", last.progress)
	}
}

// --- WithTimeout ---

func TestWithTimeout_Success(t *testing.T) {
	ctx := context.Background()
	err := WithTimeout(ctx, 50*time.Millisecond, func(ctx context.Context) error {
		// simulate fast work
		time.Sleep(5 * time.Millisecond)
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWithTimeout_Timeout(t *testing.T) {
	ctx := context.Background()
	start := time.Now()
	err := WithTimeout(ctx, 20*time.Millisecond, func(ctx context.Context) error {
		// sleep long enough to outlast the timeout; honor ctx so the
		// test doesn't have to wait the full duration.
		select {
		case <-time.After(500 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
	if elapsed > 250*time.Millisecond {
		t.Errorf("WithTimeout waited too long after deadline (%s) — fn should have observed ctx.Done() quickly", elapsed)
	}
}

func TestWithTimeout_ParentCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	observed := make(chan error, 1)
	start := time.Now()
	err := WithTimeout(parent, 5*time.Second, func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			observed <- ctx.Err()
			return ctx.Err()
		case <-time.After(2 * time.Second):
			observed <- nil
			return nil
		}
	})
	elapsed := time.Since(start)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	select {
	case inner := <-observed:
		if !errors.Is(inner, context.Canceled) {
			t.Errorf("fn observed ctx.Err() = %v, want context.Canceled", inner)
		}
	case <-time.After(time.Second):
		t.Fatal("fn never observed ctx.Done()")
	}
	if elapsed > 250*time.Millisecond {
		t.Errorf("WithTimeout took %s after parent cancel — expected fast exit", elapsed)
	}
}

// TestWithTimeout_PassesContextToFn verifies the ctx fn receives is a
// CHILD of the parent (not the parent itself). The child should carry
// the parent's Values but have its own Done channel tied to the
// timeout deadline. We probe captured properties from inside fn
// (NOT after WithTimeout returns) because WithTimeout's deferred
// cancel() will mark the child ctx as canceled once it returns —
// which is the documented contract of context.WithTimeout, not a
// helper bug.
func TestWithTimeout_PassesContextToFn(t *testing.T) {
	type ctxKey struct{}
	parent := context.WithValue(context.Background(), ctxKey{}, "v")
	type captured struct {
		ctx         context.Context
		errInFlight error
		hasDeadline bool
		deadline    time.Time
	}
	var cap captured
	err := WithTimeout(parent, 100*time.Millisecond, func(ctx context.Context) error {
		cap.ctx = ctx
		cap.errInFlight = ctx.Err()
		cap.deadline, cap.hasDeadline = ctx.Deadline()
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cap.ctx == nil {
		t.Fatal("fn did not receive a context")
	}
	if cap.ctx == parent {
		t.Fatal("fn received the parent ctx directly — expected a derived child ctx")
	}
	if v, _ := cap.ctx.Value(ctxKey{}).(string); v != "v" {
		t.Errorf("child ctx did not carry parent's Value(): got %q, want %q", v, "v")
	}
	if cap.errInFlight != nil {
		t.Errorf("child ctx should not be done while fn is still running successfully, got Err=%v", cap.errInFlight)
	}
	if !cap.hasDeadline {
		t.Error("child ctx has no Deadline — expected one from WithTimeout")
	}
	if !time.Now().Before(cap.deadline) {
		t.Errorf("child ctx deadline %v is in the past", cap.deadline)
	}
}

// --- TrackElapsed ---

func TestTrackElapsed_AddsCreatedAndElapsedFields(t *testing.T) {
	got, err := TrackElapsed("Parser", func() (map[string]any, error) {
		time.Sleep(5 * time.Millisecond)
		return map[string]any{"chunks": 3}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["_created_time"]; !ok {
		t.Fatal("result missing _created_time")
	}
	ct, ok := got["_created_time"].(string)
	if !ok || ct == "" {
		t.Fatalf("_created_time = %v (type %T), want non-empty string", got["_created_time"], got["_created_time"])
	}
	if _, err := time.Parse(time.RFC3339Nano, ct); err != nil {
		t.Errorf("_created_time %q is not RFC3339Nano: %v", ct, err)
	}
	elapsed, ok := got["_elapsed_time"].(float64)
	if !ok {
		t.Fatalf("_elapsed_time = %v (type %T), want float64", got["_elapsed_time"], got["_elapsed_time"])
	}
	if elapsed < 0 {
		t.Errorf("_elapsed_time = %f, want >= 0", elapsed)
	}
	// We slept 5ms; elapsed should be in a reasonable range (loose bound
	// to keep the test stable on noisy CI runners).
	if elapsed < 0.001 {
		t.Errorf("_elapsed_time = %f, expected >= ~0.005 after 5ms sleep", elapsed)
	}
}

func TestTrackElapsed_PreservesExistingKeys(t *testing.T) {
	in := map[string]any{"x": 1, "name": "kept"}
	got, err := TrackElapsed("Tokenizer", func() (map[string]any, error) {
		return in, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["x"] != 1 {
		t.Errorf("existing key x = %v, want 1", got["x"])
	}
	if got["name"] != "kept" {
		t.Errorf("existing key name = %v, want %q", got["name"], "kept")
	}
	if _, ok := got["_created_time"]; !ok {
		t.Error("missing _created_time")
	}
	if _, ok := got["_elapsed_time"]; !ok {
		t.Error("missing _elapsed_time")
	}
}

func TestTrackElapsed_PropagatesError(t *testing.T) {
	want := errors.New("downstream boom")
	got, err := TrackElapsed("Extractor", func() (map[string]any, error) {
		return map[string]any{"partial": true}, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want wraps %v", err, want)
	}
	if got != nil {
		t.Errorf("result map = %+v, want nil when fn errors", got)
	}
	// name parameter captured in the error message (documented
	// in the TrackElapsed package doc: on error, `name` is
	// recorded in the error message so log readers can attribute
	// the failure to a specific component).
	if !strings.Contains(err.Error(), "Extractor") {
		t.Errorf("err message %q should mention the component name %q", err.Error(), "Extractor")
	}
}

// TestTrackElapsed_NameParameterRecorded verifies that `name` appears
// somewhere observable — we chose to surface it in the error message
// on failure (see TrackElapsed doc).
func TestTrackElapsed_NameParameterRecorded(t *testing.T) {
	// On failure path: name is in the error message.
	_, err := TrackElapsed("MyComp", func() (map[string]any, error) {
		return nil, errors.New("nope")
	})
	if err == nil || !strings.Contains(err.Error(), "MyComp") {
		t.Fatalf("name not recorded on error path: err=%v", err)
	}

	// On success path: name is not part of the output map (per the
	// chosen design — name appears in error messages only). We
	// document this here so future maintainers don't expect it in
	// the success map.
	out, err := TrackElapsed("MyComp", func() (map[string]any, error) {
		return map[string]any{}, nil
	})
	if err != nil {
		t.Fatalf("unexpected error on success path: %v", err)
	}
	for k := range out {
		if strings.Contains(k, "MyComp") {
			t.Errorf("success-path map contains key %q referencing component name; name should appear in error messages only", k)
		}
	}
}

// TestTrackElapsed_NilMapFromFn covers the edge case where fn returns
// (nil, nil) — TrackElapsed must still populate the bookkeeping keys
// without panicking on the nil-map write.
func TestTrackElapsed_NilMapFromFn(t *testing.T) {
	got, err := TrackElapsed("X", func() (map[string]any, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got["_created_time"]; !ok {
		t.Error("missing _created_time after nil-map input")
	}
	if _, ok := got["_elapsed_time"]; !ok {
		t.Error("missing _elapsed_time after nil-map input")
	}
}
