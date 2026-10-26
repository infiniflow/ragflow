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
	"strings"
	"testing"

	"ragflow/internal/agent/component/prompts"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/tokenizer"
)

func TestLimitCitationSources_StopsAtBudget(t *testing.T) {
	sources := []prompts.CitationSource{
		{ID: "a", Content: strings.Repeat("alpha ", 20)},
		{ID: "b", Content: strings.Repeat("beta ", 20)},
		{ID: "c", Content: strings.Repeat("gamma ", 20)},
	}
	first := tokenizer.NumTokensFromString(sources[0].ID) + tokenizer.NumTokensFromString(sources[0].Content)
	second := tokenizer.NumTokensFromString(sources[1].ID) + tokenizer.NumTokensFromString(sources[1].Content)

	kept := limitCitationSources(sources, first+second-1)
	if len(kept) != 1 || kept[0].ID != "a" {
		t.Fatalf("kept = %#v, want only the first source", kept)
	}
	if all := limitCitationSources(sources, first+second+1); len(all) != 2 {
		t.Fatalf("kept = %d, want 2", len(all))
	}
}

func TestLimitCitationSources_KeepsFirstOversizedSource(t *testing.T) {
	sources := []prompts.CitationSource{{ID: "a", Content: strings.Repeat("x", 50000)}}
	if kept := limitCitationSources(sources, 1); len(kept) != 1 {
		t.Fatalf("kept = %d, want the first oversized source", len(kept))
	}
}

func TestLimitCitationSources_SkipsEmpty(t *testing.T) {
	sources := []prompts.CitationSource{
		{ID: "", Content: "no id"},
		{ID: "empty-content", Content: "   "},
		{ID: "ok", Content: "body"},
	}
	kept := limitCitationSources(sources, 30000)
	if len(kept) != 1 || kept[0].ID != "ok" {
		t.Fatalf("kept = %#v, want only the valid source", kept)
	}
}

func TestFilterCitationMarkers(t *testing.T) {
	cases := []struct {
		name    string
		content string
		allowed []string
		want    string
	}{
		{"keeps allowed", "answer [ID:good] more", []string{"good"}, "answer [ID:good] more"},
		{"drops unknown", "a [ID:good] b [ID:bad] c", []string{"good"}, "a [ID:good] b  c"},
		{"normalizes whitespace", "[ID: good ]", []string{"good"}, "[ID:good]"},
		{"drops empty id", "[ID:]", []string{"good"}, ""},
		{"drops all when no sources", "[ID:good]", nil, ""},
		{"leaves plain brackets", "see [note]", []string{"good"}, "see [note]"},
		{"leaves an unclosed marker before a markdown link", "answer [ID:oops more [link] end", []string{"good"}, "answer [ID:oops more [link] end"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := filterCitationMarkers(tc.content, tc.allowed); got != tc.want {
				t.Fatalf("filterCitationMarkers(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

func TestCitationMarkerStreamFilter_SplitMarkers(t *testing.T) {
	filter := newCitationMarkerStreamFilter([]string{"abc123"})
	var out strings.Builder
	for _, delta := range []string{"answer [ID:a", "bc", "123] done"} {
		out.WriteString(filter.Push(delta))
	}
	out.WriteString(filter.Flush())
	if got, want := out.String(), "answer [ID:abc123] done"; got != want {
		t.Fatalf("streamed = %q, want %q", got, want)
	}
}

func TestCitationMarkerStreamFilter_DropsSplitInvalidMarker(t *testing.T) {
	filter := newCitationMarkerStreamFilter([]string{"good"})
	var out strings.Builder
	for _, delta := range []string{"keep [ID:ba", "d] text"} {
		out.WriteString(filter.Push(delta))
	}
	out.WriteString(filter.Flush())
	if got, want := out.String(), "keep  text"; got != want {
		t.Fatalf("streamed = %q, want %q", got, want)
	}
}

func TestCitationMarkerStreamFilter_FlushRules(t *testing.T) {
	filter := newCitationMarkerStreamFilter([]string{"good"})
	if got := filter.Push("see ["); got != "see " {
		t.Fatalf("Push held the partial bracket, got %q", got)
	}
	if got := filter.Flush(); got != "[" {
		t.Fatalf("Flush = %q, want the literal bracket", got)
	}

	unclosed := newCitationMarkerStreamFilter([]string{"good"})
	if got := unclosed.Push("answer [ID:good"); got != "answer " {
		t.Fatalf("Push held the unclosed marker, got %q", got)
	}
	if got := unclosed.Flush(); got != "[ID:good" {
		t.Fatalf("Flush = %q, want the unclosed marker left literal", got)
	}
}

func TestCitationMarkerStreamFilter_KeepsTextAfterUnclosedMarker(t *testing.T) {
	filter := newCitationMarkerStreamFilter([]string{"good"})
	var out strings.Builder
	for _, delta := range []string{"answer ", "[ID:good", " more text"} {
		out.WriteString(filter.Push(delta))
	}
	out.WriteString(filter.Flush())

	content := "answer [ID:good more text"
	want := filterCitationMarkers(content, []string{"good"})
	if got := out.String(); got != want {
		t.Fatalf("streamed = %q, want %q", got, want)
	}
	if !strings.Contains(out.String(), "more text") {
		t.Fatalf("streamed = %q, want the text after the unclosed marker", out.String())
	}
}

func TestCitationMarkerStreamFilter_BoundsLongCandidate(t *testing.T) {
	filter := newCitationMarkerStreamFilter([]string{"good"})
	long := "[ID:" + strings.Repeat("x", citationMarkerMaxLength)

	got := filter.Push(long) + filter.Flush()
	if got == "" {
		t.Fatal("Push buffered an over-long non-marker instead of emitting it")
	}
	if want := filterCitationMarkers(long, []string{"good"}); got != want {
		t.Fatalf("streamed = %q, want %q", got, want)
	}
}

func TestCitationMarkerStreamFilter_MatchesFinalFilter(t *testing.T) {
	allowed := []string{"good", "a2d067519c9e0db1"}
	cases := []struct {
		name    string
		content string
		splits  []int
	}{
		{name: "valid marker", content: "answer [ID:good] done", splits: []int{7, 12}},
		{name: "invalid marker", content: "keep [ID:bad] text", splits: []int{9}},
		{name: "unclosed marker", content: "answer [ID:good more text", splits: []int{7, 15}},
		{name: "over-long marker", content: "[ID:" + strings.Repeat("x", citationMarkerMaxLength+10) + "] tail", splits: []int{20, 40}},
		{name: "plain brackets", content: "see [note] and [", splits: []int{5}},
		{name: "unclosed marker before a markdown link", content: "answer [ID:oops more [link] end", splits: []int{7}},
		{name: "no markers", content: "just answer text", splits: []int{4}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := filterCitationMarkers(tc.content, allowed)
			filter := newCitationMarkerStreamFilter(allowed)
			var out strings.Builder
			prev := 0
			for _, cut := range tc.splits {
				if cut <= prev || cut >= len(tc.content) {
					continue
				}
				out.WriteString(filter.Push(tc.content[prev:cut]))
				prev = cut
			}
			out.WriteString(filter.Push(tc.content[prev:]))
			out.WriteString(filter.Flush())
			if got := out.String(); got != want {
				t.Fatalf("streamed = %q, want %q", got, want)
			}
		})
	}
}

func TestFilterCitationMarkers_LeavesOverlongMarkerLiteral(t *testing.T) {
	id := strings.Repeat("a", citationMarkerMaxLength)
	overlong := "[ID:" + id + "]"
	if got := filterCitationMarkers(overlong, []string{id}); got != overlong {
		t.Fatalf("filterCitationMarkers(overlong) = %q, want it left literal", got)
	}
}

func TestApplyCitationGrounding_FiltersUnknownIDs(t *testing.T) {
	inv := &groundingStreamingInvoker{groundingTestInvoker: groundingTestInvoker{
		responses: []string{"answer [ID:good] [ID:bad]"},
	}}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(inv)
	defer SetDefaultChatInvoker(prev)

	state := runtime.NewCanvasState("run-filter", "task-filter")
	ctx := runtime.WithState(t.Context(), state)
	var streamed string
	ctx = runtime.WithAgentMessageEmitter(ctx, func(content, _ string) {
		streamed += content
	})

	got, err := applyCitationGrounding(ctx, nil, AgentParam{ModelID: "stub", Cite: true}, "original", []prompts.CitationSource{
		{ID: "good", Content: "source body"},
	})
	if err != nil {
		t.Fatalf("applyCitationGrounding: %v", err)
	}
	if want := "answer [ID:good] "; got != want {
		t.Fatalf("returned content = %q, want %q", got, want)
	}
	if streamed != got {
		t.Fatalf("streamed = %q, want the filtered content %q", streamed, got)
	}
}
