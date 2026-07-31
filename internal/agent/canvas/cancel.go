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

// cancel.go implements the cross-process cancel signal for ordinary Agent
// sessions. A canvas run polls Redis for "{sessionID}-cancel"; when the HTTP
// handler sets the key, the watcher cancels the same context used by the
// workflow.
package canvas

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"ragflow/internal/common"
	redis2 "ragflow/internal/engine/redis"
)

// cancelKeySuffix is appended to the session id to form the Redis key.
const cancelKeySuffix = "-cancel"

func cancelKey(sessionID string) string { return sessionID + cancelKeySuffix }

// cancelPollInterval is the gap between Redis Get polls. 500ms keeps
// cancel latency p99 ≤ 500ms while staying cheap (one GET every half-
// second per active run). Tunable later if a tenant needs lower latency.
const cancelPollInterval = 500 * time.Millisecond

// RequestCancelTTL is the lifetime of the cancel flag in Redis. Long
// enough to outlast any legitimate canvas run; short enough that stale
// flags from a previous run do not poison a later run.
const RequestCancelTTL = 24 * time.Hour

// cancelClientFn resolves the Redis client for cancel operations. It is
// a package-level variable so tests can override it with a miniredis
// client (the production path goes through cache.Get()).
var cancelClientFn = func() (*redis.Client, error) {
	rc := redis2.Get()
	if rc == nil {
		return nil, errors.New("cancel: redis cache not initialized")
	}
	c := rc.GetClient()
	if c == nil {
		return nil, errors.New("cancel: redis client not initialized")
	}
	return c, nil
}

// WatchCancel blocks until either ctx is cancelled or the Redis
// "{sessionID}-cancel" key contains a non-empty value. When fired, it calls
// onCancel exactly once and returns. Polling interval is fixed
// at 500ms (see plan §4.9 — revised 2026-06-03 from 1s to 500ms).
//
// WatchCancel is intended to run as a side goroutine with onCancel wired to
// the cancel function for the workflow context:
//
//	go canvas.WatchCancel(ctx, sessionID, cancelRun)
func WatchCancel(ctx context.Context, sessionID string, onCancel func()) {
	c, err := cancelClientFn()
	if err != nil {
		common.Warn("agent cancel watcher unavailable", zap.String("session_id", sessionID), zap.Error(err))
		return
	}
	key := cancelKey(sessionID)
	ticker := time.NewTicker(cancelPollInterval)
	defer ticker.Stop()
	readFailureLogged := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			v, err := c.Get(ctx, key).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				if !readFailureLogged {
					common.Warn("agent cancel watcher redis read failed", zap.String("session_id", sessionID), zap.Error(err))
					readFailureLogged = true
				}
				continue
			}
			readFailureLogged = false
			if v != "" {
				if onCancel != nil {
					onCancel()
				}
				return
			}
		}
	}
}

// RequestCancel publishes a cancel signal for the given session. The marker
// is cleared atomically when the owning run releases its active lease.
func RequestCancel(ctx context.Context, sessionID string) error {
	c, err := cancelClientFn()
	if err != nil {
		return err
	}
	return c.Set(ctx, cancelKey(sessionID), "x", RequestCancelTTL).Err()
}

// CancelRequested reports whether a cancellation marker is currently
// published for sessionID. Registration uses this as an immediate check after
// acquiring the lease so a cancel that wins the startup race cannot be erased
// before the polling watcher gets its first tick.
func CancelRequested(ctx context.Context, sessionID string) (bool, error) {
	c, err := cancelClientFn()
	if err != nil {
		return false, err
	}
	v, err := c.Get(ctx, cancelKey(sessionID)).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return v != "", nil
}

// ClearCancel removes a session cancel marker. Production run registration
// and release perform this operation atomically with active-session ownership;
// this helper remains useful for Redis-degraded and focused test paths.
func ClearCancel(ctx context.Context, sessionID string) error {
	c, err := cancelClientFn()
	if err != nil {
		return err
	}
	return c.Del(ctx, cancelKey(sessionID)).Err()
}
