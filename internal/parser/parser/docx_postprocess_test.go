package parser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"reflect"
	"testing"
)

// helper: build a minimal docx ZIP in-memory with given header/footer XML.
func buildDocxZIP(t *testing.T, headerText, footerText string) []byte {
	t.Helper()
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	// Minimal required parts for a valid docx: [Content_Types].xml + word/document.xml.
	mustWrite := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(body))
	}
	mustWrite("[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="xml" ContentType="application/xml"/><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/></Types>`)
	mustWrite("word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Body text</w:t></w:r></w:p></w:body></w:document>`)
	if headerText != "" {
		mustWrite("word/header1.xml", `<?xml version="1.0"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>`+headerText+`</w:t></w:r></w:p></w:hdr>`)
	}
	if footerText != "" {
		mustWrite("word/footer1.xml", `<?xml version="1.0"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>`+footerText+`</w:t></w:r></w:p></w:ftr>`)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractDOCXHeaderFooterTexts(t *testing.T) {
	data := buildDocxZIP(t, "Company Confidential", "Page 1")
	texts := extractDOCXHeaderFooterTexts(data)
	if !texts["Company Confidential"] {
		t.Error("missing header text 'Company Confidential'")
	}
	if !texts["Page 1"] {
		t.Error("missing footer text 'Page 1'")
	}
}

func TestExtractDOCXHeaderFooterTexts_NoHeaders(t *testing.T) {
	data := buildDocxZIP(t, "", "")
	texts := extractDOCXHeaderFooterTexts(data)
	if len(texts) != 0 {
		t.Errorf("expected empty map, got %v", texts)
	}
}

func TestExtractDOCXHeaderFooterTexts_Normalization(t *testing.T) {
	// Whitespace runs should be collapsed and trimmed.
	data := buildDocxZIP(t, "  Multiple   Spaces  ", "")
	texts := extractDOCXHeaderFooterTexts(data)
	if !texts["Multiple Spaces"] {
		t.Errorf("expected normalized 'Multiple Spaces', got %v", texts)
	}
}

// TestRemoveDOCXHeaderFooterSections filters JSON items whose
// normalized text exactly matches a header/footer text.
func TestRemoveDOCXHeaderFooterSections(t *testing.T) {
	items := []map[string]any{
		{"text": "Company Confidential"},
		{"text": "Real content"},
		{"text": "Page 1"},
		{"text": "More content"},
	}
	hfTexts := map[string]bool{"Company Confidential": true, "Page 1": true}
	got := removeDOCXHeaderFooterSections(items, hfTexts)
	want := []map[string]any{
		{"text": "Real content"},
		{"text": "More content"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestRemoveDOCXHeaderFooterSections_EmptyHF keeps all items when no
// header/footer texts are provided.
func TestRemoveDOCXHeaderFooterSections_EmptyHF(t *testing.T) {
	items := []map[string]any{{"text": "A"}, {"text": "B"}}
	got := removeDOCXHeaderFooterSections(items, nil)
	if !reflect.DeepEqual(got, items) {
		t.Errorf("expected unchanged, got %+v", got)
	}
}

// TestExtractDOCXOutlines verifies heading elements are extracted
// from the office_oxide IR as (title, level) outlines.
func TestExtractDOCXOutlines(t *testing.T) {
	irJSON := `{"sections":[{"title":"","elements":[
		{"type":"heading","level":1,"content":[{"type":"text","text":"第一章 概述"}]},
		{"type":"paragraph","content":[{"type":"text","text":"正文"}]},
		{"type":"heading","level":2,"content":[{"type":"text","text":"1.1 背景"}]}
	]}]}`
	outlines := extractDOCXOutlines(irJSON)
	want := []docxOutline{{Title: "第一章 概述", Level: 0}, {Title: "1.1 背景", Level: 1}}
	if !reflect.DeepEqual(outlines, want) {
		t.Errorf("got %+v, want %+v", outlines, want)
	}
}

// TestRemoveTOCWord_NoOutlines delegates to removeContentsTable when
// no outlines are available (mirrors Python utils.py:263-264).
func TestRemoveTOCWord_NoOutlines(t *testing.T) {
	items := []map[string]any{
		{"text": "前言"},
		{"text": "目录"},
		{"text": "第一章 概述"},
		{"text": "正文"},
	}
	got := removeTOCWord(items, nil, false)
	want := []map[string]any{
		{"text": "前言"},
		{"text": "正文"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestRemoveTOCWord_WithOutlines deletes the TOC heading and
// following entries that match outline-title prefixes or the
// "dots + page number" regex (mirrors Python utils.py:115-144).
func TestRemoveTOCWord_WithOutlines(t *testing.T) {
	items := []map[string]any{
		{"text": "前言"},
		{"text": "目录"},
		{"text": "第一章 概述"},
		{"text": "第一章 概述 .......... 1"},
		{"text": "第二章 方法 .......... 5"},
		{"text": "正文开始"},
	}
	outlines := []docxOutline{
		{Title: "第一章 概述", Level: 0},
		{Title: "第二章 方法", Level: 0},
	}
	got := removeTOCWord(items, outlines, false)
	// "目录" heading + "第一章 概述" (outline prefix match) +
	// "第一章 概述 .......... 1" (prefix match) +
	// "第二章 方法 .......... 5" (prefix match + dot+page regex) removed.
	// "前言" and "正文开始" survive.
	want := []map[string]any{
		{"text": "前言"},
		{"text": "正文开始"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// TestRemoveTOCWord_NoTOCHeading keeps items unchanged when no TOC
// heading is found (only removeContentsTable fallback runs).
func TestRemoveTOCWord_NoTOCHeading(t *testing.T) {
	items := []map[string]any{
		{"text": "第一章 概述"},
		{"text": "正文"},
	}
	outlines := []docxOutline{{Title: "第一章 概述", Level: 0}}
	got := removeTOCWord(items, outlines, false)
	// No "目录/Contents" heading → outline-prefix loop skipped;
	// removeContentsTable fallback finds no TOC heading either.
	want := []map[string]any{
		{"text": "第一章 概述"},
		{"text": "正文"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestDocxOutline_JSONRoundtrip(t *testing.T) {
	o := docxOutline{Title: "Heading 1", Level: 2}
	b, _ := json.Marshal(o)
	var got docxOutline
	_ = json.Unmarshal(b, &got)
	if !reflect.DeepEqual(got, o) {
		t.Errorf("roundtrip got %+v, want %+v", got, o)
	}
}
