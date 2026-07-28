// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");

package service

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"ragflow/internal/entity"
)

func TestCancelSessionRunLocalPermissionAndIdempotency(t *testing.T) {
	svc := NewAgentServiceWithOptions(nil, nil, nil)
	active := &activeAgentRun{sessionID: "session-1", userID: "user-a"}
	var calls atomic.Int32
	active.cancelRun = func() {
		calls.Add(1)
	}
	svc.activeSessions[active.sessionID] = active

	if err := svc.CancelSessionRun(t.Context(), "user-b", "session-1"); !errors.Is(err, ErrAgentNotOwner) {
		t.Fatalf("other user cancel error = %v, want ErrAgentNotOwner", err)
	}
	if calls.Load() != 0 {
		t.Fatal("unauthorized cancel invoked the active cancel func")
	}
	if err := svc.CancelSessionRun(t.Context(), "user-a", "session-1"); err != nil {
		t.Fatalf("owner CancelSessionRun: %v", err)
	}
	if calls.Load() != 1 || !active.cancelRequested.Load() {
		t.Fatalf("local cancel calls=%d requested=%v", calls.Load(), active.cancelRequested.Load())
	}

	delete(svc.activeSessions, "session-1")
	if err := svc.CancelSessionRun(t.Context(), "user-a", "session-1"); err != nil {
		t.Fatalf("finished session cancel must be idempotent: %v", err)
	}
	if err := svc.CancelSessionRun(t.Context(), "user-a", "unknown"); err != nil {
		t.Fatalf("unknown session cancel must be idempotent: %v", err)
	}
}

func TestCancelSessionRunDoesNotAffectAnotherSession(t *testing.T) {
	svc := NewAgentServiceWithOptions(nil, nil, nil)
	var callsA, callsB atomic.Int32
	svc.activeSessions["session-a"] = &activeAgentRun{
		userID: "user-a", sessionID: "session-a", cancelRun: func() { callsA.Add(1) },
	}
	svc.activeSessions["session-b"] = &activeAgentRun{
		userID: "user-a", sessionID: "session-b", cancelRun: func() { callsB.Add(1) },
	}
	if err := svc.CancelSessionRun(t.Context(), "user-a", "session-a"); err != nil {
		t.Fatalf("CancelSessionRun: %v", err)
	}
	if callsA.Load() != 1 || callsB.Load() != 0 {
		t.Fatalf("cancel calls A=%d B=%d; want 1, 0", callsA.Load(), callsB.Load())
	}
}

func TestCancelSessionRunAuthorizesPersistedSessionBeforeTombstone(t *testing.T) {
	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)
	if err := testDB.Create(&entity.API4Conversation{
		ID:        "session-persisted",
		DialogID:  "agent-1",
		UserID:    "user-a",
		Message:   json.RawMessage(`[]`),
		Reference: json.RawMessage(`[]`),
	}).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	svc := NewAgentServiceWithOptions(nil, nil, nil)
	if err := svc.CancelSessionRun(t.Context(), "user-b", "session-persisted"); !errors.Is(err, ErrAgentNotOwner) {
		t.Fatalf("other user cancel error = %v, want ErrAgentNotOwner", err)
	}
	if err := svc.CancelSessionRun(t.Context(), "user-a", "session-persisted"); err != nil {
		t.Fatalf("owner cancel error = %v", err)
	}
	if err := svc.CancelSessionRun(t.Context(), "user-a", "missing-session"); err != nil {
		t.Fatalf("missing session cancel must be idempotent: %v", err)
	}
}
