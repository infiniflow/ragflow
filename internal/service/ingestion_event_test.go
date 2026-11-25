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
	"testing"

	"ragflow/internal/progressstore"
)

func testSnapshot(stages ...progressstore.Stage) *progressstore.Snapshot {
	return &progressstore.Snapshot{Status: progressstore.StatusRunning, Stages: stages}
}

func TestIngestionEventItemFromSnapshotUsesActiveUnit(t *testing.T) {
	snap := testSnapshot(
		progressstore.Stage{Name: "Parser", Phase: "done", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 3266, Total: 3266},
		}},
		progressstore.Stage{Name: "Extractor", Phase: "running", Active: "chunk:tags", Units: map[string]progressstore.UnitProgress{
			"chunk:keywords": {Current: 5047, Total: 5047},
			"chunk:tags":     {Current: 3123, Total: 5047},
		}},
	)

	item := IngestionEventItemFromSnapshot(snap)
	want := "Extractor · 标签提取 3123/5047"
	if item.Message != want {
		t.Fatalf("message = %q, want %q", item.Message, want)
	}
	if item.Component != "Extractor" || item.Phase != 0 {
		t.Fatalf("component/phase = %q/%d", item.Component, item.Phase)
	}
}

func TestIngestionEventItemFromSnapshotFallsBackWhenNoActiveUnit(t *testing.T) {
	// Adversarial: snapshot exists but no stage has an active unit — must not
	// fabricate a "0/0" message.
	snap := testSnapshot(
		progressstore.Stage{Name: "File", Phase: "running", Units: map[string]progressstore.UnitProgress{}},
	)
	item := IngestionEventItemFromSnapshot(snap)
	if item.Message != "" {
		t.Fatalf("message = %q, want empty", item.Message)
	}
}

func TestIngestionEventItemFromSnapshotNilAndTerminal(t *testing.T) {
	if item := IngestionEventItemFromSnapshot(nil); item.Message != "" {
		t.Fatalf("nil snapshot message = %q, want empty", item.Message)
	}

	// Terminal snapshot still derives from its last active stage, so the
	// document list shows a meaningful final position rather than a blank.
	snap := &progressstore.Snapshot{
		Status: progressstore.StatusCompleted,
		Stages: []progressstore.Stage{{Name: "Tokenizer", Phase: "done", Active: "chunk:embed", Units: map[string]progressstore.UnitProgress{
			"chunk:embed": {Current: 5047, Total: 5047},
		}}},
	}
	item := IngestionEventItemFromSnapshot(snap)
	if item.Message == "" {
		t.Fatal("terminal snapshot should still yield a message")
	}
}

func TestIngestionEventItemFromSnapshotUnknownUnitKeepsKey(t *testing.T) {
	snap := testSnapshot(progressstore.Stage{Name: "Compiler", Active: "exotic:unit", Units: map[string]progressstore.UnitProgress{
		"exotic:unit": {Current: 7, Total: 9},
	}})
	item := IngestionEventItemFromSnapshot(snap)
	if want := "Compiler · exotic:unit 7/9"; item.Message != want {
		t.Fatalf("message = %q, want %q", item.Message, want)
	}
}

func TestIngestionEventItemFromSnapshotMissingActivePosition(t *testing.T) {
	// Adversarial: active names a unit absent from Units — no fabricated 0/0.
	snap := testSnapshot(progressstore.Stage{Name: "Parser", Active: "page:parse", Units: map[string]progressstore.UnitProgress{
		"other": {Current: 1, Total: 2},
	}})
	item := IngestionEventItemFromSnapshot(snap)
	if item.Message != "" {
		t.Fatalf("message = %q, want empty for missing active position", item.Message)
	}
}

func TestIngestionEventItemFromSnapshotPrefersLatestFailedStage(t *testing.T) {
	// Adversarial: Parser finished and Extractor then failed, both retaining an
	// active unit. A forward scan would freeze on Parser and hide the failure.
	// The newest (failed) stage must win so the user sees where it stopped.
	snap := testSnapshot(
		progressstore.Stage{Name: "Parser", Phase: progressstore.PhaseDone, Active: "page:parse", Units: map[string]progressstore.UnitProgress{
			"page:parse": {Current: 10, Total: 10},
		}},
		progressstore.Stage{Name: "Extractor", Phase: progressstore.PhaseFailed, Active: "chunk:tags", Units: map[string]progressstore.UnitProgress{
			"chunk:tags": {Current: 3, Total: 10},
		}},
	)
	item := IngestionEventItemFromSnapshot(snap)
	if want := "Extractor · 标签提取 3/10"; item.Message != want {
		t.Fatalf("message = %q, want %q", item.Message, want)
	}
	if item.Component != "Extractor" {
		t.Fatalf("component = %q, want Extractor (the failed stage)", item.Component)
	}
}

func TestIngestionEventItemFromSnapshotPrefersLatestRunningStage(t *testing.T) {
	// Adversarial: two running stages both retain active units; the newest must
	// win, matching latestActiveStage in dataset/progress.go.
	snap := testSnapshot(
		progressstore.Stage{Name: "Extractor", Phase: progressstore.PhaseRunning, Active: "chunk:tags", Units: map[string]progressstore.UnitProgress{
			"chunk:tags": {Current: 5, Total: 10},
		}},
		progressstore.Stage{Name: "Tokenizer", Phase: progressstore.PhaseRunning, Active: "chunk:embed", Units: map[string]progressstore.UnitProgress{
			"chunk:embed": {Current: 7, Total: 10},
		}},
	)
	item := IngestionEventItemFromSnapshot(snap)
	if want := "Tokenizer · 向量化 7/10"; item.Message != want {
		t.Fatalf("message = %q, want %q", item.Message, want)
	}
}
