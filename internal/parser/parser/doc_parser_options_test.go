package parser

import (
	"archive/zip"
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestDOCParser_ConfigureFromSetup(t *testing.T) {
	p := NewDOCParser()
	p.ConfigureFromSetup(map[string]any{
		"remove_toc":           true,
		"remove_header_footer": true,
	})
	if !p.RemoveTOC {
		t.Error("RemoveTOC = false, want true")
	}
	if !p.RemoveHeaderFooter {
		t.Error("RemoveHeaderFooter = false, want true")
	}
}

func TestDOCParser_ConfigureFromSetup_NilSafe(t *testing.T) {
	p := NewDOCParser()
	p.ConfigureFromSetup(nil)
	if p.RemoveTOC || p.RemoveHeaderFooter {
		t.Error("expected default false values on nil setup")
	}

	var nilParser *DOCParser
	nilParser.ConfigureFromSetup(map[string]any{"remove_toc": true}) // should not panic
}

func TestDOCParser_RemoveTOC_StructuredIR_WithOutlines(t *testing.T) {
	irJSON := `{"sections":[{"title":"","elements":[
		{"type":"heading","level":1,"content":[{"type":"text","text":"前言"}]},
		{"type":"paragraph","content":[{"type":"text","text":"前言内容。"}]},
		{"type":"heading","level":1,"content":[{"type":"text","text":"目录"}]},
		{"type":"paragraph","content":[{"type":"text","text":"第一章 概述 .......... 1"}]},
		{"type":"paragraph","content":[{"type":"text","text":"第二章 架构 .......... 5"}]},
		{"type":"paragraph","content":[{"type":"text","text":"正文开始"}]},
		{"type":"heading","level":1,"content":[{"type":"text","text":"第一章 概述"}]},
		{"type":"paragraph","content":[{"type":"text","text":"第一章正文内容。"}]},
		{"type":"heading","level":1,"content":[{"type":"text","text":"第二章 架构"}]},
		{"type":"paragraph","content":[{"type":"text","text":"第二章正文内容。"}]}
	]}]}`

	orig := docExtract
	defer func() { docExtract = orig }()
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	ctx := context.Background()

	// 1. With RemoveTOC = false, all 10 sections are retained.
	pDisabled := NewDOCParser()
	resDisabled := pDisabled.ParseWithResult(ctx, "test.doc", []byte("dummy"))
	if resDisabled.Err != nil {
		t.Fatalf("ParseWithResult disabled error: %v", resDisabled.Err)
	}
	if len(resDisabled.JSON) != 10 {
		t.Fatalf("len(resDisabled.JSON) = %d, want 10", len(resDisabled.JSON))
	}

	// 2. With RemoveTOC = true, TOC heading and items are filtered out.
	pEnabled := NewDOCParser()
	pEnabled.ConfigureFromSetup(map[string]any{"remove_toc": true})
	resEnabled := pEnabled.ParseWithResult(ctx, "test.doc", []byte("dummy"))
	if resEnabled.Err != nil {
		t.Fatalf("ParseWithResult enabled error: %v", resEnabled.Err)
	}

	// "目录", "第一章 概述 .......... 1", "第二章 架构 .......... 5" are removed.
	// Remaining 7 items should survive.
	if len(resEnabled.JSON) != 7 {
		t.Fatalf("len(resEnabled.JSON) = %d, want 7", len(resEnabled.JSON))
	}
	for _, item := range resEnabled.JSON {
		text := itemText(item.ToMap())
		if text == "目录" || strings.Contains(text, "..........") {
			t.Errorf("TOC item %q was not removed", text)
		}
	}
}

func TestDOCParser_RemoveTOC_StructuredIR_NoOutlinesDelegatesToContentsTable(t *testing.T) {
	// IR without headings: all paragraphs, but contains TOC structure.
	irJSON := `{"sections":[{"title":"","elements":[
		{"type":"paragraph","content":[{"type":"text","text":"前言"}]},
		{"type":"paragraph","content":[{"type":"text","text":"目录"}]},
		{"type":"paragraph","content":[{"type":"text","text":"第一节 概述"}]},
		{"type":"paragraph","content":[{"type":"text","text":"杂项"}]},
		{"type":"paragraph","content":[{"type":"text","text":"第一节 背景"}]},
		{"type":"paragraph","content":[{"type":"text","text":"正文"}]}
	]}]}`

	orig := docExtract
	defer func() { docExtract = orig }()
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	ctx := context.Background()

	pEnabled := NewDOCParser()
	pEnabled.ConfigureFromSetup(map[string]any{"remove_toc": true})
	resEnabled := pEnabled.ParseWithResult(ctx, "no_outlines.doc", []byte("dummy"))
	if resEnabled.Err != nil {
		t.Fatalf("ParseWithResult enabled error: %v", resEnabled.Err)
	}

	// "目录", "第一节 概述", and "杂项" are removed; "前言", "第一节 背景", and "正文" survive.
	if len(resEnabled.JSON) != 3 {
		t.Fatalf("len(resEnabled.JSON) = %d, want 3", len(resEnabled.JSON))
	}
	want := []string{"前言", "第一节 背景", "正文"}
	for i, w := range want {
		if got := itemText(resEnabled.JSON[i].ToMap()); got != w {
			t.Errorf("item %d = %q, want %q", i, got, w)
		}
	}
}

func TestDOCParser_RemoveHeaderFooter_DoesNotError(t *testing.T) {
	irJSON := `{"sections":[{"title":"","elements":[
		{"type":"paragraph","content":[{"type":"text","text":"Some doc text"}]}
	]}]}`

	orig := docExtract
	defer func() { docExtract = orig }()
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	ctx := context.Background()

	p := NewDOCParser()
	p.ConfigureFromSetup(map[string]any{"remove_header_footer": true})
	res := p.ParseWithResult(ctx, "header_footer.doc", []byte("dummy binary doc data"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(res.JSON) = %d, want 1", len(res.JSON))
	}
}

func TestDOCParser_HeaderFooter_PositionalOrderAndRemoval(t *testing.T) {
	irJSON := `{"sections":[{"title":"","first_page_header":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"First Header"}]}
	]},"header":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"DOC-123"}]}
	]},"even_page_header":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"Even Header"}]}
	]},"elements":[
		{"type":"paragraph","content":[{"type":"text","text":"Body text"}]},
		{"type":"paragraph","content":[{"type":"text","text":"DOC-123"}]},
		{"type":"paragraph","content":[{"type":"text","text":"Approval"}]}
	],"first_page_footer":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"First Footer"}]}
	]},"footer":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"Approval"}]}
	]},"even_page_footer":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"Even Footer"}]}
	]}}]}`

	orig := docExtract
	defer func() { docExtract = orig }()
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}

	itemTexts := func(items []DocItem) []string {
		out := make([]string, len(items))
		for i, item := range items {
			out[i] = itemText(item.ToMap())
		}
		return out
	}

	ctx := context.Background()

	// Default parse emits all six header/footer slots around the section body.
	pDefault := NewDOCParser()
	resDefault := pDefault.ParseWithResult(ctx, "report.doc", []byte("dummy ole2 bytes"))
	if resDefault.Err != nil {
		t.Fatalf("default ParseWithResult error: %v", resDefault.Err)
	}
	wantDefault := []string{
		"First Header", "DOC-123", "Even Header",
		"Body text", "DOC-123", "Approval",
		"First Footer", "Approval", "Even Footer",
	}
	if got := itemTexts(resDefault.JSON); !reflect.DeepEqual(got, wantDefault) {
		t.Errorf("default texts = %#v, want %#v", got, wantDefault)
	}

	// remove_header_footer=true skips header/footer slots by position while
	// keeping body paragraphs whose text matches header/footer lines.
	pRemoved := NewDOCParser()
	pRemoved.ConfigureFromSetup(map[string]any{"remove_header_footer": true})
	resRemoved := pRemoved.ParseWithResult(ctx, "report.doc", []byte("dummy ole2 bytes"))
	if resRemoved.Err != nil {
		t.Fatalf("remove_header_footer ParseWithResult error: %v", resRemoved.Err)
	}
	wantBody := []string{"Body text", "DOC-123", "Approval"}
	if got := itemTexts(resRemoved.JSON); !reflect.DeepEqual(got, wantBody) {
		t.Errorf("removed texts = %#v, want %#v", got, wantBody)
	}

	// A .docx renamed to .doc must also skip by position instead of text-matching
	// against word/header*.xml and word/footer*.xml.
	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)
	for name, body := range map[string]string{
		"word/header1.xml": `<?xml version="1.0"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>DOC-123</w:t></w:r></w:p></w:hdr>`,
		"word/footer1.xml": `<?xml version="1.0"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:r><w:t>Approval</w:t></w:r></w:p></w:ftr>`,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	resRenamed := pRemoved.ParseWithResult(ctx, "renamed.doc", buf.Bytes())
	if resRenamed.Err != nil {
		t.Fatalf("renamed .docx ParseWithResult error: %v", resRenamed.Err)
	}
	if got := itemTexts(resRenamed.JSON); !reflect.DeepEqual(got, wantBody) {
		t.Errorf("renamed .docx texts = %#v, want %#v", got, wantBody)
	}
}

func TestDOCParser_HeaderFooterOnlyIR(t *testing.T) {
	irJSON := `{"sections":[{"title":"","header":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"DOC-123"}]}
	]},"elements":[],"footer":{"content":[
		{"type":"paragraph","content":[{"type":"text","text":"Page 1"}]}
	]}}]}`

	orig := docExtract
	defer func() { docExtract = orig }()

	ctx := context.Background()

	// Default parse emits header/footer content even when the body and plain-text views are empty.
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return irJSON, "", "", nil, nil
	}
	resDefault := NewDOCParser().ParseWithResult(ctx, "hf_only.doc", []byte("dummy"))
	if resDefault.Err != nil {
		t.Fatalf("default ParseWithResult error: %v", resDefault.Err)
	}
	want := []string{"DOC-123", "Page 1"}
	got := make([]string, len(resDefault.JSON))
	for i, item := range resDefault.JSON {
		got[i] = itemText(item.ToMap())
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("default texts = %#v, want %#v", got, want)
	}

	// remove_header_footer=true must not fall back to plainText and re-emit removed header/footer text.
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return irJSON, "", "DOC-123\nPage 1", nil, nil
	}
	pRemoved := NewDOCParser()
	pRemoved.ConfigureFromSetup(map[string]any{"remove_header_footer": true})
	resRemoved := pRemoved.ParseWithResult(ctx, "hf_only.doc", []byte("dummy"))
	if resRemoved.Err != nil {
		t.Fatalf("remove_header_footer ParseWithResult error: %v", resRemoved.Err)
	}
	if len(resRemoved.JSON) != 1 || itemText(resRemoved.JSON[0].ToMap()) != "" {
		t.Errorf("remove_header_footer items = %+v, want single empty text item", resRemoved.JSON)
	}
}

func TestDOCParser_RemoveTOC_PlainTextFallback(t *testing.T) {
	// When irJSON is empty, the parser falls back to plainText / mdText.
	rawText := strings.Join([]string{
		"前言内容",
		"目录",
		"第一节 概述",
		"第二章 核心实现",
	}, "\n")

	orig := docExtract
	defer func() { docExtract = orig }()
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return "", "", rawText, nil, nil
	}

	ctx := context.Background()

	// 1. With RemoveTOC = false, raw text is returned verbatim.
	pDisabled := NewDOCParser()
	resDisabled := pDisabled.ParseWithResult(ctx, "fallback.doc", []byte("dummy"))
	if resDisabled.Err != nil {
		t.Fatalf("ParseWithResult disabled error: %v", resDisabled.Err)
	}
	if len(resDisabled.JSON) != 1 {
		t.Fatalf("len(resDisabled.JSON) = %d, want 1", len(resDisabled.JSON))
	}
	if got := itemText(resDisabled.JSON[0].ToMap()); got != rawText {
		t.Errorf("got %q, want %q", got, rawText)
	}

	// 2. With RemoveTOC = true, removeContentsTable filters out TOC lines.
	pEnabled := NewDOCParser()
	pEnabled.ConfigureFromSetup(map[string]any{"remove_toc": true})
	resEnabled := pEnabled.ParseWithResult(ctx, "fallback.doc", []byte("dummy"))
	if resEnabled.Err != nil {
		t.Fatalf("ParseWithResult enabled error: %v", resEnabled.Err)
	}
	if len(resEnabled.JSON) != 1 {
		t.Fatalf("len(resEnabled.JSON) = %d, want 1", len(resEnabled.JSON))
	}
	gotText := itemText(resEnabled.JSON[0].ToMap())
	if strings.Contains(gotText, "目录") || strings.Contains(gotText, "第一节 概述") {
		t.Errorf("TOC lines were not removed in fallback text: %q", gotText)
	}
	if !strings.Contains(gotText, "前言内容") || !strings.Contains(gotText, "第二章 核心实现") {
		t.Errorf("content was unexpectedly dropped: %q", gotText)
	}
}

func TestDOCParser_RemoveTOC_PreservesFormatting(t *testing.T) {
	// Ensure that when RemoveTOC is enabled, blank lines and indentation
	// are preserved even when no TOC heading is present.
	rawText := "Section 1\n\n   Indented paragraph.\n\nSection 2"

	orig := docExtract
	defer func() { docExtract = orig }()
	docExtract = func(data []byte, format string) (string, string, string, error, error) {
		return "", "", rawText, nil, nil
	}

	ctx := context.Background()
	p := NewDOCParser()
	p.ConfigureFromSetup(map[string]any{"remove_toc": true})
	res := p.ParseWithResult(ctx, "formatting.doc", []byte("dummy"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult error: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(res.JSON) = %d, want 1", len(res.JSON))
	}
	itemMap := res.JSON[0].ToMap()
	gotText, ok := itemMap["text"].(string)
	if !ok {
		t.Fatalf("res.JSON[0].text is not a string: %v", itemMap["text"])
	}
	if gotText != rawText {
		t.Errorf("formatting lost:\ngot:  %q\nwant: %q", gotText, rawText)
	}
}
