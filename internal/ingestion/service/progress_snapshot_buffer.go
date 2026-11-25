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
	"sync"
)

// snapshotWriter is the subset of the progress store used by the sink's
// buffered writer. Injecting an interface keeps the emit path testable.
type snapshotWriter interface {
	Update(ctx context.Context, id, docID, datasetID, component, unit string, current, total uint32) bool
	SetPhase(ctx context.Context, id, docID, datasetID, component, phase string) bool
}

type unitRef struct {
	component, unit string
}

type unitPosition struct {
	current, total uint32
}

// progressSnapshotBuffer coalesces progress-detail events in memory so the
// emit path (which runs under progressSink.emitMu) never performs KV I/O.
// A single flusher goroutine drains pending updates on a ticker and on Close.
type progressSnapshotBuffer struct {
	mu        sync.Mutex
	writer    snapshotWriter
	id        string
	docID     string
	datasetID string
	details   map[unitRef]unitPosition
	phases    map[string]string
}

func newProgressSnapshotBuffer(writer snapshotWriter, id, docID, datasetID string) *progressSnapshotBuffer {
	return &progressSnapshotBuffer{
		writer:    writer,
		id:        id,
		docID:     docID,
		datasetID: datasetID,
		details:   make(map[unitRef]unitPosition),
		phases:    make(map[string]string),
	}
}

// detail records the newest position for a (component, unit). Out-of-order
// callbacks from parallel workers must never regress a stored position.
func (b *progressSnapshotBuffer) detail(component, unit string, current, total uint32) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ref := unitRef{component, unit}
	if prev, ok := b.details[ref]; ok && current <= prev.current {
		return
	}
	b.details[ref] = unitPosition{current, total}
}

// phase records the newest lifecycle phase for a component.
func (b *progressSnapshotBuffer) phase(component, phase string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.phases[component] = phase
}

// flush drains pending updates to the store. It holds the buffer lock only
// while copying the pending maps; the KV writes happen after releasing it, so
// emitters are never blocked behind storage I/O.
func (b *progressSnapshotBuffer) flush(ctx context.Context) {
	b.mu.Lock()
	details := b.details
	phases := b.phases
	b.details = make(map[unitRef]unitPosition)
	b.phases = make(map[string]string)
	b.mu.Unlock()

	if len(details) == 0 && len(phases) == 0 {
		return
	}
	for ref, pos := range details {
		b.writer.Update(ctx, b.id, b.docID, b.datasetID, ref.component, ref.unit, pos.current, pos.total)
	}
	for component, phase := range phases {
		b.writer.SetPhase(ctx, b.id, b.docID, b.datasetID, component, phase)
	}
}

// installProgressWriter attaches a buffered snapshot writer to the sink. It
// must be called before progress events begin.
func (s *progressSink) installProgressWriter(writer snapshotWriter, id, docID, datasetID string) {
	s.snapshotBuf.Store(newProgressSnapshotBuffer(writer, id, docID, datasetID))
}

func (s *progressSink) flushSnapshot(ctx context.Context) {
	if buf := s.snapshotBuf.Load(); buf != nil {
		buf.flush(ctx)
	}
}
