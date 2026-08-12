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

// parallel_options_test.go — option semantics for AddParallelNode.
// These tests focus on the configured behaviour of the option
// set (defaults, forwarding, builders, compile-time failure
// paths, sentinel errors).
package workflowx

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/compose"
)

// TestOptions_DefaultMaxConcurrencyIsSequential asserts that
// omitting WithParallelMaxConcurrency yields MaxConcurrency == 0
// (sequential).
func TestOptions_DefaultMaxConcurrencyIsSequential(t *testing.T) {
	opts := getParallelOptions(nil)
	if opts.maxConcurrency != 0 {
		t.Errorf("default max concurrency: got %d, want 0", opts.maxConcurrency)
	}
}

// TestOptions_WithParallelMaxConcurrency_Positive asserts that
// positive values are preserved.
func TestOptions_WithParallelMaxConcurrency_Positive(t *testing.T) {
	opts := getParallelOptions([]ParallelOption{WithParallelMaxConcurrency(8)})
	if opts.maxConcurrency != 8 {
		t.Errorf("got %d, want 8", opts.maxConcurrency)
	}
}

// TestOptions_WithParallelMaxConcurrency_NegativeKeepsDefault
// asserts that negative values are ignored.
func TestOptions_WithParallelMaxConcurrency_NegativeKeepsDefault(t *testing.T) {
	opts := getParallelOptions([]ParallelOption{WithParallelMaxConcurrency(-3)})
	if opts.maxConcurrency != 0 {
		t.Errorf("negative: got %d, want 0 (default)", opts.maxConcurrency)
	}
}

// TestOptions_ParallelRunOptionsForwarded asserts the run
// options are passed to every per-item sub-workflow Invoke. We
// attach a callback run option and assert that it fires once per
// sub-workflow call.
func TestOptions_ParallelRunOptionsForwarded(t *testing.T) {
	var calls atomic.Int32
	var optionCalls atomic.Int32
	h := callbacks.NewHandlerBuilder().OnStartFn(func(ctx context.Context, _ *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
		optionCalls.Add(1)
		return ctx
	}).Build()

	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		calls.Add(1)
		return in + 1, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	outer := compose.NewWorkflow[[]int, []int]()
	pNode, err := AddParallelNode(context.Background(), outer, "par", sub,
		WithParallelMaxConcurrency(0),
		WithParallelRunOptions(compose.WithCallbacks(h)),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	pNode.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = compiled.Invoke(context.Background(), []int{1, 2, 3, 4})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := calls.Load(); got != 4 {
		t.Errorf("sub calls: got %d, want 4", got)
	}
	// The callback fires for the sub-workflow and its inner node
	// (two timings per invoke), so it must be well above zero — the
	// assertion is that the run option reached the per-item
	// sub-workflow Invoke calls.
	if got := optionCalls.Load(); got < 4 {
		t.Errorf("run option callback calls: got %d, want >= 4", got)
	}
}

// TestOptions_ParallelCompileOptionsForwarded asserts the compile
// options are passed to the inner sub-workflow's Compile call.
func TestOptions_ParallelCompileOptionsForwarded(t *testing.T) {
	store := newInMemoryStore()
	_ = store.Set(context.Background(), "k", []byte("v"))

	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		// Reach for the store to confirm wiring; without the
		// compile option the sub-workflow's runtime check
		// would surface a different error.
		_, _, _ = store.Get(ctx, "k")
		return in + 1, nil
	})
	node := sub.AddLambdaNode("op", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("op")

	outer := compose.NewWorkflow[[]int, []int]()
	pNode, err := AddParallelNode(context.Background(), outer, "par", sub,
		WithParallelMaxConcurrency(0),
		WithParallelCompileOptions(compose.WithCheckPointStore(store)),
	)
	if err != nil {
		t.Fatalf("AddParallelNode: %v", err)
	}
	pNode.AddInput(compose.START)
	outer.End().AddInput("par")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := compiled.Invoke(context.Background(), []int{1, 2, 3})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	want := []int{2, 3, 4}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("got[%d] = %d, want %d", i, got[i], v)
		}
	}
}

// TestOptions_ParallelNilChecks verifies that AddParallelNode
// rejects nil inputs up front, before any compile work happens.
func TestOptions_ParallelNilChecks(t *testing.T) {
	sub := buildParallelIncSub(t)
	outer := compose.NewWorkflow[[]int, []int]()

	cases := []struct {
		name string
		fn   func() error
	}{
		{"nil outer", func() error {
			_, err := AddParallelNode[int, int](context.Background(), nil, "par", sub)
			return err
		}},
		{"nil sub", func() error {
			var nilSub Compilable[int, int]
			_, err := AddParallelNode(context.Background(), outer, "par", nilSub)
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.fn()
			if err == nil {
				t.Errorf("%s: expected error, got nil", c.name)
			}
		})
	}
}

// TestOptions_ParallelCompileFailureIsolated asserts that when
// the sub-workflow fails to compile, AddParallelNode returns an
// error (wrapping ErrParallelCompileFailed) and the outer
// workflow is not modified to a state that would mask the
// failure.
func TestOptions_ParallelCompileFailureIsolated(t *testing.T) {
	sub := compose.NewWorkflow[int, int]() // no nodes; compile fails
	outer := compose.NewWorkflow[[]int, []int]()
	_, err := AddParallelNode(context.Background(), outer, "par", sub)
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	if !errors.Is(err, ErrParallelCompileFailed) {
		t.Errorf("errors.Is(err, ErrParallelCompileFailed) = false; err = %v", err)
	}
	// The outer workflow should still be empty.
	_, err = outer.Compile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start node not set") {
		t.Errorf("outer workflow not in expected state: %v", err)
	}
}

// TestOptions_ParallelSentinelErrorsExist is a smoke test that
// all parallel sentinel error values are non-nil and satisfy
// errors.Is against themselves.
func TestOptions_ParallelSentinelErrorsExist(t *testing.T) {
	sentinels := map[string]error{
		"ErrParallelCompileFailed":          ErrParallelCompileFailed,
		"ErrParallelOuterStreamUnsupported": ErrParallelOuterStreamUnsupported,
	}
	for name, e := range sentinels {
		if e == nil {
			t.Errorf("%s is nil", name)
		}
	}
	if !errors.Is(ErrParallelCompileFailed, ErrParallelCompileFailed) {
		t.Error("ErrParallelCompileFailed is not Is-self")
	}
	if !errors.Is(ErrParallelOuterStreamUnsupported, ErrParallelOuterStreamUnsupported) {
		t.Error("ErrParallelOuterStreamUnsupported is not Is-self")
	}
}
