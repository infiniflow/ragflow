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
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
)

// PruneRunner executes one PRUNE task after collecting a full slim snapshot.
type PruneRunner struct {
	taskService  *service.SyncTaskService
	pruneService *service.SyncPruneService
}

// NewPruneRunner creates a PRUNE runner.
func NewPruneRunner(taskService *service.SyncTaskService, pruneService *service.SyncPruneService) *PruneRunner {
	return &PruneRunner{taskService: taskService, pruneService: pruneService}
}

// Run collects the full prune snapshot before deleting stale documents.
func (r *PruneRunner) Run(ctx context.Context, taskContext service.SyncTaskContext, connector syncerconnector.Connector) error {
	if r.pruneService == nil {
		return errors.New("prune service is not configured")
	}
	// Get the run session
	session, err := connector.OpenPrune(ctx, syncerconnector.PruneRequest{
		TaskID:      taskContext.Task.ID,
		ConnectorID: taskContext.Connector.ID,
		KBID:        taskContext.Knowledgebase.ID,
	})
	if err != nil {
		return err
	}
	defer session.Close()

	retain := map[string]struct{}{}
	for {
		batch, nextErr := session.NextBatch(ctx)
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nextErr
		}
		for _, doc := range batch.Documents {
			service.AddRetainDocumentID(retain, taskContext.Knowledgebase.ID, taskContext.Connector.ID, doc.SourceID)
		}
	}

	removed, err := r.pruneService.DeleteStale(ctx, taskContext, retain)
	if err != nil {
		return err
	}
	return r.taskService.CompletePrune(ctx, taskContext, removed)
}
