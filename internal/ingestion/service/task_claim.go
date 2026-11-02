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
	"fmt"
	"sync/atomic"
	"time"

	"ragflow/internal/common"
)

const (
	// defaultTaskClaimTTL is how long a Kvrocks task claim survives without a
	// renewal. It is the crash-failover window: a dead ingestor's claim is
	// reclaimable after this long.
	defaultTaskClaimTTL = 30 * time.Second
	// defaultTaskClaimRenewInterval must stay comfortably below the TTL so a
	// transient Kvrocks error does not expire a live claim.
	defaultTaskClaimRenewInterval = 10 * time.Second

	taskClaimKeyPrefix = "ingestion:task-claim:"
)

// taskClaimStore is the narrow Kvrocks surface the task claim needs. The
// concrete *kvrocks.Client implements it; tests supply an in-memory double.
type taskClaimStore interface {
	SetNX(ctx context.Context, key, value string, exp time.Duration) (bool, error)
	ExpireIfEqual(ctx context.Context, key, expectedValue string, exp time.Duration) (bool, error)
	DeleteIfEqual(ctx context.Context, key, expectedValue string) (bool, error)
}

// taskClaim is one in-flight task claim. owner is the unique token stored in
// Kvrocks for this acquisition; it is never empty for an acquired claim.
type taskClaim struct {
	taskID string
	owner  string
	ctx    context.Context
	cancel context.CancelFunc
	lost   atomic.Bool
}

func (c *taskClaim) Lost() bool {
	return c != nil && c.lost.Load()
}

func taskClaimKey(taskID string) string {
	return taskClaimKeyPrefix + taskID
}

func (e *Ingestor) taskClaimTTLValue() time.Duration {
	if e.taskClaimTTL > 0 {
		return e.taskClaimTTL
	}
	return defaultTaskClaimTTL
}

// acquireTaskClaim takes a Kvrocks claim for taskID. It fails closed: a
// missing store or a SET NX error returns an error and the caller must Nack
// the delivery. There is no process-local fallback.
//
// The returned claim owns a context derived from the ingestor's execution
// context. Callers must pass it to the task context so a lost claim cancels
// the pipeline, and must release it when the task finishes.
func (e *Ingestor) acquireTaskClaim(taskID string) (*taskClaim, bool, error) {
	if e.taskClaims == nil {
		return nil, false, errors.New("ingestion task claim store (kvrocks) is not configured")
	}

	ctx, cancel := context.WithCancel(e.ctx)
	claim := &taskClaim{taskID: taskID, ctx: ctx, cancel: cancel}

	ttl := e.taskClaimTTLValue()
	renewInterval := e.taskClaimRenewInterval
	if renewInterval <= 0 {
		renewInterval = defaultTaskClaimRenewInterval
	}

	owner := fmt.Sprintf("%s:%s", e.id, common.GenerateUUID())
	acquired, err := e.taskClaims.SetNX(ctx, taskClaimKey(taskID), owner, ttl)
	if err != nil {
		cancel()
		return nil, false, fmt.Errorf("acquire task claim for %s: %w", taskID, err)
	}
	if !acquired {
		cancel()
		return nil, false, nil
	}

	claim.owner = owner
	go claim.watch(e.taskClaims, ttl, renewInterval)
	return claim, true, nil
}

// watch renews the Kvrocks claim and cancels the task when the claim can no
// longer be proven. Expiry or a value change means the lease was lost; it is
// never re-acquired, because another ingestor may have owned the task in the
// gap. Transient errors are retried only until the last confirmed renewal is
// older than the safety window, then the worker fails closed.
func (c *taskClaim) watch(store taskClaimStore, ttl, interval time.Duration) {
	lostAfter := ttl * 2 / 3
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastConfirmed := time.Now()

	lose := func(message string) {
		c.lost.Store(true)
		common.Warn(message)
		c.cancel()
	}

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			owned, err := store.ExpireIfEqual(c.ctx, taskClaimKey(c.taskID), c.owner, ttl)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				if time.Since(lastConfirmed) >= lostAfter {
					lose(fmt.Sprintf("task %s: cannot renew kvrocks claim before the safety window; treating claim as lost: %v", c.taskID, err))
					return
				}
				common.Warn(fmt.Sprintf("task %s: renew kvrocks claim failed, retrying: %v", c.taskID, err))
				continue
			}
			if !owned {
				lose(fmt.Sprintf("task %s: kvrocks claim expired or was taken over by another ingestor", c.taskID))
				return
			}
			lastConfirmed = time.Now()
		}
	}
}

// confirmTaskClaim synchronously verifies that the Kvrocks claim still belongs
// to this worker. It narrows the window between the watcher's last renewal and
// a side effect such as PrepareValidatedRun or a terminal write, but it is not
// a fencing token: a process pause after this check can still let a stale owner
// write. A transient Kvrocks error returns true and leaves cancellation to the
// watcher's safety window; a definitive false marks the claim lost and cancels
// the task.
func (e *Ingestor) confirmTaskClaim(ctx context.Context, claim *taskClaim) bool {
	if claim == nil {
		return true
	}
	if claim.Lost() {
		return false
	}
	if claim.owner == "" || e.taskClaims == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	owned, err := e.taskClaims.ExpireIfEqual(checkCtx, taskClaimKey(claim.taskID), claim.owner, e.taskClaimTTLValue())
	if err != nil {
		common.Warn(fmt.Sprintf("task %s: confirm kvrocks claim failed, relying on watcher: %v", claim.taskID, err))
		return true
	}
	if owned {
		return true
	}
	claim.lost.Store(true)
	if claim.cancel != nil {
		claim.cancel()
	}
	common.Warn(fmt.Sprintf("task %s: kvrocks claim no longer owned at synchronous check", claim.taskID))
	return false
}

// releaseTask drops the Kvrocks claim when this worker still owns it. The
// release is owner-checked so a stale worker cannot delete a claim that
// another ingestor has since acquired.
func (e *Ingestor) releaseTask(claim *taskClaim) {
	if claim == nil {
		return
	}
	claim.cancel()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(e.ctx), 5*time.Second)
	defer cancel()
	if _, err := e.taskClaims.DeleteIfEqual(ctx, taskClaimKey(claim.taskID), claim.owner); err != nil {
		common.Warn(fmt.Sprintf("task %s: release kvrocks claim failed: %v", claim.taskID, err))
	}
}
