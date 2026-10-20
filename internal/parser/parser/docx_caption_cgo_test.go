//go:build cgo

//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parser

import (
	"archive/zip"
	"bytes"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// docxCaptionZip builds a minimal .docx package from the given parts.
func docxCaptionZip(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range parts {
		writeZipFile(t, zw, name, body)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// docxCaptionDocumentXML is a body with a Heading1 paragraph, a
// Caption-styled paragraph and a one-row table, in that order.
const docxCaptionDocumentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>设备台账</w:t></w:r></w:p>
    <w:p><w:pPr><w:pStyle w:val="Caption"/></w:pPr><w:r><w:t>设备流转明细</w:t></w:r></w:p>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc>
      </w:tr>
    </w:tbl>
  </w:body>
</w:document>`

// docxCaptionStylesXML defines the built-in Heading1 and Caption styles.
// office_oxide resolves heading levels through the style sheet, so a
// document without it degrades headings to plain paragraphs.
const docxCaptionStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style>
  <w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="caption"/></w:style>
</w:styles>`

func docxCaptionTestDOCX(t *testing.T) []byte {
	t.Helper()
	return docxCaptionZip(t, map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>
</Relationships>`,
		"word/styles.xml":   docxCaptionStylesXML,
		"word/document.xml": docxCaptionDocumentXML,
	})
}

// TestDOCXParser_TableCaptionFromHeading_EndToEnd runs a real .docx
// through the native office_oxide extraction and asserts the table chunk
// carries the heading-location caption, rendered into the table HTML the
// chunker persists. This is the production path the pure-Go IR unit tests
// cannot exercise.
func TestDOCXParser_TableCaptionFromHeading_EndToEnd(t *testing.T) {
	p := NewDOCXParser()
	p.outputFormat = "json"
	res := p.ParseWithResult(t.Context(), "caption.docx", docxCaptionTestDOCX(t))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var td *entity.TableData
	for _, item := range res.JSON {
		ti, ok := item.(TableItem)
		if !ok || ti.Table == nil {
			continue
		}
		td = ti.Table
		break
	}
	if td == nil {
		t.Fatal("no table item in parse result")
	}
	if td.Caption != "Table Location: 设备台账" {
		t.Errorf("table caption = %q, want %q", td.Caption, "Table Location: 设备台账")
	}
	rendered := tableutil.RenderTableHTML(td)
	if !strings.Contains(rendered, "<caption>Table Location: 设备台账</caption>") {
		t.Errorf("rendered table HTML missing caption:\n%s", rendered)
	}
}

// TestDOCXParser_CaptionStyledParagraphStaysText_EndToEnd characterizes the
// current office_oxide contract: paragraph style names never reach the IR
// JSON, so a "Caption"-styled paragraph stays an ordinary text section and
// is not attached to the following table. If office_oxide starts emitting
// styles, this test fails and caption-style detection becomes implementable.
func TestDOCXParser_CaptionStyledParagraphStaysText_EndToEnd(t *testing.T) {
	p := NewDOCXParser()
	p.outputFormat = "json"
	res := p.ParseWithResult(t.Context(), "caption.docx", docxCaptionTestDOCX(t))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	var captionParagraphs, tableItems int
	for _, item := range res.JSON {
		ti, ok := item.(TableItem)
		if ok && ti.Table != nil {
			tableItems++
			if ti.Table.Caption != "" && ti.Table.Caption != "Table Location: 设备台账" {
				t.Errorf("unexpected caption %q: office_oxide must not surface paragraph styles", ti.Table.Caption)
			}
			continue
		}
		if textItem, ok := item.(TextItem); ok && textItem.Text == "设备流转明细" {
			captionParagraphs++
		}
	}
	if tableItems != 1 {
		t.Fatalf("table items = %d, want 1", tableItems)
	}
	if captionParagraphs != 1 {
		t.Errorf("Caption-styled paragraph sections = %d, want 1 (it must stay a text section)", captionParagraphs)
	}
}
