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

// run_tracker.go persists canvas-run business metadata to a Redis Hash.
// See plan §2.6 (Key 2: "agent:run:{run_id}"). This is the *business*
// channel — checkpoint payload (eino bytes) lives in checkpoint_store.go.
//
// Status code mapping (stored as int under the "status" field):
//
//	0 = running, 1 = succeeded, 2 = failed, 3 = cancelled.
package canvas

import (
	"context"
	"errors"
	redis2 "ragflow/internal/engine/redis"
	"time"

	"github.com/redis/go-redis/v9"
)

// runKeyPrefix is the Redis Hash key namespace for run metadata.
// The full key is "agent:run:{run_id}".
const runKeyPrefix = "agent:run:"

// runStatus values for the "status" hash field.
const (
	runStatusRunning   = "0"
	runStatusSucceeded = "1"
	runStatusFailed    = "2"
	runStatusCancelled = "3"
)

func runKey(runID string) string { return runKeyPrefix + runID }

// RunTracker manages canvas-run metadata (canvas_id, status, checkpoint
// link, resume chain, ...) on a Redis Hash. Operations are explicit — the
// eino CheckPointStore does NOT write these fields, so callers (HTTP
// handler, cancel watcher) must invoke Start/Mark* at the right points.
type RunTracker struct {
	client *redis.Client
	ttl    time.Duration
}

// NewRunTracker returns a tracker wired to the global Redis client. When
// the cache is uninitialized, client is nil; methods error in that case
// rather than panicking, and tests can inject a client via struct-literal
// construction.
func NewRunTracker(ttl time.Duration) *RunTracker {
	var client *redis.Client
	if rc := redis2.Get(); rc != nil {
		client = rc.GetClient()
	}
	return &RunTracker{client: client, ttl: ttl}
}

// NewRunTrackerWithClient returns a tracker wired to a caller-supplied
// redis.Client. The intended use is tests that want to drive the
// RunTracker against an in-memory miniredis without touching the
// global Redis cache, but the helper is exported so non-test callers
// (multi-tenant deployments, custom Redis pools) can inject a
// dedicated client without going through the global cache singleton.
func NewRunTrackerWithClient(client *redis.Client, ttl time.Duration) *RunTracker {
	return &RunTracker{client: client, ttl: ttl}
}

// Start records a new run as in-progress. canvasID and tenantID identify
// the source DSL and tenant; parentRunID may be empty for fresh runs and
// carries the source run-id for resume chains (R1 in plan §2.6).
//
// The HSet + Expire are sent through a pipeline so a TTL is set on the
// first write — without that, the key would have no expiry and a crashed
// run would leak the hash.
func (t *RunTracker) Start(ctx context.Context, runID, canvasID, tenantID, parentRunID string) error {
	if t == nil || t.client == nil {
		return errors.New("run tracker: redis client not initialized")
	}
	now := time.Now().UnixMilli()
	key := runKey(runID)
	pipe := t.client.Pipeline()
	pipe.HSet(ctx, key, map[string]any{
		"canvas_id":        canvasID,
		"tenant_id":        tenantID,
		"parent_run_id":    parentRunID,
		"status":           runStatusRunning,
		"cancel_requested": 0,
		"started_at":       now,
	})
	pipe.Expire(ctx, key, t.ttl)
	_, err := pipe.Exec(ctx)
	return err
}

// AttachCheckpoint writes the latest checkpoint id for this run. It is the
// ONLY writer of the "checkpoint_id" field; every W1/W2/W3/W4 path (plan
// §2.6) must call this once before the run goroutine returns.
func (t *RunTracker) AttachCheckpoint(ctx context.Context, runID, checkpointID string) error {
	if t == nil || t.client == nil {
		return errors.New("run tracker: redis client not initialized")
	}
	return t.client.HSet(ctx, runKey(runID), "checkpoint_id", checkpointID).Err()
}

// MarkSucceeded transitions the run to status=1 and stamps finished_at.
func (t *RunTracker) MarkSucceeded(ctx context.Context, runID string) error {
	if t == nil || t.client == nil {
		return errors.New("run tracker: redis client not initialized")
	}
	return t.client.HSet(ctx, runKey(runID),
		"status", runStatusSucceeded,
		"finished_at", time.Now().UnixMilli(),
	).Err()
}

// MarkFailed transitions the run to status=2 and records the reason.
func (t *RunTracker) MarkFailed(ctx context.Context, runID, reason string) error {
	if t == nil || t.client == nil {
		return errors.New("run tracker: redis client not initialized")
	}
	return t.client.HSet(ctx, runKey(runID),
		"status", runStatusFailed,
		"finished_at", time.Now().UnixMilli(),
		"failure_reason", reason,
	).Err()
}

// MarkCancelled transitions the run to status=3 and sets the cancel flag.
func (t *RunTracker) MarkCancelled(ctx context.Context, runID string) error {
	if t == nil || t.client == nil {
		return errors.New("run tracker: redis client not initialized")
	}
	return t.client.HSet(ctx, runKey(runID),
		"status", runStatusCancelled,
		"finished_at", time.Now().UnixMilli(),
		"cancel_requested", 1,
	).Err()
}

// Get returns all hash fields for a run. The empty map (not nil) plus a
// nil error means "no such run" — callers can detect this with len(map)==0
// if they need to distinguish from a key that exists with no fields.
func (t *RunTracker) Get(ctx context.Context, runID string) (map[string]string, error) {
	if t == nil || t.client == nil {
		return nil, errors.New("run tracker: redis client not initialized")
	}
	return t.client.HGetAll(ctx, runKey(runID)).Result()
}
