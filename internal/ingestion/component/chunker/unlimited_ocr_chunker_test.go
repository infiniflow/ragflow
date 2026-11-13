//
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

package chunker

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"ragflow/internal/ingestion/component"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/parser"
)

// TestUnlimitedOCR_CrossPageTablePreservedInChunker asserts that a table spanning
// two pages in content_list is recognized as a single merged table in markdown and
// arrives at the chunker as one table chunk, preserving heading_level on the title.
func TestUnlimitedOCR_CrossPageTablePreservedInChunker(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"msg":     "Task accepted",
				"task_id": "task_chunker_crosspage",
				"data": map[string]any{
					"task_id": "task_chunker_crosspage",
				},
			})
		case "/tasks/task_chunker_crosspage/result":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":  0,
				"msg":   "success",
				"state": "done",
				"results": map[string]any{
					"earnings.pdf": map[string]any{
						"md_content": "# Earnings Overview\n\nQuarterly summary report.\n\n| Quarter | Revenue | Profit |\n| --- | --- | --- |\n| Q1 | 100M | 20M |\n| Q2 | 120M | 25M |\n| Q3 | 140M | 30M |\n| Q4 | 160M | 35M |",
						"content_list": []map[string]any{
							{
								"type":     "title",
								"text":     "Earnings Overview",
								"page_idx": 0,
								"bbox":     []float64{100, 100, 800, 150},
							},
							{
								"type":     "text",
								"text":     "Quarterly summary report.",
								"page_idx": 0,
								"bbox":     []float64{100, 160, 800, 220},
							},
							{
								"type":       "table",
								"table_body": "<table><tr><th>Quarter</th><th>Revenue</th><th>Profit</th></tr><tr><td>Q1</td><td>100M</td><td>20M</td></tr><tr><td>Q2</td><td>120M</td><td>25M</td></tr></table>",
								"page_idx":   0,
								"bbox":       []float64{100, 240, 800, 700},
							},
							{
								"type":       "append_table",
								"table_body": "<table><tr><td>Q3</td><td>140M</td><td>30M</td></tr><tr><td>Q4</td><td>160M</td><td>35M</td></tr></table>",
								"page_idx":   1,
								"bbox":       []float64{100, 100, 800, 500},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": mockServer.URL,
	}

	pdfPath := filepath.Join("..", "..", "..", "deepdoc", "parser", "pdf", "pdfium", "testdata", "page_sizes.pdf")
	pdfBinary, err := os.ReadFile(pdfPath)
	if err != nil {
		pdfBinary = []byte("%PDF-1.4 test")
	}

	result, err := component.DispatchUnlimitedOCR2PDFForTest(
		context.Background(),
		nil,
		"earnings.pdf",
		pdfBinary,
		"",
		setup,
		"",
	)
	if err != nil {
		t.Fatalf("DispatchUnlimitedOCR2PDFForTest: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}

	// Invoke GeneralChunker with the parser output
	general, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512, "table_context_size": 32})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}

	parsedInput := map[string]any{
		"name":          "earnings.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json":          parser.ItemsToMaps(result.JSON),
		"file":          result.File,
	}

	chunked, err := general.Invoke(context.Background(), nil, parsedInput)
	if err != nil {
		t.Fatalf("general.Invoke: %v", err)
	}

	chunks, ok := schema.ChunkCarrierToMaps(chunked["chunks"])
	if !ok || len(chunks) == 0 {
		t.Fatalf("expected chunks in chunker output, got %#v", chunked["chunks"])
	}

	tableChunkCount := 0
	titleHeadingLevelFound := false
	for _, ck := range chunks {
		docType, _ := ck["doc_type_kwd"].(string)
		if docType == "table" {
			tableChunkCount++
		}
		if lvl, ok := ck["heading_level"].(float64); ok && lvl == 1 {
			titleHeadingLevelFound = true
		} else if lvl, ok := ck["heading_level"].(int); ok && lvl == 1 {
			titleHeadingLevelFound = true
		}
	}

	if tableChunkCount != 1 {
		t.Fatalf("expected exactly 1 table chunk from cross-page table, got %d", tableChunkCount)
	}
	if !titleHeadingLevelFound {
		t.Errorf("expected title chunk to preserve heading_level=1, chunks: %#v", chunks)
	}
}

// TestUnlimitedOCR_StackedTablesPreservedWithDistinctPositionsInChunker asserts that
// two consecutive stacked tables on the same page are not collapsed into a single group,
// so that each table chunk in the chunker receives its own distinct visual grounding box.
func TestUnlimitedOCR_StackedTablesPreservedWithDistinctPositionsInChunker(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/file_parse":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":    0,
				"msg":     "Task accepted",
				"task_id": "task_stacked_tables",
				"data": map[string]any{
					"task_id": "task_stacked_tables",
				},
			})
		case "/tasks/task_stacked_tables/result":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code":  0,
				"msg":   "success",
				"state": "done",
				"results": map[string]any{
					"stacked.pdf": map[string]any{
						"md_content": "# Stacked Tables\n\n| Item | Count |\n| --- | --- |\n| A | 1 |\n| B | 2 |\n\n| Metric | Value |\n| --- | --- |\n| Speed | 100 |\n| Memory | 200 |",
						"content_list": []map[string]any{
							{
								"type":     "title",
								"text":     "Stacked Tables",
								"page_idx": 0,
								"bbox":     []float64{100, 50, 800, 90},
							},
							{
								"type":       "table",
								"table_body": "<table><tr><th>Item</th><th>Count</th></tr><tr><td>A</td><td>1</td></tr><tr><td>B</td><td>2</td></tr></table>",
								"page_idx":   0,
								"bbox":       []float64{100, 100, 800, 300},
							},
							{
								"type":       "table",
								"table_body": "<table><tr><th>Metric</th><th>Value</th></tr><tr><td>Speed</td><td>100</td></tr><tr><td>Memory</td><td>200</td></tr></table>",
								"page_idx":   0,
								"bbox":       []float64{100, 350, 800, 550},
							},
						},
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": mockServer.URL,
	}

	pdfPath := filepath.Join("..", "..", "..", "deepdoc", "parser", "pdf", "pdfium", "testdata", "page_sizes.pdf")
	pdfBinary, err := os.ReadFile(pdfPath)
	if err != nil {
		pdfBinary = []byte("%PDF-1.4 test")
	}

	result, err := component.DispatchUnlimitedOCR2PDFForTest(
		context.Background(),
		nil,
		"stacked.pdf",
		pdfBinary,
		"",
		setup,
		"",
	)
	if err != nil {
		t.Fatalf("DispatchUnlimitedOCR2PDFForTest: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}

	// Verify parser output has 2 distinct table items with distinct positions
	tableItems := []map[string]any{}
	for _, it := range result.JSON {
		m := it.ToMap()
		if m["doc_type_kwd"] == "table" || m["layout"] == "table" {
			tableItems = append(tableItems, m)
		}
	}
	if len(tableItems) != 2 {
		t.Fatalf("expected 2 table items in parser JSON, got %d", len(tableItems))
	}

	general, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512, "table_context_size": 32})
	if err != nil {
		t.Fatalf("NewGeneralChunker: %v", err)
	}

	parsedInput := map[string]any{
		"name":          "stacked.pdf",
		"file_type":     "pdf",
		"output_format": "json",
		"json":          parser.ItemsToMaps(result.JSON),
		"file":          result.File,
	}

	chunked, err := general.Invoke(context.Background(), nil, parsedInput)
	if err != nil {
		t.Fatalf("general.Invoke: %v", err)
	}

	chunks, ok := schema.ChunkCarrierToMaps(chunked["chunks"])
	if !ok || len(chunks) == 0 {
		t.Fatalf("expected chunks in chunker output, got %#v", chunked["chunks"])
	}

	var tableChunks []map[string]any
	for _, ck := range chunks {
		if docType, _ := ck["doc_type_kwd"].(string); docType == "table" {
			tableChunks = append(tableChunks, ck)
		}
	}

	if len(tableChunks) != 2 {
		t.Fatalf("expected 2 distinct table chunks for stacked tables, got %d", len(tableChunks))
	}

	// Under CGO (where page sizes can be resolved), verify each table chunk has its own distinct positions
	pos1, ok1 := tableChunks[0]["_pdf_positions"].([][]any)
	pos2, ok2 := tableChunks[1]["_pdf_positions"].([][]any)
	if ok1 && ok2 && len(pos1) > 0 && len(pos2) > 0 {
		top1, _ := pos1[0][3].(float64)
		top2, _ := pos2[0][3].(float64)
		if top1 == top2 {
			t.Errorf("expected distinct vertical positions for stacked tables, got top1=%v top2=%v", top1, top2)
		}
	}
}
