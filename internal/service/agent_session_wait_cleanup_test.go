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

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"ragflow/internal/agent/canvas"
)

// failRegisterHook makes RegisterActiveSession fail on its next invocation so
// the waiter exits without acquiring the local slot.
type failRegisterHook struct{}

var _ redis.Hook = (*failRegisterHook)(nil)

func (*failRegisterHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (*failRegisterHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) >= 2 {
			if name, ok := args[0].(string); ok && name == "eval" {
				if script, ok := args[1].(string); ok && strings.Contains(script, "EXISTS") && strings.Contains(script, "HSET") {
					return errors.New("injected registration failure")
				}
			}
		}
		return next(ctx, cmd)
	}
}

func (*failRegisterHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// TestRunAgentWaiterEntryRemovedOnCancel asserts that a waiter that exits via
// context cancellation does not leave a sessionWaiters entry behind. Without
// cleanup, a session held by another process never gets a local release, so
// the entry and its channel leak permanently.
func TestRunAgentWaiterEntryRemovedOnCancel(t *testing.T) {
	const sessionID = "session-waiter-cleanup-cancel"
	hook := &registerEvalHook{}
	tracker := newPollingTracker(t, hook)
	svc, _ := setupBusySessionService(t, sessionID, tracker)

	ctx, cancel := context.WithCancel(serviceTestContext(t, "polling-user", "polling-tenant"))
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil)
	}()
	waitForRegistrations(t, hook, 1)

	svc.runMu.Lock()
	_, created := svc.sessionWaiters[sessionID]
	svc.runMu.Unlock()
	if !created {
		t.Fatal("waiter entry was not created; cleanup assertion would be vacuous")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunAgent did not return after cancellation")
	}

	svc.runMu.Lock()
	_, leaked := svc.sessionWaiters[sessionID]
	svc.runMu.Unlock()
	if leaked {
		t.Fatal("sessionWaiters entry leaked after the waiter was cancelled")
	}
}

// TestRunAgentWaiterEntryRemovedOnRegisterError asserts the same cleanup on
// the registration-error exit path.
func TestRunAgentWaiterEntryRemovedOnRegisterError(t *testing.T) {
	const sessionID = "session-waiter-cleanup-register-error"
	client := newFailingClient(t)
	client.AddHook(&failRegisterHook{})
	tracker := canvas.NewRunTrackerWithClient(client, time.Hour)
	svc, _ := setupBusySessionService(t, sessionID, tracker)

	ctx := serviceTestContext(t, "polling-user", "polling-tenant")
	if _, err := svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil); err == nil {
		t.Fatal("RunAgent with failing registration returned nil error")
	}

	svc.runMu.Lock()
	_, leaked := svc.sessionWaiters[sessionID]
	svc.runMu.Unlock()
	if leaked {
		t.Fatal("sessionWaiters entry leaked after a registration error")
	}
}

// newFailingClient returns a redis client wired to miniredis without the
// registration-counting hook, used for error-injection tests.
func newFailingClient(t *testing.T) *redis.Client {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestRunAgentNoReRegisterOnRecheckWhileLocalSlotHeld keeps a local run active
// across the one-second recheck tick and asserts the waiter does not re-enter
// RegisterActiveSession while the local slot is still occupied.
func TestRunAgentNoReRegisterOnRecheckWhileLocalSlotHeld(t *testing.T) {
	const sessionID = "session-wait-recheck-local"
	hook := &registerEvalHook{}
	tracker := newPollingTracker(t, hook)
	svc, _ := setupBusySessionService(t, sessionID, tracker)

	ctx, cancel := context.WithCancel(serviceTestContext(t, "polling-user", "polling-tenant"))
	defer cancel()
	go func() {
		_, _ = svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil)
	}()
	waitForRegistrations(t, hook, 1)

	// Cross the one-second recheck tick while the local slot stays held.
	time.Sleep(1300 * time.Millisecond)
	if got := hook.registrations(); got > 1 {
		t.Fatalf("registrations after the recheck tick with local slot held = %d, want 1", got)
	}
}

// TestRunAgentRemoteLeaseReleasePickedUpByRecheck covers the cross-process
// path: no local slot is occupied, the lease is held elsewhere, and the waiter
// must pick up the release on the recheck tick.
func TestRunAgentRemoteLeaseReleasePickedUpByRecheck(t *testing.T) {
	const sessionID = "session-wait-recheck-remote"
	hook := &registerEvalHook{}
	tracker := newPollingTracker(t, hook)
	svc, _ := setupBusySessionService(t, sessionID, tracker)
	svc.runMu.Lock()
	delete(svc.activeSessions, sessionID)
	svc.runMu.Unlock()

	const remoteToken = "remote-lease-owner"
	if ok, err := tracker.RegisterActiveSession(t.Context(), canvas.ActiveSession{
		SessionID: sessionID, Token: remoteToken, UserID: "remote", CanvasID: "polling-canvas", RunID: "remote-run",
	}); err != nil || !ok {
		t.Fatalf("register remote lease = %v, %v", ok, err)
	}

	ctx, cancel := context.WithCancel(serviceTestContext(t, "polling-user", "polling-tenant"))
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil)
	}()
	// One registration for the remote lease setup, one for the failed waiter.
	waitForRegistrations(t, hook, 2)

	if _, err := tracker.ReleaseActiveSession(t.Context(), sessionID, remoteToken); err != nil {
		t.Fatalf("release remote lease: %v", err)
	}
	deadline := time.Now().Add(1600 * time.Millisecond)
	for hook.registrations() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("waiter did not pick up the remote lease release on recheck (registrations=%d)", hook.registrations())
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunAgent did not return after cancellation")
	}
}
