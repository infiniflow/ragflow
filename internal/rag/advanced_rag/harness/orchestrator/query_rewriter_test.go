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

package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRewriteGapToQueryNeedsModelAndGaps(t *testing.T) {
	mdl := &stubJSONModel{value: map[string]any{"queries": []any{"q1"}}}
	if got := RewriteGapToQuery(context.Background(), RewriteDeps{Model: mdl}, "q", nil); got != nil {
		t.Error("no gaps must short-circuit")
	}
	if mdl.calls != 0 {
		t.Error("no gaps must not call the model")
	}
	if got := RewriteGapToQuery(context.Background(), RewriteDeps{}, "q",
		[]MissingPiece{{What: "x"}}); got != nil {
		t.Error("no model must return nil")
	}
}

func TestRewriteGapToQueryParsesAndDedupes(t *testing.T) {
	mdl := &stubJSONModel{value: map[string]any{"queries": []any{
		map[string]any{"query": "MASH finale run time minutes"},
		map[string]any{"question": "Cheers finale run time minutes"}, // key drift
		"plain string query",
		map[string]any{"query": "MASH finale run time minutes"}, // duplicate
		map[string]any{"query": "   "},                          // blank
	}}}
	got := RewriteGapToQuery(context.Background(), RewriteDeps{Model: mdl}, "which finale ran longest?",
		[]MissingPiece{{What: "runtimes", SearchHint: "finale run time"}})
	if len(got) != 3 {
		t.Fatalf("queries = %d, want 3 (blank + duplicate dropped)", len(got))
	}
	if got[0]["query"] != "MASH finale run time minutes" {
		t.Errorf("queries[0] = %q", got[0]["query"])
	}
	if got[1]["query"] != "Cheers finale run time minutes" {
		t.Errorf("queries[1] = %q, want the 'question' key accepted", got[1]["query"])
	}
	if got[2]["query"] != "plain string query" {
		t.Errorf("queries[2] = %q", got[2]["query"])
	}
}

func TestRewriteGapToQueryRendersAllVars(t *testing.T) {
	mdl := &stubJSONModel{value: map[string]any{"queries": []any{}}}
	RewriteGapToQuery(context.Background(), RewriteDeps{
		Model:           mdl,
		Prompts:         nil, // resolves the embedded sca_query_rewrite.md
		BridgeValues:    []string{"M*A*S*H", "Cheers", "  "},
		ResearchContext: "tried: finale runtimes (nothing new)",
	}, "which finale ran longest?",
		[]MissingPiece{{What: "runtimes", SearchHint: "finale run time"}})

	// The canonical template uses the spaced Jinja form, and every var must be
	// substituted (an unreplaced {{ }} would corrupt the prompt).
	if strings.Contains(mdl.last, "{{") {
		t.Errorf("unsubstituted placeholder remains: %.200s", mdl.last)
	}
	for _, want := range []string{"which finale ran longest?", "- M*A*S*H", "- Cheers",
		"what: runtimes; hint: finale run time", "tried: finale runtimes"} {
		if !strings.Contains(mdl.last, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// Blank bridge values are dropped.
	if strings.Contains(mdl.last, "-   ") {
		t.Error("blank bridge value must be dropped")
	}
}

func TestRewriteGapToQueryReturnsNilOnFailure(t *testing.T) {
	mdl := &stubJSONModel{err: errors.New("provider down")}
	if got := RewriteGapToQuery(context.Background(), RewriteDeps{Model: mdl}, "q",
		[]MissingPiece{{What: "x"}}); got != nil {
		t.Error("a model failure must return nil so the caller falls back")
	}
	// A non-dict response is also "no signal".
	mdl = &stubJSONModel{value: []any{"not", "a", "dict"}}
	if got := RewriteGapToQuery(context.Background(), RewriteDeps{Model: mdl}, "q",
		[]MissingPiece{{What: "x"}}); got != nil {
		t.Error("a non-dict response must return nil")
	}
}

// ---------------------------------------------------------------------------
// Prompt rendering
// ---------------------------------------------------------------------------
