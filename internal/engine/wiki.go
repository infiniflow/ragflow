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

package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"ragflow/internal/engine/types"
)

// SearchWithWikiContent reads the canonical body first and loads Markdown only
// for Wiki rows whose body is empty. Other rows and query errors are unchanged.
func SearchWithWikiContent(ctx context.Context, eng DocEngine, req *types.SearchRequest) (*types.SearchResult, error) {
	result, err := eng.Search(ctx, req)
	if !slices.Contains(req.SelectFields, "content_with_weight") {
		return result, err
	}
	contentErr := err
	if isMissingWikiField(err, "content") || isMissingWikiField(err, "content_with_weight") {
		metadataReq := *req
		metadataReq.SelectFields = slices.DeleteFunc(slices.Clone(req.SelectFields), func(field string) bool { return field == "content_with_weight" })
		result, err = eng.Search(ctx, &metadataReq)
	}
	if err != nil || result == nil {
		return result, err
	}
	var ids []string
	rows := make(map[string][]map[string]any)
	for _, row := range result.Chunks {
		if types.CompilationKind(row) != "wiki" {
			if contentErr != nil {
				return nil, contentErr
			}
			continue
		}
		if types.WikiPageContent(map[string]any{"content_with_weight": row["content_with_weight"]}) != "" {
			continue
		}
		id, _ := row["id"].(string)
		if id == "" {
			return nil, fmt.Errorf("wiki body fallback requires a row ID")
		}
		if _, ok := rows[id]; !ok {
			ids = append(ids, id)
		}
		rows[id] = append(rows[id], row)
	}
	if len(ids) == 0 {
		return result, nil
	}
	filter := make(map[string]any, len(req.Filter)+1)
	for field, value := range req.Filter {
		filter[field] = value
	}
	filter["id"] = ids
	fallback, err := eng.Search(ctx, &types.SearchRequest{
		IndexNames:         req.IndexNames,
		KbIDs:              req.KbIDs,
		SelectFields:       []string{"id", "md_with_weight"},
		Filter:             filter,
		Limit:              len(ids),
		IncludeUnavailable: true,
	})
	if isMissingWikiField(err, "md_with_weight") {
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read wiki Markdown fallback: %w", err)
	}
	if fallback != nil {
		for _, row := range fallback.Chunks {
			id, _ := row["id"].(string)
			for _, target := range rows[id] {
				target["md_with_weight"] = row["md_with_weight"]
			}
		}
	}
	return result, nil
}

func isMissingWikiField(err error, field string) bool {
	var exception *infinity.InfinityException
	if !errors.As(err, &exception) {
		return false
	}
	message := strings.TrimPrefix(exception.ErrorMsg, "Failed to execute query: ")
	switch infinity.ErrorCode(exception.ErrorCode) {
	case infinity.ErrorCodeColumnNotExist:
		return message == "Column: "+field+" doesn't exist"
	case infinity.ErrorCodeSyntaxError:
		// Infinity can report an absent SELECT column as a binder error. Other
		// syntax errors, including errors naming a different field, must surface.
		return message == "Fail to bind the expression: "+field
	default:
		return false
	}
}
