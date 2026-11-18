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

package service

import (
	"strings"
	"testing"
)

// TestNavPromptsConstrainLanguage pins the language rule on both cluster prompts.
// The prompts are English, and without the rule the model answered in English
// even for a Chinese dataset: the cluster name it returns (plus the deterministic
// hash suffix) is what the nav tree displays, and its "summary" is the cluster
// description the artifacts page shows.
func TestNavPromptsConstrainLanguage(t *testing.T) {
	naming := navNamingPrompt("赵云的英勇事迹与忠诚品质\n周瑜是东吴大都督……")
	if !strings.Contains(naming, "same language as the excerpts") {
		t.Errorf("naming prompt lost its language constraint: %s", naming)
	}
	// The source text and the JSON contract must survive the extra instruction.
	if !strings.Contains(naming, "赵云的英勇事迹与忠诚品质") {
		t.Error("naming prompt dropped the source text")
	}
	if !strings.Contains(naming, `Return ONLY JSON: {"name": "<2-6 word topic title>"`) {
		t.Error("naming prompt dropped the JSON contract")
	}

	merging := navMergePrompt("Three Kingdoms Historical Figures", "赵云的英勇事迹与忠诚品质")
	if !strings.Contains(merging, "same language as the New description") {
		t.Errorf("merge prompt lost its language constraint: %s", merging)
	}
	if !strings.Contains(merging, "Existing: Three Kingdoms Historical Figures") ||
		!strings.Contains(merging, "New: 赵云的英勇事迹与忠诚品质") {
		t.Error("merge prompt dropped one of the two descriptions")
	}
	if !strings.Contains(merging, "Return ONLY the merged text, no commentary.") {
		t.Error("merge prompt dropped its output contract")
	}
}

// TestNavSummaryFromReply pins the naming reply contract (Python
// _llm_create_summary): "name" is the topic title, "summary" falls back to
// "result" and then to the source text. An empty name is surfaced as-is so the
// caller keeps its deterministic "<summary first line> <hash>" name.
func TestNavSummaryFromReply(t *testing.T) {
	tests := []struct {
		name        string
		reply       map[string]any
		fallback    string
		wantName    string
		wantSummary string
	}{
		{
			name:        "name and summary",
			reply:       map[string]any{"name": "三国名将事迹", "summary": "三国名将的事迹概览。"},
			fallback:    "source summary",
			wantName:    "三国名将事迹",
			wantSummary: "三国名将的事迹概览。",
		},
		{
			name:        "summary key missing falls back to result",
			reply:       map[string]any{"name": " topic ", "result": "merged text"},
			fallback:    "source summary",
			wantName:    "topic",
			wantSummary: "merged text",
		},
		{
			name:        "no usable fields keeps the source text",
			reply:       map[string]any{},
			fallback:    "source summary",
			wantName:    "",
			wantSummary: "source summary",
		},
		{
			name:        "name missing leaves the caller to fall back",
			reply:       map[string]any{"summary": "only a summary"},
			fallback:    "source summary",
			wantName:    "",
			wantSummary: "only a summary",
		},
		{
			name:        "non-string values are ignored, not stringified",
			reply:       map[string]any{"name": 42, "summary": map[string]any{"x": 1}},
			fallback:    "source summary",
			wantName:    "",
			wantSummary: "source summary",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotSummary := navSummaryFromReply(tt.reply, tt.fallback)
			if gotName != tt.wantName || gotSummary != tt.wantSummary {
				t.Errorf("navSummaryFromReply() = (%q, %q), want (%q, %q)",
					gotName, gotSummary, tt.wantName, tt.wantSummary)
			}
		})
	}
}

// TestNavMergedFromReply pins the merge reply contract (Python _llm_merge), with
// the deliberate deviation documented on the function: a plain-text reply is
// used (the prompt asks for "only the merged text"), while a JSON reply prefers
// "merged" then "result" and an unusable reply keeps the existing description.
func TestNavMergedFromReply(t *testing.T) {
	tests := []struct {
		name     string
		reply    string
		existing string
		want     string
	}{
		{
			name:     "plain text reply is the merged summary",
			reply:    "三国名将与曹操的对比，两人才能各具特色。",
			existing: "old description",
			want:     "三国名将与曹操的对比，两人才能各具特色。",
		},
		{
			name:     "merged key wins",
			reply:    `{"merged": "fused description"}`,
			existing: "old description",
			want:     "fused description",
		},
		{
			name:     "result key is accepted",
			reply:    `{"result": "fused via result"}`,
			existing: "old description",
			want:     "fused via result",
		},
		{
			name:     "json without the expected keys keeps the existing text",
			reply:    `{"unexpected": "value"}`,
			existing: "old description",
			want:     "old description",
		},
		{
			name:     "empty reply keeps the existing text",
			reply:    "",
			existing: "old description",
			want:     "old description",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := navMergedFromReply(tt.reply, tt.existing); got != tt.want {
				t.Errorf("navMergedFromReply(%q) = %q, want %q", tt.reply, got, tt.want)
			}
		})
	}
}
