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

package models

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
)

// naceNDIRecorder records the method, path and status of every request the
// driver sends to the API, so the test can check what was deleted.
type naceNDIRecorder struct {
	base http.RoundTripper
	mu   sync.Mutex
	log  []string
}

func (r *naceNDIRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.base.RoundTrip(req)
	if err == nil {
		r.mu.Lock()
		r.log = append(r.log, fmt.Sprintf("%s %s %d", req.Method, req.URL.Path, resp.StatusCode))
		r.mu.Unlock()
	}
	return resp, err
}

// naceNDISyntheticPDF builds a one-page PDF with a title and two text lines.
func naceNDISyntheticPDF() []byte {
	stream := "BT /F1 20 Tf 72 720 Td (Synthetic Parse Check) Tj ET\n" +
		"BT /F1 12 Tf 72 690 Td (This page was generated for an integration test.) Tj ET\n" +
		"BT /F1 12 Tf 72 670 Td (Widget count: 42) Tj ET\n"
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	var buf bytes.Buffer
	buf.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, object := range objects {
		offsets[i] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return buf.Bytes()
}

// TestNaceOCRFileLive parses a synthetic one-page PDF with production NDI
// through OCRFile, twice, and checks that every parse job and the uploaded
// file are deleted afterwards:
//   - pages 1-600 are more than one parse accepts (500 pages), so the whole
//     document is parsed without page_ranges and filtered locally;
//   - pages 1-300 are sent as page_ranges; NDI fails a range that far past the
//     last page with ocr_failed, so the driver parses the whole document once
//     more and keeps the selected pages.
//
// It runs two billed low-effort parses of one page (the failed job is not
// billed), so it needs NACE_NDI_LIVE_INTEGRATION=1 and NACE_API_KEY;
// NACE_BASE_URL optionally overrides the API base URL.
func TestNaceOCRFileLive(t *testing.T) {
	if os.Getenv("NACE_NDI_LIVE_INTEGRATION") != "1" {
		t.Skip("set NACE_NDI_LIVE_INTEGRATION=1 and NACE_API_KEY to run two billed parses with the live Nace API")
	}
	key := strings.TrimSpace(os.Getenv("NACE_API_KEY"))
	if key == "" {
		t.Fatal("NACE_API_KEY is required")
	}
	baseURL := strings.TrimSpace(os.Getenv("NACE_BASE_URL"))
	if baseURL == "" {
		baseURL = "https://console.nace.ai/v1"
	}
	driver := NewNaceModel(map[string]string{"default": baseURL}, URLSuffix{Models: "models", OCR: "documents/parse"})
	previous := naceNDIAPITransport
	t.Cleanup(func() { naceNDIAPITransport = previous })

	for _, tc := range []struct {
		name   string
		pages  [][]int
		parses int
	}{
		{"over the parse limit", [][]int{{1, 600}}, 1},
		{"ocr_failed fallback", [][]int{{1, 300}}, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := &naceNDIRecorder{base: naceNDITransport(previous)}
			naceNDIAPITransport = recorder
			model := "ndi-parse-low"
			resp, err := driver.OCRFile(t.Context(), &model, naceNDISyntheticPDF(), nil, &APIConfig{ApiKey: &key}, &OCRConfig{Pages: tc.pages}, nil)
			if err != nil {
				t.Fatalf("OCRFile: %v", err)
			}
			if resp.Text == nil || !strings.Contains(*resp.Text, "Widget count: 42") {
				t.Fatalf("text=%v", resp.Text)
			}
			if len(resp.Pages) != 1 || !resp.Pages[0].Normalized || len(resp.Pages[0].Blocks) == 0 {
				t.Fatalf("pages=%+v", resp.Pages)
			}
			for _, block := range resp.Pages[0].Blocks {
				if len(block.BBox) != 4 || block.BBox[0] < 0 || block.BBox[2] > 1 || block.BBox[1] < 0 || block.BBox[3] > 1 {
					t.Errorf("block %q box=%v", block.Text, block.BBox)
				}
				t.Logf("block label=%s box=%.3f text=%q", block.Label, block.BBox, block.Text)
			}

			// A job DELETE answers 2xx when done. A file DELETE confirms the
			// deletion only with 200 ("deleted"); 202 ("deleting") is
			// accepted, not confirmed.
			seen := map[string]int{}
			for _, entry := range recorder.log {
				t.Log(entry)
				fields := strings.Fields(entry)
				if len(fields) != 3 {
					continue
				}
				switch {
				case fields[0] == http.MethodPost && strings.HasSuffix(fields[1], "/documents/parse"):
					seen["parse"]++
				case fields[0] != http.MethodDelete:
				case strings.Contains(fields[1], "/documents/jobs/") && strings.HasPrefix(fields[2], "2"):
					seen["job"]++
				case strings.Contains(fields[1], "/documents/workspaces/") && fields[2] == "200":
					seen["file"]++
				case strings.Contains(fields[1], "/documents/workspaces/") && fields[2] == "202":
					t.Errorf("the file deletion was accepted (202 deleting) but not confirmed")
				}
			}
			if seen["parse"] != tc.parses || seen["job"] != tc.parses || seen["file"] != 1 {
				t.Fatalf("want %d parse jobs, each deleted, and the uploaded file deleted: %v", tc.parses, recorder.log)
			}
		})
	}
}
