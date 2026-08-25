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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/pipeline"
	servicepkg "ragflow/internal/service"
	documentpkg "ragflow/internal/service/document"
)

// progressLogMaxChars bounds the accumulated run log mirrored into
// document.progress_msg.
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
	logMu   sync.Mutex
	seeded  bool
	log     strings.Builder
	pending []string
	now     func() time.Time
}

// docProgressSvc is the subset of *service.DocumentService the sink needs to
// mirror run progress into the document row. Extracted as an interface so
// tests can inject a stub and assert the mirror call without depending on the
// full DocumentService surface.
type docProgressSvc interface {
	GetDocumentByID(ctx context.Context, docID string) (*documentpkg.DocumentResponse, error)
	UpdateRunState(ctx context.Context, docID string, progress float64, run string) error
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
		now:     time.Now,
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
	if err = s.updateDocumentProgress(ctx, ev.DocumentID, progress, run, ev.Message); err != nil {
		common.Error(fmt.Sprintf("progressSink: mirror progress to document %s for task %s failed: %v", ev.DocumentID, ev.TaskID, err), err)
	}
}

// OnComponentMessage appends detailed compiler-stage information without
// creating a lifecycle row. This keeps component_total and completion percent
// based on actual canvas components while exposing MAP/REDUCE/PLAN/REFINE
// counts through the existing document progress log.
func (s *progressSink) OnComponentMessage(ctx context.Context, taskID, docID, component, message string) {
	if docID == "" || message == "" {
		return
	}
	s.logMu.Lock()
	defer s.logMu.Unlock()
	doc, err := s.docSvc.GetDocumentByID(ctx, docID)
	if err != nil {
		common.Error(fmt.Sprintf("progressSink: read document %s for detail failed", docID), err)
		return
	}
	if doc == nil {
		return
	}
	run := ""
	if doc.Run != nil {
		run = *doc.Run
	}
	log, err := s.accumulateLogLocked(ctx, docID, fmt.Sprintf("%s: %s", component, message))
	if err != nil {
		common.Error(fmt.Sprintf("progressSink: append detail for document %s failed", docID), err)
		return
	}
	if err := s.docSvc.UpdateRunProgress(ctx, docID, doc.Progress, run, log); err != nil {
		common.Error(fmt.Sprintf("progressSink: persist detail for document %s failed", docID), err)
	}
}

// updateDocumentProgress serializes log accumulation with the corresponding
// document write. Without holding the same lock across both operations, a
// slower database write can store an older snapshot after a newer event has
// already been persisted.
func (s *progressSink) updateDocumentProgress(ctx context.Context, docID string, progress float64, run, msg string) error {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	log, err := s.accumulateLogLocked(ctx, docID, msg)
	if err != nil {
		// Keep the current run state durable even when the existing log cannot
		// be seeded. The next event will retry the seed and persist the log.
		stateErr := s.docSvc.UpdateRunState(ctx, docID, progress, run)
		return errors.Join(err, stateErr)
	}
	return s.docSvc.UpdateRunProgress(ctx, docID, progress, run, log)
}

// accumulateLog appends one component lifecycle line to the run log. It is
// retained as a small test seam; production writes use updateDocumentProgress
// so the append and document update share one lock.
func (s *progressSink) accumulateLog(ctx context.Context, docID, msg string) string {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	log, _ := s.accumulateLogLocked(ctx, docID, msg)
	return log
}

// accumulateLogLocked appends one component lifecycle line to the run log and
// returns the full accumulated log, or an error when the existing log could
// not be seeded. The caller must hold logMu.
func (s *progressSink) accumulateLogLocked(ctx context.Context, docID, msg string) (string, error) {
	if !s.seeded {
		doc, err := s.docSvc.GetDocumentByID(ctx, docID)
		if err != nil {
			if msg != "" {
				s.pending = append(s.pending, msg)
			}
			common.Warn(fmt.Sprintf("progressSink: seed run log from document %s: %v", docID, err))
			return "", fmt.Errorf("seed run log for document %s: %w", docID, err)
		}
		s.seeded = true
		if doc != nil && doc.ProgressMsg != nil {
			s.log.WriteString(*doc.ProgressMsg)
		}
		for _, pending := range s.pending {
			s.appendLogLineLocked(pending)
		}
		s.pending = nil
	}
	if msg == "" {
		return s.log.String(), nil
	}
	s.appendLogLineLocked(msg)
	return s.log.String(), nil
}

func (s *progressSink) appendLogLineLocked(msg string) {
	if s.log.Len() > 0 {
		s.log.WriteByte('\n')
	}
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	s.log.WriteString(now().Format("15:04:05"))
	s.log.WriteString(": ")
	s.log.WriteString(msg)
	log := trimLogHead(s.log.String(), progressLogMaxChars)
	if len(log) != s.log.Len() {
		s.log.Reset()
		s.log.WriteString(log)
	}
}

// trimLogHead drops whole lines from the head of text until the remainder
// fits maxChars, keeping the newest lines. Text without a fitting newline
// boundary is returned unchanged.
func trimLogHead(text string, maxChars int) string {
	if len(text) <= maxChars {
		return text
	}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' && len(text)-(i+1) <= maxChars {
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
	progress := agg.Percent / 100
	if progress > 1 {
		progress = 1
	}
	return progress, run
}
