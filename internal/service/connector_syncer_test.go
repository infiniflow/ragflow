package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
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
		Config:      entity.ConnectorConfig{},
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

	_, code, err := NewConnectorService().UpdateConnector(serviceTestContext(t, "user-1", "user-1"), "conn-1", &UpdateConnectorRequest{
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
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.Knowledgebase{}, &entity.SyncLogs{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	if err := db.Create(&entity.Connector{
		ID:          "conn-1",
		TenantID:    "user-1",
		Name:        "conn-1",
		Source:      "gmail",
		InputType:   "poll",
		Config:      entity.ConnectorConfig{},
		Status:      string(entity.TaskStatusFail),
		RefreshFreq: 0,
		PruneFreq:   0,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	valid := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{
		ID: "kb-1", TenantID: "user-1", CreatedBy: "user-1", Name: "kb-1", EmbdID: "embd",
		Permission: string(entity.TenantPermissionMe), Status: &valid,
	}).Error; err != nil {
		t.Fatalf("insert knowledgebase: %v", err)
	}
	if err := db.Create(&entity.Connector2Kb{ID: "conn-1-kb-1", ConnectorID: "conn-1", KbID: "kb-1", AutoParse: "1"}).Error; err != nil {
		t.Fatalf("insert connector dataset link: %v", err)
	}
	if err := db.Create(&entity.UserTenant{ID: "owner-user-1", UserID: "user-1", TenantID: "user-1", Role: string(permission.RoleOwner), Status: &valid}).Error; err != nil {
		t.Fatalf("insert tenant owner: %v", err)
	}
	if err := db.Create(&entity.SyncLogs{
		ID:            "task-1",
		ConnectorID:   "conn-1",
		KbID:          "kb-1",
		TaskType:      dao.TaskTypeSync,
		Status:        string(entity.TaskStatusFail),
		FromBeginning: stringPtr("1"),
		ErrorMsg:      "sync task failed after 2 retries, last error: unexpected EOF",
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

	ok, code, err := NewConnectorService().ResumeFailedSync(serviceTestContext(t, "user-1", "user-1"), "conn-1", &ResumeFailedSyncRequest{KbID: "kb-1", TaskID: "task-1"})
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
		Config:      entity.ConnectorConfig{},
		Status:      string(entity.TaskStatusFail),
		RefreshFreq: 0,
		PruneFreq:   0,
		TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:         "kb-1",
		TenantID:   "user-1",
		Name:       "kb-1",
		CreatedBy:  "user-1",
		EmbdID:     "embd",
		Permission: string(entity.TenantPermissionMe),
		Status:     sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := db.Create(&entity.UserTenant{
		ID: "ut-owner", UserID: "user-1", TenantID: "user-1",
		Role: "owner", Status: sptr(string(entity.StatusValid)),
	}).Error; err != nil {
		t.Fatalf("insert tenant owner: %v", err)
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

	ok, code, err := NewConnectorService().RebuildConnector(serviceTestContext(t, "user-1", "user-1"), "conn-1", "kb-1")
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
		InputType: "poll", Config: entity.ConnectorConfig{}, Status: string(entity.TaskStatusSchedule),
		RefreshFreq: 0, PruneFreq: 0, TimeoutSecs: 60,
	}).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	// kb-1 is owned by the caller but is never bound to conn-1.
	if err := db.Create(&entity.Knowledgebase{ID: "kb-1", TenantID: "user-1", Name: "kb-1", CreatedBy: "user-1", EmbdID: "embd", Permission: string(entity.TenantPermissionMe), Status: sptr(string(entity.StatusValid))}).Error; err != nil {
		t.Fatalf("insert kb-1: %v", err)
	}
	// kb-foreign belongs to another tenant.
	if err := db.Create(&entity.Knowledgebase{ID: "kb-foreign", TenantID: "user-2", Name: "kb-foreign", CreatedBy: "user-2", EmbdID: "embd", Permission: string(entity.TenantPermissionMe), Status: sptr(string(entity.StatusValid))}).Error; err != nil {
		t.Fatalf("insert kb-foreign: %v", err)
	}
	if err := db.Create([]entity.UserTenant{
		{ID: "ut-owner-1", UserID: "user-1", TenantID: "user-1", Role: "owner", Status: sptr(string(entity.StatusValid))},
		{ID: "ut-owner-2", UserID: "user-2", TenantID: "user-2", Role: "owner", Status: sptr(string(entity.StatusValid))},
	}).Error; err != nil {
		t.Fatalf("insert tenant owners: %v", err)
	}

	svc := NewConnectorService()

	// The caller can access kb-1, but conn-1 is not bound to it: denied before
	// any documents are listed or deleted.
	ok, code, err := svc.RebuildConnector(serviceTestContext(t, "user-1", "user-1"), "conn-1", "kb-1")
	if err == nil || !errors.Is(err, ErrConnectorNotBoundToKB) {
		t.Fatalf("err = %v, want ErrConnectorNotBoundToKB", err)
	}
	if ok || code != common.CodeAuthenticationError {
		t.Fatalf("ok/code = %v/%v, want false/authentication error", ok, code)
	}

	// The caller cannot access a cross-tenant kb at all: denied up front.
	ok, code, err = svc.RebuildConnector(serviceTestContext(t, "user-1", "user-1"), "conn-1", "kb-foreign")
	if err == nil || (!errors.Is(err, permission.ErrResourceNotFound) && !errors.Is(err, permission.ErrPermissionDenied)) {
		t.Fatalf("err = %v, want hidden cross-tenant resource", err)
	}
	if ok || (code != common.CodeForbidden && code != common.CodeNotFound) {
		t.Fatalf("ok/code = %v/%v, want false/denied", ok, code)
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
		Config:      entity.ConnectorConfig{},
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

	_, code, err := NewConnectorService().UpdateConnector(serviceTestContext(t, "user-1", "user-1"), "conn-1", &UpdateConnectorRequest{
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

func TestUpdateRestAPIWindowScopeBackfillsWithoutRebuilding(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.SyncLogs{}); err != nil {
		t.Fatalf("migrate connector tables: %v", err)
	}
	const connectorID, kbID, userID = "conn-1", "kb-1", "user-1"
	windowConfig := func(start string) entity.JSONMap {
		return entity.JSONMap{
			"url": "https://api.example.test/records", "method": "GET", "items_path": "$.records",
			"id_field": "id", "poll_timestamp_field": "updated_at", "content_fields": "text",
			"request_window": map[string]any{"start_param": "from", "end_param": "to", "initial_start": start},
		}
	}
	connector := entity.Connector{
		ID: connectorID, TenantID: userID, Name: "Wiki", Source: "rest_api", InputType: "poll",
		Config: entity.ConnectorConfig(windowConfig("2026-10-01T00:00:00Z")),
		Status: string(entity.TaskStatusSchedule),
	}
	if err := db.Create(&connector).Error; err != nil {
		t.Fatalf("insert connector: %v", err)
	}
	if err := db.Create(&entity.Connector2Kb{ID: "link-1", ConnectorID: connectorID, KbID: kbID, AutoParse: "1"}).Error; err != nil {
		t.Fatalf("insert connector link: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{ID: kbID, TenantID: userID, Name: "Wiki", CreatedBy: userID, EmbdID: "embd"}).Error; err != nil {
		t.Fatalf("insert knowledge base: %v", err)
	}
	if err := db.Create(&entity.Document{ID: "existing-page", KbID: kbID, ParserConfig: entity.JSONMap{}, SourceType: SourceType("rest_api", connectorID)}).Error; err != nil {
		t.Fatalf("insert existing page: %v", err)
	}

	waterline := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	createdAt := waterline.UnixMilli()
	for id, status := range map[string]entity.TaskStatus{
		"old-success": entity.TaskStatusDone,
		"old-active":  entity.TaskStatusRunning,
		"old-failed":  entity.TaskStatusFail,
	} {
		if err := db.Create(&entity.SyncLogs{
			ID: id, ConnectorID: connectorID, KbID: kbID, TaskType: dao.TaskTypeSync, Status: string(status),
			PollRangeStart: entity.NewFlexibleTime(&waterline), PollRangeEnd: entity.NewFlexibleTime(&waterline),
			BaseModel: entity.BaseModel{CreateTime: &createdAt},
		}).Error; err != nil {
			t.Fatalf("insert prior task: %v", err)
		}
	}

	publisher := &fakeSyncerTaskPublisher{}
	previousPublisher := getSyncerTaskPublisher
	getSyncerTaskPublisher = func() (syncTaskPublisher, bool) { return publisher, true }
	t.Cleanup(func() { getSyncerTaskPublisher = previousPublisher })
	svc := NewConnectorService()
	ctx := serviceTestContext(t, userID, userID)

	update := func(req *UpdateConnectorRequest) entity.SyncLogs {
		t.Helper()
		before := len(publisher.taskIDs)
		if _, code, err := svc.UpdateConnector(ctx, connectorID, req); err != nil || code != common.CodeSuccess {
			t.Fatalf("UpdateConnector: code=%v, err=%v", code, err)
		}
		if len(publisher.taskIDs) != before+1 {
			t.Fatalf("update published %d tasks, want one", len(publisher.taskIDs)-before)
		}
		var task entity.SyncLogs
		if err := db.Take(&task, "id = ?", publisher.taskIDs[before]).Error; err != nil {
			t.Fatalf("load task: %v", err)
		}
		return task
	}

	newScope := windowConfig("2026-10-02T00:00:00Z")
	backfill := update(&UpdateConnectorRequest{Config: newScope})
	if !IsFromBeginning(backfill.FromBeginning) || backfill.PollRangeStart != nil {
		t.Fatalf("new scope must start from the beginning: %+v", backfill)
	}
	var oldTask entity.SyncLogs
	if err := db.Take(&oldTask, "id = ?", "old-active").Error; err != nil || oldTask.Status != string(entity.TaskStatusCancel) {
		t.Fatalf("old task status = %q, err = %v", oldTask.Status, err)
	}
	if err := db.Take(&entity.Document{}, "id = ?", "existing-page").Error; err != nil {
		t.Fatalf("scope save removed an existing page: %v", err)
	}
	if err := svc.connectorDAO.ResumeFailedSyncTask(ctx, db, connectorID, kbID, "old-failed"); !errors.Is(err, dao.ErrConnectorSyncScopeChanged) {
		t.Fatalf("resume from old scope: got %v, want scope-change error", err)
	}

	if err := db.Model(&backfill).Update("status", string(entity.TaskStatusFail)).Error; err != nil {
		t.Fatalf("mark full scan failed: %v", err)
	}
	if err := svc.connectorDAO.ResumeFailedSyncTask(ctx, db, connectorID, kbID, backfill.ID); err != nil {
		t.Fatalf("resume current scope: %v", err)
	}
	// A fresh schedule also keeps the full scan after a failure.
	if err := db.Model(&backfill).Update("status", string(entity.TaskStatusFail)).Error; err != nil {
		t.Fatalf("mark resumed scan failed: %v", err)
	}
	retry := update(&UpdateConnectorRequest{Status: string(entity.TaskStatusSchedule)})
	if !IsFromBeginning(retry.FromBeginning) || retry.PollRangeStart != nil {
		t.Fatalf("rescheduled backfill used an older scope's waterline: %+v", retry)
	}

	newWaterline := waterline.Add(30 * time.Minute)
	if err := db.Model(&retry).Updates(map[string]any{
		"status": string(entity.TaskStatusDone), "poll_range_end": entity.NewFlexibleTime(&newWaterline),
	}).Error; err != nil {
		t.Fatalf("mark full scan successful: %v", err)
	}
	refreshFreq := int64(600)
	incremental := update(&UpdateConnectorRequest{Config: newScope, RefreshFreq: &refreshFreq, Reschedule: true})
	if IsFromBeginning(incremental.FromBeginning) || incremental.PollRangeStart == nil ||
		!incremental.PollRangeStart.Time().Equal(newWaterline) {
		t.Fatalf("interval-only change did not reuse the new waterline: %+v", incremental)
	}
}
