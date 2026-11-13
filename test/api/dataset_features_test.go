//go:build integration

package api

import (
	"context"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestDatasetMetadataConfigPersistenceAndValidation(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	path := "/datasets/" + dataset["id"].(string) + "/metadata/config"
	empty := apiObject{"enabled": false, "metadata": []any{}, "built_in_metadata": []any{}}
	read := func() apiObject {
		return object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, path, nil, nil))["data"])
	}
	if got := read(); !reflect.DeepEqual(got, empty) {
		t.Fatalf("fresh metadata config=%v want=%v", got, empty)
	}
	fields := []any{
		apiObject{"key": "author", "type": "string", "description": "Author name"},
		apiObject{"key": "tags", "type": "list", "description": "Tag list", "enum": []string{"foo", "bar"}},
	}
	body := apiObject{"metadata": fields, "built_in_metadata": []any{apiObject{"key": "size", "type": "number", "description": "File size"}}}
	expected := apiObject{
		"enabled": false,
		"metadata": []any{
			apiObject{"key": "author", "type": "string", "description": "Author name", "enum": nil},
			apiObject{"key": "tags", "type": "list", "description": "Tag list", "enum": []any{"foo", "bar"}},
		},
		"built_in_metadata": []any{apiObject{"key": "size", "type": "number", "description": "File size", "enum": nil}},
	}
	for _, enabled := range []bool{false, true} {
		if enabled {
			body["enabled"] = true
		}
		expected["enabled"] = enabled
		result := restOK(t, restCall(t, TestConfig, http.MethodPut, path, body, nil))
		if !reflect.DeepEqual(result["data"], expected) || !reflect.DeepEqual(read(), expected) {
			t.Fatalf("metadata config did not persist normalized fields: response=%v stored=%v want=%v", result["data"], read(), expected)
		}
	}
	for _, invalid := range []apiObject{
		{"metadata": []any{apiObject{"key": " ", "type": "string"}}},
		{"metadata": []any{apiObject{"key": "author", "type": "unknown"}}},
		{"metadata": "invalid"},
		{"enabled": "true"},
	} {
		restCode(t, restCall(t, TestConfig, http.MethodPut, path, invalid, nil), 200, 102)
		if got := read(); !reflect.DeepEqual(got, expected) {
			t.Fatalf("invalid metadata update changed config: got=%v want=%v", got, expected)
		}
	}
	restOK(t, restCall(t, TestConfig, http.MethodPut, path, apiObject{}, nil))
	if got := read(); !reflect.DeepEqual(got, empty) {
		t.Fatalf("empty update did not reset metadata config: %v", got)
	}
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		restCode(t, restCall(t, TestConfig, method, "/datasets/00000000000040008000000000000000/metadata/config", apiObject{}, nil), 200, 404)
	}
}

func TestDatasetMetadataSummaryAndFlattenedIdentities(t *testing.T) {
	first := createRESTDataset(t, nil)["id"].(string)
	second := createRESTDataset(t, nil)["id"].(string)
	documents := uploadRESTDocuments(t, TestConfig, first, []documentUpload{
		{Name: "one.txt", Content: []byte("metadata one")},
		{Name: "two.txt", Content: []byte("metadata two")},
		{Name: "three.txt", Content: []byte("metadata three")},
	})
	metadata := []apiObject{
		{"tags": []string{"foo", "bar"}, "author": "alice"},
		{"tags": []string{"foo"}, "author": "bob"},
		{"tags": []string{"bar", "baz"}, "author": ""},
	}
	var ids []string
	for i, doc := range documents {
		id := object(t, doc)["id"].(string)
		ids = append(ids, id)
		restOK(t, restCall(t, TestConfig, http.MethodPatch, "/datasets/"+first+"/documents/"+id, apiObject{"meta_fields": metadata[i]}, nil))
	}
	foreign := object(t, uploadRESTDocuments(t, TestConfig, second, []documentUpload{{Name: "other.txt", Content: []byte("foreign metadata")}})[0])["id"].(string)
	restOK(t, restCall(t, TestConfig, http.MethodPatch, "/datasets/"+second+"/documents/"+foreign, apiObject{"meta_fields": apiObject{"tags": []string{"foreign"}}}, nil))
	// Metadata lives in the document engine. Poll for the precise aggregate,
	// allowing index refresh latency without accepting an empty result.
	for _, tc := range []struct {
		name    string
		query   apiObject
		tags    apiObject
		authors apiObject
	}{
		{"all_documents", nil, apiObject{"foo": float64(2), "bar": float64(2), "baz": float64(1)}, apiObject{"alice": float64(1), "bob": float64(1), "": float64(1)}},
		{"explicit_documents", apiObject{"doc_ids": strings.Join(ids[:2], ",")}, apiObject{"foo": float64(2), "bar": float64(1)}, apiObject{"alice": float64(1), "bob": float64(1)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			wantCounts := tc.tags
			for {
				data := object(t, restOK(t, restCallContext(t, ctx, TestConfig, http.MethodGet, "/datasets/"+first+"/metadata/summary", nil, tc.query))["data"])
				summary := object(t, data["summary"])
				counts := apiObject{}
				if tags, ok := summary["tags"].(apiObject); ok {
					for _, value := range array(t, tags["values"]) {
						pair := array(t, value)
						if len(pair) != 2 {
							t.Fatalf("invalid metadata value/count pair: %v", pair)
						}
						counts[pair[0].(string)] = pair[1]
					}
				}
				if reflect.DeepEqual(counts, wantCounts) {
					authors := object(t, summary["author"])
					got := apiObject{}
					for _, value := range array(t, authors["values"]) {
						pair := array(t, value)
						if len(pair) != 2 {
							t.Fatalf("invalid author value/count: %v", pair)
						}
						got[pair[0].(string)] = pair[1]
					}
					if !reflect.DeepEqual(got, tc.authors) {
						t.Fatalf("author counts differ from stored values: %v", got)
					}
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("metadata summary never matched owned documents: got=%v want=%v", counts, wantCounts)
				case <-time.After(100 * time.Millisecond):
				}
			}
		})
	}
	for _, tc := range []struct {
		datasetIDs string
		expected   map[string][]string
	}{
		{first, map[string][]string{"foo": {ids[0], ids[1]}, "bar": {ids[0], ids[2]}, "baz": {ids[2]}}},
		{first + "," + second, map[string][]string{"foo": {ids[0], ids[1]}, "bar": {ids[0], ids[2]}, "baz": {ids[2]}, "foreign": {foreign}}},
	} {
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/metadata/flattened", nil, apiObject{"dataset_ids": tc.datasetIDs}))["data"])
		tags := object(t, data["tags"])
		if len(tags) != len(tc.expected) {
			t.Fatalf("flattened tags contain unexpected values: %v", tags)
		}
		for value, expectedIDs := range tc.expected {
			var actual []string
			for _, id := range array(t, tags[value]) {
				actual = append(actual, id.(string))
			}
			slices.Sort(actual)
			slices.Sort(expectedIDs)
			if !slices.Equal(actual, expectedIDs) {
				t.Errorf("tag %s documents=%v want=%v", value, actual, expectedIDs)
			}
		}
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/metadata/flattened", nil, nil), 200, 102)
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/metadata/flattened", nil, apiObject{"dataset_ids": "invalid"}), 200, 404)
}

func TestDatasetFreshGraphTagsAndIngestionState(t *testing.T) {
	id := createRESTDataset(t, nil)["id"].(string)
	graph := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/graph", nil, nil))["data"])
	for _, key := range []string{"graph", "mind_map"} {
		if len(object(t, graph[key])) != 0 {
			t.Fatalf("fresh dataset has %s entries: %v", key, graph)
		}
	}
	tags := restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/tags/aggregation", nil, apiObject{"dataset_ids": id}))["data"]
	if len(array(t, tags)) != 0 {
		t.Fatalf("fresh dataset has tags: %v", tags)
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/tags/aggregation", nil, nil), 200, 102)
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/tags/aggregation", nil, apiObject{"dataset_ids": "invalid"}), 200, 102)
	summary := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/ingestions/summary", nil, nil))["data"])
	for _, key := range []string{"doc_num", "chunk_num", "token_num"} {
		if summary[key] != float64(0) {
			t.Errorf("fresh dataset %s=%v", key, summary[key])
		}
	}
	for _, key := range []string{"status", "download_status"} {
		counts := object(t, summary[key])
		expectedKeys := []string{"running_count", "done_count", "fail_count"}
		if key == "status" {
			expectedKeys = append(expectedKeys, "unstart_count", "cancel_count")
		}
		for _, field := range expectedKeys {
			if counts[field] != float64(0) {
				t.Errorf("fresh %s.%s=%v", key, field, counts[field])
			}
		}
	}
	for _, query := range []apiObject{
		nil,
		{"log_type": "file", "keywords": uniqueName("missing"), "page": 1, "page_size": 2},
		{"create_date_from": "2999-12-31", "create_date_to": "2000-01-01"},
	} {
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/ingestions", nil, query))["data"])
		if data["total"] != float64(0) || len(array(t, data["logs"])) != 0 {
			t.Fatalf("fresh dataset logs=%v", data)
		}
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/ingestions/nonexistent", nil, nil), 200, 102)
	for _, query := range []apiObject{{"page": "invalid"}, {"page_size": "invalid"}} {
		restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/ingestions", nil, query), 200, 101)
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/ingestions", nil, apiObject{"log_type": "unknown"}), 200, 102)
	for _, suffix := range []string{"/ingestions/summary", "/ingestions", "/ingestions/nonexistent", "/graph", "/compilation/status"} {
		restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/00000000000040008000000000000000"+suffix, nil, nil), 200, 404)
	}
}

func TestDatasetCompilationStatusAndEmbeddingRequirements(t *testing.T) {
	id := createRESTDataset(t, nil)["id"].(string)
	// Compilation status is the supported replacement for the retired /index routes.
	for _, kind := range []string{"", "graph", "raptor", "mindmap", "wiki"} {
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/compilation/status", nil, apiObject{"kind": kind}))["data"])
		for field, expected := range (apiObject{"kind": kind, "state": "idle", "progress": float64(0), "inflight": float64(0), "backlog": float64(0)}) {
			if data[field] != expected {
				t.Errorf("fresh compilation status %s=%v want=%v", field, data[field], expected)
			}
		}
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+id+"/compilation/status", nil, apiObject{"kind": "invalid"}), 200, 102)
	result := restCall(t, TestConfig, http.MethodPost, "/datasets/"+id+"/embedding/check", apiObject{}, nil)
	restCode(t, result, 200, 102)
	if message, _ := result.JSON["message"].(string); !strings.Contains(strings.ToLower(message), "embedding") || !strings.Contains(message, "required") {
		t.Fatalf("missing embedding error does not identify required model: %v", result.JSON)
	}
}

func TestDatasetSearchParameterValidation(t *testing.T) {
	id := createRESTDataset(t, nil)["id"].(string)
	// Existing retrieval_api_test.go verifies nonempty matching chunks,
	// document isolation and parity with this dataset-specific search route.
	for _, tc := range []struct {
		name   string
		fields apiObject
	}{
		{"missing_question", apiObject{"question": nil}},
		{"blank_question", apiObject{"question": " "}},
		{"typed_question", apiObject{"question": 1}},
		{"zero_page", apiObject{"page": 0}},
		{"negative_page_size", apiObject{"page_size": -1}},
		{"zero_size", apiObject{"size": 0}},
		{"low_top_k", apiObject{"knn_top_k": 0}},
		{"high_top_k", apiObject{"knn_top_k": 2049}},
		{"low_candidates", apiObject{"knn_num_candidates": 0}},
		{"high_candidates", apiObject{"knn_num_candidates": 10001}},
		{"candidates_below_top_k", apiObject{"knn_top_k": 5, "knn_num_candidates": 4}},
		{"low_similarity", apiObject{"similarity_threshold": -0.01}},
		{"high_similarity", apiObject{"similarity_threshold": 1.01}},
		{"low_vector_weight", apiObject{"vector_similarity_weight": -0.01}},
		{"high_vector_weight", apiObject{"vector_similarity_weight": 1.01}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := apiObject{"question": "contract validation"}
			for key, value := range tc.fields {
				body[key] = value
			}
			restCode(t, restCall(t, TestConfig, http.MethodPost, "/datasets/"+id+"/search", body, nil), 200, 101)
		})
	}
}
