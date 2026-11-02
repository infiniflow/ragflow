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

package parser

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestPDFParser_NormalizeMarkdown_ConvertsToJSON pins that a backend markdown
// payload is converted into normalized JSON items with doc_type_kwd set.
func TestPDFParser_NormalizeMarkdown_ConvertsToJSON(t *testing.T) {
	p := NewPDFParser()
	res := p.NormalizeMarkdown(context.Background(), "doc.md", "# Title\n\nSome body text.\n")
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	if !res.Produced() {
		t.Fatal("NormalizeMarkdown did not produce a payload")
	}
	if len(res.JSON) == 0 {
		t.Fatal("empty JSON after markdown normalization")
	}
	for _, it := range res.JSON {
		if _, ok := it.ToMap()["doc_type_kwd"]; !ok {
			t.Errorf("item missing doc_type_kwd: %#v", it.ToMap())
		}
	}
}

// TestPDFParser_NormalizeMarkdown_DoesNotFetchRemoteImages pins that backend
// markdown normalization never pulls remote image URLs (untrusted OCR response,
// deterministic local normalization only). ssrfAllowLoopback is enabled so the
// SSRF guard does not reject the loopback request before the handler runs; that
// would make requests stay 0 regardless and mask a regression where NormalizeMarkdown
// started fetching remote images.
func TestPDFParser_NormalizeMarkdown_DoesNotFetchRemoteImages(t *testing.T) {
	prev := ssrfAllowLoopback
	ssrfAllowLoopback = true
	t.Cleanup(func() { ssrfAllowLoopback = prev })

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewPDFParser()
	res := p.NormalizeMarkdown(context.Background(), "doc.md", "![remote]("+srv.URL+"/img.png)\n")
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	if requests != 0 {
		t.Errorf("NormalizeMarkdown fetched %d remote image(s); want 0", requests)
	}
}

// TestPDFParser_NormalizeMarkdown_FlattenMediaToText pins that, with flatten
// enabled, image markdown is flattened to a text carrier (doc_type_kwd == text).
func TestPDFParser_NormalizeMarkdown_FlattenMediaToText(t *testing.T) {
	p := NewPDFParser()
	p.FlattenMediaToText = true
	res := p.NormalizeMarkdown(context.Background(), "doc.md", "![img](data:image/png;base64,abc)\n")
	if res.Err != nil {
		t.Fatalf("err: %v", res.Err)
	}
	for _, it := range res.JSON {
		if dt, _ := it.ToMap()["doc_type_kwd"].(string); dt != "text" {
			t.Errorf("doc_type_kwd = %q, want text", dt)
		}
	}
}

// TestPDFParser_NormalizeMarkdown_PreservesFileOnCancel pins that a cancelled
// context returns the error (not silently swallowed) and still carries the
// filename in File so callers can report which document failed.
func TestPDFParser_NormalizeMarkdown_PreservesFileOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewPDFParser()
	res := p.NormalizeMarkdown(ctx, "doc.md", "# Title\n\nbody\n")
	if res.Err == nil {
		t.Fatal("expected cancellation error")
	}
	if _, ok := res.File["name"]; !ok {
		t.Error("File.name not preserved on cancellation")
	}
}

// TestPDFParser_NormalizeMarkdown_IdempotentAfterSecondNormalize pins that the
// result of NormalizeMarkdown can be safely passed through NormalizeResult again
// (the PDF vision dispatch path re-normalizes already-normalized JSON), with no
// change to item shape.
func TestPDFParser_NormalizeMarkdown_IdempotentAfterSecondNormalize(t *testing.T) {
	p := NewPDFParser()
	res1 := p.NormalizeMarkdown(context.Background(), "doc.md",
		"# Title\n\n| A | B |\n| --- | --- |\n| 1 | 2 |\n\nbody\n")
	if res1.Err != nil {
		t.Fatalf("err: %v", res1.Err)
	}
	res2 := p.NormalizeResult(context.Background(), "doc.md", res1)
	if res2.Err != nil {
		t.Fatalf("second normalize err: %v", res2.Err)
	}
	if len(res1.JSON) != len(res2.JSON) {
		t.Fatalf("item count changed: %d -> %d", len(res1.JSON), len(res2.JSON))
	}
	for i := range res1.JSON {
		a, b := res1.JSON[i].ToMap(), res2.JSON[i].ToMap()
		if a["doc_type_kwd"] != b["doc_type_kwd"] {
			t.Errorf("doc_type_kwd mismatch at item %d: %v -> %v", i, a["doc_type_kwd"], b["doc_type_kwd"])
		}
	}
}

// TestPDFParser_NormalizeMarkdown_EmptyFallsBackToText pins the historical text
// fallback: a backend markdown that yields no structured items is still indexed
// as a single raw-text item rather than failing the task.
func TestPDFParser_NormalizeMarkdown_EmptyFallsBackToText(t *testing.T) {
	p := NewPDFParser()
	res := p.NormalizeMarkdown(context.Background(), "ocr.pdf", "")
	if res.Err != nil {
		t.Fatalf("NormalizeMarkdown returned error: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("empty markdown must fall back to a single text item, got %d", len(res.JSON))
	}
	if text, _ := res.JSON[0].ToMap()["text"].(string); text != "" {
		t.Errorf("fallback text item should carry the raw markdown, got %q", text)
	}
}
