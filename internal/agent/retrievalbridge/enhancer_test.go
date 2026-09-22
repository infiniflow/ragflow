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

// The document scope FilterDocuments returns is passed straight into
// nlp.RetrievalRequest.DocIDs, so it decides what the agent can find at all.
// Nothing downstream unpacks the "-999" no-match sentinel -- it reaches the doc
// store as a literal document id and matches no chunk. That is the intended
// answer for manual conditions the user wrote, and the wrong answer for an auto
// filter that could not narrow the search, which Python reports as None, i.e.
// no metadata narrowing at all (common/metadata_utils.py).

package retrievalbridge

import (
	"testing"

	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
)

func newFilterChatModel() *modelModule.ChatModel {
	name := "fake-model"
	driver := modelModule.NewDummyModel(nil, modelModule.URLSuffix{})
	return &modelModule.ChatModel{ModelDriver: driver, ModelName: &name, APIConfig: &modelModule.APIConfig{}}
}

// An auto filter that yields no conditions -- because the question named no
// metadata, or because the value space did not fit the model's context and
// generation was refused -- must leave the caller's scope alone. Answering with
// the no-match sentinel turns "we could not narrow this" into zero recall,
// which is strictly worse than searching the caller's whole scope.
func TestFilterDocumentsKeepsBaseScopeWhenTheFilterCannotNarrow(t *testing.T) {
	base := []string{"doc-1", "doc-2"}

	got, err := NewEnhancer(nil, nil).FilterDocuments(
		t.Context(), map[string]any{"method": "auto"}, "who wrote it?", newFilterChatModel(), base, nil,
	)
	if err != nil {
		t.Fatalf("FilterDocuments: %v", err)
	}

	for _, id := range got {
		if id == service.NoMatchDocIDSentinel {
			t.Fatalf("got %v: a filter that produced no conditions must not scope retrieval to nothing", got)
		}
	}
	if len(got) != len(base) {
		t.Fatalf("got %v, want the caller's scope %v", got, base)
	}
	for i := range got {
		if got[i] != base[i] {
			t.Fatalf("got %v, want the caller's scope %v", got, base)
		}
	}
}

// Manual conditions are the user's own, so a manual filter that matches nothing
// still scopes retrieval to nothing. This is the one case the sentinel exists for.
func TestFilterDocumentsReturnsTheSentinelForManualNoMatch(t *testing.T) {
	filter := map[string]any{
		"method": "manual",
		"logic":  "and",
		"manual": []any{map[string]any{"key": "author", "op": "=", "value": "nobody"}},
	}

	got, err := NewEnhancer(nil, nil).FilterDocuments(
		t.Context(), filter, "", nil, []string{"doc-1"}, nil,
	)
	if err != nil {
		t.Fatalf("FilterDocuments: %v", err)
	}
	if len(got) != 1 || got[0] != service.NoMatchDocIDSentinel {
		t.Fatalf("got %v, want [%s]", got, service.NoMatchDocIDSentinel)
	}
}
