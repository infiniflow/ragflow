// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");

package canvas

import (
	"context"
	"testing"
	"time"
)

func blockingRun(started chan<- struct{}) RunFunc {
	return func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
}

func waitClosed(t *testing.T, events <-chan RunEvent) {
	t.Helper()
	select {
	case _, ok := <-events:
		if ok {
			for range events {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run event channel did not close after cancellation")
	}
}

func TestRunnerTaskIDMatchesSessionIDAndNotVersionID(t *testing.T) {
	withCancelClient(t)
	r := NewRunner()

	rootA := map[string]any{"version_id": "version-1"}
	rootB := map[string]any{"version_id": "version-1"}
	startedA, startedB := make(chan struct{}), make(chan struct{})
	eventsA := r.Run(t.Context(), blockingRun(startedA), "canvas-1", "session-1", nil, rootA)
	eventsB := r.Run(t.Context(), blockingRun(startedB), "canvas-1", "session-2", nil, rootB)
	<-startedA
	<-startedB

	taskA := rootA["__task_id__"].(string)
	taskB := rootB["__task_id__"].(string)
	if taskA != "session-1" || taskB != "session-2" {
		t.Fatalf("task ids = %q, %q; want their session ids", taskA, taskB)
	}
	if taskA == "version-1" || taskB == "version-1" {
		t.Fatal("task_id must not reuse version_id")
	}
	r.Cancel("session-1")
	r.Cancel("session-2")
	waitClosed(t, eventsA)
	waitClosed(t, eventsB)
}

func TestRunnerCancelIsTaskScoped(t *testing.T) {
	withCancelClient(t)
	r := NewRunner()
	rootA := map[string]any{"__task_id__": "session-a"}
	rootB := map[string]any{"__task_id__": "session-b"}
	startedA, startedB := make(chan struct{}), make(chan struct{})
	eventsA := r.Run(t.Context(), blockingRun(startedA), "same-canvas", "session-a", nil, rootA)
	eventsB := r.Run(t.Context(), blockingRun(startedB), "same-canvas", "session-b", nil, rootB)
	<-startedA
	<-startedB

	r.Cancel("session-a")
	waitClosed(t, eventsA)
	select {
	case <-eventsB:
		t.Fatal("cancelling session-a also stopped session-b")
	case <-time.After(100 * time.Millisecond):
	}
	r.Cancel("session-b")
	waitClosed(t, eventsB)
}

func TestRunnerCancelsWhenAnotherInstancePublishesRedisSignal(t *testing.T) {
	withCancelClient(t)
	r := NewRunner()
	started := make(chan struct{})
	events := r.Run(t.Context(), blockingRun(started), "canvas", "session", nil, map[string]any{
		"__task_id__": "remote-task",
	})
	<-started
	if err := RequestCancel(t.Context(), "remote-task"); err != nil {
		t.Fatalf("RequestCancel: %v", err)
	}
	waitClosed(t, events)
}

func TestRunnerParentContextCancelsManagedRun(t *testing.T) {
	withCancelClient(t)
	r := NewRunner()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	returned := make(chan struct{})
	run := func(ctx context.Context, _ map[string]any) (*CanvasState, error) {
		close(started)
		<-ctx.Done()
		close(returned)
		return nil, ctx.Err()
	}
	events := r.Run(ctx, run, "canvas", "session", nil, map[string]any{})
	<-started
	cancel()
	waitClosed(t, events)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("RunFunc was left running after parent context cancellation")
	}
}
