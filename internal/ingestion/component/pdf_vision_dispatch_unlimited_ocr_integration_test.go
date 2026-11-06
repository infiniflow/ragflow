//go:build integration

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

package component

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"ragflow/internal/ingestion/component/schema"
)

func TestDispatchUnlimitedOCR2PDF_LiveService(t *testing.T) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:18000/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Skip("Live Unlimited-OCR-2 service not available on 127.0.0.1:18000; skipping")
	}
	_ = resp.Body.Close()

	var pdfBytes []byte
	candidates := []string{
		"../../test/benchmark/test_docs/Doc1.pdf",
		"test/benchmark/test_docs/Doc1.pdf",
		"../../../test/benchmark/test_docs/Doc1.pdf",
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			pdfBytes = data
			break
		}
	}
	if len(pdfBytes) == 0 {
		t.Skip("Doc1.pdf benchmark sample not found; skipping live test")
	}

	setup := schema.ParserSetup{
		"unlimited_ocr_server_url": "http://127.0.0.1:18000",
	}

	result, err := dispatchUnlimitedOCR2PDF(
		context.Background(),
		nil,
		"Doc1.pdf",
		pdfBytes,
		"",
		setup,
		"",
	)
	if err != nil {
		t.Fatalf("dispatchUnlimitedOCR2PDF live: %v", err)
	}
	if result.Err != nil {
		t.Fatalf("result error: %v", result.Err)
	}
	if len(result.JSON) == 0 {
		t.Fatalf("expected non-empty JSON items in result")
	}

	found := false
	for _, it := range result.JSON {
		m := it.ToMap()
		if text, ok := m["text"].(string); ok && strings.Contains(text, "Purpose of RAGFlow") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected parsed items to contain 'Purpose of RAGFlow', got items: %#v", result.JSON)
	}
}
