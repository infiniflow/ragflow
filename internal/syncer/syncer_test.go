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
	"fmt"
	"io"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	connectormock "ragflow/internal/syncer/connector/mock"
	"strings"
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
	onUpsert       func(input service.DocumentUpsertInput)
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
	if s.onUpsert != nil {
		s.onUpsert(input)
	}
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

// ListFingerprintsBySourceType returns configured fingerprints.
func (s fakeStore) ListFingerprintsBySourceType(ctx context.Context, kbID, sourceType string) (map[string]string, error) {
	if s.fingerprints == nil {
		return map[string]string{}, nil
	}
	return s.fingerprints, nil
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

type fakeTaskHandle struct {
	mu         sync.Mutex
	msg        common.TaskMessage
	acks       int
	nacks      int
	inProgress int
}

func (h *fakeTaskHandle) GetMessage() common.TaskMessage { return h.msg }
func (h *fakeTaskHandle) Ack() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.acks++
	return nil
}
func (h *fakeTaskHandle) Nack() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nacks++
	return nil
}
func (h *fakeTaskHandle) InProgress() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.inProgress++
	return nil
}

func (h *fakeTaskHandle) counts() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.acks, h.nacks
}

type fakeSyncTaskBroker struct {
	mu        sync.Mutex
	published []string
	handler   func(common.TaskHandle)
	onPublish func(taskID string)
}

func (b *fakeSyncTaskBroker) InitSyncerStream() error   { return nil }
func (b *fakeSyncTaskBroker) InitSyncerConsumer() error { return nil }
func (b *fakeSyncTaskBroker) PublishSyncerTask(taskID string) error {
	b.mu.Lock()
	b.published = append(b.published, taskID)
	b.mu.Unlock()
	if b.onPublish != nil {
		b.onPublish(taskID)
	}
	return nil
}
func (b *fakeSyncTaskBroker) PublishSyncerTaskWakeup(taskID string) error {
	return b.PublishSyncerTask(taskID)
}
func (b *fakeSyncTaskBroker) SubscribeSyncerTasks(ctx context.Context, handler func(common.TaskHandle)) error {
	b.handler = handler
	return nil
}
func (b *fakeSyncTaskBroker) publishedIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.published...)
}

type closingLocker struct {
	db *gorm.DB
}

func (l closingLocker) TryLock(connectorID, kbID string) (ConnectorLockLease, bool) {
	sqlDB, _ := l.db.DB()
	_ = sqlDB.Close()
	return ConnectorLockLease{}, false
}

func (l closingLocker) Unlock(connectorID, kbID string) {}

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
			TimeoutSecs: 60,
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

func insertKnowledgebaseMapping(t *testing.T, db *gorm.DB, connectorID, kbID string) {
	t.Helper()
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
}

func insertSyncLog(t *testing.T, db *gorm.DB, connectorID, kbID, taskID, taskType string) {
	t.Helper()
	now := time.Now().Add(-time.Hour).Truncate(time.Second)
	ts := now.UnixMilli()
	fromBeginning := "0"
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
	registry.Register("mock", func(ctx context.Context, taskContext dao.SyncTaskContext) (syncerconnector.Connector, error) {
		return connectors[taskContext.Connector.ID], nil
	})
	return registry
}

// newCoordinator creates a test coordinator.
func newCoordinator(taskDAO *dao.SyncTaskDAO, taskService *service.SyncTaskService, registry *syncerconnector.Registry, sink service.DocumentSink, pruneService *service.SyncPruneService, store service.DocumentStore) *TaskCoordinator {
	return newCoordinatorWithCheckpoints(taskDAO, taskService, registry, sink, pruneService, store, newMemorySyncCheckpointStore())
}

func newCoordinatorWithCheckpoints(taskDAO *dao.SyncTaskDAO, taskService *service.SyncTaskService, registry *syncerconnector.Registry, sink service.DocumentSink, pruneService *service.SyncPruneService, store service.DocumentStore, checkpoints SyncCheckpointStore) *TaskCoordinator {
	return newCoordinatorWithConfigAndCheckpoints(taskDAO, taskService, registry, sink, pruneService, store, checkpoints, SyncRunnerConfig{ItemRetryCount: 1, ItemRetryBaseDelay: time.Millisecond})
}

func newCoordinatorWithConfigAndCheckpoints(taskDAO *dao.SyncTaskDAO, taskService *service.SyncTaskService, registry *syncerconnector.Registry, sink service.DocumentSink, pruneService *service.SyncPruneService, store service.DocumentStore, checkpoints SyncCheckpointStore, syncRunnerConfig SyncRunnerConfig) *TaskCoordinator {
	executor := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 16})
	return NewTaskCoordinator(syncRunnerConfig, taskDAO, taskService, registry, sink, pruneService, service.NewDocumentIDResolver(store), executor, checkpoints)
}

// TestClaimBlocksSameConnectorKBRunningTasks verifies DB-backed task mutual exclusion.
func TestClaimBlocksSameConnectorKBRunningTasks(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	insertSyncLog(t, db, "conn-1", "kb-1", "task-2", dao.TaskTypePrune)
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	claimed, err := taskService.Claim(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("claim task-1: %v", err)
	}
	if !claimed {
		t.Fatalf("task-1 not claimed")
	}
	claimed, err = taskService.Claim(t.Context(), "task-2")
	if err != nil {
		t.Fatalf("claim task-2: %v", err)
	}
	if claimed {
		t.Fatalf("task-2 claimed while task-1 is running")
	}
}

// TestClaimAllowsSameConnectorDifferentKB verifies connector work can run for different KBs.
func TestClaimAllowsSameConnectorDifferentKB(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	insertKnowledgebaseMapping(t, db, "conn-1", "kb-2")
	insertSyncLog(t, db, "conn-1", "kb-2", "task-2", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	for _, taskID := range []string{"task-1", "task-2"} {
		claimed, err := taskService.Claim(t.Context(), taskID)
		if err != nil {
			t.Fatalf("claim %s: %v", taskID, err)
		}
		if !claimed {
			t.Fatalf("%s not claimed", taskID)
		}
	}
}

// TestSchedulerRequiresBroker verifies JetStream is mandatory for the scheduler.
func TestSchedulerRequiresBroker(t *testing.T) {
	scheduler := NewScheduler(make(chan TaskEnvelope, 1), dao.NewSyncTaskDAO(nil))
	err := scheduler.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "NATS broker") {
		t.Fatalf("Run error = %v, want missing broker", err)
	}
}

// TestNATSSchedulerPublishesDueTasksWithoutClaiming verifies NATS mode leaves MySQL claim to workers.
func TestNATSSchedulerPublishesDueTasksWithoutClaiming(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	broker := &fakeSyncTaskBroker{}
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)
	if err := scheduler.publishStartupTasks(t.Context()); err != nil {
		t.Fatalf("publish startup tasks: %v", err)
	}
	if len(broker.published) != 1 || broker.published[0] != "task-1" {
		t.Fatalf("published = %v", broker.published)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusSchedule {
		t.Fatalf("status = %s, want schedule", task.Status)
	}
}

// TestNATSSchedulerStartupPaginatesScheduledTasks verifies startup reconciliation publishes every scheduled task.
func TestNATSSchedulerStartupPaginatesScheduledTasks(t *testing.T) {
	db := setupSyncerDB(t)
	total := scheduledTaskStartupPageSize + 1
	for i := 0; i < total; i++ {
		taskID := fmt.Sprintf("task-%04d", i)
		insertTaskContext(t, db, fmt.Sprintf("conn-%04d", i), fmt.Sprintf("kb-%04d", i), taskID, dao.TaskTypeSync)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	broker := &fakeSyncTaskBroker{onPublish: func(taskID string) {
		claimed, err := taskDAO.ClaimTask(t.Context(), taskID, time.Now())
		if err != nil {
			t.Errorf("claim published task %s: %v", taskID, err)
			return
		}
		if !claimed {
			t.Errorf("published task %s was not claimable", taskID)
		}
	}}
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)

	if err := scheduler.publishStartupTasks(t.Context()); err != nil {
		t.Fatalf("publish startup tasks: %v", err)
	}
	got := broker.publishedIDs()
	if len(got) != total {
		t.Fatalf("published tasks = %d, want %d", len(got), total)
	}
	seen := make(map[string]struct{}, len(got))
	for _, taskID := range got {
		seen[taskID] = struct{}{}
	}
	if len(seen) != total {
		t.Fatalf("published unique tasks = %d, want %d", len(seen), total)
	}
}

func TestListScheduledTasksPaginatesNullUpdateTime(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-0", dao.TaskTypeSync)
	insertSyncLog(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	insertSyncLog(t, db, "conn-1", "kb-1", "task-2", dao.TaskTypeSync)
	if err := db.Model(&entity.SyncLogs{}).
		Where("id IN ?", []string{"task-0", "task-1", "task-2"}).
		UpdateColumns(map[string]any{"update_time": gorm.Expr("NULL"), "update_date": gorm.Expr("NULL")}).Error; err != nil {
		t.Fatalf("clear update timestamps: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	firstPage, err := taskDAO.ListScheduledTasks(t.Context(), 2, nil)
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if len(firstPage) != 2 {
		t.Fatalf("first page length = %d, want 2", len(firstPage))
	}
	cursor := firstPage[len(firstPage)-1].Cursor()
	secondPage, err := taskDAO.ListScheduledTasks(t.Context(), 2, &cursor)
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}

	seen := map[string]struct{}{}
	for _, task := range append(firstPage, secondPage...) {
		seen[task.ID] = struct{}{}
	}
	if len(seen) != 3 {
		t.Fatalf("listed unique tasks = %d, want 3", len(seen))
	}
	for _, taskID := range []string{"task-0", "task-1", "task-2"} {
		if _, ok := seen[taskID]; !ok {
			t.Fatalf("task %s was not listed", taskID)
		}
	}
}

// TestNATSSchedulerStartupDelaysFreshScheduledTask verifies startup keeps refresh windows without periodic DB scans.
func TestNATSSchedulerStartupDelaysFreshScheduledTask(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	now := time.Now()
	updateTime := now.UnixMilli()
	if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("refresh_freq", 5).Error; err != nil {
		t.Fatalf("set refresh freq: %v", err)
	}
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Updates(map[string]any{
		"update_date": now,
		"update_time": updateTime,
	}).Error; err != nil {
		t.Fatalf("set task update time: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	broker := &fakeSyncTaskBroker{}
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)
	if err := scheduler.publishStartupTasks(t.Context()); err != nil {
		t.Fatalf("publish startup tasks: %v", err)
	}
	if got := broker.publishedIDs(); len(got) != 0 {
		t.Fatalf("published = %v, want delayed task", got)
	}
	scheduler.timerMu.Lock()
	_, scheduled := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if !scheduled {
		t.Fatalf("task timer was not scheduled")
	}
}

// TestNATSSchedulerRecoversRunningTasksOnStartup verifies startup reconciliation publishes interrupted tasks.
func TestNATSSchedulerRecoversRunningTasksOnStartup(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatalf("mark task running: %v", err)
	}
	if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatalf("mark connector running: %v", err)
	}
	taskDAO := dao.NewSyncTaskDAO(db)
	broker := &fakeSyncTaskBroker{}
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)

	if err := scheduler.publishStartupTasks(t.Context()); err != nil {
		t.Fatalf("publish startup tasks: %v", err)
	}
	if got := broker.publishedIDs(); len(got) != 1 || got[0] != "task-1" {
		t.Fatalf("published = %v", got)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusSchedule {
		t.Fatalf("task status = %s, want schedule", task.Status)
	}
	var connector entity.Connector
	if err := db.First(&connector, "id = ?", "conn-1").Error; err != nil {
		t.Fatalf("load connector: %v", err)
	}
	if connector.Status != dao.SyncStatusSchedule {
		t.Fatalf("connector status = %s, want schedule", connector.Status)
	}
}

// TestNATSSchedulerStartupPublishesDueTasks verifies startup scan immediately publishes due work.
func TestNATSSchedulerStartupPublishesDueTasks(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	now := time.Now()
	updateTime := now.UnixMilli()
	if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("refresh_freq", 5).Error; err != nil {
		t.Fatalf("set refresh freq: %v", err)
	}
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Updates(map[string]any{
		"update_date": now,
		"update_time": updateTime,
	}).Error; err != nil {
		t.Fatalf("set fresh task update time: %v", err)
	}
	taskDAO := dao.NewSyncTaskDAO(db)
	broker := &fakeSyncTaskBroker{}
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, broker)

	if err := scheduler.publishStartupTasks(t.Context()); err != nil {
		t.Fatalf("publish fresh startup tasks: %v", err)
	}
	if got := broker.publishedIDs(); len(got) != 0 {
		t.Fatalf("fresh task published = %v, want none", got)
	}

	dueAt := now.Add(-10 * time.Minute)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Updates(map[string]any{
		"update_date": dueAt,
		"update_time": dueAt.UnixMilli(),
	}).Error; err != nil {
		t.Fatalf("set due task update time: %v", err)
	}

	if err := scheduler.publishStartupTasks(t.Context()); err != nil {
		t.Fatalf("publish due startup tasks: %v", err)
	}
	if got := broker.publishedIDs(); len(got) != 1 || got[0] != "task-1" {
		t.Fatalf("published = %v, want due task", got)
	}
}

// TestNATSSchedulerBuffersPushedTasks verifies enqueueHandle buffers pushed NATS handles in the local queue.
func TestNATSSchedulerBuffersPushedTasks(t *testing.T) {
	taskDAO := dao.NewSyncTaskDAO(nil)
	queue := make(chan TaskEnvelope, 2)
	scheduler := NewNATSScheduler(queue, taskDAO, &fakeSyncTaskBroker{})
	handles := []common.TaskHandle{
		&fakeTaskHandle{msg: common.TaskMessage{TaskID: "task-1", TaskType: common.TaskTypeSyncer}},
		&fakeTaskHandle{msg: common.TaskMessage{TaskID: "task-2", TaskType: common.TaskTypeSyncer}},
	}

	for _, handle := range handles {
		if err := scheduler.enqueueHandle(t.Context(), handle); err != nil {
			t.Fatalf("enqueue handle: %v", err)
		}
	}
	if got := scheduler.queueAvailable(); got != 0 {
		t.Fatalf("queue capacity while buffered = %d, want 0", got)
	}

	first := <-queue
	stopEnvelopeHeartbeat(first)
	if got := scheduler.queueAvailable(); got != 1 {
		t.Fatalf("queue capacity after dequeue = %d, want 1", got)
	}
	second := <-queue
	stopEnvelopeHeartbeat(second)
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
	worker := NewTaskWorker(queue, taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(connectors), sink, nil, fakeStore{}), NewConnectorLock())
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

// TestNATSTaskWorkerClaimsAndAcksOnSuccess verifies NATS messages are ACKed after durable completion.
func TestNATSTaskWorkerClaimsAndAcksOnSuccess(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	now := time.Now()
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "a", UpdatedAt: now}}}}}
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "task-1", TaskType: common.TaskTypeSyncer}}
	worker := NewTaskWorker(
		make(chan TaskEnvelope, 1),
		taskDAO,
		taskService,
		newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{}),
		NewConnectorLock(),
	)
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1", Handle: handle})

	if handle.acks != 1 || handle.nacks != 0 {
		t.Fatalf("settlement acks=%d nacks=%d", handle.acks, handle.nacks)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusDone {
		t.Fatalf("status = %s, want done", task.Status)
	}
}

// TestNATSTaskWorkerAcksUnclaimableMessage verifies duplicate/stale messages do not redeliver forever.
func TestNATSTaskWorkerAcksUnclaimableMessage(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "task-1", TaskType: common.TaskTypeSyncer}}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(nil), &fakeSink{}, nil, fakeStore{}), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1", Handle: handle})
	if handle.acks != 1 || handle.nacks != 0 {
		t.Fatalf("settlement acks=%d nacks=%d", handle.acks, handle.nacks)
	}
}

func TestNATSTaskWorkerRetriesStillScheduledUnclaimedTask(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	insertSyncLog(t, db, "conn-1", "kb-1", "task-2", dao.TaskTypePrune)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatalf("mark sync running: %v", err)
	}
	if err := db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatalf("mark connector running: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, &fakeSyncTaskBroker{})
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(nil), &fakeSink{}, nil, fakeStore{}), NewConnectorLock()).WithScheduler(scheduler)
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "task-2", TaskType: common.TaskTypeSyncer}}

	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-2", Handle: handle})

	if handle.acks != 1 || handle.nacks != 0 {
		t.Fatalf("settlement acks=%d nacks=%d", handle.acks, handle.nacks)
	}
	scheduler.timerMu.Lock()
	timer := scheduler.timers["task-2"]
	if timer != nil {
		timer.Stop()
		delete(scheduler.timers, "task-2")
	}
	scheduler.timerMu.Unlock()
	if timer == nil {
		t.Fatalf("scheduled prune retry timer was not registered")
	}
}

func TestNATSTaskWorkerNacksWhenLockContentionRescheduleFails(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "task-1", TaskType: common.TaskTypeSyncer}}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, nil, closingLocker{db: db})

	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1", Handle: handle})

	if handle.acks != 0 || handle.nacks != 1 {
		t.Fatalf("settlement acks=%d nacks=%d, want nack without ack", handle.acks, handle.nacks)
	}
}

func TestNATSTaskWorkerDoesNotRetryCompletedUnclaimedTask(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusDone).Error; err != nil {
		t.Fatalf("mark done: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	scheduler := NewNATSScheduler(make(chan TaskEnvelope, 1), taskDAO, &fakeSyncTaskBroker{})
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(nil), &fakeSink{}, nil, fakeStore{}), NewConnectorLock()).WithScheduler(scheduler)
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "task-1", TaskType: common.TaskTypeSyncer}}

	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1", Handle: handle})

	if handle.acks != 1 || handle.nacks != 0 {
		t.Fatalf("settlement acks=%d nacks=%d", handle.acks, handle.nacks)
	}
	scheduler.timerMu.Lock()
	_, scheduled := scheduler.timers["task-1"]
	scheduler.timerMu.Unlock()
	if scheduled {
		t.Fatalf("completed task should not be scheduled for retry")
	}
}

// TestSameConnectorDifferentKBsRunInParallel verifies one datasource can sync into different KBs concurrently.
func TestSameConnectorDifferentKBsRunInParallel(t *testing.T) {
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
	worker := NewTaskWorker(queue, taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}), NewConnectorLock())
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
		t.Fatalf("max concurrent same-connector different-kb sink calls = %d, want >= 2", maxConcurrent)
	}
}

// TestConnectorKBLockSerializesSyncAndPrune verifies sync and prune for the same connector/KB do not overlap.
func TestConnectorKBLockSerializesSyncAndPrune(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	now := time.Now().Add(-time.Hour).Truncate(time.Second)
	ts := now.UnixMilli()
	fromBeginning := "0"
	if err := db.Create(&entity.SyncLogs{
		ID:            "task-2",
		ConnectorID:   "conn-1",
		KbID:          "kb-1",
		TaskType:      dao.TaskTypePrune,
		Status:        dao.SyncStatusSchedule,
		FromBeginning: &fromBeginning,
		TimeStarted:   &now,
		ErrorMsg:      "",
		BaseModel:     entity.BaseModel{UpdateDate: &now, UpdateTime: &ts},
	}).Error; err != nil {
		t.Fatalf("insert prune task: %v", err)
	}
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	for _, id := range []string{"task-1", "task-2"} {
		if _, err := taskDAO.ClaimTask(t.Context(), id, time.Now()); err != nil {
			t.Fatalf("claim %s: %v", id, err)
		}
	}
	sink := &fakeSink{delay: 120 * time.Millisecond}
	connector := &connectormock.Connector{
		SyncBatches:  []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "a", UpdatedAt: time.Now()}}}},
		PruneBatches: []syncerconnector.PruneBatch{{Documents: []syncerconnector.SlimDocument{{SourceID: "a"}}}},
	}
	pruneService := service.NewSyncPruneService(&fakeDeleter{}, fakeStore{ids: map[string]struct{}{}})
	worker := NewTaskWorker(make(chan TaskEnvelope, 2), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, pruneService, fakeStore{}), NewConnectorLock())
	done := make(chan struct{})
	go func() {
		defer close(done)
		worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})
	}()
	waitForSinkConcurrency(t, sink, 1)
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-2"})
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-2").Error; err != nil {
		t.Fatalf("load prune task: %v", err)
	}
	if task.Status != dao.SyncStatusSchedule {
		t.Fatalf("same connector/kb prune status = %s, want schedule", task.Status)
	}
	<-done
}

func waitForSinkConcurrency(t *testing.T, sink *fakeSink, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for sink concurrency >= %d", want)
		case <-ticker.C:
			sink.mu.Lock()
			current := sink.current
			sink.mu.Unlock()
			if current >= want {
				return
			}
		}
	}
}

// TestSyncRunnerSubmitsBatchesBeforeWaiting verifies source reads are not blocked by prior batch jobs.
func TestSyncRunnerSubmitsBatchesBeforeWaiting(t *testing.T) {
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
	coordinator := newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{})
	taskContext, _ := taskDAO.GetTaskContext(t.Context(), "task-1")
	if _, err := coordinator.Execute(t.Context(), taskContext, testLockLease()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if secondBatchAt.Sub(start) >= 70*time.Millisecond {
		t.Fatalf("second batch was blocked by first batch for %s", secondBatchAt.Sub(start))
	}
}

// TestSyncRunnerProcessesBatchJobsInParallel verifies source batches run as parallel BatchJobs.
func TestSyncRunnerProcessesBatchJobsInParallel(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	now := time.Now()
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{
		{Documents: []syncerconnector.SourceDocument{{SourceID: "a", UpdatedAt: now}}},
		{Documents: []syncerconnector.SourceDocument{{SourceID: "b", UpdatedAt: now}}},
		{Documents: []syncerconnector.SourceDocument{{SourceID: "c", UpdatedAt: now}}},
	}}
	sink := &fakeSink{delay: 80 * time.Millisecond}
	coordinator := newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{})
	taskContext, _ := taskDAO.GetTaskContext(t.Context(), "task-1")
	if _, err := coordinator.Execute(t.Context(), taskContext, testLockLease()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	sink.mu.Lock()
	maxConcurrent := sink.maxConcurrent
	sink.mu.Unlock()
	if maxConcurrent < 2 {
		t.Fatalf("max concurrent batches = %d, want >= 2", maxConcurrent)
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
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Updates(map[string]any{
		"status":             dao.SyncStatusRunning,
		"total_docs_indexed": int64(1),
	}).Error; err != nil {
		t.Fatalf("set running task: %v", err)
	}
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	legacyID := service.Hash128("conn-1:source-1")
	store := fakeStore{ids: map[string]struct{}{legacyID: {}}, fingerprints: map[string]string{legacyID: "fp-1"}}
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "source-1", Fingerprint: "fp-1", FetchRef: &syncerconnector.FetchReference{Key: "lazy"}, UpdatedAt: time.Now()}}}}}
	sink := &fakeSink{}
	coordinator := newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, store)
	taskContext, _ := taskDAO.GetTaskContext(t.Context(), "task-1")
	if _, err := coordinator.Execute(t.Context(), taskContext, testLockLease()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sink.callCount() != 0 {
		t.Fatalf("sink calls = %d, want 0", sink.callCount())
	}
	var completed entity.SyncLogs
	if err := db.First(&completed, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load completed task: %v", err)
	}
	if completed.NewDocsIndexed != 0 || completed.TotalDocsIndexed != 0 {
		t.Fatalf("completed stats new/total = %d/%d, want 0/0", completed.NewDocsIndexed, completed.TotalDocsIndexed)
	}
	var next entity.SyncLogs
	if err := db.Where("id <> ? AND connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", "task-1", "conn-1", "kb-1", dao.TaskTypeSync, dao.SyncStatusSchedule).First(&next).Error; err != nil {
		t.Fatalf("load next scheduled task: %v", err)
	}
	if next.TotalDocsIndexed != 1 {
		t.Fatalf("next scheduled total docs indexed = %d, want 1", next.TotalDocsIndexed)
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
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "source-1", Blob: []byte("x"), UpdatedAt: time.Now()}}}}}
	sink := &fakeSink{}
	coordinator := newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{})
	taskContext, _ := taskDAO.GetTaskContext(t.Context(), "task-1")
	if _, err := coordinator.Execute(t.Context(), taskContext, testLockLease()); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if sink.autoParseByDoc["source-1"] {
		t.Fatalf("auto_parse flowed as true, want false")
	}
}

// TestCompleteSyncSchedulesNextRun verifies completing one sync keeps the connector schedulable.
func TestCompleteSyncSchedulesNextRun(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	_ = db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	now := time.Now()
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "source-1", Blob: []byte("x"), UpdatedAt: now}}}}}
	coordinator := newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{})
	taskContext, _ := taskDAO.GetTaskContext(t.Context(), "task-1")
	if _, err := coordinator.Execute(t.Context(), taskContext, testLockLease()); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var connectorRow entity.Connector
	if err := db.First(&connectorRow, "id = ?", "conn-1").Error; err != nil {
		t.Fatalf("load connector: %v", err)
	}
	if connectorRow.Status != dao.SyncStatusSchedule {
		t.Fatalf("connector status = %s, want schedule", connectorRow.Status)
	}
	var scheduled int64
	if err := db.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", "conn-1", "kb-1", dao.TaskTypeSync, dao.SyncStatusSchedule).
		Count(&scheduled).Error; err != nil {
		t.Fatalf("count scheduled sync logs: %v", err)
	}
	if scheduled != 1 {
		t.Fatalf("scheduled sync logs = %d, want 1", scheduled)
	}
}

// TestCancelStopsRunningSync verifies a stop request prevents further work and completion.
func TestCancelStopsRunningSync(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	_ = db.Model(&entity.Connector{}).Where("id = ?", "conn-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	now := time.Now()
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{
		{SourceID: "source-1", Blob: []byte("one"), UpdatedAt: now},
		{SourceID: "source-2", Blob: []byte("two"), UpdatedAt: now},
	}}}}
	var cancelOnce sync.Once
	sink := &fakeSink{
		onUpsert: func(input service.DocumentUpsertInput) {
			cancelOnce.Do(func() {
				if err := db.Model(&entity.SyncLogs{}).Where("id = ?", input.TaskContext.Task.ID).Update("status", dao.SyncStatusCancel).Error; err != nil {
					t.Errorf("cancel sync log: %v", err)
				}
				if err := db.Model(&entity.Connector{}).Where("id = ?", input.TaskContext.Connector.ID).Update("status", dao.SyncStatusCancel).Error; err != nil {
					t.Errorf("cancel connector: %v", err)
				}
				time.Sleep(syncCancelCheckInterval)
			})
		},
	}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	if calls := sink.callCount(); calls != 1 {
		t.Fatalf("sink calls = %d, want 1", calls)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusCancel {
		t.Fatalf("task status = %s, want cancel", task.Status)
	}
	var scheduled int64
	if err := db.Model(&entity.SyncLogs{}).
		Where("connector_id = ? AND kb_id = ? AND task_type = ? AND status = ?", "conn-1", "kb-1", dao.TaskTypeSync, dao.SyncStatusSchedule).
		Count(&scheduled).Error; err != nil {
		t.Fatalf("count scheduled sync logs: %v", err)
	}
	if scheduled != 0 {
		t.Fatalf("scheduled sync logs = %d, want 0", scheduled)
	}
	var connectorRow entity.Connector
	if err := db.First(&connectorRow, "id = ?", "conn-1").Error; err != nil {
		t.Fatalf("load connector: %v", err)
	}
	if connectorRow.Status != dao.SyncStatusCancel {
		t.Fatalf("connector status = %s, want cancel", connectorRow.Status)
	}
}

// TestSyncRunnerResultWaitHonorsCancel verifies Run does not block forever waiting for job results after cancellation.
func TestSyncRunnerResultWaitHonorsCancel(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	taskContext, err := taskDAO.GetTaskContext(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "source-1", Blob: []byte("x"), UpdatedAt: time.Now()}}}}}
	queue := &SyncJobQueue{taskID: "task-1", jobs: make(chan *syncJob, 1)}
	runner := NewSyncRunner(SyncRunnerConfig{ItemRetryCount: 1, ItemRetryBaseDelay: time.Millisecond}, taskDAO, taskService, &fakeSink{}, service.NewDocumentIDResolver(fakeStore{}), queue, newMemorySyncCheckpointStore())

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := runner.Run(ctx, taskContext, connector)
		result <- err
	}()

	select {
	case <-queue.jobs:
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for submitted job")
	}
	cancel()

	select {
	case err = <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("Run did not return after cancellation")
	}
}

// TestBatchFailureDoesNotAdvanceWaterline verifies failed tasks keep poll_range_end.
func TestBatchFailureDoesNotAdvanceWaterline(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{Documents: []syncerconnector.SourceDocument{{SourceID: "bad", Blob: []byte("x"), UpdatedAt: time.Now()}}}}}
	sink := &fakeSink{errBySourceID: map[string]error{"bad": errors.New("boom")}}
	queue := make(chan TaskEnvelope, 1)
	worker := NewTaskWorker(queue, taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}), NewConnectorLock())
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

// TestTransientFailureReschedulesTask verifies retryable source failures keep the same task scheduled.
func TestTransientFailureReschedulesTask(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	connector := &connectormock.Connector{SyncErrAt: 1}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{}), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusSchedule {
		t.Fatalf("status = %s, want schedule", task.Status)
	}
	if task.ErrorCount != 1 {
		t.Fatalf("error_count = %d, want 1", task.ErrorCount)
	}
	if !strings.Contains(task.ErrorMsg, "unexpected EOF") {
		t.Fatalf("error_msg = %q, want unexpected EOF", task.ErrorMsg)
	}
}

// TestTransientFailureFailsAfterThreeRetries verifies retryable errors become terminal after the retry budget.
func TestTransientFailureFailsAfterThreeRetries(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	if err := db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Updates(map[string]any{"status": dao.SyncStatusRunning, "error_count": int64(2)}).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	connector := &connectormock.Connector{SyncErrAt: 1}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{}), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusFail {
		t.Fatalf("status = %s, want fail", task.Status)
	}
	if task.ErrorCount != 3 {
		t.Fatalf("error_count = %d, want 3", task.ErrorCount)
	}
	if !strings.Contains(task.ErrorMsg, "failed after 3 transient retries") || !strings.Contains(task.ErrorMsg, "unexpected EOF") {
		t.Fatalf("error_msg = %q", task.ErrorMsg)
	}
}

// TestTransientFetchFailureSavesCompletedBatchCheckpoint verifies source failures do not discard completed batch progress.
func TestTransientFetchFailureSavesCompletedBatchCheckpoint(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	checkpoints := newMemorySyncCheckpointStore()
	firstTime := time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC)
	connector := &connectormock.Connector{
		SyncErrAt: 2,
		SyncBatches: []syncerconnector.SyncBatch{{
			Documents:  []syncerconnector.SourceDocument{{SourceID: "source-1", Blob: []byte("a"), UpdatedAt: firstTime}},
			Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", UpdatedAt: &firstTime, SourceID: "source-1"},
		}},
	}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinatorWithCheckpoints(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{}, checkpoints), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	state, err := checkpoints.LoadSyncCheckpoint(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if state == nil || state.Checkpoint == nil {
		t.Fatalf("checkpoint was not saved")
	}
	if state.Checkpoint.Cursor != "cursor-1" || state.Checkpoint.SourceID != "source-1" {
		t.Fatalf("checkpoint = %+v, want cursor-1/source-1", state.Checkpoint)
	}
	if state.Added != 1 {
		t.Fatalf("checkpoint added = %d, want 1", state.Added)
	}

	var task entity.SyncLogs
	if err = db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusSchedule {
		t.Fatalf("status = %s, want schedule", task.Status)
	}
}

// TestSyncCheckpointDeletedAfterSuccess verifies completed tasks remove their checkpoint.
func TestSyncCheckpointDeletedAfterSuccess(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	checkpoints := newMemorySyncCheckpointStore()

	firstTime := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	secondTime := time.Date(2026, 8, 11, 1, 1, 0, 0, time.UTC)
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{
		{
			Documents:  []syncerconnector.SourceDocument{{SourceID: "source-1", Blob: []byte("a"), UpdatedAt: firstTime}},
			Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", UpdatedAt: &firstTime, SourceID: "source-1"},
		},
		{
			Documents:  []syncerconnector.SourceDocument{{SourceID: "source-2", Blob: []byte("b"), UpdatedAt: secondTime}},
			Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-2", UpdatedAt: &secondTime, SourceID: "source-2"},
		},
	}}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinatorWithCheckpoints(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{}, checkpoints), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	state, err := checkpoints.LoadSyncCheckpoint(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if state != nil {
		t.Fatalf("checkpoint = %+v, want deleted", state)
	}
}

// TestSyncCheckpointStopsBeforeFailedBatch verifies failed batches do not advance the resume point.
func TestSyncCheckpointStopsBeforeFailedBatch(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	checkpoints := newMemorySyncCheckpointStore()

	firstTime := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	secondTime := time.Date(2026, 8, 11, 1, 1, 0, 0, time.UTC)
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{
		{
			Documents:  []syncerconnector.SourceDocument{{SourceID: "good", Blob: []byte("a"), UpdatedAt: firstTime}},
			Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", UpdatedAt: &firstTime, SourceID: "good"},
		},
		{
			Documents:  []syncerconnector.SourceDocument{{SourceID: "bad", Blob: []byte("b"), UpdatedAt: secondTime}},
			Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-2", UpdatedAt: &secondTime, SourceID: "bad"},
		},
	}}
	sink := &fakeSink{errBySourceID: map[string]error{"bad": errors.New("boom")}}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinatorWithCheckpoints(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, checkpoints), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	state, err := checkpoints.LoadSyncCheckpoint(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if state == nil || state.Checkpoint == nil {
		t.Fatalf("checkpoint was not saved")
	}
	if state.Checkpoint.Cursor != "cursor-1" {
		t.Fatalf("checkpoint cursor = %q, want cursor-1", state.Checkpoint.Cursor)
	}
	if state.NextCommitSeq != 2 {
		t.Fatalf("next commit seq = %d, want 2", state.NextCommitSeq)
	}
	if state.Added != 1 || state.Updated != 0 {
		t.Fatalf("checkpoint stats added/updated = %d/%d, want 1/0", state.Added, state.Updated)
	}

	if err = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}
	thirdTime := secondTime.Add(time.Minute)
	resumeConnector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{
		Documents:  []syncerconnector.SourceDocument{{SourceID: "second-good", Blob: []byte("c"), UpdatedAt: thirdTime}},
		Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-3", UpdatedAt: &thirdTime, SourceID: "second-good"},
	}}}
	resumeWorker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinatorWithCheckpoints(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": resumeConnector}), &fakeSink{}, nil, fakeStore{}, checkpoints), NewConnectorLock())
	resumeWorker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})
	if len(resumeConnector.SyncRequests) != 1 {
		t.Fatalf("sync requests = %d, want 1", len(resumeConnector.SyncRequests))
	}
	request := resumeConnector.SyncRequests[0]
	if request.Resume == nil || request.Resume.Cursor != "cursor-1" || request.Resume.SourceID != "good" {
		t.Fatalf("resume = %+v, want cursor-1/good", request.Resume)
	}
	if !request.WindowEnd.Equal(state.WindowEnd) {
		t.Fatalf("window end = %s, want %s", request.WindowEnd, state.WindowEnd)
	}
	var task entity.SyncLogs
	if err = db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load completed task: %v", err)
	}
	if task.Status != dao.SyncStatusDone {
		t.Fatalf("status = %s, want done", task.Status)
	}
	if task.TotalDocsIndexed != 2 {
		t.Fatalf("total docs indexed = %d, want 2", task.TotalDocsIndexed)
	}
}

// TestSyncRunnerRestartsAfterInvalidOpenSyncResume verifies a stale resume anchor restarts
// the same task window with a cleared checkpoint.
func TestSyncRunnerRestartsAfterInvalidOpenSyncResume(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	checkpoints := newMemorySyncCheckpointStore()
	windowStart := time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	updatedAt := windowStart.Add(30 * time.Minute)
	initialCheckpoint := &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", SourceID: "source-1", UpdatedAt: &updatedAt}
	if err := checkpoints.SaveSyncCheckpoint(t.Context(), "task-1", syncerconnector.SyncCheckpointState{
		Version:       1,
		TaskID:        "task-1",
		ConnectorID:   "conn-1",
		KBID:          "kb-1",
		WindowStart:   &windowStart,
		WindowEnd:     windowEnd,
		NextCommitSeq: 2,
		Checkpoint:    initialCheckpoint,
		Added:         1,
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	source := syncerconnector.SourceDocument{SourceID: "source-2", Blob: []byte("b"), UpdatedAt: windowEnd}
	connector := &connectormock.Connector{
		OpenSyncResumeInvalidAt: 1,
		SyncBatches: []syncerconnector.SyncBatch{{
			Documents:  []syncerconnector.SourceDocument{source},
			Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-2", SourceID: "source-2", UpdatedAt: &windowEnd},
		}},
	}
	sink := &fakeSink{}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinatorWithConfigAndCheckpoints(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, checkpoints, SyncRunnerConfig{
		ItemRetryCount:        1,
		ItemRetryBaseDelay:    time.Millisecond,
		MaxAnchorRestartCount: 2,
	}), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	if len(connector.SyncRequests) != 2 {
		t.Fatalf("sync requests = %d, want 2", len(connector.SyncRequests))
	}
	initialRequest := connector.SyncRequests[0]
	if initialRequest.Resume == nil || initialRequest.Resume.Cursor != "cursor-1" || initialRequest.Resume.SourceID != "source-1" {
		t.Fatalf("initial resume = %+v, want cursor-1/source-1", initialRequest.Resume)
	}
	restartRequest := connector.SyncRequests[1]
	if restartRequest.Resume != nil {
		t.Fatalf("restart resume = %+v, want nil", restartRequest.Resume)
	}
	if initialRequest.TaskID != "task-1" || initialRequest.ConnectorID != "conn-1" || initialRequest.KBID != "kb-1" {
		t.Fatalf("initial request identity = %+v", initialRequest)
	}
	if restartRequest.TaskID != initialRequest.TaskID || restartRequest.ConnectorID != initialRequest.ConnectorID || restartRequest.KBID != initialRequest.KBID {
		t.Fatalf("restart request identity = %+v, want %+v", restartRequest, initialRequest)
	}
	if restartRequest.WindowStart == nil || !restartRequest.WindowStart.Equal(*initialRequest.WindowStart) || !restartRequest.WindowEnd.Equal(initialRequest.WindowEnd) {
		t.Fatalf("restart window = %+v, want same as %+v", restartRequest.WindowStart, initialRequest.WindowStart)
	}
	if sink.callCount() != 1 {
		t.Fatalf("sink calls = %d, want 1", sink.callCount())
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusDone {
		t.Fatalf("status = %s, want done", task.Status)
	}
	if task.TotalDocsIndexed != 1 {
		t.Fatalf("total docs indexed = %d, want 1", task.TotalDocsIndexed)
	}
}

// TestSyncRunnerRestartsAfterInvalidNextBatchResume verifies pending work is collected
// before the same source window is reprocessed from the beginning.
func TestSyncRunnerRestartsAfterInvalidNextBatchResume(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	checkpoints := newMemorySyncCheckpointStore()
	windowStart := time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	if err := checkpoints.SaveSyncCheckpoint(t.Context(), "task-1", syncerconnector.SyncCheckpointState{
		Version:       1,
		TaskID:        "task-1",
		ConnectorID:   "conn-1",
		KBID:          "kb-1",
		WindowStart:   &windowStart,
		WindowEnd:     windowEnd,
		NextCommitSeq: 1,
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	firstTime := windowStart.Add(10 * time.Minute)
	secondTime := windowStart.Add(20 * time.Minute)
	connector := &connectormock.Connector{
		NextBatchResumeInvalidAt:    2,
		NextBatchResumeFirstSession: true,
		SyncBatches: []syncerconnector.SyncBatch{
			{
				Documents:  []syncerconnector.SourceDocument{{SourceID: "source-1", Blob: []byte("a"), UpdatedAt: firstTime}},
				Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", SourceID: "source-1", UpdatedAt: &firstTime},
			},
			{
				Documents:  []syncerconnector.SourceDocument{{SourceID: "source-2", Blob: []byte("b"), UpdatedAt: secondTime}},
				Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-2", SourceID: "source-2", UpdatedAt: &secondTime},
			},
		},
	}
	sink := &fakeSink{}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinatorWithConfigAndCheckpoints(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), sink, nil, fakeStore{}, checkpoints, SyncRunnerConfig{
		ItemRetryCount:        1,
		ItemRetryBaseDelay:    time.Millisecond,
		MaxAnchorRestartCount: 2,
	}), NewConnectorLock())
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})

	if len(connector.SyncRequests) != 2 {
		t.Fatalf("sync requests = %d, want 2", len(connector.SyncRequests))
	}
	for _, request := range connector.SyncRequests {
		if request.Resume != nil {
			t.Fatalf("resume = %+v, want nil for full-window restart", request.Resume)
		}
		if request.WindowStart == nil || !request.WindowStart.Equal(windowStart) || !request.WindowEnd.Equal(windowEnd) {
			t.Fatalf("window = %+v/%s, want %s/%s", request.WindowStart, request.WindowEnd, windowStart, windowEnd)
		}
	}
	if sink.callCount() != 3 {
		t.Fatalf("sink calls = %d, want first-pass document plus 2 reprocessed documents", sink.callCount())
	}
	state, err := checkpoints.LoadSyncCheckpoint(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if state != nil {
		t.Fatalf("checkpoint = %+v, want deleted after success", state)
	}
	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != dao.SyncStatusDone {
		t.Fatalf("status = %s, want done", task.Status)
	}
	if task.TotalDocsIndexed != 2 {
		t.Fatalf("total docs indexed = %d, want 2", task.TotalDocsIndexed)
	}
}

// TestSyncRunnerStopsAfterRepeatedInvalidResumeAnchors verifies the restart budget is bounded.
func TestSyncRunnerStopsAfterRepeatedInvalidResumeAnchors(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	checkpoints := newMemorySyncCheckpointStore()
	windowStart := time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	updatedAt := windowStart.Add(30 * time.Minute)
	if err := checkpoints.SaveSyncCheckpoint(t.Context(), "task-1", syncerconnector.SyncCheckpointState{
		Version:       1,
		TaskID:        "task-1",
		ConnectorID:   "conn-1",
		KBID:          "kb-1",
		WindowStart:   &windowStart,
		WindowEnd:     windowEnd,
		NextCommitSeq: 2,
		Checkpoint:    &syncerconnector.SyncCheckpoint{Cursor: "cursor-1", SourceID: "source-1", UpdatedAt: &updatedAt},
	}); err != nil {
		t.Fatalf("save checkpoint: %v", err)
	}

	taskContext, err := taskDAO.GetTaskContext(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	executor := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1})
	queue, err := executor.RegisterTask(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("register task: %v", err)
	}
	defer executor.Close()
	defer queue.Close()

	connector := &connectormock.Connector{OpenSyncResumeInvalidAlways: true}
	runner := NewSyncRunner(SyncRunnerConfig{
		ItemRetryCount:        1,
		ItemRetryBaseDelay:    time.Millisecond,
		MaxAnchorRestartCount: 1,
	}, taskDAO, taskService, &fakeSink{}, service.NewDocumentIDResolver(fakeStore{}), queue, checkpoints)
	if _, err := runner.Run(t.Context(), taskContext, connector); !errors.Is(err, syncerconnector.ErrSyncResumeInvalid) {
		t.Fatalf("Run error = %v, want ErrSyncResumeInvalid", err)
	}
	if len(connector.SyncRequests) != 2 {
		t.Fatalf("sync requests = %d, want 2", len(connector.SyncRequests))
	}
	if connector.SyncRequests[0].Resume == nil || connector.SyncRequests[1].Resume != nil {
		t.Fatalf("sync request resumes = %+v / %+v, want checkpoint then nil", connector.SyncRequests[0].Resume, connector.SyncRequests[1].Resume)
	}
	state, err := checkpoints.LoadSyncCheckpoint(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("load checkpoint: %v", err)
	}
	if state == nil || state.RestartCount != 1 || state.Checkpoint != nil {
		t.Fatalf("checkpoint = %+v, want restart_count 1 and cleared resume", state)
	}
}

// TestSyncRunnerClampsWaterlineToWindowEnd verifies future source timestamps do not poison the next window.
func TestSyncRunnerClampsWaterlineToWindowEnd(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	start := time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC)
	if err := db.Model(&entity.SyncLogs{}).
		Where("id = ?", "task-1").
		Updates(map[string]any{"status": dao.SyncStatusRunning, "poll_range_start": &start}).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	future := time.Date(2036, 6, 28, 9, 44, 0, 0, time.UTC)
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{
		Documents:  []syncerconnector.SourceDocument{{SourceID: "future", Blob: []byte("future"), UpdatedAt: future}},
		Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-future", UpdatedAt: &future, SourceID: "future"},
	}}}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{}), NewConnectorLock())
	before := time.Now().UTC()
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})
	after := time.Now().UTC().Add(time.Second)

	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.PollRangeEnd == nil {
		t.Fatalf("poll_range_end is nil")
	}
	if task.PollRangeEnd.Time().Equal(future) || task.PollRangeEnd.Time().After(after) || task.PollRangeEnd.Time().Before(before.Add(-time.Second)) {
		t.Fatalf("poll_range_end = %s, want clamped to run window around %s - %s", task.PollRangeEnd.Time(), before, after)
	}
}

// TestFullSyncWaterlineUsesWindowEnd verifies successful full sync does not keep the waterline at the latest source timestamp.
func TestFullSyncWaterlineUsesWindowEnd(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypeSync)
	fromBeginning := "1"
	if err := db.Model(&entity.SyncLogs{}).
		Where("id = ?", "task-1").
		Updates(map[string]any{"status": dao.SyncStatusRunning, "from_beginning": &fromBeginning}).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}

	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	oldSourceTime := time.Date(2025, 5, 17, 12, 39, 23, 0, time.UTC)
	connector := &connectormock.Connector{SyncBatches: []syncerconnector.SyncBatch{{
		Documents:  []syncerconnector.SourceDocument{{SourceID: "old", Blob: []byte("old"), UpdatedAt: oldSourceTime}},
		Checkpoint: &syncerconnector.SyncCheckpoint{Cursor: "cursor-old", UpdatedAt: &oldSourceTime, SourceID: "old"},
	}}}
	worker := NewTaskWorker(make(chan TaskEnvelope, 1), taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, nil, fakeStore{}), NewConnectorLock())
	before := time.Now().UTC()
	worker.handle(t.Context(), TaskEnvelope{TaskID: "task-1"})
	after := time.Now().UTC().Add(time.Second)

	var task entity.SyncLogs
	if err := db.First(&task, "id = ?", "task-1").Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.PollRangeEnd == nil {
		t.Fatalf("poll_range_end is nil")
	}
	if task.PollRangeEnd.Time().Equal(oldSourceTime) || task.PollRangeEnd.Time().Before(before.Add(-time.Second)) || task.PollRangeEnd.Time().After(after) {
		t.Fatalf("poll_range_end = %s, want run window around %s - %s", task.PollRangeEnd.Time(), before, after)
	}
}

// TestPruneSourceFailureDoesNotDelete verifies incomplete source listings never delete.
func TestPruneSourceFailureDoesNotDelete(t *testing.T) {
	db := setupSyncerDB(t)
	insertTaskContext(t, db, "conn-1", "kb-1", "task-1", dao.TaskTypePrune)
	_ = db.Model(&entity.SyncLogs{}).Where("id = ?", "task-1").Update("status", dao.SyncStatusRunning).Error
	taskDAO := dao.NewSyncTaskDAO(db)
	taskService := service.NewSyncTaskService(taskDAO)
	deleter := &fakeDeleter{}
	pruneService := service.NewSyncPruneService(deleter, fakeStore{ids: map[string]struct{}{"stale": {}}})
	connector := &connectormock.Connector{PruneErrAt: 1, PruneBatches: []syncerconnector.PruneBatch{{Documents: []syncerconnector.SlimDocument{{SourceID: "keep"}}}}}
	queue := make(chan TaskEnvelope, 1)
	worker := NewTaskWorker(queue, taskDAO, taskService, newCoordinator(taskDAO, taskService, newTestRegistry(map[string]*connectormock.Connector{"conn-1": connector}), &fakeSink{}, pruneService, fakeStore{}), NewConnectorLock())
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

// TestTaskExecutionDeadlineCapsConnectorTimeout verifies tasks cannot outlive the connector lock lease.
func TestTaskExecutionDeadlineCapsConnectorTimeout(t *testing.T) {
	now := time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)
	taskContext := dao.SyncTaskContext{}
	taskContext.Connector.TimeoutSecs = int64(connectorLockTTL.Seconds()) + 60
	lease := ConnectorLockLease{ExpiresAt: now.Add(10 * time.Minute)}
	if got, want := taskExecutionDeadline(now, taskContext, lease), lease.ExpiresAt.Add(-connectorLockSafetyMargin); !got.Equal(want) {
		t.Fatalf("deadline = %s, want %s", got, want)
	}
	taskContext.Connector.TimeoutSecs = 30
	if got, want := taskExecutionDeadline(now, taskContext, lease), now.Add(30*time.Second); !got.Equal(want) {
		t.Fatalf("deadline = %s, want %s", got, want)
	}
	lease.ExpiresAt = now.Add(time.Second)
	if got := taskExecutionDeadline(now, taskContext, lease); !got.Equal(now) {
		t.Fatalf("deadline = %s, want %s", got, now)
	}
}

func testLockLease() ConnectorLockLease {
	return ConnectorLockLease{ExpiresAt: time.Now().Add(connectorLockTTL)}
}
