package parser

import (
	"encoding/json"
	"reflect"
	"testing"
)

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
