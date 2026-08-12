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

//go:build integration

package elasticsearch

import (
	"context"
	"reflect"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/server/config"
)

// TestInsertChunks_ReadBackSuffixedFields is the T0 integration-tier read-back
// baseline (issue #17371): it writes a chunk to a real Elasticsearch instance
// and reads it back, asserting the stored document keeps the index-physical
// field names the ingestion pipeline emits. This catches regressions that the
// unit-tier test (which only inspects the request body) cannot — e.g. an ES
// mapping coercion that changes the stored value.
//
// Requires a running Elasticsearch; set ES_TEST=1 to run.
func TestInsertChunks_ReadBackSuffixedFields(t *testing.T) {
	if common.GetEnv(common.EnvESTest) != "1" {
		t.Skip("Skipping ES integration test; set ES_TEST=1 to run")
	}

	engine, err := NewEngine(getESTestConfig())
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	ctx := context.Background()
	baseName := "ragflow_chunk_readback_test"
	datasetID := "kb-1"
	chunkID := "readback-chunk-1"
	chunk := map[string]interface{}{
		"doc_id":               "doc-1",
		"id":                   chunkID,
		"kb_id":                "producer-kb-1",
		"docnm_kwd":            "sample.md",
		"content_with_weight":  "hello world",
		"create_timestamp_flt": float64(123.0),
		"question_kwd":         []string{"q1", "q2"},
		"important_kwd":        []string{"k1"},
		"page_num_int":         int(1),
		"position_int":         int(2),
	}

	if _, err := engine.InsertChunks(ctx, []map[string]interface{}{chunk}, baseName, datasetID); err != nil {
		t.Fatalf("InsertChunks: %v", err)
	}

	got, err := engine.GetChunk(ctx, baseName, chunkID, []string{datasetID})
	if err != nil {
		t.Fatalf("GetChunk: %v", err)
	}
	stored, ok := got.(map[string]interface{})
	if !ok {
		t.Fatalf("GetChunk returned %T, want map[string]interface{}", got)
	}

	assertStoredField(t, stored, "docnm_kwd", "sample.md")
	assertStoredField(t, stored, "content_with_weight", "hello world")
	assertStoredField(t, stored, "create_timestamp_flt", float64(123.0))
	assertStoredField(t, stored, "page_num_int", float64(1))
	// The input kb_id ("producer-kb-1") differs from datasetID ("kb-1"); the
	// stored value must equal datasetID, proving InsertChunks overrides it.
	assertStoredField(t, stored, "kb_id", datasetID)
	if v, ok := stored["question_kwd"]; !ok || !reflect.DeepEqual(v, []interface{}{"q1", "q2"}) {
		t.Errorf("stored question_kwd = %#v, want [q1 q2]", v)
	}
}

// getESTestConfig builds an Elasticsearch config for integration tests from
// the environment, falling back to localhost defaults. It is kept local to this
// file so the read-back test does not depend on kg_test.go (whose getTestConfig
// is an unrelated main-branch compile fix tracked separately).
func getESTestConfig() config.ElasticsearchConfig {
	hosts := common.GetEnv(common.EnvESHost)
	if hosts == "" {
		hosts = "http://localhost:1200"
	}
	username := common.GetEnv(common.EnvESUsername)
	if username == "" {
		username = "elastic"
	}
	password := common.GetEnv(common.EnvESPassword)
	if password == "" {
		password = "infini_rag_flow"
	}
	return config.ElasticsearchConfig{
		Hosts:    hosts,
		Username: username,
		Password: password,
	}
}
