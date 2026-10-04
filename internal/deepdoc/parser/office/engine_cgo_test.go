//go:build cgo

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

package office

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"
)

// TestOpenAndExtract_ValidDocReturnsViews locks the happy path of the
// revised 5-value OpenAndExtract contract under cgo: a real document yields a
// non-empty Markdown view and a nil error, and the Markdown error (mdErr) is
// surfaced as a distinct return so callers can fail loud when only that view
// fails (PR review Finding 1 + Finding 3). office_oxide always returns a
// structured IR for an openable document, so the all-views-empty join branch
// is exercised indirectly via the parser package's stub tests, not here.
func TestOpenAndExtract_ValidDocReturnsViews(t *testing.T) {
	data := minimalDOCX(t, "surviving content")
	irJSON, markdown, plainText, mdErr, err := OpenAndExtract(data, "docx")
	if err != nil {
		t.Fatalf("expected success for a document with content, got err: %v", err)
	}
	if strings.TrimSpace(irJSON) == "" {
		t.Error("expected non-empty IR for a document with content")
	}
	if strings.TrimSpace(markdown) == "" {
		t.Error("expected non-empty markdown for a document with content")
	}
	if strings.TrimSpace(plainText) == "" {
		t.Error("expected non-empty plain text for a document with content")
	}
	// A clean document has no Markdown render error to surface.
	if mdErr != nil {
		t.Errorf("expected nil mdErr for a clean document, got: %v", mdErr)
	}
	_ = plainText
}

func minimalDOCX(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZip(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)
	writeZip(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)
	writeZip(t, zw, "word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>`+text+`</w:t></w:r></w:p>
  </w:body>
</w:document>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

func writeZip(t *testing.T, zw *zip.Writer, name, body string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry %s: %v", name, err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write zip entry %s: %v", name, err)
	}
}
