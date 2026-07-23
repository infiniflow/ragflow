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
	"fmt"
	"sync/atomic"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/pipeline"
	servicepkg "ragflow/internal/service"
	documentpkg "ragflow/internal/service/document"
)

// progressSink implements pipeline.ProgressSink. It is the single writer of
// the document / ingestion_task_log / ingestion_task.component_total tables
// for a pipeline run: the pipeline reports component lifecycle events here
// and the sink persists them through the service layer (IngestionTaskService
// + DocumentService), never the DAO directly. All writes are best-effort -
// failures are logged and never abort the run, mirroring the legacy
// pipeline-internal sink semantics.
type progressSink struct {
	taskSvc *servicepkg.IngestionTaskService
	docSvc  docProgressSvc
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
	UpdateRunProgress(docID string, progress float64, run, progressMsg string) error
}

func newProgressSink(taskSvc *servicepkg.IngestionTaskService) *progressSink {
	// Eagerly construct the DocumentService so docSvc is immutable after this
	// point. eino's compose graph runs parallel branches concurrently, so
	// OnComponentProgress (and thus docSvc) can fire from multiple goroutines;
	// a lazy check-then-act here would be a data race. The sink owns no
	// server-config dependency, so this is safe in any environment.
	return &progressSink{
		taskSvc: taskSvc,
		docSvc:  documentpkg.NewDocumentService(),
	}
}

func (s *progressSink) OnComponentTotal(taskID string, total int) {
	s.total.Store(int64(total))
	if err := s.taskSvc.UpdateComponentTotal(taskID, total); err != nil {
		common.Error(fmt.Sprintf("progressSink: update component_total for task %s failed: %v", taskID, err), err)
	}
}

func (s *progressSink) OnComponentProgress(ev pipeline.ProgressEvent) {
	if err := s.taskSvc.RecordComponentProgress(ev.TaskID, ev.Component, ev.Phase, ev.Message); err != nil {
		common.Error(fmt.Sprintf("progressSink: record component progress for task %s failed: %v", ev.TaskID, err), err)
	}
	if ev.DocumentID == "" {
		return
	}
	total := s.total.Load()
	agg, err := s.taskSvc.AggregateTaskProgress(ev.TaskID, int(total))
	if err != nil {
		common.Error(fmt.Sprintf("progressSink: aggregate task progress for task %s failed: %v", ev.TaskID, err), err)
		return
	}
	if agg == nil || total <= 0 {
		return
	}
	progress, run := deriveDocumentProgress(agg, int(total))
	if err := s.docSvc.UpdateRunProgress(ev.DocumentID, progress, run, ev.Message); err != nil {
		common.Error(fmt.Sprintf("progressSink: mirror progress to document %s for task %s failed: %v", ev.DocumentID, ev.TaskID, err), err)
	}
}

// deriveDocumentProgress computes the document-level progress (0..1) and run
// label ("0".."4", matching Python's document.run enum) from the aggregated
// ingestion_task_log. This logic is owned by the sink (the document-table
// writer), not the pipeline.
func deriveDocumentProgress(agg *dao.TaskProgress, total int) (float64, string) {
	run := string(entity.TaskStatusUnstart)
	switch {
	case agg.Failed > 0:
		run = string(entity.TaskStatusFail)
	case agg.Done == total:
		run = string(entity.TaskStatusDone)
	case agg.Done > 0 || agg.Running > 0:
		run = string(entity.TaskStatusRunning)
	}
	return agg.Percent / 100, run
}
