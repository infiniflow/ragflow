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

package knowledge_compile

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// windowEngine pages its rows the way the Elasticsearch engine does: a request
// without an explicit sort uses from/size, which Elasticsearch rejects once
// from+size passes index.max_result_window; a sorted request walks past the
// window with search_after.
type windowEngine struct {
	*fakeEngine
	rows []map[string]interface{}
}

func (w *windowEngine) Search(_ context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	w.lastSearchReq = req
	sorted := req.OrderBy != nil && len(req.OrderBy.Fields) > 0
	if !sorted && req.Offset+req.Limit > common.MAX_RESULT_WINDOW {
		return nil, fmt.Errorf("result window is too large, from + size must be less than or equal to: [%d]", common.MAX_RESULT_WINDOW)
	}
	start := min(req.Offset, len(w.rows))
	end := min(req.Offset+req.Limit, len(w.rows))
	return &types.SearchResult{Chunks: w.rows[start:end]}, nil
}

// structureRows builds n dataset-scope entity rows in id order, each sourced
// from a document that is not being deleted.
func structureRows(n int) []map[string]interface{} {
	rows := make([]map[string]interface{}, n)
	for i := range rows {
		rows[i] = map[string]interface{}{
			"id":             fmt.Sprintf("row-%05d", i),
			"compile_kwd":    "hypergraph",
			"source_doc_ids": []string{"kept"},
		}
	}
	return rows
}

// TestDeleteStructureForDocs_FindsGhostsPastResultWindow covers the ghost scan
// on a dataset with more structure rows than index.max_result_window: a ghost
// sorted past the window must still be found and deleted.
func TestDeleteStructureForDocs_FindsGhostsPastResultWindow(t *testing.T) {
	rows := structureRows(common.MAX_RESULT_WINDOW + 2000)
	ghost := common.MAX_RESULT_WINDOW + 1500
	rows[ghost]["source_doc_ids"] = []string{"gone"}
	eng := &windowEngine{fakeEngine: &fakeEngine{}, rows: rows}

	if err := (engineWriter{eng: eng}).DeleteStructureForDocs(context.Background(), "t1", "kb1", []string{"gone"}); err != nil {
		t.Fatalf("DeleteStructureForDocs: %v", err)
	}
	if len(eng.deleteConds) == 0 {
		t.Fatal("the ghost row past the result window was not deleted")
	}
	ids, _ := eng.deleteConds[0]["id"].([]string)
	if want := fmt.Sprintf("row-%05d", ghost); len(ids) != 1 || ids[0] != want {
		t.Fatalf("deleted ids = %v, want [%s]", ids, want)
	}
}
