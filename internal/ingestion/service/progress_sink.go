//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"context"
	"fmt"
	"sync/atomic"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/ingestion/pipeline"
	servicepkg "ragflow/internal/service"
	documentpkg "ragflow/internal/service/document"
)

// progressSink implements pipeline.ProgressSink. It records a run's component
// events and total through the service layer, and mirrors only the numeric
// progress state to the owning document. All writes are best-effort so a
// reporting failure never aborts the pipeline run.
type progressSink struct {
	taskSvc       *servicepkg.IngestionTaskService
	docSvc        docProgressSvc
	pipelineLogID string
	// total is the component-count denominator cached from OnComponentTotal.
	// It is Store-d once in the Run goroutine and Load-ed by OnComponentProgress,
	// which eino fires from concurrent parallel-branch goroutines. Atomic because
	// the two access paths share no other synchronization.
	total atomic.Int64
}

// docProgressSvc is the subset of *service.DocumentService the sink needs to
// mirror run progress into the document row. Extracted as an interface so
// tests can inject a stub and assert the mirror call without depending on the
// full DocumentService surface.
type docProgressSvc interface {
	UpdateRunState(ctx context.Context, docID string, progress float64) error
}

func newProgressSink(ctx context.Context, taskSvc *servicepkg.IngestionTaskService, pipelineLogID string) *progressSink {
	// Eagerly construct the DocumentService so docSvc is immutable after this
	// point. eino's compose graph runs parallel branches concurrently, so
	// OnComponentProgress (and thus docSvc) can fire from multiple goroutines;
	// a lazy check-then-act here would be a data race. The sink owns no
	// server-config dependency, so this is safe in any environment.
	return &progressSink{
		taskSvc:       taskSvc,
		docSvc:        documentpkg.NewDocumentService(),
		pipelineLogID: pipelineLogID,
	}
}

func (s *progressSink) OnComponentTotal(ctx context.Context, taskID string, total int) {
	s.total.Store(int64(total))
	if err := s.taskSvc.UpdateComponentTotal(ctx, taskID, total); err != nil {
		common.Error(fmt.Sprintf("progressSink: update component_total for task %s failed: %v", taskID, err), err)
	}
}

func (s *progressSink) OnComponentProgress(ctx context.Context, ev pipeline.ProgressEvent) {
	if err := s.taskSvc.RecordLifecycle(ctx, s.pipelineLogID, ev.TaskID, ev.Component, ev.Phase, ev.Message); err != nil {
		common.Error(fmt.Sprintf("progressSink: record component progress for task %s failed: %v", ev.TaskID, err), err)
	}
	if ev.DocumentID == "" {
		return
	}
	total := s.total.Load()
	agg, err := s.taskSvc.AggregateTaskProgressByPipelineLogID(ctx, s.pipelineLogID, int(total))
	if err != nil {
		common.Error(fmt.Sprintf("progressSink: aggregate task progress for task %s failed: %v", ev.TaskID, err), err)
		return
	}
	if agg == nil || total <= 0 {
		return
	}
	progress := deriveDocumentProgress(agg, int(total))
	if err = s.docSvc.UpdateRunState(ctx, ev.DocumentID, progress); err != nil {
		common.Error(fmt.Sprintf("progressSink: update progress state for document %s task %s failed: %v", ev.DocumentID, ev.TaskID, err), err)
	}
}

// OnComponentMessage records detailed compiler-stage information without
// creating a lifecycle row, so completion percent remains based on actual
// canvas components.
func (s *progressSink) OnComponentMessage(ctx context.Context, taskID, _ string, component, message string) {
	if message == "" {
		return
	}
	if err := s.taskSvc.RecordMessage(ctx, s.pipelineLogID, taskID, fmt.Sprintf("%s: %s", component, message)); err != nil {
		common.Error(fmt.Sprintf("progressSink: record message for task %s failed", taskID), err)
	}
}

// deriveDocumentProgress computes the document-level progress (0..1) from the
// aggregated ingestion_task_log.
func deriveDocumentProgress(agg *dao.TaskProgress, total int) float64 {
	progress := agg.Percent / 100
	if progress > 1 {
		progress = 1
	}
	return progress
}
