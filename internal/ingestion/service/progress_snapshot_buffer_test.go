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
	"testing"

	servicepkg "ragflow/internal/service"
)

type recordedUpdate struct {
	attemptID       string
	component, unit string
	current, total  uint32
}

type recordedPhase struct {
	attemptID        string
	component, phase string
}

type fakeSnapshotWriter struct {
	updates []recordedUpdate
	phases  []recordedPhase
}

func (f *fakeSnapshotWriter) UpdateAttempt(_ context.Context, _, _, _, attemptID, component, unit string, current, total uint32) bool {
	f.updates = append(f.updates, recordedUpdate{attemptID, component, unit, current, total})
	return true
}

func (f *fakeSnapshotWriter) SetPhaseAttempt(_ context.Context, _, _, _, attemptID, component, phase string) bool {
	f.phases = append(f.phases, recordedPhase{attemptID, component, phase})
	return true
}

func TestSnapshotBufferCoalescesBeforeFlush(t *testing.T) {
	w := &fakeSnapshotWriter{}
	buf := newProgressSnapshotBuffer(w, "run1", "doc1", "ds1", "attempt-1")

	buf.detail("Parser", "page:parse", 1, 100)
	buf.detail("Parser", "page:parse", 5, 100)
	buf.detail("Parser", "page:parse", 3, 100) // stale must not regress
	buf.detail("Tokenizer", "chunk:embed", 2, 10)
	buf.phase("Parser", "running")
	buf.phase("Parser", "done")

	// Emitting must be pure in-memory work: no KV round-trip under emitMu.
	if len(w.updates) != 0 || len(w.phases) != 0 {
		t.Fatalf("buffer wrote to storage before flush: updates=%v phases=%v", w.updates, w.phases)
	}

	buf.flush(context.Background())

	if len(w.updates) != 2 {
		t.Fatalf("updates = %+v, want 2 (one per stage/unit)", w.updates)
	}
	got := map[string]uint32{}
	for _, u := range w.updates {
		got[u.component+"/"+u.unit] = u.current
	}
	if got["Parser/page:parse"] != 5 {
		t.Errorf("Parser position = %d, want latest 5", got["Parser/page:parse"])
	}
	if got["Tokenizer/chunk:embed"] != 2 {
		t.Errorf("Tokenizer position = %d, want 2", got["Tokenizer/chunk:embed"])
	}
	if len(w.phases) != 1 || w.phases[0].component != "Parser" || w.phases[0].phase != "done" {
		t.Fatalf("phases = %+v, want Parser/done only", w.phases)
	}
}

func TestSnapshotBufferForwardsAttemptToken(t *testing.T) {
	// Every drained write must carry the execution that produced it; a stale
	// execution's token reaching storage would otherwise fence out nothing.
	w := &fakeSnapshotWriter{}
	buf := newProgressSnapshotBuffer(w, "run1", "doc1", "ds1", "attempt-9")

	buf.detail("Parser", "page:parse", 5, 100)
	buf.phase("Parser", "done")
	buf.flush(context.Background())

	if len(w.updates) != 1 || w.updates[0].attemptID != "attempt-9" {
		t.Fatalf("updates = %+v, want one write tagged attempt-9", w.updates)
	}
	if len(w.phases) != 1 || w.phases[0].attemptID != "attempt-9" {
		t.Fatalf("phases = %+v, want one write tagged attempt-9", w.phases)
	}
}

func TestSnapshotBufferClearsAfterFlush(t *testing.T) {
	w := &fakeSnapshotWriter{}
	buf := newProgressSnapshotBuffer(w, "run1", "doc1", "ds1", "attempt-1")
	buf.detail("Parser", "page:parse", 5, 100)

	buf.flush(context.Background())
	buf.flush(context.Background()) // nothing pending -> no extra writes

	if len(w.updates) != 1 {
		t.Fatalf("updates = %+v, want exactly 1 after two flushes", w.updates)
	}
}

func TestProgressSinkBufferedDetailFlushesOnClose(t *testing.T) {
	// The sink must route detail events through the buffer and flush pending
	// positions on Close, without doing KV work inside emitMu.
	w := &fakeSnapshotWriter{}
	sink := newProgressSink(context.Background(), servicepkg.NewIngestionTaskService(), "run1")
	sink.installProgressWriter(w, "run1", "doc1", "ds1", "attempt-1")

	sink.OnProgressDetail(context.Background(), "Parser", "page:parse", 5, 100)
	if len(w.updates) != 0 {
		t.Fatalf("emit performed a synchronous store write: %+v", w.updates)
	}

	sink.Close()
	if len(w.updates) != 1 {
		t.Fatalf("updates after Close = %+v, want 1 buffered write", w.updates)
	}
	if w.updates[0].current != 5 || w.updates[0].unit != "page:parse" {
		t.Errorf("flushed update = %+v, want Parser/page:parse 5/100", w.updates[0])
	}
}

// BenchmarkSnapshotBufferDetail guards the emit path: it must stay an
// in-memory coalescing write, not a synchronous KV round-trip per event.
func BenchmarkSnapshotBufferDetail(b *testing.B) {
	buf := newProgressSnapshotBuffer(&fakeSnapshotWriter{}, "run1", "doc1", "ds1", "attempt-1")
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.detail("Parser", "page:parse", uint32(i%1000+1), 1000)
	}
}
