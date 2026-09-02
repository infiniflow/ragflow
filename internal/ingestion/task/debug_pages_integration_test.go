//go:build integration

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

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/deepdoc/parser/pdf"
	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/storage"
)

// TestExecute_DebugViaEntry_HonorsPagesCap_Integration is the end-to-end
// proof that the canvas-debug page cap travels through Run's override_params
// channel (NOT pipeline inputs) and is honoured by the deepdoc/pdf parser.
//
// It parses a multi-page PDF twice through a real debug pipeline (real DSL,
// real pdfium parser, in-memory sqlite + storage):
//
//   - Uncapped baseline: an explicit cpnID+family page cap of [[1, 1000000]]
//     is supplied in ParserConfig. pipeline.BuildParserPageCapOverride must RESPECT it, so the
//     parser reads every page.
//   - Capped: no page cap is supplied, so pipeline.BuildParserPageCapOverride injects the
//     debug default [[1, debugPageCapPages]], and the parser reads only the
//     leading pages.
//
// The capped run must therefore yield strictly fewer chunk characters than
// the uncapped run. This exercises the full path the production code uses
// (Run's 3rd-arg override_params → applyOverrideParams → c.Setups →
// ConfigureFromSetup → deepdoc pdf Config.Pages).
//
// This is gated by //go:build integration because it depends on the native
// pdfium library, the 03_multipage.pdf fixture, and (optionally) the
// tokenizer resource dict.
func TestExecute_DebugViaEntry_HonorsPagesCap_Integration(t *testing.T) {
	requireTokenizerPool(t)
	mustLoadTaskTestConfig(t)

	// The production parse path must never degrade to a mock; install a
	// test-only MockDocAnalyzer as the in-process DeepDoc backend via the
	// public factory seam so the PDF pipeline runs without a real DeepDoc
	// service or ONNX Runtime models. Reset to nil on cleanup (this test
	// binary registers no real backend). The page-cap assertion below relies
	// on pdfium reading the capped/uncapped number of pages — not on layout
	// inference — so a mock analyzer preserves the behaviour under test.
	t.Cleanup(func() { doctype.SetNativeDocAnalyzerFactory(nil) })
	doctype.SetNativeDocAnalyzerFactory(func() (doctype.DocAnalyzer, bool) {
		return &pdf.MockDocAnalyzer{Healthy: true}, true
	})

	origDB := dao.DB
	realDB := mustOpenTaskTestDB(t)
	dao.DB = realDB
	t.Cleanup(func() { dao.DB = origDB })

	realStorage := storage.NewMemoryStorage()
	origStorage := storage.GetStorageFactory().GetStorage()
	storage.GetStorageFactory().SetStorage(realStorage)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(origStorage) })

	templatePath := filepath.Join(taskRepoRoot(t), "internal", "ingestion", "pipeline", "template", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	templateBytes = disableTokenizerEmbeddingForTaskTemplate(t, templateBytes)

	// envelopeDSL is stored verbatim on the canvas (this is what production
	// persists), so loadDSLFromCanvas marshals the envelope and Run receives
	// it. pipeline.BuildParserPageCapOverride must unwrap it to find the Parser cpnID.
	var envelope struct {
		DSL json.RawMessage `json:"dsl"`
	}
	if err := json.Unmarshal(templateBytes, &envelope); err != nil {
		t.Fatalf("unmarshal template envelope: %v", err)
	}
	var canvasDSL entity.JSONMap
	if err := json.Unmarshal(templateBytes, &canvasDSL); err != nil {
		t.Fatalf("unmarshal template into canvas dsl: %v", err)
	}

	// Discover the Parser cpnID from the inner DSL.
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

	pdfPath := filepath.Join(taskRepoRoot(t), "internal", "deepdoc", "parser", "pdf", "testdata", "pdfs", "03_multipage.pdf")
	pdfBytes, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Skipf("read pdf fixture %s: %v", pdfPath, err)
		return
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	tenantID := taskLimit32("it_tenant_" + suffix)
	canvasID := taskLimit32("it_canvas_" + suffix)

	if err := realDB.Create(&entity.UserCanvas{
		ID:             canvasID,
		UserID:         tenantID,
		Permission:     "me",
		CanvasCategory: "agent_canvas",
		DSL:            canvasDSL,
	}).Error; err != nil {
		t.Fatalf("create user canvas: %v", err)
	}
	t.Cleanup(func() { _ = realDB.Where("id = ?", canvasID).Delete(&entity.UserCanvas{}).Error })

	// Explicit "parse all pages" cap (JSON-decoded []any form, the shape the
	// parser actually consumes). pipeline.BuildParserPageCapOverride must respect it and leave
	// it untouched, so the parser reads every page of the PDF.
	allPages := map[string]any{
		parserCpnID: map[string]any{
			"pdf": map[string]any{
				"pages": []any{[]any{1, 1000000}},
			},
		},
	}

	newDebugCtx := func(parserConfig map[string]any) *TaskContext {
		// A canvas-debug (dry-run) context carries no KB: KB.ID == "" is the
		// single debug signal. The executor then skips the persist stage and
		// injects the debug page cap (see pipeline.BuildParserPageCapOverride, gated on
		// KB.ID == "").
		return &TaskContext{
			Doc: entity.Document{
				ID:           "debug-run-1",
				KbID:         "",
				Name:         taskStrPtr("03_multipage.pdf"),
				Type:         "pdf",
				ParserConfig: parserConfig,
			},
			KB: entity.Knowledgebase{
				ID:       "",
				TenantID: tenantID,
				EmbdID:   "embd-1",
			},
			Tenant:     entity.Tenant{ID: tenantID},
			File:       pdfBytes,
			PipelineID: canvasID,
		}
	}

	// Capture warnings during the runs so we assert the envelope-unwrap fix
	// did not regress into noisy warnings: with a well-formed template the
	// parserConfig cpnID matches the DSL, so warnUnknownComponentParams must
	// NOT emit its "not present in the pipeline DSL" warning.
	core, recorded := observer.New(zap.NewAtomicLevelAt(zap.DebugLevel))
	oldLogger := common.Logger
	common.Logger = zap.New(core)
	defer func() { common.Logger = oldLogger }()

	// Uncapped baseline: explicit pages=all → executor respects override.
	uncappedExec, err := NewPipelineExecutor(newDebugCtx(allPages), canvasID, 0)
	if err != nil {
		t.Fatalf("NewPipelineExecutor (uncapped): %v", err)
	}
	uncapped, err := uncappedExec.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute (uncapped): %v", err)
	}

	// Capped: no pages override → executor injects [[1, debugPageCapPages]].
	cappedExec, err := NewPipelineExecutor(newDebugCtx(nil), canvasID, 0)
	if err != nil {
		t.Fatalf("NewPipelineExecutor (capped): %v", err)
	}
	capped, err := cappedExec.Execute(context.Background())
	if err != nil {
		t.Fatalf("Execute (capped): %v", err)
	}

	// A well-formed template's parserConfig cpnID matches the DSL, so the
	// unknown-cpnID guard must stay silent. If it warned here, the envelope
	// was not unwrapped (the pre-fix no-op regression).
	for _, e := range recorded.All() {
		if strings.Contains(e.Message, "not present in the pipeline DSL") {
			t.Errorf("warnUnknownComponentParams emitted an unknown-cpnID warning during a well-formed debug run (envelope-unwrap regression?): %s", e.Message)
		}
	}

	uncappedLen := len(joinedChunks(uncapped.Chunks))
	cappedLen := len(joinedChunks(capped.Chunks))
	if uncappedLen == 0 {
		t.Fatal("uncapped baseline produced no chunks; fixture/pipeline misconfigured")
	}
	if cappedLen == 0 {
		t.Fatal("pages cap produced no chunks; pages wiring broken")
	}
	if cappedLen >= uncappedLen {
		t.Errorf("pages cap had no effect: capped text len %d >= uncapped %d (pages not honoured via override_params)", cappedLen, uncappedLen)
	}
}

// joinedChunks concatenates the text of every chunk into a single string so
// the page-cap assertion does not depend on how the chunker splits pages.
func joinedChunks(chunks []map[string]any) string {
	var b strings.Builder
	for _, ck := range chunks {
		for _, key := range []string{"content_with_weight", "content", "text"} {
			if v, ok := ck[key].(string); ok && v != "" {
				b.WriteString(v)
				b.WriteString("\n")
				break
			}
		}
	}
	return b.String()
}
