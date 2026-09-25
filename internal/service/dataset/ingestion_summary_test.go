package dataset

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	syncerconnector "ragflow/internal/syncer/connector"
)

type fakeSyncCheckpointLoader struct {
	states map[string]*syncerconnector.SyncCheckpointState
	err    error
}

func (f fakeSyncCheckpointLoader) LoadSyncCheckpoint(_ context.Context, taskID string) (*syncerconnector.SyncCheckpointState, error) {
	return f.states[taskID], f.err
}

func TestGetDownloadStatusAggregatesLinkedConnectorFiles(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	connectors := []entity.Connector{
		{ID: "connector-checkpoint", TenantID: "tenant-1", Name: "checkpoint", Source: "rss", InputType: "poll", Config: entity.JSONMap{}, Status: dao.SyncStatusSchedule},
		{ID: "connector-fallback", TenantID: "tenant-1", Name: "fallback", Source: "s3", InputType: "poll", Config: entity.JSONMap{}, Status: dao.SyncStatusSchedule},
		{ID: "connector-source-fail", TenantID: "tenant-1", Name: "source-fail", Source: "notion", InputType: "poll", Config: entity.JSONMap{}, Status: dao.SyncStatusSchedule},
		{ID: "connector-unlinked", TenantID: "tenant-1", Name: "unlinked", Source: "webdav", InputType: "poll", Config: entity.JSONMap{}, Status: dao.SyncStatusSchedule},
	}
	if err := db.Create(&connectors).Error; err != nil {
		t.Fatalf("create connectors: %v", err)
	}
	mappings := []entity.Connector2Kb{
		{ID: "mapping-checkpoint", ConnectorID: "connector-checkpoint", KbID: "dataset-1"},
		{ID: "mapping-fallback", ConnectorID: "connector-fallback", KbID: "dataset-1"},
		{ID: "mapping-source-fail", ConnectorID: "connector-source-fail", KbID: "dataset-1"},
	}
	if err := db.Create(&mappings).Error; err != nil {
		t.Fatalf("create connector mappings: %v", err)
	}

	documents := []entity.Document{
		{ID: "doc-rss", KbID: "dataset-1", ParserID: "naive", ParserConfig: entity.JSONMap{}, SourceType: "rss/connector-checkpoint", Type: "text", CreatedBy: "tenant-1", Name: sptr("rss.txt"), Suffix: "txt"},
		{ID: "doc-s3", KbID: "dataset-1", ParserID: "naive", ParserConfig: entity.JSONMap{}, SourceType: "s3/connector-fallback", Type: "text", CreatedBy: "tenant-1", Name: sptr("s3.txt"), Suffix: "txt"},
		{ID: "doc-local", KbID: "dataset-1", ParserID: "naive", ParserConfig: entity.JSONMap{}, SourceType: "local", Type: "text", CreatedBy: "tenant-1", Name: sptr("local.txt"), Suffix: "txt"},
		{ID: "doc-unlinked", KbID: "dataset-1", ParserID: "naive", ParserConfig: entity.JSONMap{}, SourceType: "webdav/connector-unlinked", Type: "text", CreatedBy: "tenant-1", Name: sptr("unlinked.txt"), Suffix: "txt"},
	}
	if err := db.Create(&documents).Error; err != nil {
		t.Fatalf("create documents: %v", err)
	}

	tasks := []entity.SyncLogs{
		{ID: "task-checkpoint", ConnectorID: "connector-checkpoint", KbID: "dataset-1", TaskType: dao.TaskTypeSync, Status: dao.SyncStatusRunning, NewDocsIndexed: 1, BaseModel: entity.BaseModel{UpdateTime: downloadInt64Pointer(600)}},
		{ID: "task-fallback", ConnectorID: "connector-fallback", KbID: "dataset-1", TaskType: dao.TaskTypeSync, Status: dao.SyncStatusRunning, NewDocsIndexed: 5, ErrorCount: 2, BaseModel: entity.BaseModel{UpdateTime: downloadInt64Pointer(500)}},
		{ID: "task-source-retry", ConnectorID: "connector-source-fail", KbID: "dataset-1", TaskType: dao.TaskTypeSync, Status: dao.SyncStatusRunning, ErrorCount: 9, ErrorClass: "transient", BaseModel: entity.BaseModel{UpdateTime: downloadInt64Pointer(450)}},
		{ID: "task-source-fail", ConnectorID: "connector-source-fail", KbID: "dataset-1", TaskType: dao.TaskTypeSync, Status: dao.SyncStatusFail, ErrorCount: 9, BaseModel: entity.BaseModel{UpdateTime: downloadInt64Pointer(400)}},
		{ID: "task-scheduled", ConnectorID: "connector-checkpoint", KbID: "dataset-1", TaskType: dao.TaskTypeSync, Status: dao.SyncStatusSchedule, NewDocsIndexed: 100, BaseModel: entity.BaseModel{UpdateTime: downloadInt64Pointer(900)}},
		{ID: "task-prune", ConnectorID: "connector-fallback", KbID: "dataset-1", TaskType: dao.TaskTypePrune, Status: dao.SyncStatusRunning, NewDocsIndexed: 100, BaseModel: entity.BaseModel{UpdateTime: downloadInt64Pointer(800)}},
	}
	if err := db.Create(&tasks).Error; err != nil {
		t.Fatalf("create sync tasks: %v", err)
	}

	svc := NewDatasetService()
	svc.checkpointLoader = fakeSyncCheckpointLoader{states: map[string]*syncerconnector.SyncCheckpointState{
		"task-checkpoint": {
			TaskID:      "task-checkpoint",
			ConnectorID: "connector-checkpoint",
			KBID:        "dataset-1",
			Added:       2,
			Updated:     1,
		},
	}}

	status, err := svc.getDownloadStatus(context.Background(), "dataset-1")
	if err != nil {
		t.Fatalf("get download status: %v", err)
	}
	if status.RunningCount != 6 || status.DoneCount != 2 || status.FailCount != 2 {
		t.Fatalf("download status = %+v, want running=6 done=2 fail=2", status)
	}

	svc.checkpointLoader = fakeSyncCheckpointLoader{err: errors.New("checkpoint unavailable")}
	status, err = svc.getDownloadStatus(context.Background(), "dataset-1")
	if err != nil {
		t.Fatalf("get download status after checkpoint error: %v", err)
	}
	if status.RunningCount != 4 || status.DoneCount != 2 || status.FailCount != 2 {
		t.Fatalf("fallback download status = %+v, want running=4 done=2 fail=2", status)
	}
}

func downloadInt64Pointer(value int64) *int64 {
	return &value
}
