package parser

import (
	"reflect"
	"testing"
)

// TestRemoveContentsTable mirrors the Python remove_contents_table
// (rag/nlp/__init__.py:937-965). It locates a TOC heading
// (contents/目录/目次/...), drops it, then drops the following
// entries that share a common prefix with the first TOC entry.
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
			got := removeContentsTable(tc.items, tc.eng)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
