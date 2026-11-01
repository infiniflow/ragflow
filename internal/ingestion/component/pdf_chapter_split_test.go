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

package component

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
	"ragflow/internal/parser/splitter"

	"gorm.io/gorm"
)

func makeTestPDF(pages int) []byte {
	var buf []byte
	buf = append(buf, []byte("%PDF-1.4\n")...)

	offsets := make([]int, 3+pages)

	offsets[1] = len(buf)
	buf = append(buf, []byte("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")...)

	kids := ""
	for i := 0; i < pages; i++ {
		kids += fmt.Sprintf("%d 0 R ", 3+i)
	}

	offsets[2] = len(buf)
	buf = append(buf, []byte(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", kids, pages))...)

	for i := 0; i < pages; i++ {
		offsets[3+i] = len(buf)
		buf = append(buf, []byte(fmt.Sprintf("%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>\nendobj\n", 3+i))...)
	}

	xrefStart := len(buf)
	buf = append(buf, []byte(fmt.Sprintf("xref\n0 %d\n", 3+pages))...)
	buf = append(buf, []byte("0000000000 65535 f \n")...)
	for i := 1; i < 3+pages; i++ {
		buf = append(buf, []byte(fmt.Sprintf("%010d 00000 n \n", offsets[i]))...)
	}
	buf = append(buf, []byte(fmt.Sprintf("trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", 3+pages, xrefStart))...)
	return buf
}

func TestResolveSplitOptions_Defaults(t *testing.T) {
	setup := schema.ParserSetup{}
	opts := resolveSplitOptions(setup)

	if opts.ThresholdPages != splitter.DefaultThresholdPages {
		t.Errorf("ThresholdPages = %d, want %d", opts.ThresholdPages, splitter.DefaultThresholdPages)
	}
	if opts.MinPages != splitter.DefaultMinPages {
		t.Errorf("MinPages = %d, want %d", opts.MinPages, splitter.DefaultMinPages)
	}
	if opts.MaxPages != splitter.DefaultMaxPages {
		t.Errorf("MaxPages = %d, want %d", opts.MaxPages, splitter.DefaultMaxPages)
	}
	if opts.MaxConcurrency != splitter.DefaultMaxConcurrency {
		t.Errorf("MaxConcurrency = %d, want %d", opts.MaxConcurrency, splitter.DefaultMaxConcurrency)
	}
}

func TestResolveSplitOptions_SetupOverrides(t *testing.T) {
	setup := schema.ParserSetup{
		"chapter_split_threshold": 150,
		"chapter_min_pages":       20,
		"chapter_max_pages":       80,
		"chapter_concurrency":     4,
	}
	opts := resolveSplitOptions(setup)

	if opts.ThresholdPages != 150 {
		t.Errorf("ThresholdPages = %d, want 150", opts.ThresholdPages)
	}
	if opts.MinPages != 20 {
		t.Errorf("MinPages = %d, want 20", opts.MinPages)
	}
	if opts.MaxPages != 80 {
		t.Errorf("MaxPages = %d, want 80", opts.MaxPages)
	}
	if opts.MaxConcurrency != 4 {
		t.Errorf("MaxConcurrency = %d, want 4", opts.MaxConcurrency)
	}
}

func TestResolveSplitOptions_EnvOverrides(t *testing.T) {
	t.Setenv(common.EnvChapterSplitThreshold, "180")
	t.Setenv(common.EnvChapterMinPages, "25")
	t.Setenv(common.EnvChapterMaxPages, "90")
	t.Setenv(common.EnvChapterConcurrency, "3")

	setup := schema.ParserSetup{}
	opts := resolveSplitOptions(setup)

	if opts.ThresholdPages != 180 {
		t.Errorf("ThresholdPages = %d, want 180", opts.ThresholdPages)
	}
	if opts.MinPages != 25 {
		t.Errorf("MinPages = %d, want 25", opts.MinPages)
	}
	if opts.MaxPages != 90 {
		t.Errorf("MaxPages = %d, want 90", opts.MaxPages)
	}
	if opts.MaxConcurrency != 3 {
		t.Errorf("MaxConcurrency = %d, want 3", opts.MaxConcurrency)
	}
}

func TestShouldAdaptiveChapterSplit(t *testing.T) {
	pdfData := makeTestPDF(10)

	// Case 1: Disabled by default
	setup := schema.ParserSetup{}
	if shouldAdaptiveChapterSplit(setup, pdfData) {
		t.Error("expected false when not enabled")
	}

	// Case 2: Enabled via setup but below threshold
	setupEnabled := schema.ParserSetup{
		"enable_adaptive_chapter_split": true,
		"chapter_split_threshold":       20,
	}
	if shouldAdaptiveChapterSplit(setupEnabled, pdfData) {
		t.Error("expected false when 10 pages < threshold 20")
	}

	// Case 3: Enabled via setup and above threshold
	setupAbove := schema.ParserSetup{
		"enable_adaptive_chapter_split": true,
		"chapter_split_threshold":       5,
	}
	if !shouldAdaptiveChapterSplit(setupAbove, pdfData) {
		t.Error("expected true when 10 pages >= threshold 5")
	}

	// Case 4: Enabled via env
	t.Setenv(common.EnvEnableAdaptiveChapterSplit, "true")
	t.Setenv(common.EnvChapterSplitThreshold, "8")
	setupEmpty := schema.ParserSetup{}
	if !shouldAdaptiveChapterSplit(setupEmpty, pdfData) {
		t.Error("expected true when enabled via env and pages >= threshold")
	}
}

func TestDispatchAdaptiveChapterPDF_BelowThreshold(t *testing.T) {
	pdfData := makeTestPDF(4)
	setup := schema.ParserSetup{
		"enable_adaptive_chapter_split": true,
		"chapter_split_threshold":       10,
	}

	normalize := func(r parser.ParseResult) parser.ParseResult { return r }
	_, handled, err := dispatchAdaptiveChapterPDF(t.Context(), nil, "doc.pdf", pdfData, "", setup, normalize)
	if handled {
		t.Error("expected handled=false when below threshold")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestDispatchAdaptiveChapterPDF_ExecuteSplit(t *testing.T) {
	orig := pdfChapterDispatchSingle
	defer func() { pdfChapterDispatchSingle = orig }()

	callCount := 0
	pdfChapterDispatchSingle = func(ctx context.Context, db *gorm.DB, filename string, binary []byte, tenantID string, setup schema.ParserSetup) (parser.ParseResult, bool, error) {
		callCount++
		return parser.ParseResult{
			File: map[string]any{"format": "pdf"},
			JSON: []parser.DocItem{
				parser.TextItem{
					Text:  fmt.Sprintf("Content of %s", filename),
					Extra: map[string]any{"page_number": 1},
				},
			},
		}, true, nil
	}

	pdfData := makeTestPDF(6)
	setup := schema.ParserSetup{
		"enable_adaptive_chapter_split": true,
		"chapter_split_threshold":       4,
		"chapter_max_pages":             3,
		"chapter_min_pages":             2,
		"chapter_concurrency":           2,
	}

	normalize := func(r parser.ParseResult) parser.ParseResult { return r }
	result, handled, err := dispatchAdaptiveChapterPDF(t.Context(), nil, "bigdoc.pdf", pdfData, "tenant-1", setup, normalize)

	if !handled {
		t.Fatal("expected handled=true")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2 (split 6 pages into 3+3)", callCount)
	}
	if len(result.JSON) != 2 {
		t.Fatalf("len(result.JSON) = %d, want 2", len(result.JSON))
	}

	// Verify coordinate offset restoration
	it0 := result.JSON[0].(parser.TextItem)
	if it0.Extra["page_number"] != 1 {
		t.Errorf("item 0 page_number = %v, want 1", it0.Extra["page_number"])
	}

	it1 := result.JSON[1].(parser.TextItem)
	if it1.Extra["page_number"] != 4 {
		t.Errorf("item 1 page_number = %v, want 4 (1 + 3)", it1.Extra["page_number"])
	}

	if result.File["page_count"] != 6 {
		t.Errorf("File page_count = %v, want 6", result.File["page_count"])
	}
}

