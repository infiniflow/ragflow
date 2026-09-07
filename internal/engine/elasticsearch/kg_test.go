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

package elasticsearch

import (
	"context"
	"os"
	"testing"

	"ragflow/internal/engine/types"
)

// TestKGSearchSelectFields verifies that SelectFields overrides default output
// columns when searching for knowledge graph entities.
// Requires a running Elasticsearch instance and KG data indexed by Python task executor.
// Set ES_TEST=1 to run.
func TestKGSearchSelectFields(t *testing.T) {
	if os.Getenv("ES_TEST") != "1" {
		t.Skip("Skipping ES integration test; set ES_TEST=1 to run")
	}

	engine, err := NewEngine(getTestConfig())
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Search for KG entities using SelectFields
	req := &types.SearchRequest{
		IndexNames: []string{"ragflow_*"},
		KbIDs:      []string{},
		Filter: map[string]interface{}{
			"knowledge_graph_kwd": "entity",
		},
		SelectFields: []string{"entity_kwd", "entity_type_kwd", "rank_flt"},
		Limit:        10,
	}

	result, err := engine.Search(context.Background(), req)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	// Verify returned chunks contain only allowed fields
	allowedFields := map[string]bool{
		"entity_kwd":      true,
		"entity_type_kwd": true,
		"rank_flt":        true,
		"_score":          true,
	}

	for i, chunk := range result.Chunks {
		for key := range chunk {
			if !allowedFields[key] {
				t.Errorf("chunk[%d] contains unexpected field: %s (allowed: entity_kwd, entity_type_kwd, rank_flt)", i, key)
			}
		}
	}
}

// getTestConfig returns a minimal ES config for testing.
// Reads from environment or uses defaults pointing to localhost.
func getTestConfig() map[string]interface{} {
	hosts := os.Getenv("ES_HOSTS")
	if hosts == "" {
		hosts = "http://localhost:1200"
	}
	username := os.Getenv("ES_USERNAME")
	if username == "" {
		username = "elastic"
	}
	password := os.Getenv("ES_PASSWORD")
	if password == "" {
		password = "infini_rag_flow"
	}
	return map[string]interface{}{
		"hosts":    []string{hosts},
		"username": username,
		"password": password,
	}
}
