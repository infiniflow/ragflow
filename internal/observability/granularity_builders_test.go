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

package observability

import (
	"context"
	"testing"
	"time"

	"ragflow/internal/entity"
)

func granularityCtx() context.Context {
	ctx := context.Background()
	ctx = WithIngestorName(ctx, "ingestor-1")
	ctx = WithNatsQueue(ctx, "queue-1")
	ctx = WithWorkerName(ctx, "worker-1")
	ctx = WithUserName(ctx, "alice")
	ctx = WithTenantName(ctx, "Acme")
	return ctx
}

func granularityTaskRun() (*entity.IngestionTask, *entity.Document, *entity.PipelineOperationLog) {
	task := &entity.IngestionTask{
		ID: "task-1", DocumentID: "doc-1", DatasetID: "kb-1", UserID: "user-1",
		Status: "RUNNING",
	}
	doc := &entity.Document{Name: ptrStr("report"), Suffix: ".pdf", Type: "pdf", Size: 100}
	run := &entity.PipelineOperationLog{ID: "run-1", TenantID: "tenant-1", RunCount: ptrInt(1)}
	return task, doc, run
}

// TestBuildComponentIngestLog verifies the component-scope builder maps every
// granularity field and carries identity. Adversarial: status must come from
// the task, and pipeline_log_id from the run — a builder that left either empty
// would break run-scoped queries.
func TestBuildComponentIngestLog(t *testing.T) {
	task, doc, run := granularityTaskRun()
	log := BuildComponentIngestLog(granularityCtx(), task, doc, run, "Parser", 1, "done")
	if log.EventType != "component" {
		t.Errorf("EventType = %q, want component", log.EventType)
	}
	if log.Status != "RUNNING" {
		t.Errorf("Status = %q, want RUNNING (task status)", log.Status)
	}
	if log.Component != "Parser" || log.Phase != 1 {
		t.Errorf("component/phase = %q/%d, want Parser/1", log.Component, log.Phase)
	}
	if log.Message != "done" {
		t.Errorf("Message = %q, want done", log.Message)
	}
	if log.PipelineLogID != "run-1" {
		t.Errorf("PipelineLogID = %q, want run-1", log.PipelineLogID)
	}
	if log.IngestorName != "ingestor-1" || log.NatsQueue != "queue-1" || log.Worker != "worker-1" {
		t.Errorf("routing identity not propagated: %+v", log)
	}
	if log.UserName != "alice" || log.TenantName != "Acme" {
		t.Errorf("display identity not propagated: %+v", log)
	}
}

// TestBuildProgressIngestLog verifies the progress-scope builder. Adversarial:
// zero total and a large current are boundary values that must map verbatim.
func TestBuildProgressIngestLog(t *testing.T) {
	task, doc, run := granularityTaskRun()
	log := BuildProgressIngestLog(granularityCtx(), task, doc, run, "Tokenizer", "chunk", 257, 3000)
	if log.EventType != "progress" {
		t.Errorf("EventType = %q, want progress", log.EventType)
	}
	if log.Component != "Tokenizer" || log.Unit != "chunk" {
		t.Errorf("component/unit = %q/%q, want Tokenizer/chunk", log.Component, log.Unit)
	}
	if log.Current != 257 || log.Total != 3000 {
		t.Errorf("current/total = %d/%d, want 257/3000", log.Current, log.Total)
	}
	if log.PipelineLogID != "run-1" {
		t.Errorf("PipelineLogID = %q, want run-1", log.PipelineLogID)
	}
	if log.Status != "RUNNING" {
		t.Errorf("Status = %q, want RUNNING", log.Status)
	}

	// boundary: zero current/total
	zero := BuildProgressIngestLog(granularityCtx(), task, doc, run, "Parser", "page", 0, 0)
	if zero.Current != 0 || zero.Total != 0 {
		t.Errorf("zero current/total = %d/%d, want 0/0", zero.Current, zero.Total)
	}
}

// TestNormalizeStageStripsInstanceSuffix is the adversarial fixture: canvas
// component ids are "<Kind>:<instance>" (e.g. "Parser:aaa"). Without stripping
// the instance, every node produces a distinct stage value, defeating the
// LowCardinality column and stage aggregation.
func TestNormalizeStageStripsInstanceSuffix(t *testing.T) {
	cases := map[string]string{
		"Parser:aaa":       "Parser",
		"Tokenizer:node-2": "Tokenizer",
		"Compiler":         "Compiler",
		"GeneralChunker:x": "Chunker",
		"CustomThing:abc":  "CustomThing", // unknown kind preserved, instance stripped
		"":                 "Other",
	}
	for in, want := range cases {
		if got := normalizeStage(in); got != want {
			t.Errorf("normalizeStage(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestGranularityBuildersCarryRunDerivedFields is the adversarial fixture for
// scope unification (finding F4 plan 1): component/progress builders must go
// through BuildIngestLog, so run-derived task fields (Progress, EnqueueTime,
// UploadTime) are carried just like task-scope events. The old private
// baseGranularityLog omitted them, so this fails until the builders are unified
// onto the shared constructor.
func TestGranularityBuildersCarryRunDerivedFields(t *testing.T) {
	task, doc, _ := granularityTaskRun()
	created := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	run := &entity.PipelineOperationLog{
		ID: "run-2", TenantID: "tenant-2", RunCount: ptrInt(2),
		Progress: 0.42, CreateDate: &created,
	}
	doc.CreateDate = &created

	comp := BuildComponentIngestLog(granularityCtx(), task, doc, run, "Parser", 0, "enter")
	if comp.Progress != 0.42 {
		t.Errorf("component Progress = %v, want 0.42 (run-derived)", comp.Progress)
	}
	if comp.EnqueueTime.IsZero() || !comp.EnqueueTime.Equal(created) {
		t.Errorf("component EnqueueTime = %v, want %v", comp.EnqueueTime, created)
	}
	if comp.UploadTime.IsZero() || !comp.UploadTime.Equal(created) {
		t.Errorf("component UploadTime = %v, want %v", comp.UploadTime, created)
	}

	prog := BuildProgressIngestLog(granularityCtx(), task, doc, run, "Tokenizer", "chunk", 1, 5)
	if prog.Progress != 0.42 || !prog.EnqueueTime.Equal(created) {
		t.Errorf("progress run-derived fields missing: progress=%v enqueue=%v", prog.Progress, prog.EnqueueTime)
	}
}

// TestBuildIngestLogNilTaskIsSafe is the adversarial fixture for the nil-tolerance
// parity that F4 plan 1 requires: once granularity builders delegate to
// BuildIngestLog, a nil task/doc/run must not panic (the previous private helper
// tolerated nil). The old BuildIngestLog dereferenced task.ID directly.
func TestBuildIngestLogNilTaskIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("BuildIngestLog panicked on nil task/doc/run: %v", r)
		}
	}()
	log := BuildIngestLog(granularityCtx(), nil, nil, nil, "component", "", "")
	if log.TaskID != "" || log.EventType != "component" {
		t.Errorf("nil-entity log = %+v, want empty TaskID and component scope", log)
	}
}
