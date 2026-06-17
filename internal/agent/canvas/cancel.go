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

// cancel.go implements the cross-process cancel signal. See plan §4.9 —
// a Go canvas run goroutine polls Redis for "{taskID}-cancel"; when the
// HTTP handler sets the key, the watcher fires onCancel. The Redis key
// naming is deliberately identical to the Python task_service.py
// protocol (line 521-523) so Go and Python canvas runs in the same
// tenant can signal each other.
package canvas

import (
	"context"
	"errors"
	redis2 "ragflow/internal/engine/redis"
	"time"

	"github.com/redis/go-redis/v9"
)

// cancelKeySuffix is appended to the task id to form the Redis key.
const cancelKeySuffix = "-cancel"

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
// "{taskID}-cancel" key is set to a non-empty value. When fired, it
// calls onCancel exactly once and returns. Polling interval is fixed
// at 500ms (see plan §4.9 — revised 2026-06-03 from 1s to 500ms).
//
// WatchCancel is intended to run as a side goroutine; the run-loop
// goroutine calls it with onCancel wired to the eino graph interrupt
// callback:
//
//	go func() {
//	    canvas.WatchCancel(ctx, taskID, func() {
//	        interrupt(compose.WithGraphInterruptTimeout(30*time.Second))
//	    })
//	}()
func WatchCancel(ctx context.Context, taskID string, onCancel func()) {
	c, err := cancelClientFn()
	if err != nil {
		// Without Redis the watcher can do nothing. Returning silently
		// matches the rest of the canvas layer: a missing cache is a
		// deployment error surfaced at startup, not at every call.
		return
	}
	key := taskID + cancelKeySuffix
	ticker := time.NewTicker(cancelPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			v, err := c.Get(ctx, key).Result()
			if err != nil && !errors.Is(err, redis.Nil) {
				// Transient Redis error — log by skipping this tick; the
				// next tick will retry. Avoid spinning on persistent
				// failure.
				continue
			}
			if v != "" {
				if onCancel != nil {
					onCancel()
				}
				return
			}
		}
	}
}

// RequestCancel publishes a cancel signal for the given task. The
// 24h TTL matches the Python task_service.py protocol so a flag set
// during one run is still observable by a resume that arrives hours
// later (e.g. after a long client-side wait).
func RequestCancel(ctx context.Context, taskID string) error {
	c, err := cancelClientFn()
	if err != nil {
		return err
	}
	return c.Set(ctx, taskID+cancelKeySuffix, "x", RequestCancelTTL).Err()
}
