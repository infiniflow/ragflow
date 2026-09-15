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

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/globals"
	"ragflow/internal/ingestion/knowledge_compile"
	"ragflow/internal/ingestion/pipeline"
)

// TestNewDebugTaskContext_InjectsDebugID asserts the debug context carries the
// side-effect-free markers: a fresh non-empty Doc.ID (a throwaway uuid, not a
// persisted row) and an empty KB (debug has no knowledgebase, so kb_id == "" is
// the debug signal used across the pipeline). The parser page cap is no longer
// stored as a flat ParserConfig key here — flat keys are dropped by the
// override_params merge and never reach the parser. It is injected at run time
// by pipeline.BuildParserPageCapOverride via Run's override_params channel
// (see TestInjectDebugPageCap).
func TestNewDebugTaskContext_InjectsDebugID(t *testing.T) {
	taskCtx := NewDebugTaskContext("t1", "canvas-1", "doc.pdf", []byte("page one\fpage two\fpage three"))

	if taskCtx.Doc.ID == "" {
		t.Errorf("Doc.ID = %q, want non-empty (uuid)", taskCtx.Doc.ID)
	}
	if taskCtx.Doc.ParserConfig != nil {
		t.Errorf("Doc.ParserConfig = %v, want nil (debug page cap is injected via override_params, not a flat ParserConfig key)", taskCtx.Doc.ParserConfig)
	}
	if taskCtx.Doc.KbID != "" {
		t.Errorf("Doc.KbID = %q, want empty (debug has no KB)", taskCtx.Doc.KbID)
	}
	if taskCtx.KB.ID != "" {
		t.Errorf("KB.ID = %q, want empty (debug has no KB)", taskCtx.KB.ID)
	}
	if taskCtx.Tenant.ID != "t1" {
		t.Errorf("Tenant.ID = %q, want t1", taskCtx.Tenant.ID)
	}
	if taskCtx.PipelineID != "canvas-1" {
		t.Errorf("PipelineID = %q, want canvas-1", taskCtx.PipelineID)
	}
}

// TestExecute_DebugViaEntry proves the entry-point constructor produces a
// valid debug TaskContext that routes through PipelineExecutor.Execute and
// returns the pipeline's chunks WITHOUT persisting (no index insert, no
// pipeline log).
func TestExecute_DebugViaEntry(t *testing.T) {
	taskCtx := NewDebugTaskContext("t1", "canvas-1", "doc.pdf", []byte("page one\fpage two\fpage three"))

	logCalled := false
	insertCalled := false

	exec, err := NewPipelineExecutor(taskCtx, "canvas-1", 0)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}
	exec.
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return "dsl", "canvas-1", nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			return map[string]any{
				"chunks": []map[string]any{
					{"text": "c1"},
					{"text": "c2"},
					{"text": "c3"},
					{"text": "c4"},
				},
			}, dsl, nil
		}).
		WithLogCreateFunc(func(ctx context.Context, db *gorm.DB, log *entity.PipelineOperationLog) error {
			logCalled = true
			return nil
		}).
		WithInsertFunc(func(ctx context.Context, chunks []map[string]any, baseName, datasetID string) ([]string, error) {
			insertCalled = true
			return nil, nil
		})

	result, err := exec.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result == nil {
		t.Fatal("result is nil")
	}
	if len(result.Chunks) != 4 {
		t.Errorf("len(result.Chunks) = %d, want 4", len(result.Chunks))
	}
	if logCalled {
		t.Error("pipeline log should NOT be created in a debug (kb_id == \"\") run")
	}
	if insertCalled {
		t.Error("chunk insert should NOT be called in a debug (kb_id == \"\") run")
	}
}

// TestInjectDebugPageCap verifies the canvas-debug page cap is delivered
// through Run's override_params channel (the existing ParserConfig shape),
// NOT through pipeline inputs. The cap must land at
// ParserConfig[cpnID][family]["pages"], expressed as the JSON-decoded
// []any{[]any{1, N}} form (a list of [from,to] pairs) — cpnID is the Parser
// component's instance id from the DSL and family is the document's filetype
// family. This is the exact shape NormalizeParserConfigPages produces after a
// storage JSON round-trip and the shape the deepdoc pdf parser consumes
// (NormalizePDFPages requires []any, not a Go [][]int).
//
// The DSL/parser-family knowledge now lives in pipeline.BuildParserPageCapOverride
// (debug-agnostic); this test drives that helper directly to pin the behavior
// the executor relies on.
//
// It also pins the regression: a flat top-level "pages" key would be dropped
// by the override_params merge and never reach the parser, so the cap must be
// nested under the cpnID.
func TestInjectDebugPageCap(t *testing.T) {
	templatePath := filepath.Join(taskRepoRoot(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	var envelope struct {
		DSL json.RawMessage `json:"dsl"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("unmarshal template envelope: %v", err)
	}
	dsl := string(envelope.DSL)

	// Discover the Parser cpnID the same way the executor does.
	schemas, err := pipeline.ExtractAllComponentParams(envelope.DSL)
	if err != nil {
		t.Fatalf("ExtractAllComponentParams: %v", err)
	}
	var parserCpnID string
	for _, s := range schemas {
		if s.ComponentName == component.ComponentNameParser {
			parserCpnID = s.CpnID
		}
	}
	if parserCpnID == "" {
		t.Fatal("template has no Parser component")
	}

	apply := func(cfg map[string]any, dslArg string, docType string) map[string]any {
		return pipeline.BuildParserPageCapOverride(
			cfg, []byte(dslArg), docType, debugPageCapPages,
			component.ComponentNameParser, component.ParserFileFamily)
	}

	t.Run("pdf injects [1,2] under cpnID+family", func(t *testing.T) {
		parserConfig := map[string]any{}
		apply(parserConfig, dsl, "pdf")
		famEntry, ok := parserConfig[parserCpnID].(map[string]any)
		if !ok {
			t.Fatalf("parserConfig[%q] = %T, want map[string]any", parserCpnID, parserConfig[parserCpnID])
		}
		pdf, ok := famEntry["pdf"].(map[string]any)
		if !ok {
			t.Fatalf("parserConfig[%q][\"pdf\"] = %T, want map[string]any", parserCpnID, famEntry["pdf"])
		}
		if !reflect.DeepEqual(pdf["pages"], []any{[]any{1, debugPageCapPages}}) {
			t.Errorf("parserConfig[%q][\"pdf\"][\"pages\"] = %v, want [[1, %d]]", parserCpnID, pdf["pages"], debugPageCapPages)
		}
	})

	t.Run("docx injects under docx family", func(t *testing.T) {
		parserConfig := map[string]any{}
		apply(parserConfig, dsl, "docx")
		famEntry, ok := parserConfig[parserCpnID].(map[string]any)
		if !ok {
			t.Fatalf("parserConfig[%q] = %T, want map[string]any", parserCpnID, parserConfig[parserCpnID])
		}
		docx, ok := famEntry["docx"].(map[string]any)
		if !ok {
			t.Fatalf("parserConfig[%q][\"docx\"] = %T, want map[string]any", parserCpnID, famEntry["docx"])
		}
		if !reflect.DeepEqual(docx["pages"], []any{[]any{1, debugPageCapPages}}) {
			t.Errorf("parserConfig[%q][\"docx\"][\"pages\"] = %v, want [[1, %d]]", parserCpnID, docx["pages"], debugPageCapPages)
		}
	})

	t.Run("empty docType is a no-op", func(t *testing.T) {
		parserConfig := map[string]any{}
		apply(parserConfig, dsl, "")
		if len(parserConfig) != 0 {
			t.Errorf("parserConfig = %v, want empty (no family derivable from empty docType)", parserConfig)
		}
	})

	t.Run("does not clobber existing parser params", func(t *testing.T) {
		parserConfig := map[string]any{
			parserCpnID: map[string]any{
				"pdf": map[string]any{"parse_method": "deepdoc"},
			},
		}
		apply(parserConfig, dsl, "pdf")
		famEntry := parserConfig[parserCpnID].(map[string]any)
		pdf := famEntry["pdf"].(map[string]any)
		if pdf["parse_method"] != "deepdoc" {
			t.Errorf("parserConfig[%q][\"pdf\"][\"parse_method\"] = %v, want deepdoc (existing params must be preserved)", parserCpnID, pdf["parse_method"])
		}
		if !reflect.DeepEqual(pdf["pages"], []any{[]any{1, debugPageCapPages}}) {
			t.Errorf("parserConfig[%q][\"pdf\"][\"pages\"] = %v, want [[1, %d]]", parserCpnID, pdf["pages"], debugPageCapPages)
		}
	})

	t.Run("envelope dsl form (production shape)", func(t *testing.T) {
		// In production dsl is the canvas envelope {"dsl": {"components": ...}},
		// not the bare components map. The helper must still find the Parser
		// cpnID after unwrapping.
		wrapped := fmt.Sprintf(`{"dsl":%s}`, string(envelope.DSL))
		parserConfig := map[string]any{}
		apply(parserConfig, wrapped, "pdf")
		famEntry, ok := parserConfig[parserCpnID].(map[string]any)
		if !ok {
			t.Fatalf("parserConfig[%q] = %T, want map[string]any (envelope dsl must unwrap)", parserCpnID, parserConfig[parserCpnID])
		}
		pdf, ok := famEntry["pdf"].(map[string]any)
		if !ok {
			t.Fatalf("parserConfig[%q][\"pdf\"] = %T, want map[string]any", parserCpnID, famEntry["pdf"])
		}
		if !reflect.DeepEqual(pdf["pages"], []any{[]any{1, debugPageCapPages}}) {
			t.Errorf("parserConfig[%q][\"pdf\"][\"pages\"] = %v, want [[1, %d]] (envelope dsl not unwrapped?)", parserCpnID, pdf["pages"], debugPageCapPages)
		}
	})

	t.Run("respects explicit caller-supplied cap", func(t *testing.T) {
		// When the document already carries an explicit cpnID+family page cap,
		// the debug default must NOT override it (so a wider/narrower cap wins).
		parserConfig := map[string]any{
			parserCpnID: map[string]any{
				"pdf": map[string]any{"pages": []any{[]any{1, 1000000}}},
			},
		}
		apply(parserConfig, dsl, "pdf")
		famEntry := parserConfig[parserCpnID].(map[string]any)
		pdf := famEntry["pdf"].(map[string]any)
		if !reflect.DeepEqual(pdf["pages"], []any{[]any{1, 1000000}}) {
			t.Errorf("parserConfig[%q][\"pdf\"][\"pages\"] = %v, want [[1, 1000000]] (explicit cap must be respected, not overridden by debug default)", parserCpnID, pdf["pages"])
		}
	})
}

// TestInjectDebugChunkCap pins the canvas-debug chunk cap on the run inputs.
// The cap travels via pipeline inputs → CanvasState.Globals (seeded by
// globals.SeedIngestionGlobals) → the chunker decorator reads it through
// globals.DebugChunkCap. It is injected only in the debug branch of
// runPipelineWithDSL (mirroring the page-cap override). The default is
// DebugChunkCapDefault (3); an explicit caller-supplied value is respected,
// exactly like a caller-supplied page cap.
func TestInjectDebugChunkCap(t *testing.T) {
	t.Run("defaults to DebugChunkCapDefault when absent", func(t *testing.T) {
		inputs := injectDebugChunkCap(map[string]any{"name": "doc.pdf"})
		got, ok := inputs[globals.DebugChunkCapKey].(int)
		if !ok {
			t.Fatalf("inputs[%q] missing or wrong type: %#v", globals.DebugChunkCapKey, inputs[globals.DebugChunkCapKey])
		}
		if got != DebugChunkCapDefault {
			t.Errorf("inputs[%q] = %d, want %d", globals.DebugChunkCapKey, got, DebugChunkCapDefault)
		}
	})

	t.Run("respects explicit caller-supplied cap", func(t *testing.T) {
		inputs := injectDebugChunkCap(map[string]any{
			"name":                   "doc.pdf",
			globals.DebugChunkCapKey: 9,
		})
		if got, _ := inputs[globals.DebugChunkCapKey].(int); got != 9 {
			t.Errorf("inputs[%q] = %d, want 9 (explicit cap must be respected)", globals.DebugChunkCapKey, got)
		}
	})

	t.Run("nil inputs is safe", func(t *testing.T) {
		inputs := injectDebugChunkCap(nil)
		if got, _ := inputs[globals.DebugChunkCapKey].(int); got != DebugChunkCapDefault {
			t.Errorf("inputs[%q] = %d, want %d (nil inputs must be initialized)", globals.DebugChunkCapKey, got, DebugChunkCapDefault)
		}
	})
}

// TestExecute_DebugViaEntry_NoKnowledgeCompilePublish pins requirement #2:
// a canvas-debug (dry-run) run must NOT trigger a dataset-level
// knowledge-compile rebuild (no PublishCompleted / no notification), even when
// the pipeline contains a knowledge-compiler node that completes. The debug
// path returns via collectDebugOutput and never reaches processOutput (the
// only caller of knowledge_compile.PublishCompleted), so a FakeScheduler
// records zero publishes. The compiler component itself is structurally free
// of any publish call (verified by grep), so this test locks the executor's
// routing rather than the component.
func TestExecute_DebugViaEntry_NoKnowledgeCompilePublish(t *testing.T) {
	fake := knowledge_compile.NewFakeScheduler()
	prev := knowledge_compile.DefaultPublisher()
	knowledge_compile.SetScheduler(fake)
	t.Cleanup(func() { knowledge_compile.SetScheduler(prev) })

	taskCtx := NewDebugTaskContext("t1", "canvas-1", "doc.pdf", []byte("page one\fpage two\fpage three"))

	exec, err := NewPipelineExecutor(taskCtx, "canvas-1", 0)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}
	exec.
		WithLoadDSLFunc(func(ctx context.Context, canvasID string) (string, string, error) {
			return "dsl", "canvas-1", nil
		}).
		WithRunPipelineFunc(func(ctx context.Context, dsl string) (map[string]any, string, error) {
			// Simulate a completed knowledge-compiler node plus chunks.
			return map[string]any{
				"chunks": []map[string]any{
					{"text": "c1"},
					{"text": "c2"},
				},
				"state": map[string]any{
					"KnowledgeCompiler:Abc": map[string]any{
						"compile_kwd": map[string]any{"structure": true},
					},
				},
			}, dsl, nil
		})

	if _, err := exec.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if n := fake.PublishedCount(); n != 0 {
		t.Errorf("debug run published %d knowledge-compile event(s); want 0 (no dataset rebuild in debug)", n)
	}
}

// TestWarnUnknownComponentParamsDetectsUnknownCPNFromEnvelope pins the fix for
// the enveloped-DSL no-op bug: warnUnknownComponentParams previously passed the
// raw (enveloped) DSL straight to ExtractAllComponentParams, whose "components"
// key is nested under "dsl", so it errored and silently returned — never
// detecting unknown cpnIDs in production. The helper now unwraps the envelope
// first, so an unknown cpnID in parserConfig is actually surfaced.
func TestWarnUnknownComponentParamsDetectsUnknownCPNFromEnvelope(t *testing.T) {
	core, recorded := observer.New(zap.NewAtomicLevelAt(zap.DebugLevel))
	old := common.Logger
	common.Logger = zap.New(core)
	defer func() { common.Logger = old }()

	// Enveloped DSL (production shape) carrying only a Parser component.
	dsl := `{"dsl": {"components": {"Parser:Abc": {"obj": {"component_name": "Parser", "params": {}}}}}}`
	// parserConfig references a cpnID NOT present in the DSL -> must be warned.
	parserConfig := map[string]any{
		"Parser:Unknown": map[string]any{"pdf": map[string]any{}},
	}

	warnUnknownComponentParams(dsl, parserConfig)

	found := false
	for _, e := range recorded.All() {
		if strings.Contains(e.Message, "Parser:Unknown") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning about unknown cpnID Parser:Unknown (envelope DSL must be unwrapped); got logs: %v", recorded.All())
	}
}

// TestBuildDebugResultDSL_Envelope pins that BuildDebugResultDSL unwraps the
// canvas envelope before reading "components" — the same shared
// pipeline.UnwrapCanvasDSL the cap override and warnUnknownComponentParams
// use. An enveloped DSL (production shape {"dsl": {...}}) must resolve the
// components map exactly like the equivalent raw (non-enveloped) DSL.
func TestBuildDebugResultDSL_Envelope(t *testing.T) {
	const compID = "Parser:Abc"
	rawDSL := `{"components": {"` + compID + `": {"obj": {"component_name": "Parser", "params": {"parse_method": "deepdoc"}}}}}`
	output := map[string]any{
		"state": map[string]any{
			compID: map[string]any{"chunks": []any{map[string]any{"text": "hi"}}},
		},
	}

	// Raw (non-enveloped) DSL.
	rawRes, err := BuildDebugResultDSL(rawDSL, output)
	if err != nil {
		t.Fatalf("raw DSL: %v", err)
	}
	rawComps, ok := rawRes["components"].(map[string]any)
	if !ok {
		t.Fatalf("raw DSL: components missing: %#v", rawRes)
	}
	if _, ok := rawComps[compID]; !ok {
		t.Fatalf("raw DSL: components missing %q: %#v", compID, rawComps)
	}

	// Enveloped DSL (production shape) must unwrap to the same result.
	envDSL := `{"dsl": ` + rawDSL + `}`
	envRes, err := BuildDebugResultDSL(envDSL, output)
	if err != nil {
		t.Fatalf("enveloped DSL: %v", err)
	}
	envComps, ok := envRes["components"].(map[string]any)
	if !ok {
		t.Fatalf("enveloped DSL: components missing (envelope not unwrapped?): %#v", envRes)
	}
	if _, ok := envComps[compID]; !ok {
		t.Fatalf("enveloped DSL: components missing %q (envelope not unwrapped?): %#v", compID, envComps)
	}

	// The two shapes must yield an identical component output.
	if !reflect.DeepEqual(rawComps[compID], envComps[compID]) {
		t.Fatalf("enveloped and raw DSL produced different results:\nraw=%#v\nenv=%#v", rawComps[compID], envComps[compID])
	}
}
