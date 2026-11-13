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

// Package reviewcarry keeps a review verdict attached to the row it was made
// about when a compiler rewrites that row.
//
// A compiled row is written whole: the engine replaces the document, so every
// column a compiler does not restate is gone. The review columns are written by
// the review endpoint and never by a compiler, so without this step a recompile
// silently resets every decision. The dataset-wide folds make that worse than it
// sounds — they are rewritten whenever any document's text changes, so the
// decision a reviewer just made is the one most likely to disappear next.
//
// Rows are matched on (id, compilation_template_ids). A row's id is a stable
// identity (name and class for structure rows, variant and cluster key for merged
// rows), not a content hash, so the verdict still belongs to the row it was made
// about: only a renamed or reclassed row gets a new id, and that row is a
// different claim. The template check stops one model's verdict from being
// inherited by the same knowledge compiled under another model.
package reviewcarry

import (
	"context"
	"sort"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// batchSize bounds one lookup. Ids are queried together because a compile writes
// in batches already; a smaller page would multiply round trips for no reason.
const batchSize = 200

// Searcher is the only engine method carry-over needs: reading the rows that are
// about to be replaced. Declaring it here rather than taking engine.DocEngine
// keeps the dependency honest and the tests small.
type Searcher interface {
	Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error)
}

// verdictFields are the columns the review endpoint owns. Everything else in a
// row comes from the compiler, so only these have to be carried.
var verdictFields = []string{
	"review_status_kwd",
	"review_by_kwd",
	"review_at_flt",
	"review_note",
}

// CarryOver copies the stored verdict onto rows that replace it, and leaves rows
// with no prior verdict untouched (a new row is unreviewed, which is the truth).
//
// Rows without a template stamp are skipped: those are ordinary document chunks,
// which carry no verdict and cost no query.
//
// A failed lookup is logged and swallowed. Losing a batch of chunks because the
// engine could not answer one auxiliary read would be worse than the reviewer
// having to judge a row twice.
func CarryOver(
	ctx context.Context,
	eng Searcher,
	baseName string,
	datasetID string,
	rows []map[string]interface{},
) {
	if eng == nil || len(rows) == 0 {
		return
	}
	byID := make(map[string]map[string]interface{}, len(rows))
	templates := make(map[string][]string, len(rows))
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		id, _ := row["id"].(string)
		if id == "" {
			continue
		}
		stamps := stringSlice(row["compilation_template_ids"])
		if len(stamps) == 0 {
			continue
		}
		if _, seen := byID[id]; !seen {
			ids = append(ids, id)
		}
		byID[id] = row
		templates[id] = stamps
	}
	if len(ids) == 0 {
		return
	}
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		res, err := eng.Search(ctx, &types.SearchRequest{
			IndexNames: []string{baseName},
			KbIDs:      []string{datasetID},
			SelectFields: append([]string{"id", "compilation_template_ids"},
				verdictFields...),
			Filter: map[string]interface{}{
				"id":    ids[start:end],
				"kb_id": datasetID,
			},
			Offset: 0,
			Limit:  end - start,
		})
		if err != nil {
			common.Warn("review carry-over: reading prior verdicts failed, this batch keeps none",
				zap.String("dataset_id", datasetID),
				zap.Int("rows", len(ids[start:end])),
				zap.Error(err))
			return
		}
		for _, prior := range res.Chunks {
			id, _ := prior["id"].(string)
			row := byID[id]
			if row == nil {
				continue
			}
			if !sameStamps(templates[id], stringSlice(prior["compilation_template_ids"])) {
				continue
			}
			for _, field := range verdictFields {
				value, ok := prior[field]
				if !ok || value == nil {
					continue
				}
				if text, isText := value.(string); isText && text == "" {
					continue
				}
				row[field] = value
			}
		}
	}
}

// sameStamps compares the template stamps as sets: a row compiled by the same
// models is the same claim regardless of the order the ids were stored in.
func sameStamps(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	a := append([]string{}, left...)
	b := append([]string{}, right...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// stringSlice reads a stamped column. The engine hands back []string for keyword
// columns and []interface{} for anything it had to decode, so both are accepted.
func stringSlice(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if text, ok := item.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}
