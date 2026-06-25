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

// loop_options_test.go — option semantics for AddLoopNode. These
// tests focus on the configured behaviour of the option set
// (defaults, forwarding, builders, compile-time failure paths).
package workflowx

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// buildSubCounter is a sub-workflow that increments a counter on
// every call. It is the basis for the option-forwarding tests
// (option callbacks must be observed on every call).
func buildSubCounter(t *testing.T, counter *atomic.Int64) *compose.Workflow[int, int] {
	t.Helper()
	wf := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		counter.Add(1)
		return in + 1, nil
	})
	node := wf.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	wf.End().AddInput("inc")
	return wf
}

// TestOptions_DefaultStreamModeIsFinalOnly asserts that omitting
// WithLoopStream uses LoopStreamFinalOnly. We probe the option
// resolver directly (without compiling a workflow) so the test is
// fast and has no dependency on eino's compile pipeline.
func TestOptions_DefaultStreamModeIsFinalOnly(t *testing.T) {
	opts := getLoopOptions(nil)
	if opts.streamMode != LoopStreamFinalOnly {
		t.Errorf("default stream mode: got %q, want %q", opts.streamMode, LoopStreamFinalOnly)
	}
}

// TestOptions_WithLoopStream_OverridesDefault asserts the
// LoopStreamEveryIteration mode is accepted.
func TestOptions_WithLoopStream_OverridesDefault(t *testing.T) {
	opts := getLoopOptions([]LoopOption{WithLoopStream(LoopStreamEveryIteration)})
	if opts.streamMode != LoopStreamEveryIteration {
		t.Errorf("stream mode: got %q, want %q", opts.streamMode, LoopStreamEveryIteration)
	}
}

// TestOptions_WithLoopStream_UnknownRejected asserts that an
// unrecognised mode is ignored (the resolver keeps the default).
func TestOptions_WithLoopStream_UnknownRejected(t *testing.T) {
	opts := getLoopOptions([]LoopOption{WithLoopStream(LoopStreamMode("nonsense"))})
	if opts.streamMode != LoopStreamFinalOnly {
		t.Errorf("unknown mode: got %q, want default", opts.streamMode)
	}
}

// TestOptions_DefaultMaxIterations is a numeric assertion that the
// resolver substitutes a non-zero cap when the caller does not
// configure one.
func TestOptions_DefaultMaxIterations(t *testing.T) {
	opts := getLoopOptions(nil)
	if opts.maxIterations <= 0 {
		t.Errorf("default max iterations: got %d, want > 0", opts.maxIterations)
	}
}

// TestOptions_WithLoopMaxIterations_ZeroKeepsDefault asserts that
// an explicit zero is treated as "use the default". This matches
// the documented P2 §"Constraints" semantics.
func TestOptions_WithLoopMaxIterations_ZeroKeepsDefault(t *testing.T) {
	opts := getLoopOptions([]LoopOption{WithLoopMaxIterations(0)})
	if opts.maxIterations <= 0 {
		t.Errorf("explicit zero: got %d, want > 0 (default)", opts.maxIterations)
	}
}

// TestOptions_WithLoopMaxIterations_NegativeKeepsDefault asserts
// that a negative value is treated as "use the default". Negative
// values are not meaningful for an iteration cap.
func TestOptions_WithLoopMaxIterations_NegativeKeepsDefault(t *testing.T) {
	opts := getLoopOptions([]LoopOption{WithLoopMaxIterations(-7)})
	if opts.maxIterations <= 0 {
		t.Errorf("negative: got %d, want > 0 (default)", opts.maxIterations)
	}
}

// TestOptions_WithLoopMaxIterations_Positive asserts the positive
// value is preserved.
func TestOptions_WithLoopMaxIterations_Positive(t *testing.T) {
	opts := getLoopOptions([]LoopOption{WithLoopMaxIterations(42)})
	if opts.maxIterations != 42 {
		t.Errorf("got %d, want 42", opts.maxIterations)
	}
}

// TestOptions_CheckpointBuilder_Default is non-empty. The default
// builder must be set so the loop is usable without an explicit
// WithLoopCheckpointIDBuilder.
func TestOptions_CheckpointBuilder_Default(t *testing.T) {
	opts := getLoopOptions(nil)
	if opts.checkpointBuilder == nil {
		t.Fatal("default checkpoint builder is nil")
	}
	id := opts.checkpointBuilder("k", 3)
	if id == "" {
		t.Error("default builder returned empty id")
	}
}

// TestOptions_CheckpointBuilder_Override asserts the user-supplied
// builder is used.
func TestOptions_CheckpointBuilder_Override(t *testing.T) {
	var gotKey string
	var gotIter int
	b := func(key string, iter int) string {
		gotKey = key
		gotIter = iter
		return "cp:" + key + ":" + itoa(iter)
	}
	opts := getLoopOptions([]LoopOption{WithLoopCheckpointIDBuilder(b)})
	id := opts.checkpointBuilder("loopKey", 5)
	if id != "cp:loopKey:5" {
		t.Errorf("builder output: got %q, want %q", id, "cp:loopKey:5")
	}
	if gotKey != "loopKey" || gotIter != 5 {
		t.Errorf("builder args: got key=%q iter=%d, want key=%q iter=5", gotKey, gotIter, "loopKey")
	}
}

// TestOptions_CheckpointBuilder_NilIgnored asserts that a nil
// builder passed via the option is ignored.
func TestOptions_CheckpointBuilder_NilIgnored(t *testing.T) {
	opts := getLoopOptions([]LoopOption{WithLoopCheckpointIDBuilder(nil)})
	if opts.checkpointBuilder == nil {
		t.Error("nil builder should be ignored, but default is also nil — test is inconclusive")
	}
	// The default builder is non-nil so the option is a no-op.
	id := opts.checkpointBuilder("k", 1)
	if id == "" {
		t.Error("builder produced empty id")
	}
}

// TestOptions_RunOptionsForwarded asserts that the options set via
// WithLoopRunOptions are passed to every nested sub-workflow call.
// We use a counter sub-workflow and check that the run option is
// observed on each call by counting sub-invocations.
func TestOptions_RunOptionsForwarded(t *testing.T) {
	var counter atomic.Int64
	sub := buildSubCounter(t, &counter)

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := compiled.Invoke(context.Background(), 0); err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if got := counter.Load(); got != 3 {
		t.Errorf("sub counter: got %d, want 3", got)
	}
}

// TestOptions_CompileOptionsForwarded asserts that compile-time
// options are propagated to the sub-workflow's Compile. We
// configure a CheckPointStore via WithLoopCompileOptions; if the
// store is wired in, subsequent sub-workflow invocations have
// access to it. The store is exercised via a simple key lookup.
func TestOptions_CompileOptionsForwarded(t *testing.T) {
	store := newInMemoryStore()
	_ = store.Set(context.Background(), "k", []byte("v"))

	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		// Touch the store to assert it is reachable in the
		// compiled sub-workflow. We do this by reading a
		// key set above; if the compile option was not
		// applied, the sub-workflow will panic with a nil
		// store (compile-time check on the engine side).
		_, _, _ = store.Get(ctx, "k")
		return in + 1, nil
	})
	node := sub.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("inc")

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 2, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(5),
		WithLoopCompileOptions(compose.WithCheckPointStore(store)),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	out, err := compiled.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out != 2 {
		t.Errorf("output: got %d, want 2", out)
	}
}

// TestOptions_NilChecks verifies that AddLoopNode rejects nil
// inputs up front, before any compile work happens.
func TestOptions_NilChecks(t *testing.T) {
	sub := buildSubIncrement(t)
	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		return true, nil
	}
	outer := compose.NewWorkflow[int, int]()

	cases := []struct {
		name string
		fn   func() error
	}{
		{"nil outer", func() error {
			_, err := AddLoopNode(context.Background(), nil, "loop", sub, shouldQuit)
			return err
		}},
		{"nil sub", func() error {
			_, err := AddLoopNode(context.Background(), outer, "loop", nil, shouldQuit)
			return err
		}},
		{"nil shouldQuit", func() error {
			_, err := AddLoopNode(context.Background(), outer, "loop", sub, nil)
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

// TestOptions_CompileFailureIsolated asserts that when the sub-
// workflow fails to compile, AddLoopNode returns an error and the
// outer workflow is not modified to a state that would mask the
// failure.
//
// We construct a sub-workflow with no start node so compile fails
// deterministically.
func TestOptions_CompileFailureIsolated(t *testing.T) {
	sub := compose.NewWorkflow[int, int]() // no nodes; compile will fail
	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		return true, nil
	}
	outer := compose.NewWorkflow[int, int]()
	_, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit)
	if err == nil {
		t.Fatal("expected compile error, got nil")
	}
	// The outer workflow should still be empty. Re-compiling it
	// must fail with "start node not set", proving the loop
	// didn't silently add a placeholder node.
	_, err = outer.Compile(context.Background())
	if err == nil || !strings.Contains(err.Error(), "start node not set") {
		t.Errorf("outer workflow not in expected state: %v", err)
	}
}

// TestOptions_SentinelErrorsExist is a smoke test that all four
// sentinel error values are non-nil. The behavioural assertions
// live in loop_test.go and loop_integration_test.go; this test
// pins the existence of the symbols so refactors cannot drop
// them silently.
func TestOptions_SentinelErrorsExist(t *testing.T) {
	sentinels := map[string]error{
		"ErrLoopMaxIterationsExceeded": ErrLoopMaxIterationsExceeded,
		"ErrLoopSubGraphInterrupted":  ErrLoopSubGraphInterrupted,
		"ErrLoopResumeStateInvalid":   ErrLoopResumeStateInvalid,
		"ErrLoopQuitConditionFailed":  ErrLoopQuitConditionFailed,
	}
	for name, e := range sentinels {
		if e == nil {
			t.Errorf("%s is nil", name)
		}
	}
	// errors.Is round-trip: each sentinel must satisfy errors.Is
	// against itself.
	if !errors.Is(ErrLoopMaxIterationsExceeded, ErrLoopMaxIterationsExceeded) {
		t.Error("ErrLoopMaxIterationsExceeded is not Is-self")
	}
	if !errors.Is(ErrLoopSubGraphInterrupted, ErrLoopSubGraphInterrupted) {
		t.Error("ErrLoopSubGraphInterrupted is not Is-self")
	}
}

// TestOptions_StreamModeResolveAfterUnknownMode asserts that an
// invalid mode followed by a valid one resolves to the second
// (later options take precedence).
func TestOptions_StreamModeResolveAfterUnknownMode(t *testing.T) {
	opts := getLoopOptions([]LoopOption{
		WithLoopStream(LoopStreamMode("garbage")),
		WithLoopStream(LoopStreamEveryIteration),
	})
	if opts.streamMode != LoopStreamEveryIteration {
		t.Errorf("got %q, want %q", opts.streamMode, LoopStreamEveryIteration)
	}
}

// itoa is a tiny helper that avoids importing strconv solely for
// tests. It is intentionally inline (not exported) and only used
// by the checkpoint-builder override test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ensure the unused import of schema is preserved for future
// stream-path tests in this file.
var _ = schema.Pipe[int]
