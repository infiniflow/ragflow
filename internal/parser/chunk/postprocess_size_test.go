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

package chunk

import (
	"reflect"
	"testing"
)

func TestPostprocessSizeCountsRunes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		want    int
	}{
		{"ASCII", "plain", 5},
		{"CJK", "中文测试", 4},
		{"mixed_code_points", "a中\U00020000e\u0301", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			op, err := NewPostprocessOperator(nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &ChunkContext{SplitChunks: []ChunkData{{Content: tc.content}}}
			if err := op.Execute(ctx); err != nil {
				t.Fatal(err)
			}
			if len(ctx.ResultChunks) != 1 {
				t.Fatalf("got %d chunks, want 1", len(ctx.ResultChunks))
			}
			got := ctx.ResultChunks[0]
			if got.Content != tc.content {
				t.Errorf("content = %q, want %q", got.Content, tc.content)
			}
			if got.Size != tc.want {
				t.Errorf("Size = %d, want %d runes (content bytes = %d)", got.Size, tc.want, len(got.Content))
			}
		})
	}
}

func TestPostprocessPreservesLengthSplitRuneSizes(t *testing.T) {
	chunks := runLength(t, "甲乙丙丁戊", 2, 0)
	want := append([]ChunkData(nil), chunks...)
	op, err := NewPostprocessOperator(map[string]interface{}{
		"filter": map[string]interface{}{"min_length": float64(1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &ChunkContext{SplitChunks: chunks}
	if err := op.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ctx.ResultChunks, want) {
		t.Errorf("postprocess changed valid length-split chunks:\ngot  %#v\nwant %#v", ctx.ResultChunks, want)
	}
}

func TestRunSizeCountsRunes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		opts    ChunkOptions
		content []string
		sizes   []int
	}{
		{
			name: "CJK_merge", text: "甲乙\n丙丁",
			opts:    ChunkOptions{SplitStrategy: "paragraph", MergeTargetSize: 8},
			content: []string{"甲乙 丙丁"}, sizes: []int{5},
		},
		{
			name: "ASCII_merge", text: "a\nbc",
			opts:    ChunkOptions{SplitStrategy: "paragraph", MergeTargetSize: 8},
			content: []string{"a bc"}, sizes: []int{4},
		},
		{
			name: "CJK_filter", text: "中\n中文",
			opts:    ChunkOptions{SplitStrategy: "paragraph", FilterMinLength: 2},
			content: []string{"中文"}, sizes: []int{2},
		},
		{
			name: "CJK_oversize_units", text: "甲乙丙丁\n戊己庚辛",
			opts:    ChunkOptions{SplitStrategy: "paragraph", MergeTargetSize: 4},
			content: []string{"甲乙丙丁", "戊己庚辛"}, sizes: []int{4, 4},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := Run(tc.text, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(ctx.ResultChunks) != len(tc.content) {
				t.Fatalf("got %d chunks, want %d", len(ctx.ResultChunks), len(tc.content))
			}
			for i, got := range ctx.ResultChunks {
				if got.Content != tc.content[i] || got.Index != i {
					t.Errorf("chunk[%d] = %+v, want content %q and index %d", i, got, tc.content[i], i)
				}
				if got.Size != tc.sizes[i] {
					t.Errorf("chunk[%d] Size = %d, want %d runes", i, got.Size, tc.sizes[i])
				}
			}
		})
	}
}
