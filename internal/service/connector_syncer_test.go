package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	syncerconnector "ragflow/internal/syncer/connector"
)

type fakeSyncerTaskPublisher struct {
	taskIDs       []string
	wakeupTaskIDs []string
}

func (p *fakeSyncerTaskPublisher) PublishSyncerTask(taskID string) error {
	p.taskIDs = append(p.taskIDs, taskID)
	return nil
}

func (p *fakeSyncerTaskPublisher) PublishSyncerTaskWakeup(taskID string) error {
	p.wakeupTaskIDs = append(p.wakeupTaskIDs, taskID)
	return nil
}

type fakeSyncCheckpointLoader struct {
	state      *syncerconnector.SyncCheckpointState
	deletedIDs []string
}

func (l *fakeSyncCheckpointLoader) LoadSyncCheckpoint(ctx context.Context, taskID string) (*syncerconnector.SyncCheckpointState, error) {
	return l.state, nil
}

func (l *fakeSyncCheckpointLoader) DeleteSyncCheckpoint(ctx context.Context, taskID string) error {
	l.deletedIDs = append(l.deletedIDs, taskID)
	return nil
}

func stringPtr(value string) *string {
	return &value
}

func TestUpdateConnectorSchedulePublishesSyncerTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.Knowledgebase{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	if err := db.Create(&entity.Connector{
		ID:          "conn-1",
		TenantID:    "user-1",
		Name:        "conn-1",
		Source:      "rss",
		InputType:   "poll",
		Config:      entity.JSONMap{},
		Status:      string(entity.TaskStatusCancel),
		RefreshFreq: 0,
		PruneFreq:   0,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "user-1",
		Name:      "kb-1",
		CreatedBy: "user-1",
		EmbdID:    "embd",
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := db.Create(&entity.Connector2Kb{
		ID:          "conn-1-kb-1",
		ConnectorID: "conn-1",
		KbID:        "kb-1",
		AutoParse:   "1",
	}).Error; err != nil {
		t.Fatalf("insert connector2kb: %v", err)
	}

	publisher := &fakeSyncerTaskPublisher{}
	previousPublisher := getSyncerTaskPublisher
	getSyncerTaskPublisher = func() (syncTaskPublisher, bool) {
		return publisher, true
	}
	t.Cleanup(func() { getSyncerTaskPublisher = previousPublisher })

	_, code, err := NewConnectorService().UpdateConnector(t.Context(), "conn-1", "user-1", &UpdateConnectorRequest{
		Status: string(entity.TaskStatusSchedule),
	})
	if err != nil {
		t.Fatalf("UpdateConnector error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want success", code)
	}
	if len(publisher.taskIDs) != 1 {
		t.Fatalf("published task IDs = %v, want one", publisher.taskIDs)
	}

	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", publisher.taskIDs[0]).Error; err != nil {
		t.Fatalf("load published task: %v", err)
	}
	if task.Status != string(entity.TaskStatusSchedule) || task.TaskType != dao.TaskTypeSync {
		t.Fatalf("task status/type = %s/%s, want schedule/sync", task.Status, task.TaskType)
	}
}

func TestResumeFailedSyncSchedulesOriginalTaskFromCheckpoint(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	if err := db.Create(&entity.Connector{
		ID:          "conn-1",
		TenantID:    "user-1",
		Name:        "conn-1",
		Source:      "gmail",
		InputType:   "poll",
		Config:      entity.JSONMap{},
		Status:      string(entity.TaskStatusFail),
		RefreshFreq: 0,
		PruneFreq:   0,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if err := db.Create(&entity.SyncLogs{
		ID:            "task-1",
		ConnectorID:   "conn-1",
		KbID:          "kb-1",
		TaskType:      dao.TaskTypeSync,
		Status:        string(entity.TaskStatusFail),
		FromBeginning: stringPtr("1"),
		ErrorMsg:      "sync task failed after 3 transient retries: unexpected EOF",
		ErrorCount:    3,
	}).Error; err != nil {
		t.Fatalf("insert sync log: %v", err)
	}

	publisher := &fakeSyncerTaskPublisher{}
	previousPublisher := getSyncerTaskPublisher
	getSyncerTaskPublisher = func() (syncTaskPublisher, bool) {
		return publisher, true
	}
	previousLoader := getSyncCheckpointLoader
	store := &fakeSyncCheckpointLoader{state: &syncerconnector.SyncCheckpointState{
		Version:     1,
		TaskID:      "task-1",
		ConnectorID: "conn-1",
		KBID:        "kb-1",
		Checkpoint:  &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", SourceID: "source-1"},
	}}
	getSyncCheckpointLoader = func() (syncCheckpointLoader, bool) {
		return store, true
	}
	t.Cleanup(func() {
		getSyncerTaskPublisher = previousPublisher
		getSyncCheckpointLoader = previousLoader
	})

	ok, code, err := NewConnectorService().ResumeFailedSync(t.Context(), "conn-1", "user-1", &ResumeFailedSyncRequest{KbID: "kb-1", TaskID: "task-1"})
	if err != nil {
		t.Fatalf("ResumeFailedSync error: %v", err)
	}
	if !ok || code != common.CodeSuccess {
		t.Fatalf("ok/code = %v/%v, want true/success", ok, code)
	}
	if len(publisher.wakeupTaskIDs) != 1 || publisher.wakeupTaskIDs[0] != "task-1" {
		t.Fatalf("wakeup task IDs = %v, want task-1", publisher.wakeupTaskIDs)
	}
	if len(publisher.taskIDs) != 0 {
		t.Fatalf("regular publish task IDs = %v, want none", publisher.taskIDs)
	}

	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != string(entity.TaskStatusSchedule) {
		t.Fatalf("task status = %s, want schedule", task.Status)
	}
	if task.ErrorCount != 0 || task.ErrorMsg != "" {
		t.Fatalf("error count/msg = %d/%q, want reset", task.ErrorCount, task.ErrorMsg)
	}
	if task.FromBeginning == nil || *task.FromBeginning != "1" {
		t.Fatalf("from_beginning = %v, want preserved full-sync task", task.FromBeginning)
	}
}

func TestRebuildConnectorDeletesOldSyncCheckpointsBeforePublishing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.Knowledgebase{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	if err := db.Create(&entity.Connector{
		ID:          "conn-1",
		TenantID:    "user-1",
		Name:        "conn-1",
		Source:      "gmail",
		InputType:   "poll",
		Config:      entity.JSONMap{},
		Status:      string(entity.TaskStatusFail),
		RefreshFreq: 0,
		PruneFreq:   0,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "user-1",
		Name:      "kb-1",
		CreatedBy: "user-1",
		EmbdID:    "embd",
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := db.Create(&entity.Connector2Kb{ID: "conn-1-kb-1", ConnectorID: "conn-1", KbID: "kb-1", AutoParse: "1"}).Error; err != nil {
		t.Fatalf("insert connector mapping: %v", err)
	}
	if err := db.Create([]entity.SyncLogs{
		{ID: "failed-sync", ConnectorID: "conn-1", KbID: "kb-1", TaskType: dao.TaskTypeSync, Status: string(entity.TaskStatusFail), ErrorMsg: ""},
		{ID: "done-sync", ConnectorID: "conn-1", KbID: "kb-1", TaskType: dao.TaskTypeSync, Status: string(entity.TaskStatusDone), ErrorMsg: ""},
		{ID: "failed-prune", ConnectorID: "conn-1", KbID: "kb-1", TaskType: dao.TaskTypePrune, Status: string(entity.TaskStatusFail), ErrorMsg: ""},
	}).Error; err != nil {
		t.Fatalf("insert sync logs: %v", err)
	}

	publisher := &fakeSyncerTaskPublisher{}
	previousPublisher := getSyncerTaskPublisher
	getSyncerTaskPublisher = func() (syncTaskPublisher, bool) {
		return publisher, true
	}
	store := &fakeSyncCheckpointLoader{}
	previousDeleter := getSyncCheckpointDeleter
	getSyncCheckpointDeleter = func() (syncCheckpointDeleter, bool) {
		return store, true
	}
	t.Cleanup(func() {
		getSyncerTaskPublisher = previousPublisher
		getSyncCheckpointDeleter = previousDeleter
	})

	ok, code, err := NewConnectorService().RebuildConnector(t.Context(), "conn-1", "user-1", "kb-1")
	if err != nil {
		t.Fatalf("RebuildConnector error: %v", err)
	}
	if !ok || code != common.CodeSuccess {
		t.Fatalf("ok/code = %v/%v, want true/success", ok, code)
	}
	if len(publisher.taskIDs) != 1 {
		t.Fatalf("published task IDs = %v, want one new rebuild task", publisher.taskIDs)
	}
	if len(store.deletedIDs) != 2 {
		t.Fatalf("deleted checkpoint IDs = %v, want failed-sync and done-sync", store.deletedIDs)
	}
	deleted := map[string]bool{}
	for _, taskID := range store.deletedIDs {
		deleted[taskID] = true
	}
	if !deleted["failed-sync"] || !deleted["done-sync"] || deleted["failed-prune"] {
		t.Fatalf("deleted checkpoint IDs = %v", store.deletedIDs)
	}
}

func TestRebuildConnectorRejectsCrossTenantAndUnboundKB(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.Knowledgebase{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	if err := db.Create(&entity.Connector{
		ID: "conn-1", TenantID: "user-1", Name: "conn-1", Source: "gmail",
		InputType: "poll", Config: entity.JSONMap{}, Status: string(entity.TaskStatusSchedule),
		RefreshFreq: 0, PruneFreq: 0, TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	// kb-1 is owned by the caller but is never bound to conn-1.
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: "user-1", Name: "kb-1", CreatedBy: "user-1", EmbdID: "embd"}).Error; err != nil {
		t.Fatalf("insert kb-1: %v", err)
	}
	// kb-foreign belongs to another tenant.
	if err := db.Create(&entity.Knowledgebase{ID: "kb-foreign", TenantID: "user-2", Name: "kb-foreign", CreatedBy: "user-2", EmbdID: "embd"}).Error; err != nil {
		t.Fatalf("insert kb-foreign: %v", err)
	}

	svc := NewConnectorService()

	// The caller can access kb-1, but conn-1 is not bound to it: denied before
	// any documents are listed or deleted.
	ok, code, err := svc.RebuildConnector(t.Context(), "conn-1", "user-1", "kb-1")
	if err == nil || !errors.Is(err, ErrConnectorNotBoundToKB) {
		t.Fatalf("err = %v, want ErrConnectorNotBoundToKB", err)
	}
	if ok || code != common.CodeAuthenticationError {
		t.Fatalf("ok/code = %v/%v, want false/authentication error", ok, code)
	}

	// The caller cannot access a cross-tenant kb at all: denied up front.
	ok, code, err = svc.RebuildConnector(t.Context(), "conn-1", "user-1", "kb-foreign")
	if err == nil || !errors.Is(err, ErrConnectorNoAuth) {
		t.Fatalf("err = %v, want ErrConnectorNoAuth", err)
	}
	if ok || code != common.CodeAuthenticationError {
		t.Fatalf("ok/code = %v/%v, want false/authentication error", ok, code)
	}
}

func TestUpdateConnectorScheduleDoesNotDuplicateRunningTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}

	if err := db.Create(&entity.Connector{
		ID:          "conn-1",
		TenantID:    "user-1",
		Name:        "conn-1",
		Source:      "rss",
		InputType:   "poll",
		Config:      entity.JSONMap{},
		Status:      string(entity.TaskStatusRunning),
		RefreshFreq: 0,
		PruneFreq:   0,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:        "kb-1",
		TenantID:  "user-1",
		Name:      "kb-1",
		CreatedBy: "user-1",
		EmbdID:    "embd",
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := db.Create(&entity.Connector2Kb{
		ID:          "conn-1-kb-1",
		ConnectorID: "conn-1",
		KbID:        "kb-1",
		AutoParse:   "1",
	}).Error; err != nil {
		t.Fatalf("insert connector2kb: %v", err)
	}
	if err := db.Create(&entity.SyncLogs{
		ID:          "running-task",
		ConnectorID: "conn-1",
		KbID:        "kb-1",
		TaskType:    dao.TaskTypeSync,
		Status:      string(entity.TaskStatusRunning),
		ErrorMsg:    "",
	}).Error; err != nil {
		t.Fatalf("insert running task: %v", err)
	}

	publisher := &fakeSyncerTaskPublisher{}
	previousPublisher := getSyncerTaskPublisher
	getSyncerTaskPublisher = func() (syncTaskPublisher, bool) {
		return publisher, true
	}
	t.Cleanup(func() { getSyncerTaskPublisher = previousPublisher })

	_, code, err := NewConnectorService().UpdateConnector(t.Context(), "conn-1", "user-1", &UpdateConnectorRequest{
		Status: string(entity.TaskStatusSchedule),
	})
	if err != nil {
		t.Fatalf("UpdateConnector error: %v", err)
	}
	if code != common.CodeSuccess {
		t.Fatalf("code = %v, want success", code)
	}
	if len(publisher.taskIDs) != 0 {
		t.Fatalf("published task IDs = %v, want none", publisher.taskIDs)
	}

	var activeCount int64
	if err := db.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status IN ?", "conn-1", "kb-1", dao.TaskTypeSync, []string{string(entity.TaskStatusSchedule), string(entity.TaskStatusRunning)}).
		Count(&activeCount).Error; err != nil {
		t.Fatalf("count active tasks: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active tasks = %d, want 1", activeCount)
	}
}
