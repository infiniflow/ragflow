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

package common

import (
	"context"
	"fmt"
	"time"
)

// waitForReadyInterval is how often a dependency is re-probed while waiting.
const waitForReadyInterval = 500 * time.Millisecond

// waitForReadyLogEvery controls how often "still waiting" is logged: one line
// per this many probes (~5s at the default interval), so a long dependency
// boot stays visible without flooding the log.
const waitForReadyLogEvery = 10

// WaitForReady retries fn until it succeeds, the timeout elapses, or ctx is
// done. It exists so a server process can boot in parallel with its
// infrastructure dependencies (MySQL, Elasticsearch, Kvrocks, NATS): without
// it the only way to order startup is a compose healthcheck gate, which
// serializes the whole stack behind the slowest container's boot.
//
// The first failure is logged immediately, then one line every ~5s, so an
// operator can tell the process is waiting on a dependency rather than hung.
func WaitForReady(ctx context.Context, name string, timeout time.Duration, fn func(ctx context.Context) error) error {
	start := time.Now()
	deadline := start.Add(timeout)
	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := fn(ctx); err == nil {
			if attempt > 1 {
				Info(fmt.Sprintf("Dependency %s is ready after %s (%d attempts)", name, time.Since(start).Round(time.Millisecond), attempt))
			}
			return nil
		} else {
			lastErr = err
			if attempt == 1 || attempt%waitForReadyLogEvery == 0 {
				Warn(fmt.Sprintf("Waiting for dependency %s (attempt %d): %v", name, attempt, err))
			}
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return fmt.Errorf("dependency %s not ready after %s: %w", name, timeout, lastErr)
		}
		wait := waitForReadyInterval
		if remaining < wait {
			wait = remaining
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("dependency %s not ready before shutdown: %w", name, lastErr)
		case <-time.After(wait):
		}
	}
}
