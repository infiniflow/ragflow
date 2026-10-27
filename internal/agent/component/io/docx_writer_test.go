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

package io

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// TestDOCXWriter_MinimalDocument: the smallest possible DOCX — no
// header, no footer, no watermark, no page numbers. The output must be
// a valid ZIP starting with the PK magic and contain a document.xml
// with the source text.
func TestDOCXWriter_MinimalDocument(t *testing.T) {
	doc, err := WriteDOCX("Hello", DOCXOptions{})
	if err != nil {
		t.Fatalf("WriteDOCX: %v", err)
	}
	if len(doc) < 4 {
		t.Fatalf("doc too small: %d bytes", len(doc))
	}
	if !bytes.HasPrefix(doc, []byte{'P', 'K', 0x03, 0x04}) {
		t.Fatalf("doc does not start with ZIP magic; first 4 bytes: % x", doc[:4])
	}
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	body, ok := readZipFile(t, zr, "word/document.xml")
	if !ok {
		t.Fatal("word/document.xml not found in zip")
	}
	if !strings.Contains(body, "Hello") {
		t.Errorf("document.xml missing source text; first 200 chars:\n%s", truncate(body, 200))
	}
	// The static parts should always be present.
	if _, ok := readZipFile(t, zr, "[Content_Types].xml"); !ok {
		t.Error("[Content_Types].xml missing")
	}
	if _, ok := readZipFile(t, zr, "_rels/.rels"); !ok {
		t.Error("_rels/.rels missing")
	}
}

// TestDOCXWriter_WithHeader: when HeaderText is set, the produced
// zip must contain word/header1.xml with the header text and a
// corresponding relationship entry in document.xml.rels.
func TestDOCXWriter_WithHeader(t *testing.T) {
	doc, err := WriteDOCX("X", DOCXOptions{HeaderText: "TOP"})
	if err != nil {
		t.Fatalf("WriteDOCX: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	hdr, ok := readZipFile(t, zr, "word/header1.xml")
	if !ok {
		t.Fatal("word/header1.xml missing when HeaderText set")
	}
	if !strings.Contains(hdr, "TOP") {
		t.Errorf("header1.xml missing 'TOP':\n%s", truncate(hdr, 200))
	}
	rels, ok := readZipFile(t, zr, "word/_rels/document.xml.rels")
	if !ok {
		t.Fatal("word/_rels/document.xml.rels missing")
	}
	if !strings.Contains(rels, "rIdHeader1") || !strings.Contains(rels, "header1.xml") {
		t.Errorf("document.xml.rels missing header relationship:\n%s", truncate(rels, 200))
	}
}

// TestDOCXWriter_XMLEscape: source content with <, >, &, " must be
// XML-escaped in the produced document.xml — the writer must never
// let raw user content break the OOXML topology.
func TestDOCXWriter_XMLEscape(t *testing.T) {
	in := `A < B & C > D "quoted"`
	doc, err := WriteDOCX(in, DOCXOptions{})
	if err != nil {
		t.Fatalf("WriteDOCX: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	body, ok := readZipFile(t, zr, "word/document.xml")
	if !ok {
		t.Fatal("word/document.xml missing")
	}
	// Escaped forms must appear. html.EscapeString produces the
	// standard XML entity set: &lt; / &gt; / &amp; / &#34; (numeric
	// for the double-quote, matching Go's stdlib contract).
	want := "A &lt; B &amp; C &gt; D &#34;quoted&#34;"
	if !strings.Contains(body, want) {
		t.Errorf("expected XML-escaped content %q, got:\n%s", want, truncate(body, 400))
	}
	// Raw < and & must NOT appear inside the <w:t> text run.
	if strings.Contains(body, "A < B &") {
		t.Errorf("raw 'A < B &' leaked into document.xml")
	}
}

// TestDOCXWriter_Watermark: setting WatermarkText should produce a
// header with the VML watermark shape, and the document.xml.rels
// should still include the header reference.
func TestDOCXWriter_Watermark(t *testing.T) {
	doc, err := WriteDOCX("body", DOCXOptions{WatermarkText: "DRAFT"})
	if err != nil {
		t.Fatalf("WriteDOCX: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	hdr, ok := readZipFile(t, zr, "word/header1.xml")
	if !ok {
		t.Fatal("header1.xml missing when WatermarkText set")
	}
	if !strings.Contains(hdr, "DRAFT") {
		t.Errorf("header1.xml missing watermark text 'DRAFT'")
	}
	if !strings.Contains(hdr, "v:textpath") {
		t.Errorf("header1.xml missing v:textpath (VML watermark shape)")
	}
}

// TestDOCXWriter_EmptyContent: an empty content string should still
// produce a valid DOCX (one empty paragraph).
func TestDOCXWriter_EmptyContent(t *testing.T) {
	doc, err := WriteDOCX("", DOCXOptions{})
	if err != nil {
		t.Fatalf("WriteDOCX: %v", err)
	}
	if len(doc) < 4 || !bytes.HasPrefix(doc, []byte{'P', 'K', 0x03, 0x04}) {
		t.Fatalf("expected ZIP magic, got: % x", doc[:4])
	}
}

func TestDOCXWriter_MarkdownTables(t *testing.T) {
	content := "Before table\n\n| A | B | C |\n| --- | --- | --- |\n| one | two | three |\n| 1 | 2 | 3 |\n\n" +
		"```markdown\n| X | Y | Z |\n| --- | --- | --- |\n| α & < | 二 | 3 |\n| x | y | z |\n```\n\n" +
		"| L | M | N |\n| --- | --- |\n| four | five | six |\n| 4 | 5 | 6 |\n\n" +
		"```text\n| not | a | table |\n| --- | --- | --- |\n```\nAfter table"
	doc, err := WriteDOCX(content, DOCXOptions{})
	if err != nil {
		t.Fatalf("WriteDOCX: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if err != nil {
		t.Fatalf("zip.NewReader: %v", err)
	}
	body, ok := readZipFile(t, zr, "word/document.xml")
	if !ok {
		t.Fatal("word/document.xml missing")
	}
	decoder := xml.NewDecoder(strings.NewReader(body))
	var tables, rows, cells int
	var inCell bool
	var values []string
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("decode document.xml: %v", err)
		}
		switch token := tok.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "tbl":
				tables++
			case "tr":
				rows++
			case "tc":
				cells++
				inCell = true
			}
		case xml.EndElement:
			if token.Name.Local == "tc" {
				inCell = false
			}
		case xml.CharData:
			if inCell && strings.TrimSpace(string(token)) != "" {
				values = append(values, strings.TrimSpace(string(token)))
			}
		}
	}
	if tables != 3 || rows != 9 || cells != 27 {
		t.Fatalf("DOCX table structure = %d tables, %d rows, %d cells; want 3, 9, 27", tables, rows, cells)
	}
	wantValues := []string{"A", "B", "C", "one", "two", "three", "1", "2", "3", "X", "Y", "Z", "α & <", "二", "3", "x", "y", "z", "L", "M", "N", "four", "five", "six", "4", "5", "6"}
	if strings.Join(values, "\x00") != strings.Join(wantValues, "\x00") {
		t.Errorf("table cells = %q, want %q", values, wantValues)
	}
	if strings.Index(body, "Before table") > strings.Index(body, "<w:tbl>") || strings.Index(body, "<w:tbl>") > strings.Index(body, "After table") {
		t.Error("table changed its order relative to surrounding paragraphs")
	}
	if !strings.Contains(body, "Before table") || !strings.Contains(body, "After table") || !strings.Contains(body, "```text\n| not | a | table |\n| --- | --- | --- |\n```") {
		t.Errorf("paragraph or non-Markdown code content changed: %s", truncate(body, 1200))
	}
}

// readZipFile returns the file body as a string, or ("", false) if the
// file is not present.
func readZipFile(t *testing.T, zr *zip.Reader, name string) (string, bool) {
	t.Helper()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		return string(b), true
	}
	return "", false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
