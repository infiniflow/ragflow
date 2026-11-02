// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package splitter

import (
	"fmt"
	"reflect"
	"testing"

	doctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

func TestPlanAdaptiveChapters_BelowThreshold(t *testing.T) {
	opts := SplitOptions{
		MinPages:       30,
		MaxPages:       120,
		ThresholdPages: 200,
	}

	plans := PlanAdaptiveChapters(150, nil, opts)
	if len(plans) != 1 {
		t.Fatalf("expected 1 chapter for 150 pages below threshold, got %d", len(plans))
	}
	if plans[0].StartPage != 0 || plans[0].EndPage != 149 || plans[0].PageCount != 150 {
		t.Fatalf("unexpected plan: %+v", plans[0])
	}
}

func TestPlanAdaptiveChapters_NoOutlines(t *testing.T) {
	opts := SplitOptions{
		MinPages:       30,
		MaxPages:       100,
		ThresholdPages: 200,
	}

	plans := PlanAdaptiveChapters(250, nil, opts)
	if len(plans) != 3 {
		t.Fatalf("expected 3 chapters for 250 pages with max 100, got %d", len(plans))
	}

	expectedRanges := [][2]int{
		{0, 99},
		{100, 199},
		{200, 249},
	}
	for i, exp := range expectedRanges {
		if plans[i].StartPage != exp[0] || plans[i].EndPage != exp[1] {
			t.Errorf("plan %d: got [%d, %d], want [%d, %d]", i, plans[i].StartPage, plans[i].EndPage, exp[0], exp[1])
		}
		if plans[i].PageCount != exp[1]-exp[0]+1 {
			t.Errorf("plan %d: got PageCount %d, want %d", i, plans[i].PageCount, exp[1]-exp[0]+1)
		}
	}
}

func TestPlanAdaptiveChapters_HierarchicalTOC(t *testing.T) {
	opts := SplitOptions{
		MinPages:       30,
		MaxPages:       100,
		ThresholdPages: 200,
	}

	// 260 total pages with TOC
	outlines := []doctype.Outline{
		{Title: "Chapter 1", Level: 0, PageNumber: 1},
		{Title: "Chapter 2 (Small)", Level: 0, PageNumber: 41},
		{Title: "Chapter 3 (Large)", Level: 0, PageNumber: 61},
		{Title: "Section 3.1", Level: 1, PageNumber: 61},
		{Title: "Section 3.2", Level: 1, PageNumber: 131},
		{Title: "Chapter 4", Level: 0, PageNumber: 201},
	}

	plans := PlanAdaptiveChapters(260, outlines, opts)
	if len(plans) == 0 {
		t.Fatal("expected plans, got none")
	}

	// Verify continuous coverage
	curr := 0
	for i, p := range plans {
		if p.StartPage != curr {
			t.Errorf("plan %d (%s): start page %d != expected %d", i, p.Title, p.StartPage, curr)
		}
		if p.EndPage < p.StartPage {
			t.Errorf("plan %d (%s): invalid range [%d, %d]", i, p.Title, p.StartPage, p.EndPage)
		}
		if p.PageCount != p.EndPage-p.StartPage+1 {
			t.Errorf("plan %d: pageCount %d != %d", i, p.PageCount, p.EndPage-p.StartPage+1)
		}
		curr = p.EndPage + 1
	}
	if curr != 260 {
		t.Errorf("final page covered %d != 260", curr)
	}

	// Chapter 1 (1-40: 40 pages) and Chapter 2 (41-60: 20 pages) should coalesce into 0-59 (60 pages <= 100)
	if plans[0].StartPage != 0 || plans[0].EndPage != 59 {
		t.Errorf("expected plan 0 to coalesce Ch1+Ch2 into [0, 59], got [%d, %d]", plans[0].StartPage, plans[0].EndPage)
	}
}

func TestOffsetPositions(t *testing.T) {
	offset := 50

	table := &entity.TableData{
		Page: 2,
	}
	tableItem := parser.TableItem{
		Table: table,
		Positions: parser.Positions{
			parser.AbsoluteBox{PageNumber: 2, Left: 10, Right: 50, Top: 20, Bottom: 80},
		},
		Extra: map[string]any{
			"page_number": 2,
			"positions":   [][]any{{2, 10.0, 50.0, 20.0, 80.0}},
		},
	}

	imageItem := parser.ImageItem{
		Text: "Test Image",
		Positions: parser.Positions{
			parser.AbsoluteBox{PageNumber: 3, Left: 15, Right: 60, Top: 25, Bottom: 90},
		},
		Extra: map[string]any{
			"page_number": 3,
			"positions":   [][]float64{{3.0, 15.0, 60.0, 25.0, 90.0}},
		},
	}

	textItem := parser.TextItem{
		Text: "Sample text",
		Extra: map[string]any{
			"page_number": 4,
			"positions":   []float64{4.0, 5.0, 100.0, 10.0, 20.0},
		},
	}

	rawItem := parser.RawItem{
		M: map[string]any{
			"doc_type_kwd": "custom",
			"page_number":  5,
			"positions":    [][]any{{5, 1.0, 2.0, 3.0, 4.0}},
		},
	}

	items := []parser.DocItem{tableItem, imageItem, textItem, rawItem}
	offsetItems := OffsetPositions(items, offset)

	if len(offsetItems) != 4 {
		t.Fatalf("expected 4 offset items, got %d", len(offsetItems))
	}

	// 1. TableItem check
	resTable, ok := offsetItems[0].(parser.TableItem)
	if !ok {
		t.Fatalf("expected TableItem, got %T", offsetItems[0])
	}
	if resTable.Table.Page != 2+offset {
		t.Errorf("Table.Page = %d, want %d", resTable.Table.Page, 2+offset)
	}
	box0 := resTable.Positions[0].(parser.AbsoluteBox)
	if box0.PageNumber != 2+offset {
		t.Errorf("Table Positions[0].PageNumber = %d, want %d", box0.PageNumber, 2+offset)
	}
	if resTable.Extra["page_number"] != 2+offset {
		t.Errorf("Table Extra[page_number] = %v, want %d", resTable.Extra["page_number"], 2+offset)
	}
	extraPos := resTable.Extra["positions"].([][]any)
	if extraPos[0][0] != 2+offset {
		t.Errorf("Table Extra[positions][0][0] = %v, want %d", extraPos[0][0], 2+offset)
	}

	// 2. ImageItem check
	resImage, ok := offsetItems[1].(parser.ImageItem)
	if !ok {
		t.Fatalf("expected ImageItem, got %T", offsetItems[1])
	}
	imgBox := resImage.Positions[0].(parser.AbsoluteBox)
	if imgBox.PageNumber != 3+offset {
		t.Errorf("Image Positions[0].PageNumber = %d, want %d", imgBox.PageNumber, 3+offset)
	}
	imgPos := resImage.Extra["positions"].([][]float64)
	if imgPos[0][0] != float64(3+offset) {
		t.Errorf("Image Extra[positions][0][0] = %v, want %f", imgPos[0][0], float64(3+offset))
	}

	// 3. TextItem check
	resText, ok := offsetItems[2].(parser.TextItem)
	if !ok {
		t.Fatalf("expected TextItem, got %T", offsetItems[2])
	}
	if resText.Extra["page_number"] != 4+offset {
		t.Errorf("Text Extra[page_number] = %v, want %d", resText.Extra["page_number"], 4+offset)
	}
	textTuple := resText.Extra["positions"].([]float64)
	if textTuple[0] != float64(4+offset) {
		t.Errorf("Text Extra[positions][0] = %v, want %f", textTuple[0], float64(4+offset))
	}

	// 4. RawItem check
	resRaw, ok := offsetItems[3].(parser.RawItem)
	if !ok {
		t.Fatalf("expected RawItem, got %T", offsetItems[3])
	}
	if resRaw.M["page_number"] != 5+offset {
		t.Errorf("Raw page_number = %v, want %d", resRaw.M["page_number"], 5+offset)
	}
	rawPos := resRaw.M["positions"].([][]any)
	if rawPos[0][0] != 5+offset {
		t.Errorf("Raw positions[0][0] = %v, want %d", rawPos[0][0], 5+offset)
	}
}

func TestConcatenateResults(t *testing.T) {
	ch1 := parser.ParseResult{
		File: map[string]any{"page_count": 50, "format": "pdf"},
		JSON: []parser.DocItem{
			parser.TextItem{Text: "Chapter 1 text", Extra: map[string]any{"page_number": 1}},
		},
		Warnings: []string{"warning 1"},
	}
	ch2 := parser.ParseResult{
		File: map[string]any{"page_count": 50, "format": "pdf"},
		JSON: []parser.DocItem{
			parser.TextItem{Text: "Chapter 2 text", Extra: map[string]any{"page_number": 1}},
		},
		Warnings: []string{"warning 2"},
	}

	plans := []ChapterPlan{
		{Index: 0, Title: "Ch 1", StartPage: 0, EndPage: 49, PageCount: 50},
		{Index: 1, Title: "Ch 2", StartPage: 50, EndPage: 99, PageCount: 50},
	}

	final := ConcatenateResults(nil, []parser.ParseResult{ch1, ch2}, plans)
	if final.Err != nil {
		t.Fatalf("unexpected error: %v", final.Err)
	}
	if len(final.JSON) != 2 {
		t.Fatalf("expected 2 items, got %d", len(final.JSON))
	}

	// Verify page_count in File
	if final.File["page_count"] != 100 {
		t.Errorf("final page_count = %v, want 100", final.File["page_count"])
	}

	// Item 1 page_number should remain 1
	it0 := final.JSON[0].(parser.TextItem)
	if it0.Extra["page_number"] != 1 {
		t.Errorf("item 0 page_number = %v, want 1", it0.Extra["page_number"])
	}

	// Item 2 page_number should be 1 + 50 = 51
	it1 := final.JSON[1].(parser.TextItem)
	if it1.Extra["page_number"] != 51 {
		t.Errorf("item 1 page_number = %v, want 51", it1.Extra["page_number"])
	}

	if !reflect.DeepEqual(final.Warnings, []string{"warning 1", "warning 2"}) {
		t.Errorf("warnings = %v, want [warning 1, warning 2]", final.Warnings)
	}
}

func makeTestMultiPagePDF(pages int) []byte {
	var buf []byte
	buf = append(buf, []byte("%PDF-1.4\n")...)
	buf = append(buf, []byte("1 0 obj\n<</Type /Catalog /Pages 2 0 R>>\nendobj\n")...)
	kids := ""
	for i := 0; i < pages; i++ {
		kids += fmt.Sprintf("%d 0 R ", 3+i)
	}
	buf = append(buf, []byte(fmt.Sprintf("2 0 obj\n<</Type /Pages /Kids [%s] /Count %d>>\nendobj\n", kids, pages))...)
	for i := 0; i < pages; i++ {
		buf = append(buf, []byte(fmt.Sprintf("%d 0 obj\n<</Type /Page /Parent 2 0 R /MediaBox [0 0 612 792]>>\nendobj\n", 3+i))...)
	}
	startxref := len(buf)
	buf = append(buf, []byte("xref\n0 1\n0000000000 65535 f \n")...)
	buf = append(buf, []byte(fmt.Sprintf("trailer\n<</Size %d /Root 1 0 R>>\nstartxref\n%d\n%%%%EOF", 3+pages, startxref))...)
	return buf
}

func TestSlicePDF_Execution(t *testing.T) {
	pdfData := makeTestMultiPagePDF(5)
	subPDF, err := SlicePDF(pdfData, 1, 3)
	if err != nil {
		t.Fatalf("SlicePDF failed: %v", err)
	}
	if len(subPDF) == 0 {
		t.Fatal("SlicePDF returned empty slice")
	}
}

func TestPlanAdaptiveChapters_MinPagesRespected(t *testing.T) {
	opts := SplitOptions{
		MinPages:       40,
		MaxPages:       110,
		ThresholdPages: 100,
	}

	// Case 1: Three chapters of 50 pages each (total 150 pages > MaxPages 110).
	// Sec 1 (50 pages) + Sec 2 (50 pages) = 100 pages <= MaxPages (110).
	// Without MinPages enforcement, Sec 1 and Sec 2 would coalesce into 100 pages.
	// But because each section already satisfies MinPages (50 >= 40),
	// they should NOT coalesce and remain 3 distinct chapters.
	outlinesIndependent := []doctype.Outline{
		{Title: "Section 1", Level: 0, PageNumber: 1},
		{Title: "Section 2", Level: 0, PageNumber: 51},
		{Title: "Section 3", Level: 0, PageNumber: 101},
	}
	plans := PlanAdaptiveChapters(150, outlinesIndependent, opts)
	if len(plans) != 3 {
		t.Fatalf("expected 3 plans when each section >= MinPages, got %d", len(plans))
	}
	for i, p := range plans {
		if p.PageCount != 50 {
			t.Errorf("plan %d (%s) page count = %d, want 50", i, p.Title, p.PageCount)
		}
	}

	// Case 2: Small chapters (< MinPages) should be coalesced up to MaxPages.
	outlinesSmall := []doctype.Outline{
		{Title: "Small A", Level: 0, PageNumber: 1},
		{Title: "Small B", Level: 0, PageNumber: 21},
		{Title: "Small C", Level: 0, PageNumber: 41},
		{Title: "Big D", Level: 0, PageNumber: 61},
	}
	plansSmall := PlanAdaptiveChapters(120, outlinesSmall, opts)
	for i, p := range plansSmall {
		if p.PageCount < opts.MinPages {
			t.Errorf("plan %d (%s) page count %d < MinPages %d", i, p.Title, p.PageCount, opts.MinPages)
		}
	}
}
