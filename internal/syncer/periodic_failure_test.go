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
	"testing"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	connectormock "ragflow/internal/syncer/connector/mock"
)

func TestPeriodicFailureSchedulesNextRound(t *testing.T) {
	for _, taskType := range []string{dao.TaskTypeSync, dao.TaskTypePrune} {
		for _, fullScan := range []string{"0", "1"} {
			t.Run(taskType+"_full_"+fullScan, func(t *testing.T) {
				db := setupSyncerDB(t)
				insertTaskContext(t, db, "conn-1", "kb-1", "task-1", taskType)
				start := time.Now().Add(-24 * time.Hour).UTC().Truncate(time.Second)
				if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Updates(map[string]any{
					"retry_count": 3, "error_class": "transient", "poll_range_start": start,
					"from_beginning": fullScan, "total_docs_indexed": 7,
				}).Error; err != nil {
					t.Fatal(err)
				}
				taskDAO := dao.NewSyncTaskDAO(db)
				taskService := service.NewSyncTaskService(taskDAO)
				connector := &connectormock.Connector{ValidateErr: context.DeadlineExceeded}
				broker := &fakeSyncTaskBroker{}
				scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)
				t.Cleanup(scheduler.stopTimers)
				sink := &fakeSink{}
				pruner := service.NewSyncPruneService(&fakeDeleter{}, fakeStore{})
				worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService,
					newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, pruner, fakeStore{}),
					NewConnectorLock()).WithScheduler(scheduler)
				handle := &fakeTaskHandle{}
				worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1", Handle: handle})
				if acks, nacks := handle.counts(); acks != 1 || nacks != 0 {
					t.Fatalf("delivery acks/nacks = %d/%d", acks, nacks)
				}
				var failed entity.SyncLogs
				if err := db.First(&failed, "id = ?", "task-1").Error; err != nil {
					t.Fatal(err)
				}
				if failed.Status != dao.SyncStatusFail || failed.RetryCount != 4 || failed.ErrorMsg == "" || failed.PollRangeEnd != nil {
					t.Fatalf("failed round lost its status, error or original waterline: %+v", failed)
				}
				tasks, err := taskDAO.ListScheduledTasks(t.Context(), 10, nil)
				if err != nil || len(tasks) != 1 {
					t.Fatalf("next scheduled rounds = %d, err = %v", len(tasks), err)
				}
				next := tasks[0]
				if next.ID == failed.ID || next.RetryCount != 0 || next.TaskType != taskType || next.TotalDocsIndexed != 7 || next.FromBeginning == nil || *next.FromBeginning != fullScan {
					t.Fatalf("unexpected next round: %+v", next.SyncLogs)
				}
				if next.PollRangeStart == nil || !next.PollRangeStart.Time().Equal(start) {
					t.Fatalf("next round advanced or dropped original waterline: %+v", next.PollRangeStart)
				}
				if delay, enabled := scheduler.taskDelay(next, time.Now()); !enabled || delay < 4*time.Minute || delay > 5*time.Minute {
					t.Fatalf("next round delay = %s, enabled = %v", delay, enabled)
				}
				scheduler.timerMu.Lock()
				armed := scheduler.timers[next.ID] != nil && scheduler.timers[failed.ID] == nil
				scheduler.timerMu.Unlock()
				if !armed || len(broker.publishedIDs()) != 0 {
					t.Fatal("next round must wait for its periodic timer")
				}

				// A redelivery of the terminal round cannot create another round.
				worker.handle(t.Context(), TaskEnvelope{TaskID: failed.ID, Handle: &fakeTaskHandle{}})
				var count int64
				if err := db.Model(&entity.SyncLogs{}).Count(&count).Error; err != nil || count != 2 {
					t.Fatalf("duplicate delivery created rows: count=%d, err=%v", count, err)
				}

				// Simulate the next period after the source becomes reachable.
				connector.ValidateErr = nil
				connector.SyncBatches = []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "recovered", UpdatedAt: time.Now()}}}}
				worker.handle(t.Context(), TaskEnvelope{TaskID: next.ID, Handle: &fakeTaskHandle{}})
				var completed entity.SyncLogs
				if err := db.First(&completed, "id = ?", next.ID).Error; err != nil || completed.Status != dao.SyncStatusDone {
					t.Fatalf("next round did not recover: status=%s, err=%v", completed.Status, err)
				}
				if taskType == dao.TaskTypeSync && sink.callCount() != 1 {
					t.Fatalf("recovered document writes = %d", sink.callCount())
				}
			})
		}
	}
}

func TestPeriodicFailureHonorsStopAndBindingState(t *testing.T) {
	for _, scenario := range []string{"canceled_task", "paused_source", "unbound", "deleted_dataset", "zero_frequency", "disabled_prune"} {
		t.Run(scenario, func(t *testing.T) {
			db := setupSyncerDB(t)
			taskType := dao.TaskTypeSync
			if scenario == "disabled_prune" {
				taskType = dao.TaskTypePrune
			}
			insertTaskContext(t, db, "conn-1", "kb-1", "task-1", taskType)
			if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
				t.Fatal(err)
			}
			var err error
			switch scenario {
			case "canceled_task":
				err = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusCancel).Error
			case "paused_source":
				err = db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusCancel).Error
			case "unbound":
				err = db.Where("connector_id = ?", "conn-1").Delete(&entity.Connector2Kb{}).Error
			case "deleted_dataset":
				err = db.Where("id = ?", "kb-1").Delete(&entity.Knowledgebase{}).Error
			case "zero_frequency":
				err = db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("refresh_freq", 0).Error
			case "disabled_prune":
				err = db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("config", entity.JSONMap{"sync_deleted_files": false}).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			_, _, next, err := dao.NewSyncTaskDAO(db).HandleTransientFailure(t.Context(), "task-1", "conn-1", "failed", "transient", 1)
			if err != nil || next != "" {
				t.Fatalf("stopped source scheduled next round: %q, %v", next, err)
			}
			var count int64
			if err := db.Model(&entity.SyncLogs{}).Count(&count).Error; err != nil || count != 1 {
				t.Fatalf("unexpected round count=%d, err=%v", count, err)
			}
			if scenario == "paused_source" {
				var connector entity.Connector
				if err := db.First(&connector, "id = ?", "conn-1").Error; err != nil || connector.Status != dao.SyncStatusCancel {
					t.Fatalf("paused source was resumed: status=%s, err=%v", connector.Status, err)
				}
			}
		})
	}
}

func TestPeriodicFailureDoesNotBlockOtherBindings(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	insertTaskContext(t, db, "conn-1", "kb-2", "task-2", dao.TaskTypeSync)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("refresh_freq", 0).Error; err != nil {
		t.Fatal(err)
	}
	taskDAO := dao.NewSyncTaskDAO(db)
	if _, _, _, err := taskDAO.HandleTransientFailure(t.Context(), "task-1", "conn-1", "failed", "transient", 1); err != nil {
		t.Fatal(err)
	}
	tasks, err := taskDAO.ListScheduledTasks(t.Context(), 10, nil)
	if err != nil || len(tasks) != 1 || tasks[0].ID != "task-2" {
		t.Fatalf("other binding hidden by connector failure: %+v, %v", tasks, err)
	}
	if _, err := taskDAO.GetScheduledTask(t.Context(), "task-2"); err != nil {
		t.Fatalf("other binding cannot be scheduled: %v", err)
	}
	if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusCancel).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := taskDAO.GetScheduledTask(t.Context(), "task-2"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("paused source remains discoverable: %v", err)
	}
	if tasks, err := taskDAO.ListScheduledTasks(t.Context(), 10, nil); err != nil || len(tasks) != 0 {
		t.Fatalf("paused source has discoverable tasks: %+v, %v", tasks, err)
	}
}

func TestPeriodicFailureRollsBackIfSchedulingFails(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("fail_next_round", func(tx *gorm.DB) {
		if tx.Statement.Table == "sync_logs" {
			tx.AddError(errors.New("next round insert failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := dao.NewSyncTaskDAO(db).HandleTransientFailure(t.Context(), "task-1", "conn-1", "failed", "transient", 1); err == nil {
		t.Fatal("expected scheduling transaction failure")
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil || task.Status != dao.SyncStatusRunning || task.RetryCount != 0 {
		t.Fatalf("failure state committed without next round: %+v, %v", task, err)
	}
}
