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

package chunk

import (
<<<<<<< HEAD
	"reflect"
	"testing"
)

func TestPostprocessFilterChunks(t *testing.T) {
	tests := []struct {
		name   string
		filter filterConfig
		input  []ChunkData
		want   []string
	}{
		{
			name:   "keeps current length bound behavior",
			filter: filterConfig{MinLength: 3, MaxLength: 5},
			input: []ChunkData{
				{Content: "go"},
				{Content: "rust"},
				{Content: "python"},
				{Content: "数据库"},
			},
			want: []string{"rust", "数据库"},
		},
		{
			name:   "drops empty and whitespace only chunks",
			filter: filterConfig{DropEmpty: true},
			input: []ChunkData{
				{Content: ""},
				{Content: "   "},
				{Content: "\n\t"},
				{Content: "content"},
			},
			want: []string{"content"},
		},
		{
			name:   "drops exact duplicate chunks and keeps first occurrence",
			filter: filterConfig{DropDuplicates: true},
			input: []ChunkData{
				{Content: "alpha"},
				{Content: "beta"},
				{Content: "alpha"},
				{Content: " alpha "},
				{Content: "beta"},
			},
			want: []string{"alpha", "beta", " alpha "},
		},
		{
			name:   "combines empty duplicate and length filters",
			filter: filterConfig{MinLength: 3, MaxLength: 6, DropEmpty: true, DropDuplicates: true},
			input: []ChunkData{
				{Content: "  "},
				{Content: "go"},
				{Content: "alpha"},
				{Content: "alpha"},
				{Content: "charlie"},
				{Content: "beta"},
			},
			want: []string{"alpha", "beta"},
		},
		{
			name:   "defaults preserve empty and duplicate chunks",
			filter: filterConfig{},
			input: []ChunkData{
				{Content: ""},
				{Content: "same"},
				{Content: "same"},
			},
			want: []string{"", "same", "same"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &PostprocessOperator{filter: &tt.filter}
			gotChunks := op.filterChunks(tt.input)
			got := contents(gotChunks)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("filterChunks() = %#v, want %#v", got, tt.want)
			}
		})
=======
	"strings"
	"testing"
)

// runFilter builds a postprocess operator whose only stage is the filter, runs
// it over the given chunk contents, and returns the resulting contents in order.
func runFilter(t *testing.T, filter map[string]interface{}, contents ...string) []string {
	t.Helper()
	op, err := NewPostprocessOperator(map[string]interface{}{"filter": filter})
	if err != nil {
		t.Fatalf("NewPostprocessOperator returned error: %v", err)
	}
	ctx := &ChunkContext{SplitChunks: make([]ChunkData, len(contents))}
	for i, c := range contents {
		ctx.SplitChunks[i] = ChunkData{Content: c, Index: i}
	}
	if err := op.Execute(ctx); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	out := make([]string, len(ctx.ResultChunks))
	for i, c := range ctx.ResultChunks {
		out[i] = c.Content
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestFilterLengthBounds(t *testing.T) {
	got := runFilter(t, map[string]interface{}{
		"min_length": float64(2),
	}, "a", "bb", "cccc", "ddddd")
	want := []string{"bb", "cccc", "ddddd"}
	if !eq(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterDropEmpty(t *testing.T) {
	got := runFilter(t, map[string]interface{}{
		"drop_empty": true,
	}, "keep", "   ", "", "\n\t", "also")
	want := []string{"keep", "also"}
	if !eq(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterDropDuplicatesPreservesFirst(t *testing.T) {
	got := runFilter(t, map[string]interface{}{
		"drop_duplicates": true,
	}, "a", "b", "a", "c", "b", "a")
	want := []string{"a", "b", "c"}
	if !eq(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterDropEmptyAndDuplicatesCombined(t *testing.T) {
	got := runFilter(t, map[string]interface{}{
		"drop_empty":      true,
		"drop_duplicates": true,
	}, "x", "  ", "x", "y", "")
	want := []string{"x", "y"}
	if !eq(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterDuplicatesAreContentExact(t *testing.T) {
	// Differing whitespace => distinct content => both kept.
	got := runFilter(t, map[string]interface{}{
		"drop_duplicates": true,
	}, "a", "a ", "a")
	want := []string{"a", "a "}
	if !eq(got, want) {
		t.Fatalf("got %v, want %v", got, want)
>>>>>>> upstream/main
	}
}

func TestNewPostprocessOperatorParsesFilterFlags(t *testing.T) {
	op, err := NewPostprocessOperator(map[string]interface{}{
		"filter": map[string]interface{}{
			"min_length":      float64(3),
<<<<<<< HEAD
			"max_length":      float64(10),
=======
>>>>>>> upstream/main
			"drop_empty":      true,
			"drop_duplicates": true,
		},
	})
	if err != nil {
<<<<<<< HEAD
		t.Fatalf("NewPostprocessOperator() unexpected error: %v", err)
	}
	if op.filter == nil {
		t.Fatal("NewPostprocessOperator() did not configure filter")
	}
	if op.filter.MinLength != 3 || op.filter.MaxLength != 10 || !op.filter.DropEmpty || !op.filter.DropDuplicates {
		t.Fatalf("filter config = %#v", *op.filter)
	}
}

func contents(chunks []ChunkData) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, c.Content)
	}
	return out
}
=======
		t.Fatalf("unexpected error: %v", err)
	}
	if op.filter == nil {
		t.Fatal("filter config not parsed")
	}
	if op.filter.MinLength != 3 || !op.filter.DropEmpty || !op.filter.DropDuplicates {
		t.Errorf("parsed filter = %+v, want min_length=3 drop_empty=true drop_duplicates=true", *op.filter)
	}
	// The new flags must surface in String() for plan explainability.
	s := op.String()
	if !strings.Contains(s, "drop_empty: true") || !strings.Contains(s, "drop_duplicates: true") {
		t.Errorf("String() missing filter flags:\n%s", s)
	}
}
>>>>>>> upstream/main
