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

package document

import (
	"context"
	"testing"

	"ragflow/internal/progressstore"
)

type fakeProgressReader struct {
	snapshots map[string]*progressstore.Snapshot
	calls     int
}

func (f *fakeProgressReader) Load(_ context.Context, id string) (*progressstore.Snapshot, bool) {
	f.calls++
	snap, ok := f.snapshots[id]
	return snap, ok
}

func runningSnapshot(name, unit string, current, total uint32) *progressstore.Snapshot {
	return &progressstore.Snapshot{
		Status: progressstore.StatusRunning,
		Stages: []progressstore.Stage{{
			Name: name, Phase: "running", Active: unit,
			Units: map[string]progressstore.UnitProgress{unit: {Current: current, Total: total}},
		}},
	}
}

func TestLatestEventsByPipelineLogIDsReadsKV(t *testing.T) {
	reader := &fakeProgressReader{snapshots: map[string]*progressstore.Snapshot{
		"run-1": runningSnapshot("Extractor", "chunk:tags", 3123, 5047),
		"run-2": runningSnapshot("Parser", "page:parse", 100, 500),
	}}
	svc := &DocumentService{}
	svc.SetProgressReader(reader)

	items, err := svc.LatestIngestionEventsByPipelineLogIDs(context.Background(), []string{"run-1", "run-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items["run-1"] == nil || items["run-1"].Message != "Extractor · 标签提取 3123/5047" {
		t.Fatalf("run-1 = %+v", items["run-1"])
	}
	if items["run-2"] == nil || items["run-2"].Message != "Parser · 页面解析 100/500" {
		t.Fatalf("run-2 = %+v", items["run-2"])
	}
	if reader.calls != 2 {
		t.Fatalf("reader calls = %d, want 2", reader.calls)
	}
}

func TestLatestEventsByPipelineLogIDsSkipsMissingSnapshots(t *testing.T) {
	reader := &fakeProgressReader{snapshots: map[string]*progressstore.Snapshot{
		"run-1": runningSnapshot("Parser", "page:parse", 1, 10),
	}}
	svc := &DocumentService{}
	svc.SetProgressReader(reader)

	items, err := svc.LatestIngestionEventsByPipelineLogIDs(context.Background(), []string{"run-1", "missing"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items["run-1"] == nil {
		t.Fatalf("items = %v, want only run-1", items)
	}
	if items["missing"] != nil {
		t.Fatal("missing run should not produce an item")
	}
}

func TestLatestEventsByPipelineLogIDsEmptyInputDoesNotTouchKV(t *testing.T) {
	reader := &fakeProgressReader{}
	svc := &DocumentService{}
	svc.SetProgressReader(reader)

	items, err := svc.LatestIngestionEventsByPipelineLogIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 || reader.calls != 0 {
		t.Fatalf("items=%v calls=%d, want empty/no KV access", items, reader.calls)
	}
}

func TestLatestEventsByPipelineLogIDsNilReaderDegradesSafely(t *testing.T) {
	// Adversarial: no reader installed (e.g. KV disabled) must not panic and
	// must return an empty map rather than fabricated events.
	svc := &DocumentService{}
	items, err := svc.LatestIngestionEventsByPipelineLogIDs(context.Background(), []string{"run-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %v, want empty", items)
	}
}

func TestNewDocumentServiceInstallsProgressReader(t *testing.T) {
	svc := NewDocumentService()
	if svc.progressReader == nil {
		t.Fatal("NewDocumentService must install a KV progress reader")
	}
}
