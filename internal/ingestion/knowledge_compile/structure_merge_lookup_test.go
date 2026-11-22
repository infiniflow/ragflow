package knowledge_compile

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/engine/types"
)

// keyedStructureLookupFilter names the narrowing filter a structure-merge read
// carried. Every such read must be keyed by the current id or by the identity
// fields; a read with none of them is the paged full-dataset scan that held the
// dataset write lock past the claim heartbeat's timeout (#22626).
func keyedStructureLookupFilter(req *types.SearchRequest) string {
	for _, key := range []string{"id", "name_kwd", "from_entity_kwd"} {
		if _, ok := req.Filter[key]; ok {
			return key
		}
	}
	return ""
}

func TestMergeExistingStructureBucketsFoldsRowsFetchedByIDAndIdentity(t *testing.T) {
	const tenant, kb = "t1", "kb1"
	buckets := []StructureBucket{{
		Name: "Engine", Type: "component", Description: "engine", CompileKwd: "hypergraph",
		TemplateID: "tpl1", TemplateKind: "knowledge_graph",
		SourceDocIDs: []string{"d0"}, SourceChunkIDs: []string{"c0"},
	}}
	currentID := datasetLevelStructureID(tenant, kb, structureTemplateIdentity("tpl1", "knowledge_graph"), "Engine", "component", "hypergraph", "")
	legacyID := "dataset_structure_legacy_type_scoped"
	entityRow := func(id string, docs, chunks []string) map[string]interface{} {
		return map[string]interface{}{
			"id": id, "compile_kwd": "hypergraph",
			"compilation_template_ids":      []string{"tpl1"},
			"compilation_template_kind_kwd": "knowledge_graph",
			"type_kwd":                      "entity", "name_kwd": "engine",
			"entity_type_kwd":     "other",
			"content_with_weight": `{"name":"Engine","type":"other"}`,
			"source_doc_ids":      docs, "source_chunk_ids": chunks,
		}
	}
	eng := &fakeEngine{engineType: "infinity", searchFn: func(req *types.SearchRequest) ([]map[string]interface{}, error) {
		switch keyedStructureLookupFilter(req) {
		case "id":
			return []map[string]interface{}{entityRow(currentID, []string{"d1"}, []string{"c1"})}, nil
		case "name_kwd":
			return []map[string]interface{}{entityRow(legacyID, []string{"d2"}, []string{"c2"})}, nil
		}
		return nil, nil
	}}

	stale, err := mergeExistingStructureBuckets(context.Background(), eng, "ragflow_t1", tenant, kb, buckets)
	if err != nil {
		t.Fatalf("mergeExistingStructureBuckets: %v", err)
	}
	if len(stale) != 1 || stale[0] != legacyID {
		t.Fatalf("stale ids = %v, want [%s] (the row stored under the older id)", stale, legacyID)
	}
	bucket := buckets[0]
	if got := strings.Join(bucket.SourceDocIDs, ","); got != "d0,d1,d2" {
		t.Errorf("source docs = %v, want the incoming, current-id and legacy-id docs unioned", bucket.SourceDocIDs)
	}
	if got := strings.Join(bucket.SourceChunkIDs, ","); got != "c0,c1,c2" {
		t.Errorf("source chunks = %v, want the incoming, current-id and legacy-id chunks unioned", bucket.SourceChunkIDs)
	}
	if bucket.Type != "component" {
		t.Errorf("type = %q, want the incoming specific type to win over the stored %q", bucket.Type, "other")
	}
	if len(eng.searchReqs) != 2 {
		t.Fatalf("issued %d searches, want 2 (one by id, one by entity name); nothing may scan the dataset", len(eng.searchReqs))
	}
	for _, req := range eng.searchReqs {
		if keyedStructureLookupFilter(req) == "" {
			t.Fatalf("unkeyed structure lookup: %+v", req.Filter)
		}
		if req.Offset != 0 {
			t.Fatalf("keyed lookup paged with offset %d: %+v", req.Offset, req.Filter)
		}
	}
}

func TestMergeExistingStructureBucketsSweepsOldRelationIDsByEndpoints(t *testing.T) {
	const tenant, kb = "t1", "kb1"
	buckets := []StructureBucket{{
		Name: "Acme Corp -> Beta Ltd", Type: "relation", Description: "supplies",
		FromEntity: "Acme Corp", ToEntity: "Beta Ltd", RelationType: "supplies",
		CompileKwd: "knowledge_graph", TemplateID: "tpl1", TemplateKind: "knowledge_graph",
	}}
	legacyID := "dataset_structure_legacy_relation"
	eng := &fakeEngine{engineType: "infinity", searchFn: func(req *types.SearchRequest) ([]map[string]interface{}, error) {
		if _, ok := req.Filter["from_entity_kwd"]; ok {
			if _, ok := req.Filter["to_entity_kwd"]; !ok {
				t.Fatalf("relation sweep must narrow by both endpoints: %+v", req.Filter)
			}
			return []map[string]interface{}{{
				"id": legacyID, "compile_kwd": "graph",
				"compilation_template_ids":      []string{"tpl1"},
				"compilation_template_kind_kwd": "knowledge_graph",
				"type_kwd":                      "relation",
				"from_entity_kwd":               "Acme Corp", "to_entity_kwd": "Beta Ltd",
				"content_with_weight": `{"from":"Acme Corp","to":"Beta Ltd","type":"supplies"}`,
			}}, nil
		}
		return nil, nil
	}}

	stale, err := mergeExistingStructureBuckets(context.Background(), eng, "ragflow_t1", tenant, kb, buckets)
	if err != nil {
		t.Fatalf("mergeExistingStructureBuckets: %v", err)
	}
	if len(stale) != 1 || stale[0] != legacyID {
		t.Fatalf("stale ids = %v, want [%s]", stale, legacyID)
	}
	for _, req := range eng.searchReqs {
		if keyedStructureLookupFilter(req) == "" {
			t.Fatalf("unkeyed structure lookup: %+v", req.Filter)
		}
	}
}

// One relation lookup must bound both endpoint lists: the engines cap a single
// terms filter (Elasticsearch rejects more than 65,536 values), so an unbounded
// to_entity_kwd list failed every attempt and kept the batch retrying.
func TestMergeExistingStructureBucketsBoundsRelationSweepPerBatch(t *testing.T) {
	const tenant, kb = "t1", "kb1"
	buckets := make([]StructureBucket, 0, 3*structureLookupBatch)
	for i := 0; i < 3*structureLookupBatch; i++ {
		from, to := fmt.Sprintf("From %d", i), fmt.Sprintf("To %d", i)
		buckets = append(buckets, StructureBucket{
			Name: from + " -> " + to, Type: "relation", Description: "links",
			FromEntity: from, ToEntity: to, RelationType: "links",
			CompileKwd: "knowledge_graph", TemplateID: "tpl1", TemplateKind: "knowledge_graph",
		})
	}
	eng := &fakeEngine{engineType: "infinity"}

	if _, err := mergeExistingStructureBuckets(context.Background(), eng, "ragflow_t1", tenant, kb, buckets); err != nil {
		t.Fatalf("mergeExistingStructureBuckets: %v", err)
	}
	relationSearches := 0
	for _, req := range eng.searchReqs {
		froms, hasFrom := req.Filter["from_entity_kwd"]
		tos, hasTo := req.Filter["to_entity_kwd"]
		if !hasFrom && !hasTo {
			continue
		}
		relationSearches++
		if hasFrom && len(froms.([]string)) > structureLookupBatch {
			t.Fatalf("from_entity_kwd carries %d values, over the batch bound %d", len(froms.([]string)), structureLookupBatch)
		}
		if hasTo && len(tos.([]string)) > structureLookupBatch {
			t.Fatalf("to_entity_kwd carries %d values, over the batch bound %d", len(tos.([]string)), structureLookupBatch)
		}
	}
	if relationSearches < 2 {
		t.Fatalf("relation sweep issued %d searches, want the endpoints spread over several bounded batches", relationSearches)
	}
}
