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
	"time"

	"ragflow/internal/common"
	redis2 "ragflow/internal/engine/redis"
)

const (
	taskClaimKeyPrefix      = "ragflow:ingestion:task-claim:"
	taskClaimReleaseTimeout = 2 * time.Second
)

var errTaskClaimLost = errors.New("ingestion task claim lost")

type taskClaimStore interface {
	Available() bool
	Acquire(ctx context.Context, taskID, owner string, ttl time.Duration) (bool, error)
	Renew(ctx context.Context, taskID, owner string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, taskID, owner string) (bool, error)
}

type activeTaskClaim struct {
	ctx                  context.Context
	cancel               context.CancelCauseFunc
	distributed          bool
	renewalDone          chan struct{}
	messageHeartbeatStop func()
}

type redisTaskClaimStore struct{}

func (redisTaskClaimStore) Available() bool {
	return redis2.Get() != nil
}

func (redisTaskClaimStore) Acquire(ctx context.Context, taskID, owner string, ttl time.Duration) (bool, error) {
	client := redis2.Get()
	if client == nil {
		return false, fmt.Errorf("redis task claims unavailable")
	}
	return client.AcquireLease(ctx, taskClaimKeyPrefix+taskID, owner, ttl)
}

func (redisTaskClaimStore) Renew(ctx context.Context, taskID, owner string, ttl time.Duration) (bool, error) {
	client := redis2.Get()
	if client == nil {
		return false, fmt.Errorf("redis task claims unavailable")
	}
	return client.RenewLease(ctx, taskClaimKeyPrefix+taskID, owner, ttl)
}

func (redisTaskClaimStore) Release(ctx context.Context, taskID, owner string) (bool, error) {
	client := redis2.Get()
	if client == nil {
		return false, fmt.Errorf("redis task claims unavailable")
	}
	return client.ReleaseLease(ctx, taskClaimKeyPrefix+taskID, owner)
}

// claimTask takes both the process-local claim and, when Redis is configured,
// an owner-scoped distributed lease. Its returned context covers queue wait as
// well as execution, so losing the lease prevents a delayed local worker from
// starting after another ingestor has taken ownership.
func (e *Ingestor) claimTask(ctx context.Context, taskID string) (context.Context, bool, error) {
	e.tasksMu.RLock()
	_, locallyClaimed := e.currentTasks[taskID]
	e.tasksMu.RUnlock()
	if locallyClaimed {
		return nil, false, nil
	}

	distributed := e.taskClaims != nil && e.taskClaims.Available()
	if distributed {
		acquired, err := e.taskClaims.Acquire(ctx, taskID, e.id, e.taskClaimTTL)
		if err != nil {
			return nil, false, err
		}
		if !acquired {
			return nil, false, nil
		}
	}
	claimCtx, cancel := context.WithCancelCause(ctx)
	claim := &activeTaskClaim{
		ctx:         claimCtx,
		cancel:      cancel,
		distributed: distributed,
	}
	if distributed {
		claim.renewalDone = make(chan struct{})
	}

	e.tasksMu.Lock()
	if _, exists := e.currentTasks[taskID]; exists {
		e.tasksMu.Unlock()
		cancel(context.Canceled)
		if distributed {
			releaseCtx, cancel := context.WithTimeout(context.Background(), taskClaimReleaseTimeout)
			defer cancel()
			_, _ = e.taskClaims.Release(releaseCtx, taskID, e.id)
		}
		return nil, false, nil
	}
	e.currentTasks[taskID] = claim
	e.tasksMu.Unlock()
	if distributed {
		go e.renewTaskClaim(taskID, claim)
	}
	return claimCtx, true, nil
}

// releaseTask drops the local claim and releases the Redis lease only if this
// ingestor still owns it. If Redis is unavailable, the TTL provides recovery.
func (e *Ingestor) releaseTask(taskID string) {
	e.tasksMu.Lock()
	claim := e.currentTasks[taskID]
	var stopMessageHeartbeat func()
	if claim != nil {
		stopMessageHeartbeat = claim.messageHeartbeatStop
		claim.messageHeartbeatStop = nil
	}
	delete(e.currentTasks, taskID)
	e.tasksMu.Unlock()
	if claim == nil {
		return
	}
	if stopMessageHeartbeat != nil {
		stopMessageHeartbeat()
	}
	claim.cancel(context.Canceled)
	if claim.renewalDone != nil {
		<-claim.renewalDone
	}
	if !claim.distributed || e.taskClaims == nil {
		return
	}

	releaseCtx, cancel := context.WithTimeout(context.Background(), taskClaimReleaseTimeout)
	defer cancel()
	if _, err := e.taskClaims.Release(releaseCtx, taskID, e.id); err != nil {
		common.Warn(fmt.Sprintf("release distributed claim for task %s: %v", taskID, err))
	}
}

// renewTaskClaim keeps a distributed claim alive from scheduling through
// settlement. Losing ownership cancels the claim context with a distinct cause
// so the message is Nacked and the task remains RUNNING for redelivery.
func (e *Ingestor) renewTaskClaim(taskID string, claim *activeTaskClaim) {
	defer close(claim.renewalDone)
	if e.taskClaims == nil {
		claim.cancel(errTaskClaimLost)
		return
	}
	refreshInterval := e.taskClaimRefreshInterval
	if refreshInterval <= 0 {
		refreshInterval = e.taskClaimTTL / 3
	}
	if refreshInterval <= 0 {
		claim.cancel(errTaskClaimLost)
		return
	}
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			renewed, err := e.taskClaims.Renew(claim.ctx, taskID, e.id, e.taskClaimTTL)
			if err == nil && renewed {
				continue
			}
			if err != nil {
				common.Error(fmt.Sprintf("renew distributed claim for task %s", taskID), err)
			} else {
				common.Warn(fmt.Sprintf("distributed claim for task %s is no longer owned by ingestor %s", taskID, e.id))
			}
			claim.cancel(errTaskClaimLost)
			return
		case <-claim.ctx.Done():
			return
		}
	}
}
