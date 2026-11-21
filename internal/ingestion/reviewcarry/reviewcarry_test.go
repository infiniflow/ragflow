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

package reviewcarry

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/engine/types"
)

type fakeSearcher struct {
	result []map[string]interface{}
	err    error
	calls  int
	req    *types.SearchRequest
}

func (f *fakeSearcher) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	f.calls++
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	return &types.SearchResult{Chunks: f.result}, nil
}

func priorRow(status, by string) map[string]interface{} {
	return map[string]interface{}{
		"id":                       "row-1",
		"compilation_template_ids": []string{"tpl-1"},
		"review_status_kwd":        status,
		"review_by_kwd":            by,
		"review_at_flt":            float64(1794573916608),
		"review_note":              "checked against §3",
	}
}

func replacementRow() map[string]interface{} {
	return map[string]interface{}{
		"id":                       "row-1",
		"compilation_template_ids": []string{"tpl-1"},
		"content_with_weight":      "{}",
	}
}

// TestCarryOverKeepsTheVerdict is the whole point of the package: a recompile
// replaces the row, and the decision must survive it.
func TestCarryOverKeepsTheVerdict(t *testing.T) {
	searcher := &fakeSearcher{result: []map[string]interface{}{priorRow("accepted", "user-1")}}
	row := replacementRow()

	CarryOver(context.Background(), searcher, "ragflow_tenant", "kb-1", []map[string]interface{}{row})

	if row["review_status_kwd"] != "accepted" {
		t.Errorf("status: got %v", row["review_status_kwd"])
	}
	if row["review_by_kwd"] != "user-1" {
		t.Errorf("author: got %v", row["review_by_kwd"])
	}
	if row["review_at_flt"] != float64(1794573916608) {
		t.Errorf("time: got %v", row["review_at_flt"])
	}
	if row["review_note"] != "checked against §3" {
		t.Errorf("note: got %v", row["review_note"])
	}
	// The lookup is scoped to the dataset and asks for the verdict columns plus
	// the two identity columns.
	if got := searcher.req.Filter["kb_id"]; got != "kb-1" {
		t.Errorf("kb filter: got %v", got)
	}
	for _, field := range verdictFields {
		found := false
		for _, selected := range searcher.req.SelectFields {
			if selected == field {
				found = true
			}
		}
		if !found {
			t.Errorf("SelectFields must request %s: %v", field, searcher.req.SelectFields)
		}
	}
}

// TestCarryOverRefusesAnotherTemplate is the isolation rule: one dataset can be
// compiled by several models, and a verdict belongs to the model that produced
// the row.
func TestCarryOverRefusesAnotherTemplate(t *testing.T) {
	prior := priorRow("rejected", "user-1")
	prior["compilation_template_ids"] = []string{"tpl-2"}
	row := replacementRow()

	CarryOver(context.Background(), &fakeSearcher{result: []map[string]interface{}{prior}}, "ragflow_tenant", "kb-1", []map[string]interface{}{row})

	if _, carried := row["review_status_kwd"]; carried {
		t.Errorf("a verdict from another model must not be inherited: %v", row["review_status_kwd"])
	}
}

// TestCarryOverIgnoresTemplateOrder guards the set comparison: the same models
// stamped in another order still describe the same claim.
func TestCarryOverIgnoresTemplateOrder(t *testing.T) {
	prior := priorRow("accepted", "user-1")
	prior["compilation_template_ids"] = []string{"tpl-2", "tpl-1"}
	row := replacementRow()
	row["compilation_template_ids"] = []string{"tpl-1", "tpl-2"}

	CarryOver(context.Background(), &fakeSearcher{result: []map[string]interface{}{prior}}, "ragflow_tenant", "kb-1", []map[string]interface{}{row})

	if row["review_status_kwd"] != "accepted" {
		t.Errorf("status: got %v", row["review_status_kwd"])
	}
}

// TestCarryOverSkipsOrdinaryChunks covers the cost rule: a document's plain
// chunks carry no review, so the write path must not pay a lookup for them.
func TestCarryOverSkipsOrdinaryChunks(t *testing.T) {
	searcher := &fakeSearcher{}
	rows := []map[string]interface{}{
		{"id": "chunk-1", "content_with_weight": "text"},
		{"id": "chunk-2", "content_with_weight": "text"},
	}

	CarryOver(context.Background(), searcher, "ragflow_tenant", "kb-1", rows)

	if searcher.calls != 0 {
		t.Errorf("no compiled rows means no lookup, got %d call(s)", searcher.calls)
	}
}

// TestCarryOverSurvivesALookupFailure: losing a batch of chunks because an
// auxiliary read failed would be worse than judging a row twice.
func TestCarryOverSurvivesALookupFailure(t *testing.T) {
	searcher := &fakeSearcher{err: errors.New("engine down")}
	row := replacementRow()

	CarryOver(context.Background(), searcher, "ragflow_tenant", "kb-1", []map[string]interface{}{row})

	if _, carried := row["review_status_kwd"]; carried {
		t.Errorf("a failed lookup must leave the row unreviewed: %v", row["review_status_kwd"])
	}
}

// TestCarryOverMatchesOnBothIDs: the engine answers with every row whose id it
// matched, so a same-id row from another dataset must not be copied across.
func TestCarryOverMatchesOnBothIDs(t *testing.T) {
	prior := priorRow("accepted", "user-1")
	prior["id"] = "row-other"
	row := replacementRow()

	CarryOver(context.Background(), &fakeSearcher{result: []map[string]interface{}{prior}}, "ragflow_tenant", "kb-1", []map[string]interface{}{row})

	if _, carried := row["review_status_kwd"]; carried {
		t.Errorf("only the row being replaced may be matched: %v", row["review_status_kwd"])
	}
}
