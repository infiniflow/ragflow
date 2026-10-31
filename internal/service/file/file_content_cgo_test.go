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
//

//go:build cgo

package file

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalDOCX builds a one-paragraph .docx so the upload path exercises the
// office_oxide-backed DOCX parser without a checked-in binary fixture.
func minimalDOCX(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, body string) {
		t.Helper()
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	write("[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)
	write("_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)
	write("word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
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

// TestParseFileContent_DOCXReadsJSONItems pins the upload path contract after
// the parser became JSON-only: a .docx must be flattened from its structured
// JSON items into readable text. The DOCX parser's former Markdown default is
// gone, so a raw-bytes fallback (the parseFileContent failure path) would mean
// the JSON branch regressed.
func TestParseFileContent_DOCXReadsJSONItems(t *testing.T) {
	data := minimalDOCX(t, "Hello DOCX content")

	content := parseFileContent(t.Context(), "note.docx", data)
	if !strings.Contains(content, "Hello DOCX content") {
		t.Fatalf("docx content missing item text: %q", content)
	}
	if content == string(data) {
		t.Fatal("content fell back to raw docx bytes instead of parsed items")
	}
}

// TestOfficeContentExtractionWithoutMediaCache pins that previews, uploads and
// other non-ingestion callers use the text-only Office parsers: an
// image-bearing .docx must yield readable text instead of failing because no
// run media cache is attached.
func TestOfficeContentExtractionWithoutMediaCache(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "parser", "parser", "testdata", "docx_table_image.docx"))
	if err != nil {
		t.Fatal(err)
	}

	for name, parse := range map[string]func() (string, error){
		"file":  func() (string, error) { return parseFileContent(t.Context(), "image.docx", data), nil },
		"agent": func() (string, error) { return parseAgentUploadContent(t.Context(), "image.docx", data, "") },
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parse()
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"intro", "before"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing %q: %q", want, got)
				}
			}
			if strings.Contains(got, "media_ref") || strings.Contains(got, "base64,") {
				t.Fatalf("media payload leaked into text-only content: %q", got)
			}
		})
	}
}
