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
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0)

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
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0)
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
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0)
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
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusDone)), snap, 0)
	if resp.Stage != "Tokenizer" || resp.Unit != "chunk:final" {
		t.Fatalf("stage/unit = %q/%q, want Tokenizer/chunk:final", resp.Stage, resp.Unit)
	}
}

func TestMergeProgressFallsBackWhenSnapshotMissing(t *testing.T) {
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), nil, 0)
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
	// Adversarial: KV key exists but no stage ever reported a position.
	snap := testSnapshot(progressstore.StatusRunning,
		progressstore.Stage{Name: "File", Phase: "running", Units: map[string]progressstore.UnitProgress{}})
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusRunning)), snap, 0)
	if !resp.Fallback {
		t.Fatal("snapshot without an active unit must mark fallback, not show 0/0")
	}
}

func TestMergeProgressTerminalCarriesChunkCount(t *testing.T) {
	snap := testSnapshot(progressstore.StatusCompleted,
		progressstore.Stage{Name: "Tokenizer", Phase: "done", Active: "chunk:embed", Units: map[string]progressstore.UnitProgress{
			"chunk:embed": {Current: 5047, Total: 5047},
		}})
	resp := mergeProgressSnapshot(testRun(string(entity.TaskStatusDone)), snap, 5047)
	if resp.Phase != progressstore.StatusCompleted || resp.ChunkNum != 5047 {
		t.Fatalf("phase/chunkNum = %q/%d", resp.Phase, resp.ChunkNum)
	}
	if resp.Status != string(entity.TaskStatusDone) {
		t.Fatalf("status = %q, want authoritative terminal status", resp.Status)
	}
}
