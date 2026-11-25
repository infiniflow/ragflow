package syncer

import (
	"context"
	"errors"
	"io"
	"slices"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	connectormock "ragflow/internal/syncer/connector/mock"
)

type observedPruneConnector struct {
	connectormock.Connector
	onSnapshotComplete func()
	opens              int
}

func (c *observedPruneConnector) OpenPrune(ctx context.Context, request syncerconnector.PruneRequest) (syncerconnector.PruneSession, error) {
	c.opens++
	session, err := c.Connector.OpenPrune(ctx, request)
	if err != nil {
		return nil, err
	}
	return &observedPruneSession{PruneSession: session, onComplete: c.onSnapshotComplete}, nil
}

type observedPruneSession struct {
	syncerconnector.PruneSession
	onComplete func()
}

func (s *observedPruneSession) NextBatch(ctx context.Context) (syncerconnector.PruneBatch, error) {
	batch, err := s.PruneSession.NextBatch(ctx)
	if errors.Is(err, io.EOF) && s.onComplete != nil {
		s.onComplete()
	}
	return batch, err
}

type observedPruneDeleter struct {
	fakeDeleter
	onDelete func()
}

func (d *observedPruneDeleter) DeleteDocument(ctx context.Context, docID string) error {
	if err := d.fakeDeleter.DeleteDocument(ctx, docID); err != nil {
		return err
	}
	if d.onDelete != nil {
		d.onDelete()
	}
	return nil
}

func TestPruneWorkerRespectsCurrentConnectorSetting(t *testing.T) {
	for _, tc := range []struct {
		name        string
		wantOpens   int
		wantDeletes int
		wantNext    int64
	}{
		{name: "enabled", wantOpens: 1, wantDeletes: 2, wantNext: 1},
		{name: "disabled before claim"},
		{name: "disabled after snapshot", wantOpens: 1},
		{name: "disabled after first delete", wantOpens: 1, wantDeletes: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupSyncerDB(t)
			insertTaskContext(t, db, "conn-1", "kb-1", "prune", dao.TaskTypePrune)

			disable := func() {
				if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").
					Update("config", entity.JSONMap{"sync_deleted_files": false}).Error; err != nil {
					t.Fatal(err)
				}
			}
			connector := &observedPruneConnector{}
			deleter := &observedPruneDeleter{}
			switch tc.name {
			case "disabled before claim":
				disable()
			case "disabled after snapshot":
				connector.onSnapshotComplete = disable
			case "disabled after first delete":
				deleter.onDelete = disable
			}

			taskDAO := dao.NewSyncTaskDAO(db)
			taskService := service.NewSyncTaskService(taskDAO)
			pruneService := service.NewSyncPruneService(deleter, fakeStore{ids: map[string]struct{}{"stale-a": {}, "stale-b": {}}})
			registry := syncerconnector.NewRegistry()
			registry.Register("mock", func(context.Context, dao.SyncTaskContext) (syncerconnector.Connector, error) {
				return connector, nil
			})
			coordinator := newCoordinator(taskDAO, taskService, registry, nil, pruneService, fakeStore{})
			t.Cleanup(coordinator.executor.Close)
			worker := NewTaskWorker(nil, taskDAO, taskService, coordinator, NewConnectorLock())
			handle := &fakeTaskHandle{}
			worker.handle(ctx, TaskEnvelope{TaskID: "prune", Handle: handle})

			if handle.acks != 1 || handle.nacks != 0 || connector.opens != tc.wantOpens ||
				len(deleter.deleted) != tc.wantDeletes {
				t.Fatalf("ACKs=%d NACKs=%d opens=%d deletes=%v", handle.acks, handle.nacks, connector.opens, deleter.deleted)
			}
			task, err := taskDAO.GetTaskContext(ctx, "prune")
			if err != nil || task.Task.DocsRemovedFromIndex != int64(tc.wantDeletes) {
				t.Fatalf("prune task=%+v err=%v", task.Task, err)
			}
			if task.Connector.Status == dao.SyncStatusRunning {
				t.Fatal("prune left the connector running")
			}
			var next int64
			if err := db.Model(&entity.SyncLogs{}).Where("id <> ? AND task_type = ?", "prune", dao.TaskTypePrune).Count(&next).Error; err != nil || next != tc.wantNext {
				t.Fatalf("successor prune count=%d err=%v, want %d", next, err, tc.wantNext)
			}

			insertSyncLog(t, db, "conn-1", "kb-1", "sync", dao.TaskTypeSync)
			if claimed, err := taskService.Claim(ctx, "sync"); err != nil || !claimed {
				t.Fatalf("ordinary sync claimed=%v err=%v", claimed, err)
			}
		})
	}
}

func TestSchedulerWakeupRespectsCurrentPruneSetting(t *testing.T) {
	for _, tc := range []struct {
		name      string
		taskType  string
		enabled   bool
		published bool
	}{
		{name: "disabled prune", taskType: dao.TaskTypePrune},
		{name: "enabled prune", taskType: dao.TaskTypePrune, enabled: true, published: true},
		{name: "ordinary sync", taskType: dao.TaskTypeSync, published: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupSyncerDB(t)
			insertTaskContext(t, db, "conn-1", "kb-1", "queued", tc.taskType)
			broker := &fakeSyncTaskBroker{}
			scheduler := NewNATSScheduler(nil, dao.NewSyncTaskDAO(db), broker)
			t.Cleanup(scheduler.stopTimers)
			if err := scheduler.ScheduleTaskAfter(ctx, "queued", time.Hour); err != nil {
				t.Fatal(err)
			}
			scheduler.timerMu.Lock()
			timer := scheduler.timers["queued"]
			scheduler.timerMu.Unlock()
			if timer == nil {
				t.Fatal("wakeup timer was not armed")
			}
			timer.Stop()
			if !tc.enabled {
				if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").
					Update("config", entity.JSONMap{"sync_deleted_files": false}).Error; err != nil {
					t.Fatal(err)
				}
			}

			scheduler.handleTimerWakeup(ctx, "queued", &timer)
			var want []string
			if tc.published {
				want = []string{"queued"}
			}
			if got := broker.publishedIDs(); !slices.Equal(got, want) {
				t.Fatalf("published %v, want %v", got, want)
			}
		})
	}
}
