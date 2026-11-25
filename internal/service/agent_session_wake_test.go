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
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// registerEvalHook counts distributed active-session registrations. The
// registration Lua script is the only one that contains both EXISTS and
// HSET, which distinguishes it from release/refresh scripts.
type registerEvalHook struct {
	count atomic.Int32
}

var _ redis.Hook = (*registerEvalHook)(nil)

func (*registerEvalHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *registerEvalHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		args := cmd.Args()
		if len(args) >= 2 {
			if name, ok := args[0].(string); ok && strings.EqualFold(name, "eval") {
				if script, ok := args[1].(string); ok && strings.Contains(script, "EXISTS") && strings.Contains(script, "HSET") {
					h.count.Add(1)
				}
			}
		}
		return next(ctx, cmd)
	}
}

func (*registerEvalHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

func (h *registerEvalHook) registrations() int32 { return h.count.Load() }

func newPollingTrackerWithClient(t *testing.T, hook *registerEvalHook) (*canvas.RunTracker, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis.Run: %v", err)
	}
	t.Cleanup(mr.Close)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	client.AddHook(hook)
	t.Cleanup(func() { _ = client.Close() })
	return canvas.NewRunTrackerWithClient(client, time.Hour), client
}

func newPollingTracker(t *testing.T, hook *registerEvalHook) *canvas.RunTracker {
	tracker, _ := newPollingTrackerWithClient(t, hook)
	return tracker
}

// setupBusySessionService builds a service whose local activeSessions slot
// for sessionID is already held, simulating a first run in the same process.
func setupBusySessionService(t *testing.T, sessionID string, tracker *canvas.RunTracker) (*AgentService, *activeAgentRun) {
	t.Helper()
	testDB := setupServiceTestDB(t)
	pushServiceDB(t, testDB)
	const (
		canvasID = "polling-canvas"
		userID   = "polling-user"
		tenantID = "polling-tenant"
	)
	makeCanvasWithDSL(t, canvasID, userID, tenantID, "polling-version", map[string]any{})
	if err := dao.DB.Create(&entity.API4Conversation{
		ID:       sessionID,
		DialogID: canvasID,
		UserID:   userID,
		Message:  json.RawMessage(`[]`),
	}).Error; err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	svc := NewAgentServiceWithOptions(nil, nil, tracker)
	holder := &activeAgentRun{
		sessionID: sessionID,
		userID:    userID,
		canvasID:  canvasID,
	}
	svc.runMu.Lock()
	svc.activeSessions[sessionID] = holder
	svc.runMu.Unlock()
	return svc, holder
}

// waitForRegistrations waits until at least want registrations have happened.
func waitForRegistrations(t *testing.T, hook *registerEvalHook, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for hook.registrations() < want {
		if time.Now().After(deadline) {
			t.Fatalf("registrations = %d, want >= %d", hook.registrations(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestRunAgentWaitingSessionStopsReRegistering verifies that a waiter does not
// re-acquire the distributed lease on a fixed 100ms cadence while another run
// holds the session. A timer-based loop keeps writing the lease; an
// event-driven loop registers once and then blocks on a release signal.
func TestRunAgentWaitingSessionStopsReRegistering(t *testing.T) {
	const sessionID = "session-wait-no-reRegister"
	hook := &registerEvalHook{}
	tracker := newPollingTracker(t, hook)
	svc, _ := setupBusySessionService(t, sessionID, tracker)

	ctx, cancel := context.WithCancel(serviceTestContext(t, "polling-user", "polling-tenant"))
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil)
	}()

	// The waiter registers once, discovers the local slot is busy, and
	// releases the lease before blocking.
	waitForRegistrations(t, hook, 1)
	time.Sleep(350 * time.Millisecond)
	if got := hook.registrations(); got > 1 {
		t.Fatalf("registrations while another run holds the session = %d, want 1 (timer loop keeps re-registering)", got)
	}
}

// TestRunAgentWaitingSessionWakesOnRelease verifies that releasing the local
// session unblocks the waiter without waiting for the slow lease recheck.
func TestRunAgentWaitingSessionWakesOnRelease(t *testing.T) {
	const sessionID = "session-wait-wake"
	hook := &registerEvalHook{}
	tracker := newPollingTracker(t, hook)
	svc, holder := setupBusySessionService(t, sessionID, tracker)

	ctx, cancel := context.WithCancel(serviceTestContext(t, "polling-user", "polling-tenant"))
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil)
	}()
	waitForRegistrations(t, hook, 1)

	svc.releaseLocalSession(sessionID, holder)

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("waiter did not proceed promptly after release (fell back to lease recheck)")
	}
}

// TestRunAgentWaitingSessionRespectsContextCancel verifies that a waiter
// blocked on another run still exits promptly when its context is cancelled.
func TestRunAgentWaitingSessionRespectsContextCancel(t *testing.T) {
	const sessionID = "session-wait-cancel"
	hook := &registerEvalHook{}
	tracker := newPollingTracker(t, hook)
	svc, _ := setupBusySessionService(t, sessionID, tracker)

	ctx, cancel := context.WithCancel(serviceTestContext(t, "polling-user", "polling-tenant"))
	errCh := make(chan error, 1)
	go func() {
		_, err := svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil)
		errCh <- err
	}()
	waitForRegistrations(t, hook, 1)

	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunAgent after cancel = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunAgent did not return after context cancellation")
	}
}

// postEvalGate holds a registration eval's result until the test releases the
// active session, reproducing a release that lands after a failed
// RegisterActiveSession but before the waiter creates its release channel.
type postEvalGate struct {
	evalDone chan struct{}
	resume   chan struct{}
	once     sync.Once
}

var _ redis.Hook = (*postEvalGate)(nil)

func (*postEvalGate) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *postEvalGate) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		err := next(ctx, cmd)
		args := cmd.Args()
		if len(args) >= 2 {
			if name, ok := args[0].(string); ok && strings.EqualFold(name, "eval") {
				if script, ok := args[1].(string); ok && strings.Contains(script, "EXISTS") && strings.Contains(script, "HSET") {
					h.once.Do(func() {
						close(h.evalDone)
						<-h.resume
					})
				}
			}
		}
		return err
	}
}

func (*postEvalGate) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

// TestRunAgentWaitingSessionWakesWhenReleaseRacesRegistration is the
// adversarial lost-wakeup case: the distributed lease and local slot are both
// released while the waiter's failed registration is still returning. The
// waiter must not miss the broadcast and fall back to the slow recheck.
func TestRunAgentWaitingSessionWakesWhenReleaseRacesRegistration(t *testing.T) {
	const sessionID = "session-wait-release-race"
	hook := &registerEvalHook{}
	tracker, client := newPollingTrackerWithClient(t, hook)
	svc, holder := setupBusySessionService(t, sessionID, tracker)

	if ok, err := tracker.RegisterActiveSession(t.Context(), canvas.ActiveSession{
		SessionID: sessionID, Token: holder.leaseToken, UserID: holder.userID,
		CanvasID: holder.canvasID, RunID: "holder-run",
	}); err != nil || !ok {
		t.Fatalf("register holder lease = %v, %v", ok, err)
	}

	gate := &postEvalGate{evalDone: make(chan struct{}), resume: make(chan struct{})}
	client.AddHook(gate)

	ctx, cancel := context.WithCancel(serviceTestContext(t, "polling-user", "polling-tenant"))
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = svc.RunAgent(ctx, "polling-canvas", sessionID, "", "hello", nil)
	}()

	// The waiter's first registration sees the holder's lease and returns false.
	<-gate.evalDone
	if _, err := tracker.ReleaseActiveSession(t.Context(), sessionID, holder.leaseToken); err != nil {
		t.Fatalf("release holder lease: %v", err)
	}
	svc.releaseLocalSession(sessionID, holder)
	close(gate.resume)

	// One registration belongs to the holder setup; the second is the failed
	// registration, and the third must be the immediate retry after the wake.
	deadline := time.Now().Add(100 * time.Millisecond)
	for hook.registrations() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("waiter did not re-register after release raced registration (registrations=%d)", hook.registrations())
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
