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
	"ragflow/internal/service/document"
	"ragflow/internal/utility"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	connectorTaskTypeSync  = "sync"
	connectorTaskTypePrune = "prune"

	connectorSourceRSS = "rss"

	// defaultIndexBatchSize mirrors Python common.data_source.config.INDEX_BATCH_SIZE.
	defaultIndexBatchSize = 2
	// defaultTimeoutSecs mirrors entity.Connector's timeout_secs default.
	defaultTimeoutSecs = 3600
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

// pollAndExecute queries due sync & prune tasks, claims one, and runs it.
// One task per poll per worker; the worker count bounds total concurrency.
func (s *Syncer) pollAndExecute(workerID int) {
	connectorDAO := dao.NewConnectorDAO()
	for _, candidate := range []struct {
		taskType  string
		freqField string
	}{
		{connectorTaskTypeSync, "refresh_freq"},
		{connectorTaskTypePrune, "prune_freq"},
	} {
		tasks, err := connectorDAO.ListDueTasks(s.ctx, dao.GetDB(), candidate.taskType, candidate.freqField)
		if err != nil {
			common.Error(fmt.Sprintf("Syncer worker %d failed to list due %s tasks", workerID, candidate.taskType), err)
			continue
		}
		for _, task := range tasks {
			if candidate.taskType == connectorTaskTypePrune {
				// Prune is opt-in at the connector config level; keep the
				// scheduler blind to prune_freq until the flag is enabled.
				if !connectorConfigBool(task.Config, "sync_deleted_files") || task.PruneFreq <= 0 {
					continue
				}
			}
			claimed, err := connectorDAO.ClaimTask(s.ctx, dao.GetDB(), task.ID, task.ConnectorID)
			if err != nil {
				common.Error(fmt.Sprintf("Syncer worker %d failed to claim task %s", workerID, task.ID), err)
				continue
			}
			if !claimed {
				continue
			}
			s.executeTask(task)
			return
		}
	}
}

// executeTask runs one claimed task with its configured timeout and handles
// the done/fail bookkeeping plus rescheduling, mirroring Python
// SyncBase.__call__ in rag/svr/sync_data_source.py.
func (s *Syncer) executeTask(task *dao.DueSyncTask) {
	connectorDAO := dao.NewConnectorDAO()
	db := dao.GetDB()

	timeoutSecs := task.TimeoutSecs
	if timeoutSecs <= 0 {
		timeoutSecs = defaultTimeoutSecs
	}
	ctx, cancel := context.WithTimeout(s.ctx, time.Duration(timeoutSecs)*time.Second)

	var runErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = fmt.Errorf("%v\n%s", recovered, debug.Stack())
			}
		}()
		switch task.TaskType {
		case connectorTaskTypePrune:
			runErr = s.executePruneTask(ctx, task)
		default:
			runErr = s.executeSyncTask(ctx, task)
		}
	}()
	cancel()

	if ctx.Err() == context.DeadlineExceeded {
		msg := fmt.Sprintf("Task timeout after %d seconds", timeoutSecs)
		if finishErr := connectorDAO.FinishTask(context.Background(), db, task.ID, task.ConnectorID, entity.TaskStatusFail, msg, ""); finishErr != nil {
			common.Error(fmt.Sprintf("Failed to mark task %s failed", task.ID), finishErr)
		}
		return
	}
	if runErr != nil {
		if finishErr := connectorDAO.FinishTask(context.Background(), db, task.ID, task.ConnectorID, entity.TaskStatusFail, runErr.Error(), string(debug.Stack())); finishErr != nil {
			common.Error(fmt.Sprintf("Failed to mark task %s failed", task.ID), finishErr)
		}
		return
	}

	if err := connectorDAO.FinishTask(context.Background(), db, task.ID, task.ConnectorID, entity.TaskStatusDone, "", ""); err != nil {
		common.Error(fmt.Sprintf("Failed to mark task %s done", task.ID), err)
		return
	}

	switch task.TaskType {
	case connectorTaskTypeSync:
		if err := connectorDAO.RescheduleTask(context.Background(), db, task.ConnectorID, task.KbID, connectorTaskTypeSync); err != nil {
			common.Error(fmt.Sprintf("Failed to reschedule sync for connector %s", task.ConnectorID), err)
		}
	case connectorTaskTypePrune:
		if connectorConfigBool(task.Config, "sync_deleted_files") {
			if err := connectorDAO.RescheduleTask(context.Background(), db, task.ConnectorID, task.KbID, connectorTaskTypePrune); err != nil {
				common.Error(fmt.Sprintf("Failed to reschedule prune for connector %s", task.ConnectorID), err)
			}
		}
	}
}

// executeSyncTask runs a sync task: fetch documents from the source, import
// the changed ones into the knowledge base, and enqueue parsing when the
// connector link has auto_parse enabled. Mirrors Python SyncBase._run_sync_task_logic.
func (s *Syncer) executeSyncTask(ctx context.Context, task *dao.DueSyncTask) error {
	common.Info("Executing sync task",
		zap.String("task_id", task.ID),
		zap.String("connector_id", task.ConnectorID),
		zap.String("source", task.Source),
		zap.String("kb_id", task.KbID))

	if task.Source != connectorSourceRSS {
		return fmt.Errorf("data source %q is not supported by the Go syncer", task.Source)
	}

	connectorDAO := dao.NewConnectorDAO()
	db := dao.GetDB()

	kb, err := dao.NewKnowledgebaseDAO().GetByID(ctx, db, task.KbID)
	if err != nil {
		return fmt.Errorf("knowledgebase %s: %w", task.KbID, err)
	}

	connector := newRSSConnector(connectorConfigString(task.Config, "feed_url"), connectorConfigInt(task.Config, "batch_size", defaultIndexBatchSize))

	reindex := task.FromBeginning != nil && *task.FromBeginning == "1"
	var start, end *time.Time
	if !reindex && task.PollRangeStart != nil {
		start = task.PollRangeStart
		now := time.Now().UTC()
		end = &now
	}
	batches, err := connector.load(ctx, start, end)
	if err != nil {
		return err
	}

	sourceType := fmt.Sprintf("%s/%s", task.Source, task.ConnectorID)
	existingDocs, err := connectorDAO.ListDocumentsByKBAndSourceType(ctx, db, task.KbID, sourceType)
	if err != nil {
		return err
	}
	existingDocIDs := make(map[string]bool, len(existingDocs))
	for _, existingDoc := range existingDocs {
		existingDocIDs[existingDoc.ID] = true
	}

	nextUpdate := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	if task.PollRangeStart != nil {
		nextUpdate = *task.PollRangeStart
	}

	docSvc := document.NewDocumentService()
	autoParse := connectorAutoParse(task.AutoParse)
	addedDocs, updatedDocs := 0, 0

	for _, batch := range batches {
		if err := ctx.Err(); err != nil {
			return err
		}
		maxUpdate := batch[0].UpdatedAt
		for _, doc := range batch {
			if doc.UpdatedAt.After(maxUpdate) {
				maxUpdate = doc.UpdatedAt
			}
		}
		if maxUpdate.After(nextUpdate) {
			nextUpdate = maxUpdate
		}

		inputs := make([]document.ConnectorDocumentInput, 0, len(batch))
		for _, doc := range batch {
			legacyDocID := document.Hash128Hex([]byte(task.ConnectorID + ":" + doc.ID))
			docID := document.Hash128Hex([]byte(task.KbID + ":" + task.ConnectorID + ":" + doc.ID))
			if existingDocIDs[legacyDocID] {
				docID = legacyDocID
			}
			inputs = append(inputs, document.ConnectorDocumentInput{
				ID:                 docID,
				SemanticIdentifier: doc.SemanticIdentifier,
				Extension:          ".txt",
				Blob:               doc.Blob,
				Metadata:           doc.Metadata,
			})
		}

		results, errs := docSvc.UploadConnectorDocuments(ctx, kb, task.TenantID, sourceType, inputs)
		for _, result := range results {
			if len(result.Metadata) > 0 {
				if err := docSvc.SetDocumentMetadata(ctx, result.Doc.ID, result.Metadata); err != nil {
					common.Warn(fmt.Sprintf("Failed to set metadata for document %s: %v", result.Doc.ID, err))
				}
			}
			if autoParse {
				opts := document.StartParseOptions{RerunWithDelete: result.Existed}
				if err := docSvc.StartParseDocuments(ctx, result.Doc, kb, task.TenantID, opts); err != nil {
					errs = append(errs, err.Error())
				}
			}
			if existingDocIDs[result.Doc.ID] {
				updatedDocs++
			} else {
				addedDocs++
				existingDocIDs[result.Doc.ID] = true
			}
		}

		if err := connectorDAO.IncreaseDocs(ctx, db, task.ID, maxUpdate, int64(len(batch)), strings.Join(errs, "\n"), int64(len(errs))); err != nil {
			common.Error(fmt.Sprintf("Failed to increase docs for task %s", task.ID), err)
		}
	}

	totalChanged := addedDocs + updatedDocs
	common.Info(fmt.Sprintf("rss sync summary till %s: total=%d, added=%d, updated=%d",
		nextUpdate.UTC().Format(time.RFC3339), totalChanged, addedDocs, updatedDocs),
		zap.String("task_id", task.ID),
		zap.String("connector_id", task.ConnectorID),
		zap.String("kb_id", task.KbID))
	return nil
}

// executePruneTask deletes connector documents that no longer exist at the
// source. Mirrors Python SyncBase._run_prune_task_logic.
func (s *Syncer) executePruneTask(ctx context.Context, task *dao.DueSyncTask) error {
	common.Info("Executing prune task",
		zap.String("task_id", task.ID),
		zap.String("connector_id", task.ConnectorID),
		zap.String("source", task.Source),
		zap.String("kb_id", task.KbID))

	if !connectorConfigBool(task.Config, "sync_deleted_files") {
		return nil
	}
	if task.Source != connectorSourceRSS {
		return fmt.Errorf("data source %q is not supported by the Go syncer", task.Source)
	}

	connectorDAO := dao.NewConnectorDAO()
	db := dao.GetDB()

	connector := newRSSConnector(connectorConfigString(task.Config, "feed_url"), connectorConfigInt(task.Config, "batch_size", defaultIndexBatchSize))
	entryIDs, err := connector.listEntryIDs(ctx)
	if err != nil {
		common.Warn(fmt.Sprintf("rss prune snapshot retrieval failed (connector_id=%s, kb_id=%s): %v", task.ConnectorID, task.KbID, err))
		return nil
	}

	retain := make(map[string]bool, len(entryIDs)*2)
	for _, entryID := range entryIDs {
		retain[document.Hash128Hex([]byte(task.ConnectorID+":"+entryID))] = true
		retain[document.Hash128Hex([]byte(task.KbID+":"+task.ConnectorID+":"+entryID))] = true
	}

	sourceType := fmt.Sprintf("%s/%s", task.Source, task.ConnectorID)
	existingDocs, err := connectorDAO.ListDocumentsByKBAndSourceType(ctx, db, task.KbID, sourceType)
	if err != nil {
		return err
	}
	staleIDs := make([]string, 0)
	for _, existingDoc := range existingDocs {
		if !retain[existingDoc.ID] {
			staleIDs = append(staleIDs, existingDoc.ID)
		}
	}

	var removed int64
	var errs []string
	if len(staleIDs) > 0 {
		deleted, err := document.NewDocumentService().DeleteDocuments(ctx, staleIDs, false, task.KbID, task.TenantID)
		removed = int64(deleted)
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	if err := connectorDAO.IncreaseRemovedDocs(ctx, db, task.ID, removed, strings.Join(errs, "\n"), int64(len(errs))); err != nil {
		common.Error(fmt.Sprintf("Failed to increase removed docs for task %s", task.ID), err)
	}
	common.Info(fmt.Sprintf("rss prune summary: deleted=%d, errors=%d", removed, len(errs)),
		zap.String("task_id", task.ID),
		zap.String("connector_id", task.ConnectorID),
		zap.String("kb_id", task.KbID))
	return nil
}

func connectorConfigString(config entity.JSONMap, key string) string {
	if value, ok := config[key].(string); ok {
		return value
	}
	return ""
}

func connectorConfigInt(config entity.JSONMap, key string, defaultValue int) int {
	switch value := config[key].(type) {
	case float64:
		return int(value)
	case int64:
		return int(value)
	case int:
		return value
	case string:
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return defaultValue
}

func connectorConfigBool(config entity.JSONMap, key string) bool {
	switch value := config[key].(type) {
	case bool:
		return value
	case string:
		return value == "1" || value == "true" || value == "TRUE"
	}
	return false
}

func connectorAutoParse(autoParse string) bool {
	return autoParse != "0" && autoParse != "false"
}
