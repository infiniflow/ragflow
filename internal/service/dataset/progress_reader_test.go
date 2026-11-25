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
	"context"
	"testing"

	"ragflow/internal/progressstore"
)

type stubProgressReader struct {
	snapshots map[string]*progressstore.Snapshot
}

func (s *stubProgressReader) Load(_ context.Context, id string) (*progressstore.Snapshot, bool) {
	snap, ok := s.snapshots[id]
	return snap, ok
}

func datasetRunningSnapshot(name, unit string, current, total uint32) *progressstore.Snapshot {
	return &progressstore.Snapshot{
		Status: progressstore.StatusRunning,
		Stages: []progressstore.Stage{{
			Name: name, Phase: "running", Active: unit,
			Units: map[string]progressstore.UnitProgress{unit: {Current: current, Total: total}},
		}},
	}
}

func TestLatestIngestionEventsByRunReadsKV(t *testing.T) {
	d := &DatasetService{progressReader: &stubProgressReader{snapshots: map[string]*progressstore.Snapshot{
		"run-1": datasetRunningSnapshot("Extractor", "chunk:tags", 3123, 5047),
		"run-2": datasetRunningSnapshot("Parser", "page:parse", 100, 500),
	}}}
	items := d.latestIngestionEventsByRun(context.Background(), []string{"run-1", "run-2"})
	if items["run-1"] == nil || items["run-1"].Message != "Extractor · 标签提取 3123/5047" {
		t.Fatalf("run-1 = %+v", items["run-1"])
	}
	if items["run-2"] == nil || items["run-2"].Message != "Parser · 页面解析 100/500" {
		t.Fatalf("run-2 = %+v", items["run-2"])
	}
}

func TestLatestIngestionEventsByRunSkipsMissingAndEmpty(t *testing.T) {
	d := &DatasetService{progressReader: &stubProgressReader{snapshots: map[string]*progressstore.Snapshot{
		"run-1": datasetRunningSnapshot("Parser", "page:parse", 1, 10),
	}}}
	items := d.latestIngestionEventsByRun(context.Background(), []string{"run-1", "missing"})
	if len(items) != 1 || items["run-1"] == nil {
		t.Fatalf("items = %v, want only run-1", items)
	}
	if got := d.latestIngestionEventsByRun(context.Background(), nil); len(got) != 0 {
		t.Fatalf("empty input = %v, want empty", got)
	}
}

func TestLatestIngestionEventsByRunNilReader(t *testing.T) {
	d := &DatasetService{}
	items := d.latestIngestionEventsByRun(context.Background(), []string{"run-1"})
	if len(items) != 0 {
		t.Fatalf("items = %v, want empty with no reader", items)
	}
}
