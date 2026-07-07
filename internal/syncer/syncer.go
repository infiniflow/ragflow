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
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Syncer periodically polls the sync_logs table and dispatches due
// sync/prune tasks to a fixed-size worker pool.
type Syncer struct {
	id             string
	maxConcurrency int
	pollInterval   time.Duration // how often each worker queries for due tasks

	ctx    context.Context
	cancel context.CancelFunc

	workerWg sync.WaitGroup

	// ShutdownCh is closed when Stop() completes.
	ShutdownCh chan struct{}
}

// NewSyncer creates a syncer with the given concurrency and poll interval.
func NewSyncer(maxConcurrency int, pollInterval time.Duration) *Syncer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Syncer{
		id:             utility.GenerateUUID(),
		maxConcurrency: maxConcurrency,
		pollInterval:   pollInterval,
		ctx:            ctx,
		cancel:         cancel,
		ShutdownCh:     make(chan struct{}),
	}
}

// Start launches maxConcurrency worker goroutines.
func (s *Syncer) Start() error {
	common.Info(fmt.Sprintf("Syncer %s starting with %d workers (poll every %v)",
		s.id, s.maxConcurrency, s.pollInterval))

	for i := 0; i < s.maxConcurrency; i++ {
		s.workerWg.Add(1)
		go s.workerLoop(i)
	}
	return nil
}

// Stop cancels all workers and waits for them to finish.
func (s *Syncer) Stop() {
	common.Info(fmt.Sprintf("Stopping syncer %s", s.id))
	s.cancel()
	s.workerWg.Wait()
	close(s.ShutdownCh)
	common.Info(fmt.Sprintf("Syncer %s stopped", s.id))
}

func (s *Syncer) ID() string {
	return s.id
}

// workerLoop periodically polls the DB for due tasks until ctx is cancelled.
func (s *Syncer) workerLoop(workerID int) {
	defer s.workerWg.Done()
	common.Debug(fmt.Sprintf("Syncer worker %d started", workerID))

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			common.Debug(fmt.Sprintf("Syncer worker %d exiting (ctx cancelled)", workerID))
			return
		case <-ticker.C:
			s.pollAndExecute(workerID)
		}
	}
}

// pollAndExecute queries due sync & prune tasks, picks one, and runs it.
func (s *Syncer) pollAndExecute(workerID int) {
	common.Info(fmt.Sprintf("Syncer worker %d polling for due tasks", workerID))
}

// executeSyncTask runs a sync task.
func (s *Syncer) executeSyncTask(task *entity.SyncLogs) {
	common.Info("Executing sync task",
		zap.String("task_id", task.ID),
		zap.String("connector_id", task.ConnectorID),
		zap.String("kb_id", task.KbID))
	// TODO: implement actual data-source-specific sync logic.
	// For now, mark done.
	s.markTaskDone(task.ID, task.ConnectorID)
}

// executePruneTask runs a prune (delete stale docs) task.
func (s *Syncer) executePruneTask(task *entity.SyncLogs) {
	common.Info("Executing prune task",
		zap.String("task_id", task.ID),
		zap.String("connector_id", task.ConnectorID),
		zap.String("kb_id", task.KbID))
	// TODO: implement actual prune logic.
	s.markTaskDone(task.ID, task.ConnectorID)
}

// markTaskDone updates task and connector status to DONE.
func (s *Syncer) markTaskDone(taskID, connectorID string) {
	db := dao.GetDB()
	now := time.Now().Local()

	db.Model(&entity.SyncLogs{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"status":      string(entity.TaskStatusDone),
		"update_time": now,
	})
	db.Model(&entity.Connector{}).Where("id = ?", connectorID).Updates(map[string]interface{}{
		"status":      string(entity.TaskStatusDone),
		"update_time": now,
	})
}
