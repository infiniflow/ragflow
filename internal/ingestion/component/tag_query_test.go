//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"fmt"
	"math"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/tokenizer"
)

func TestTagFileIDFromParserConfig(t *testing.T) {
	tests := []struct {
		name         string
		parserConfig map[string]any
		want         string
	}{
		{
			name:         "nil config",
			parserConfig: nil,
			want:         "",
		},
		{
			name:         "flat tags map",
			parserConfig: map[string]any{"tags": map[string]any{"tag_file_id": "file-1"}},
			want:         "file-1",
		},
		{
			name:         "flat tags map trims",
			parserConfig: map[string]any{"tags": map[string]any{"tag_file_id": "  file-2  "}},
			want:         "file-2",
		},
		{
			name: "extractor scoped form",
			parserConfig: map[string]any{
				"Extractor:AutoExtractDefault": map[string]any{
					"tags": map[string]any{"tag_file_id": "file-3"},
				},
			},
			want: "file-3",
		},
		{
			name: "entity.JSONMap nested value",
			parserConfig: map[string]any{
				"tags": entity.JSONMap{"tag_file_id": "file-4"},
			},
			want: "file-4",
		},
		{
			name:         "no tag file id",
			parserConfig: map[string]any{"tags": map[string]any{"top_n": 3}},
			want:         "",
		},
		{
			name:         "unrelated config",
			parserConfig: map[string]any{"chunk_token_num": 128},
			want:         "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TagFileIDFromParserConfig(tt.parserConfig); got != tt.want {
				t.Fatalf("TagFileIDFromParserConfig(%v) = %q, want %q", tt.parserConfig, got, tt.want)
			}
		})
	}
}

func TestTagFileIDFromParserConfigPicksStableExtractorNode(t *testing.T) {
	// Several Extractor nodes can each declare their own tags block. Go
	// randomizes map iteration, so an unsorted scan would return a different
	// tag file - and thus tag the dataset from a different tag source - on
	// every call and every process restart.
	parserConfig := map[string]any{
		"Extractor": map[string]any{"tags": map[string]any{"tag_file_id": "file-unscoped"}},
	}
	for i := 0; i < 8; i++ {
		parserConfig[fmt.Sprintf("Extractor:node-%d", i)] = map[string]any{
			"tags": map[string]any{"tag_file_id": fmt.Sprintf("file-%d", i)},
		}
	}
	// The unscoped "Extractor" node wins: it is a prefix of the scoped keys,
	// so it sorts first.
	for i := 0; i < 200; i++ {
		if got := TagFileIDFromParserConfig(parserConfig); got != "file-unscoped" {
			t.Fatalf("TagFileIDFromParserConfig with unscoped node = %q, want %q", got, "file-unscoped")
		}
	}
	// Without it the lowest-sorting scoped node wins, still stably.
	delete(parserConfig, "Extractor")
	for i := 0; i < 200; i++ {
		if got := TagFileIDFromParserConfig(parserConfig); got != "file-0" {
			t.Fatalf("TagFileIDFromParserConfig across scoped nodes = %q, want %q", got, "file-0")
		}
	}
}

func newTagQueryTestIndex(t *testing.T) *MemoryTagIndex {
	t.Helper()
	// The infinity engine tokenizes by identity (whitespace split), so these
	// tests need no native analyzer pool.
	tokenizer.SetEngineType("infinity")
	t.Cleanup(func() { tokenizer.SetEngineType("") })

	labels := []schema.TagLabel{
		{Content: "kubernetes cluster container orchestration pod scheduling", Tags: []string{"K8s", "Container"}},
		{Content: "retrieval augmented generation vector search database", Tags: []string{"RAG", "Search"}},
	}
	idx := buildMemoryTagIndex(labels, tokenizer.New("English"))
	if idx == nil {
		t.Fatal("buildMemoryTagIndex returned nil")
	}
	return idx
}

func TestTagProportions(t *testing.T) {
	idx := newTagQueryTestIndex(t)

	got := idx.TagProportions(tagQueryProportionSmoothing)
	// Four distinct tags, each appearing once, so total = 4.
	want := (1.0 + 1) / (4.0 + tagQueryProportionSmoothing)
	for _, tag := range []string{"K8s", "Container", "RAG", "Search"} {
		if math.Abs(got[tag]-want) > 1e-12 {
			t.Fatalf("proportion[%q] = %v, want %v", tag, got[tag], want)
		}
	}
}

func TestMatchTagQueryMatchesRelevantTags(t *testing.T) {
	idx := newTagQueryTestIndex(t)

	got := idx.MatchTagQuery("kubernetes cluster orchestration", 3)
	if len(got) == 0 {
		t.Fatal("expected a non-empty tag distribution")
	}
	for _, tag := range []string{"K8s", "Container"} {
		if _, ok := got[tag]; !ok {
			t.Fatalf("expected tag %q in result, got %v", tag, got)
		}
	}
	if _, ok := got["RAG"]; ok {
		t.Fatalf("unrelated tag RAG must not be boosted, got %v", got)
	}
	for tag, weight := range got {
		if weight < 1 {
			t.Fatalf("weight[%q] = %v, want >= 1", tag, weight)
		}
	}
}

// Python rounds the raw score before flooring it at 1
// (search.py::tag_query: max(1, round(0.1*(c+1)/(cnt+S)/max(1e-6, all_tags[t])))).
// A matched tag of the index above has c=1, cnt=2 and all_tags[t]=2/1004, so
// the raw score is 0.1001996..., which Python rounds down to 0 and then floors
// at 1. Go must land on the same integral weight instead of returning the raw
// fraction.
func TestMatchTagQueryWeightMatchesPythonRounding(t *testing.T) {
	idx := newTagQueryTestIndex(t)

	got := idx.MatchTagQuery("kubernetes cluster orchestration", 3)
	for _, tag := range []string{"K8s", "Container"} {
		weight, ok := got[tag]
		if !ok {
			t.Fatalf("expected tag %q in result, got %v", tag, got)
		}
		if weight != 1.0 {
			t.Errorf("weight[%q] = %v, want 1 (Python: max(1, round(0.1001996)))", tag, weight)
		}
	}
}

// Python's round() yields an integer, so every returned weight is integral.
func TestMatchTagQueryWeightsAreIntegral(t *testing.T) {
	idx := newTagQueryTestIndex(t)

	got := idx.MatchTagQuery("kubernetes cluster container orchestration pod scheduling", 3)
	if len(got) == 0 {
		t.Fatal("expected a non-empty tag distribution")
	}
	for tag, weight := range got {
		if weight != math.Trunc(weight) {
			t.Errorf("weight[%q] = %v, want an integral value", tag, weight)
		}
	}
}

func TestMatchTagQueryUnmatchedQuestionIsEmpty(t *testing.T) {
	idx := newTagQueryTestIndex(t)

	if got := idx.MatchTagQuery("zzzz qqqq", 3); len(got) != 0 {
		t.Fatalf("expected empty result for unmatched question, got %v", got)
	}
}

func TestMatchTagQueryCapsToTopN(t *testing.T) {
	idx := newTagQueryTestIndex(t)

	got := idx.MatchTagQuery("kubernetes cluster container orchestration pod scheduling", 1)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 tag with topn=1, got %v", got)
	}
}

func TestMatchTagQueryNilIndexOrEmptyInput(t *testing.T) {
	var idx *MemoryTagIndex
	if got := idx.MatchTagQuery("anything", 3); len(got) != 0 {
		t.Fatalf("nil index should yield empty result, got %v", got)
	}

	idx = newTagQueryTestIndex(t)
	if got := idx.MatchTagQuery("", 3); len(got) != 0 {
		t.Fatalf("empty question should yield empty result, got %v", got)
	}
	if got := idx.MatchTagQuery("kubernetes", 0); len(got) != 0 {
		t.Fatalf("topn=0 should yield empty result, got %v", got)
	}
}
