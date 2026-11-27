package knowledge_compile

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/common"
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

// One relation lookup must bound the product of its two endpoint lists: the
// engines reject a single terms filter past 65,536 values. Every incoming
// endpoint must still reach at least one lookup.
func TestMergeExistingStructureBucketsBoundsRelationSweepProduct(t *testing.T) {
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
	coveredFroms := map[string]bool{}
	for _, req := range eng.searchReqs {
		froms, hasFrom := req.Filter["from_entity_kwd"]
		tos, hasTo := req.Filter["to_entity_kwd"]
		if !hasFrom && !hasTo {
			continue
		}
		relationSearches++
		fromList, _ := froms.([]string)
		toList, _ := tos.([]string)
		// The terms filter takes both endpoint lists; the engines cap one terms
		// list far below the product this sweep allows.
		if len(fromList)*len(toList) > relationSweepPairCap {
			t.Fatalf("relation lookup product = %d values (%d x %d), over the cap %d",
				len(fromList)*len(toList), len(fromList), len(toList), relationSweepPairCap)
		}
		for _, from := range fromList {
			coveredFroms[from] = true
		}
	}
	if relationSearches < 2 {
		t.Fatalf("relation sweep issued %d searches, want the pairs spread over several bounded batches", relationSearches)
	}
	for i := 0; i < 3*structureLookupBatch; i++ {
		if !coveredFroms[fmt.Sprintf("From %d", i)] {
			t.Fatalf("relation %d: from endpoint missing from every lookup, the sweep is incomplete", i)
		}
	}
}

// The endpoint product does not bound the rows one relation lookup returns: an
// endpoint pair can hold one row per relation type, template and superseded id.
// A group whose rows fill the engine result window must be split until every
// request stays inside the window, or the offset paging dies on the deep page
// and the merge never writes.
func TestMergeExistingStructureBucketsSplitsRelationSweepAtTheResultWindow(t *testing.T) {
	const tenant, kb = "t1", "kb1"
	const fromCount, toCount, copies = 20, 20, 26 // 400 pairs x 26 rows = 10,400 rows
	buckets := make([]StructureBucket, 0, fromCount*toCount)
	for i := 0; i < fromCount; i++ {
		for j := 0; j < toCount; j++ {
			from, to := fmt.Sprintf("entity-%d", i), fmt.Sprintf("target-%d", j)
			index := i*toCount + j
			buckets = append(buckets, StructureBucket{
				Name: from + " -> " + to, Type: "relation", Description: "links",
				FromEntity: from, ToEntity: to, RelationType: "links",
				CompileKwd: "knowledge_graph", TemplateID: "tpl1", TemplateKind: "knowledge_graph",
				SourceDocIDs:   []string{fmt.Sprintf("incoming-doc-%d", index)},
				SourceChunkIDs: []string{fmt.Sprintf("incoming-chunk-%d", index)},
			})
		}
	}
	stored := make([]map[string]interface{}, 0, fromCount*toCount*copies)
	currentIDs := make(map[string]bool, fromCount*toCount)
	wantStale := make(map[string]bool, fromCount*toCount*(copies-1))
	for i := 0; i < fromCount; i++ {
		for j := 0; j < toCount; j++ {
			from, to := fmt.Sprintf("entity-%d", i), fmt.Sprintf("target-%d", j)
			currentID := datasetLevelStructureID(tenant, kb, structureTemplateIdentity("tpl1", "knowledge_graph"), from+" -> "+to, "relation", "knowledge_graph", "links")
			currentIDs[currentID] = true
			for k := 0; k < copies; k++ {
				id := currentID
				if k > 0 {
					id = fmt.Sprintf("legacy-%d-%d-%d", i, j, k)
					wantStale[id] = true
				}
				stored = append(stored, map[string]interface{}{
					"id": id, "compile_kwd": "knowledge_graph",
					"compilation_template_ids":      []string{"tpl1"},
					"compilation_template_kind_kwd": "knowledge_graph",
					"type_kwd":                      "relation",
					"from_entity_kwd":               from, "to_entity_kwd": to,
					"content_with_weight": fmt.Sprintf(`{"from":%q,"to":%q,"type":"links"}`, from, to),
					"source_doc_ids":      []string{fmt.Sprintf("stored-doc-%d-%d-%d", i, j, k)},
					"source_chunk_ids":    []string{fmt.Sprintf("stored-chunk-%d-%d-%d", i, j, k)},
				})
			}
		}
	}
	eng := &fakeEngine{engineType: "infinity", searchFn: func(req *types.SearchRequest) ([]map[string]interface{}, error) {
		if req.Offset+req.Limit > common.MAX_RESULT_WINDOW {
			return nil, fmt.Errorf("Result window is too large, from + size must be <= %d", common.MAX_RESULT_WINDOW)
		}
		matched := stored
		if raw, ok := req.Filter["id"]; ok {
			wanted := make(map[string]bool, len(raw.([]string)))
			for _, id := range raw.([]string) {
				wanted[id] = true
			}
			matched = nil
			for _, row := range stored {
				if wanted[row["id"].(string)] {
					matched = append(matched, row)
				}
			}
		}
		if raw, ok := req.Filter["from_entity_kwd"]; ok {
			froms := make(map[string]bool, len(raw.([]string)))
			for _, from := range raw.([]string) {
				froms[from] = true
			}
			tos := make(map[string]bool)
			for _, to := range req.Filter["to_entity_kwd"].([]string) {
				tos[to] = true
			}
			filtered := matched[:0:0]
			for _, row := range matched {
				if froms[row["from_entity_kwd"].(string)] && tos[row["to_entity_kwd"].(string)] {
					filtered = append(filtered, row)
				}
			}
			matched = filtered
		}
		start, end := req.Offset, req.Offset+req.Limit
		if start > len(matched) {
			start = len(matched)
		}
		if end > len(matched) {
			end = len(matched)
		}
		return matched[start:end], nil
	}}

	stale, err := mergeExistingStructureBuckets(context.Background(), eng, "ragflow_t1", tenant, kb, buckets)
	if err != nil {
		t.Fatalf("mergeExistingStructureBuckets: %v", err)
	}
	if len(stale) != len(wantStale) {
		t.Fatalf("stale ids = %d, want %d (every superseded row)", len(stale), len(wantStale))
	}
	for _, id := range stale {
		if !wantStale[id] {
			t.Fatalf("stale id %q is not a superseded stored row", id)
		}
	}
	for k := range buckets {
		b := buckets[k]
		if len(b.SourceDocIDs) != copies+1 || len(b.SourceChunkIDs) != copies+1 {
			t.Fatalf("bucket %q kept %d docs and %d chunks, want the incoming one plus all %d stored rows",
				b.Name, len(b.SourceDocIDs), len(b.SourceChunkIDs), copies)
		}
	}
	relationSearches := 0
	for _, req := range eng.searchReqs {
		if _, ok := req.Filter["from_entity_kwd"]; ok {
			relationSearches++
		}
	}
	if relationSearches < 2 {
		t.Fatalf("relation sweep issued %d searches, want the window-filling group split into bounded batches", relationSearches)
	}
}

// structureFilterMatches evaluates the subset of the engine filter language the
// structure merge writes — id value lists, "and" groups and must_not negation —
// with the Boolean meaning the engines give it. Keys outside that subset are
// left to the caller, which narrows the rows it feeds in.
func structureFilterMatches(filter map[string]interface{}, id string) bool {
	for k, v := range filter {
		switch k {
		case "id":
			switch ids := v.(type) {
			case []string:
				found := false
				for _, candidate := range ids {
					if candidate == id {
						found = true
						break
					}
				}
				if !found {
					return false
				}
			case string:
				if ids != id {
					return false
				}
			default:
				return false
			}
		case "and":
			children := types.FilterClauses(v)
			if len(children) == 0 {
				return false
			}
			for _, child := range children {
				if !structureFilterMatches(child, id) {
					return false
				}
			}
		case "must_not":
			negated, ok := v.(map[string]interface{})
			if !ok || structureFilterMatches(negated, id) {
				return false
			}
		}
	}
	return true
}

// A single endpoint pair cannot be split any further, but its stored rows can
// still fill the engine result window. The sweep must read the pair completely
// in window-sized rounds (excluding the ids already read) instead of failing the
// merge, for a pair holding exactly the window and for one holding more.
func TestMergeExistingStructureBucketsReadsASinglePairPastTheResultWindow(t *testing.T) {
	for _, tc := range []struct {
		name          string
		rows          int
		wantExclusion bool
	}{
		{name: "exactly the window", rows: common.MAX_RESULT_WINDOW},
		{name: "past the window", rows: common.MAX_RESULT_WINDOW + 400, wantExclusion: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const tenant, kb = "t1", "kb1"
			const from, to = "entity-a", "target-b"
			bucket := StructureBucket{
				Name: from + " -> " + to, Type: "relation", Description: "links",
				FromEntity: from, ToEntity: to, RelationType: "links",
				CompileKwd: "knowledge_graph", TemplateID: "tpl1", TemplateKind: "knowledge_graph",
				SourceDocIDs: []string{"incoming-doc"}, SourceChunkIDs: []string{"incoming-chunk"},
			}
			currentID := datasetLevelStructureID(tenant, kb, structureTemplateIdentity("tpl1", "knowledge_graph"), bucket.Name, "relation", "knowledge_graph", "links")
			wantStale := make(map[string]bool, tc.rows-1)
			stored := make([]map[string]interface{}, 0, tc.rows)
			for k := 0; k < tc.rows; k++ {
				id := currentID
				if k > 0 {
					id = fmt.Sprintf("legacy-%d", k)
					wantStale[id] = true
				}
				stored = append(stored, map[string]interface{}{
					"id": id, "compile_kwd": "knowledge_graph",
					"compilation_template_ids":      []string{"tpl1"},
					"compilation_template_kind_kwd": "knowledge_graph",
					"type_kwd":                      "relation",
					"from_entity_kwd":               from, "to_entity_kwd": to,
					"content_with_weight": fmt.Sprintf(`{"from":%q,"to":%q,"type":"links"}`, from, to),
					"source_doc_ids":      []string{fmt.Sprintf("stored-doc-%d", k)},
					"source_chunk_ids":    []string{fmt.Sprintf("stored-chunk-%d", k)},
				})
			}
			sawExclusion := false
			eng := &fakeEngine{engineType: "infinity", searchFn: func(req *types.SearchRequest) ([]map[string]interface{}, error) {
				if req.Offset+req.Limit > common.MAX_RESULT_WINDOW {
					return nil, fmt.Errorf("Result window is too large, from + size must be <= %d", common.MAX_RESULT_WINDOW)
				}
				matched := make([]map[string]interface{}, 0, len(stored))
				for _, row := range stored {
					if structureFilterMatches(req.Filter, row["id"].(string)) {
						matched = append(matched, row)
					}
				}
				if _, ok := req.Filter["and"]; ok {
					sawExclusion = true
				}
				start, end := req.Offset, req.Offset+req.Limit
				if start > len(matched) {
					start = len(matched)
				}
				if end > len(matched) {
					end = len(matched)
				}
				return matched[start:end], nil
			}}

			buckets := []StructureBucket{bucket}
			stale, err := mergeExistingStructureBuckets(context.Background(), eng, "ragflow_t1", tenant, kb, buckets)
			if err != nil {
				t.Fatalf("mergeExistingStructureBuckets: %v", err)
			}
			if len(stale) != len(wantStale) {
				t.Fatalf("stale ids = %d, want %d (every superseded row)", len(stale), len(wantStale))
			}
			for _, id := range stale {
				if !wantStale[id] {
					t.Fatalf("stale id %q is not a superseded stored row", id)
				}
			}
			if got := len(buckets[0].SourceDocIDs); got != tc.rows+1 {
				t.Fatalf("bucket kept %d docs, want the incoming one plus all %d stored rows", got, tc.rows)
			}
			if got := len(buckets[0].SourceChunkIDs); got != tc.rows+1 {
				t.Fatalf("bucket kept %d chunks, want the incoming one plus all %d stored rows", got, tc.rows)
			}
			// A window-filling read always probes with the ids it already read,
			// which is what tells "exactly the window" from "more rows behind it".
			if !sawExclusion {
				t.Fatalf("no exclusion round: the window was filled but the read never continued past it")
			}
		})
	}
}

// A backend that drops the must_not clause (the SQL engines only implement
// must_not.exists) hands back the same window-sized rows every round. The read
// must stop with a clear error instead of spinning forever.
func TestMergeExistingStructureBucketsStopsWhenTheWindowReadMakesNoProgress(t *testing.T) {
	const tenant, kb = "t1", "kb1"
	const from, to = "entity-a", "target-b"
	buckets := []StructureBucket{{
		Name: from + " -> " + to, Type: "relation", Description: "links",
		FromEntity: from, ToEntity: to, RelationType: "links",
		CompileKwd: "knowledge_graph", TemplateID: "tpl1", TemplateKind: "knowledge_graph",
		SourceDocIDs: []string{"incoming-doc"}, SourceChunkIDs: []string{"incoming-chunk"},
	}}
	stored := make([]map[string]interface{}, 0, common.MAX_RESULT_WINDOW)
	for k := 0; k < common.MAX_RESULT_WINDOW; k++ {
		stored = append(stored, map[string]interface{}{
			"id": fmt.Sprintf("legacy-%d", k), "compile_kwd": "knowledge_graph",
			"compilation_template_ids":      []string{"tpl1"},
			"compilation_template_kind_kwd": "knowledge_graph",
			"type_kwd":                      "relation",
			"from_entity_kwd":               from, "to_entity_kwd": to,
			"content_with_weight": fmt.Sprintf(`{"from":%q,"to":%q,"type":"links"}`, from, to),
			"source_doc_ids":      []string{fmt.Sprintf("stored-doc-%d", k)},
			"source_chunk_ids":    []string{fmt.Sprintf("stored-chunk-%d", k)},
		})
	}
	eng := &fakeEngine{engineType: "infinity", searchFn: func(req *types.SearchRequest) ([]map[string]interface{}, error) {
		// Deliberately ignore the conjoined exclusion: emulate a backend that
		// drops it and keeps handing back the same rows.
		if req.Offset+req.Limit > common.MAX_RESULT_WINDOW {
			return nil, fmt.Errorf("Result window is too large, from + size must be <= %d", common.MAX_RESULT_WINDOW)
		}
		matched := stored
		if raw, ok := req.Filter["id"]; ok {
			wanted := make(map[string]bool)
			for _, id := range raw.([]string) {
				wanted[id] = true
			}
			matched = nil
			for _, row := range stored {
				if wanted[row["id"].(string)] {
					matched = append(matched, row)
				}
			}
		}
		start, end := req.Offset, req.Offset+req.Limit
		if start > len(matched) {
			start = len(matched)
		}
		if end > len(matched) {
			end = len(matched)
		}
		return matched[start:end], nil
	}}

	_, err := mergeExistingStructureBuckets(context.Background(), eng, "ragflow_t1", tenant, kb, buckets)
	if err == nil || !strings.Contains(err.Error(), "keeps returning rows already read") {
		t.Fatalf("mergeExistingStructureBuckets error = %v, want the no-progress guard", err)
	}
}

// The exclusion clauses are the only thing that keeps a continued read from
// re-reading the rows it already has, so pin both their shape — must_not id
// chunks, each within the engine term-count limit — and their polarity once
// conjoined under "and".
func TestRelationExclusionClausesChunksTheReadIDs(t *testing.T) {
	ids := make([]string, relationExclusionChunk*2+7)
	for i := range ids {
		ids[i] = fmt.Sprintf("id-%d", i)
	}
	clauses := relationExclusionClauses(ids)
	if len(clauses) != 3 {
		t.Fatalf("chunks = %d, want 3", len(clauses))
	}
	seen := 0
	for _, child := range clauses {
		mustNot, ok := child["must_not"].(map[string]interface{})
		if !ok {
			t.Fatalf("chunk %#v lacks a must_not clause", child)
		}
		chunk, ok := mustNot["id"].([]string)
		if !ok || len(chunk) == 0 || len(chunk) > relationExclusionChunk {
			t.Fatalf("chunk %#v is not a bounded id list", mustNot)
		}
		seen += len(chunk)
	}
	if seen != len(ids) {
		t.Fatalf("excluded ids = %d, want %d", seen, len(ids))
	}
}

// Nesting the exclusion under a top-level must_not instead of conjoining it
// flips the predicate: the engines read NOT(a AND b) of two negations as a OR b,
// so the round would select the ids it means to exclude. Pin both directions.
func TestStructureFilterMatchesKeepsExclusionPolarity(t *testing.T) {
	ids := []string{"id-a", "id-b"}
	conjoined := map[string]interface{}{"and": relationExclusionClauses(ids)}
	for _, id := range ids {
		if structureFilterMatches(conjoined, id) {
			t.Fatalf("conjoined exclusion selected the already-read id %q", id)
		}
	}
	if !structureFilterMatches(conjoined, "id-c") {
		t.Fatal("conjoined exclusion dropped an unread id")
	}
	negated := map[string]interface{}{"must_not": map[string]interface{}{"and": relationExclusionClauses(ids)}}
	for _, id := range ids {
		if !structureFilterMatches(negated, id) {
			t.Fatalf("a doubly negated exclusion removed the id %q it selects", id)
		}
	}
}
