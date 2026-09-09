// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");

package service

import (
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/entity"
)

func newAgentCancelTracker(t *testing.T) (*canvas.RunTracker, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return canvas.NewRunTrackerWithClient(client, time.Hour), mr
}

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

func TestCancelSessionRunFinishedSessionDoesNotCreateCancelMarker(t *testing.T) {
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

	tracker, mr := newAgentCancelTracker(t)

	ctx := t.Context()
	const (
		sessionID = "session-persisted"
		runID     = "agent-1-session-persisted"
		token     = "run-owner"
	)
	registered, err := tracker.RegisterActiveSession(ctx, canvas.ActiveSession{
		SessionID: sessionID,
		Token:     token,
		UserID:    "user-a",
		CanvasID:  "agent-1",
		RunID:     runID,
	})
	if err != nil || !registered {
		t.Fatalf("RegisterActiveSession = %v, %v; want true, nil", registered, err)
	}
	if err := tracker.Start(ctx, runID, "agent-1", "user-a", ""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	requested, err := tracker.RequestCancelActiveSession(ctx, sessionID, token)
	if err != nil || !requested {
		t.Fatalf("RequestCancelActiveSession = %v, %v; want true, nil", requested, err)
	}
	if err := tracker.MarkCancelled(ctx, runID); err != nil {
		t.Fatalf("MarkCancelled: %v", err)
	}
	released, err := tracker.ReleaseActiveSession(ctx, sessionID, token)
	if err != nil || !released {
		t.Fatalf("ReleaseActiveSession = %v, %v; want true, nil", released, err)
	}

	svc := NewAgentServiceWithOptions(nil, nil, tracker)
	if err := svc.CancelSessionRun(t.Context(), "user-a", "session-persisted"); err != nil {
		t.Fatalf("late owner cancel error = %v", err)
	}
	if mr.Exists(sessionID + "-cancel") {
		t.Fatal("late cancel recreated a marker after the run finished")
	}
	if err := svc.CancelSessionRun(t.Context(), "user-a", "missing-session"); err != nil {
		t.Fatalf("missing session cancel must be idempotent: %v", err)
	}
	if mr.Exists("missing-session-cancel") {
		t.Fatal("unknown-session cancel created a marker")
	}
}

func TestRunAgentInitializationFailureReleasesActiveSession(t *testing.T) {
	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)
	if err := testDB.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate agent tables: %v", err)
	}
	if err := testDB.Create(&entity.UserCanvas{ID: "agent-1", UserID: "user-a"}).Error; err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	tracker, mr := newAgentCancelTracker(t)
	svc := NewAgentServiceWithOptions(nil, nil, tracker)

	const sessionID = "session-initialization-failure"
	if _, err := svc.RunAgent(t.Context(), "user-a", "agent-1", sessionID, "missing-version", "", nil); err == nil {
		t.Fatal("RunAgent with missing version returned nil error")
	}
	active, err := tracker.GetActiveSession(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("GetActiveSession: %v", err)
	}
	if active != nil {
		t.Fatalf("active session remains after initialization failure: %#v", active)
	}
	if mr.Exists(sessionID + "-cancel") {
		t.Fatal("cancel marker remains after initialization failure")
	}
	svc.runMu.Lock()
	_, locallyActive := svc.activeSessions[sessionID]
	svc.runMu.Unlock()
	if locallyActive {
		t.Fatal("local active session remains after initialization failure")
	}
}
