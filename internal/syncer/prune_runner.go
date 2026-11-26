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
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"

	"go.uber.org/zap"
)

// PruneRunner executes one PRUNE task after collecting a full slim snapshot.
type PruneRunner struct {
	taskDAO      *dao.SyncTaskDAO
	taskService  *service.SyncTaskService
	pruneService *service.SyncPruneService
}

// NewPruneRunner creates a PRUNE runner.
func NewPruneRunner(taskDAO *dao.SyncTaskDAO, taskService *service.SyncTaskService, pruneService *service.SyncPruneService) *PruneRunner {
	return &PruneRunner{taskDAO: taskDAO, taskService: taskService, pruneService: pruneService}
}

// Run collects the full prune snapshot before deleting stale documents.
func (r *PruneRunner) Run(ctx context.Context, taskContext dao.SyncTaskContext, connector syncerconnector.Connector) (string, error) {
	if r.pruneService == nil {
		return "", errors.New("prune service is not configured")
	}
	// Get the run session
	session, err := connector.OpenPrune(ctx, syncerconnector.PruneRequest{
		TaskID:      taskContext.Task.ID,
		ConnectorID: taskContext.Connector.ID,
		KBID:        taskContext.Knowledgebase.ID,
	})
	if err != nil {
		if errors.Is(err, syncerconnector.ErrPruneUnsupported) {
			// Connectors without a slim snapshot interface (e.g. REST API)
			// complete PRUNE as a no-op without deleting anything.
			if err := checkTaskCanceled(r.taskDAO, ctx, taskContext.Task.ID); err != nil {
				return "", err
			}
			common.Warn("prune unsupported by connector, completing as no-op", zap.String("task_id", taskContext.Task.ID), zap.Error(err))
			recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "prune", "Source does not support pruning")
			return r.taskService.CompletePrune(ctx, taskContext, 0)
		}
		recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "prune", "[ERROR] Could not open prune source scan")
		return "", err
	}
	defer session.Close()

	retain := map[string]struct{}{}
	sourceIDs := map[string]struct{}{}
	for {
		if err := checkTaskCanceled(r.taskDAO, ctx, taskContext.Task.ID); err != nil {
			return "", err
		}
		batch, nextErr := session.NextBatch(ctx)
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "prune", "[ERROR] Could not read prune source batch")
			return "", nextErr
		}
		for _, doc := range batch.Documents {
			sourceIDs[doc.SourceID] = struct{}{}
			service.AddRetainDocumentID(retain, taskContext.Knowledgebase.ID, taskContext.Connector.ID, doc.SourceID)
		}
	}

	if err := checkTaskCanceled(r.taskDAO, ctx, taskContext.Task.ID); err != nil {
		return "", err
	}
	recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "prune", fmt.Sprintf("Source scan found %d document IDs to retain", len(sourceIDs)))
	removed, err := r.pruneService.DeleteStale(ctx, taskContext, retain, func(docID string, deleteErr error) {
		message := "File " + docID + " deleted"
		if deleteErr != nil {
			message = "[ERROR] Could not delete file " + docID
		}
		recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "file", message)
	})
	if err != nil {
		recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "prune", fmt.Sprintf("[ERROR] Prune stopped after deleting %d files", removed))
		return "", err
	}
	recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "prune", fmt.Sprintf("Removed %d stale files", removed))
	nextTask, err := r.taskService.CompletePrune(ctx, taskContext, removed)
	if err != nil {
		recordTaskEvent(taskContext.Task.ID, dao.EventTypeMessage, "prune", "[ERROR] Could not save prune completion")
	}
	return nextTask, err
}
