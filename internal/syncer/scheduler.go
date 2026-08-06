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

package syncer

import (
	"context"
	"errors"
	"ragflow/internal/common"
	"ragflow/internal/service"
	"time"
)

// TaskEnvelope is the only payload sent through the task queue.
type TaskEnvelope struct {
	TaskID string
}

// Scheduler scans due work and enqueues claimed task IDs.
type Scheduler struct {
	pollInterval time.Duration
	queue        chan<- TaskEnvelope
	taskService  *service.SyncTaskService
}

// NewScheduler creates a global scheduler for datasource sync tasks.
func NewScheduler(pollInterval time.Duration, queue chan<- TaskEnvelope, taskService *service.SyncTaskService) *Scheduler {
	return &Scheduler{pollInterval: pollInterval, queue: queue, taskService: taskService}
}

// Run starts recovery and periodic task discovery.
func (s *Scheduler) Run(ctx context.Context) error {
	if err := s.scan(ctx); err != nil && ctx.Err() == nil {
		common.Error("syncer scheduler scan failed", err)
	}
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.scan(ctx); err != nil && ctx.Err() == nil {
				common.Error("syncer scheduler scan failed", err)
			}
		}
	}
}

// scan claims due tasks and places their IDs on the bounded queue.
func (s *Scheduler) scan(ctx context.Context) error {
	now := time.Now()
	if err := s.taskService.RecoverStaleRunning(ctx, now); err != nil {
		return err
	}
	tasks, err := s.taskService.ListDueTasks(ctx, now)
	if err != nil {
		return err
	}
	var claimErr error
	for _, task := range tasks {
		claimed, err := s.taskService.Claim(ctx, task.ID)
		if err != nil {
			claimErr = errors.Join(claimErr, err)
			continue
		}
		if !claimed {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case s.queue <- TaskEnvelope{TaskID: task.ID}:
		}
	}
	return claimErr
}
