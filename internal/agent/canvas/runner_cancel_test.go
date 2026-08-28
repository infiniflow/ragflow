// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");

package canvas

import (
	"context"
	"encoding/json"
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

func TestRunnerEmitsCancelledEvent(t *testing.T) {
	r := NewRunner()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	events := r.Run(WithEventContext(ctx, context.Background()), blockingRun(started), "canvas", "session", nil, map[string]any{})
	<-started
	cancel()

	select {
	case ev, ok := <-events:
		if !ok {
			t.Fatal("run event channel closed without a cancellation event")
		}
		if ev.Type != "cancelled" {
			t.Fatalf("event type = %q, want cancelled", ev.Type)
		}
		var payload CancelledEvent
		if err := json.Unmarshal([]byte(ev.Data), &payload); err != nil {
			t.Fatalf("decode cancellation event: %v", err)
		}
		if payload.Message != "Agent run was cancelled." {
			t.Errorf("cancellation message = %q, want default message", payload.Message)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cancellation event")
	}

	if _, ok := <-events; ok {
		t.Fatal("run event channel should close after cancellation event")
	}
}

// TestPushEventSkipsCancelledConsumer verifies that an already-cancelled
// event context prevents delivery even when the destination buffer is writable.
func TestPushEventSkipsCancelledConsumer(t *testing.T) {
	eventCtx, cancelEvents := context.WithCancel(context.Background())
	workflowCtx := WithEventContext(t.Context(), eventCtx)
	events := make(chan RunEvent, 1)
	cancelEvents()

	PushEvent(workflowCtx, events, RunEvent{Type: "message"})
	if len(events) != 0 {
		t.Fatalf("event channel length = %d, want 0 after consumer cancellation", len(events))
	}
}

// TestRunnerDropsEventsAfterConsumerCancellation verifies that a blocked
// producer is released when the event consumer cancels its context.
func TestRunnerDropsEventsAfterConsumerCancellation(t *testing.T) {
	r := NewRunner()
	runCtx := t.Context()
	eventCtx, cancelEvents := context.WithCancel(context.Background())
	runCtx = WithEventContext(runCtx, eventCtx)
	started := make(chan struct{})
	run := func(ctx context.Context, root map[string]any) (*CanvasState, error) {
		close(started)
		events := root["__events__"].(chan RunEvent)
		for i := 0; i <= cap(events); i++ {
			PushEvent(ctx, events, RunEvent{Type: "message"})
		}
		return nil, nil
	}
	events := r.Run(runCtx, run, "canvas", "session", nil, map[string]any{})
	<-started
	cancelEvents()
	waitClosed(t, events)
}
