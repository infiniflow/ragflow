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
	}
}

func TestNewPostprocessOperatorParsesFilterFlags(t *testing.T) {
	op, err := NewPostprocessOperator(map[string]interface{}{
		"filter": map[string]interface{}{
			"min_length":      float64(3),
			"max_length":      float64(10),
			"drop_empty":      true,
			"drop_duplicates": true,
		},
	})
	if err != nil {
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
