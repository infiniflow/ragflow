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

// loop_test.go — pure logic and state-machine tests for the loop
// extension. These tests build minimal outer/sub workflows and
// assert the documented behavior of the loop state machine
// without exercising full eino checkpoint persistence. Integration
// scenarios (real checkpoint store, interrupt/resume) live in
// loop_integration_test.go.
package workflowx

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/compose"
)

// inMemoryStore is a minimal CheckPointStore used by these tests.
// It is duplicated from eino's own test helpers so this extension
// has no test-time dependency on eino's internal symbols.
type inMemoryStore struct {
	m map[string][]byte
}

func (s *inMemoryStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	v, ok := s.m[id]
	return v, ok, nil
}

func (s *inMemoryStore) Set(_ context.Context, id string, payload []byte) error {
	s.m[id] = payload
	return nil
}

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{m: make(map[string][]byte)}
}

// loopCounter is a test-only counter used to assert per-iteration
// call counts.
var loopCounter atomic.Int64

// counterLambda returns a lambda that increments loopCounter and
// returns the new value.
func counterLambda() *compose.Lambda {
	return compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		loopCounter.Add(1)
		return in + 1, nil
	})
}

// buildSubIncrement is a tiny sub-workflow that takes an int and
// returns int+1. It is the canonical "increments each iteration"
// body used by the iteration-numbering tests.
func buildSubIncrement(t *testing.T) *compose.Workflow[int, int] {
	t.Helper()
	wf := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, in int) (int, error) {
		return in + 1, nil
	})
	node := wf.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	wf.End().AddInput("inc")
	return wf
}

// TestLoop_IterationNumbering asserts that shouldQuit sees iteration
// values 1, 2, 3, ... in order. The sub-workflow is a plain
// increment, and shouldQuit returns true once the value reaches 4,
// so the loop runs 3 times.
func TestLoop_IterationNumbering(t *testing.T) {
	var iterations []int
	shouldQuit := func(_ context.Context, iter, prev, next int) (bool, error) {
		iterations = append(iterations, iter)
		return next >= 4, nil
	}

	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop",
		buildSubIncrement(t), shouldQuit,
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
	out, err := compiled.Invoke(context.Background(), 1)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out != 4 {
		t.Errorf("output: got %d, want 4", out)
	}
	want := []int{1, 2, 3}
	if len(iterations) != len(want) {
		t.Fatalf("iterations: got %v, want %v", iterations, want)
	}
	for i := range want {
		if iterations[i] != want[i] {
			t.Errorf("iterations[%d]: got %d, want %d", i, iterations[i], want[i])
		}
	}
}

// TestLoop_DoWhileContract asserts that the sub-workflow runs at
// least once. WithLoopMaxIterations(1) yields a single-iteration
// do-while: shouldQuit is called once with iter=1 and the loop
// exits.
func TestLoop_DoWhileContract(t *testing.T) {
	var seen int
	shouldQuit := func(_ context.Context, iter, prev, next int) (bool, error) {
		seen = iter
		return true, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop",
		buildSubIncrement(t), shouldQuit,
		WithLoopMaxIterations(1),
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
	out, err := compiled.Invoke(context.Background(), 7)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out != 8 {
		t.Errorf("output: got %d, want 8", out)
	}
	if seen != 1 {
		t.Errorf("shouldQuit saw iter %d, want 1", seen)
	}
}

// TestLoop_MaxIterationsExceeded asserts that exceeding the
// configured cap returns ErrLoopMaxIterationsExceeded.
func TestLoop_MaxIterationsExceeded(t *testing.T) {
	shouldQuit := func(_ context.Context, _ int, _, _ int) (bool, error) {
		return false, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop",
		buildSubIncrement(t), shouldQuit,
		WithLoopMaxIterations(3),
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
	_, err = compiled.Invoke(context.Background(), 0)
	if !errors.Is(err, ErrLoopMaxIterationsExceeded) {
		t.Fatalf("invoke: got %v, want ErrLoopMaxIterationsExceeded", err)
	}
}

// TestLoop_QuitConditionError asserts that a non-nil error from
// shouldQuit is wrapped in ErrLoopQuitConditionFailed.
func TestLoop_QuitConditionError(t *testing.T) {
	shouldQuit := func(_ context.Context, _ int, _, _ int) (bool, error) {
		return false, errors.New("boom")
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop",
		buildSubIncrement(t), shouldQuit,
		WithLoopMaxIterations(5),
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
	_, err = compiled.Invoke(context.Background(), 0)
	if !errors.Is(err, ErrLoopQuitConditionFailed) {
		t.Fatalf("invoke: got %v, want ErrLoopQuitConditionFailed", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q must wrap 'boom'", err)
	}
}

// TestLoop_NormalConvergence asserts the basic happy path.
func TestLoop_NormalConvergence(t *testing.T) {
	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop",
		buildSubIncrement(t), shouldQuit,
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
	out, err := compiled.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if out != 3 {
		t.Errorf("output: got %d, want 3", out)
	}
}

// TestLoop_SubErrorStopsLoop asserts that a non-interrupt error
// from the sub-workflow surfaces immediately (no
// ErrLoopSubGraphInterrupted wrap) and that shouldQuit is not
// called.
func TestLoop_SubErrorStopsLoop(t *testing.T) {
	sub := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(_ context.Context, _ int) (int, error) {
		return 0, errors.New("sub-fail")
	})
	node := sub.AddLambdaNode("err", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("err")
	shouldQuit := func(_ context.Context, _ int, _, _ int) (bool, error) {
		t.Fatal("shouldQuit must not be called when sub errors")
		return false, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = compiled.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if errors.Is(err, ErrLoopSubGraphInterrupted) {
		t.Errorf("non-interrupt sub error must NOT be wrapped as ErrLoopSubGraphInterrupted: %v", err)
	}
	if !strings.Contains(err.Error(), "sub-fail") {
		t.Errorf("error %q must propagate 'sub-fail'", err)
	}
}

// TestLoop_CounterIncrementedPerIteration asserts that the counter
// helper is called once per iteration. The sub-workflow invokes
// counterLambda() which bumps the global loopCounter. Three
// iterations → counter == 3.
func TestLoop_CounterIncrementedPerIteration(t *testing.T) {
	loopCounter.Store(0)

	sub := compose.NewWorkflow[int, int]()
	node := sub.AddLambdaNode("inc", counterLambda())
	node.AddInput(compose.START)
	sub.End().AddInput("inc")

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
	if got := loopCounter.Load(); got != 3 {
		t.Errorf("counter: got %d, want 3", got)
	}
}
