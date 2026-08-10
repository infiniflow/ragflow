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
	"io"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	connectormock "ragflow/internal/syncer/connector/mock"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// fakeSink records document writes for coordinator tests.
type fakeSink struct {
	mu             sync.Mutex
	delay          time.Duration
	current        int
	maxConcurrent  int
	calls          []service.DocumentUpsertInput
	errBySourceID  map[string]error
	autoParseByDoc map[string]bool
}

// Upsert records one document write.
func (s *fakeSink) Upsert(ctx context.Context, input service.DocumentUpsertInput) (service.DocumentUpsertResult, error) {
	s.mu.Lock()
	s.current++
	if s.current > s.maxConcurrent {
		s.maxConcurrent = s.current
	}
	s.calls = append(s.calls, input)
	if s.autoParseByDoc == nil {
		s.autoParseByDoc = map[string]bool{}
	}
	s.autoParseByDoc[input.SourceDocument.SourceID] = input.AutoParse
	s.mu.Unlock()
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return service.DocumentUpsertResult{}, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	s.mu.Lock()
	s.current--
	s.mu.Unlock()
	if err := s.errBySourceID[input.SourceDocument.SourceID]; err != nil {
		return service.DocumentUpsertResult{}, err
	}
	return service.DocumentUpsertResult{DocID: input.DocumentID, Action: service.DocumentActionAdded}, nil
}

// callCount returns the number of sink calls.
func (s *fakeSink) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

// fakeStore provides document IDs and fingerprints.
type fakeStore struct {
	ids          map[string]struct{}
	fingerprints map[string]string
}

// ListIDs returns configured IDs.
func (s fakeStore) ListIDs(ctx context.Context, kbID, sourceType string) (map[string]struct{}, error) {
	if s.ids == nil {
		return map[string]struct{}{}, nil
	}
	return s.ids, nil
}

// GetFingerprintsByIDs returns configured fingerprints for requested IDs.
func (s fakeStore) GetFingerprintsByIDs(ctx context.Context, kbID, sourceType string, ids []string) (map[string]string, error) {
	result := make(map[string]string, len(ids))
	if s.fingerprints == nil {
		return result, nil
	}
	for _, id := range ids {
		if fingerprint, ok := s.fingerprints[id]; ok {
			result[id] = fingerprint
		}
	}
	return result, nil
}

// fakeDeleter records document deletes.
type fakeDeleter struct {
	mu      sync.Mutex
	deleted []string
}

// DeleteDocument records one delete.
func (d *fakeDeleter) DeleteDocument(ctx context.Context, docID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.deleted = append(d.deleted, docID)
	return nil
}

// setupSyncerDB creates a SQLite test database.
func setupSyncerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err = db.AutoMigrate(&entity.Connector{}, &entity.Connector2Kb{}, &entity.Knowledgebase{}, &entity.SyncLogs{}, &entity.Document{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
	return db
}

// insertTaskContext inserts one connector, mapping, KB, and sync task.
func insertTaskContext(t *testing.T, db *gorm.DB, connectorID, kbID, taskID, taskType string) {
	t.Helper()
	now := time.Now().Add(-time.Hour).Truncate(time.Second)
	ts := now.UnixMilli()
	fromBeginning := "0"
	var existingConnector int64
	if err := db.Model(&entity.Connector{}).Where("id = ?", connectorID).Count(&existingConnector).Error; err != nil {
		t.Fatalf("count connector: %v", err)
	}
	if existingConnector == 0 {
		if err := db.Create(&entity.Connector{
			ID:          connectorID,
			TenantID:    "tenant-1",
			Name:        connectorID,
			Source:      "mock",
			InputType:   "poll",
			Config:      entity.JSONMap{"sync_deleted_files": true},
			RefreshFreq: 0,
			PruneFreq:   0,
			TimeoutSecs: 1,
			Status:      dao.SyncStatusSchedule,
			BaseModel:   entity.BaseModel{UpdateDate: &now, UpdateTime: &ts},
		}).Error; err != nil {
			t.Fatalf("insert connector: %v", err)
		}
	}
	if err := db.Create(&entity.Knowledgebase{
		ID:           kbID,
		TenantID:     "tenant-1",
		Name:         kbID,
		EmbdID:       "embd",
		CreatedBy:    "tenant-1",
		ParserID:     "naive",
		ParserConfig: entity.JSONMap{},
	}).Error; err != nil {
		t.Fatalf("insert kb: %v", err)
	}
	if err := db.Create(&entity.Connector2Kb{ID: connectorID + kbID, ConnectorID: connectorID, KbID: kbID, AutoParse: "1"}).Error; err != nil {
		t.Fatalf("insert mapping: %v", err)
	}
	if err := db.Create(&entity.SyncLogs{
		ID:            taskID,
		ConnectorID:   connectorID,
		KbID:          kbID,
		TaskType:      taskType,
		Status:        dao.SyncStatusSchedule,
		FromBeginning: &fromBeginning,
		TimeStarted:   &now,
		ErrorMsg:      "",
		BaseModel:     entity.BaseModel{UpdateDate: &now, UpdateTime: &ts},
	}).Error; err != nil {
		t.Fatalf("insert task: %v", err)
	}
}

// newTestRegistry creates a mock connector registry.
func newTestRegistry(connectors map[string]*connectormock.Connector) *syncerconnector.Registry {
	registry := syncerconnector.NewRegistry()
	registry.Register("mock", func(ctx context.Context, taskContext any) (syncerconnector.Connector, error) {
		row := taskContext.(dao.SyncTaskContext)
		return connectors[row.Connector.ID], nil
	})
	return registry
}

// newCoordinator creates a test coordinator.
func newCoordinator(taskService *service.SyncTaskService, registry *syncerconnector.Registry, sink service.DocumentSink, pruneService *service.SyncPruneService, store service.DocumentStore, perTask int) *TaskCoordinator {
	return NewTaskCoordinator(TaskCoordinatorConfig{PerTaskItemConcurrency: perTask, ItemRetryCount: 1, ItemRetryBaseDelay: time.Millisecond}, taskService, registry, sink, pruneService, service.NewDocumentIDResolver(store), make(chan struct{}, 16))
}

// TestSchedulerClaimsDueTasks verifies conditional task claiming.
func TestSchedulerClaimsDueTasks(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	taskService := service.NewSyncTaskService(dao.NewSyncTaskDAO(db))
	queue := make(chan TaskEnvelope, 1)
	scheduler := NewScheduler(time.Hour, queue, taskService)
	if err := scheduler.scan(t.Context()); err != nil {
		t.Fatalf("scan: %v", err)
	}
	envelope := <-queue
	if envelope.TaskID != "task-1" {
		t.Fatalf("TaskID = %s, want task-1", envelope.TaskID)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusRunning {
		t.Fatalf("status = %s, want running", task.Status)
	}
}

// TestWorkersRunDifferentConnectorsInParallel verifies task-level parallelism.
func TestWorkersRunDifferentConnectorsInParallel(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	insertTaskContext(t, db, "conn-2", "kb-2", "task-2", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	now := time.Now()
	for _, id := range []string{"task-1", "task-2"} {
		claimed, err := taskDAO.ClaimTask(t.Context(), id, now)
		if err != nil || !claimed {
			t.Fatalf("claim %s: %v %v", id, claimed, err)
		}
	}
	sink := &fakeSink{delay: 120 * time.Millisecond}
	connectors := map[string]*connectormock.Connector{
		"conn-1": {SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "a", UpdatedAt: now}}}}},
		"conn-2": {SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "b", UpdatedAt: now}}}}},
	}
	queue := make(chan TaskEnvelope, 2)
	worker := NewTaskWorker(queue, taskService, newCoordinator(taskService, newTestRegistry(connectors), sink, nil, fakeStore{}, 1), NewConnectorLock())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go worker.Run(ctx, 2)
	queue <- TaskEnvelope{TaskID: "task-1"}
	queue <- TaskEnvelope{TaskID: "task-2"}
	time.Sleep(40 * time.Millisecond)
	sink.mu.Lock()
	maxConcurrent := sink.maxConcurrent
	sink.mu.Unlock()
	if maxConcurrent < 2 {
		t.Fatalf("max concurrent sink calls = %d, want >= 2", maxConcurrent)
	}
}

// TestConnectorLockSerializesSameConnector verifies connector-level mutual exclusion.
func TestConnectorLockSerializesSameConnector(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	insertTaskContext(t, db, "conn-1", "kb-2", "task-2", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	for _, id := range []string{"task-1", "task-2"} {
		if _, err := taskDAO.ClaimTask(t.Context(), id, time.Now()); err != nil {
			t.Fatalf("claim %s: %v", id, err)
		}
	}
	sink := &fakeSink{delay: 100 * time.Millisecond}
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "a", UpdatedAt: time.Now()}}}}}
	queue := make(chan TaskEnvelope, 2)
	worker := NewTaskWorker(queue, taskService, newCoordinator(taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, 1), NewConnectorLock())
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go worker.Run(ctx, 2)
	queue <- TaskEnvelope{TaskID: "task-1"}
	queue <- TaskEnvelope{TaskID: "task-2"}
	time.Sleep(180 * time.Millisecond)
	var scheduled int64
	if err := db.Model(&entity.SyncLogs{}).Where("connector_id = ? AND status = ?", "conn-1", dao.SyncStatusSchedule).Count(&scheduled).Error; err != nil {
		t.Fatalf("count scheduled: %v", err)
	}
	if scheduled == 0 {
		t.Fatalf("expected one same-connector task to be rescheduled")
	}
	sink.mu.Lock()
	maxConcurrent := sink.maxConcurrent
	sink.mu.Unlock()
	if maxConcurrent > 1 {
		t.Fatalf("same connector ran concurrently: %d", maxConcurrent)
	}
}

// TestSyncRunnerReadsBatchesSerially verifies the next batch waits for current items.
func TestSyncRunnerReadsBatchesSerially(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	start := time.Now()
	var secondBatchAt time.Time
	connector := &connectormock.Connector{
		SyncBatches: []syncerconnector.SyncBatch{
			{Documents: []syncerconnector.SourceDocument{{SourceID: "a", UpdatedAt: start}, {SourceID: "b", UpdatedAt: start}}},
			{Documents: []syncerconnector.SourceDocument{{SourceID: "c", UpdatedAt: start}}},
		},
		OnSyncBatch: func(index int) {
			if index == 1 {
				secondBatchAt = time.Now()
			}
		},
	}
	sink := &fakeSink{delay: 80 * time.Millisecond}
	coordinator := newCoordinator(taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, 2)
	taskContext, _ := taskService.GetContext(t.Context(), "task-1")
	if err := coordinator.Execute(t.Context(), taskContext); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if secondBatchAt.Sub(start) < 70*time.Millisecond {
		t.Fatalf("second batch was read before first batch completed")
	}
}

// TestSyncRunnerProcessesBatchItemsInParallel verifies per-batch item concurrency.
func TestSyncRunnerProcessesBatchItemsInParallel(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskService := service.NewSyncTaskService(dao.NewSyncTaskDAO(db))
	now := time.Now()
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "a", UpdatedAt: now}, {SourceID: "b", UpdatedAt: now}, {SourceID: "c", UpdatedAt: now}}}}}
	sink := &fakeSink{delay: 80 * time.Millisecond}
	coordinator := newCoordinator(taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, 3)
	taskContext, _ := taskService.GetContext(t.Context(), "task-1")
	if err := coordinator.Execute(t.Context(), taskContext); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sink.maxConcurrent < 2 {
		t.Fatalf("max concurrent items = %d, want >= 2", sink.maxConcurrent)
	}
}

// TestHash128MatchesPythonGolden verifies Python-compatible xxhash128.
func TestHash128MatchesPythonGolden(t *testing.T) {
	cases := map[string]string{
		"connector-1:source-1":      "0b66c92e6cefff918067e3c42606cda8",
		"kb-1:connector-1:source-1": "ef785a8871b90d910f42cf31a8155476",
		"":                          "99aa06d3014798d86001c324468d497f",
		"hello":                     "b5e9c1ad071b3e7fc779cfaa5e523818",
	}
	for input, want := range cases {
		if got := service.Hash128(input); got != want {
			t.Fatalf("Hash128(%q) = %s, want %s", input, got, want)
		}
	}
}

// TestFingerprintSkipsUnchangedDocument verifies unchanged docs are not fetched or upserted.
func TestFingerprintSkipsUnchangedDocument(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskService := service.NewSyncTaskService(dao.NewSyncTaskDAO(db))
	legacyID := service.Hash128("conn-1:source-1")
	store := fakeStore{ids: map[string]struct{}{legacyID: {}}, fingerprints: map[string]string{legacyID: "fp-1"}}
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "source-1", Fingerprint: "fp-1", FetchRef: &syncerconnector.FetchReference{Key: "lazy"}, UpdatedAt: time.Now()}}}}}
	sink := &fakeSink{}
	coordinator := newCoordinator(taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, store, 1)
	taskContext, _ := taskService.GetContext(t.Context(), "task-1")
	if err := coordinator.Execute(t.Context(), taskContext); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sink.callCount() != 0 {
		t.Fatalf("sink calls = %d, want 0", sink.callCount())
	}
}

// TestAutoParseFlagFlowsToSink verifies connector2kb.auto_parse is preserved.
func TestAutoParseFlagFlowsToSink(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	if err := db.Model(&entity.Connector2Kb{}).Where("connector_id = ? AND kb_id = ?", "conn-1", "kb-1").Update("auto_parse", "0").Error; err != nil {
		t.Fatalf("disable auto_parse: %v", err)
	}
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskService := service.NewSyncTaskService(dao.NewSyncTaskDAO(db))
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "source-1", Blob: []byte("x"), UpdatedAt: time.Now()}}}}}
	sink := &fakeSink{}
	coordinator := newCoordinator(taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, 1)
	taskContext, _ := taskService.GetContext(t.Context(), "task-1")
	if err := coordinator.Execute(t.Context(), taskContext); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sink.autoParseByDoc["source-1"] {
		t.Fatalf("auto_parse flowed as true, want false")
	}
}

// TestBatchFailureDoesNotAdvanceWaterline verifies failed tasks keep poll_range_end.
func TestBatchFailureDoesNotAdvanceWaterline(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskService := service.NewSyncTaskService(dao.NewSyncTaskDAO(db))
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "bad", Blob: []byte("x"), UpdatedAt: time.Now()}}}}}
	sink := &fakeSink{errBySourceID: map[string]error{"bad": errors.New("boom")}}
	queue := make(chan TaskEnvelope, 1)
	worker := NewTaskWorker(queue, taskService, newCoordinator(taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, 1), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusFail {
		t.Fatalf("status = %s, want fail", task.Status)
	}
	if task.PollRangeEnd != nil {
		t.Fatalf("poll_range_end advanced on failure")
	}
}

// TestRecoverStaleRunningTasks verifies timeout recovery.
func TestRecoverStaleRunningTasks(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	started := time.Now().Add(-2 * time.Hour)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Updates(map[string]any{"status": dao.SyncStatusRunning, "time_started": started}).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}
	taskService := service.NewSyncTaskService(dao.NewSyncTaskDAO(db))
	if err := taskService.RecoverStaleRunning(t.Context(), time.Now()); err != nil {
		t.Fatalf("recover: %v", err)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusSchedule {
		t.Fatalf("status = %s, want schedule", task.Status)
	}
}

// TestPruneSourceFailureDoesNotDelete verifies incomplete source listings never delete.
func TestPruneSourceFailureDoesNotDelete(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypePrune)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskService := service.NewSyncTaskService(dao.NewSyncTaskDAO(db))
	deleter := &fakeDeleter{}
	pruneService := service.NewSyncPruneService(deleter, fakeStore{ids: map[string]struct{}{"stale": {}}})
	connector := &connectormock.Connector{PruneErrAt: 1, PruneBatches: []syncerconnector.PruneBatch{{Documents: []syncerconnector.SlimDocument{{SourceID: "keep"}}}}}
	queue := make(chan TaskEnvelope, 1)
	worker := NewTaskWorker(queue, taskService, newCoordinator(taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, pruneService, fakeStore{}, 1), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})
	if len(deleter.deleted) != 0 {
		t.Fatalf("deleted %v despite incomplete snapshot", deleter.deleted)
	}
}

// TestPruneRetainIDsIncludeLegacyAndNew verifies PRUNE retains both ID schemes.
func TestPruneRetainIDsIncludeLegacyAndNew(t *testing.T) {
	retain := service.RetainDocumentIDs("kb-1", "conn-1", []string{"source-1"})
	legacy := service.Hash128("conn-1:source-1")
	next := service.Hash128("kb-1:conn-1:source-1")
	if _, ok := retain[legacy]; !ok {
		t.Fatalf("legacy ID missing from retain set")
	}
	if _, ok := retain[next]; !ok {
		t.Fatalf("new ID missing from retain set")
	}
}

// TestMockSessionEOF documents the mock session EOF contract.
func TestMockSessionEOF(t *testing.T) {
	session, _ := (&connectormock.Connector{}).OpenSync(t.Context(), syncerconnector.SyncRequest{})
	_, err := session.NextBatch(t.Context())
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF", err)
	}
}
