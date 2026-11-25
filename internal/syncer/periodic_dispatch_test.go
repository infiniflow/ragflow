package syncer

import (
	"context"
	"errors"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	connectormock "ragflow/internal/syncer/connector/mock"
)

func TestPeriodicDispatchRecoversAfterLookupFailure(t *testing.T) {
	for _, previousStatus := range []string{dao.SyncStatusDone, dao.SyncStatusFail} {
		t.Run(previousStatus, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db := setupSyncerDB(t)
				sqlDB, err := db.DB()
				if err != nil {
					t.Fatal(err)
				}
				defer sqlDB.Close()

				taskDAO := dao.NewSyncTaskDAO(db)
				broker := &fakeSyncTaskBroker{}
				scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				go func() {
					if err := scheduler.Run(ctx); !errors.Is(err, context.Canceled) {
						t.Errorf("scheduler exit: %v", err)
					}
				}()
				synctest.Wait()

				insertTaskContext(t, db, "conn-1", "kb-1", "previous", dao.TaskTypeSync)
				connector := &connectormock.Connector{}
				start := time.Now().Add(-time.Hour).UTC()
				if previousStatus == dao.SyncStatusFail {
					connector.ValidateErr = context.DeadlineExceeded
					if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "previous").Updates(map[string]any{
						"retry_count": 3, "error_class": "transient", "poll_range_start": start,
					}).Error; err != nil {
						t.Fatal(err)
					}
				}
				lookupAttempts := failScheduledTaskQueries(t, db, 2)
				taskService := service.NewSyncTaskService(taskDAO)
				coordinator := newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{})
				defer coordinator.executor.Close()
				worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, coordinator, NewConnectorLock()).WithScheduler(scheduler)
				handle := &fakeTaskHandle{}
				worker.handle(ctx, TaskEnvelope{TaskID: "previous", Handle: handle})
				if acks, nacks := handle.counts(); acks != 1 || nacks != 0 {
					t.Fatalf("old delivery ACK/NACK = %d/%d", acks, nacks)
				}
				var previous entity.SyncLogs
				if err := db.First(&previous, "id = ?", "previous").Error; err != nil || previous.Status != previousStatus {
					t.Fatalf("previous status = %s, want %s, err = %v", previous.Status, previousStatus, err)
				}
				worker.handle(ctx, TaskEnvelope{TaskID: "previous", Handle: &fakeTaskHandle{}})
				tasks, err := taskDAO.ListScheduledTasks(ctx, 10, nil)
				if err != nil || len(tasks) != 1 || tasks[0].ID == "previous" {
					t.Fatalf("expected one new scheduled row after redelivery, got %d, err = %v", len(tasks), err)
				}
				next := tasks[0]
				if previousStatus == dao.SyncStatusFail && (next.PollRangeStart == nil || !next.PollRangeStart.Time().Equal(start)) {
					t.Fatal("the next run lost the original polling start")
				}

				time.Sleep(5*time.Minute - time.Nanosecond)
				synctest.Wait()
				if *lookupAttempts != 3 || len(broker.publishedIDs()) != 0 {
					t.Fatalf("before the due time: %d lookups, publishes %v", *lookupAttempts, broker.publishedIDs())
				}

				time.Sleep(time.Nanosecond)
				synctest.Wait()
				if ids := broker.publishedIDs(); !slices.Equal(ids, []string{next.ID}) {
					t.Fatalf("published %v, want exactly %s at its due time", ids, next.ID)
				}
			})
		})
	}
}

func TestPeriodicDispatchStopsAfterPauseOrUnlink(t *testing.T) {
	for _, change := range []struct {
		name  string
		apply func(*gorm.DB) error
	}{
		{"pause", func(db *gorm.DB) error {
			return db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusCancel).Error
		}},
		{"unlink", func(db *gorm.DB) error {
			return db.Where("connector_id = ? AND kb_id = ?", "conn-1", "kb-1").Delete(&entity.Connector2Kb{}).Error
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db := setupSyncerDB(t)
				sqlDB, err := db.DB()
				if err != nil {
					t.Fatal(err)
				}
				defer sqlDB.Close()
				insertTaskContext(t, db, "conn-1", "kb-1", "next", dao.TaskTypeSync)
				broker := &fakeSyncTaskBroker{}
				scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), dao.NewSyncTaskDAO(db), broker)
				defer scheduler.stopTimers()
				lookupAttempts := failScheduledTaskQueries(t, db, 1)

				scheduler.scheduleNextTask(t.Context(), "next")
				if err := change.apply(db); err != nil {
					t.Fatal(err)
				}
				time.Sleep(3 * taskDispatchRetryDelay)
				synctest.Wait()
				if *lookupAttempts != 2 || len(broker.publishedIDs()) != 0 || len(scheduler.timers) != 0 {
					t.Fatalf("inactive task: %d lookups, publishes %v, timers %d", *lookupAttempts, broker.publishedIDs(), len(scheduler.timers))
				}
			})
		})
	}
}

func TestPeriodicWakeupSkipsPausedConnector(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := setupSyncerDB(t)
		insertTaskContext(t, db, "conn-1", "kb-1", "next", dao.TaskTypeSync)
		taskDAO := dao.NewSyncTaskDAO(db)
		broker := &fakeSyncTaskBroker{}
		scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)
		defer scheduler.stopTimers()

		if err := scheduler.ScheduleTaskAfter(t.Context(), "next", time.Second); err != nil {
			t.Fatal(err)
		}
		// Leave a scheduled row to exercise the guards against an old delivery.
		if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusCancel).Error; err != nil {
			t.Fatal(err)
		}
		if claimed, err := service.NewSyncTaskService(taskDAO).Claim(t.Context(), "next"); err != nil || claimed {
			t.Errorf("paused connector: claimed=%t, err=%v", claimed, err)
		}

		time.Sleep(time.Second)
		synctest.Wait()
		if got := broker.publishedIDs(); len(got) != 0 {
			t.Fatalf("paused connector wakeup published %v", got)
		}
		taskContext, err := taskDAO.GetTaskContext(t.Context(), "next")
		if err != nil || taskContext.Connector.Status != dao.SyncStatusCancel || taskContext.Task.Status != dao.SyncStatusSchedule {
			t.Fatalf("paused connector: connector=%s, task=%s, err=%v", taskContext.Connector.Status, taskContext.Task.Status, err)
		}
	})
}

func TestCompletedTaskDoesNotSchedulePausedConnector(t *testing.T) {
	for _, taskType := range []string{dao.TaskTypeSync, dao.TaskTypePrune} {
		t.Run(taskType, func(t *testing.T) {
			db := setupSyncerDB(t)
			insertTaskContext(t, db, "conn-1", "kb-1", "current", taskType)
			taskDAO := dao.NewSyncTaskDAO(db)
			taskService := service.NewSyncTaskService(taskDAO)
			ctx := t.Context()
			if claimed, err := taskService.Claim(ctx, "current"); err != nil || !claimed {
				t.Fatalf("initial claim: claimed=%t, err=%v", claimed, err)
			}
			taskContext, err := taskDAO.GetTaskContext(ctx, "current")
			if err != nil || taskContext.Connector.Status != dao.SyncStatusRunning {
				t.Fatalf("running connector: status=%s, err=%v", taskContext.Connector.Status, err)
			}
			if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusCancel).Error; err != nil {
				t.Fatal(err)
			}
			var next string
			switch taskType {
			case dao.TaskTypeSync:
				next, err = taskService.CompleteSync(ctx, taskContext, time.Now(), service.SyncStats{})
			case dao.TaskTypePrune:
				next, err = taskService.CompletePrune(ctx, taskContext, 0)
			}
			if err != nil || next != "" {
				t.Fatalf("paused completion: next=%q, err=%v", next, err)
			}
			current, err := taskDAO.GetTaskContext(ctx, "current")
			if err != nil || current.Connector.Status != dao.SyncStatusCancel {
				t.Fatalf("paused connector: status=%s, err=%v", current.Connector.Status, err)
			}
		})
	}
}

func TestPeriodicDispatchRecoversAfterPublishFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := setupSyncerDB(t)
		sqlDB, err := db.DB()
		if err != nil {
			t.Fatal(err)
		}
		defer sqlDB.Close()
		insertTaskContext(t, db, "conn-1", "kb-1", "due", dao.TaskTypeSync)
		broker := &failingNextTaskBroker{fakeSyncTaskBroker: &fakeSyncTaskBroker{}}
		scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), dao.NewSyncTaskDAO(db), broker)
		defer scheduler.stopTimers()
		lookupAttempts := failScheduledTaskQueries(t, db, 0)

		scheduler.scheduleNextTask(t.Context(), "due")
		if *lookupAttempts != 1 || len(broker.publishedIDs()) != 0 {
			t.Fatalf("before retry: %d lookups and publishes %v", *lookupAttempts, broker.publishedIDs())
		}
		time.Sleep(taskDispatchRetryDelay)
		synctest.Wait()
		if *lookupAttempts != 2 || !slices.Equal(broker.publishedIDs(), []string{"due"}) {
			t.Fatalf("after retry: %d lookups and publishes %v", *lookupAttempts, broker.publishedIDs())
		}
	})
}

type failingNextTaskBroker struct {
	*fakeSyncTaskBroker
}

func (b *failingNextTaskBroker) PublishSyncerTask(string) error {
	return errors.New("temporary NATS publish failure")
}

func failScheduledTaskQueries(t *testing.T, db *gorm.DB, failures int) *int {
	t.Helper()
	attempts := new(int)
	err := db.Callback().Query().Before("gorm:query").Register("test:fail_scheduled_task_lookup", func(query *gorm.DB) {
		if _, isSyncLog := query.Statement.Model.(*entity.SyncLogs); !isSyncLog || len(query.Statement.Joins) == 0 {
			return
		}
		*attempts++
		if *attempts <= failures {
			_ = query.AddError(errors.New("temporary schedule lookup failure"))
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	return attempts
}
