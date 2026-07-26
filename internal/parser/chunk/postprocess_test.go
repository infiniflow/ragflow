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
	}
}

func TestNewPostprocessOperatorParsesFilterFlags(t *testing.T) {
	op, err := NewPostprocessOperator(map[string]interface{}{
		"filter": map[string]interface{}{
			"min_length":      float64(3),
			"drop_empty":      true,
			"drop_duplicates": true,
		},
	})
	if err != nil {
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
