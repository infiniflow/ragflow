//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"sync"
	"time"
)

// Share the budget across sync, prune and validation in this server process.
// Multiple syncer replicas need a distributed limiter before sharing this budget.
var sharePointLimiters sync.Map

type sharePointLimiter struct {
	mu       sync.Mutex
	next     time.Time
	interval time.Duration
	slots    chan struct{}
}

func newSharePointLimiter(interval time.Duration) *sharePointLimiter {
	return &sharePointLimiter{interval: interval, slots: make(chan struct{}, 2)}
}

func sharedSharePointLimiter(tenantID, clientID string) *sharePointLimiter {
	key := [2]string{tenantID, clientID}
	limiter, _ := sharePointLimiters.LoadOrStore(key, newSharePointLimiter(500*time.Millisecond))
	return limiter.(*sharePointLimiter)
}

func (l *sharePointLimiter) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case l.slots <- struct{}{}:
	}
	for {
		l.mu.Lock()
		delay := time.Until(l.next)
		if delay <= 0 && ctx.Err() == nil {
			l.next = time.Now().Add(l.interval)
			l.mu.Unlock()
			return nil
		}
		l.mu.Unlock()
		if err := waitSharePointRetry(ctx, delay); err != nil {
			l.release()
			return err
		}
	}
}

func (l *sharePointLimiter) release() { <-l.slots }

func (l *sharePointLimiter) postpone(delay time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until := time.Now().Add(delay); until.After(l.next) {
		l.next = until
	}
}

func waitSharePointRetry(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(max(delay, 0))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
