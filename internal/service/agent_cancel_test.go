// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");

package service

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestCancelTaskLocalPermissionAndIdempotency(t *testing.T) {
	svc := NewAgentServiceWithOptions(nil, nil, nil)
	active := &activeAgentRun{taskID: "task-1", userID: "user-a"}
	var calls atomic.Int32
	active.cancel = func() {
		active.cancelRequested.Store(true)
		calls.Add(1)
	}
	svc.activeRuns[active.taskID] = active

	if err := svc.CancelTask(t.Context(), "user-b", "task-1"); !errors.Is(err, ErrAgentNotOwner) {
		t.Fatalf("other user cancel error = %v, want ErrAgentNotOwner", err)
	}
	if calls.Load() != 0 {
		t.Fatal("unauthorized cancel invoked the active cancel func")
	}
	if err := svc.CancelTask(t.Context(), "user-a", "task-1"); err != nil {
		t.Fatalf("owner CancelTask: %v", err)
	}
	if calls.Load() != 1 || !active.cancelRequested.Load() {
		t.Fatalf("local cancel calls=%d requested=%v", calls.Load(), active.cancelRequested.Load())
	}

	delete(svc.activeRuns, "task-1")
	if err := svc.CancelTask(t.Context(), "user-a", "task-1"); err != nil {
		t.Fatalf("finished task cancel must be idempotent: %v", err)
	}
	if err := svc.CancelTask(t.Context(), "user-a", "unknown"); err != nil {
		t.Fatalf("unknown task cancel must be idempotent: %v", err)
	}
}
