//go:build integration

package api

import (
	"context"
	"fmt"
	"maps"
	"math"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

type retrievalFixture struct {
	datasetID   string
	documentIDs []string
	chunkIDs    []string
	content     map[string]string
}

func createRESTRetrievalFixture(t *testing.T) retrievalFixture {
	t.Helper()
	response := restCall(t, TestConfig, http.MethodPost, "/datasets", apiObject{"name": uniqueName("retrieval"), "parse_type": 1, "parser_id": "general"}, nil)
	if data, ok := response.JSON["data"].(map[string]any); ok {
		if id, ok := data["id"].(string); ok && id != "" {
			cleanupResource(t, TestConfig, "datasets", id)
		}
	}
	dataset := object(t, restOK(t, response)["data"])
	id, ok := dataset["id"].(string)
	if !ok || id == "" {
		t.Fatal("dataset lacks nonempty id")
	}
	f := retrievalFixture{datasetID: id, content: map[string]string{}}
	for i := range 2 {
		doc := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, "/datasets/"+id+"/documents", apiObject{"name": uniqueName("retrieval") + ".txt"}, apiObject{"type": "empty"}))["data"])
		docID, ok := doc["id"].(string)
		if !ok || docID == "" {
			t.Fatal("document lacks nonempty id")
		}
		f.documentIDs = append(f.documentIDs, docID)
		for j := range 2 {
			content := fmt.Sprintf("Coriander botanical archive records dataset %s document %d entry %d.", id, i, j)
			added := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, "/datasets/"+id+"/documents/"+docID+"/chunks", apiObject{"content": content}, nil))["data"])
			chunk := object(t, added["chunk"])
			chunkID, ok := chunk["id"].(string)
			if !ok || chunkID == "" {
				t.Fatal("chunk lacks nonempty id")
			}
			f.chunkIDs = append(f.chunkIDs, chunkID)
			f.content[chunkID] = content
		}
	}
	waitRESTRetrievalIDs(t, f.request(), f.chunkIDs)
	return f
}

func (f retrievalFixture) request() apiObject {
	return apiObject{"dataset_ids": []string{f.datasetID}, "question": "coriander botanical archive", "similarity_threshold": 0, "vector_similarity_weight": 0, "include_knowledge_compilation": false}
}

func retrievalChunks(t *testing.T, response restResult) (apiObject, []apiObject) {
	t.Helper()
	data := object(t, restOK(t, response)["data"])
	total, ok := data["total"].(float64)
	if !ok || total < 0 || total != math.Trunc(total) {
		t.Fatalf("invalid total: %v", data["total"])
	}
	chunks := array(t, data["chunks"])
	result := make([]apiObject, 0, len(chunks))
	seen := map[string]bool{}
	for _, item := range chunks {
		chunk := object(t, item)
		for _, key := range []string{"id", "content", "document_id", "dataset_id"} {
			if value, ok := chunk[key].(string); !ok || strings.TrimSpace(value) == "" {
				t.Fatalf("invalid chunk %s: %v", key, chunk[key])
			}
		}
		id := chunk["id"].(string)
		if seen[id] {
			t.Fatalf("duplicate chunk %s", id)
		}
		seen[id] = true
		result = append(result, chunk)
	}
	if total < float64(len(chunks)) {
		t.Fatalf("total %v smaller than returned count %d", total, len(chunks))
	}
	return data, result
}

func retrievalIDs(chunks []apiObject) []string {
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		ids = append(ids, chunk["id"].(string))
	}
	slices.Sort(ids)
	return ids
}

// Only index visibility is retried: authentication, transport, and application errors fail immediately.
func waitRESTRetrievalIDs(t *testing.T, body apiObject, expected []string) []apiObject {
	t.Helper()
	want := slices.Sorted(slices.Values(expected))
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, chunks := retrievalChunks(t, restCallContext(t, ctx, TestConfig, http.MethodPost, "/retrieval", body, nil))
		got := retrievalIDs(chunks)
		if slices.Equal(got, want) {
			return chunks
		}
		select {
		case <-ctx.Done():
			t.Fatalf("index visibility timeout: got chunk IDs %v want %v", got, want)
		case <-ticker.C:
		}
	}
}

func assertRESTRetrievalFixture(t *testing.T, f retrievalFixture, chunks []apiObject) {
	t.Helper()
	if !slices.Equal(retrievalIDs(chunks), slices.Sorted(slices.Values(f.chunkIDs))) {
		t.Fatalf("retrieved IDs %v want %v", retrievalIDs(chunks), f.chunkIDs)
	}
	for _, chunk := range chunks {
		if chunk["dataset_id"] != f.datasetID || !slices.Contains(f.documentIDs, chunk["document_id"].(string)) || chunk["content"] != f.content[chunk["id"].(string)] {
			t.Fatalf("chunk escaped fixture or content changed: %v", chunk)
		}
	}
}

func TestRESTRetrievalRoutesAndIsolation(t *testing.T) {
	first := createRESTRetrievalFixture(t)
	second := createRESTRetrievalFixture(t)
	var baselineTotal any
	for _, path := range []string{"/datasets/" + first.datasetID + "/search", "/datasets/search", "/retrieval"} {
		data, chunks := retrievalChunks(t, restCall(t, TestConfig, http.MethodPost, path, first.request(), nil))
		assertRESTRetrievalFixture(t, first, chunks)
		if baselineTotal == nil {
			baselineTotal = data["total"]
		} else if data["total"] != baselineTotal {
			t.Fatalf("search route %s differs from single-dataset contract", path)
		}
	}
	body := first.request()
	body["dataset_ids"] = []string{first.datasetID, second.datasetID}
	_, chunks := retrievalChunks(t, restCall(t, TestConfig, http.MethodPost, "/datasets/search", body, nil))
	want := append(slices.Clone(first.chunkIDs), second.chunkIDs...)
	if !slices.Equal(retrievalIDs(chunks), slices.Sorted(slices.Values(want))) {
		t.Fatal("multi-dataset search did not return both datasets")
	}
	for _, chunk := range chunks {
		fixture := first
		if chunk["dataset_id"] == second.datasetID {
			fixture = second
		}
		if chunk["dataset_id"] != fixture.datasetID || chunk["content"] != fixture.content[chunk["id"].(string)] {
			t.Fatal("multi-dataset result has wrong ownership or content")
		}
	}
}

func TestRESTRetrievalValidation(t *testing.T) {
	body := apiObject{"dataset_ids": []string{TestConfig.InvalidID}, "question": "coriander"}
	for _, path := range []string{"/retrieval", "/datasets/search", "/datasets/" + TestConfig.InvalidID + "/search"} {
		for _, token := range []string{"", "invalid-api-key"} {
			config := *TestConfig
			config.APIKey = token
			restCode(t, restCall(t, &config, http.MethodPost, path, body, nil), 401, 401)
		}
		restCode(t, restCall(t, TestConfig, http.MethodPost, path, body, nil), 200, 404)
	}
	for _, tc := range []struct {
		name string
		body apiObject
	}{
		{"missing datasets", apiObject{"question": "coriander"}},
		{"null datasets", apiObject{"question": "coriander", "dataset_ids": nil}},
		{"empty datasets", apiObject{"question": "coriander", "dataset_ids": []string{}}},
		{"dataset type", apiObject{"question": "coriander", "dataset_ids": "bad"}},
		{"missing question", apiObject{"dataset_ids": []string{TestConfig.InvalidID}}},
		{"blank question", apiObject{"question": "  ", "dataset_ids": []string{TestConfig.InvalidID}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, http.MethodPost, "/retrieval", tc.body, nil), 200, 101)
		})
	}
	for _, tc := range []struct {
		name    string
		updates apiObject
	}{
		{"page zero", apiObject{"page": 0}}, {"page negative", apiObject{"page": -1}}, {"page string", apiObject{"page": "1"}},
		{"page size zero", apiObject{"page_size": 0}}, {"page size string", apiObject{"page_size": "1"}},
		{"highlight string", apiObject{"highlight": "True"}}, {"keyword string", apiObject{"keyword": "False"}},
		{"weight low", apiObject{"vector_similarity_weight": -0.1}}, {"weight high", apiObject{"vector_similarity_weight": 1.1}},
		{"weight string", apiObject{"vector_similarity_weight": "0.5"}}, {"threshold high", apiObject{"similarity_threshold": 1.1}},
		{"top k zero", apiObject{"top_k": 0}}, {"top k high", apiObject{"top_k": 2049}}, {"top k string", apiObject{"top_k": "1"}},
		{"candidates smaller than k", apiObject{"top_k": 2, "knn_num_candidates": 1}}, {"candidates high", apiObject{"knn_num_candidates": 10001}},
		{"document ids type", apiObject{"document_ids": "bad"}}, {"metadata type", apiObject{"metadata_condition": "bad"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := maps.Clone(body)
			maps.Copy(request, tc.updates)
			restCode(t, restCall(t, TestConfig, http.MethodPost, "/retrieval", request, nil), 200, 101)
		})
	}
}

func TestRESTRetrievalPaginationAndOptions(t *testing.T) {
	f := createRESTRetrievalFixture(t)
	var all []string
	for page := 1; page <= 2; page++ {
		body := f.request()
		body["page"] = page
		body["page_size"] = 2
		data, chunks := retrievalChunks(t, restCall(t, TestConfig, http.MethodPost, "/retrieval", body, nil))
		if len(chunks) != 2 || data["total"] != float64(4) {
			t.Fatal("incorrect pagination count or total")
		}
		for _, id := range retrievalIDs(chunks) {
			if slices.Contains(all, id) {
				t.Fatal("pagination repeated a chunk")
			}
			all = append(all, id)
		}
	}
	if !slices.Equal(slices.Sorted(slices.Values(all)), slices.Sorted(slices.Values(f.chunkIDs))) {
		t.Fatal("pagination lost fixture chunks")
	}
	for _, updates := range []apiObject{
		{"page": nil, "page_size": nil, "highlight": nil, "keyword": nil},
		{"highlight": false, "keyword": false},
		{"top_k": 1}, {"top_k": 2048}, {"vector_similarity_weight": 0.5}, {"vector_similarity_weight": 1},
	} {
		body := f.request()
		maps.Copy(body, updates)
		_, chunks := retrievalChunks(t, restCall(t, TestConfig, http.MethodPost, "/retrieval", body, nil))
		if len(chunks) == 0 {
			t.Fatal("valid search options lost all matching chunks")
		}
		for _, chunk := range chunks {
			if chunk["dataset_id"] != f.datasetID || chunk["content"] != f.content[chunk["id"].(string)] || !slices.Contains(f.documentIDs, chunk["document_id"].(string)) {
				t.Fatal("options leaked or corrupted a fixture chunk")
			}
			if _, ok := chunk["highlight"]; ok {
				t.Fatal("highlight appeared when omitted, false, or null")
			}
		}
	}
	body := f.request()
	body["highlight"] = true
	_, chunks := retrievalChunks(t, restCall(t, TestConfig, http.MethodPost, "/retrieval", body, nil))
	assertRESTRetrievalFixture(t, f, chunks)
	for _, chunk := range chunks {
		text, ok := chunk["highlight"].(string)
		if !ok || !strings.Contains(strings.ToLower(text), "coriander") || !strings.Contains(text, "<em>") || !strings.Contains(text, "</em>") {
			t.Fatalf("missing useful highlight: %v", chunk["highlight"])
		}
	}
	for _, tc := range []struct {
		updates apiObject
		message string
	}{
		{apiObject{"rerank_candidates_count": 0}, "must be greater than or equal to page"},
		{apiObject{"rerank_candidates_count": -1}, "must be greater than or equal to page"},
		{apiObject{"page": 3, "page_size": 30}, "must be greater than or equal to page"},
		{apiObject{"rerank_id": uniqueName("missing_rerank")}, "failed to get rerank model"},
	} {
		body := f.request()
		maps.Copy(body, tc.updates)
		result := restCall(t, TestConfig, http.MethodPost, "/retrieval", body, nil)
		restCode(t, result, 200, 102)
		if message, ok := result.JSON["message"].(string); !ok || !strings.Contains(message, tc.message) {
			t.Fatalf("search rejection message=%v want %q", result.JSON["message"], tc.message)
		}
	}
	_, chunks = retrievalChunks(t, restCall(t, TestConfig, http.MethodPost, "/retrieval", f.request(), nil))
	assertRESTRetrievalFixture(t, f, chunks)
	t.Run("keyword_expansion", func(t *testing.T) {
		chat := createRESTChat(t, nil)
		if model, ok := chat["llm_id"].(string); !ok || model == "" {
			t.Skip("keyword expansion requires a configured default chat model")
		}
		body := f.request()
		body["keyword"] = true
		ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
		defer cancel()
		_, chunks := retrievalChunks(t, restCallContext(t, ctx, TestConfig, http.MethodPost, "/retrieval", body, nil))
		assertRESTRetrievalFixture(t, f, chunks)
	})
}

func TestRESTRetrievalDocumentAndMetadataFilters(t *testing.T) {
	f := createRESTRetrievalFixture(t)
	body := f.request()
	body["document_ids"] = []string{f.documentIDs[0]}
	chunks := waitRESTRetrievalIDs(t, body, f.chunkIDs[:2])
	for _, chunk := range chunks {
		if chunk["document_id"] != f.documentIDs[0] {
			t.Fatal("document filter leaked excluded document")
		}
	}
	body["document_ids"] = []string{TestConfig.InvalidID}
	_, chunks = retrievalChunks(t, restCall(t, TestConfig, http.MethodPost, "/retrieval", body, nil))
	if len(chunks) != 0 {
		t.Fatal("unknown document filter broadened search")
	}
	author := uniqueName("author")
	result := object(t, restOK(t, restCall(t, TestConfig, http.MethodPatch, "/datasets/"+f.datasetID+"/documents/metadatas", apiObject{"selector": apiObject{"document_ids": []string{f.documentIDs[0]}}, "updates": []any{apiObject{"key": "author", "value": author}}, "deletes": []any{}}, nil))["data"])
	if result["updated"] != float64(1) || result["matched_docs"] != float64(1) {
		t.Fatalf("metadata update did not target one document: %v", result)
	}
	docs := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/datasets/"+f.datasetID+"/documents", nil, apiObject{"id": f.documentIDs[0]}))["data"])
	items := array(t, docs["docs"])
	if len(items) != 1 {
		t.Fatal("metadata readback did not return selected document")
	}
	meta := object(t, object(t, items[0])["meta_fields"])
	if meta["author"] != author {
		t.Fatalf("metadata did not persist: %v", meta)
	}
	for _, tc := range []struct {
		value string
		ids   []string
	}{{author, f.chunkIDs[:2]}, {uniqueName("no_match"), nil}} {
		body := f.request()
		body["metadata_condition"] = apiObject{"logic": "and", "conditions": []any{apiObject{"name": "author", "comparison_operator": "=", "value": tc.value}}}
		waitRESTRetrievalIDs(t, body, tc.ids)
	}
}

func TestRESTRetrievalConcurrentRequests(t *testing.T) {
	f := createRESTRetrievalFixture(t)
	// Parallel subtests collect every outcome through testing and finish before fixture cleanup.
	for i := range 5 {
		t.Run(fmt.Sprintf("request_%d", i), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			_, chunks := retrievalChunks(t, restCallContext(t, ctx, TestConfig, http.MethodPost, "/retrieval", f.request(), nil))
			assertRESTRetrievalFixture(t, f, chunks)
		})
	}
}

func TestRESTRetrievalChunkDeletionConsistency(t *testing.T) {
	f := createRESTRetrievalFixture(t)
	// Establish all target chunks are searchable before either deletion, so an empty index cannot pass.
	waitRESTRetrievalIDs(t, f.request(), f.chunkIDs)
	path := "/datasets/" + f.datasetID + "/documents/" + f.documentIDs[0] + "/chunks"
	deleted := restOK(t, restCall(t, TestConfig, http.MethodDelete, path, apiObject{"chunk_ids": f.chunkIDs[:1]}, nil))["data"]
	if deleted != float64(1) {
		t.Fatalf("deleted count=%v want 1", deleted)
	}
	waitRESTRetrievalIDs(t, f.request(), f.chunkIDs[1:])
	if count := restOK(t, restCall(t, TestConfig, http.MethodDelete, path, apiObject{"chunk_ids": f.chunkIDs[1:2]}, nil))["data"]; count != float64(1) {
		t.Fatalf("deleted count=%v want 1", count)
	}
	path = "/datasets/" + f.datasetID + "/documents/" + f.documentIDs[1] + "/chunks"
	deleted = restOK(t, restCall(t, TestConfig, http.MethodDelete, path, apiObject{"chunk_ids": f.chunkIDs[2:]}, nil))["data"]
	if deleted != float64(2) {
		t.Fatalf("batch deleted count=%v want 2", deleted)
	}
	waitRESTRetrievalIDs(t, f.request(), nil)
}

func TestRESTRelatedQuestions(t *testing.T) {
	path := "/searchbots/related_questions"
	for _, token := range []string{"", "invalid-api-key"} {
		config := *TestConfig
		config.APIKey = token
		restCode(t, restCall(t, &config, http.MethodPost, path, apiObject{"question": "botanical archives"}, nil), 200, 102)
	}
	restCode(t, restCall(t, TestConfig, http.MethodPost, path, apiObject{}, nil), 200, 101)
	t.Run("generation", func(t *testing.T) {
		chat := createRESTChat(t, nil)
		if model, ok := chat["llm_id"].(string); !ok || model == "" {
			t.Skip("related questions requires a configured default chat model")
		}
		ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
		defer cancel()
		questions := array(t, restOK(t, restCallContext(t, ctx, TestConfig, http.MethodPost, path, apiObject{"question": "How can botanical archives preserve coriander specimens?"}, nil))["data"])
		if len(questions) == 0 {
			t.Fatal("related question generation returned no questions")
		}
		for _, item := range questions {
			question, ok := item.(string)
			if !ok || strings.TrimSpace(question) == "" || strings.Contains(question, "**ERROR**") {
				t.Fatalf("invalid generated question: %v", item)
			}
		}
	})
}
