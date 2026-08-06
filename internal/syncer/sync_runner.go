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
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	"sync"
	"time"
)

// SyncRunner executes one SYNC task with serial batches and parallel items.
type SyncRunner struct {
	config      TaskCoordinatorConfig
	taskService *service.SyncTaskService
	sink        service.DocumentSink
	idResolver  *service.DocumentIDResolver
	globalItems chan struct{}
}

// NewSyncRunner creates a SYNC runner.
func NewSyncRunner(config TaskCoordinatorConfig, taskService *service.SyncTaskService, sink service.DocumentSink, idResolver *service.DocumentIDResolver, globalItems chan struct{}) *SyncRunner {
	return &SyncRunner{config: config, taskService: taskService, sink: sink, idResolver: idResolver, globalItems: globalItems}
}

// Run executes all sync batches and commits the final waterline.
func (r *SyncRunner) Run(ctx context.Context, taskContext service.SyncTaskContext, connector syncerconnector.Connector) error {
	if r.sink == nil {
		return errors.New("document sink is not configured")
	}
	windowEnd := time.Now().UTC()

	var windowStart *time.Time
	if !service.IsFromBeginning(taskContext.Task.FromBeginning) {
		windowStart = taskContext.Task.PollRangeStart
	}

	session, err := connector.OpenSync(ctx, syncerconnector.SyncRequest{
		TaskID:        taskContext.Task.ID,
		ConnectorID:   taskContext.Connector.ID,
		KBID:          taskContext.Knowledgebase.ID,
		FromBeginning: windowStart == nil,
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
	})
	if err != nil {
		return err
	}
	defer session.Close()

	sourceType := service.SourceType(taskContext.Connector.Source, taskContext.Connector.ID)
	candidateEnd := windowStart
	stats := service.SyncStats{}
	for {
		batch, nextErr := session.NextBatch(ctx)
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nextErr
		}
		for _, doc := range batch.Documents {
			if candidateEnd == nil || doc.UpdatedAt.After(*candidateEnd) {
				updatedAt := doc.UpdatedAt
				candidateEnd = &updatedAt
			}
		}
		batchStats, err := r.processBatch(ctx, taskContext, sourceType, session, batch)
		if err != nil {
			return err
		}
		stats.Add(batchStats)
	}

	if candidateEnd == nil {
		candidateEnd = &windowEnd
	}
	return r.taskService.CompleteSync(ctx, taskContext, *candidateEnd, stats)
}

// processBatch processes one batch with bounded document concurrency.
func (r *SyncRunner) processBatch(ctx context.Context, taskContext service.SyncTaskContext, sourceType string, session syncerconnector.SyncSession, batch syncerconnector.SyncBatch) (service.SyncStats, error) {
	sem := make(chan struct{}, r.config.PerTaskItemConcurrency)
	results := make(chan service.DocumentUpsertResult, len(batch.Documents))
	errs := make(chan error, len(batch.Documents))

	var wg sync.WaitGroup
	for _, sourceDocument := range batch.Documents {
		sourceDocument := sourceDocument

		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			result, err := r.processDocumentWithRetry(ctx, taskContext, sourceType, session, sourceDocument)
			if err != nil {
				errs <- err
				return
			}
			results <- result
		}()
	}

	wg.Wait()
	close(results)
	close(errs)

	if len(errs) > 0 {
		return service.SyncStats{}, <-errs
	}
	stats := service.SyncStats{}

	for result := range results {
		stats.AddResult(result)
	}
	return stats, nil
}

// processDocumentWithRetry retries transient item failures.
func (r *SyncRunner) processDocumentWithRetry(ctx context.Context, taskContext service.SyncTaskContext, sourceType string, session syncerconnector.SyncSession, sourceDocument syncerconnector.SourceDocument) (service.DocumentUpsertResult, error) {
	var lastErr error
	for attempt := 1; attempt <= r.config.ItemRetryCount; attempt++ {
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

// processDocument resolves IDs, skips unchanged fingerprints, fetches blobs, and upserts.
func (r *SyncRunner) processDocument(ctx context.Context, taskContext service.SyncTaskContext, sourceType string, session syncerconnector.SyncSession, sourceDocument syncerconnector.SourceDocument) (service.DocumentUpsertResult, error) {
	if r.globalItems != nil {
		select {
		case <-ctx.Done():
			return service.DocumentUpsertResult{}, ctx.Err()
		case r.globalItems <- struct{}{}:
			defer func() { <-r.globalItems }()
		}
	}

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
