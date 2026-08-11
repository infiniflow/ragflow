package service

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type fakeSyncerTaskPublisher struct {
	taskIDs []string
}

func (p *fakeSyncerTaskPublisher) PublishSyncerTask(taskID string) error {
	p.taskIDs = append(p.taskIDs, taskID)
	return nil
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

	_, code, err := NewConnectorService().UpdateConnector(context.Background(), "conn-1", "user-1", &UpdateConnectorRequest{
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

	_, code, err := NewConnectorService().UpdateConnector(context.Background(), "conn-1", "user-1", &UpdateConnectorRequest{
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
