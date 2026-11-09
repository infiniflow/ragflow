package parser

import (
	"reflect"
	"testing"
)

// TestRemoveContentsTable covers the plain-text rules that HTML, DOC and DOCX
// share. It locates a TOC heading (contents/目录/目次/...), drops it, then drops
// the following entries that share a common prefix with the first TOC entry.
func TestRemoveContentsTable(t *testing.T) {
	cases := []struct {
		name  string
		items []map[string]any
		eng   bool
		want  []map[string]any
	}{
		{
			name:  "no TOC heading → unchanged",
			items: []map[string]any{{"text": "Intro"}, {"text": "Body"}},
			eng:   false,
			want:  []map[string]any{{"text": "Intro"}, {"text": "Body"}},
		},
		{
			name: "Chinese 目录 heading + first TOC entry removed",
			items: []map[string]any{
				{"text": "前言"},
				{"text": "目录"},
				{"text": "第一章 概述"},
				{"text": "第二章 方法"},
				{"text": "正文开始"},
			},
			eng: false,
			// Python remove_contents_table drops the heading and the
			// first entry after it (prefix "第一章"). Subsequent
			// entries with a different prefix ("第二章") are kept
			// because re.match("第一章", "第二章 方法") fails.
			want: []map[string]any{
				{"text": "前言"},
				{"text": "第二章 方法"},
				{"text": "正文开始"},
			},
		},
		{
			name: "multi-word Table of Contents heading + first TOC entry removed",
			items: []map[string]any{
				{"text": "Intro"},
				{"text": "Table of  Contents"},
				{"text": "Chapter 1 Overview"},
				{"text": "Chapter 2 Method"},
				{"text": "Body starts"},
			},
			eng: true,
			want: []map[string]any{
				{"text": "Intro"},
				{"text": "Chapter 2 Method"},
				{"text": "Body starts"},
			},
		},
		{
			name: "English Contents heading + first TOC entry removed",
			items: []map[string]any{
				{"text": "Intro"},
				{"text": "Contents"},
				{"text": "Chapter 1 Overview"},
				{"text": "Chapter 2 Method"},
				{"text": "Body starts"},
			},
			eng: true,
			want: []map[string]any{
				{"text": "Intro"},
				{"text": "Chapter 2 Method"},
				{"text": "Body starts"},
			},
		},
		{
			name: "目录 heading with no following entry → heading dropped only",
			items: []map[string]any{
				{"text": "前言"},
				{"text": "目录"},
			},
			eng: false,
			want: []map[string]any{
				{"text": "前言"},
			},
		},
		{
			name: "prefix-matching subsequent entries collapse non-matching gap",
			items: []map[string]any{
				{"text": "前言"},
				{"text": "目录"},
				{"text": "第一节 概述"},
				{"text": "杂项"},
				{"text": "第一节 背景"},
				{"text": "正文"},
			},
			eng: false,
			// prefix="第一节"; after dropping heading + first entry,
			// scan finds "第一节 背景" at j=2 (gap "杂项" at j=1);
			// Python deletes [i, j) = the gap, keeps the match.
			want: []map[string]any{
				{"text": "前言"},
				{"text": "第一节 背景"},
				{"text": "正文"},
			},
		},
		{
			name:  "empty input → unchanged",
			items: nil,
			eng:   false,
			want:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := removeContentsTable(tc.items, tc.eng, plainTOCRules)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMarkdownTOCText(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"1. Introduction", "Introduction"},
		{"2) Setup", "Setup"},
		{"10. Deep Dive", "Deep Dive"},
		{"* Chapter One", "Chapter One"},
		{"- Chapter Two", "Chapter Two"},
		{"+ Chapter Three", "Chapter Three"},
		{"# Introduction", "Introduction"},
		{"### 1. Introduction", "Introduction"},
		{"* 1. Nested", "Nested"},
		{"Plain text", "Plain text"},
		{"  #  *  Spaced  ", "Spaced"},
	}
	for _, tc := range cases {
		if got := markdownTOCText(tc.in); got != tc.want {
			t.Errorf("markdownTOCText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestIsEnglishTexts classifies text as English when >80% of sampled segments
// are ASCII.
func TestIsEnglishTexts(t *testing.T) {
	cases := []struct {
		name  string
		texts []string
		want  bool
	}{
		{name: "empty", texts: nil, want: false},
		{name: "all english", texts: []string{"Chapter One", "Section Two", "Body text here"}, want: true},
		{name: "all chinese", texts: []string{"第一章 概述", "第二章 方法", "正文内容"}, want: false},
		{name: "mostly english (>80%)", texts: []string{
			"Introduction to the system", "Background and related work",
			"Methodology details", "Evaluation results", "Conclusion",
			"第一章", // single CJK line among 6 (>80% english)
		}, want: true},
		{name: "exactly 80% english", texts: []string{
			"Introduction", "Background", "Method", "Conclusion", "第一章",
		}, want: false}, // 4/5 == 80% is NOT > 80%
		{name: "mostly chinese (<80% english)", texts: []string{
			"第一章 概述", "第二章 方法", "第三章 结果", "English abstract only",
		}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEnglishTexts(tc.texts, ""); got != tc.want {
				t.Errorf("isEnglishTexts(%v) = %v, want %v", tc.texts, got, tc.want)
			}
		})
	}
}

// TestRemoveTOCWordEnglishDetection verifies English content uses the 2-word
// TOC prefix, so "Chapter 2 Method" is NOT over-deleted (a 3-character prefix
// would have dropped it).
func TestRemoveTOCWordEnglishDetection(t *testing.T) {
	items := []map[string]any{
		{"text": "Intro"},
		{"text": "Contents"},
		{"text": "Chapter 1 Overview"},
		{"text": "Chapter 2 Method"},
		{"text": "Body starts"},
	}
	if !isEnglishItems(items, "") {
		t.Fatalf("expected isEnglishItems=true for English content")
	}
	got := removeContentsTable(items, isEnglishItems(items, ""), plainTOCRules)
	want := []map[string]any{
		{"text": "Intro"},
		{"text": "Chapter 2 Method"},
		{"text": "Body starts"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("English TOC detection: got %v, want %v", got, want)
	}
}

// TestMarkdownTOCRules pins the Markdown-specific matching: heading and list
// markers are stripped before matching, while standalone front/back-matter
// headings stay.
func TestMarkdownTOCRules(t *testing.T) {
	cases := []struct {
		name  string
		items []map[string]any
		eng   bool
		want  []map[string]any
	}{
		{
			name: "heading and list markers stripped",
			items: []map[string]any{
				{"text": "# 目录"},
				{"text": "* 第一章 概述"},
				{"text": "* 第二章 架构"},
				{"text": "# 第一章 概述"},
				{"text": "正文"},
			},
			eng: false,
			want: []map[string]any{
				{"text": "# 第一章 概述"},
				{"text": "正文"},
			},
		},
		{
			name: "acknowledgments heading stays",
			items: []map[string]any{
				{"text": "# 致谢"},
				{"text": "感谢各位的支持与帮助。"},
				{"text": "# Acknowledgments"},
				{"text": "We thank all contributors."},
			},
			eng: false,
			want: []map[string]any{
				{"text": "# 致谢"},
				{"text": "感谢各位的支持与帮助。"},
				{"text": "# Acknowledgments"},
				{"text": "We thank all contributors."},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := removeContentsTable(tc.items, tc.eng, markdownTOCRules)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPlainTOCRulesKeepMarkup pins that the HTML/DOC/DOCX contract is unchanged:
// markup markers are part of the text there, so a marked heading is not a TOC.
func TestPlainTOCRulesKeepMarkup(t *testing.T) {
	items := []map[string]any{
		{"text": "# 目录"},
		{"text": "* 第一章 概述"},
		{"text": "正文"},
	}
	got := removeContentsTable(items, false, plainTOCRules)
	if !reflect.DeepEqual(got, items) {
		t.Errorf("got %v, want unchanged %v", got, items)
	}
}

// TestRemoveContentsTableKeepsEntriesWithoutAnchor pins the conservative
// fallback: when no later item shares the first TOC entry's prefix, the
// remaining entries are kept rather than deleting body text to reach one.
func TestRemoveContentsTableKeepsEntriesWithoutAnchor(t *testing.T) {
	items := []map[string]any{
		{"text": "# Table of Contents"},
		{"text": "* Chapter 1 Intro"},
		{"text": "* Chapter 2 Deep Dive"},
		{"text": "# Introduction"},
		{"text": "Body."},
	}
	got := removeContentsTable(items, true, markdownTOCRules)
	want := []map[string]any{
		{"text": "* Chapter 2 Deep Dive"},
		{"text": "# Introduction"},
		{"text": "Body."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
