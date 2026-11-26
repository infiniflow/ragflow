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
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/progressstore"
)

// progressBeginner opens the progress window for one worker execution. It is
// an interface so the claim-time wiring can be tested without a live KV.
type progressBeginner interface {
	BeginAttempt(ctx context.Context, id, docID, datasetID, attemptID string, startedAt time.Time) bool
}

// phaseLabel maps runtime.ProgressPhase (enter/exit/error) to the vocabulary
// stored in the progress snapshot. An out-of-range value yields "" so the
// store skips the write instead of persisting a corrupt phase.
func phaseLabel(phase uint8) string {
	switch phase {
	case 0:
		return progressstore.PhaseRunning
	case 1:
		return progressstore.PhaseDone
	case 2:
		return progressstore.PhaseFailed
	default:
		return ""
	}
}

// progressTerminalStatus maps an authoritative task status to the snapshot's
// terminal status. Non-terminal statuses yield "" so a snapshot is never
// prematurely frozen.
func progressTerminalStatus(status string) string {
	switch status {
	case string(entity.TaskStatusDone):
		return progressstore.StatusCompleted
	case string(entity.TaskStatusFail):
		return progressstore.StatusFailed
	case string(entity.TaskStatusCancel):
		return progressstore.StatusStopped
	default:
		return ""
	}
}

// beginProgressAttempt opens a fresh KV progress window for the execution
// identified by the attempt token on ctx. It must run after the worker claims
// the task and before any component can emit progress, so positions from the
// previous (crashed) execution can never leak into this one.
func (e *Ingestor) beginProgressAttempt(ctx context.Context, task *entity.IngestionTask, beginner progressBeginner) {
	if e == nil || task == nil || task.PipelineLogID == nil || beginner == nil {
		return
	}
	attemptID := progressstore.AttemptFromContext(ctx)
	if attemptID == "" {
		return
	}
	beginner.BeginAttempt(ctx, *task.PipelineLogID, task.DocumentID, task.DatasetID, attemptID, time.Now().UTC())
}

// markProgressTerminal flips the run's snapshot to terminal. Best-effort:
// when Kvrocks is unavailable the authoritative MySQL run state still drives
// the UI, so a failure here is only logged by progressstore.
func (e *Ingestor) markProgressTerminal(ctx context.Context, task *entity.IngestionTask, status string) {
	if e == nil || e.deps.KV == nil || task == nil || task.PipelineLogID == nil {
		return
	}
	label := progressTerminalStatus(status)
	if label == "" {
		return
	}
	progressstore.New(e.deps.KV).MarkTerminal(ctx, *task.PipelineLogID, task.DocumentID, task.DatasetID, label)
}
