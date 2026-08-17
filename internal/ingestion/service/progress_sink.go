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
	"strings"
	"sync"
	"sync/atomic"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/pipeline"
	servicepkg "ragflow/internal/service"
	documentpkg "ragflow/internal/service/document"
)

// progressLogMaxChars bounds the accumulated run log mirrored into
// document.progress_msg. Mirrors Python's TASK_MAX_LOG_LENGTH
// (api/db/services/task_service.py).
const progressLogMaxChars = 3000

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
	// logMu guards the accumulated run log below: OnComponentProgress fires
	// from concurrent parallel-branch goroutines, so both the lazy seed and
	// the append must hold the same mutex.
	logMu  sync.Mutex
	seeded bool
	log    strings.Builder
}

// docProgressSvc is the subset of *service.DocumentService the sink needs to
// mirror run progress into the document row. Extracted as an interface so
// tests can inject a stub and assert the mirror call without depending on the
// full DocumentService surface.
type docProgressSvc interface {
	GetDocumentByID(ctx context.Context, docID string) (*documentpkg.DocumentResponse, error)
	UpdateRunProgress(ctx context.Context, docID string, progress float64, run, progressMsg string) error
}

func newProgressSink(ctx context.Context, taskSvc *servicepkg.IngestionTaskService) *progressSink {
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

func (s *progressSink) OnComponentTotal(ctx context.Context, taskID string, total int) {
	s.total.Store(int64(total))
	if err := s.taskSvc.UpdateComponentTotal(ctx, taskID, total); err != nil {
		common.Error(fmt.Sprintf("progressSink: update component_total for task %s failed: %v", taskID, err), err)
	}
}

func (s *progressSink) OnComponentProgress(ctx context.Context, ev pipeline.ProgressEvent) {
	if err := s.taskSvc.RecordComponentProgress(ctx, ev.TaskID, ev.Component, ev.Phase, ev.Message); err != nil {
		common.Error(fmt.Sprintf("progressSink: record component progress for task %s failed: %v", ev.TaskID, err), err)
	}
	if ev.DocumentID == "" {
		return
	}
	total := s.total.Load()
	agg, err := s.taskSvc.AggregateTaskProgress(ctx, ev.TaskID, int(total))
	if err != nil {
		common.Error(fmt.Sprintf("progressSink: aggregate task progress for task %s failed: %v", ev.TaskID, err), err)
		return
	}
	if agg == nil || total <= 0 {
		return
	}
	progress, run := deriveDocumentProgress(agg, int(total))
	if err = s.docSvc.UpdateRunProgress(ctx, ev.DocumentID, progress, run, s.accumulateLog(ctx, ev.DocumentID, ev.Message)); err != nil {
		common.Error(fmt.Sprintf("progressSink: mirror progress to document %s for task %s failed: %v", ev.DocumentID, ev.TaskID, err), err)
	}
}

// accumulateLog appends one component lifecycle line to the run log and
// returns the full accumulated log for mirroring into document.progress_msg.
// The document-details dialog renders progress_msg verbatim as a multi-line
// log (whitespace-pre-line), matching Python's append-and-trim semantics in
// TaskService.update_progress; a single overwrite would leave the dialog
// showing only the latest line. The buffer is lazily seeded from the
// document row on the first event so a resumed run keeps the lines logged
// before the crash/redelivery instead of restarting empty (StartRunning
// resets the row to "" on a fresh run, which then seeds an empty log).
func (s *progressSink) accumulateLog(ctx context.Context, docID, msg string) string {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if !s.seeded {
		s.seeded = true
		doc, err := s.docSvc.GetDocumentByID(ctx, docID)
		switch {
		case err != nil:
			common.Warn(fmt.Sprintf("progressSink: seed run log from document %s: %v", docID, err))
		case doc != nil && doc.ProgressMsg != nil:
			s.log.WriteString(*doc.ProgressMsg)
		}
	}
	if msg == "" {
		return s.log.String()
	}
	if s.log.Len() > 0 {
		s.log.WriteByte('\n')
	}
	s.log.WriteString(msg)
	log := trimLogHead(s.log.String(), progressLogMaxChars)
	if len(log) != s.log.Len() {
		s.log.Reset()
		s.log.WriteString(log)
	}
	return log
}

// trimLogHead drops whole lines from the head of text until the remainder
// fits maxChars, keeping the newest lines. Mirrors Python's
// trim_header_by_lines: text without a fitting newline boundary is returned
// unchanged.
func trimLogHead(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' && len(text)-i <= maxChars {
			return text[i+1:]
		}
	}
	return text
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
