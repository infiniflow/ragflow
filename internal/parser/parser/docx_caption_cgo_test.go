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

// docxCaptionStylesXML defines the built-in Heading1 and Caption styles.
// office_oxide resolves heading levels through the style sheet, so a
// document without it degrades headings to plain paragraphs.
const docxCaptionStylesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>
  <w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/></w:style>
  <w:style w:type="paragraph" w:styleId="Caption"><w:name w:val="caption"/></w:style>
</w:styles>`

// docxCaptionBody wraps the given <w:body> children in a document.
func docxCaptionBody(inner string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>` + inner + `
  </w:body>
</w:document>`
}

func docxCaptionPara(styleID, text string) string {
	return `<w:p><w:pPr><w:pStyle w:val="` + styleID + `"/></w:pPr><w:r><w:t>` + text + `</w:t></w:r></w:p>`
}

func docxCaptionTable() string {
	return `<w:tbl>
  <w:tr>
    <w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc>
    <w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc>
  </w:tr>
</w:tbl>`
}

func docxCaptionTestDOCX(t *testing.T, bodyInner string) []byte {
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
		"word/document.xml": docxCaptionBody(bodyInner),
	})
}

// docxCaptionFindTable returns the single TableData from a parse result.
func docxCaptionFindTable(t *testing.T, res ParseResult) *entity.TableData {
	t.Helper()
	for _, item := range res.JSON {
		if ti, ok := item.(TableItem); ok && ti.Table != nil {
			return ti.Table
		}
	}
	t.Fatal("no table item in parse result")
	return nil
}

func docxCaptionParse(t *testing.T, bodyInner string) ParseResult {
	p := NewDOCXParser()
	res := p.ParseWithResult(t.Context(), "caption.docx", docxCaptionTestDOCX(t, bodyInner))
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	return res
}

// TestDOCXParser_TableCaptionFromHeading_EndToEnd runs a real .docx
// (heading immediately above a table) through the native office_oxide
// extraction and asserts the table chunk carries the heading-location
// caption, rendered into the table HTML the chunker persists.
func TestDOCXParser_TableCaptionFromHeading_EndToEnd(t *testing.T) {
	res := docxCaptionParse(t, docxCaptionPara("Heading1", "设备台账")+docxCaptionTable())
	td := docxCaptionFindTable(t, res)
	if td.Caption != "Table Location: 设备台账" {
		t.Errorf("table caption = %q, want %q", td.Caption, "Table Location: 设备台账")
	}
	rendered := tableutil.RenderTableHTML(td)
	if !strings.Contains(rendered, "<caption>Table Location: 设备台账</caption>") {
		t.Errorf("rendered table HTML missing caption:\n%s", rendered)
	}
}

// TestDOCXParser_CaptionStyledParagraphBecomesTableCaption_EndToEnd pins the
// Go-side temporary stopgap (docx_caption_scan.go): a "Caption"-styled
// paragraph immediately above a table is attached as the table caption and is
// NOT emitted as its own text section. (office_oxide itself never surfaces the
// paragraph style; the Go parser recovers it by re-parsing document.xml.)
func TestDOCXParser_CaptionStyledParagraphBecomesTableCaption_EndToEnd(t *testing.T) {
	res := docxCaptionParse(t, docxCaptionPara("Caption", "设备流转明细")+docxCaptionTable())
	td := docxCaptionFindTable(t, res)
	if td.Caption != "设备流转明细" {
		t.Errorf("table caption = %q, want %q", td.Caption, "设备流转明细")
	}
	rendered := tableutil.RenderTableHTML(td)
	if !strings.Contains(rendered, "<caption>设备流转明细</caption>") {
		t.Errorf("rendered table HTML missing caption:\n%s", rendered)
	}
	for _, item := range res.JSON {
		if ti, ok := item.(TextItem); ok && ti.Text == "设备流转明细" {
			t.Errorf("Caption-styled paragraph must be consumed as a caption, not left as a text section")
		}
	}
}

// TestDOCXParser_CaptionPreferredOverHeading_EndToEnd asserts that when both a
// heading and a Caption-styled paragraph precede the table, the explicit
// caption wins over the synthesized heading-location caption.
func TestDOCXParser_CaptionPreferredOverHeading_EndToEnd(t *testing.T) {
	res := docxCaptionParse(t,
		docxCaptionPara("Heading1", "设备台账")+
			docxCaptionPara("Caption", "设备流转明细")+
			docxCaptionTable())
	td := docxCaptionFindTable(t, res)
	if td.Caption != "设备流转明细" {
		t.Errorf("table caption = %q, want explicit caption %q", td.Caption, "设备流转明细")
	}
}
