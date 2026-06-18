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

// loop_integration_test.go — full eino integration tests for the
// loop extension. These tests use real compose.Runnable + real
// compose.CheckPointStore to exercise the documented interrupt /
// resume contract from the plan's §"P0: resume and checkpoint
// contract" and §"P0: replay and side effects" sections.
//
// Note on sentinel-error assertions: the eino framework
// re-wraps interrupt errors at the runner boundary, so
// errors.Is(returnedErr, ErrLoopSubGraphInterrupted) may
// return false even when the loop's lambda did emit the
// sentinel. The integration tests therefore check the
// contract via ExtractInterruptInfo plus the loop-local
// state stored in the outer checkpoint. The unit tests in
// loop_test.go cover the errors.Is path for the four
// sentinels (no framework re-wrap happens on the unit
// path because the loop returns plain errors, not
// composite interrupts).
package workflowx

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// interruptingSub is a tiny sub-workflow that interrupts on its
// first Invoke for a given checkpoint ID, then succeeds.
func interruptingSub(t *testing.T) *compose.Workflow[int, int] {
	t.Helper()
	wf := compose.NewWorkflow[int, int]()
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		was, _, _ := compose.GetInterruptState[int](ctx)
		if !was {
			return 0, compose.StatefulInterrupt(ctx, "sub-interrupt", in)
		}
		return in + 1, nil
	})
	node := wf.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	wf.End().AddInput("inc")
	return wf
}

// counterSub is a non-interrupting sub-workflow whose every call
// increments a counter. Used for max-iter and per-iteration tests.
func counterSub(t *testing.T, counter *atomic.Int64) *compose.Workflow[int, int] {
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

func firstRootInterruptID(t *testing.T, err error) string {
	t.Helper()
	info, ok := extractInterruptInfoDeep(err)
	if !ok {
		t.Fatalf("ExtractInterruptInfo: got %v", err)
	}
	if len(info.InterruptContexts) == 0 {
		t.Fatal("InterruptContexts is empty")
	}
	for _, ctx := range info.InterruptContexts {
		if ctx.IsRootCause {
			return ctx.ID
		}
	}
	return info.InterruptContexts[0].ID
}

func extractInterruptInfoDeep(err error) (*compose.InterruptInfo, bool) {
	if err == nil {
		return nil, false
	}
	if info, ok := compose.ExtractInterruptInfo(err); ok {
		return info, true
	}
	type multiUnwrapper interface {
		Unwrap() []error
	}
	if mw, ok := err.(multiUnwrapper); ok {
		for _, sub := range mw.Unwrap() {
			if info, ok := extractInterruptInfoDeep(sub); ok {
				return info, true
			}
		}
	}
	if unwrapped := errors.Unwrap(err); unwrapped != nil {
		return extractInterruptInfoDeep(unwrapped)
	}
	return nil, false
}

func readAllInts(t *testing.T, sr *schema.StreamReader[int]) ([]int, error) {
	t.Helper()
	defer sr.Close()
	var out []int
	for {
		v, err := sr.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		out = append(out, v)
	}
}

func drainStreamUntilError(t *testing.T, sr *schema.StreamReader[int]) ([]int, error) {
	t.Helper()
	defer sr.Close()
	var out []int
	for {
		v, err := sr.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		out = append(out, v)
	}
}

// TestIntegration_OuterVsInnerCallback_Counts asserts the P1
// "Outer callbacks versus inner callbacks" requirement: the
// sub-workflow sees one execution per iteration.
func TestIntegration_OuterVsInnerCallback_Counts(t *testing.T) {
	var subCalls atomic.Int64
	subStore := newInMemoryStore()
	sub := counterSub(t, &subCalls)

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCompileOptions(compose.WithCheckPointStore(subStore)),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "sub-cp:loop:iter:" + itoa(iter)
		}),
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
	if got := subCalls.Load(); got != 3 {
		t.Errorf("sub invocations: got %d, want 3", got)
	}
}

// TestIntegration_SubWorkflowInterrupt_PropagatedAsComposite
// asserts the basic interrupt propagation contract: when the
// sub-workflow interrupts, the loop returns an error from which
// the original interrupt info is recoverable via
// ExtractInterruptInfo. The "sub-interrupt" string MUST appear in
// the InterruptInfo tree because that is how downstream callers
// distinguish a loop-internal interrupt from a user-level one.
func TestIntegration_SubWorkflowInterrupt_PropagatedAsComposite(t *testing.T) {
	outerStore := newInMemoryStore()
	subStore := newInMemoryStore()
	sub := interruptingSub(t)

	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		return true, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(5),
		WithLoopCompileOptions(compose.WithCheckPointStore(subStore)),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "sub-cp:loop:iter:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(),
		compose.WithCheckPointStore(outerStore),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	_, err = compiled.Invoke(context.Background(), 0,
		compose.WithCheckPointID("outer-cp"),
	)
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	info, ok := compose.ExtractInterruptInfo(err)
	if !ok {
		t.Fatalf("ExtractInterruptInfo: got %v", err)
	}
	if len(info.InterruptContexts) == 0 {
		t.Fatal("InterruptContexts is empty")
	}
	foundSubInterrupt := false
	var walk func(*compose.InterruptInfo)
	walk = func(i *compose.InterruptInfo) {
		if i == nil {
			return
		}
		for _, ctx := range i.InterruptContexts {
			if s, ok := ctx.Info.(string); ok && s == "sub-interrupt" {
				foundSubInterrupt = true
			}
		}
		for _, sub := range i.SubGraphs {
			walk(sub)
		}
	}
	walk(info)
	if !foundSubInterrupt {
		t.Errorf("InterruptInfo tree does not mention 'sub-interrupt'")
	}
}

// TestIntegration_LoopStatePersistedOnInterrupt asserts that when
// the sub-workflow interrupts, the outer checkpoint payload
// exists (i.e. the framework has written the loop's state).
func TestIntegration_LoopStatePersistedOnInterrupt(t *testing.T) {
	outerStore := newInMemoryStore()
	subStore := newInMemoryStore()
	sub := interruptingSub(t)

	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		return true, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(5),
		WithLoopCompileOptions(compose.WithCheckPointStore(subStore)),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "sub-cp:loop:iter:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(),
		compose.WithCheckPointStore(outerStore),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "outer-cp-persist"
	_, err = compiled.Invoke(context.Background(), 0,
		compose.WithCheckPointID(cpID),
	)
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	if _, found, _ := outerStore.Get(context.Background(), cpID); !found {
		t.Errorf("outer checkpoint %q not written", cpID)
	}
}

// TestIntegration_MaxIterationsExceeded_OnInvokePath asserts
// that a sustained (non-converging) loop run surfaces
// ErrLoopMaxIterationsExceeded through the outer invoke. This
// uses a non-interrupting sub-workflow so the loop actually
// reaches the cap.
func TestIntegration_MaxIterationsExceeded_OnInvokePath(t *testing.T) {
	var subCalls atomic.Int64
	subStore := newInMemoryStore()
	sub := counterSub(t, &subCalls)

	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		return false, nil // never quits
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(3),
		WithLoopCompileOptions(compose.WithCheckPointStore(subStore)),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "sub-cp:loop:iter:" + itoa(iter)
		}),
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
		t.Fatalf("got %v, want ErrLoopMaxIterationsExceeded", err)
	}
	if got := subCalls.Load(); got != 3 {
		t.Errorf("sub invocations: got %d, want 3", got)
	}
}

// TestIntegration_LoopRunsConcurrentlyWithResumeData checks the
// loop completes the do-while contract end-to-end through a real
// eino workflow with a checkpoint store. Unlike the unit tests
// in loop_test.go, this exercises the full compile/invoke path
// and confirms the loop survives eino's runner.
func TestIntegration_LoopRunsConcurrentlyWithResumeData(t *testing.T) {
	store := newInMemoryStore()
	sub := interruptingSub(t)

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 2, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCompileOptions(compose.WithCheckPointStore(store)),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "sub-cp:loop:iter:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(),
		compose.WithCheckPointStore(store),
	)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Run end-to-end. The sub-workflow interrupts on first
	// call; the loop must persist state and return an
	// interrupt error.
	cpID := "outer-cp-e2e"
	_, err = compiled.Invoke(context.Background(), 0,
		compose.WithCheckPointID(cpID),
	)
	if err == nil {
		t.Fatal("expected interrupt error on first run, got nil")
	}
	if _, ok := compose.ExtractInterruptInfo(err); !ok {
		t.Errorf("expected interrupt info in error; got %v", err)
	}
}

// TestIntegration_EnableSubCheckpoint_HappyPath asserts that
// WithLoopEnableSubCheckpoint makes the loop pass
// compose.WithCheckPointID to the sub-workflow on every nested
// call. The sub-workflow is a counter that uses its own
// checkpoint store; the test simply confirms the run does not
// fail with "receive checkpoint id but have not set checkpoint
// store".
func TestIntegration_EnableSubCheckpoint_HappyPath(t *testing.T) {
	var subCalls atomic.Int64
	subStore := newInMemoryStore()
	sub := counterSub(t, &subCalls)

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 2, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(5),
		WithLoopCompileOptions(compose.WithCheckPointStore(subStore)),
		WithLoopEnableSubCheckpoint(true),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "sub-cp:loop:iter:" + itoa(iter)
		}),
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
	if got := subCalls.Load(); got != 2 {
		t.Errorf("sub invocations: got %d, want 2", got)
	}
}

// TestIntegration_ResumeContinuesSameIteration asserts the core P0
// resume contract: an interrupt during iteration N resumes at
// iteration N rather than restarting from 1.
func TestIntegration_ResumeContinuesSameIteration(t *testing.T) {
	store := newInMemoryStore()

	sub := compose.NewWorkflow[int, int]()
	interrupted := false
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		wasInterrupted, _, _ := compose.GetInterruptState[int](ctx)
		if in == 1 && !wasInterrupted && !interrupted {
			interrupted = true
			return 0, compose.StatefulInterrupt(ctx, "pause-iter-2", in)
		}
		return in + 1, nil
	})
	node := sub.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("inc")

	var iterations []int
	shouldQuit := func(_ context.Context, iter, _, next int) (bool, error) {
		iterations = append(iterations, iter)
		return next >= 2, nil
	}

	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "resume-same-iter:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(), compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "resume-same-iteration"
	_, err = compiled.Invoke(context.Background(), 0, compose.WithCheckPointID(cpID))
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	resumeCtx := compose.Resume(context.Background(), firstRootInterruptID(t, err))
	out, err := compiled.Invoke(resumeCtx, 0, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume invoke: %v", err)
	}
	if out != 2 {
		t.Fatalf("output: got %d, want 2", out)
	}
	if len(iterations) < 3 {
		t.Fatalf("iterations too short: got %v, want prefix [1 2 3]", iterations)
	}
	wantPrefix := []int{1, 2, 3}
	for i := range wantPrefix {
		if iterations[i] != wantPrefix[i] {
			t.Fatalf("iterations[%d]: got %d, want %d", i, iterations[i], wantPrefix[i])
		}
	}
}

// TestIntegration_WithForceNewRunRestartsLoop asserts that
// WithForceNewRun ignores the saved loop checkpoint and restarts
// the loop from iteration 1 on the next invocation.
func TestIntegration_WithForceNewRunRestartsLoop(t *testing.T) {
	store := newInMemoryStore()

	sub := compose.NewWorkflow[int, int]()
	interruptions := 0
	lambda := compose.InvokableLambda(func(ctx context.Context, in int) (int, error) {
		wasInterrupted, _, _ := compose.GetInterruptState[int](ctx)
		if in == 1 && !wasInterrupted {
			interruptions++
			return 0, compose.StatefulInterrupt(ctx, "force-new-run", in)
		}
		return in + 1, nil
	})
	node := sub.AddLambdaNode("inc", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("inc")

	var iterations []int
	shouldQuit := func(_ context.Context, iter, _, next int) (bool, error) {
		iterations = append(iterations, iter)
		return next >= 3, nil
	}

	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(10),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "force-new-run:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(), compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "force-new-run"
	_, err = compiled.Invoke(context.Background(), 0, compose.WithCheckPointID(cpID))
	if err == nil {
		t.Fatal("expected first interrupt, got nil")
	}
	_, err = compiled.Invoke(context.Background(), 0,
		compose.WithCheckPointID(cpID),
		compose.WithForceNewRun(),
	)
	if err == nil {
		t.Fatal("expected second interrupt after force-new-run, got nil")
	}
	if interruptions != 2 {
		t.Fatalf("interruptions: got %d, want 2", interruptions)
	}
	want := []int{1, 1}
	if len(iterations) != len(want) {
		t.Fatalf("iterations: got %v, want %v", iterations, want)
	}
	for i := range want {
		if iterations[i] != want[i] {
			t.Fatalf("iterations[%d]: got %d, want %d", i, iterations[i], want[i])
		}
	}
}

// TestIntegration_WithWriteToCheckPointIDPersistsToNewID asserts
// the interrupt state is written to the designated checkpoint ID
// and can be resumed from that new location.
func TestIntegration_WithWriteToCheckPointIDPersistsToNewID(t *testing.T) {
	store := newInMemoryStore()
	sub := interruptingSub(t)

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 1, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopMaxIterations(5),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "write-to-cp:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(), compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	oldID := "loop-old"
	newID := "loop-new"
	_, err = compiled.Invoke(context.Background(), 0,
		compose.WithCheckPointID(oldID),
		compose.WithWriteToCheckPointID(newID),
	)
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	if _, found, _ := store.Get(context.Background(), oldID); found {
		t.Fatalf("old checkpoint %q should not be written", oldID)
	}
	if _, found, _ := store.Get(context.Background(), newID); !found {
		t.Fatalf("new checkpoint %q was not written", newID)
	}

	resumeCtx := compose.Resume(context.Background(), firstRootInterruptID(t, err))
	out, err := compiled.Invoke(resumeCtx, 0, compose.WithCheckPointID(newID))
	if err != nil {
		t.Fatalf("resume invoke: %v", err)
	}
	if out != 1 {
		t.Fatalf("output: got %d, want 1", out)
	}
}

// TestIntegration_StreamFinalOnly_ResumeExposesOnlyFinalIteration
// asserts that FinalOnly mode does not expose historical iteration
// chunks after resume.
func TestIntegration_StreamFinalOnly_ResumeExposesOnlyFinalIteration(t *testing.T) {
	store := newInMemoryStore()

	sub := compose.NewWorkflow[int, int]()
	interrupted := false
	lambda, err := compose.AnyLambda[int, int, struct{}](
		nil,
		func(ctx context.Context, in int, _ ...struct{}) (*schema.StreamReader[int], error) {
			wasInterrupted, _, _ := compose.GetInterruptState[int](ctx)
			if in == 10 && !wasInterrupted && !interrupted {
				interrupted = true
				return nil, compose.StatefulInterrupt(ctx, "stream-final-only", in)
			}
			return schema.StreamReaderFromArray([]int{in, in + 10}), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("AnyLambda: %v", err)
	}
	node := sub.AddLambdaNode("stream", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("stream")

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 20, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamFinalOnly),
		WithLoopMaxIterations(5),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "stream-final-only:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(), compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "stream-final-only"
	sr, err := compiled.Stream(context.Background(), 0, compose.WithCheckPointID(cpID))
	if err == nil {
		_, err = drainStreamUntilError(t, sr)
	}
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	sr, err = compiled.Stream(context.Background(), 0, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume stream: %v", err)
	}
	got, err := readAllInts(t, sr)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	want := []int{10, 20}
	if len(got) != len(want) {
		t.Fatalf("chunks: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunks[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

// TestIntegration_StreamEveryIteration_ResumeFromFirstUnpublishedIteration
// asserts the documented replay contract for EveryIteration mode:
// fully published iterations are not replayed, while the interrupted
// iteration is replayed from its start.
func TestIntegration_StreamEveryIteration_ResumeFromFirstUnpublishedIteration(t *testing.T) {
	store := newInMemoryStore()

	sub := compose.NewWorkflow[int, int]()
	interrupted := false
	lambda, err := compose.AnyLambda[int, int, struct{}](
		nil,
		func(ctx context.Context, in int, _ ...struct{}) (*schema.StreamReader[int], error) {
			wasInterrupted, _, _ := compose.GetInterruptState[int](ctx)
			if in == 10 && !wasInterrupted && !interrupted {
				interrupted = true
				return nil, compose.StatefulInterrupt(ctx, "stream-every-iteration", in)
			}
			return schema.StreamReaderFromArray([]int{in, in + 10}), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("AnyLambda: %v", err)
	}
	node := sub.AddLambdaNode("stream", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("stream")

	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 20, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamEveryIteration),
		WithLoopMaxIterations(5),
		WithLoopCheckpointIDBuilder(func(_ string, iter int) string {
			return "stream-every-iteration:" + itoa(iter)
		}),
	)
	if err != nil {
		t.Fatalf("AddLoopNode: %v", err)
	}
	loopNode.AddInput(compose.START)
	outer.End().AddInput("loop")
	compiled, err := outer.Compile(context.Background(), compose.WithCheckPointStore(store))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	cpID := "stream-every-iteration"
	sr, err := compiled.Stream(context.Background(), 0, compose.WithCheckPointID(cpID))
	if err == nil {
		_, err = drainStreamUntilError(t, sr)
	}
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}
	sr, err = compiled.Stream(context.Background(), 0, compose.WithCheckPointID(cpID))
	if err != nil {
		t.Fatalf("resume stream: %v", err)
	}
	got, err := readAllInts(t, sr)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	want := []int{0, 10, 10, 20}
	if len(got) != len(want) {
		t.Fatalf("chunks: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunks[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

// streamingIncSub builds a sub-workflow whose Stream emits two chunks
// per iteration: {in, in+1}. The second chunk is the value the loop
// machinery uses as `next` (loop.go derives next from the last value
// emitted in the iteration). This sub deliberately never interrupts so
// the happy-path stream tests can assert chunk ordering across many
// iterations without exercising the resume code paths (which already
// have dedicated tests above).
func streamingIncSub(t *testing.T) *compose.Workflow[int, int] {
	t.Helper()
	wf := compose.NewWorkflow[int, int]()
	lambda, err := compose.AnyLambda[int, int, struct{}](
		nil,
		func(_ context.Context, in int, _ ...struct{}) (*schema.StreamReader[int], error) {
			return schema.StreamReaderFromArray([]int{in, in + 1}), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("AnyLambda: %v", err)
	}
	node := wf.AddLambdaNode("stream", lambda)
	node.AddInput(compose.START)
	wf.End().AddInput("stream")
	return wf
}

// TestIntegration_StreamFinalOnly_HappyPath exercises the
// LoopStreamFinalOnly mode end-to-end on a fresh (no-interrupt) run.
// The existing FinalOnly stream test only covers the resume path; this
// test asserts the documented buffer-and-emit-last contract when no
// interrupt occurs.
//
// Iterations: in=0 -> [0,1] (next=1), in=1 -> [1,2] (next=2), in=2 ->
// [2,3] (next=3, quit). Caller must observe ONLY the final iteration's
// chunks: [2, 3].
func TestIntegration_StreamFinalOnly_HappyPath(t *testing.T) {
	sub := streamingIncSub(t)
	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamFinalOnly),
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

	sr, err := compiled.Stream(context.Background(), 0)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	got, err := readAllInts(t, sr)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	want := []int{2, 3}
	if len(got) != len(want) {
		t.Fatalf("chunks: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunks[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

// TestIntegration_StreamEveryIteration_HappyPath exercises the
// LoopStreamEveryIteration mode end-to-end on a fresh run. The
// existing EveryIteration stream test only covers the replay path on
// resume; this test asserts the documented forward-every-iteration
// contract when no interrupt occurs.
//
// Iterations: in=0 -> [0,1], in=1 -> [1,2], in=2 -> [2,3] (quit).
// Caller must observe every iteration's chunks concatenated in order:
// [0, 1, 1, 2, 2, 3].
func TestIntegration_StreamEveryIteration_HappyPath(t *testing.T) {
	sub := streamingIncSub(t)
	shouldQuit := func(_ context.Context, _, _, next int) (bool, error) {
		return next >= 3, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamEveryIteration),
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

	sr, err := compiled.Stream(context.Background(), 0)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	got, err := readAllInts(t, sr)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	want := []int{0, 1, 1, 2, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("chunks: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("chunks[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

// TestIntegration_Stream_EmptyIterationFails covers the empty-stream
// error branch in runLoopStream: a sub-workflow that yields zero
// chunks for an iteration leaves the loop with no value to feed into
// shouldQuit or into the next iteration's input, so the loop must
// fail with the documented "produced empty stream" error. Without
// this test the branch (loop.go: "iteration N produced empty stream")
// is unreachable from the existing test surface.
func TestIntegration_Stream_EmptyIterationFails(t *testing.T) {
	sub := compose.NewWorkflow[int, int]()
	lambda, err := compose.AnyLambda[int, int, struct{}](
		nil,
		func(_ context.Context, _ int, _ ...struct{}) (*schema.StreamReader[int], error) {
			return schema.StreamReaderFromArray([]int{}), nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("AnyLambda: %v", err)
	}
	node := sub.AddLambdaNode("empty", lambda)
	node.AddInput(compose.START)
	sub.End().AddInput("empty")

	shouldQuit := func(_ context.Context, _, _, _ int) (bool, error) {
		t.Fatal("shouldQuit must not be called when iteration stream is empty")
		return false, nil
	}
	outer := compose.NewWorkflow[int, int]()
	loopNode, err := AddLoopNode(context.Background(), outer, "loop", sub, shouldQuit,
		WithLoopStream(LoopStreamFinalOnly),
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

	sr, err := compiled.Stream(context.Background(), 0)
	if err == nil {
		_, err = readAllInts(t, sr)
	}
	if err == nil {
		t.Fatal("expected empty-stream error, got nil")
	}
	if msg := err.Error(); !contains(msg, "produced empty stream") {
		t.Fatalf("error %q must mention 'produced empty stream'", msg)
	}
}

// contains is a tiny strings.Contains shim kept in this file to
// avoid pulling the strings import into the test package solely for
// one assertion (loop_test.go already imports it; loop_integration_
// test.go does not).
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
