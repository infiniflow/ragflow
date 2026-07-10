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

package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"
)

// TestTitleChunker_Registered asserts the registry has a CategoryIngestion
// entry for TitleChunker.
func TestTitleChunker_Registered(t *testing.T) {
	factory, cat, meta, ok := runtime.DefaultRegistry.Lookup("TitleChunker")
	if !ok {
		t.Fatal("TitleChunker: registry miss")
	}
	if cat != runtime.CategoryIngestion {
		t.Errorf("category = %q, want %q", cat, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Error("factory is nil")
	}
	if len(meta.Inputs) == 0 {
		t.Errorf("inputs metadata is empty")
	}
	if len(meta.Outputs) == 0 {
		t.Errorf("outputs metadata is empty")
	}
}

// TestTitleChunker_Parallelism enforces plan row 2.3b parallelism (2).
func TestTitleChunker_Parallelism(t *testing.T) {
	c, err := NewTitleChunker(map[string]any{
		"method": "group",
		"levels": [][]string{{"^# "}},
	})
	if err != nil {
		t.Fatalf("NewTitleChunker: %v", err)
	}
	tc, ok := c.(*TitleChunkerComponent)
	if !ok {
		t.Fatalf("NewTitleChunker returned %T", c)
	}
	if got := tc.Parallelism(); got != 2 {
		t.Errorf("Parallelism() = %d, want 2", got)
	}
}

// TestTitleChunker_InputsOutputs_NonEmpty asserts metadata is
// populated for both ends.
func TestTitleChunker_InputsOutputs_NonEmpty(t *testing.T) {
	_, _, meta, ok := runtime.DefaultRegistry.Lookup("TitleChunker")
	if !ok {
		t.Fatal("registry miss")
	}
	if len(meta.Inputs) == 0 {
		t.Error("inputs metadata is empty")
	}
	if len(meta.Outputs) == 0 {
		t.Error("outputs metadata is empty")
	}
}

// TestTitleChunker_NewRejectsBadMethod enforces the method check.
func TestTitleChunker_NewRejectsBadMethod(t *testing.T) {
	if _, err := NewTitleChunker(map[string]any{
		"method": "unknown",
		"levels": [][]string{{"^# "}},
	}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestTitleChunker_NewRejectsHierarchyWithoutHierarchyParam enforces
// the hierarchy branch's required-field check.
func TestTitleChunker_NewRejectsHierarchyWithoutHierarchyParam(t *testing.T) {
	if _, err := NewTitleChunker(map[string]any{
		"method": "hierarchy",
		"levels": [][]string{{"^# "}},
	}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestTitleChunker_InvokeEmptyInput returns empty chunks for an
// empty payload.
func TestTitleChunker_InvokeEmptyInput(t *testing.T) {
	c, err := NewTitleChunker(map[string]any{
		"method": "group",
		"levels": [][]string{{"^# "}},
	})
	if err != nil {
		t.Fatalf("NewTitleChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "chunks"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 0 {
		t.Errorf("chunks = %d, want 0", len(chunks))
	}
}

// TestTitleChunker_Headings_ASCII is the golden-file parity check
// for the markdown detector: a # heading + body + ## subheading + body
// is recognized at least one chunked partition boundary.
//
// Note: the title strategy is dispatched to the underlying strategy
// (`group` by default, or `hierarchy` when configured). Each
// strategy test lives in its own file; this test routes through
// both via the dispatcher.
func TestTitleChunker_Headings_ASCII(t *testing.T) {
	c, err := NewTitleChunker(map[string]any{
		"method": "group",
		"levels": [][]string{
			{`^# `},
			{`^## `},
			{`^### `},
		},
	})
	if err != nil {
		t.Fatalf("NewTitleChunker: %v", err)
	}

	input := "# Top\nFirst body line under Top.\nSecond body.\n## Sub\nBody under Sub heading.\n# TopTwo\nBody under TopTwo."
	out, err := c.Invoke(context.Background(), map[string]any{
		"name": "doc.md",
		"text": input,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["output_format"], "chunks"; got != want {
		t.Errorf("output_format = %v, want %v", got, want)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
	for i, ck := range chunks {
		if text, _ := ck["text"].(string); text == "" {
			t.Errorf("chunk[%d] text is empty", i)
		}
	}
}

// TestTitleChunker_NoHeadings_FallsBack feeds plain text without any
// markdown heading; the chunker should still produce a chunk
// containing the body text (single chunk fallback).
func TestTitleChunker_NoHeadings_FallsBack(t *testing.T) {
	c, err := NewTitleChunker(map[string]any{
		"method": "group",
		"levels": [][]string{{"^# "}},
	})
	if err != nil {
		t.Fatalf("NewTitleChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), map[string]any{
		"name": "doc.txt",
		"text": "alpha line one\nalpha line two",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatal("chunks: want >=1, got 0")
	}
}

// TestTitleChunker_DispatcherHierarchy routes to the hierarchy
// strategy without panicking.
func TestTitleChunker_DispatcherHierarchy(t *testing.T) {
	c, err := NewTitleChunker(map[string]any{
		"method":    "hierarchy",
		"hierarchy": 1,
		"levels":    [][]string{{`^# `}, {`^## `}},
	})
	if err != nil {
		t.Fatalf("NewTitleChunker: %v", err)
	}
	if _, ok := c.(*TitleChunkerComponent); !ok {
		t.Fatalf("NewTitleChunker returned %T", c)
	}
	_, err = c.Invoke(context.Background(), map[string]any{
		"name": "doc.md",
		"text": "# Top\nFirst body.\n# TopTwo\nBody under TopTwo.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// TestTitleChunker_InvokeDeterministic runs the heading detector
// 10 times and asserts the chunks list is identical.
func TestTitleChunker_InvokeDeterministic(t *testing.T) {
	c, err := NewTitleChunker(map[string]any{
		"method": "group",
		"levels": [][]string{
			{`^# `},
			{`^## `},
		},
	})
	if err != nil {
		t.Fatalf("NewTitleChunker: %v", err)
	}
	inputs := map[string]any{
		"name": "doc.md",
		"text": "# A\nbody a line 1\nbody a line 2\n# B\nbody b line 1\n# C\nbody c line 1",
	}
	var firstLen int
	var firstTexts []string
	for run := 0; run < 10; run++ {
		out, err := c.Invoke(context.Background(), inputs)
		if err != nil {
			t.Fatalf("Invoke run %d: %v", run, err)
		}
		chunks, _ := out["chunks"].([]map[string]any)
		texts := make([]string, len(chunks))
		for i, ck := range chunks {
			texts[i], _ = ck["text"].(string)
		}
		if run == 0 {
			firstLen = len(chunks)
			firstTexts = texts
			continue
		}
		if firstLen != len(chunks) {
			t.Fatalf("run %d: chunk count changed (%d vs %d)", run, len(chunks), firstLen)
		}
		for i := range chunks {
			if firstTexts[i] != texts[i] {
				t.Fatalf("run %d: chunk[%d] text changed", run, i)
			}
		}
	}
}

// TestNewLevelContext_MostLevelIsMode pins Gap A: most_level is the
// most-frequent non-body heading level (the mode), matching python
// Counter(levels).most_common(). The old loop iterated map values and
// returned the smallest count, which skewed the group fallback target.
func TestNewLevelContext_MostLevelIsMode(t *testing.T) {
	p := &titleChunkerParam{TitleChunkerParam: schema.TitleChunkerParam{
		Method: "group",
		Levels: [][]string{{`^# `, `^## `, `^### `}},
	}}
	records := []lineRecord{
		{text: "# a", docType: "text"},
		{text: "## b", docType: "text"},
		{text: "## c", docType: "text"},
		{text: "### d", docType: "text"},
	}
	lc := newLevelContext(records, p)
	// selectLevelGroup picks the single 3-pattern family; per-line
	// levels are [1,2,2,3], so the mode (most frequent heading level)
	// is level 2.
	if lc.mostLevel != 2 {
		t.Errorf("mostLevel = %d, want 2 (the most frequent heading level)", lc.mostLevel)
	}
}

// TestNotBullet pins Gap B: the port of rag/nlp.not_bullet must reject
// numbered/bulleted list lines so they are not promoted to headings.
func TestNotBullet(t *testing.T) {
	bullet := []string{
		"0",
		"1 2个",
		"1... trailing",
		"1.2.3.4的",
		"2.3.4中",
		"0.5 section",
	}
	for _, s := range bullet {
		if !notBullet(s) {
			t.Errorf("notBullet(%q) = false, want true", s)
		}
	}
	heading := []string{
		"# Heading",
		"## Subheading",
		"Section 1",
		"Article 12",
		"1.2 Normal sentence",
		"1.2.3 without marker",
	}
	for _, s := range heading {
		if notBullet(s) {
			t.Errorf("notBullet(%q) = true, want false", s)
		}
	}
}

// TestNotTitle pins the port of rag/nlp.not_title used by the layout
// fallback: long/no-space/punctuated lines are "not a title", while
// the 第…条 exception and short title-like lines are titles.
func TestNotTitle(t *testing.T) {
	longLine := "one two three four five six seven eight nine ten eleven twelve thirteen"
	if !notTitle(longLine) {
		t.Errorf("notTitle(>12 words) = false, want true")
	}
	noSpace := strings.Repeat("x", 40)
	if !notTitle(noSpace) {
		t.Errorf("notTitle(no space, >=32 chars) = false, want true")
	}
	if !notTitle("This, is a body line") {
		t.Errorf("notTitle(comma line) = false, want true")
	}
	if notTitle("第三条 关于某某规定") {
		t.Errorf("notTitle(第…条) = true, want false (exception)")
	}
	if notTitle("Chapter One") {
		t.Errorf("notTitle(Chapter One) = true, want false")
	}
}

// TestResolveTitleLevels_NonTextPinnedToBody pins Gap D: a non-text
// record whose caption would match a heading regex is still pinned to
// BODY_LEVEL, because python never runs regex/layout detection for
// doc_type_kwd != "text".
func TestResolveTitleLevels_NonTextPinnedToBody(t *testing.T) {
	p := &titleChunkerParam{TitleChunkerParam: schema.TitleChunkerParam{
		Method: "group",
		Levels: [][]string{{`^# `}},
	}}
	records := []lineRecord{
		{text: "# Real Heading", docType: "text"},
		{text: "# image caption that matches", docType: "image"},
		{text: "body line", docType: "text"},
	}
	levels := resolveTitleLevels(records, p)
	if levels[0] != 1 {
		t.Errorf("text heading level = %d, want 1", levels[0])
	}
	if levels[1] != bodyLevel {
		t.Errorf("non-text caption level = %d, want bodyLevel (%d)", levels[1], bodyLevel)
	}
	if levels[2] != bodyLevel {
		t.Errorf("body level = %d, want bodyLevel", levels[2])
	}
}

// TestResolveTitleLevels_LayoutFallback pins Gap C: a text record that
// misses every regex but whose layout flags it as a section/title/head
// and whose text passes not_title is promoted to fallback_level =
// len(selected_group) + 1. The group must have at least one regex hit
// (here "# Real Heading") so it is selected; with one pattern the
// fallback_level is 2.
func TestResolveTitleLevels_LayoutFallback(t *testing.T) {
	p := &titleChunkerParam{TitleChunkerParam: schema.TitleChunkerParam{
		Method: "group",
		Levels: [][]string{{`^# `}},
	}}
	records := []lineRecord{
		{text: "# Real Heading", docType: "text", layout: "title"},
		{text: "Chapter One Overview", docType: "text", layout: "title"},
		{text: "Some body paragraph text here.", docType: "text", layout: "text"},
	}
	levels := resolveTitleLevels(records, p)
	if levels[0] != 1 {
		t.Errorf("regex heading level = %d, want 1", levels[0])
	}
	if levels[1] != 2 {
		t.Errorf("layout title level = %d, want 2 (fallback_level)", levels[1])
	}
	if levels[2] != bodyLevel {
		t.Errorf("plain body level = %d, want bodyLevel", levels[2])
	}
}
