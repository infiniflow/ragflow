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

package pipeline

import (
	"context"
	"sync"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// ProgressSink persists per-stage checkpoint state. Production
// writes go through TaskLogSink; tests use TestSink (in-memory
// recording).
//
// The Persist signature is intentionally narrow: it takes the
// taskID + a flat map[string]any. The pipeline runner constructs
// the map; the sink is responsible for serialising it the way
// the underlying store expects. TaskLogSink writes through
// IngestionTaskLogDAO.Create, which expects an entity.IngestionTaskLog
// with a JSONMap checkpoint field.
type ProgressSink interface {
	Persist(ctx context.Context, taskID string, checkpoint map[string]any) error
}

// GoroutineStatus records one worker's outcome for the
// work_unit_status[] entry of the checkpoint. Pages / OutputRef are optional — File emits
// OutputRef, Parser emits Pages, Chunker / Tokenizer /
// Extractor emit a single (no-pages, no-output-ref) "done".
type GoroutineStatus struct {
	Goroutine int    `json:"goroutine"`
	Status    string `json:"status"` // "done" | "failed"
	OutputRef string `json:"output_ref,omitempty"`
	Pages     []int  `json:"pages,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TaskLogSink is the production sink. Each call to Persist
// inserts a new IngestionTaskLog row (mirrors the Python
// REDIS_CONN.set_obj(log_key, obj) cadence — every pipeline
// completion checkpoint writes a fresh row).
type TaskLogSink struct {
	dao *dao.IngestionTaskLogDAO
}

// NewTaskLogSink wires a TaskLogSink against the production
// IngestionTaskLogDAO. Tests should not call this — use
// NewTestSink for in-memory recording.
func NewTaskLogSink() *TaskLogSink {
	return &TaskLogSink{dao: dao.NewIngestionTaskLogDAO()}
}

// Persist inserts a checkpoint row. The checkpoint map is
// copied into a fresh entity.JSONMap so the caller's mutations
// (the runner mutates the map after each stage) don't leak into
// the persisted record. We intentionally create a NEW log row
// per checkpoint rather than UPDATE so task-log history stays
// append-only and race-free.
func (s *TaskLogSink) Persist(ctx context.Context, taskID string, checkpoint map[string]any) error {
	if s == nil || s.dao == nil {
		return errNilSink
	}
	cp := make(entity.JSONMap, len(checkpoint))
	for k, v := range checkpoint {
		cp[k] = v
	}
	row := &entity.IngestionTaskLog{
		TaskID:     taskID,
		Checkpoint: cp,
	}
	// Honour caller cancellation: skip the insert if ctx is
	// already done. The runner uses ctx.Err() elsewhere to
	// surface cancellation; we don't add a second error path
	// here — the caller (Pipeline.Run) records the cancellation
	// in the in-memory map before calling Persist, so the
	// partially-populated row is still informative.
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.dao.Create(row)
}

// TestSink is the in-memory sink for unit tests. It records
// every Persist call in insertion order and is safe for
// concurrent use.
type TestSink struct {
	mu  sync.Mutex
	Row []TestSinkRow
}

// TestSinkRow is one recorded Persist call. TaskID is captured
// for tests that exercise multi-task scenarios; Checkpoint is
// the exact map the runner handed in.
type TestSinkRow struct {
	TaskID     string
	Checkpoint map[string]any
}

// NewTestSink returns a fresh in-memory sink.
func NewTestSink() *TestSink {
	return &TestSink{}
}

// Persist appends a row. The map is stored as-is (no copy) so
// tests that mutate the runner's in-memory checkpoint map
// observe the same mutations. If a test needs isolation, it
// should hand the sink a fresh map per call.
func (s *TestSink) Persist(ctx context.Context, taskID string, checkpoint map[string]any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Row = append(s.Row, TestSinkRow{TaskID: taskID, Checkpoint: checkpoint})
	return nil
}

// Snapshots returns a copy of the recorded rows. Tests that
// assert on insertion order read this; tests that assert on
// checkpoint contents read the rows' Checkpoint field directly.
func (s *TestSink) Snapshots() []TestSinkRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]TestSinkRow, len(s.Row))
	copy(out, s.Row)
	return out
}

// Last returns the most recent recorded row, or nil if none.
func (s *TestSink) Last() *TestSinkRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.Row) == 0 {
		return nil
	}
	return &s.Row[len(s.Row)-1]
}

// errNilSink is a defensive guard so a misconfigured TaskLogSink
// (nil DAO) doesn't silently drop checkpoints.
var errNilSink = &sinkError{Reason: "nil TaskLogSink"}
