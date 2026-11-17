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

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine/internal/esoscommon"
	"ragflow/internal/engine/types"
)

const (
	chunkPageKeepAlive = "2m"
	chunkPageTimeout   = 30 * time.Second
)

// WalkChunkPages visits a filtered chunk store in bounded pages from one
// snapshot. The caller selects the fields it needs for each page.
func (e *Engine) WalkChunkPages(ctx context.Context, req *types.SearchRequest, visit func([]map[string]any) error) error {
	if len(req.IndexNames) != 1 || len(req.KbIDs) == 0 || len(req.SelectFields) == 0 || req.Limit <= 0 {
		return fmt.Errorf("chunk scan requires one index, a dataset, fields and a positive page size")
	}
	pitID, err := e.openChunkPIT(ctx, req.IndexNames)
	if err != nil || pitID == "" {
		return err
	}
	defer func() {
		if err := e.closeChunkPIT(ctx, pitID); err != nil {
			common.Warn(fmt.Sprintf("close Elasticsearch chunk snapshot: %v", err))
		}
	}()

	pit := map[string]any{"id": pitID, "keep_alive": chunkPageKeepAlive}
	query := map[string]any{
		"query":   buildBoolQueryFromCondition(req.Filter, req.KbIDs, false, false),
		"_source": req.SelectFields,
		"sort":    []string{"_shard_doc"},
		"pit":     pit,
	}
	// The point in time already identifies the index. Its search requests
	// go to /_search and carry the latest PIT ID in the body.
	search := &types.SearchRequest{}
	var cursor []any
	for {
		pageCtx, cancel := context.WithTimeout(ctx, chunkPageTimeout)
		response, err := e.executeSearchRequest(pageCtx, search, query, req.Limit, cursor, false)
		cancel()
		if err != nil {
			return err
		}
		if response.PITID != "" {
			pitID = response.PITID
			pit["id"] = pitID
		}
		if response.TimedOut || response.Shards.Failed > 0 {
			return fmt.Errorf("incomplete Elasticsearch chunk page (timed out: %t, failed shards: %d)", response.TimedOut, response.Shards.Failed)
		}
		hits := response.Hits.Hits
		if len(hits) == 0 {
			return nil
		}
		nextCursor := hits[len(hits)-1].Sort
		if len(nextCursor) != 1 || esoscommon.SortValuesEqual(cursor, nextCursor) {
			return fmt.Errorf("Elasticsearch chunk cursor did not advance")
		}
		if err := visit(convertESResponse(&response, "")); err != nil {
			return err
		}
		if len(hits) < req.Limit {
			return nil
		}
		cursor = nextCursor
	}
}

func (e *Engine) openChunkPIT(ctx context.Context, indices []string) (string, error) {
	openCtx, cancel := context.WithTimeout(ctx, chunkPageTimeout)
	defer cancel()
	res, err := e.client.OpenPointInTime(indices, chunkPageKeepAlive, e.client.OpenPointInTime.WithContext(openCtx))
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.IsError() {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return "", err
		}
		var failure struct {
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		if res.StatusCode == http.StatusNotFound && json.Unmarshal(body, &failure) == nil && failure.Error.Type == "index_not_found_exception" {
			return "", nil
		}
		return "", fmt.Errorf("open Elasticsearch chunk snapshot: %s", body)
	}
	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", fmt.Errorf("Elasticsearch returned an empty chunk snapshot ID")
	}
	return result.ID, nil
}

func (e *Engine) closeChunkPIT(ctx context.Context, pitID string) error {
	closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	body, err := json.Marshal(map[string]string{"id": pitID})
	if err != nil {
		return err
	}
	res, err := e.client.ClosePointInTime(e.client.ClosePointInTime.WithBody(bytes.NewReader(body)), e.client.ClosePointInTime.WithContext(closeCtx))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	if res.IsError() {
		return fmt.Errorf("Elasticsearch returned %s", res.Status())
	}
	return nil
}
