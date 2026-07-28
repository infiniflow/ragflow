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

func TestRunnerUsesSessionMetadata(t *testing.T) {
	r := NewRunner()
	root := map[string]any{}
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	events := r.Run(ctx, blockingRun(started), "canvas-1", "session-1", nil, root)
	<-started
	if got := root["__session_id__"]; got != "session-1" {
		t.Fatalf("session metadata = %v, want session-1", got)
	}
	cancel()
	waitClosed(t, events)
}

func TestRunnerParentContextCancelsManagedRun(t *testing.T) {
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
