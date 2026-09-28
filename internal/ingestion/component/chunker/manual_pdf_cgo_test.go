//go:build cgo && integration

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
	"os"
	"path/filepath"
	"testing"

	native_analyzer "ragflow/internal/deepdoc/parser/pdf/inference/native_analyzer"
	deepdoctype "ragflow/internal/deepdoc/parser/type"
	"ragflow/internal/parser/parser"
)

// TestManualChunker_RealPDFResort proves the (page, top, left) resort operates
// on the ACTUAL records the Go PDF parser produces. This closes gap 1 from the
// review: the template integration test only feeds a .txt fixture with no
// coordinates, so nothing exercised the resort against real parser output.
//
// It parses a real PDF, normalizes its records through the SAME extractLineRecords
// path the chunker uses in production, then asserts the resort yields a
// non-decreasing physical order — i.e. the shared pdfPosRowLess comparator
// genuinely consumes the parser-emitted _pdf_positions rather than silently
// no-op'ing. The assertion is layout-independent: whether or not the reading
// order differed from the physical order, the sorted result must be monotonic.
func TestManualChunker_RealPDFResort(t *testing.T) {
	// The real-PDF resort exercises the in-process DeepDoc backend — now the sole
	// inference path. Register it here (following the convention used by the other
	// integration tests) so the test runs for real wherever the models + static
	// ONNX Runtime are present. The CI runner bakes both (MODEL_DIR is set to
	// /opt/ragflow-deepdoc-models), so the generic integration job exercises this
	// path instead of skipping it. When DEEPDOC_NATIVE_REQUIRED=1 the backend is
	// mandatory, so a missing prerequisite fails loudly rather than silently.
	modelDir := os.Getenv("MODEL_DIR")
	if modelDir == "" {
		if os.Getenv("DEEPDOC_NATIVE_REQUIRED") == "1" {
			t.Fatalf("MODEL_DIR must be set: the in-process DeepDoc backend is required (DEEPDOC_NATIVE_REQUIRED=1)")
		}
		t.Skip("set MODEL_DIR to run the real-PDF resort test (in-process DeepDoc backend)")
	}
	if err := native_analyzer.Register(modelDir, native_analyzer.DefaultDropScore); err != nil {
		if os.Getenv("DEEPDOC_NATIVE_REQUIRED") == "1" {
			t.Fatalf("in-process DeepDoc backend unavailable but required (DEEPDOC_NATIVE_REQUIRED=1): %v", err)
		}
		t.Skipf("in-process DeepDoc backend unavailable, skipping real-PDF resort test: %v", err)
	}
	t.Cleanup(func() { deepdoctype.SetNativeDocAnalyzerFactory(nil) })

	path := filepath.Join("..", "..", "..", "..", "test", "benchmark", "test_docs", "Doc1.pdf")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	pdf := parser.NewPDFParser()
	res := pdf.ParseWithResult(t.Context(), "Doc1.pdf", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) == 0 {
		t.Fatal("parser produced no JSON items")
	}

	// Same shape the chunker receives in production.
	input := map[string]any{
		"name":          "Doc1.pdf",
		"output_format": "json",
		"chunks":        res.JSON,
	}
	records := extractLineRecords(input)
	if len(records) == 0 {
		t.Fatal("extractLineRecords produced no records from real parser output")
	}

	positioned := 0
	for _, r := range records {
		if _, ok := firstPositionRow(r); ok {
			positioned++
		}
	}
	if positioned == 0 {
		t.Fatal("no real parser records carried _pdf_positions; resort would be a no-op")
	}

	sortRecordsByPosition(records)

	// After the resort, the physical (page, top, left) order must be
	// non-decreasing. pdfPosRowLess(cur, prev) being true would mean a record
	// physically precedes its predecessor — a resort violation. A coordinate-free
	// record (no firstPositionRow) is skipped and never becomes prev, so prev may
	// be nil until the first positioned record; guard against that.
	var prev []float64
	for i := 0; i < len(records); i++ {
		cur, ok := firstPositionRow(records[i])
		if !ok {
			continue
		}
		if prev != nil && pdfPosRowLess(cur, prev) {
			t.Fatalf("resort produced out-of-order records at %d: row %v precedes %v", i, cur, prev)
		}
		prev = cur
	}
}
