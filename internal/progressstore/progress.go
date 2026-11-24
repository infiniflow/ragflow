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

// Package progressstore persists a per-run ingestion progress snapshot in
// Kvrocks. It is shared by the ingestor (writer) and the API server (reader).
//
// The snapshot is a running-state convenience, not a system of record: the
// authoritative task status lives in MySQL. A missing or expired key is a
// normal condition and must degrade to that authoritative state.
package progressstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"ragflow/internal/engine/kvrocks"
)

const (
	StatusRunning   = "RUNNING"
	StatusCompleted = "COMPLETED"
	StatusFailed    = "FAILED"
	StatusStopped   = "STOPPED"

	snapshotTTL = 24 * time.Hour
)

// Phase labels stored in Stage.Phase, written on component lifecycle events.
const (
	PhaseRunning = "running"
	PhaseDone    = "done"
	PhaseFailed  = "failed"
)

// UnitProgress is one stage's discrete position for a named unit.
type UnitProgress struct {
	Current uint32 `json:"current"`
	Total   uint32 `json:"total"`
}

// Stage is one canvas component's progress. Units is a map because a single
// component (Extractor, Compiler) runs several sequential phases, each with
// its own counter restarting from zero — storing one position per unit keeps
// a later phase from overwriting an earlier one.
type Stage struct {
	Name   string                  `json:"name"`
	Phase  string                  `json:"phase,omitempty"`
	Active string                  `json:"active,omitempty"`
	Units  map[string]UnitProgress `json:"units,omitempty"`
}

// Snapshot is the whole run's latest progress state.
type Snapshot struct {
	PipelineLogID string    `json:"pipeline_log_id"`
	DocID         string    `json:"doc_id"`
	DatasetID     string    `json:"dataset_id"`
	Status        string    `json:"status"`
	UpdatedAt     time.Time `json:"updated_at"`
	Stages        []Stage   `json:"stages"`
}

// Key returns the Kvrocks key for one run. pipeline_log_id is the immutable
// run identity; task ids are reused across retries and must not be used.
func Key(pipelineLogID string) string { return "ingest:progress:" + pipelineLogID }

type kvClient interface {
	// Get distinguishes a confirmed-absent key (redis.Nil) from a failed
	// read, which the bool-returning GetObj cannot express.
	Get(ctx context.Context, key string) (string, error)
	SetObj(ctx context.Context, key string, obj any, exp time.Duration) bool
}

// Store reads and writes progress snapshots through a Kvrocks client.
type Store struct{ kv kvClient }

// New wraps an injected client. A nil client is tolerated: every method
// becomes a no-op failure, letting callers degrade without a nil panic.
func New(kv *kvrocks.Client) *Store {
	if kv == nil {
		return &Store{}
	}
	return &Store{kv: kv}
}

// Default resolves the process-wide client. It is intended for the API
// server, where the read path is not threaded through ingestion.Deps.
func Default() *Store { return New(kvrocks.Get()) }

type loadState uint8

const (
	loadMissing loadState = iota // key confirmed absent
	loadFound                    // snapshot decoded
	loadFailed                   // read or decode failed; never overwrite
)

func (s *Store) loadState(ctx context.Context, id string) (*Snapshot, loadState) {
	if s == nil || s.kv == nil || id == "" {
		return nil, loadMissing
	}
	raw, err := s.kv.Get(ctx, Key(id))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, loadMissing
		}
		return nil, loadFailed
	}
	// kvrocks.Client.Get swallows redis.Nil and reports an absent key as an
	// empty value with a nil error; a stored snapshot is never empty JSON.
	if raw == "" {
		return nil, loadMissing
	}
	var snap Snapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return nil, loadFailed
	}
	return &snap, loadFound
}

func (s *Store) load(ctx context.Context, id string) (*Snapshot, bool) {
	snap, state := s.loadState(ctx, id)
	return snap, state == loadFound
}

func (s *Store) save(ctx context.Context, snap *Snapshot) bool {
	if s == nil || s.kv == nil || snap == nil {
		return false
	}
	// A terminal settlement may land between this writer's load and save
	// (lifecycle callbacks and detail callbacks use different locks). Re-read
	// the current status and preserve a terminal one so a late detail write
	// cannot revert a finished run to RUNNING.
	current, state := s.loadState(ctx, snap.PipelineLogID)
	if state == loadFailed {
		return false // never overwrite a snapshot we could not read
	}
	if state == loadFound && isTerminal(current.Status) {
		snap.Status = current.Status
	}
	snap.UpdatedAt = time.Now().UTC()
	return s.kv.SetObj(ctx, Key(snap.PipelineLogID), snap, snapshotTTL)
}

func newSnapshot(id, docID, datasetID string) *Snapshot {
	return &Snapshot{
		PipelineLogID: id,
		DocID:         docID,
		DatasetID:     datasetID,
		Status:        StatusRunning,
		UpdatedAt:     time.Now().UTC(),
	}
}

func stageByName(snap *Snapshot, name string) *Stage {
	for i := range snap.Stages {
		if snap.Stages[i].Name == name {
			return &snap.Stages[i]
		}
	}
	snap.Stages = append(snap.Stages, Stage{Name: name, Units: map[string]UnitProgress{}})
	return &snap.Stages[len(snap.Stages)-1]
}

func isTerminal(status string) bool {
	switch status {
	case StatusCompleted, StatusFailed, StatusStopped:
		return true
	default:
		return false
	}
}

// Update records a component's discrete position. It is monotonic per
// (component, unit): out-of-order callbacks from parallel futures settle in
// arbitrary order, and a stale lower value must never pull the snapshot back.
// Equal positions are skipped so repeated callbacks do not rewrite the key.
func (s *Store) Update(ctx context.Context, id, docID, datasetID, component, unit string, current, total uint32) bool {
	if s == nil || s.kv == nil || id == "" || component == "" || unit == "" {
		return false
	}
	snap, state := s.loadState(ctx, id)
	switch state {
	case loadMissing:
		snap = newSnapshot(id, docID, datasetID)
	case loadFailed:
		return false // a failed read must not create a fresh, lossy snapshot
	case loadFound:
		if isTerminal(snap.Status) {
			return false
		}
	}
	stage := stageByName(snap, component)
	if stage.Units == nil {
		stage.Units = map[string]UnitProgress{}
	}
	if prev, exists := stage.Units[unit]; exists && current <= prev.Current {
		return false // stale or duplicate — no write applied
	}
	stage.Units[unit] = UnitProgress{Current: current, Total: total}
	stage.Active = unit
	return s.save(ctx, snap)
}

// SetPhase records a component lifecycle phase (running/done/failed).
func (s *Store) SetPhase(ctx context.Context, id, docID, datasetID, component, phase string) bool {
	if s == nil || s.kv == nil || id == "" || component == "" || phase == "" {
		return false
	}
	snap, state := s.loadState(ctx, id)
	switch state {
	case loadMissing:
		snap = newSnapshot(id, docID, datasetID)
	case loadFailed:
		return false
	case loadFound:
		if isTerminal(snap.Status) {
			return false
		}
	}
	stage := stageByName(snap, component)
	if stage.Phase == phase {
		return true
	}
	stage.Phase = phase
	return s.save(ctx, snap)
}

// MarkTerminal flips the snapshot to a terminal status exactly once. A second
// call with a different status is ignored: the first settlement wins, matching
// the authoritative run row's one-way status transition.
func (s *Store) MarkTerminal(ctx context.Context, id, docID, datasetID, status string) bool {
	if s == nil || s.kv == nil || id == "" || !isTerminal(status) {
		return false
	}
	// Terminal settlement runs on cancel/timeout paths whose context is
	// already cancelled. Detach cancellation while retaining context values
	// so the snapshot write still lands.
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = context.WithoutCancel(ctx)
	snap, state := s.loadState(ctx, id)
	switch state {
	case loadMissing:
		snap = newSnapshot(id, docID, datasetID)
	case loadFailed:
		return false
	case loadFound:
		if isTerminal(snap.Status) {
			return true
		}
	}
	snap.Status = status
	return s.save(ctx, snap)
}

// Load returns the latest snapshot, or (nil, false) when absent/unavailable.
func (s *Store) Load(ctx context.Context, id string) (*Snapshot, bool) {
	return s.load(ctx, id)
}
