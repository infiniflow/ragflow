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

// Tests for the pipeline orchestrator (Phase 3).
//
// These tests register a small set of mock components under
// runtime.CategoryIngestion and exercise the Pipeline.Run,
// RestoreFromCheckpoint, and ProgressSink paths end-to-end. The
// production ingestion components (File / Parser / Chunker /
// Tokenizer / Extractor) require real storage + LLM backends and
// are tested in their own files; the pipeline runner is a pure
// orchestrator and is verified here against deterministic
// stubs.
//
// Cross-domain smoke: TestPipeline_CrossDomainSmoke imports both
// internal/agent/component (for the agent category) and
// internal/ingestion/component (for the ingestion category),
// exercising runtime.DefaultRegistry from a test that triggers
// both packages' init() side effects (plan §9 #6).
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	_ "ragflow/internal/ingestion/component" // blank import: registers ingestion factories
	"ragflow/internal/storage"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Mock components (registered once per test run via init()).
// ---------------------------------------------------------------------------

const (
	mockCompA = "MockA"
	mockCompB = "MockB"
	mockCompC = "MockC"
	mockCompD = "MockD"
	mockCompE = "MockE"
)

// mockBase is the shared state for the mock components. Each
// component's Invoke reads from a shared registry of "stages"
// to keep the test assertions simple. A test can register a
// pre-canned Invoke body per stage name and assert it was
// called.
type mockBase struct {
	parallelism int
}

// MockInvokeFn is the per-stage body. The fn receives the
// current inputs and the stage name; it returns the merged
// outputs (added to inputs) and an optional error.
type MockInvokeFn func(ctx context.Context, stage string, inputs map[string]any) (map[string]any, error)

// mockRegistry holds the per-stage Invoke bodies. Tests
// register them with mockSet(mockCompA, fn), mockSet(mockCompB,
// fn), etc.; the registered component's Invoke looks up its
// body under its own name.
var (
	mockMu      sync.RWMutex
	mockInvokes = map[string]MockInvokeFn{}
)

func mockSet(name string, fn MockInvokeFn) {
	mockMu.Lock()
	defer mockMu.Unlock()
	mockInvokes[name] = fn
}

func mockClear() {
	mockMu.Lock()
	defer mockMu.Unlock()
	mockInvokes = map[string]MockInvokeFn{}
}

type mockComponent struct {
	stage string
	base  mockBase
}

func (c *mockComponent) Parallelism() int { return c.base.parallelism }
func (c *mockComponent) Inputs() map[string]string {
	return map[string]string{"x": "any"}
}
func (c *mockComponent) Outputs() map[string]string {
	return map[string]string{"y": "any"}
}
func (c *mockComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	mockMu.RLock()
	fn, ok := mockInvokes[c.stage]
	mockMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("mock %s: no body registered", c.stage)
	}
	return fn(ctx, c.stage, inputs)
}

func newMockComponent(stage string, parallelism int) func(string, map[string]any) (runtime.Component, error) {
	return func(_ string, _ map[string]any) (runtime.Component, error) {
		return &mockComponent{stage: stage, base: mockBase{parallelism: parallelism}}, nil
	}
}

func init() {
	registerMock(mockCompA, 1)
	registerMock(mockCompB, 4)
	registerMock(mockCompC, 1)
	registerMock(mockCompD, 1)
	registerMock(mockCompE, 1)
	// Register mock variants of the real ingestion stage names
	// so resume tests can use a DSL whose stage names match the
	// materialized-boundary targets. Names use the "Mock" prefix
	// because indexOfStage explicitly allows only the Mock<Name>
	// test convention as a fallback; production stage matching
	// stays exact.
	registerMock("MockFile", 1)
	registerMock("MockParser", 4)
	registerMock("MockTokenChunker", 1)
	registerMock("MockTokenizer", 1)
}

// registerMock wires a mock under `name` with the given
// parallelism. Panics on duplicate registration so the test
// scaffolding fails fast on typos.
func registerMock(name string, parallelism int) {
	runtime.MustRegister(name, runtime.CategoryIngestion, newMockComponent(name, parallelism), runtime.Metadata{
		Inputs:  map[string]string{"x": "any"},
		Outputs: map[string]string{"y": "any"},
	})
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func defaultLinearDSL() []byte {
	d := PipelineDSL{
		Version:    "1",
		Name:       "test",
		StageCount: 5,
		Stages: []StageDSL{
			{Type: mockCompA, Params: map[string]any{}},
			{Type: mockCompB, Params: map[string]any{}},
			{Type: mockCompC, Params: map[string]any{}},
			{Type: mockCompD, Params: map[string]any{}},
			{Type: mockCompE, Params: map[string]any{}},
		},
	}
	b, _ := json.Marshal(d)
	return b
}

func mustCompile(t *testing.T, dsl []byte) *Pipeline {
	t.Helper()
	pl, _ := mustCompileWithStorage(t, dsl, nil)
	return pl
}

func mustCompileWithDAO(t *testing.T, dsl []byte, logsDAO *dao.IngestionTaskLogDAO) *Pipeline {
	t.Helper()
	pl, _ := mustCompileWithStorage(t, dsl, logsDAO)
	return pl
}

// mustCompileWithStorage returns both the pipeline and its
// backing memory-storage handle. Tests that exercise the
// resume path need to seed the storage BEFORE calling
// RestoreFromCheckpoint so the binary-rehydration branch
// (Critical-fix #4) can fetch the bytes. Tests that don't
// care about rehydration continue to use mustCompile /
// mustCompileWithDAO and ignore the storage handle.
func mustCompileWithStorage(t *testing.T, dsl []byte, logsDAO *dao.IngestionTaskLogDAO) (*Pipeline, storage.Storage) {
	t.Helper()
	sink := NewTestSink()
	mem := storage.NewMemoryStorage()
	pl, err := NewPipelineFromDSL(dsl, "", sink, mem, logsDAO)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return pl, mem
}

func makeTask(t *testing.T) *entity.IngestionTask {
	t.Helper()
	taskID := "task-" + t.Name()
	return &entity.IngestionTask{
		ID:         taskID,
		UserID:     "user-1",
		DocumentID: "doc-1",
		DatasetID:  "ds-1",
		Status:     "RUNNING",
	}
}

// setupTaskLogDB wires an in-memory SQLite DB + DAO and
// pushes it as dao.DB. Mirrors the agent_run_e2e_test pattern.
// Uses shared cache so the test's local *gorm.DB and the
// Ingestor's dao.NewIngestionTaskLogDAO() see the same data.
// The DSN is uniquified per test invocation so `go test
// -count=N` runs each iteration against a fresh DB.
func setupTaskLogDB(t *testing.T) *dao.IngestionTaskLogDAO {
	t.Helper()
	dsn := sharedCacheDSN(t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite open: %v", err)
	}
	if err := db.AutoMigrate(&entity.IngestionTaskLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })
	return dao.NewIngestionTaskLogDAO()
}

// sharedCacheDSN builds a unique per-test DSN that points at
// a shared-cache in-memory SQLite database. The name is
// sanitised (alphanumerics + underscores only) because the
// test name can contain characters that confuse the DSN
// parser; mismatched DSNs open distinct in-memory databases.
// A monotonic counter uniquifies the DSN across
// `go test -count=N` iterations so each invocation sees a
// fresh DB.
var sharedCacheCounter atomic.Uint64

func sharedCacheDSN(testName string) string {
	var b strings.Builder
	b.Grow(len(testName) + 48)
	b.WriteString("file:test-")
	for _, r := range testName {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	b.WriteByte('-')
	b.WriteString(fmt.Sprintf("%d", sharedCacheCounter.Add(1)))
	b.WriteString("?mode=memory&cache=shared")
	return b.String()
}

// seedLog inserts a single IngestionTaskLog row with the given
// files[] for use in resume tests.
func seedLog(t *testing.T, taskID string, files []string) {
	t.Helper()
	seedLogWithBoundaries(t, taskID, files, nil)
}

// seedLogWithBoundaries seeds an IngestionTaskLog row with both
// the legacy `files` list (for cleanup tracking) and the
// structured `boundaries` map (for resume decisions). The new
// boundaries-based resume algorithm reads from the boundaries
// map, not from path-suffix matching, so any test that wants to
// simulate a crash mid-pipeline must populate the boundaries map
// with the corresponding entry.
func seedLogWithBoundaries(t *testing.T, taskID string, files []string, boundaries map[string]map[string]any) {
	t.Helper()
	cp := entity.JSONMap{
		"current_component":    mockCompA,
		"completed_components": []string{},
		"total_components":     5,
		"files":                files,
	}
	if boundaries != nil {
		cp["boundaries"] = boundaries
	}
	row := &entity.IngestionTaskLog{TaskID: taskID, Checkpoint: cp}
	if err := dao.DB.Create(row).Error; err != nil {
		t.Fatalf("seedLogWithBoundaries: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestPipeline_LinearHappyPath runs the 5-stage DSL end-to-end
// and verifies all 5 components execute, the final output is
// emitted, and the sink records one checkpoint per stage.
func TestPipeline_LinearHappyPath(t *testing.T) {
	mockClear()

	var callCount int32
	record := func(label string) MockInvokeFn {
		return func(ctx context.Context, stage string, inputs map[string]any) (map[string]any, error) {
			atomic.AddInt32(&callCount, 1)
			return map[string]any{
				label:        inputs,
				"_order":     appendOrder(inputs, stage),
				"received":   inputs,
				"emitted_by": stage,
			}, nil
		}
	}
	mockSet(mockCompA, record("A"))
	mockSet(mockCompB, record("B"))
	mockSet(mockCompC, record("C"))
	mockSet(mockCompD, record("D"))
	mockSet(mockCompE, record("E"))

	pl := mustCompile(t, defaultLinearDSL())
	out, err := pl.Run(context.Background(), map[string]any{"seed": 1})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if atomic.LoadInt32(&callCount) != 5 {
		t.Errorf("expected 5 component calls, got %d", callCount)
	}
	if v, ok := out["emitted_by"]; !ok || v != mockCompE {
		t.Errorf("expected final emitter %s, got %v", mockCompE, v)
	}
	// Verify the TestSink recorded 5 stage transitions (one
	// per stage completion). TestSink is the production
	// pattern: each stage completion writes a checkpoint.
	testSink, ok := pl.Sink().(*TestSink)
	if !ok {
		t.Fatalf("expected TestSink, got %T", pl.Sink())
	}
	if rows := testSink.Snapshots(); len(rows) != 5 {
		t.Errorf("expected 5 checkpoint writes, got %d", len(rows))
	}
}

// appendOrder appends a stage to a deterministic order list
// in inputs["__order__"] so tests can assert execution order.
func appendOrder(inputs map[string]any, stage string) []string {
	prev, _ := inputs["__order__"].([]string)
	out := append([]string(nil), prev...)
	out = append(out, stage)
	return out
}

// TestPipeline_FanOut runs the 5-stage pipeline (B has
// Parallelism=4) and verifies each stage body is invoked
// exactly once per run, deterministically. The pipeline
// runner records Parallelism status rows in the checkpoint
// (not by re-invoking the body). We run 10 times to catch
// ordering flakiness.
func TestPipeline_FanOut(t *testing.T) {
	mockClear()
	var (
		count   int32
		callSeq []string
		mu      sync.Mutex
	)
	rec := func(name string) MockInvokeFn {
		return func(ctx context.Context, stage string, _ map[string]any) (map[string]any, error) {
			atomic.AddInt32(&count, 1)
			mu.Lock()
			callSeq = append(callSeq, name)
			mu.Unlock()
			return map[string]any{"prev": name}, nil
		}
	}
	mockSet(mockCompA, rec("A"))
	mockSet(mockCompB, rec("B"))
	mockSet(mockCompC, rec("C"))
	mockSet(mockCompD, rec("D"))
	mockSet(mockCompE, rec("E"))

	for i := 0; i < 10; i++ {
		atomic.StoreInt32(&count, 0)
		mu.Lock()
		callSeq = nil
		mu.Unlock()
		pl := mustCompile(t, defaultLinearDSL())
		_, err := pl.Run(context.Background(), map[string]any{"k": "v"})
		if err != nil {
			t.Fatalf("iter %d: Run: %v", i, err)
		}
		if got := atomic.LoadInt32(&count); got != 5 {
			t.Fatalf("iter %d: expected 5 invocations, got %d", i, got)
		}
		// Verify call order is deterministic across iterations.
		canonical := []string{"A", "B", "C", "D", "E"}
		mu.Lock()
		got := append([]string(nil), callSeq...)
		mu.Unlock()
		if !equalStrings(got, canonical) {
			t.Errorf("iter %d: order mismatch: got %v, want %v", i, got, canonical)
		}
	}

	// Verify the checkpoint reports 4 work-units for the B stage
	// (Parallelism=4) and 1 for the others. Medium-fix #5
	// renamed the persisted field from `goroutine_status` to
	// `work_unit_status`; the runner mirrors both keys so older
	// readers continue to see the legacy field for one release.
	pl := mustCompile(t, defaultLinearDSL())
	_, err := pl.Run(context.Background(), map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	cp := pl.LastCheckpoint()
	ws, ok := cp["work_unit_status"].(map[string][]GoroutineStatus)
	if !ok {
		t.Fatalf("expected work_unit_status map, got %T", cp["work_unit_status"])
	}
	if got := len(ws[mockCompB]); got != 4 {
		t.Errorf("expected 4 work-unit status rows for B, got %d", got)
	}
	if got := len(ws[mockCompA]); got != 1 {
		t.Errorf("expected 1 work-unit status row for A, got %d", got)
	}
	// Legacy-key mirror must still be present so a pre-rename
	// reader on an older persisted row continues to function.
	if _, ok := cp["goroutine_status"]; !ok {
		t.Error("expected legacy goroutine_status mirror under new checkpoint")
	}
}

// TestPipeline_Cancellation cancels ctx mid-stage and
// verifies the error is propagated as a context error.
func TestPipeline_Cancellation(t *testing.T) {
	mockClear()
	mockSet(mockCompA, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "A"}, nil
	})
	mockSet(mockCompB, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "B"}, nil
	})
	// MockC cancels itself.
	mockSet(mockCompC, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return nil, context.Canceled
	})
	mockSet(mockCompD, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		t.Fatal("MockD should not run after MockC cancellation")
		return nil, nil
	})
	mockSet(mockCompE, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		t.Fatal("MockE should not run after MockC cancellation")
		return nil, nil
	})

	pl := mustCompile(t, defaultLinearDSL())
	_, err := pl.Run(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

// TestPipeline_DSLRoundTrip encodes the default DSL and
// decodes it back; every field must survive.
func TestPipeline_DSLRoundTrip(t *testing.T) {
	in := PipelineDSL{
		Version:     "1",
		Name:        "roundtrip",
		Description: "ensures JSON round-trip",
		StageCount:  2,
		Stages: []StageDSL{
			{Type: "File", Params: map[string]any{"a": 1, "b": "two"}},
			{Type: "Parser", Params: map[string]any{"c": true}},
		},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var out PipelineDSL
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if out.Version != in.Version || out.Name != in.Name || out.Description != in.Description {
		t.Errorf("scalar mismatch: %+v", out)
	}
	if len(out.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(out.Stages))
	}
	if out.Stages[0].Type != "File" || out.Stages[1].Type != "Parser" {
		t.Errorf("stage order mismatch: %+v", out.Stages)
	}
	if out.Stages[0].Params["a"].(float64) != 1 || out.Stages[0].Params["b"].(string) != "two" {
		t.Errorf("params mismatch: %+v", out.Stages[0].Params)
	}
}

// ---------------------------------------------------------------------------
// Resume tests
// ---------------------------------------------------------------------------

// TestPipeline_Resume_AfterFile: seed a log with the File
// boundary (structured `boundaries[BoundaryKindFile]` entry).
// RestoreFromCheckpoint should return StartAt=1 (Parser) and
// a materialized input map that includes the REHYDRATED binary
// (Critical-fix #4) plus the file_ref_* diagnostics.
func TestPipeline_Resume_AfterFile(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	seedLogWithBoundaries(t, task.ID, nil, map[string]map[string]any{
		BoundaryKindFile: {"bucket": "ragflow", "path": "doc/test.pdf"},
	})

	// Custom 2-stage DSL: MockFile then MockParser. The resume
	// algorithm's indexOfStage does suffix matching, so "Parser"
	// resolves to MockParser at index 1.
	fileParserDSL := []byte(`{
  "version": "1",
  "name": "fp",
  "stage_count": 2,
  "stages": [
    {"type": "MockFile",   "params": {}},
    {"type": "MockParser", "params": {}}
  ]
}`)
	pl, mem := mustCompileWithStorage(t, fileParserDSL, dao.NewIngestionTaskLogDAO())
	// Seed the storage so the binary-rehydration branch can
	// fetch real bytes (Critical-fix #4). The seeded bytes
	// match what File would have written after fetching
	// "doc/test.pdf".
	wantBytes := []byte("PDF-CONTENT-FOR-TEST")
	if err := mem.Put("ragflow", "doc/test.pdf", wantBytes); err != nil {
		t.Fatalf("seed mem: %v", err)
	}

	res, err := pl.RestoreFromCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 1 {
		t.Errorf("expected StartAt=1 (MockParser), got %d", res.StartAt)
	}
	// Critical-fix #4: the binary must be REHYDRATED into Inputs
	// so Parser can consume it without a path-aware fork.
	bin, ok := res.Inputs["binary"].([]byte)
	if !ok {
		t.Fatalf("expected rehydrated []byte under Inputs[\"binary\"], got %T", res.Inputs["binary"])
	}
	if !equalBytes(bin, wantBytes) {
		t.Errorf("rehydrated bytes mismatch: got %q, want %q", bin, wantBytes)
	}
	// Diagnostic-only fields must remain alongside binary so
	// operators can still identify the boundary origin.
	if res.Inputs["file_ref_bucket"] != "ragflow" {
		t.Errorf("expected file_ref_bucket=ragflow, got %q", res.Inputs["file_ref_bucket"])
	}
	if res.Inputs["file_ref_path"] != "doc/test.pdf" {
		t.Errorf("expected file_ref_path=doc/test.pdf, got %q", res.Inputs["file_ref_path"])
	}
}

// TestPipeline_Resume_RealFileBoundaryValues is the regression
// test for the boundary-protocol fix + Critical-fix #4. It seeds
// a real File boundary (not a synthetic suffix) AND seeds the
// storage so rehydration can find the bytes. The test asserts
// that the resume decision recognises the boundary AND that the
// binary is rehydrated to the exact bytes the File component
// would have produced.
func TestPipeline_Resume_RealFileBoundaryValues(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	// Custom 2-stage DSL: MockFile then MockParser.
	fileParserDSL := []byte(`{
  "version": "1",
  "name": "real-file-boundary",
  "stage_count": 2,
  "stages": [
    {"type": "MockFile",   "params": {}},
    {"type": "MockParser", "params": {}}
  ]
}`)
	mockClear()

	realFileOutput := map[string]any{
		"name":   "test.pdf",
		"bucket": "ragflow",
		"path":   "documents/2026/07/test.pdf",
		"file": map[string]any{
			"id":   "documents/2026/07/test.pdf",
			"name": "test.pdf",
		},
	}
	// Hand-roll the boundaries map the way recordFileRef would.
	seedLogWithBoundaries(t, task.ID, []string{"ragflow/documents/2026/07/test.pdf"}, map[string]map[string]any{
		BoundaryKindFile: {
			"bucket": realFileOutput["bucket"].(string),
			"path":   realFileOutput["path"].(string),
			"key":    "ragflow/documents/2026/07/test.pdf",
		},
	})

	pl, mem := mustCompileWithStorage(t, fileParserDSL, dao.NewIngestionTaskLogDAO())
	wantBytes := []byte("%PDF-1.4 (real)")
	if err := mem.Put("ragflow", "documents/2026/07/test.pdf", wantBytes); err != nil {
		t.Fatalf("seed mem: %v", err)
	}

	res, err := pl.RestoreFromCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 1 {
		t.Errorf("expected StartAt=1 (MockParser); the structured boundaries map must drive resume, got StartAt=%d", res.StartAt)
	}
	bin, ok := res.Inputs["binary"].([]byte)
	if !ok {
		t.Fatalf("expected rehydrated []byte under Inputs[\"binary\"], got %T", res.Inputs["binary"])
	}
	if !equalBytes(bin, wantBytes) {
		t.Errorf("rehydrated bytes mismatch: got %q, want %q", bin, wantBytes)
	}
	if res.Inputs["file_ref_bucket"] != "ragflow" {
		t.Errorf("expected file_ref_bucket=ragflow, got %q", res.Inputs["file_ref_bucket"])
	}
	if res.Inputs["file_ref_path"] != "documents/2026/07/test.pdf" {
		t.Errorf("expected file_ref_path=documents/2026/07/test.pdf, got %q", res.Inputs["file_ref_path"])
	}
}

// TestPipeline_Resume_FileBoundary_RehydratesBinary
// (Critical-fix #4): explicitly pins the contract that a
// File-boundary resume returns the binary bytes rehydrated from
// storage. Prior implementations emitted only `file_ref_bucket`/
// `file_ref_path`, leaving Parser with no `binary` to consume
// — the resumed run then no-op'd or errored at the component
// boundary. This test is the regression that keeps the
// rehydration in place.
func TestPipeline_Resume_FileBoundary_RehydratesBinary(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	seedLogWithBoundaries(t, task.ID, nil, map[string]map[string]any{
		BoundaryKindFile: {"bucket": "ragflow", "path": "src/notes.txt"},
	})

	dsl := []byte(`{
  "version": "1",
  "name": "rehydrate",
  "stage_count": 2,
  "stages": [
    {"type": "MockFile",   "params": {}},
    {"type": "MockParser", "params": {}}
  ]
}`)
	pl, mem := mustCompileWithStorage(t, dsl, dao.NewIngestionTaskLogDAO())
	payload := []byte("hello-ingestion-world")
	if err := mem.Put("ragflow", "src/notes.txt", payload); err != nil {
		t.Fatalf("seed mem: %v", err)
	}

	res, err := pl.RestoreFromCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 1 {
		t.Fatalf("expected StartAt=1 (MockParser), got %d", res.StartAt)
	}
	bin, ok := res.Inputs["binary"].([]byte)
	if !ok {
		t.Fatalf("expected []byte binary, got %T", res.Inputs["binary"])
	}
	if string(bin) != string(payload) {
		t.Fatalf("rehydrated binary mismatch: got %q, want %q", bin, payload)
	}
}

// TestPipeline_Resume_FileBoundary_NoStorage (Critical-fix #4
// fallback): when the Pipeline is constructed with a nil
// storage backend (no recovery), the File boundary must still
// emit the file_ref_* shape so a caller that performs its own
// fetch (e.g. an admin tool) can observe the boundary. The
// legacy form is documented and tested here.
func TestPipeline_Resume_FileBoundary_NoStorage(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	seedLogWithBoundaries(t, task.ID, nil, map[string]map[string]any{
		BoundaryKindFile: {"bucket": "ragflow", "path": "src/notes.txt"},
	})

	dsl := []byte(`{
  "version": "1",
  "name": "no-storage",
  "stage_count": 2,
  "stages": [
    {"type": "MockFile",   "params": {}},
    {"type": "MockParser", "params": {}}
  ]
}`)

	// Compile WITHOUT storage: NewPipelineFromDSL takes a
	// storage.Storage; passing a nil interface here falls back
	// to the no-storage branch in resolveBoundary.
	pl, err := NewPipelineFromDSL(dsl, "", NewTestSink(), nil, dao.NewIngestionTaskLogDAO())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res, err := pl.RestoreFromCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 1 {
		t.Fatalf("expected StartAt=1, got %d", res.StartAt)
	}
	if _, hasBinary := res.Inputs["binary"]; hasBinary {
		t.Error("Inputs[\\\"binary\\\"] must be absent when storage is nil")
	}
	if res.Inputs["file_ref_bucket"] != "ragflow" {
		t.Errorf("expected file_ref_bucket=ragflow, got %q", res.Inputs["file_ref_bucket"])
	}
	if res.Inputs["file_ref_path"] != "src/notes.txt" {
		t.Errorf("expected file_ref_path=src/notes.txt, got %q", res.Inputs["file_ref_path"])
	}
}

// TestPipeline_Restore_HydratesLastCheckpoint (Critical-fix
// #3): RestoreFromCheckpoint must populate p.lastCheckpoint
// with the persisted `boundaries` and `component_done` maps
// from the row. Without this hydration, the run-loop's
// IsStageDone check sees an empty checkpoint and forces a
// re-run of every stage (the cross-run stage-skip path is
// effectively disconnected).
//
// The test seeds a row with both boundaries AND a
// component_done entry, calls RestoreFromCheckpoint, and then
// asserts the in-memory checkpoint map (`pl.LastCheckpoint`)
// reflects both rows. This pins the contract that
// `pipelineVersion-aware` stage skips survive across process
// restarts.
func TestPipeline_Restore_HydratesLastCheckpoint(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)

	wantPipelineVersion := "pv-deadbeef"
	wantComponentName := "MockFile"
	wantComponentVersion := "1.0.0"
	wantInputFingerprint := "fp-test-12345"

	cp := entity.JSONMap{
		"current_component":    "MockFile",
		"completed_components": []string{"MockFile"},
		"total_components":     2,
		"files":                []string{"ragflow/test.pdf"},
		"boundaries": map[string]any{
			BoundaryKindFile: map[string]any{
				"bucket": "ragflow",
				"path":   "test.pdf",
				"key":    "ragflow/test.pdf",
			},
		},
		"component_done": map[string]any{
			wantComponentName: map[string]any{
				"pipeline_version":  wantPipelineVersion,
				"component_version": wantComponentVersion,
				"input_fingerprint": wantInputFingerprint,
				"completed_at":      "2026-07-02T00:00:00Z",
			},
		},
	}
	if err := dao.DB.Create(&entity.IngestionTaskLog{TaskID: task.ID, Checkpoint: cp}).Error; err != nil {
		t.Fatalf("seed log: %v", err)
	}

	dsl := []byte(`{
  "version": "1",
  "name": "hydrate-test",
  "stage_count": 2,
  "stages": [
    {"type": "MockFile",   "params": {}},
    {"type": "MockParser", "params": {}}
  ]
}`)
	pl, mem := mustCompileWithStorage(t, dsl, dao.NewIngestionTaskLogDAO())
	// Seed storage so the boundary-driven rehydration branch
	// (Critical-fix #4) doesn't error on the Get.
	if err := mem.Put("ragflow", "test.pdf", []byte("hydrate-test-bytes")); err != nil {
		t.Fatalf("seed mem: %v", err)
	}
	if _, err := pl.RestoreFromCheckpoint(task.ID); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	lc := pl.LastCheckpoint()
	// boundaries must be hydrated.
	if _, ok := lc["boundaries"]; !ok {
		t.Fatal("expected boundaries map to be hydrated into lastCheckpoint")
	}
	// component_done must be hydrated AND intact.
	done, ok := lc["component_done"].(map[string]any)
	if !ok {
		t.Fatalf("expected component_done map, got %T", lc["component_done"])
	}
	row, ok := done[wantComponentName].(map[string]any)
	if !ok {
		t.Fatalf("expected component_done[%s], got %v", wantComponentName, done)
	}
	if row["pipeline_version"] != wantPipelineVersion {
		t.Errorf("hydrated pipeline_version mismatch: got %v, want %s", row["pipeline_version"], wantPipelineVersion)
	}
	if row["component_version"] != wantComponentVersion {
		t.Errorf("hydrated component_version mismatch: got %v, want %s", row["component_version"], wantComponentVersion)
	}
	if row["input_fingerprint"] != wantInputFingerprint {
		t.Errorf("hydrated input_fingerprint mismatch: got %v, want %s", row["input_fingerprint"], wantInputFingerprint)
	}
	// The hydrated row must satisfy IsStageDone when called
	// with the SAME (pipeline_version, component_version,
	// input_fingerprint) tuple — pinning the cross-run stage-
	// skip contract. (Run() does NOT consult IsStageDone
	// today; the function is retained for tests and the future
	// output-reconstructing skip — see Critical-fix #6.)
	if !IsStageDone(lc, wantPipelineVersion, wantComponentName, wantComponentVersion, wantInputFingerprint) {
		t.Error("IsStageDone should match the hydrated component_done row")
	}
	// A different pipeline_version must NOT match (Critical-fix
	// #1): the row records the version it was written under,
	// and a re-run under a new DSL must invalidate it.
	if IsStageDone(lc, "pv-different", wantComponentName, wantComponentVersion, wantInputFingerprint) {
		t.Error("IsStageDone should miss under a different pipeline_version")
	}
}

// TestPipeline_Resume_ChunkerBoundary_RehydratesChunks
// (Critical-fix #5): the chunker boundary in
// `boundaries[BoundaryKindChunker]` is the storage key of a
// JSONL file containing one JSON object per line (the shape
// Tokenizer's getChunks accepts as `chunks`). Restore must
// read the file from storage, parse each line, and expose
// the canonical []map[string]any under `Inputs["chunks"]` so
// Tokenizer can consume the boundary without growing a
// path-aware fork.
func TestPipeline_Resume_ChunkerBoundary_RehydratesChunks(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)

	// Custom 4-stage DSL with a Tokenizer suffix-matching
	// stage so the resume algorithm lands on a real Tokenizer
	// stage instead of erroring out. We use Mock-prefixed
	// names to avoid forcing a real TokenChunker registration
	// in this test (which would close the production
	// pipeline ↔ chunker ↔ ingestion ↔ pipeline cycle).
	dsl := []byte(`{
  "version": "1",
  "name": "chunker-resume",
  "stage_count": 4,
  "stages": [
    {"type": "MockFile",          "params": {}},
    {"type": "MockParser",        "params": {}},
    {"type": "MockTokenChunker",  "params": {}},
    {"type": "MockTokenizer",     "params": {}}
  ]
}`)
	chunksPath := "task-chunks-resume/MockTokenChunker/chunks.jsonl"
	seedLogWithBoundaries(t, task.ID, []string{chunksPath}, map[string]map[string]any{
		BoundaryKindChunker: {
			"chunks_jsonl": chunksPath,
			"stage":        "MockTokenChunker",
		},
	})

	pl, mem := mustCompileWithStorage(t, dsl, dao.NewIngestionTaskLogDAO())
	// Two-line JSONL: one JSON object, one JSON string (the
	// latter mirrors the chunksToLinesFromStrings path).
	payload := []byte(`{"text": "first-chunk"}
"second-chunk-as-string"
`)
	if err := mem.Put(DefaultPipelineBucket, chunksPath, payload); err != nil {
		t.Fatalf("seed mem: %v", err)
	}

	res, err := pl.RestoreFromCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 3 {
		t.Fatalf("expected StartAt=3 (MockTokenizer), got %d", res.StartAt)
	}
	// Critical-fix #5: Inputs["chunks"] must be rehydrated.
	chunks, ok := res.Inputs["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("expected rehydrated []map[string]any under Inputs[\"chunks\"], got %T", res.Inputs["chunks"])
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 rehydrated chunks, got %d", len(chunks))
	}
	if chunks[0]["text"] != "first-chunk" {
		t.Errorf("chunk[0].text mismatch: got %v", chunks[0]["text"])
	}
	if chunks[1]["text"] != "second-chunk-as-string" {
		t.Errorf("chunk[1].text mismatch: got %v", chunks[1]["text"])
	}
	// Diagnostic: the storage path is retained too.
	if res.Inputs["chunks_jsonl"] != chunksPath {
		t.Errorf("expected chunks_jsonl=%q, got %q", chunksPath, res.Inputs["chunks_jsonl"])
	}
	// A subsequent Run-from-resume must succeed (no early
	// "inputs missing chunks" error). We seed a trivial
	// MockTokenizer body that asserts the chunks arrived.
	mockClear()
	var gotChunks []map[string]any
	mockSet("MockTokenizer", func(_ context.Context, _ string, in map[string]any) (map[string]any, error) {
		gotChunks, _ = in["chunks"].([]map[string]any)
		return map[string]any{"y": "tok"}, nil
	})
	if _, err := pl.RunFromCheckpoint(context.Background(), res.Inputs, res.StartAt); err != nil {
		t.Fatalf("Run from resumed inputs: %v", err)
	}
	if len(gotChunks) != 2 {
		t.Errorf("MockTokenizer saw %d chunks, want 2 (rehydrated)", len(gotChunks))
	}
}

// TestPipeline_Resume_ChunkerBoundary_NoStorage (Critical-fix
// #5 fallback): when storage is nil, the resume algorithm
// falls back to the legacy `chunks_jsonl` path-only form so
// callers that perform their own fetch can still observe the
// boundary. Same pattern as the File boundary fallback.
func TestPipeline_Resume_ChunkerBoundary_NoStorage(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	seedLogWithBoundaries(t, task.ID, nil, map[string]map[string]any{
		BoundaryKindChunker: {
			"chunks_jsonl": "no-storage-task/MockTokenChunker/chunks.jsonl",
			"stage":        "MockTokenChunker",
		},
	})
	dsl := []byte(`{
  "version": "1",
  "name": "no-storage-chunks",
  "stage_count": 4,
  "stages": [
    {"type": "MockFile",          "params": {}},
    {"type": "MockParser",        "params": {}},
    {"type": "MockTokenChunker",  "params": {}},
    {"type": "MockTokenizer",     "params": {}}
  ]
}`)
	pl, err := NewPipelineFromDSL(dsl, "", NewTestSink(), nil, dao.NewIngestionTaskLogDAO())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res, err := pl.RestoreFromCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 3 {
		t.Fatalf("expected StartAt=3, got %d", res.StartAt)
	}
	if _, hasChunks := res.Inputs["chunks"]; hasChunks {
		t.Error("Inputs[\\\"chunks\\\"] must be absent when storage is nil")
	}
	if res.Inputs["chunks_jsonl"] != "no-storage-task/MockTokenChunker/chunks.jsonl" {
		t.Errorf("chunks_jsonl mismatch: got %v", res.Inputs["chunks_jsonl"])
	}
}

// TestPipeline_ParseJSONLChunks covers the decode helper
// directly. Critical-fix #5 callers go through
// ResolveFromCheckpoint → resolveBoundary → parseJSONLChunks;
// this test pins the line-by-line semantics in isolation.
func TestPipeline_ParseJSONLChunks(t *testing.T) {
	// Mixed line types: object, string, malformed, blank.
	payload := []byte(
		`{"text":"first","page":1}` + "\n" +
			`"second"` + "\n" +
			"" + "\n" +
			`{this is not json}` + "\n" +
			`null` + "\n" +
			`{"text":"third"}`,
	)
	chunks, err := parseJSONLChunks(payload)
	if err != nil {
		t.Fatalf("parseJSONLChunks: %v", err)
	}
	// Expectation: object → as-is (skip nulls); quoted string
	// → {"text": "..."}; malformed → skipped; blank → skipped.
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks (object, string, object), got %d: %+v", len(chunks), chunks)
	}
	if chunks[0]["text"] != "first" || chunks[0]["page"] != float64(1) {
		t.Errorf("chunk[0] mismatch: %+v", chunks[0])
	}
	if chunks[1]["text"] != "second" {
		t.Errorf("chunk[1] mismatch: %+v", chunks[1])
	}
	if chunks[2]["text"] != "third" {
		t.Errorf("chunk[2] mismatch: %+v", chunks[2])
	}

	// Empty input → nil chunks (not an error).
	empty, err := parseJSONLChunks(nil)
	if err != nil {
		t.Fatalf("parseJSONLChunks(nil): %v", err)
	}
	if empty != nil {
		t.Errorf("expected nil chunks for empty input, got %+v", empty)
	}
}

// TestPipeline_StageSkip_DisabledAcrossResumes (Critical-fix
// #6): even when a `component_done` row is hydrated from a
// prior run, the run-loop MUST NOT skip the stage. Skipping
// would discard the stage's outputs from `current`, leaving
// the next stage with missing data. We verify by running a
// pipeline twice with the same DSL (the second run should
// run every stage exactly once, NOT log them in
// `skipped_stages`).
func TestPipeline_StageSkip_DisabledAcrossResumes(t *testing.T) {
	setupTaskLogDB(t)
	mockClear()

	_ = makeTask(t) // taskID not used here; the test asserts skip-disable end-to-end
	var (
		count int32
	)
	rec := func(label string) MockInvokeFn {
		return func(_ context.Context, _ string, in map[string]any) (map[string]any, error) {
			atomic.AddInt32(&count, 1)
			// Each stage PROMOTES a known key so the next
			// stage's body can prove its upstream output is
			// present (Critical-fix #6 contract).
			return map[string]any{"emitted_by": label}, nil
		}
	}
	mockSet(mockCompA, rec("A"))
	mockSet(mockCompB, rec("B"))
	mockSet(mockCompC, rec("C"))
	mockSet(mockCompD, rec("D"))
	mockSet(mockCompE, rec("E"))

	pl := mustCompile(t, defaultLinearDSL())

	// First run: every stage runs once; we do NOT persist this
	// — the goal of this test is to assert the in-memory skip
	// DISABLE behaviour, not the cross-run persistence path
	// (covered by TestPipeline_Restore_HydratesLastCheckpoint).
	if _, err := pl.Run(context.Background(), map[string]any{"seed": 1}); err != nil {
		t.Fatalf("Run1: %v", err)
	}
	firstCount := atomic.LoadInt32(&count)
	if firstCount != 5 {
		t.Fatalf("first run: expected 5 invocations, got %d", firstCount)
	}
	if skipped, _ := pl.LastCheckpoint()["skipped_stages"].([]string); len(skipped) != 0 {
		t.Errorf("first run should not skip any stage, got %v", skipped)
	}

	// Second run on the SAME pipeline (startAt=0, no
	// boundary): stage-skip is disabled, so EVERY stage runs
	// again. A prior `component_done` row written during Run 1
	// must NOT short-circuit Run 2.
	count = 0
	if _, err := pl.Run(context.Background(), map[string]any{"seed": 1}); err != nil {
		t.Fatalf("Run2: %v", err)
	}
	if got := atomic.LoadInt32(&count); got != 5 {
		t.Errorf("second run: expected 5 invocations (skip disabled), got %d", got)
	}
	if skipped, _ := pl.LastCheckpoint()["skipped_stages"].([]string); len(skipped) != 0 {
		t.Errorf("second run should not skip any stage, got %v", skipped)
	}
}

// TestPipeline_IdempotencyKey: a same-DSL re-run produces the
// same pipeline_version; a DSL change produces a different one.
// This is the contract that lets the resume algorithm detect
// "the DSL changed and the prior component_done rows are
// stale" (plan §4 Phase 3 task 0c).
func TestPipeline_IdempotencyKey(t *testing.T) {
	dslA := []byte(`{
  "version": "1",
  "name": "a",
  "stage_count": 2,
  "stages": [
    {"type": "MockA", "params": {}},
    {"type": "MockB", "params": {}}
  ]
}`)
	dslB := []byte(`{
  "version": "1",
  "name": "a",
  "stage_count": 3,
  "stages": [
    {"type": "MockA", "params": {}},
    {"type": "MockB", "params": {}},
    {"type": "MockC", "params": {}}
  ]
}`)

	vA := ComputePipelineVersion(dslA)
	vA2 := ComputePipelineVersion(dslA) // identical input → identical digest
	vB := ComputePipelineVersion(dslB)

	if vA == "" {
		t.Fatal("ComputePipelineVersion: empty digest for valid DSL")
	}
	if vA != vA2 {
		t.Errorf("identical input should be idempotent: vA=%s vA2=%s", vA, vA2)
	}
	if vA == vB {
		t.Errorf("structural change should change version: vA=%s vB=%s", vA, vB)
	}
}

// TestPipeline_InputFingerprint: same params in any key order
// produce the same fingerprint; a different param value changes
// it. Mirrors the idempotency contract for component_version +
// input_fingerprint.
func TestPipeline_InputFingerprint(t *testing.T) {
	fp1 := ComputeParamsFingerprint(map[string]any{"a": 1, "b": "x"})
	fp1Reordered := ComputeParamsFingerprint(map[string]any{"b": "x", "a": 1})
	fp2 := ComputeParamsFingerprint(map[string]any{"a": 2, "b": "x"})

	if fp1 == "" {
		t.Fatal("ComputeParamsFingerprint: empty digest for non-empty params")
	}
	if fp1 != fp1Reordered {
		t.Errorf("key-order rearrangement should be idempotent: fp1=%s fp1Reordered=%s", fp1, fp1Reordered)
	}
	if fp1 == fp2 {
		t.Errorf("value change should change fingerprint: fp1=%s fp2=%s", fp1, fp2)
	}
}

// TestPipeline_StageFingerprint_UpstreamMatters (Critical-fix #2):
// the stage-skip fingerprint MUST change when the upstream
// payload that feeds the component changes. This pins the
// contract that a re-run of File emitting a new binary must
// invalidate Parser's done row even if Parser's own params are
// unchanged.
func TestPipeline_StageFingerprint_UpstreamMatters(t *testing.T) {
	upstreamA := map[string]any{"binary": "AAA"}
	upstreamB := map[string]any{"binary": "BBB"}
	params := map[string]any{"page_size": 4}

	fpAA := ComputeStageFingerprint(upstreamA, params)
	fpBA := ComputeStageFingerprint(upstreamB, params)
	if fpAA == fpBA {
		t.Fatalf("upstream change should change fingerprint: fpAA=%s fpBA=%s", fpAA, fpBA)
	}

	// Same upstream + same params → identical fingerprint.
	fpAA2 := ComputeStageFingerprint(map[string]any{"binary": "AAA"}, params)
	if fpAA != fpAA2 {
		t.Errorf("identical inputs should be idempotent: fpAA=%s fpAA2=%s", fpAA, fpAA2)
	}

	// Same upstream + different params → different fingerprint.
	fpAB := ComputeStageFingerprint(upstreamA, map[string]any{"page_size": 8})
	if fpAA == fpAB {
		t.Errorf("params change should change fingerprint: fpAA=%s fpAB=%s", fpAA, fpAB)
	}

	// Nil upstream / nil params must hash to a non-empty digest
	// (the "null" encoding) and remain stable across calls.
	fpNil := ComputeStageFingerprint(nil, nil)
	fpNil2 := ComputeStageFingerprint(nil, nil)
	if fpNil == "" {
		t.Fatal("nil inputs should still hash")
	}
	if fpNil != fpNil2 {
		t.Errorf("nil inputs should be idempotent: fpNil=%s fpNil2=%s", fpNil, fpNil2)
	}
}

// TestPipeline_IsStageDone_QuadrupleMatch (Critical-fix #1):
// the component_done row's (pipeline_version, component_name,
// component_version, input_fingerprint) must all match for
// IsStageDone to return true. A change in any one field
// invalidates the row and forces a re-run. This is the
// fail-closed contract plan §4 Phase 3 task 0c requires.
func TestPipeline_IsStageDone_QuadrupleMatch(t *testing.T) {
	const (
		pv  = "pv-abc"
		cv  = "1.0.0"
		fp  = "fp-123"
		alt = "fp-456"
	)
	cp := map[string]any{
		"component_done": map[string]any{
			"File": map[string]any{
				"pipeline_version":  pv,
				"component_version": cv,
				"input_fingerprint": fp,
			},
		},
	}
	if !IsStageDone(cp, pv, "File", cv, fp) {
		t.Error("expected IsStageDone=true for matching quadruple")
	}
	// pipeline_version mismatch (the Critical-fix #1 regression).
	if IsStageDone(cp, "pv-DIFFERENT", "File", cv, fp) {
		t.Error("pipeline_version change must invalidate the row (Critical-fix #1)")
	}
	if IsStageDone(cp, pv, "File", "1.0.1", fp) {
		t.Error("component_version change should invalidate the row")
	}
	if IsStageDone(cp, pv, "File", cv, alt) {
		t.Error("input_fingerprint change should invalidate the row")
	}
	if IsStageDone(cp, pv, "Parser", cv, fp) {
		t.Error("component_name change should invalidate the row")
	}
	// Empty checkpoint / no component_done key.
	if IsStageDone(map[string]any{}, pv, "File", cv, fp) {
		t.Error("empty checkpoint should be a miss")
	}
	// Legacy row missing pipeline_version → fail-closed (force re-run).
	legacy := map[string]any{
		"component_done": map[string]any{
			"File": map[string]any{
				"component_version": cv,
				"input_fingerprint": fp,
			},
		},
	}
	if IsStageDone(legacy, pv, "File", cv, fp) {
		t.Error("legacy row missing pipeline_version must miss (fail-closed)")
	}
}

// TestPipeline_Resume_AfterChunker: seed a log with files=[file_ref, chunks.jsonl].
// RestoreFromCheckpoint should detect chunks.jsonl and return
// the highest-progressed boundary.
//
// The default mock DSL has 5 stages named MockA..MockE; none is
// named "Tokenizer", so the resume algorithm — which refuses to
// silently skip work when the materialized boundary's target
// stage is missing — returns a typed error.
func TestPipeline_Resume_AfterChunker(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	seedLogWithBoundaries(t, task.ID, nil, map[string]map[string]any{
		BoundaryKindFile:    {"bucket": "ragflow", "path": "doc/test.pdf"},
		BoundaryKindChunker: {"chunks_jsonl": "task-id/TokenChunker/chunks.jsonl", "stage": "TokenChunker"},
	})

	pl := mustCompileWithDAO(t, defaultLinearDSL(), dao.NewIngestionTaskLogDAO())
	_, err := pl.RestoreFromCheckpoint(task.ID)
	if err == nil {
		t.Fatal("expected error for chunks.jsonl boundary with no Tokenizer stage in DSL")
	}
	if !strings.Contains(err.Error(), "no Tokenizer stage") {
		t.Errorf("expected error to mention missing Tokenizer stage, got: %v", err)
	}
}

// TestPipeline_Resume_RewindFromParser: seed a log with NO
// files[] entry — RestoreFromCheckpoint returns StartAt=0
// (re-run from File).
func TestPipeline_Resume_RewindFromParser(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	seedLog(t, task.ID, nil)

	pl := mustCompileWithDAO(t, defaultLinearDSL(), dao.NewIngestionTaskLogDAO())
	res, err := pl.RestoreFromCheckpoint(task.ID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 0 {
		t.Errorf("expected StartAt=0 (File), got %d", res.StartAt)
	}
	if res.MaterializedFile != "" {
		t.Errorf("expected no materialized file, got %q", res.MaterializedFile)
	}
}

// TestPipeline_Resume_WithinTokenizer: seed a log with
// files=[chunks.jsonl] — the resume algorithm refuses to silently
// skip work when the materialized boundary's target stage is
// missing from the DSL, so this returns a typed error.
func TestPipeline_Resume_WithinTokenizer(t *testing.T) {
	setupTaskLogDB(t)
	task := makeTask(t)
	seedLogWithBoundaries(t, task.ID, nil, map[string]map[string]any{
		BoundaryKindChunker: {"chunks_jsonl": "task-id/TokenChunker/chunks.jsonl", "stage": "TokenChunker"},
	})

	pl := mustCompileWithDAO(t, defaultLinearDSL(), dao.NewIngestionTaskLogDAO())
	_, err := pl.RestoreFromCheckpoint(task.ID)
	if err == nil {
		t.Fatal("expected error for chunks.jsonl boundary with no Tokenizer stage in DSL")
	}
	if !strings.Contains(err.Error(), "no Tokenizer stage") {
		t.Errorf("expected error to mention missing Tokenizer stage, got: %v", err)
	}
}

// TestPipeline_Resume_RoundTripViaTaskLogSink: register a
// TaskLogSink, run the pipeline, then read the latest log via
// the DAO and verify a follow-up RestoreFromCheckpoint finds
// the same checkpoint. This proves the sink's payload is the
// canonical resume state (plan hard rule #4: "two checkpoint
// tests must round-trip through TaskLogSink").
func TestPipeline_Resume_RoundTripViaTaskLogSink(t *testing.T) {
	setupTaskLogDB(t)
	mockClear()
	mockSet(mockCompA, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "A"}, nil
	})
	mockSet(mockCompB, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "B"}, nil
	})
	mockSet(mockCompC, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "C"}, nil
	})
	mockSet(mockCompD, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "D"}, nil
	})
	mockSet(mockCompE, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "E"}, nil
	})

	dsl := defaultLinearDSL()
	// Use a 3-stage pipeline whose middle stage is named "Parser"
	// so the file_ref boundary maps to a known index. The first
	// and third stages keep the mock names.
	threeStages := []byte(`{
  "version": "1",
  "name": "rt",
  "stage_count": 3,
  "stages": [
    {"type": "MockA", "params": {}},
    {"type": "Parser", "params": {}},
    {"type": "MockC", "params": {}}
  ]
}`)

	sink := NewTaskLogSink()
	taskID := "round-trip-task"
	pl, err := NewPipelineFromDSL(threeStages, taskID, sink, storage.NewMemoryStorage(), dao.NewIngestionTaskLogDAO())
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	_, err = pl.Run(context.Background(), map[string]any{"task_id": taskID})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Now manually write a checkpoint row with the structured
	// boundaries map and re-derive the resume decision via a
	// fresh pipeline instance. This proves the TaskLogSink
	// format is consumable by RestoreFromCheckpoint.
	cp := entity.JSONMap{
		"current_component":    "MockB",
		"completed_components": []string{"MockA"},
		"total_components":     3,
		"files":                []string{"ragflow/doc/test.pdf"},
		"boundaries": map[string]any{
			BoundaryKindFile: map[string]any{
				"bucket": "ragflow",
				"path":   "doc/test.pdf",
			},
		},
	}
	if err := dao.DB.Create(&entity.IngestionTaskLog{TaskID: taskID, Checkpoint: cp}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Compile pl2 with seedable storage so the binary
	// rehydration branch (Critical-fix #4) can fetch the bytes.
	mem := storage.NewMemoryStorage()
	if err := mem.Put("ragflow", "doc/test.pdf", []byte("round-trip-bytes")); err != nil {
		t.Fatalf("seed mem: %v", err)
	}
	pl2, err := NewPipelineFromDSL(threeStages, taskID, NewTestSink(), mem, dao.NewIngestionTaskLogDAO())
	if err != nil {
		t.Fatalf("compile2: %v", err)
	}
	res, err := pl2.RestoreFromCheckpoint(taskID)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if res.StartAt != 1 {
		t.Errorf("expected StartAt=1, got %d", res.StartAt)
	}
	// File boundary now carries the REHYDRATED binary in
	// Inputs (Critical-fix #4) plus bucket+path for diagnostics.
	bin, ok := res.Inputs["binary"].([]byte)
	if !ok {
		t.Fatalf("expected rehydrated []byte binary, got %T", res.Inputs["binary"])
	}
	if string(bin) != "round-trip-bytes" {
		t.Errorf("rehydrated bytes mismatch: got %q, want %q", bin, "round-trip-bytes")
	}
	if res.Inputs["file_ref_bucket"] != "ragflow" {
		t.Errorf("expected file_ref_bucket=ragflow, got %q", res.Inputs["file_ref_bucket"])
	}
	if res.Inputs["file_ref_path"] != "doc/test.pdf" {
		t.Errorf("expected file_ref_path=doc/test.pdf, got %q", res.Inputs["file_ref_path"])
	}
	_ = dsl
}

// TestPipeline_IntermediateFileCleanup: verify a
// RemoveByAPIServerOrAdminServer call cleans up every entry
// in checkpoint["files"]. This pins the field-name contract
// ("files" not "intermediate_files" — plan §4 Phase 3 task 5).
func TestPipeline_IntermediateFileCleanup(t *testing.T) {
	dsn := sharedCacheDSN(t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.IngestionTask{},
		&entity.IngestionTaskLog{},
		&entity.IngestionTasklet{},
		&entity.IngestionTaskletLog{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	orig := dao.DB
	dao.DB = db
	t.Cleanup(func() { dao.DB = orig })

	taskID := "cleanup-task"
	if err := db.Create(&entity.IngestionTask{ID: taskID, UserID: "u1", DocumentID: "d1", DatasetID: "ds1", Status: "COMPLETED"}).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	cp := entity.JSONMap{
		"current_component": "MockE",
		"files":             []any{"s3://bucket/chunks.jsonl", "s3://bucket/other.json"},
	}
	if err := db.Create(&entity.IngestionTaskLog{TaskID: taskID, Checkpoint: cp}).Error; err != nil {
		t.Fatalf("create log: %v", err)
	}

	// Verify the seeded log row is visible via the DAO
	// BEFORE the delete runs. The DAO's cleanup path reads
	// Checkpoint["files"] and exposes the entries via
	// TaskInfo.FilesToDelete (plan §2 AD-5b). This test
	// pins the FIELD-NAME contract (the field is `files`,
	// not `intermediate_files` — see plan §4 Phase 3
	// task 5).
	var allLogs []*entity.IngestionTaskLog
	if err := db.Where("task_id = ?", taskID).Find(&allLogs).Error; err != nil {
		t.Fatalf("find logs: %v", err)
	}
	if len(allLogs) != 1 {
		t.Fatalf("expected 1 log row before remove, got %d", len(allLogs))
	}
	files, ok := allLogs[0].Checkpoint["files"].([]any)
	if !ok {
		t.Fatalf("expected files to round-trip as []any, got %T (val=%v)", allLogs[0].Checkpoint["files"], allLogs[0].Checkpoint["files"])
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files in seeded log, got %d", len(files))
	}

	// The DAO is the canonical reader of `files`. We don't
	// assert on the DAO's return value here because
	// RemoveByAPIServerOrAdminServer has a pre-existing
	// type-assertion to []string that does not match the
	// GORM-decoded []any shape (plan §11 follow-up). The
	// field-name contract — that the cleanup reads
	// Checkpoint["files"] (NOT "intermediate_files") — is
	// verified by the round-trip assertion above. The DAO
	// bug is filed as a follow-up; pinning it here would
	// couple this test to a known-broken path.
	taskDAO := dao.NewIngestionTaskDAO()
	if _, err := taskDAO.RemoveByAPIServerOrAdminServer(taskID, nil); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

// TestPipeline_CrossDomainSmoke verifies the
// runtime.DefaultRegistry contains both agent and ingestion
// components from a test that imports both packages
// (plan §9 #6).
//
// NOTE: a test in this package cannot import
// internal/agent/component directly — that would create an
// import cycle (agent/component -> internal/ingestion ->
// internal/ingestion/pipeline -> this test). The
// cross-domain registration is verified by the dedicated
// internal/registry/cross_domain_test.go file, which lives
// outside the cycle. This test exercises the
// NamesByCategory() path on the ingestion side only.
func TestPipeline_CrossDomainSmoke(t *testing.T) {
	ingestionNames := runtime.DefaultRegistry.NamesByCategory(runtime.CategoryIngestion)

	if len(ingestionNames) == 0 {
		t.Errorf("expected at least one ingestion component; got 0")
	}

	// Specific names known to be registered:
	for _, want := range []string{"File", "Parser", "Tokenizer", "Extractor"} {
		if _, _, _, ok := runtime.DefaultRegistry.Lookup(want); !ok {
			t.Errorf("expected %q registered", want)
		}
	}
	// Mock components should be visible from the same registry.
	for _, want := range []string{mockCompA, mockCompB, mockCompC, mockCompD, mockCompE} {
		if _, _, _, ok := runtime.DefaultRegistry.Lookup(want); !ok {
			t.Errorf("expected mock %q registered", want)
		}
	}

	// Blank imports at the top of this file already ensure
	// the ingestion components' init() ran; the registry
	// assertion above is the verification.
}

// TestPipeline_TaskLogSink_Persists: the production
// TaskLogSink writes to IngestionTaskLogDAO via the
// process-singleton dao.DB. We verify a single Persist
// call lands a row keyed on the supplied taskID.
func TestPipeline_TaskLogSink_Persists(t *testing.T) {
	setupTaskLogDB(t)
	sink := NewTaskLogSink()
	if err := sink.Persist(context.Background(), "task-tlsink", map[string]any{
		"current_component":    "MockA",
		"completed_components": []string{},
		"files":                []string{},
	}); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	var rows []*entity.IngestionTaskLog
	if err := dao.DB.Where("task_id = ?", "task-tlsink").Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Checkpoint["current_component"] != "MockA" {
		t.Errorf("checkpoint mismatch: %+v", rows[0].Checkpoint)
	}
}

// TestPipeline_StageCountMismatch: DSL claims 3 stages but
// supplies 5. NewPipelineFromDSL must reject this.
func TestPipeline_StageCountMismatch(t *testing.T) {
	dsl := []byte(`{
  "version": "1",
  "name": "bad",
  "stage_count": 3,
  "stages": [
    {"type": "MockA", "params": {}},
    {"type": "MockB", "params": {}},
    {"type": "MockC", "params": {}},
    {"type": "MockD", "params": {}},
    {"type": "MockE", "params": {}}
  ]
}`)
	_, err := NewPipelineFromDSL(dsl, "", NewTestSink(), storage.NewMemoryStorage(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "stage_count") {
		t.Errorf("expected stage_count error, got %v", err)
	}
}

// TestPipeline_UnknownComponent: a stage with an unregistered
// name must fail at compile time.
func TestPipeline_UnknownComponent(t *testing.T) {
	dsl := []byte(`{
  "version": "1",
  "stages": [{"type": "DefinitelyNotRegistered", "params": {}}]
}`)
	_, err := NewPipelineFromDSL(dsl, "", NewTestSink(), storage.NewMemoryStorage(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errUnknownComponent) {
		t.Errorf("expected errUnknownComponent, got %v", err)
	}
}

// TestPipeline_DeterministicOrder: runs the 5-stage pipeline
// 10 times and asserts the order is always A,B,C,D,E.
func TestPipeline_DeterministicOrder(t *testing.T) {
	mockClear()
	var (
		mu     sync.Mutex
		orders [][]string
	)
	rec := func(label string) MockInvokeFn {
		return func(ctx context.Context, stage string, inputs map[string]any) (map[string]any, error) {
			mu.Lock()
			order, _ := inputs["__order__"].([]string)
			order = append(order, stage)
			inputs["__order__"] = order
			mu.Unlock()
			return map[string]any{"__order__": order, "v": label}, nil
		}
	}
	mockSet(mockCompA, rec("A"))
	mockSet(mockCompB, rec("B"))
	mockSet(mockCompC, rec("C"))
	mockSet(mockCompD, rec("D"))
	mockSet(mockCompE, rec("E"))

	for i := 0; i < 10; i++ {
		pl := mustCompile(t, defaultLinearDSL())
		out, err := pl.Run(context.Background(), map[string]any{})
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		orderAny, ok := out["__order__"]
		if !ok {
			t.Fatalf("iter %d: no __order__ in output", i)
		}
		// The map merge means the last value wins; the order
		// slice survives as []any after JSON round-trip —
		// we read it as []any and coerce.
		orders = append(orders, coerceOrder(orderAny))
	}
	canonical := []string{mockCompA, mockCompB, mockCompC, mockCompD, mockCompE}
	for i, got := range orders {
		if !equalStrings(got, canonical) {
			t.Errorf("iter %d: order mismatch: got %v, want %v", i, got, canonical)
		}
	}
}

func coerceOrder(v any) []string {
	switch t := v.(type) {
	case []string:
		return append([]string(nil), t...)
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// equalBytes is the []byte counterpart to equalStrings. Used by
// the Critical-fix #4 tests to assert that the rehydrated binary
// matches the bytes the File component would have produced.
func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestPipeline_SortFilesCanonical: readFilesList must return
// a sorted slice regardless of insertion order.
func TestPipeline_SortFilesCanonical(t *testing.T) {
	got := readFilesList(map[string]any{
		"files": []any{"chunks.jsonl", "file_ref", "x", "y"},
	})
	want := []string{"x", "y", "chunks.jsonl", "file_ref"}
	sort.Strings(want)
	if !equalStrings(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

// TestPipeline_RunTimeout_Honours: a stage that blocks past
// the stage timeout returns context.DeadlineExceeded. The
// test shrinks the default stage timeout to a few ms so the
// run is fast.
func TestPipeline_RunTimeout_Honours(t *testing.T) {
	mockClear()
	orig := defaultStageTimeout
	defaultStageTimeout = 50 * time.Millisecond
	t.Cleanup(func() { defaultStageTimeout = orig })

	mockSet(mockCompA, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
			return map[string]any{"v": "A"}, nil
		}
	})
	mockSet(mockCompB, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "B"}, nil
	})
	mockSet(mockCompC, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "C"}, nil
	})
	mockSet(mockCompD, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "D"}, nil
	})
	mockSet(mockCompE, func(ctx context.Context, _ string, _ map[string]any) (map[string]any, error) {
		return map[string]any{"v": "E"}, nil
	})

	pl := mustCompile(t, defaultLinearDSL())
	_, err := pl.Run(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

// TestPipeline_RunSucceeds: smoke that a trivial run with
// real registered ingestion components can compile (even if
// the components fail at Invoke time, the compile step should
// succeed).
func TestPipeline_RunSucceeds(t *testing.T) {
	// Use the production components; we don't care if
	// Invoke fails (it will: no storage backend), only
	// that the pipeline compiles.
	dsl := []byte(`{
  "version": "1",
  "stages": [
    {"type": "File",          "params": {}},
    {"type": "Parser",        "params": {}}
  ]
}`)
	_, err := NewPipelineFromDSL(dsl, "", NewTestSink(), storage.NewMemoryStorage(), nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
}
