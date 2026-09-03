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

package serenedb

import (
	"context"
	"fmt"
	"strconv"
)

// asChunkMap coerces a skill document to a chunk map.
func asChunkMap(doc interface{}) (map[string]interface{}, error) {
	m, ok := doc.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("serenedb: document must be a map, got %T", doc)
	}
	return m, nil
}

// ensureSkillTable creates the skill table (named by indexName) if absent,
// inferring the vector size from the documents.
func (e *serenedbEngine) ensureSkillTable(ctx context.Context, indexName string, docs []map[string]interface{}) error {
	exists, err := e.tableExists(ctx, indexName)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	size := 0
	for _, doc := range docs {
		for k := range doc {
			if m := vectorColumnPattern.FindStringSubmatch(k); m != nil {
				size, _ = strconv.Atoi(m[1])
			}
		}
	}
	for _, stmt := range chunkTableDDL(indexName, size) {
		if err := e.exec(ctx, stmt); err != nil {
			return fmt.Errorf("serenedb: create skill table %s: %w", indexName, err)
		}
	}
	return nil
}

// IndexDocument upserts a single skill document.
func (e *serenedbEngine) IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
	m, err := asChunkMap(doc)
	if err != nil {
		return err
	}
	if _, ok := m["id"]; !ok {
		m["id"] = docID
	}
	if err := e.ensureSkillTable(ctx, indexName, []map[string]interface{}{m}); err != nil {
		return err
	}
	cols, vals := prepareChunkRow(m, "")
	query, args := buildUpsert(indexName, cols, [][]interface{}{vals})
	return e.exec(ctx, query, args...)
}

// BulkIndex upserts a batch of skill documents.
func (e *serenedbEngine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
	maps := make([]map[string]interface{}, 0, len(docs))
	for _, d := range docs {
		m, err := asChunkMap(d)
		if err != nil {
			return nil, err
		}
		maps = append(maps, m)
	}
	if len(maps) == 0 {
		return nil, nil
	}
	if err := e.ensureSkillTable(ctx, indexName, maps); err != nil {
		return nil, err
	}
	for _, m := range maps {
		cols, vals := prepareChunkRow(m, "")
		query, args := buildUpsert(indexName, cols, [][]interface{}{vals})
		if err := e.exec(ctx, query, args...); err != nil {
			return nil, fmt.Errorf("serenedb: bulk index %s: %w", indexName, err)
		}
	}
	return len(maps), nil
}

// DeleteDocument removes a skill document by id. A missing table is not an error.
func (e *serenedbEngine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	exists, err := e.tableExists(ctx, indexName)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	return e.exec(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", indexName), docID)
}
