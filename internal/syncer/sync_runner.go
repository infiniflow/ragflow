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
	"math/rand"
	"ragflow/internal/common"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	"time"

	"go.uber.org/zap"
)

var errSyncTaskCanceled = errors.New("sync task canceled")

const syncCancelCheckInterval = time.Second

// SyncRunner executes one SYNC task by submitting source batches as BatchJobs.
type SyncRunner struct {
	config      TaskCoordinatorConfig
	taskService *service.SyncTaskService
	sink        service.DocumentSink
	idResolver  *service.DocumentIDResolver
	queue       *SyncJobQueue
	checkpoints SyncCheckpointStore
}

// NewSyncRunner creates a SYNC runner.
func NewSyncRunner(config TaskCoordinatorConfig, taskService *service.SyncTaskService, sink service.DocumentSink, idResolver *service.DocumentIDResolver, queue *SyncJobQueue, checkpoints SyncCheckpointStore) *SyncRunner {
	if checkpoints == nil {
		checkpoints = newMemorySyncCheckpointStore()
	}
	return &SyncRunner{config: config, taskService: taskService, sink: sink, idResolver: idResolver, queue: queue, checkpoints: checkpoints}
}

// Run executes all sync batches and commits the final waterline.
func (r *SyncRunner) Run(ctx context.Context, taskContext service.SyncTaskContext, connector syncerconnector.Connector) (string, error) {
	// sink is nil means this syncer task cannot write it to document, it will fail anyway
	if r.sink == nil {
		return "", errors.New("document sink is not configured")
	}
	var windowStart *time.Time // = nil if it is `Full synchronisation`
	if !service.IsFromBeginning(taskContext.Task.FromBeginning) {
		if pollRangeStart := taskContext.Task.PollRangeStart; pollRangeStart != nil {
			if start := pollRangeStart.Time(); !start.IsZero() {
				windowStart = &start
			}
		}
	}
	checkpointState, err := r.prepareCheckpoint(ctx, taskContext, windowStart, time.Now().UTC())
	if err != nil {
		return "", err
	}

	sourceType := service.SourceType(taskContext.Connector.Source, taskContext.Connector.ID)
	fingerprints, err := r.idResolver.ListFingerprintsBySourceType(ctx, taskContext.Knowledgebase.ID, sourceType)
	if err != nil {
		return "", err
	}
	session, err := connector.OpenSync(ctx, syncerconnector.SyncRequest{
		TaskID:        taskContext.Task.ID,
		ConnectorID:   taskContext.Connector.ID,
		KBID:          taskContext.Knowledgebase.ID,
		SourceType:    sourceType,
		Fingerprints:  fingerprints,
		FromBeginning: checkpointState.WindowStart == nil,
		WindowStart:   checkpointState.WindowStart,
		WindowEnd:     checkpointState.WindowEnd,
		Resume:        checkpointState.Checkpoint,
	})
	if err != nil {
		return "", err
	}
	defer session.Close()

	// prepare sourceType, waterline, stats, resultChan
	stats := statsFromCheckpointState(checkpointState) // count `add`, `updated`, `skipped`
	resultChans := make([]<-chan syncJobResult, 0)

	for {
		// check if task has been canceled
		if err := r.checkCanceled(ctx, taskContext.Task.ID); err != nil {
			return "", err
		}

		// get a batch of files
		batch, nextErr := session.NextBatch(ctx)
		if errors.Is(nextErr, io.EOF) { // end of file
			break
		}
		if nextErr != nil {
			if err = r.collectResults(ctx, taskContext.Task.ID, resultChans, &checkpointState, &stats); err != nil {
				return "", err
			}
			return "", nextErr
		}

		// a batch, a syncJob
		resultChan, err := r.submitBatch(ctx, taskContext, sourceType, session, batch)
		if err != nil {
			return "", err
		}
		resultChans = append(resultChans, resultChan)
	}

	if err = r.collectResults(ctx, taskContext.Task.ID, resultChans, &checkpointState, &stats); err != nil {
		return "", err
	}

	if err := r.checkCanceled(ctx, taskContext.Task.ID); err != nil {
		return "", err
	}
	nextTaskID, err := r.taskService.CompleteSync(ctx, taskContext, checkpointState.WindowEnd, stats)
	if err != nil {
		return "", err
	}
	if err = r.checkpoints.DeleteSyncCheckpoint(context.WithoutCancel(ctx), taskContext.Task.ID); err != nil {
		common.Warn("delete sync checkpoint failed", zap.Error(err))
	} else {
		common.Info("delete sync checkpoint completed", zap.String("task_id", taskContext.Task.ID))
	}
	return nextTaskID, nil
}

func (r *SyncRunner) collectResults(ctx context.Context, taskID string, resultChans []<-chan syncJobResult, checkpointState *syncerconnector.SyncCheckpointState, stats *service.SyncStats) error {
	var firstErr error
	for _, resultChan := range resultChans {
		var jobResult syncJobResult
		select {
		case <-ctx.Done():
			return ctx.Err()
		case jobResult = <-resultChan:
		}
		stats.Add(jobResult.stats)
		if jobResult.err != nil && firstErr == nil {
			firstErr = jobResult.err
		}
		if firstErr == nil && jobResult.checkpoint != nil {
			checkpointState.Checkpoint = cloneSyncCheckpoint(jobResult.checkpoint)
			checkpointState.NextCommitSeq++
			applyStatsToCheckpointState(checkpointState, *stats)
			if err := r.checkpoints.SaveSyncCheckpoint(ctx, taskID, *checkpointState); err != nil {
				return err
			}
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return nil
}

func (r *SyncRunner) prepareCheckpoint(ctx context.Context, taskContext service.SyncTaskContext, windowStart *time.Time, windowEnd time.Time) (syncerconnector.SyncCheckpointState, error) {
	state, err := r.checkpoints.LoadSyncCheckpoint(ctx, taskContext.Task.ID)
	if err != nil {
		return syncerconnector.SyncCheckpointState{}, err
	}
	if state != nil && !state.WindowEnd.IsZero() && state.TaskID == taskContext.Task.ID && state.ConnectorID == taskContext.Connector.ID && state.KBID == taskContext.Knowledgebase.ID {
		if state.Version == 0 {
			state.Version = 1
		}
		if state.NextCommitSeq == 0 {
			state.NextCommitSeq = 1
		}
		return *state, nil
	}
	// if checkpoint is not exist
	initial := syncerconnector.SyncCheckpointState{
		Version:       1,
		TaskID:        taskContext.Task.ID,
		ConnectorID:   taskContext.Connector.ID,
		KBID:          taskContext.Knowledgebase.ID,
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		NextCommitSeq: 1,
	}
	if err = r.checkpoints.SaveSyncCheckpoint(ctx, taskContext.Task.ID, initial); err != nil {
		return syncerconnector.SyncCheckpointState{}, err
	}
	return initial, nil
}

func statsFromCheckpointState(state syncerconnector.SyncCheckpointState) service.SyncStats {
	return service.SyncStats{
		Added:      state.Added,
		Updated:    state.Updated,
		Skipped:    state.Skipped,
		ErrorCount: state.ErrorCount,
		ErrorMsg:   state.ErrorMsg,
	}
}

// applyStatsToCheckpointState store checkpoint's state data
func applyStatsToCheckpointState(state *syncerconnector.SyncCheckpointState, stats service.SyncStats) {
	state.Added = stats.Added
	state.Updated = stats.Updated
	state.Skipped = stats.Skipped
	state.ErrorCount = stats.ErrorCount
	state.ErrorMsg = stats.ErrorMsg
}

// submitBatch submits one source batch as one BatchJob.
func (r *SyncRunner) submitBatch(ctx context.Context, taskContext service.SyncTaskContext, sourceType string, session syncerconnector.SyncSession, batch syncerconnector.SyncBatch) (<-chan syncJobResult, error) {
	resultChan, err := r.queue.submit(ctx, func(jobCtx context.Context) (service.SyncStats, error) {
		return r.processDocuments(jobCtx, taskContext, sourceType, session, batch.Documents)
	}, batch.Checkpoint)
	if err != nil {
		return nil, err
	}
	return resultChan, nil
}

// cloneSyncCheckpoint safe clone
func cloneSyncCheckpoint(checkpoint *syncerconnector.SyncCheckpoint) *syncerconnector.SyncCheckpoint {
	if checkpoint == nil {
		return nil
	}
	clone := &syncerconnector.SyncCheckpoint{
		Cursor:   checkpoint.Cursor,
		SourceID: checkpoint.SourceID,
	}
	if checkpoint.UpdatedAt != nil {
		updatedAt := *checkpoint.UpdatedAt
		clone.UpdatedAt = &updatedAt
	}
	return clone
}

// processDocuments
func (r *SyncRunner) processDocuments(ctx context.Context, taskContext service.SyncTaskContext, sourceType string, session syncerconnector.SyncSession, documents []syncerconnector.SourceDocument) (service.SyncStats, error) {
	stats := service.SyncStats{}

	var firstErr error
	lastCancelCheck := time.Time{}
	for _, sourceDocument := range documents {
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		if lastCancelCheck.IsZero() || time.Since(lastCancelCheck) >= syncCancelCheckInterval {
			if err := r.checkCanceled(ctx, taskContext.Task.ID); err != nil {
				return stats, err
			}
			lastCancelCheck = time.Now()
		}
		result, err := r.processDocumentWithRetry(ctx, taskContext, sourceType, session, sourceDocument)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		stats.AddResult(result)
	}
	return stats, firstErr
}

// processDocumentWithRetry retries transient item failures.
func (r *SyncRunner) processDocumentWithRetry(ctx context.Context, taskContext service.SyncTaskContext, sourceType string, session syncerconnector.SyncSession, sourceDocument syncerconnector.SourceDocument) (service.DocumentUpsertResult, error) {
	var lastErr error
	for attempt := 1; attempt <= r.config.ItemRetryCount; attempt++ {
		if err := ctx.Err(); err != nil {
			return service.DocumentUpsertResult{}, err
		}
		result, err := r.processDocument(ctx, taskContext, sourceType, session, sourceDocument)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !service.IsRetryable(err) || attempt == r.config.ItemRetryCount {
			break
		}

		shift := attempt - 1
		if shift > 30 {
			shift = 30
		}
		delay := r.config.ItemRetryBaseDelay * time.Duration(1<<shift)
		if delay < 0 {
			delay = time.Hour
		}
		if jitterMax := r.config.ItemRetryBaseDelay / 2; jitterMax > 0 {
			delay += time.Duration(rand.Int63n(int64(jitterMax)))
		}

		select {
		case <-ctx.Done():
			return service.DocumentUpsertResult{}, ctx.Err()
		case <-time.After(delay):
		}
	}
	return service.DocumentUpsertResult{}, lastErr
}

// checkCanceled check if the task has been canceled
func (r *SyncRunner) checkCanceled(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	canceled, err := r.taskService.IsCanceled(ctx, taskID)
	if err != nil {
		return err
	}

	if canceled {
		return errSyncTaskCanceled
	}
	return nil
}

// processDocument resolves IDs, skips unchanged fingerprints, fetches blobs, and upserts.
func (r *SyncRunner) processDocument(ctx context.Context, taskContext service.SyncTaskContext, sourceType string, session syncerconnector.SyncSession, sourceDocument syncerconnector.SourceDocument) (service.DocumentUpsertResult, error) {
	resolved, err := r.idResolver.Resolve(ctx, taskContext.Knowledgebase.ID, taskContext.Connector.ID, sourceType, sourceDocument.SourceID)
	if err != nil {
		return service.DocumentUpsertResult{}, err
	}
	if sourceDocument.Fingerprint != "" && resolved.StoredFingerprint == sourceDocument.Fingerprint {
		return service.DocumentUpsertResult{DocID: resolved.DocID, Action: service.DocumentActionSkipped}, nil
	}

	if len(sourceDocument.Blob) == 0 && sourceDocument.FetchRef != nil {
		fetcher, ok := session.(syncerconnector.Fetcher)
		if !ok {
			return service.DocumentUpsertResult{}, fmt.Errorf("connector session cannot fetch %s", sourceDocument.FetchRef.Key)
		}
		blob, err := fetcher.Fetch(ctx, *sourceDocument.FetchRef)
		if err != nil {
			return service.DocumentUpsertResult{}, err
		}
		sourceDocument.Blob = blob
	}

	return r.sink.Upsert(ctx, service.DocumentUpsertInput{
		TaskContext:    taskContext,
		SourceType:     sourceType,
		DocumentID:     resolved.DocID,
		LegacyID:       resolved.LegacyID,
		NewID:          resolved.NewID,
		SourceDocument: sourceDocument,
		AutoParse:      taskContext.Connector2Kb.AutoParse != "0",
	})
}
