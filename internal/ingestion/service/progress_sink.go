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
	"sync"
	"time"

	agentruntime "ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/ingestion/pipeline"
	servicepkg "ragflow/internal/service"
	documentpkg "ragflow/internal/service/document"
)

// progressFlushInterval bounds how often the flusher persists the in-memory
// runProgress to the document row. Fraction reports (per parsed page, per
// embedded chunk) only mutate memory; the ticker coalesces them into at most
// one UPDATE per interval regardless of event rate.
const progressFlushInterval = time.Second

// progressSink implements pipeline.ProgressSink. It records a run's component
// events and total through the service layer, accumulates completion percent
// in memory (runProgress), and mirrors that percent into the owning document
// row from a single flusher goroutine. All writes are best-effort so a
// reporting failure never aborts the pipeline run.
type progressSink struct {
	taskSvc       *servicepkg.IngestionTaskService
	docSvc        docProgressSvc
	pipelineLogID string
	baseCtx       context.Context

	progress *runProgress

	// mu guards docID, which eino's parallel-branch callbacks bind
	// concurrently from lifecycle events.
	mu    sync.Mutex
	docID string

	wake      chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup

	// lastFlushed dedupes ticker writes while percent is unchanged. Only the
	// flusher goroutine touches it, and Close re-flushes after wg.Wait, so no
	// additional synchronization is needed.
	lastFlushed float64
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
	s := &progressSink{
		taskSvc:       taskSvc,
		docSvc:        documentpkg.NewDocumentService(),
		pipelineLogID: pipelineLogID,
		baseCtx:       ctx,
		progress:      newRunProgress(),
		wake:          make(chan struct{}, 1),
		closed:        make(chan struct{}),
	}
	s.wg.Add(1)
	go s.runFlusher(ctx)
	return s
}

// Close stops the flusher and performs one final forced flush so the last
// in-memory percent reaches the document row before the caller writes the
// terminal progress state. It is idempotent and safe to call from any
// goroutine. The final flush detaches from the (possibly cancelled) run
// context; callers must invoke it before returning from the run.
func (s *progressSink) Close() {
	s.closeOnce.Do(func() {
		close(s.closed)
		s.wg.Wait()
		ctx, cancel := context.WithTimeout(context.WithoutCancel(s.baseCtx), 5*time.Second)
		defer cancel()
		s.flush(ctx, true)
	})
}

func (s *progressSink) runFlusher(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(progressFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.closed:
			return
		case <-s.wake:
			s.flush(ctx, true)
		case <-ticker.C:
			s.flush(ctx, false)
		}
	}
}

// flush persists the current percent to the bound document. Forced flushes
// (lifecycle wake, Close) always write so component boundaries stay sharp;
// ticker flushes skip when percent is unchanged since the last successful
// write.
func (s *progressSink) flush(ctx context.Context, force bool) {
	s.mu.Lock()
	docID := s.docID
	s.mu.Unlock()
	if docID == "" {
		return
	}
	p := s.progress.Percent()
	if !force && p == s.lastFlushed {
		return
	}
	if err := s.docSvc.UpdateRunState(ctx, docID, p); err != nil {
		common.Warn(fmt.Sprintf("progressSink: flush progress for document %s failed: %v", docID, err))
		return
	}
	s.lastFlushed = p
}

func (s *progressSink) bindDocument(docID string) {
	s.mu.Lock()
	if s.docID == "" {
		s.docID = docID
	}
	s.mu.Unlock()
}

func (s *progressSink) wakeFlusher() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *progressSink) OnComponentTotal(ctx context.Context, taskID string, total int) {
	s.progress.SetTotal(total)
	if err := s.taskSvc.UpdateComponentTotal(ctx, taskID, total); err != nil {
		common.Error(fmt.Sprintf("progressSink: update component_total for task %s failed: %v", taskID, err), err)
	}
}

func (s *progressSink) OnComponentProgress(ctx context.Context, ev pipeline.ProgressEvent) {
	if err := s.taskSvc.RecordLifecycle(ctx, s.pipelineLogID, ev.TaskID, ev.Component, ev.Phase, ev.Message); err != nil {
		common.Error(fmt.Sprintf("progressSink: record component progress for task %s failed: %v", ev.TaskID, err), err)
	}
	if ev.Phase == int(agentruntime.PhaseExit) {
		s.progress.MarkDone(ev.Component)
	}
	if ev.DocumentID != "" {
		s.bindDocument(ev.DocumentID)
	}
	s.wakeFlusher()
}

// OnComponentFraction records an in-flight component's 0..1 completion
// fraction (pages parsed, chunks embedded). It deliberately does not wake the
// flusher: fraction reports are high-frequency and the ticker coalesces them.
// The pipeline reaches this method through an optional-interface assertion,
// mirroring detailedProgressSink.
func (s *progressSink) OnComponentFraction(_ context.Context, component string, frac float64) {
	s.progress.SetFrac(component, frac)
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
