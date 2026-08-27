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

// node_body_timeout_test.go — pins the cancellation-only execution context
// contract of realComponentBody. The framework imposes no wall-clock deadline
// on component execution (see realComponentBodyWithOptions), so a component
// only stops when its parent context is cancelled; there is no timeout wrap to
// test any more. These tests assert cancellation propagation and the
// pass-through of fast components.

package canvas

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/agent/runtime"

	"gorm.io/gorm"
)

// blockingComponent is a runtime.Component whose Invoke blocks until ctx is
// cancelled.
type blockingComponent struct{}

func (b *blockingComponent) Name() string { return "blocking" }

func (b *blockingComponent) Invoke(ctx context.Context, _ *gorm.DB, _ map[string]any) (map[string]any, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (b *blockingComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func (b *blockingComponent) Inputs() map[string]string  { return nil }
func (b *blockingComponent) Outputs() map[string]string { return nil }

// TestRealComponentBody_RespectsParentCancellation verifies that when the
// parent context is already cancelled, the body surfaces a wrapped
// context.Canceled error. With no framework deadline, parent cancellation is
// the only mechanism that stops a long-running component.
func TestRealComponentBody_RespectsParentCancellation(t *testing.T) {
	comp := &blockingComponent{}
	body := realComponentBody("test-cpn", "TestBlocking", comp)

	parentCtx, cancel := context.WithCancel(t.Context())
	cancel() // pre-cancel

	_, err := body(parentCtx, map[string]any{"x": 1})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled wrapped error, got: %v", err)
	}
}

// TestRealComponentBody_NoTimeoutWhenFast verifies that a component returning
// immediately does not incur any timeout-induced latency or error wrapping.
func TestRealComponentBody_NoTimeoutWhenFast(t *testing.T) {
	comp := &echoComponent{}
	body := realComponentBody("test-cpn", "TestEcho", comp)

	out, err := body(t.Context(), map[string]any{"x": 1})
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

// echoComponent is a minimal runtime.Component used by the no-timeout test. It
// returns the input map unchanged plus a __cpn_id__ tag.
type echoComponent struct{}

func (e *echoComponent) Name() string { return "echo" }

func (e *echoComponent) Invoke(_ context.Context, _ *gorm.DB, in map[string]any) (map[string]any, error) {
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
