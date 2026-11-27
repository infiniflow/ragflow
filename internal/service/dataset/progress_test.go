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

package dataset

import (
	"testing"
	"time"

	"ragflow/internal/entity"
	"ragflow/internal/progressstore"
)

func testRun(status string) *entity.PipelineOperationLog {
	return &entity.PipelineOperationLog{OperationStatus: status}
}

func testSnapshot(status string, stages ...progressstore.Stage) *progressstore.Snapshot {
	return &progressstore.Snapshot{Status: status, Stages: stages}
}

func TestMergeProgressUsesActiveUnit(t *testing.T) {
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Extractor", Phase: "running", Active: "chunk:tags", Units: map[string]progressstore.UnitProgress{
			"chunk:keywords": {Current: 5047, Total: 5047},
			"chunk:tags":     {Current: 3123, Total: 5047},
		}})
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0, nil)

	if resp.Stage != "Extractor" || resp.Unit != "chunk:tags" {
		t.Fatalf("stage/unit = %q/%q, want Extractor/chunk:tags", resp.Stage, resp.Unit)
	}
	if resp.Current != 3123 || resp.Total != 5047 {
		t.Fatalf("position = %d/%d, want 3123/5047", resp.Current, resp.Total)
	}
	if resp.Percent < 61.8 || resp.Percent > 62.0 {
		t.Fatalf("percent = %v, want ~62", resp.Percent)
	}
	if resp.Fallback {
		t.Fatal("fallback must be false when snapshot has an active unit")
	}
}

func TestMergeProgressSelectsRunningStageNotCompletedOne(t *testing.T) {
	// Adversarial: a completed stage still carries its Active unit because the
	// store never clears it. The summary must not stop there while a later
	// stage is actually running.
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Parser", Phase: "done", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 10, Total: 10},
		}},
		progressstore.Stage{Name: "Tokenizer", Phase: "running", Active: "chunk:final", Units: map[string]progressstore.UnitProgress{
			"chunk:final": {Current: 2, Total: 100},
		}},
	)
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0, nil)
	if resp.Stage != "Tokenizer" || resp.Unit != "chunk:final" {
		t.Fatalf("stage/unit = %q/%q, want Tokenizer/chunk:final (completed stage must not shadow the running one)", resp.Stage, resp.Unit)
	}
	if resp.Current != 2 || resp.Total != 100 {
		t.Fatalf("position = %d/%d, want 2/100", resp.Current, resp.Total)
	}
}

func TestMergeProgressSelectsLatestRunningStage(t *testing.T) {
	// Adversarial: multiple non-terminal stages; the most recently reported
	// stage supplies the summary, not the first one in the slice.
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Parser", Phase: "running", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 10, Total: 100},
		}},
		progressstore.Stage{Name: "Extractor", Phase: "running", Active: "chunk:tags", Units: map[string]progressstore.UnitProgress{
			"chunk:tags": {Current: 7, Total: 50},
		}},
	)
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0, nil)
	if resp.Stage != "Extractor" || resp.Unit != "chunk:tags" {
		t.Fatalf("stage/unit = %q/%q, want Extractor/chunk:tags", resp.Stage, resp.Unit)
	}
}

func TestMergeProgressFallsBackToLastStageWhenAllDone(t *testing.T) {
	// At settlement no stage is running; still report the latest position so
	// API consumers do not see a fabricated 0/0.
	snap := testSnapshot(progressstore.StatusCompleted,
		progressstore.Stage{Name: "Parser", Phase: "done", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 10, Total: 10},
		}},
		progressstore.Stage{Name: "Tokenizer", Phase: "done", Active: "chunk:final", Units: map[string]progressstore.UnitProgress{
			"chunk:final": {Current: 100, Total: 100},
		}},
	)
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusDone)), snap, 0, nil)
	if resp.Stage != "Tokenizer" || resp.Unit != "chunk:final" {
		t.Fatalf("stage/unit = %q/%q, want Tokenizer/chunk:final", resp.Stage, resp.Unit)
	}
}

func TestMergeProgressFallsBackWhenSnapshotMissing(t *testing.T) {
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), nil, 0, nil)
	if !resp.Fallback {
		t.Fatal("missing snapshot must mark fallback")
	}
	if resp.Stage != "" || resp.Unit != "" || resp.Total != 0 {
		t.Fatalf("fallback response must not fabricate a position: %+v", resp)
	}
	if resp.Status != string(entity.TaskStatusRunning) {
		t.Fatalf("status = %q, want authoritative run status", resp.Status)
	}
}

func TestMergeProgressFallsBackWhenSnapshotHasNoActiveUnit(t *testing.T) {
	// Adversarial: KV key exists but no stage has been reported at all. A stage
	// that is running without a unit still carries identity and is surfaced (see
	// TestMergeProgressPrefersRunningStageWithoutUnit); here there is nothing to
	// show, so the response must be marked fallback rather than fabricate 0/0.
	snap := testSnapshot(progressstore.StatusRunning)
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0, nil)
	if !resp.Fallback {
		t.Fatal("snapshot with no stage at all must mark fallback, not show 0/0")
	}
}

func TestMergeProgressFallsBackForLegacySnapshot(t *testing.T) {
	// Adversarial: snapshots written before attempts existed have no identity.
	// Their positions belong to an unknown execution and must not be shown.
	started := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	run := testRun(string(entity.TaskStatusRunning))
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Parser", Phase: "running", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 250, Total: 500},
		}})

	resp := mergeProgressSnapshot(run, snap, 0, &started)
	if !resp.Fallback {
		t.Fatal("legacy snapshot without attempt identity must mark fallback")
	}
	if resp.Current != 0 || resp.Total != 0 || resp.Stage != "" {
		t.Fatalf("fallback response must not expose stale position: %+v", resp)
	}
}

func TestMergeProgressFallsBackWhenSnapshotPredatesRun(t *testing.T) {
	// Adversarial: a run started again while the KV key still holds the
	// previous execution's snapshot. The old timestamp makes it untrusted.
	started := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	run := testRun(string(entity.TaskStatusRunning))
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Parser", Phase: "running", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 250, Total: 500},
		}})
	snap.AttemptID = "attempt-old"
	snap.AttemptStartedAt = started.Add(-time.Minute)

	resp := mergeProgressSnapshot(run, snap, 0, &started)
	if !resp.Fallback {
		t.Fatal("snapshot from an earlier execution must mark fallback")
	}
	if resp.Current != 0 || resp.Stage != "" {
		t.Fatalf("stale snapshot leaked into response: %+v", resp)
	}
}

func TestMergeProgressUsesCurrentAttemptSnapshot(t *testing.T) {
	started := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	run := testRun(string(entity.TaskStatusRunning))
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Parser", Phase: "running", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 250, Total: 500},
		}})
	snap.AttemptID = "attempt-current"
	snap.AttemptStartedAt = started.Add(time.Second)

	resp := mergeProgressSnapshot(run, snap, 0, &started)
	if resp.Fallback {
		t.Fatal("snapshot from the current attempt must be trusted")
	}
	if resp.Current != 250 || resp.Total != 500 {
		t.Fatalf("position = %d/%d, want 250/500", resp.Current, resp.Total)
	}
}

func TestMergeProgressTerminalCarriesChunkCount(t *testing.T) {
	snap := testSnapshot(progressstore.StatusCompleted,
		progressstore.Stage{Name: "Tokenizer", Phase: "done", Active: "chunk:embed", Units: map[string]progressstore.UnitProgress{
			"chunk:embed": {Current: 5047, Total: 5047},
		}})
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusDone)), snap, 5047, nil)
	if resp.Phase != progressstore.StatusCompleted || resp.ChunkNum != 5047 {
		t.Fatalf("phase/chunkNum = %q/%d", resp.Phase, resp.ChunkNum)
	}
	if resp.Status != string(entity.TaskStatusDone) {
		t.Fatalf("status = %q, want authoritative terminal status", resp.Status)
	}
}

// Adversarial: a running stage that has not reported its first unit yet must
// still own the summary. A wrong implementation falls back to the last done
// stage and shows its stale 100% (the exact freeze reported by users).
func TestMergeProgressPrefersRunningStageWithoutUnit(t *testing.T) {
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "File", Phase: progressstore.PhaseDone},
		progressstore.Stage{Name: "Parser:HipSignsRhyme", Phase: progressstore.PhaseDone, Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 3266, Total: 3266},
		}},
		progressstore.Stage{Name: "GeneralChunker:SixApplesFall", Phase: progressstore.PhaseRunning},
	)

	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0, nil)
	if resp.Stage != "GeneralChunker:SixApplesFall" {
		t.Fatalf("stage = %q, want GeneralChunker:SixApplesFall (running stage without a unit must not fall back to a done stage)", resp.Stage)
	}
	// The response's phase is the snapshot STATUS (uppercase), not the stage's
	// lifecycle label; mixing the two breaks the API contract.
	if resp.Phase != progressstore.StatusRunning {
		t.Errorf("phase = %q, want %q", resp.Phase, progressstore.StatusRunning)
	}
	// No unit has been reported yet, so there is no position to display. The
	// response must not fabricate 0/0; mark fallback while keeping Stage set.
	if !resp.Fallback {
		t.Error("running stage without a unit must be marked fallback (no fabricated 0/0)")
	}
}

// Guard: a running stage that DOES carry a unit keeps the established status
// phase and is not marked fallback.
func TestMergeProgressRunningStageWithUnitKeepsStatusPhase(t *testing.T) {
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Tokenizer", Phase: progressstore.PhaseRunning, Active: "chunk:embed", Units: map[string]progressstore.UnitProgress{
			"chunk:embed": {Current: 20, Total: 100},
		}},
	)
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0, nil)
	if resp.Phase != progressstore.StatusRunning {
		t.Errorf("phase = %q, want %q", resp.Phase, progressstore.StatusRunning)
	}
	if resp.Fallback {
		t.Error("running stage with a unit must not be marked fallback")
	}
}

// With a running stage that does carry an active unit, the existing behaviour
// (that stage's unit wins) must be preserved.
func TestMergeProgressRunningStageWithUnitStillWins(t *testing.T) {
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Parser", Phase: progressstore.PhaseDone, Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 10, Total: 10},
		}},
		progressstore.Stage{Name: "GeneralChunker", Phase: progressstore.PhaseRunning, Active: "image:crop", Units: map[string]progressstore.UnitProgress{
			"image:crop": {Current: 120, Total: 800},
		}},
	)
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0, nil)
	if resp.Stage != "GeneralChunker" || resp.Unit != "image:crop" {
		t.Fatalf("stage/unit = %q/%q, want GeneralChunker/image:crop", resp.Stage, resp.Unit)
	}
	if resp.Current != 120 || resp.Total != 800 {
		t.Fatalf("position = %d/%d, want 120/800", resp.Current, resp.Total)
	}
}

// A running run must report a live elapsed time derived from its begin time;
// ProcessDuration is only written at settlement and stays 0 while running.
func TestMergeProgressRunningUsesLiveElapsed(t *testing.T) {
	run := testRun(string(entity.TaskStatusRunning))
	run.ProcessDuration = 0
	begin := time.Now().Add(-90 * time.Second)
	run.ProcessBeginAt = &begin

	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "Parser", Phase: progressstore.PhaseRunning, Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 10, Total: 100},
		}})
	resp := mergeProgressSnapshot(run, snap, 0, nil)

	if resp.DurationS < 89 || resp.DurationS > 91 {
		t.Fatalf("running duration = %v, want ~90s (live elapsed), got %+v", resp.DurationS, resp)
	}
}

// Adversarial: a terminal run keeps its persisted duration even though its
// begin time is old. Recomputing from begin time here would misreport the
// settled duration.
func TestMergeProgressTerminalKeepsPersistedDuration(t *testing.T) {
	for _, status := range []string{
		string(entity.TaskStatusDone),
		string(entity.TaskStatusFail),
		string(entity.TaskStatusCancel),
		"DONE", "FAIL", "CANCEL",
	} {
		run := testRun(status)
		run.ProcessDuration = 42
		begin := time.Now().Add(-90 * time.Second)
		run.ProcessBeginAt = &begin

		resp := mergeProgressSnapshot(run, testSnapshot(progressstore.StatusCompleted), 0, nil)
		if resp.DurationS != 42 {
			t.Errorf("status %q duration = %v, want persisted 42", status, resp.DurationS)
		}
	}
}

// Adversarial: a running run with no begin time cannot compute elapsed; it
// must stay at 0 and not panic.
func TestMergeProgressRunningWithoutBeginStaysZero(t *testing.T) {
	run := testRun(string(entity.TaskStatusRunning))
	run.ProcessDuration = 0
	run.ProcessBeginAt = nil

	resp := mergeProgressSnapshot(run, testSnapshot(progressstore.StatusRunning), 0, nil)
	if resp.DurationS != 0 {
		t.Fatalf("duration = %v, want 0 when begin time is missing", resp.DurationS)
	}
}

// Adversarial: a begin time in the future (clock skew / bad data) must not
// produce a negative elapsed, which the UI renders as "—".
func TestMergeProgressFutureBeginNeverNegative(t *testing.T) {
	run := testRun(string(entity.TaskStatusRunning))
	run.ProcessDuration = 0
	begin := time.Now().Add(30 * time.Second)
	run.ProcessBeginAt = &begin

	resp := mergeProgressSnapshot(run, testSnapshot(progressstore.StatusRunning), 0, nil)
	if resp.DurationS < 0 {
		t.Fatalf("duration = %v, want non-negative", resp.DurationS)
	}
}
