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

package service

import (
	"context"
	"fmt"
	"ragflow/internal/dao"
	"strings"
	"time"
)

const (
	// TaskTypeSync is the Python-compatible SYNC task type.
	TaskTypeSync = dao.TaskTypeSync
	// TaskTypePrune is the Python-compatible PRUNE task type.
	TaskTypePrune = dao.TaskTypePrune
)

// SyncTaskContext contains the database rows needed to execute a task.
type SyncTaskContext = dao.SyncTaskContext

// SyncStats accumulates task-level document results.
type SyncStats struct {
	Added      int64
	Updated    int64
	Skipped    int64
	ErrorCount int64
	ErrorMsg   string
}

// Add merges another stats value.
func (s *SyncStats) Add(other SyncStats) {
	s.Added += other.Added
	s.Updated += other.Updated
	s.Skipped += other.Skipped
	s.ErrorCount += other.ErrorCount
	if other.ErrorMsg != "" {
		if s.ErrorMsg != "" {
			s.ErrorMsg += "\n"
		}
		s.ErrorMsg += other.ErrorMsg
	}
}

// AddResult merges one document result.
func (s *SyncStats) AddResult(result DocumentUpsertResult) {
	switch result.Action {
	case DocumentActionAdded:
		s.Added++
	case DocumentActionUpdated:
		s.Updated++
	case DocumentActionSkipped:
		s.Skipped++
	}
}

// SyncTaskService contains sync task business updates.
type SyncTaskService struct {
	taskDAO *dao.SyncTaskDAO
}

// NewSyncTaskService creates a task service.
func NewSyncTaskService(taskDAO *dao.SyncTaskDAO) *SyncTaskService {
	return &SyncTaskService{taskDAO: taskDAO}
}

// ListDueTasks returns due schedule tasks.
func (s *SyncTaskService) ListDueTasks(ctx context.Context, now time.Time) ([]dao.SyncTask, error) {
	tasks, err := s.taskDAO.ListDueTasks(ctx, now, 128)
	if err != nil {
		return nil, err
	}
	out := make([]dao.SyncTask, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, dao.SyncTask{SyncLogs: task})
	}
	return out, nil
}

// Claim marks a scheduled task running if no other scanner claimed it first.
func (s *SyncTaskService) Claim(ctx context.Context, taskID string) (bool, error) {
	claimed, err := s.taskDAO.ClaimTask(ctx, taskID, time.Now().Local())
	if err != nil || !claimed {
		return claimed, err
	}

	taskContext, err := s.taskDAO.GetTaskContext(ctx, taskID)
	if err != nil {
		return true, err
	}

	return true, s.taskDAO.MarkConnectorRunning(ctx, taskContext.Connector.ID)
}

// TODO: refactor some needless func

// GetContext loads a task execution context.
func (s *SyncTaskService) GetContext(ctx context.Context, taskID string) (SyncTaskContext, error) {
	return s.taskDAO.GetTaskContext(ctx, taskID)
}

// RescheduleClaimed puts a claimed task back into schedule state.
func (s *SyncTaskService) RescheduleClaimed(ctx context.Context, taskID string) error {
	return s.taskDAO.RescheduleClaimed(ctx, taskID)
}

// Fail records a failed task.
func (s *SyncTaskService) Fail(ctx context.Context, taskID, connectorID string, err error) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return s.taskDAO.FailTask(ctx, taskID, connectorID, message, 1)
}

// CompleteSync commits a successful SYNC task and schedules the next one.
func (s *SyncTaskService) CompleteSync(ctx context.Context, taskContext SyncTaskContext, pollRangeEnd time.Time, stats SyncStats) error {
	changed := stats.Added + stats.Updated
	return s.taskDAO.CompleteSyncTask(ctx, taskContext, pollRangeEnd, changed, changed, stats.ErrorCount, stats.ErrorMsg)
}

// CompletePrune commits a successful PRUNE task and schedules the next one.
func (s *SyncTaskService) CompletePrune(ctx context.Context, taskContext SyncTaskContext, removed int64) error {
	return s.taskDAO.CompletePruneTask(ctx, taskContext, removed)
}

// RecoverStaleRunning restores timed-out running tasks.
func (s *SyncTaskService) RecoverStaleRunning(ctx context.Context, now time.Time) error {
	_, err := s.taskDAO.RecoverStaleRunning(ctx, now)
	return err
}

// IsFromBeginning reports whether a task is a full sync.
func IsFromBeginning(value *string) bool {
	if value == nil {
		return false
	}
	return strings.TrimSpace(*value) == "1"
}

// SourceType returns document source_type.
func SourceType(source, connectorID string) string {
	return fmt.Sprintf("%s/%s", source, connectorID)
}
