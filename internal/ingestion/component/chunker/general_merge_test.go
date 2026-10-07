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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestMergeGeneralUnitsAllowsOneBoundaryOverflow(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "alpha", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "beta", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "gamma", DocType: "text", CKType: "text", TKNums: intPtr(2)},
	}

	got := mergeGeneralUnits(units, 3, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"alpha\nbeta", "gamma"}) {
		t.Fatalf("texts = %q, want [alpha\\nbeta gamma]", texts)
	}
	if count := intValue(got[0].TKNums); count != 4 {
		t.Errorf("first chunk tokens = %d, want running sum 4", count)
	}
}

// TestSameSpreadsheetTableUsesPositionSheet covers the media-context sheet
// guard once carried by merged row runs: identity falls through to the
// positions matrix when the sheet fields are absent.
func TestSameSpreadsheetTableUsesPositionSheet(t *testing.T) {
	one := schema.ChunkDoc{Text: "sheet-1", Positions: json.RawMessage(`[[1,2,2,1,2]]`)}
	two := schema.ChunkDoc{Text: "sheet-2", Positions: json.RawMessage(`[[2,2,2,1,2]]`)}
	if !sameSpreadsheetTable(one, one) {
		t.Errorf("same position sheet rejected")
	}
	if sameSpreadsheetTable(one, two) {
		t.Errorf("different position sheets accepted")
	}
}

func TestMergeGeneralUnitsMergesWhenCurrentEqualsCap(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "at cap", DocType: "text", CKType: "text", TKNums: intPtr(3)},
		{Text: "overflow", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	got := mergeGeneralUnits(units, 3, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"at cap\noverflow"}) {
		t.Fatalf("texts = %q, want one overflow chunk", texts)
	}
}

func TestMergeGeneralUnitsKeepsOversizedAtomWhole(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "before", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "oversized atom stays whole", DocType: "text", CKType: "text", TKNums: intPtr(8)},
		{Text: "after", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	got := mergeGeneralUnits(units, 3, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"before", "oversized atom stays whole", "after"}) {
		t.Fatalf("texts = %q", texts)
	}
}

func TestMergeGeneralUnitsAppliesUnconditionalCharacterOverlap(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "abcdefghij@@1\t0\t1\t2\t3##", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "klmnopqrst", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "uvwxyzABCD", DocType: "text", CKType: "text", TKNums: intPtr(2)},
	}

	got := mergeGeneralUnits(units, 2, 50, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{
		"abcdefghij@@1\t0\t1\t2\t3##",
		"fghij\nklmnopqrst",
		"mnopqrst\nuvwxyzABCD",
	}) {
		t.Fatalf("texts = %q", texts)
	}
	if intValue(got[1].TKNums) <= 2 {
		t.Errorf("overlap was trimmed to cap: token count = %d", intValue(got[1].TKNums))
	}
}

func TestApplyGeneralOverlapDoesNotAccumulatePositions(t *testing.T) {
	position := func(page int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf("[[%d,0,10,0,5]]", page))
	}
	chunks := []schema.ChunkDoc{
		{Text: "alpha beta", DocType: "text", CKType: "text", PDFPositions: position(1), Positions: position(1)},
		{Text: "gamma delta", DocType: "text", CKType: "text", PDFPositions: position(2), Positions: position(2)},
		{Text: "epsilon zeta", DocType: "text", CKType: "text", PDFPositions: position(3), Positions: position(3)},
	}

	got := applyGeneralOverlap(chunks, 50, "\n")
	if len(got) != 3 {
		t.Fatalf("chunks = %d, want 3", len(got))
	}
	if string(got[1].PDFPositions) != string(position(2)) || string(got[1].Positions) != string(position(2)) {
		t.Fatalf("second overlap chunk inherited prior positions: pdf=%s positions=%s", got[1].PDFPositions, got[1].Positions)
	}
	if string(got[2].PDFPositions) != string(position(3)) || string(got[2].Positions) != string(position(3)) {
		t.Fatalf("third overlap chunk accumulated prior positions: pdf=%s positions=%s", got[2].PDFPositions, got[2].Positions)
	}
}

func TestMergeMarkdownUnitsCarriesCharacterOverlapIntoNextBudget(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "alpha beta", DocType: "text", CKType: "text"},
		{Text: "gamma delta", DocType: "text", CKType: "text"},
		{Text: "epsilon zeta", DocType: "text", CKType: "text"},
		{Text: "eta theta", DocType: "text", CKType: "text"},
	}
	for i := range units {
		units[i].TKNums = intPtr(tokenizeStr(units[i].Text))
	}
	target := intValue(units[0].TKNums) + intValue(units[1].TKNums)

	got := mergeMarkdownUnits(units, target, 50, "\n")
	want := []string{
		"alpha beta\ngamma delta",
		"gamma delta\nepsilon zeta",
		"epsilon zeta\neta theta",
	}
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, want) {
		t.Fatalf("markdown overlap = %q, want %q", texts, want)
	}
}

func TestMergeMarkdownUnitsOverlapKeepsCurrentChunkMetadata(t *testing.T) {
	previousPage := 4
	currentPage := 5
	units := []schema.ChunkDoc{
		{
			Text:       "alpha beta",
			DocType:    "text",
			CKType:     "text",
			Image:      "previous-image",
			ImgID:      "previous-image-id",
			PageNumber: &previousPage,
			Positions:  json.RawMessage(`[[0,0,10,0,10]]`),
			Extra:      map[string]json.RawMessage{"previous": json.RawMessage(`true`)},
		},
		{
			Text:       "gamma delta",
			DocType:    "text",
			CKType:     "text",
			PageNumber: &currentPage,
			Positions:  json.RawMessage(`[[0,0,10,20,30]]`),
			Extra:      map[string]json.RawMessage{"current": json.RawMessage(`true`)},
		},
	}
	for i := range units {
		units[i].TKNums = intPtr(tokenizeStr(units[i].Text))
	}

	target := intValue(units[0].TKNums)
	got := mergeMarkdownUnits(units, target, 50, "\n")
	if len(got) != 2 {
		t.Fatalf("chunks = %#v, want previous and overlapped current chunks", got)
	}
	if got[1].Text == "gamma delta" || !strings.HasSuffix(got[1].Text, "\ngamma delta") {
		t.Fatalf("overlapped text = %q, want a prefix followed by current text", got[1].Text)
	}
	if got[1].Image != "" {
		t.Errorf("overlap chunk inherited previous image %q", got[1].Image)
	}
	if got[1].ImgID != "" {
		t.Errorf("overlap chunk inherited previous image id %q", got[1].ImgID)
	}
	if got[1].PageNumber == nil || *got[1].PageNumber != currentPage {
		t.Errorf("overlap chunk page number = %v, want current page %d", got[1].PageNumber, currentPage)
	}
	if string(got[1].Positions) != `[[0,0,10,0,10],[0,0,10,20,30]]` {
		t.Errorf("overlap chunk positions = %s, want previous and current positions", got[1].Positions)
	}
	if _, ok := got[1].Extra["previous"]; ok {
		t.Errorf("overlap chunk inherited previous extra metadata: %#v", got[1].Extra)
	}
	if _, ok := got[1].Extra["current"]; !ok {
		t.Errorf("overlap chunk lost current extra metadata: %#v", got[1].Extra)
	}
}

func TestMergeMarkdownUnitsStripsFoldedHeadingFromTableContextAbove(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "## Section", DocType: "text", CKType: "heading", TKNums: intPtr(1)},
		{
			Text:         "<table></table>",
			DocType:      "table",
			CKType:       "table",
			ContextAbove: "## Section",
			TKNums:       intPtr(1),
		},
	}
	got := mergeMarkdownUnits(units, 10, 0, "\n")
	if len(got) != 1 {
		t.Fatalf("chunks = %#v, want one table chunk", got)
	}
	if got[0].ContextAbove != "" {
		t.Fatalf("ContextAbove = %q, want empty after heading fold", got[0].ContextAbove)
	}
	if got[0].Text != "## Section\n<table></table>" {
		t.Fatalf("text = %q, want heading folded into body", got[0].Text)
	}
}

func TestMergeMarkdownUnitsStripsOnlyFoldedHeadingFromTableContextAbove(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "intro", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{Text: "## Section", DocType: "text", CKType: "heading", TKNums: intPtr(1)},
		{
			Text:         "<table></table>",
			DocType:      "table",
			CKType:       "table",
			ContextAbove: "intro\n## Section",
			TKNums:       intPtr(1),
		},
	}
	got := mergeMarkdownUnits(units, 10, 0, "\n")
	if len(got) != 2 {
		t.Fatalf("chunks = %#v, want intro text and table", got)
	}
	if got[1].ContextAbove != "intro" {
		t.Fatalf("ContextAbove = %q, want intro only", got[1].ContextAbove)
	}
}

func TestMergeMarkdownUnitsKeepsImageWithMediaContextStandalone(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "before", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{
			Text:         "figure",
			DocType:      "image",
			CKType:       "image",
			ContextAbove: "before",
			ContextBelow: "after",
			Image:        "data:image/png;base64,AAAA",
			TKNums:       intPtr(1),
		},
		{Text: "after", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}
	got := mergeMarkdownUnits(units, 10, 0, "\n")
	if len(got) != 3 {
		t.Fatalf("chunks = %d, want text, image, text", len(got))
	}
	if got[1].CKType != "image" || got[1].ContextAbove != "before" || got[1].ContextBelow != "after" {
		t.Fatalf("image chunk = %#v, want preserved media context", got[1])
	}
}

func TestMergeMarkdownUnitsStartsNewChunkForIncomingHeading(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "Background details", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{Text: "## Architecture", DocType: "text", CKType: "heading", TKNums: intPtr(1)},
		{Text: "The architecture is split into parser and chunker.", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	got := mergeMarkdownUnits(units, 3, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{
		"Background details",
		"## Architecture\nThe architecture is split into parser and chunker.",
	}) {
		t.Fatalf("markdown chunks = %q, want body and heading/body chunks", texts)
	}
	if got[1].CKType != "text" {
		t.Fatalf("heading/body chunk CKType = %q, want text after heading absorption", got[1].CKType)
	}
}

func TestShortMarkdownHeadingBudgetIgnoresATXMarker(t *testing.T) {
	content := strings.TrimSpace(strings.Repeat("word ", 49))
	unit := schema.ChunkDoc{CKType: "heading", Text: "## " + content}
	if tokenizeStr(content) >= 50 {
		t.Fatalf("test heading content unexpectedly reached threshold: %d", tokenizeStr(content))
	}
	if tokenizeStr(unit.Text) < 50 {
		t.Fatalf("test ATX heading did not cross raw threshold: %d", tokenizeStr(unit.Text))
	}
	if !isShortMarkdownHeading(unit) {
		t.Fatalf("heading %q was not classified short after removing ATX marker", unit.Text)
	}
}

func TestMergeGeneralUnitsOverlapDoesNotCopyPreviousMediaMetadata(t *testing.T) {
	previousPage := 4
	units := []schema.ChunkDoc{
		{Text: "first", DocType: "text", CKType: "text", TKNums: intPtr(2), Image: "previous-image", ImgID: "previous-image-id", PageNumber: &previousPage, Extra: map[string]json.RawMessage{"source": json.RawMessage(`"previous"`)}},
		{Text: "second", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		{Text: "third", DocType: "text", CKType: "text", TKNums: intPtr(2)},
	}

	got := mergeGeneralUnits(units, 4, 50, "\n")
	if len(got) != 2 {
		t.Fatalf("chunks = %#v, want two chunks", got)
	}
	current := got[1]
	if current.Image != "" || current.ImgID != "" {
		t.Fatalf("overlap chunk copied previous media: image=%q img_id=%q", current.Image, current.ImgID)
	}
	if current.PageNumber != nil {
		t.Fatalf("overlap chunk copied previous page number: %v", current.PageNumber)
	}
	if _, ok := current.Extra["source"]; ok {
		t.Fatalf("overlap chunk copied previous extra metadata: %#v", current.Extra)
	}
}

func TestTakeContextSentencesPreservesBoundarySelection(t *testing.T) {
	text := "one!two!three!"
	if got, want := takeContextSentences(text, tokenizeStr("two!three!"), true), "two!three!"; got != want {
		t.Fatalf("suffix context = %q, want %q", got, want)
	}
	if got, want := takeContextSentences(text, tokenizeStr("one!two!"), false), "one!two!"; got != want {
		t.Fatalf("prefix context = %q, want %q", got, want)
	}
}

func TestMergeGeneralUnitsKeepsMediaAtomicAndBreaksTextRun(t *testing.T) {
	units := []schema.ChunkDoc{
		{Text: "before", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		{Text: "A|B", DocType: "table", CKType: "table", TKNums: intPtr(2)},
		{Text: "after", DocType: "text", CKType: "text", TKNums: intPtr(1)},
	}

	got := mergeGeneralUnits(units, 10, 0, "\n")
	if texts := generalChunkTexts(got); !reflect.DeepEqual(texts, []string{"before", "A|B", "after"}) {
		t.Fatalf("texts = %q", texts)
	}
	if got[1].DocType != "table" || got[1].CKType != "table" {
		t.Errorf("table type changed: %#v", got[1])
	}
}

func TestMergeGeneralUnitsCombinesPositionsAndMetadata(t *testing.T) {
	first := schema.ChunkDoc{
		Text:         "alpha",
		DocType:      "text",
		CKType:       "text",
		TKNums:       intPtr(1),
		Image:        "image-a",
		Positions:    json.RawMessage(`[[0,0,10,0,10]]`),
		PDFPositions: json.RawMessage(`[[0,0,10,0,10]]`),
		Extra:        map[string]json.RawMessage{"source_order": json.RawMessage(`1`)},
		PageNumber:   intPtr(4),
	}
	second := schema.ChunkDoc{
		Text:         "beta",
		DocType:      "text",
		CKType:       "text",
		TKNums:       intPtr(1),
		Positions:    json.RawMessage(`[[0,0,10,20,30]]`),
		PDFPositions: json.RawMessage(`[[0,0,10,20,30]]`),
		Extra:        map[string]json.RawMessage{"heading_level": json.RawMessage(`2`)},
	}

	got := mergeGeneralUnits([]schema.ChunkDoc{first, second}, 1, 0, "\n")
	if len(got) != 1 {
		t.Fatalf("chunks = %d, want 1", len(got))
	}
	if got[0].Image != "image-a" {
		t.Errorf("image = %q, want image-a", got[0].Image)
	}
	if got[0].PageNumber == nil || *got[0].PageNumber != 4 {
		t.Errorf("page number = %v, want 4", got[0].PageNumber)
	}
	if string(got[0].Positions) != `[[0,0,10,0,10],[0,0,10,20,30]]` {
		t.Errorf("positions = %s", got[0].Positions)
	}
	if string(got[0].PDFPositions) != `[[0,0,10,0,10],[0,0,10,20,30]]` {
		t.Errorf("PDF positions = %s", got[0].PDFPositions)
	}
	if string(got[0].Extra["source_order"]) != "1" || string(got[0].Extra["heading_level"]) != "2" {
		t.Errorf("extra metadata = %#v", got[0].Extra)
	}
}

func TestGeneralChunkerPreservesImageWithoutText(t *testing.T) {
	component, err := NewGeneralChunker(nil)
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name":          "document.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json": []map[string]any{{
			"doc_type_kwd": "image",
			"image":        "image-payload",
		}},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 || chunks[0]["image"] != "image-payload" {
		t.Fatalf("chunks = %#v, want image-only unit", chunks)
	}
}

func TestGeneralChunkerCustomDelimiterBypassesMerge(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 1,
		"delimiters":       []string{"`||`"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, generalTextInput("alpha beta||gamma delta"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"alpha beta", "gamma delta"}) {
		t.Fatalf("texts = %q", texts)
	}
}

func TestGeneralChunkerBareDelimiterCreatesMergeAtoms(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 0,
		"delimiters":       []string{"|"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, generalTextInput("alpha|beta"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"alpha", "beta"}) {
		t.Fatalf("texts = %q", texts)
	}
}

func TestGeneralChunkerSplitsChildrenAfterParentMerge(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size":    100,
		"delimiters":          []string{},
		"children_delimiters": []string{"|"},
	})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}
	out, err := component.Invoke(t.Context(), nil, generalTextInput("alpha|beta"))
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks := outputChunks(t, out)
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"alpha|", "beta"}) {
		t.Fatalf("texts = %q", texts)
	}
	for i, chunk := range chunks {
		if chunk["mom"] != "alpha|beta" {
			t.Errorf("chunk[%d].mom = %q", i, chunk["mom"])
		}
	}
}

// TestMergeDOCXUnitsPrefixGreedy covers the prefix-greedy merge that fixes
// #20496 (cycle 94): when a backtick-wrapped custom-delimiter prefix is
// supplied (e.g. "问："), a paragraph whose stripped text starts with the
// prefix begins a new chunk, while other paragraphs extend the current
// chunk up to chunk_token_size. Without a prefix the legacy "custom
// delimiter means no merge" behaviour is preserved.
func TestMergeDOCXUnitsPrefixGreedy(t *testing.T) {
	t.Run("single-record-groups-five-paragraphs", func(t *testing.T) {
		units := []schema.ChunkDoc{
			{Text: "问：什么是RAG？", DocType: "text", CKType: "text", TKNums: intPtr(3)},
			{Text: "名称：检索增强生成", DocType: "text", CKType: "text", TKNums: intPtr(4)},
			{Text: "发布日期：2026-01-01", DocType: "text", CKType: "text", TKNums: intPtr(3)},
			{Text: "答：RAG是检索增强生成。", DocType: "text", CKType: "text", TKNums: intPtr(2)},
			{Text: "", DocType: "text", CKType: "text", TKNums: intPtr(0)},
		}
		got := mergeDOCXUnits(units, 1024, true, "问：", "\n")
		texts := generalChunkTexts(got)
		if len(texts) != 1 {
			t.Fatalf("texts = %q, want exactly one chunk (record grouped)", texts)
		}
		want := "问：什么是RAG？\n名称：检索增强生成\n发布日期：2026-01-01\n答：RAG是检索增强生成。"
		if texts[0] != want {
			t.Fatalf("texts[0] = %q, want %q", texts[0], want)
		}
	})

	t.Run("multiple-records-keep-boundaries", func(t *testing.T) {
		units := []schema.ChunkDoc{
			{Text: "问：Q1", DocType: "text", CKType: "text", TKNums: intPtr(1)},
			{Text: "答：A1", DocType: "text", CKType: "text", TKNums: intPtr(1)},
			{Text: "", DocType: "text", CKType: "text", TKNums: intPtr(0)},
			{Text: "问：Q2", DocType: "text", CKType: "text", TKNums: intPtr(1)},
			{Text: "答：A2", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		}
		got := mergeDOCXUnits(units, 1024, true, "问：", "\n")
		texts := generalChunkTexts(got)
		if len(texts) != 2 {
			t.Fatalf("texts = %q, want exactly two chunks", texts)
		}
		want1 := "问：Q1\n答：A1"
		want2 := "问：Q2\n答：A2"
		if texts[0] != want1 {
			t.Fatalf("texts[0] = %q, want %q", texts[0], want1)
		}
		if texts[1] != want2 {
			t.Fatalf("texts[1] = %q, want %q", texts[1], want2)
		}
	})

	t.Run("token-cap-enforced-mid-record", func(t *testing.T) {
		units := []schema.ChunkDoc{
			{Text: "问：开头", DocType: "text", CKType: "text", TKNums: intPtr(2)},
			{Text: "第一段", DocType: "text", CKType: "text", TKNums: intPtr(3)},
			{Text: "第二段", DocType: "text", CKType: "text", TKNums: intPtr(3)},
			{Text: "第三段", DocType: "text", CKType: "text", TKNums: intPtr(3)},
		}
		// target=5: the second paragraph pushes the running total to 5
		// (=target), so the third paragraph must start a new chunk even
		// though no prefix-match fires.
		got := mergeDOCXUnits(units, 5, true, "问：", "\n")
		texts := generalChunkTexts(got)
		if len(texts) < 2 {
			t.Fatalf("texts = %q, want token-cap to force a split", texts)
		}
		want := "问：开头\n第一段"
		if texts[0] != want {
			t.Fatalf("texts[0] = %q, want %q", texts[0], want)
		}
	})

	t.Run("legacy-no-prefix-keeps-no-merge-behaviour", func(t *testing.T) {
		units := []schema.ChunkDoc{
			{Text: "问：Q1", DocType: "text", CKType: "text", TKNums: intPtr(1)},
			{Text: "答：A1", DocType: "text", CKType: "text", TKNums: intPtr(1)},
			{Text: "问：Q2", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		}
		// customPrefix="" preserves the legacy "any custom-delimiter means
		// no merge" behaviour: every paragraph becomes its own chunk.
		got := mergeDOCXUnits(units, 1024, true, "", "\n")
		texts := generalChunkTexts(got)
		if len(texts) != 3 {
			t.Fatalf("texts = %q, want three standalone chunks", texts)
		}
	})

	t.Run("media-resets-merge-target", func(t *testing.T) {
		units := []schema.ChunkDoc{
			{Text: "问：Q1", DocType: "text", CKType: "text", TKNums: intPtr(1)},
			{Text: "first table", DocType: "text", CKType: "table", TKNums: intPtr(1)},
			{Text: "答：A1", DocType: "text", CKType: "text", TKNums: intPtr(1)},
		}
		got := mergeDOCXUnits(units, 1024, true, "问：", "\n")
		// text + table + text. The text after the table must NOT extend
		// the text before it: media breaks the merge target.
		if len(got) != 3 {
			t.Fatalf("got len = %d, want 3 (text, table, text)", len(got))
		}
		if got[0].CKType != "text" {
			t.Fatalf("got[0].CKType = %q, want text", got[0].CKType)
		}
		if got[1].CKType != "table" {
			t.Fatalf("got[1].CKType = %q, want table", got[1].CKType)
		}
		if got[2].CKType != "text" {
			t.Fatalf("got[2].CKType = %q, want text", got[2].CKType)
		}
	})

	t.Run("non-prefix-paragraph-extends-current-chunk", func(t *testing.T) {
		// Regression for the cycle-94 fix: a paragraph that does NOT start
		// with the prefix must extend the previous chunk even when the
		// running token total is well below chunk_token_size.
		units := []schema.ChunkDoc{
			{Text: "问：开头", DocType: "text", CKType: "text", TKNums: intPtr(2)},
			{Text: "第一段", DocType: "text", CKType: "text", TKNums: intPtr(2)},
			{Text: "第二段", DocType: "text", CKType: "text", TKNums: intPtr(2)},
		}
		got := mergeDOCXUnits(units, 1024, true, "问：", "\n")
		texts := generalChunkTexts(got)
		if len(texts) != 1 {
			t.Fatalf("texts = %q, want exactly one chunk (no prefix-match = extend)", texts)
		}
		want := "问：开头\n第一段\n第二段"
		if texts[0] != want {
			t.Fatalf("texts[0] = %q, want %q", texts[0], want)
		}
	})
}

func generalChunkTexts(chunks []schema.ChunkDoc) []string {
	texts := make([]string, len(chunks))
	for i := range chunks {
		texts[i] = chunks[i].Text
	}
	return texts
}

func generalTextInput(text string) map[string]any {
	return map[string]any{
		"name":          "document.txt",
		"file_type":     "txt",
		"output_format": "json",
		"json":          []map[string]any{{"text": text, "doc_type_kwd": "text"}},
	}
}

func outputChunks(t *testing.T, out map[string]any) []map[string]any {
	t.Helper()
	chunks, ok := out["chunks"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks = %T, want []map[string]any", out["chunks"])
	}
	return chunks
}

func outputTexts(t *testing.T, out map[string]any) []string {
	t.Helper()
	chunks := outputChunks(t, out)
	texts := make([]string, len(chunks))
	for i := range chunks {
		texts[i], _ = chunks[i]["text"].(string)
	}
	return texts
}
