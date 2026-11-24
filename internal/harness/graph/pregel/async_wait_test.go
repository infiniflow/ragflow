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

package pregel

import (
	"context"
	"errors"
	"testing"
	"time"
)

// waitForIdle polls until every registered task has been removed from
// activeTasks. The deferred removal runs after the result is delivered, so a
// reader needs this extra step instead of only draining the result channel.
func waitForIdle(t *testing.T, executor *AsyncExecutor) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for executor.GetActiveTaskCount() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("active tasks did not drain")
		}
		time.Sleep(time.Millisecond)
	}
}

// waitWithWatchdog runs Wait in a goroutine and bounds it with a generous
// watchdog. A watchdog (rather than a sub-poll deadline) keeps these tests
// stable under scheduler jitter while still detecting a Wait that never wakes.
func waitWithWatchdog(t *testing.T, executor *AsyncExecutor, ctx context.Context) error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- executor.Wait(ctx) }()
	select {
	case err := <-errCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("Wait did not return within the watchdog")
		return nil
	}
}

// TestAsyncExecutorWaitReturnsNilForAlreadyIdleExecutor checks that an
// executor whose last task has drained is reported as idle immediately.
func TestAsyncExecutorWaitReturnsNilForAlreadyIdleExecutor(t *testing.T) {
	executor := NewAsyncExecutor(2)
	resultCh := executor.Execute(t.Context(), "instant", func(context.Context) (any, error) {
		return "ok", nil
	})
	select {
	case <-resultCh:
	case <-time.After(time.Second):
		t.Fatal("task did not complete")
	}
	waitForIdle(t, executor)

	if err := waitWithWatchdog(t, executor, t.Context()); err != nil {
		t.Fatalf("Wait on idle executor = %v, want nil", err)
	}
}

// TestAsyncExecutorWaitReturnsNilWithNoTasks covers the zero-task corner case.
func TestAsyncExecutorWaitReturnsNilWithNoTasks(t *testing.T) {
	executor := NewAsyncExecutor(2)
	if err := waitWithWatchdog(t, executor, t.Context()); err != nil {
		t.Fatalf("Wait with no tasks = %v, want nil", err)
	}
}

// TestAsyncExecutorWaitWakesOnLastTaskCompletion starts Wait while a task is
// active and releases that task after the waiter goroutine has signalled it is
// starting. The watchdog verifies the wake; no sub-poll deadline is used.
func TestAsyncExecutorWaitWakesOnLastTaskCompletion(t *testing.T) {
	executor := NewAsyncExecutor(2)
	started := make(chan struct{})
	release := make(chan struct{})
	executor.Execute(t.Context(), "blocking", func(context.Context) (any, error) {
		close(started)
		<-release
		return nil, nil
	})
	<-started

	waitStarting := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		close(waitStarting)
		errCh <- executor.Wait(t.Context())
	}()
	<-waitStarting
	close(release)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Wait after last task completion = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after the last task finished")
	}
	waitForIdle(t, executor)
}

// TestAsyncExecutorWaitRespectsContextCancel keeps a task active and verifies
// that Wait still returns the context error immediately on cancellation.
func TestAsyncExecutorWaitRespectsContextCancel(t *testing.T) {
	executor := NewAsyncExecutor(2)
	started := make(chan struct{})
	release := make(chan struct{})
	executor.Execute(t.Context(), "long-running", func(context.Context) (any, error) {
		close(started)
		<-release
		return nil, nil
	})
	<-started

	ctx, cancel := context.WithCancel(t.Context())
	errCh := make(chan error, 1)
	go func() { errCh <- executor.Wait(ctx) }()
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait after cancel = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return after context cancellation")
	}
	close(release)
	waitForIdle(t, executor)
}

// TestAsyncExecutorCancelThenTaskFinishDoesNotDoubleClose covers a task that
// is removed by Cancel while still running: its deferred removal later runs
// against an already-empty set and must not close the idle channel twice.
func TestAsyncExecutorCancelThenTaskFinishDoesNotDoubleClose(t *testing.T) {
	executor := NewAsyncExecutor(2)
	started := make(chan struct{})
	release := make(chan struct{})
	resultCh := executor.Execute(t.Context(), "cancelled", func(ctx context.Context) (any, error) {
		close(started)
		<-release
		return nil, ctx.Err()
	})
	<-started

	executor.Cancel()
	close(release)
	select {
	case <-resultCh:
	case <-time.After(time.Second):
		t.Fatal("cancelled task did not deliver its result")
	}
	waitForIdle(t, executor)
	if err := executor.Wait(t.Context()); err != nil {
		t.Fatalf("Wait after cancel = %v, want nil", err)
	}
}

// TestAsyncExecutorWaitCanceledContextWithNoTasks preserves the pre-event
// behavior: an already-cancelled context must surface its error even when the
// executor is idle.
func TestAsyncExecutorWaitCanceledContextWithNoTasks(t *testing.T) {
	executor := NewAsyncExecutor(2)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := executor.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait with cancelled context = %v, want context.Canceled", err)
	}
}
