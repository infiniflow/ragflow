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

package canvas

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
)

// blockingComponent is a runtime.Component whose Invoke blocks until ctx
// is cancelled. Used to test the per-component timeout wrapper in
// realComponentBody.
type blockingComponent struct{}

func (b *blockingComponent) Name() string { return "blocking" }

func (b *blockingComponent) Invoke(ctx context.Context, _ map[string]any) (map[string]any, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (b *blockingComponent) Inputs() map[string]string  { return nil }
func (b *blockingComponent) Outputs() map[string]string { return nil }

// TestRealComponentBody_RespectsTimeout verifies that a component whose
// Invoke blocks longer than the configured timeout causes the body to
// return a wrapped context.DeadlineExceeded error.
//
// The call itself runs under context.Background(); a separate watchdog
// goroutine fails the test if the inner timeout never fires. That keeps
// the assertion semantic (the returned error must wrap
// context.DeadlineExceeded) without letting an outer test context
// manufacture the same error type and create a false positive.
func TestRealComponentBody_RespectsTimeout(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "1")

	comp := &blockingComponent{}
	body := realComponentBody("test-cpn", "TestBlocking", comp)
	if body == nil {
		t.Fatalf("realComponentBody returned nil")
	}

	done := make(chan error, 1)
	go func() {
		_, err := body(context.Background(), map[string]any{"x": 1})
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("expected context.DeadlineExceeded wrapped error, got: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("body did not return within 15s — timeout wrap is broken or call is hanging")
	}
}

// TestRealComponentBody_RespectsParentCancellation verifies that when
// the parent context is already cancelled, the body surfaces a wrapped
// context.Canceled error rather than a timeout (or a generic wrap).
func TestRealComponentBody_RespectsParentCancellation(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "60")

	comp := &blockingComponent{}
	body := realComponentBody("test-cpn", "TestBlocking", comp)

	parentCtx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	_, err := body(parentCtx, map[string]any{"x": 1})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled wrapped error, got: %v", err)
	}
}

// TestRealComponentBody_NoTimeoutWhenFast verifies that a component
// returning immediately does not incur any timeout-induced latency or
// error wrapping.
func TestRealComponentBody_NoTimeoutWhenFast(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "60")

	// Stub component that returns immediately.
	comp := &echoComponent{}
	body := realComponentBody("test-cpn", "TestEcho", comp)

	out, err := body(context.Background(), map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out["__cpn_id__"] != "test-cpn" {
		t.Errorf("expected __cpn_id__=test-cpn, got %v", out["__cpn_id__"])
	}
	if out["x"] != 1 {
		t.Errorf("expected input to pass through, got x=%v", out["x"])
	}
}

// TestComponentTimeout_Default verifies the default is 600s when the env
// var is unset.
func TestComponentTimeout_Default(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "")
	if got := componentTimeout(); got != 600*time.Second {
		t.Errorf("default timeout: got %s, want 600s", got)
	}
}

// TestComponentTimeout_HonoursEnv verifies a valid env value is parsed.
func TestComponentTimeout_HonoursEnv(t *testing.T) {
	t.Setenv("COMPONENT_EXEC_TIMEOUT", "42")
	if got := componentTimeout(); got != 42*time.Second {
		t.Errorf("env timeout: got %s, want 42s", got)
	}
}

// TestComponentTimeout_InvalidEnvFallsBack verifies that non-numeric or
// non-positive env values fall back to the default — invalid input must
// never widen the timeout silently.
func TestComponentTimeout_InvalidEnvFallsBack(t *testing.T) {
	for _, v := range []string{"abc", "0", "-5"} {
		t.Setenv("COMPONENT_EXEC_TIMEOUT", v)
		if got := componentTimeout(); got != 600*time.Second {
			t.Errorf("invalid env %q: got %s, want default 600s", v, got)
		}
	}
}

// echoComponent is a minimal runtime.Component used by the no-timeout test.
// It returns the input map unchanged plus a __cpn_id__ tag (the body will
// overwrite the tag, but that's fine).
type echoComponent struct{}

func (e *echoComponent) Name() string { return "echo" }

func (e *echoComponent) Invoke(_ context.Context, in map[string]any) (map[string]any, error) {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out, nil
}

func (e *echoComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (e *echoComponent) Inputs() map[string]string  { return nil }
func (e *echoComponent) Outputs() map[string]string { return nil }

// Compile-time check that the stubs satisfy the interface.
var (
	_ runtime.Component = (*blockingComponent)(nil)
	_ runtime.Component = (*echoComponent)(nil)
)
