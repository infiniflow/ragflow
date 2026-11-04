package parser

import (
	"context"
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
