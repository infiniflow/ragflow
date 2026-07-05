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

package schema

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// File
// ---------------------------------------------------------------------------

func TestFileParamDefaults(t *testing.T) {
	p := FileParam{}.Defaults()
	if err := p.Validate(); err != nil {
		t.Fatalf("default FileParam failed Validate: %v", err)
	}
}

func TestFileFromUpstreamValidate(t *testing.T) {
	// Empty upstream: nothing to bind to -> error.
	if err := (&FileFromUpstream{}).Validate(); err == nil {
		t.Fatal("expected Validate to fail when neither doc_id nor file is set")
	}

	// DocID path: valid.
	docID := "doc-123"
	fu := FileFromUpstream{DocID: &docID}
	if err := fu.Validate(); err != nil {
		t.Fatalf("FileFromUpstream with DocID unexpectedly failed Validate: %v", err)
	}

	// File path: valid.
	fu = FileFromUpstream{File: []map[string]any{{"name": "input.pdf"}}}
	if err := fu.Validate(); err != nil {
		t.Fatalf("FileFromUpstream with File unexpectedly failed Validate: %v", err)
	}
}

func TestFileFromUpstreamJSONRoundTrip(t *testing.T) {
	created := 1.5
	elapsed := 0.25
	docID := "doc-abc"
	original := FileFromUpstream{
		CreatedTime: &created,
		ElapsedTime: &elapsed,
		DocID:       &docID,
		File:        []map[string]any{{"name": "input.pdf"}},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"_created_time":1.5`) {
		t.Errorf("expected _created_time alias in JSON, got %s", data)
	}
	if !strings.Contains(string(data), `"doc_id":"doc-abc"`) {
		t.Errorf("expected doc_id in JSON, got %s", data)
	}

	var decoded FileFromUpstream
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.DocID == nil || *decoded.DocID != docID {
		t.Errorf("DocID round-trip mismatch: got %v", decoded.DocID)
	}
	if len(decoded.File) != 1 {
		t.Errorf("File round-trip mismatch: got %d", len(decoded.File))
	}
}

func TestFileOutputsJSONRoundTrip(t *testing.T) {
	original := FileOutputs{
		Name:  "input.pdf",
		File:  map[string]any{"id": "f-1"},
		Error: "doc not found",
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"name":"input.pdf"`) {
		t.Errorf("expected name in JSON, got %s", data)
	}
	// _ERROR non-empty should be emitted.
	if !strings.Contains(string(data), `"_ERROR":"doc not found"`) {
		t.Errorf("expected _ERROR in JSON, got %s", data)
	}

	var decoded FileOutputs
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name round-trip mismatch: got %q", decoded.Name)
	}
	if decoded.Error != original.Error {
		t.Errorf("Error round-trip mismatch: got %q", decoded.Error)
	}
}

// ---------------------------------------------------------------------------
// Parser
// ---------------------------------------------------------------------------

func TestParserFromUpstreamValidate(t *testing.T) {
	// Name is required.
	if err := (&ParserFromUpstream{}).Validate(); err == nil {
		t.Fatal("expected Validate to fail when Name is empty")
	}
	if err := (&ParserFromUpstream{Name: "doc.pdf"}).Validate(); err != nil {
		t.Fatalf("Validate with Name unexpectedly failed: %v", err)
	}
}

func TestParserParamDefaults(t *testing.T) {
	p := ParserParam{}.Defaults()
	if err := p.Validate(); err != nil {
		t.Fatalf("default ParserParam failed Validate: %v", err)
	}
	// Spot-check a few entries that the Python class initializes.
	for _, key := range []string{"pdf", "docx", "image", "audio", "video", "email", "epub"} {
		if _, ok := p.Setups[key]; !ok {
			t.Errorf("default Setups missing key %q", key)
		}
	}
	if got := p.Setups["pdf"]["parse_method"]; got != "deepdoc" {
		t.Errorf("default pdf parse_method = %v, want deepdoc", got)
	}
	if got := p.AllowedOutputFormat["pdf"]; len(got) != 2 || got[0] != "json" || got[1] != "markdown" {
		t.Errorf("default pdf allowed_output_format = %v, want [json markdown]", got)
	}
}

func TestParserParamJSONRoundTrip(t *testing.T) {
	original := ParserParam{}.Defaults()
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"setups"`) {
		t.Errorf("expected setups in JSON, got %s", data)
	}
	var decoded ParserParam
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Setups["pdf"]["parse_method"] != "deepdoc" {
		t.Errorf("round-trip lost default pdf parse_method: got %v", decoded.Setups["pdf"]["parse_method"])
	}
}

func TestParserFromUpstreamJSONRoundTrip(t *testing.T) {
	original := ParserFromUpstream{
		Name:     "input.pdf",
		Abstract: true,
		Author:   false,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// abstract=true should be emitted; author=false has omitempty so it's
	// dropped (zero-value bool with omitempty). We test the
	// non-zero path.
	if !strings.Contains(string(data), `"abstract":true`) {
		t.Errorf("expected abstract=true in JSON, got %s", data)
	}
	var decoded ParserFromUpstream
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name round-trip mismatch: got %q", decoded.Name)
	}
	if !decoded.Abstract {
		t.Errorf("Abstract round-trip mismatch: got %v", decoded.Abstract)
	}
	// author field is omitempty, so JSON round-trip should leave it false
	// (default value) on both sides.
	if decoded.Author {
		t.Errorf("Author should be false, got true")
	}
}

func TestParserOutputsJSONRoundTrip(t *testing.T) {
	original := ParserOutputs{
		OutputFormat: "json",
		JSON:         []map[string]any{{"text": "hello", "doc_type_kwd": "text"}},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"output_format":"json"`) {
		t.Errorf("expected output_format in JSON, got %s", data)
	}
	var decoded ParserOutputs
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.OutputFormat != "json" {
		t.Errorf("OutputFormat round-trip mismatch: got %q", decoded.OutputFormat)
	}
	if len(decoded.JSON) != 1 {
		t.Errorf("JSON round-trip mismatch: got %d", len(decoded.JSON))
	}
}

// ---------------------------------------------------------------------------
// Chunker
// ---------------------------------------------------------------------------

func TestChunkerFromUpstreamValidate(t *testing.T) {
	if err := (&ChunkerFromUpstream{}).Validate(); err == nil {
		t.Fatal("expected Validate to fail when Name is empty")
	}
	if err := (&ChunkerFromUpstream{Name: "doc.pdf"}).Validate(); err != nil {
		t.Fatalf("Validate with Name unexpectedly failed: %v", err)
	}
}

func TestChunkerFromUpstreamJSONRoundTrip(t *testing.T) {
	md := "# title"
	original := ChunkerFromUpstream{
		Name:           "doc.pdf",
		OutputFormat:   PayloadFormatChunks,
		Chunks:         []ChunkDoc{{Text: "alpha"}},
		MarkdownResult: &md,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"output_format":"chunks"`) {
		t.Errorf("expected output_format in JSON, got %s", data)
	}
	var decoded ChunkerFromUpstream
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != "doc.pdf" || decoded.OutputFormat != PayloadFormatChunks {
		t.Errorf("round-trip mismatch: %+v", decoded)
	}
	if len(decoded.Chunks) != 1 {
		t.Errorf("Chunks round-trip mismatch: got %d", len(decoded.Chunks))
	}
}

func TestChunkerOutputsJSONRoundTrip(t *testing.T) {
	original := ChunkerOutputs{
		OutputFormat: PayloadFormatChunks,
		Chunks:       []ChunkDoc{{Text: "alpha"}, {Text: "beta"}},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"output_format":"chunks"`) {
		t.Errorf("expected output_format in JSON, got %s", data)
	}
	var decoded ChunkerOutputs
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Chunks) != 2 {
		t.Errorf("Chunks round-trip mismatch: got %d", len(decoded.Chunks))
	}
}

func TestTokenChunkerParamDefaults(t *testing.T) {
	p := TokenChunkerParam{}.Defaults()
	if p.DelimiterMode != "token_size" {
		t.Errorf("default delimiter_mode = %q, want token_size", p.DelimiterMode)
	}
	if p.ChunkTokenSize != 512 {
		t.Errorf("default chunk_token_size = %d, want 512", p.ChunkTokenSize)
	}
	if len(p.Delimiters) != 1 || p.Delimiters[0] != "\n" {
		t.Errorf("default delimiters = %v, want [\\n]", p.Delimiters)
	}
	if p.OverlappedPercent != 0 {
		t.Errorf("default overlapped_percent = %f, want 0", p.OverlappedPercent)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("default TokenChunkerParam failed Validate: %v", err)
	}
}

func TestTitleChunkerParamDefaults(t *testing.T) {
	p := TitleChunkerParam{}.Defaults()
	if p.Hierarchy != nil {
		t.Errorf("default hierarchy should be nil, got %v", *p.Hierarchy)
	}
	if p.IncludeHeadingContent {
		t.Errorf("default include_heading_content should be false")
	}
	// Default has no method set; Validate must accept it (the empty
	// method is the uninitialized state, not an enum violation).
	if err := p.Validate(); err != nil {
		t.Fatalf("default TitleChunkerParam failed Validate: %v", err)
	}
}

func TestTitleChunkerParamValidate(t *testing.T) {
	// Method=hierarchy with no levels -> error.
	p := TitleChunkerParam{Method: "hierarchy"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error when Method=hierarchy with no levels")
	}
	// Method=hierarchy with levels but no hierarchy number -> error.
	p.Levels = [][]string{{"^# "}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error when Method=hierarchy with no hierarchy number")
	}
	// Method=hierarchy with levels and a positive hierarchy -> OK.
	h := 2
	p.Hierarchy = &h
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate unexpectedly failed: %v", err)
	}
	// Method=group with levels -> OK.
	p = TitleChunkerParam{Method: "group", Levels: [][]string{{"^# "}}}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate unexpectedly failed: %v", err)
	}
	// Method=group with no levels -> error.
	p = TitleChunkerParam{Method: "group"}
	if err := p.Validate(); err == nil {
		t.Fatal("expected error when Method=group with no levels")
	}
}

func TestGroupTitleChunkerParamAlias(t *testing.T) {
	// The alias must be assignable from a TitleChunkerParam value.
	var gp GroupTitleChunkerParam = TitleChunkerParam{Method: "group", Levels: [][]string{{"^# "}}}
	if err := gp.Validate(); err != nil {
		t.Fatalf("group param via alias failed Validate: %v", err)
	}
}

func TestHierarchyTitleChunkerParamAlias(t *testing.T) {
	h := 1
	var hp HierarchyTitleChunkerParam = TitleChunkerParam{Method: "hierarchy", Levels: [][]string{{"^# "}}, Hierarchy: &h}
	if err := hp.Validate(); err != nil {
		t.Fatalf("hierarchy param via alias failed Validate: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Tokenizer
// ---------------------------------------------------------------------------

func TestTokenizerFromUpstreamValidate(t *testing.T) {
	// output_format=chunks with nil Chunks is valid.
	if err := (&TokenizerFromUpstream{OutputFormat: PayloadFormatChunks}).Validate(); err != nil {
		t.Fatalf("output_format=chunks should be valid, got %v", err)
	}
	// output_format=markdown with no MarkdownResult -> error.
	if err := (&TokenizerFromUpstream{OutputFormat: PayloadFormatMarkdown}).Validate(); err == nil {
		t.Fatal("expected error for markdown without payload")
	}
	// output_format=markdown with payload -> OK.
	md := "# title"
	if err := (&TokenizerFromUpstream{OutputFormat: PayloadFormatMarkdown, MarkdownResult: &md}).Validate(); err != nil {
		t.Fatalf("markdown with payload should be valid, got %v", err)
	}
	// output_format=text without payload -> error.
	if err := (&TokenizerFromUpstream{OutputFormat: PayloadFormatText}).Validate(); err == nil {
		t.Fatal("expected error for text without payload")
	}
	txt := "hello"
	if err := (&TokenizerFromUpstream{OutputFormat: PayloadFormatText, TextResult: &txt}).Validate(); err != nil {
		t.Fatalf("text with payload should be valid, got %v", err)
	}
	// output_format=html without payload -> error.
	if err := (&TokenizerFromUpstream{OutputFormat: PayloadFormatHTML}).Validate(); err == nil {
		t.Fatal("expected error for html without payload")
	}
	html := "<p>x</p>"
	if err := (&TokenizerFromUpstream{OutputFormat: PayloadFormatHTML, HTMLResult: &html}).Validate(); err != nil {
		t.Fatalf("html with payload should be valid, got %v", err)
	}
	// Empty output_format with neither JSON nor Chunks -> error.
	if err := (&TokenizerFromUpstream{}).Validate(); err == nil {
		t.Fatal("expected error when no output_format and no payload")
	}
	// Empty output_format with JSONResult -> OK.
	if err := (&TokenizerFromUpstream{JSONResult: []ChunkDoc{{Text: "x"}}}).Validate(); err != nil {
		t.Fatalf("empty output_format with JSONResult should be valid, got %v", err)
	}
	// Empty output_format with Chunks -> OK.
	if err := (&TokenizerFromUpstream{Chunks: []ChunkDoc{{Text: "x"}}}).Validate(); err != nil {
		t.Fatalf("empty output_format with Chunks should be valid, got %v", err)
	}
}

func TestTokenizerFromUpstreamJSONRoundTrip(t *testing.T) {
	md := "# title"
	txt := "body"
	html := "<p>x</p>"
	original := TokenizerFromUpstream{
		Name:           "doc.pdf",
		OutputFormat:   PayloadFormatChunks,
		Chunks:         []ChunkDoc{{Text: "alpha"}},
		MarkdownResult: &md,
		TextResult:     &txt,
		HTMLResult:     &html,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"output_format":"chunks"`) {
		t.Errorf("expected output_format in JSON, got %s", data)
	}
	// All *Result fields have omitempty. Non-zero values round-trip;
	// confirm at least one of them is in the payload.
	if !strings.Contains(string(data), `"markdown":"# title"`) {
		t.Errorf("expected markdown alias in JSON, got %s", data)
	}
	var decoded TokenizerFromUpstream
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.MarkdownResult == nil || *decoded.MarkdownResult != "# title" {
		t.Errorf("markdown round-trip mismatch: got %v", decoded.MarkdownResult)
	}
	// Re-marshal the decoded and confirm we can read markdown back.
	data2, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !strings.Contains(string(data2), `"markdown":"# title"`) {
		t.Errorf("expected markdown alias after re-marshal, got %s", data2)
	}
}

func TestTokenizerParamDefaults(t *testing.T) {
	p := TokenizerParam{}.Defaults()
	if len(p.SearchMethod) != 2 || p.SearchMethod[0] != "full_text" || p.SearchMethod[1] != "embedding" {
		t.Errorf("default search_method = %v, want [full_text embedding]", p.SearchMethod)
	}
	if p.FilenameEmbdWeight != 0.1 {
		t.Errorf("default filename_embd_weight = %f, want 0.1", p.FilenameEmbdWeight)
	}
	if len(p.Fields) != 1 || p.Fields[0] != "text" {
		t.Errorf("default fields = %v, want [text]", p.Fields)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("default TokenizerParam failed Validate: %v", err)
	}
}

func TestTokenizerParamValidate(t *testing.T) {
	// Empty search_method -> error.
	if err := (&TokenizerParam{}).Validate(); err == nil {
		t.Fatal("expected error for empty search_method")
	}
	// Invalid search_method entry -> error.
	if err := (&TokenizerParam{SearchMethod: []string{"unknown"}}).Validate(); err == nil {
		t.Fatal("expected error for unknown search_method entry")
	}
	// Valid search_method -> OK.
	if err := (&TokenizerParam{SearchMethod: []string{"embedding"}}).Validate(); err != nil {
		t.Fatalf("embedding search_method should be valid, got %v", err)
	}
}

func TestTokenizerOutputsJSONRoundTrip(t *testing.T) {
	tokens := 256
	original := TokenizerOutputs{
		OutputFormat:              PayloadFormatChunks,
		Chunks:                    []ChunkDoc{{Text: "alpha"}},
		EmbeddingTokenConsumption: &tokens,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"embedding_token_consumption":256`) {
		t.Errorf("expected embedding_token_consumption in JSON, got %s", data)
	}
	var decoded TokenizerOutputs
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.EmbeddingTokenConsumption == nil || *decoded.EmbeddingTokenConsumption != 256 {
		t.Errorf("EmbeddingTokenConsumption round-trip mismatch: got %v", decoded.EmbeddingTokenConsumption)
	}
}

// ---------------------------------------------------------------------------
// Extractor
// ---------------------------------------------------------------------------

func TestExtractorParamDefaults(t *testing.T) {
	p := ExtractorParam{}.Defaults()
	if p.FieldName != "" {
		t.Errorf("default field_name should be empty, got %q", p.FieldName)
	}
	if err := p.Validate(); err == nil {
		t.Fatal("default ExtractorParam should fail Validate (field_name required)")
	}
}

func TestExtractorParamValidate(t *testing.T) {
	if err := (&ExtractorParam{}).Validate(); err == nil {
		t.Fatal("expected error for empty field_name")
	}
	if err := (&ExtractorParam{FieldName: "summary"}).Validate(); err != nil {
		t.Fatalf("Validate with field_name should pass, got %v", err)
	}
}

func TestExtractorFromUpstreamJSONRoundTrip(t *testing.T) {
	original := ExtractorFromUpstream{
		Chunks: []map[string]any{{"text": "alpha"}},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"chunks"`) {
		t.Errorf("expected chunks in JSON, got %s", data)
	}
	var decoded ExtractorFromUpstream
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Chunks) != 1 {
		t.Errorf("Chunks round-trip mismatch: got %d", len(decoded.Chunks))
	}
}

func TestExtractorOutputsJSONRoundTrip(t *testing.T) {
	original := ExtractorOutputs{
		OutputFormat: "chunks",
		Chunks:       []map[string]any{{"summary": "x"}},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"output_format":"chunks"`) {
		t.Errorf("expected output_format in JSON, got %s", data)
	}
	var decoded ExtractorOutputs
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.OutputFormat != "chunks" {
		t.Errorf("OutputFormat round-trip mismatch: got %q", decoded.OutputFormat)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func ptrString(s string) *string { return &s }
