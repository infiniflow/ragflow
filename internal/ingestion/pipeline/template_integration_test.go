//	Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//	Licensed under the Apache License, Version 2.0 (the "License");
//	you may not use this file except in compliance with the License.
//	You may obtain a copy of the License at
//
//	    http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS,
//	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	See the License for the specific language governing permissions and
//	limitations under the License.

// These tests exercise the real File -> Parser -> Chunker -> Tokenizer chain
// end-to-end. They double as the regression test for run-level `name`
// propagation: the Tokenizer reads `name` from CanvasState.Globals (via
// globals.GlobalOrInput), so the filename resolved by File reaches the
// Tokenizer and title weighting runs - the q_<n>_vec first element is
// 0.1*len(name) + 0.9*len(content), and embedding_token_consumption includes
// the one title encode (len(name)) plus the per-chunk content encodes.
package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	componentpkg "ragflow/internal/ingestion/component"
	_ "ragflow/internal/ingestion/component/chunker"
	"ragflow/internal/storage"

	"github.com/signintech/gopdf"
)

type fixedEmbedder struct{}

func (fixedEmbedder) MaxTokens() int { return 0 }

func (fixedEmbedder) Encode(texts []string) ([]componentpkg.EmbeddingResult, error) {
	out := make([]componentpkg.EmbeddingResult, 0, len(texts))
	for _, text := range texts {
		out = append(out, componentpkg.EmbeddingResult{
			Vector:     []float64{float64(len(text)), 1, 2, 3},
			TokenCount: len(text),
		})
	}
	return out, nil
}

func TestPipelineRun_TemplateGeneral_RealComponents(t *testing.T) {

	RequireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_general.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:LegalReadersDecide" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:LegalReadersDecide]", terminalIDs)
	}

	mem := withRealTemplateDeps(t)
	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-general.txt"
		filename = "template-general.txt"
	)
	content := "Alpha paragraph.\n\nBeta paragraph."
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-general-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])

	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	wantChunkTexts := []string{"Alpha paragraph.", "Beta paragraph."}
	if len(chunks) != len(wantChunkTexts) {
		t.Fatalf("len(chunks) = %d, want %d", len(chunks), len(wantChunkTexts))
	}
	totalTokens := 0
	for i, wantText := range wantChunkTexts {
		if got := chunks[i]["text"]; got != wantText {
			t.Fatalf("chunks[%d].text = %v, want %q", i, got, wantText)
		}
		if _, ok := chunks[i]["content_ltks"].(string); !ok || chunks[i]["content_ltks"] == "" {
			t.Fatalf("chunks[%d].content_ltks missing or empty: %v", i, chunks[i]["content_ltks"])
		}
		if _, ok := chunks[i]["content_sm_ltks"].(string); !ok || chunks[i]["content_sm_ltks"] == "" {
			t.Fatalf("chunks[%d].content_sm_ltks missing or empty: %v", i, chunks[i]["content_sm_ltks"])
		}
		vec := floatSliceFromAny(t, chunks[i]["q_4_vec"])
		wantFirst := expectedFixedEmbedderFirst(filename, wantText)
		if len(vec) != 4 || !approxFloat(vec[0], wantFirst) {
			t.Fatalf("chunks[%d].q_4_vec = %v, want first=%v", i, vec, wantFirst)
		}
		totalTokens += len(wantText)
	}
	if got := payload["embedding_token_consumption"]; got != totalTokens+len(filename) {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, totalTokens+len(filename))
	}

	state := stateFromRunOutput(t, out)
	chunkerState, ok := state["TokenChunker:SixApplesFall"]
	if !ok {
		t.Fatal("missing TokenChunker:SixApplesFall state")
	}
	if got := chunkerState["output_format"]; got != "chunks" {
		t.Fatalf("chunker output_format = %v, want chunks", got)
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != len(wantChunkTexts) {
		t.Fatalf("chunker chunks = %T/%v, want %d items", chunkerState["chunks"], chunkerState["chunks"], len(wantChunkTexts))
	}
	for i, wantText := range wantChunkTexts {
		if got := chunkerChunks[i]["text"]; got != wantText {
			t.Fatalf("chunker chunk[%d].text = %v, want %q", i, got, wantText)
		}
		if got := chunkerChunks[i]["doc_type_kwd"]; got != "text" {
			t.Fatalf("chunker chunk[%d].doc_type_kwd = %v, want text", i, got)
		}
	}
}

func TestPipelineRun_TemplateOne_RealComponents(t *testing.T) {
	RequireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_one.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:FrankWeeksListen" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:FrankWeeksListen]", terminalIDs)
	}

	mem := withRealTemplateDeps(t)

	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-one.txt"
		filename = "template-one.txt"
	)
	content := "Alpha paragraph.\n\nBeta paragraph."
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-one-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])

	wantTexts := []string{"Alpha paragraph.", "Beta paragraph."}
	wantMergedText := "Alpha paragraph.\nBeta paragraph."
	assertTokenizerTerminalChunk(t, payload, filename, wantMergedText)

	state := stateFromRunOutput(t, out)
	fileState, ok := state["File"]
	if !ok {
		t.Fatal("missing File state")
	}
	if got := fileState["name"]; got != filename {
		t.Fatalf("file state name = %v, want %q", got, filename)
	}
	if _, ok := fileState["bucket"]; ok {
		t.Fatalf("file state should not expose bucket on doc_id path: %v", fileState["bucket"])
	}
	if _, ok := fileState["path"]; ok {
		t.Fatalf("file state should not expose path on doc_id path: %v", fileState["path"])
	}
	parserState, ok := state["Parser:HipSignsRhyme"]
	if !ok {
		t.Fatal("missing Parser:HipSignsRhyme state")
	}
	if got := parserState["output_format"]; got != "json" {
		t.Fatalf("parser output_format = %v, want json", got)
	}
	jsonItems, ok := parserState["json"].([]map[string]any)
	if !ok || len(jsonItems) != 2 {
		t.Fatalf("parser json = %T/%v, want 2 items", parserState["json"], parserState["json"])
	}
	for i, wantText := range wantTexts {
		item := jsonItems[i]
		if got := item["text"]; got != wantText {
			t.Fatalf("parser json[%d].text = %v, want %q", i, got, wantText)
		}
	}
	chunkerState, ok := state["OneChunker:DryDrinksVisit"]
	if !ok {
		t.Fatal("missing OneChunker:DryDrinksVisit state")
	}
	if got := chunkerState["output_format"]; got != "chunks" {
		t.Fatalf("chunker output_format = %v, want chunks", got)
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != 1 {
		t.Fatalf("chunker chunks = %T/%v, want 1 item", chunkerState["chunks"], chunkerState["chunks"])
	}
	chunkerChunk := chunkerChunks[0]
	if got := chunkerChunk["text"]; got != wantMergedText {
		t.Fatalf("chunker chunk[0].text = %v, want %q", got, wantMergedText)
	}
}

func TestPipelineRun_TemplateOne_RealComponents_PDFDeepdocChunking(t *testing.T) {
	RequireTokenizerPool(t)
	t.Setenv("DEEPDOC_URL", "")
	t.Setenv("OSSDEEPDOC_URL", "")

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_one.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:FrankWeeksListen" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:FrankWeeksListen]", terminalIDs)
	}

	fixture := loadTemplatePipelinePDFFixture(t)
	mem := withRealTemplateDeps(t)
	const (
		bucket = "test-bucket"
		path   = "fixtures/template-one.pdf"
	)
	docID := seedTemplateDocumentBytes(t, mem, fixture.Name, bucket, path, fixture.Bytes)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-one-pdf-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok || len(chunks) != 1 {
		t.Fatalf("chunks = %T/%v, want 1 merged chunk", payload["chunks"], payload["chunks"])
	}
	chunkText, _ := chunks[0]["text"].(string)
	assertNormalizedContainsAll(t, chunkText, fixture.ExpectContains...)
	if _, ok := chunks[0]["content_ltks"].(string); !ok || chunks[0]["content_ltks"] == "" {
		t.Fatalf("chunks[0].content_ltks missing or empty: %v", chunks[0]["content_ltks"])
	}
	if _, ok := chunks[0]["content_sm_ltks"].(string); !ok || chunks[0]["content_sm_ltks"] == "" {
		t.Fatalf("chunks[0].content_sm_ltks missing or empty: %v", chunks[0]["content_sm_ltks"])
	}
	vec := floatSliceFromAny(t, chunks[0]["q_4_vec"])
	trimmedChunkText := strings.TrimSpace(chunkText)
	wantFirst := expectedFixedEmbedderFirst(fixture.Name, trimmedChunkText)
	if len(vec) != 4 || !approxFloat(vec[0], wantFirst) {
		t.Fatalf("chunks[0].q_4_vec = %v, want first=%v", vec, wantFirst)
	}
	wantTokens := len(fixture.Name) + len(trimmedChunkText)
	if got := payload["embedding_token_consumption"]; got != wantTokens {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, wantTokens)
	}

	state := stateFromRunOutput(t, out)
	parserState, ok := state["Parser:HipSignsRhyme"]
	if !ok {
		t.Fatal("missing Parser:HipSignsRhyme state")
	}
	if got := parserState["output_format"]; got != "json" {
		t.Fatalf("parser output_format = %v, want json", got)
	}
	fileState, ok := parserState["file"].(map[string]any)
	if !ok {
		t.Fatalf("parser file = %T, want map[string]any", parserState["file"])
	}
	if got := fileState["name"]; got != fixture.Name {
		t.Fatalf("parser file.name = %v, want %q", got, fixture.Name)
	}
	if got := fileState["page_count"]; got != fixture.PageCount {
		t.Fatalf("parser file.page_count = %v, want %d", got, fixture.PageCount)
	}
	jsonItems, ok := parserState["json"].([]map[string]any)
	if !ok || len(jsonItems) == 0 {
		t.Fatalf("parser json = %T/%v, want non-empty []map[string]any", parserState["json"], parserState["json"])
	}
	parserJoined := joinJSONItemTexts(jsonItems)
	assertNormalizedContainsAll(t, parserJoined, fixture.ExpectContains...)

	chunkerState, ok := state["OneChunker:DryDrinksVisit"]
	if !ok {
		t.Fatal("missing OneChunker:DryDrinksVisit state")
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != 1 {
		t.Fatalf("chunker chunks = %T/%v, want 1 item", chunkerState["chunks"], chunkerState["chunks"])
	}
	if got := chunkerChunks[0]["text"]; got != chunkText {
		t.Fatalf("chunker chunk text != tokenizer chunk text:\nchunker=%q\ntokenizer=%q", got, chunkText)
	}
}

func TestPipelineRun_TemplateManual_RealComponents(t *testing.T) {
	RequireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_manual.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:FunnyBalloonsGrin" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:FunnyBalloonsGrin]", terminalIDs)
	}

	mem := withRealTemplateDeps(t)
	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-manual.txt"
		filename = "template-manual.txt"
	)
	content := "PART ONE\n\nIntro paragraph.\n\nSection 1\n\nDetail paragraph.\n\nPART TWO\n\nTail paragraph."
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-manual-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	wantChunkTexts := []string{
		"PART ONE\nIntro paragraph.\nSection 1\nDetail paragraph.\nPART TWO\nTail paragraph.\n",
	}
	if len(chunks) != len(wantChunkTexts) {
		t.Fatalf("len(chunks) = %d, want %d; chunks=%v", len(chunks), len(wantChunkTexts), chunks)
	}
	totalTokens := 0
	for i, wantText := range wantChunkTexts {
		if got := chunks[i]["text"]; got != wantText {
			t.Fatalf("chunks[%d].text = %v, want %q", i, got, wantText)
		}
		if _, ok := chunks[i]["content_ltks"].(string); !ok || chunks[i]["content_ltks"] == "" {
			t.Fatalf("chunks[%d].content_ltks missing or empty: %v", i, chunks[i]["content_ltks"])
		}
		vec := floatSliceFromAny(t, chunks[i]["q_4_vec"])
		wantEmbedText := strings.TrimSpace(wantText)
		wantFirst := expectedFixedEmbedderFirst(filename, wantEmbedText)
		if len(vec) != 4 || !approxFloat(vec[0], wantFirst) {
			t.Fatalf("chunks[%d].q_4_vec = %v, want first=%v", i, vec, wantFirst)
		}
		totalTokens += len(wantEmbedText)
	}
	if got := payload["embedding_token_consumption"]; got != totalTokens+len(filename) {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, totalTokens+len(filename))
	}

	state := stateFromRunOutput(t, out)
	parserState, ok := state["Parser:HipSignsRhyme"]
	if !ok {
		t.Fatal("missing Parser:HipSignsRhyme state")
	}
	jsonItems, ok := parserState["json"].([]map[string]any)
	if !ok || len(jsonItems) != 6 {
		t.Fatalf("parser json = %T/%v, want 6 items", parserState["json"], parserState["json"])
	}
	wantParserTexts := []string{"PART ONE", "Intro paragraph.", "Section 1", "Detail paragraph.", "PART TWO", "Tail paragraph."}
	for i, wantText := range wantParserTexts {
		if got := jsonItems[i]["text"]; got != wantText {
			t.Fatalf("parser json[%d].text = %v, want %q", i, got, wantText)
		}
	}
	chunkerState, ok := state["TitleChunker:NineInsectsFind"]
	if !ok {
		t.Fatal("missing TitleChunker:NineInsectsFind state")
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != len(wantChunkTexts) {
		t.Fatalf("chunker chunks = %T/%v, want %d items", chunkerState["chunks"], chunkerState["chunks"], len(wantChunkTexts))
	}
	for i, wantText := range wantChunkTexts {
		if got := chunkerChunks[i]["text"]; got != wantText {
			t.Fatalf("chunker chunk[%d].text = %v, want %q", i, got, wantText)
		}
	}
}

func TestPipelineRun_TemplateLaws_RealComponents(t *testing.T) {
	RequireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_laws.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:PublicJobsTake" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:PublicJobsTake]", terminalIDs)
	}

	mem := withRealTemplateDeps(t)
	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-laws.txt"
		filename = "template-laws.txt"
	)
	content := "PART ONE\n\nIntro\n\nSection 1\n\nClause A.\n\nSection 2\n\nClause B.\n\nPART TWO\n\nIntro 2.\n\nSection 3\n\nClause C."
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-laws-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	wantChunkTexts := []string{
		"PART ONE\nIntro\nSection 1\nClause A.\n",
		"PART ONE\nIntro\nSection 2\nClause B.\n",
		"PART TWO\nIntro 2.\nSection 3\nClause C.\n",
	}
	if len(chunks) != len(wantChunkTexts) {
		t.Fatalf("len(chunks) = %d, want %d", len(chunks), len(wantChunkTexts))
	}
	totalTokens := 0
	for i, wantText := range wantChunkTexts {
		if got := chunks[i]["text"]; got != wantText {
			t.Fatalf("chunks[%d].text = %v, want %q", i, got, wantText)
		}
		vec := floatSliceFromAny(t, chunks[i]["q_4_vec"])
		wantEmbedText := strings.TrimSpace(wantText)
		wantFirst := expectedFixedEmbedderFirst(filename, wantEmbedText)
		if len(vec) != 4 || !approxFloat(vec[0], wantFirst) {
			t.Fatalf("chunks[%d].q_4_vec = %v, want first=%v", i, vec, wantFirst)
		}
		totalTokens += len(wantEmbedText)
	}
	if got := payload["embedding_token_consumption"]; got != totalTokens+len(filename) {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, totalTokens+len(filename))
	}

	state := stateFromRunOutput(t, out)
	chunkerState, ok := state["TitleChunker:SpicyKeysKick"]
	if !ok {
		t.Fatal("missing TitleChunker:SpicyKeysKick state")
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != len(wantChunkTexts) {
		t.Fatalf("chunker chunks = %T/%v, want %d items", chunkerState["chunks"], chunkerState["chunks"], len(wantChunkTexts))
	}
	for i, wantText := range wantChunkTexts {
		if got := chunkerChunks[i]["text"]; got != wantText {
			t.Fatalf("chunker chunk[%d].text = %v, want %q", i, got, wantText)
		}
	}
}

func TestPipelineRun_TemplatePaper_RealComponents(t *testing.T) {
	RequireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_paper.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:GreatCarsWash" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:GreatCarsWash]", terminalIDs)
	}

	mem := withRealTemplateDeps(t)
	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-paper.txt"
		filename = "template-paper.txt"
	)
	content := "PART ONE\n\nAbstract paragraph.\n\nSection 1\n\nMethod paragraph.\n\nSection 2\n\nResult paragraph.\n\nPART TWO\n\nDiscussion paragraph."
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-paper-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	wantChunkTexts := []string{
		"PART ONE\nAbstract paragraph.\nSection 1\nMethod paragraph.\nSection 2\nResult paragraph.\nPART TWO\nDiscussion paragraph.\n",
	}
	if len(chunks) != len(wantChunkTexts) {
		t.Fatalf("len(chunks) = %d, want %d; chunks=%v", len(chunks), len(wantChunkTexts), chunks)
	}
	totalTokens := 0
	for i, wantText := range wantChunkTexts {
		if got := chunks[i]["text"]; got != wantText {
			t.Fatalf("chunks[%d].text = %v, want %q", i, got, wantText)
		}
		wantEmbedText := strings.TrimSpace(wantText)
		vec := floatSliceFromAny(t, chunks[i]["q_4_vec"])
		wantFirst := expectedFixedEmbedderFirst(filename, wantEmbedText)
		if len(vec) != 4 || !approxFloat(vec[0], wantFirst) {
			t.Fatalf("chunks[%d].q_4_vec = %v, want first=%v", i, vec, wantFirst)
		}
		totalTokens += len(wantEmbedText)
	}
	if got := payload["embedding_token_consumption"]; got != totalTokens+len(filename) {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, totalTokens+len(filename))
	}

	state := stateFromRunOutput(t, out)
	chunkerState, ok := state["TitleChunker:SparklySchoolsTravel"]
	if !ok {
		t.Fatal("missing TitleChunker:SparklySchoolsTravel state")
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != len(wantChunkTexts) {
		t.Fatalf("chunker chunks = %T/%v, want %d items", chunkerState["chunks"], chunkerState["chunks"], len(wantChunkTexts))
	}
	for i, wantText := range wantChunkTexts {
		if got := chunkerChunks[i]["text"]; got != wantText {
			t.Fatalf("chunker chunk[%d].text = %v, want %q", i, got, wantText)
		}
	}
}

func TestPipelineRun_TemplateBook_RealComponents(t *testing.T) {
	RequireTokenizerPool(t)

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_book.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:HotDonutsRing" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:HotDonutsRing]", terminalIDs)
	}

	mem := withRealTemplateDeps(t)
	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-book.txt"
		filename = "template-book.txt"
	)
	content := "PART ONE\n\nPrelude.\n\nChapter I\n\nOpening.\n\nSection 1\n\nDetail.\n\nArticle 1\n\nClause A.\n\nArticle 2\n\nClause B.\n\nPART TWO\n\nAfterword."
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-book-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	wantChunkTexts := []string{
		"PART ONE\nPrelude.\n",
		"PART ONE\nChapter I\nOpening.\n",
		"PART ONE\nChapter I\nSection 1\nDetail.\n",
		"PART ONE\nChapter I\nSection 1\nArticle 1\nClause A.\n",
		"PART ONE\nChapter I\nSection 1\nArticle 2\nClause B.\n",
		"PART TWO\nAfterword.\n",
	}
	if len(chunks) != len(wantChunkTexts) {
		t.Fatalf("len(chunks) = %d, want %d; chunks=%v", len(chunks), len(wantChunkTexts), chunks)
	}
	totalTokens := 0
	for i, wantText := range wantChunkTexts {
		if got := chunks[i]["text"]; got != wantText {
			t.Fatalf("chunks[%d].text = %v, want %q", i, got, wantText)
		}
		wantEmbedText := strings.TrimSpace(wantText)
		vec := floatSliceFromAny(t, chunks[i]["q_4_vec"])
		wantFirst := expectedFixedEmbedderFirst(filename, wantEmbedText)
		if len(vec) != 4 || !approxFloat(vec[0], wantFirst) {
			t.Fatalf("chunks[%d].q_4_vec = %v, want first=%v", i, vec, wantFirst)
		}
		totalTokens += len(wantEmbedText)
	}
	if got := payload["embedding_token_consumption"]; got != totalTokens+len(filename) {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, totalTokens+len(filename))
	}

	state := stateFromRunOutput(t, out)
	chunkerState, ok := state["TitleChunker:GrumpyGarlicsBake"]
	if !ok {
		t.Fatal("missing TitleChunker:GrumpyGarlicsBake state")
	}
	chunkerChunks, ok := chunkerState["chunks"].([]map[string]any)
	if !ok || len(chunkerChunks) != len(wantChunkTexts) {
		t.Fatalf("chunker chunks = %T/%v, want %d items", chunkerState["chunks"], chunkerState["chunks"], len(wantChunkTexts))
	}
	for i, wantText := range wantChunkTexts {
		if got := chunkerChunks[i]["text"]; got != wantText {
			t.Fatalf("chunker chunk[%d].text = %v, want %q", i, got, wantText)
		}
	}
}

func TestPipelineRun_TemplateResume_RealComponents(t *testing.T) {
	RequireTokenizerPool(t)
	apiKey := common.GetEnv(common.EnvOpenAIApiKey)
	baseURL := common.GetEnv(common.EnvOpenAIBaseURL)
	model := common.GetEnv(common.EnvOpenAIModel)
	if apiKey == "" || baseURL == "" || model == "" {
		t.Skip("missing required env (OPENAI_API_KEY/OPENAI_BASE_URL/OPENAI_MODEL); skipping real resume extractor integration test")
	}

	templatePath := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_resume.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
	if len(terminalIDs) != 1 || terminalIDs[0] != "Tokenizer:KindHandsWin" {
		t.Fatalf("terminal ids = %v, want [Tokenizer:KindHandsWin]", terminalIDs)
	}

	mem := withRealTemplateDeps(t)
	componentpkg.SetExtractorChatTargetResolverOverride(func(llmID string) (driver, modelName, apiKeyOut, baseURLOut string, ok bool) {
		return "openai", model, apiKey, baseURL, true
	})
	t.Cleanup(func() { componentpkg.SetExtractorChatTargetResolverOverride(nil) })

	const (
		bucket   = "test-bucket"
		path     = "fixtures/template-resume.txt"
		filename = "template-resume.txt"
	)
	content := strings.Join([]string{
		"PERSONAL INFORMATION",
		"",
		"John Example",
		"Email: john.example@resume.test",
		"Phone: +1 555 000 1234",
		"City: Seattle",
		"",
		"EDUCATION",
		"",
		"Bachelor of Science in Computer Science",
		"Example University",
		"Graduation Year: 2024",
		"",
		"WORK EXPERIENCE",
		"",
		"Software Engineer",
		"Example Corp",
		"2024 - Present",
		"",
		"SKILLS",
		"",
		"Go",
		"Python",
		"Kubernetes",
	}, "\n")
	docID := seedTemplateDocument(t, mem, filename, bucket, path, content)

	pipe, err := NewPipelineFromDSL(templateBytes, "template-resume-real")
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	attachFixedEmbedderFactory(t, pipe)
	out, err := pipe.Run(context.Background(), map[string]any{
		"doc_id": docID,
		"llm_id": model + "@openai",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok || len(chunks) == 0 {
		t.Fatalf("chunks = %T/%v, want non-empty []map[string]any", payload["chunks"], payload["chunks"])
	}

	assertExtractedMetadataContains(t, chunks[0]["metadata"], "candidate_name", "John Example")
	assertExtractedMetadataContains(t, chunks[0]["metadata"], "email", "john.example@resume.test")
	assertExtractedMetadataContains(t, chunks[0]["metadata"], "phone", "+1 555 000 1234")

	state := stateFromRunOutput(t, out)
	extractorState, ok := state["Extractor:ThreeDrinksAct"]
	if !ok {
		t.Fatal("missing Extractor:ThreeDrinksAct state")
	}
	extractorChunks, ok := extractorState["chunks"].([]map[string]any)
	if !ok || len(extractorChunks) == 0 {
		t.Fatalf("extractor chunks = %T/%v, want non-empty []map[string]any", extractorState["chunks"], extractorState["chunks"])
	}
	assertExtractedMetadataContains(t, extractorChunks[0]["metadata"], "candidate_name", "John Example")
	assertExtractedMetadataContains(t, extractorChunks[0]["metadata"], "email", "john.example@resume.test")
}

func TestPipelineRun_AllIngestionTemplates_RealComponentsSmoke(t *testing.T) {
	RequireTokenizerPool(t)

	mem := withRealTemplateDeps(t)

	const (
		bucket = "test-bucket"
		path   = "fixtures/template-smoke.md"
	)
	content := "# Title\n\nIntro paragraph.\n\n## Section\n\nBody paragraph."
	docID := seedTemplateDocument(t, mem, "template-smoke.md", bucket, path, content)

	files, err := filepath.Glob(filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_*.json"))
	if err != nil {
		t.Fatalf("glob templates: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatal("no ingestion templates found")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			templateBytes, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			if templateUsesComponent(t, templateBytes, "Extractor") {
				t.Skip("template includes real Extractor and requires model credentials; covered separately from File/Parser/Chunker/Tokenizer e2e")
			}
			if templateUsesComponent(t, templateBytes, "QAChunker") {
				t.Skip("template uses QAChunker which requires Q&A-structured content; covered separately")
			}
			if templateUsesComponent(t, templateBytes, "TagChunker") {
				t.Skip("template uses TagChunker which requires tag-structured content and parser setups not available for generic .md input; covered separately")
			}
			terminalIDs := terminalComponentIDsFromTemplate(t, templateBytes)
			if len(terminalIDs) != 1 {
				t.Fatalf("terminal ids = %v, want exactly 1 terminal", terminalIDs)
			}
			pipe, err := NewPipelineFromDSL(templateBytes, filepath.Base(file))
			if err != nil {
				t.Fatalf("NewPipelineFromDSL: %v", err)
			}
			attachFixedEmbedderFactory(t, pipe)
			out, err := pipe.Run(context.Background(), map[string]any{
				"doc_id": docID,
			})
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			payload := terminalPayloadFromRunOutput(t, out, terminalIDs[0])
			if got := payload["output_format"]; got != "chunks" {
				t.Fatalf("output_format = %v, want chunks", got)
			}
			chunks, ok := payload["chunks"].([]map[string]any)
			if !ok || len(chunks) == 0 {
				t.Fatalf("chunks = %T/%v, want non-empty []map[string]any", payload["chunks"], payload["chunks"])
			}
		})
	}
}

func attachFixedEmbedderFactory(t *testing.T, pipe *Pipeline) {
	t.Helper()
	pipe.WithComponentFactory(func(name string, params map[string]any) (runtime.Component, error) {
		if name == componentpkg.ComponentNameTokenizer {
			return componentpkg.NewTokenizerComponentWithResolver(params, func(_, _, _ string) (componentpkg.Embedder, error) {
				return fixedEmbedder{}, nil
			})
		}
		factory, _, _, ok := runtime.DefaultRegistry.Lookup(name)
		if !ok {
			return nil, fmt.Errorf("runtime: unknown component %q", name)
		}
		return factory(name, params)
	})
}

func withRealTemplateDeps(t *testing.T) storage.Storage {
	t.Helper()

	origStorage := storage.GetStorageFactory().GetStorage()
	mem := storage.NewMemoryStorage()
	storage.GetStorageFactory().SetStorage(mem)
	t.Cleanup(func() { storage.GetStorageFactory().SetStorage(origStorage) })

	refs := map[string]componentpkg.DocumentStorageRef{}
	componentpkg.ResolveDocumentStorageOverride = func(docID string) (*componentpkg.DocumentStorageRef, error) {
		ref, ok := refs[docID]
		if !ok {
			return nil, fmt.Errorf("unknown doc_id %q", docID)
		}
		copy := ref
		return &copy, nil
	}
	t.Cleanup(func() { componentpkg.ResolveDocumentStorageOverride = nil })
	registerTemplateDocumentRef = func(docID string, ref componentpkg.DocumentStorageRef) {
		refs[docID] = ref
	}
	t.Cleanup(func() { registerTemplateDocumentRef = nil })

	return mem
}

var registerTemplateDocumentRef func(docID string, ref componentpkg.DocumentStorageRef)

func seedTemplateDocument(t *testing.T, stg storage.Storage, name, bucket, path, content string) string {
	t.Helper()
	return seedTemplateDocumentBytes(t, stg, name, bucket, path, []byte(content))
}

func seedTemplateDocumentBytes(t *testing.T, stg storage.Storage, name, bucket, path string, content []byte) string {
	t.Helper()
	if err := stg.Put(bucket, path, content); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	if registerTemplateDocumentRef == nil {
		t.Fatal("template doc resolver not installed")
	}
	docID := strings.NewReplacer("/", "-", " ", "-", ":", "-").Replace(t.Name()) + ":" + name
	registerTemplateDocumentRef(docID, componentpkg.DocumentStorageRef{
		Name:   name,
		Bucket: bucket,
		Path:   path,
	})
	return docID
}

type templatePDFFixture struct {
	Name           string
	Bytes          []byte
	PageCount      int
	ExpectContains []string
}

func loadTemplatePipelinePDFFixture(t *testing.T) templatePDFFixture {
	t.Helper()
	data, err := generateTemplatePipelinePDF()
	if err != nil {
		t.Fatalf("generate pdf fixture: %v", err)
	}
	return templatePDFFixture{
		Name:      "generated-6pages.pdf",
		Bytes:     data,
		PageCount: 6,
		ExpectContains: []string{
			"Pipeline PDF Fixture",
			"Page 1 explains why deepdoc parsing matters for chunking.",
			"Page 3 ensures the parser crosses page boundaries correctly.",
			"Page 6 confirms the tokenizer sees one merged chunk at the end.",
		},
	}
}

func generateTemplatePipelinePDF() ([]byte, error) {
	fontPath, err := findTemplatePDFFont()
	if err != nil {
		return nil, err
	}
	pages := []string{
		"Pipeline PDF Fixture\nPage 1 explains why deepdoc parsing matters for chunking.",
		"Pipeline PDF Fixture\nPage 2 keeps a second page in the document for integration coverage.",
		"Pipeline PDF Fixture\nPage 3 ensures the parser crosses page boundaries correctly.",
		"Pipeline PDF Fixture\nPage 4 adds enough content to resemble a real multi-page document.",
		"Pipeline PDF Fixture\nPage 5 verifies the chunker does not drop later pages.",
		"Pipeline PDF Fixture\nPage 6 confirms the tokenizer sees one merged chunk at the end.",
	}

	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	if err := pdf.AddTTFFont("fixture", fontPath); err != nil {
		return nil, fmt.Errorf("AddTTFFont: %w", err)
	}
	for _, pageText := range pages {
		pdf.AddPage()
		if err := pdf.SetFont("fixture", "", 16); err != nil {
			return nil, fmt.Errorf("SetFont title: %w", err)
		}
		pdf.SetXY(56, 72)
		parts := strings.Split(pageText, "\n")
		for idx, line := range parts {
			if idx == 1 {
				if err := pdf.SetFont("fixture", "", 12); err != nil {
					return nil, fmt.Errorf("SetFont body: %w", err)
				}
				pdf.SetXY(56, 104)
			}
			if err := pdf.Text(line); err != nil {
				return nil, fmt.Errorf("Text(%q): %w", line, err)
			}
			pdf.Br(20)
		}
	}
	return bytes.Clone(pdf.GetBytesPdf()), nil
}

func findTemplatePDFFont() (string, error) {
	// Option 1: Prefer fc-list (fontconfig) - most elegant approach
	if fontPath, err := findFontViaFontconfig(); err == nil {
		return fontPath, nil
	}

	// Option 2: Fallback - search common directories (for systems without fontconfig)
	candidates := []string{
		"LiberationSerif-Regular.ttf",
		"DejaVuSerif.ttf",
		"DejaVuSans.ttf",
	}
	searchDirs := []string{
		"/usr/share/fonts/truetype",
		"/usr/share/fonts/truetype/dejavu",
		"/usr/share/fonts/truetype/liberation",
		"/usr/share/fonts",
		"/usr/local/share/fonts",
	}

	for _, dir := range searchDirs {
		for _, font := range candidates {
			candidate := filepath.Join(dir, font)
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("no usable TTF font found for generated PDF fixture")
}

// findFontViaFontconfig uses fc-list command to find a suitable TTF font
func findFontViaFontconfig() (string, error) {
	// Search priority: DejaVu Serif > Liberation Serif > DejaVu Sans
	fontPatterns := []string{
		"DejaVu Serif:style=Regular",
		"Liberation Serif:style=Regular",
		"DejaVu Sans:style=Regular",
	}

	for _, pattern := range fontPatterns {
		cmd := exec.Command("fc-list", "--format=%{file}\n", pattern)
		output, err := cmd.Output()
		if err != nil {
			continue
		}
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Verify file exists and is in TTF format
			if _, err := os.Stat(line); err == nil && strings.HasSuffix(strings.ToLower(line), ".ttf") {
				return line, nil
			}
		}
	}

	return "", fmt.Errorf("no font found via fontconfig")
}

func assertMetadataContainsString(t *testing.T, metadata map[string]any, key, want string) {
	t.Helper()
	raw, ok := metadata[key]
	if !ok {
		t.Fatalf("metadata missing key %q in %v", key, metadata)
	}
	switch v := raw.(type) {
	case string:
		if !strings.Contains(v, want) {
			t.Fatalf("metadata[%q] = %q, want contains %q", key, v, want)
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.Contains(s, want) {
				return
			}
		}
		t.Fatalf("metadata[%q] = %v, want one entry containing %q", key, v, want)
	case []string:
		for _, item := range v {
			if strings.Contains(item, want) {
				return
			}
		}
		t.Fatalf("metadata[%q] = %v, want one entry containing %q", key, v, want)
	default:
		t.Fatalf("metadata[%q] = %T/%v, want string or string list containing %q", key, raw, raw, want)
	}
}

func assertExtractedMetadataContains(t *testing.T, raw any, key, want string) {
	t.Helper()
	switch v := raw.(type) {
	case map[string]any:
		if len(v) == 0 {
			t.Fatalf("metadata map is empty for key %q", key)
		}
		assertMetadataContainsString(t, v, key, want)
	case string:
		if !strings.Contains(v, want) {
			t.Skipf("model returned unstructured metadata text instead of extraction output; want %q in %q", want, v)
		}
	default:
		t.Fatalf("metadata = %T/%v, want map[string]any or string", raw, raw)
	}
}

func assertTokenizerTerminalChunk(t *testing.T, payload map[string]any, name, wantMergedText string) {
	t.Helper()

	if got := payload["output_format"]; got != "chunks" {
		t.Fatalf("output_format = %v, want chunks", got)
	}
	chunks, ok := payload["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", payload["chunks"])
	}
	if len(chunks) != 1 {
		t.Fatalf("len(chunks) = %d, want 1", len(chunks))
	}
	if got := chunks[0]["text"]; got != wantMergedText {
		t.Fatalf("chunks[0].text = %v, want %q", got, wantMergedText)
	}
	if _, ok := chunks[0]["content_ltks"].(string); !ok || chunks[0]["content_ltks"] == "" {
		t.Fatalf("chunks[0].content_ltks missing or empty: %v", chunks[0]["content_ltks"])
	}
	if _, ok := chunks[0]["content_sm_ltks"].(string); !ok || chunks[0]["content_sm_ltks"] == "" {
		t.Fatalf("chunks[0].content_sm_ltks missing or empty: %v", chunks[0]["content_sm_ltks"])
	}
	vec := floatSliceFromAny(t, chunks[0]["q_4_vec"])
	if len(vec) != 4 {
		t.Fatalf("chunks[0].q_4_vec len = %d, want 4", len(vec))
	}
	wantFirst := expectedFixedEmbedderFirst(name, wantMergedText)
	if got := vec[0]; !approxFloat(got, wantFirst) {
		t.Fatalf("chunks[0].q_4_vec[0] = %v, want %v", got, wantFirst)
	}
	wantTokens := len(name) + len(wantMergedText)
	if got := payload["embedding_token_consumption"]; got != wantTokens {
		t.Fatalf("embedding_token_consumption = %v, want %d", got, wantTokens)
	}
}

func TestTemplateFixtures_AreWrappedTemplates(t *testing.T) {
	path := filepath.Join(repoRootFromPipelineTest(t), "agent", "templates", "ingestion_pipeline_one.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if _, ok := tpl["dsl"].(map[string]any); !ok {
		t.Fatalf("fixture dsl = %T, want map[string]any", tpl["dsl"])
	}
}
