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

package infinity

import (
	"reflect"
	"testing"
)

func TestGetHighlightUsesRegex(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{"id": "full-text", "content_with_weight": "Alpha beta.\nbetamax soup. BETA again!"},
		{"id": "no-match", "content_with_weight": "Keep the complete text."},
	}

	got := engine.GetHighlight(chunks, []string{"beta"}, "content_with_weight")
	want := map[string]string{
		"full-text": "Alpha <em>beta</em>.\n<em>betamax</em> soup. <em>BETA</em> again!",
		"no-match":  "Keep the complete text.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight() = %#v, want %#v", got, want)
	}
}

func TestGetHighlightExpandsLatinStems(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{
			"id":                  "latin",
			"content_with_weight": "Required coordinated isolation. Coordinación.",
		},
	}

	got := engine.GetHighlight(chunks, []string{"requir", "coordin", "isol"}, "content_with_weight")
	want := map[string]string{
		"latin": "<em>Required</em> <em>coordinated</em> <em>isolation</em>. <em>Coordinación</em>.",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight() = %#v, want %#v", got, want)
	}
}

func TestGetHighlightPrefersLongKeywordsAndFallsBackToContent(t *testing.T) {
	engine := &Engine{}
	chunks := []map[string]interface{}{
		{"id": "overlap", "content": "世界和世"},
	}

	got := engine.GetHighlight(chunks, []string{"世", "世界", ""}, "content_with_weight")
	want := map[string]string{"overlap": "<em>世界</em>和<em>世</em>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight() = %#v, want %#v", got, want)
	}

	got = engine.GetHighlight(chunks, nil, "content_with_weight")
	want = map[string]string{"overlap": "世界和世"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetHighlight() without keywords = %#v, want %#v", got, want)
	}
}
