//go:build integration

package api

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func documentRESTPath(datasetID string) string { return "/datasets/" + datasetID + "/documents" }

func seedRESTDocuments(t *testing.T, count int) (string, []apiObject) {
	t.Helper()
	dataset := createRESTDataset(t, apiObject{"parse_type": 1, "parser_id": "general"})
	id := dataset["id"].(string)
	files := make([]documentUpload, count)
	for i := range count {
		files[i] = documentUpload{Name: fmt.Sprintf("record_%02d.txt", i), Content: []byte(fmt.Sprintf("Botanical evidence record %d contains coriander leaves and seeds.\n", i))}
	}
	uploaded := uploadRESTDocuments(t, TestConfig, id, files)
	if len(uploaded) != count {
		t.Fatalf("uploaded=%d want=%d", len(uploaded), count)
	}
	docs := make([]apiObject, count)
	for i, item := range uploaded {
		docs[i] = object(t, item)
	}
	return id, docs
}

func listRESTDocuments(t *testing.T, datasetID string, query apiObject) (apiObject, []apiObject) {
	t.Helper()
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, documentRESTPath(datasetID), nil, query))["data"])
	items := array(t, data["docs"])
	docs := make([]apiObject, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		doc := object(t, item)
		id, ok := doc["id"].(string)
		if !ok || id == "" || seen[id] || doc["dataset_id"] != datasetID {
			t.Fatalf("invalid document identity: %v", doc)
		}
		seen[id] = true
		docs = append(docs, doc)
	}
	if total, ok := data["total"].(float64); !ok || total < float64(len(docs)) {
		t.Fatalf("invalid total: %v", data["total"])
	}
	return data, docs
}

func getRESTDocument(t *testing.T, datasetID, documentID string) apiObject {
	t.Helper()
	data, docs := listRESTDocuments(t, datasetID, apiObject{"id": documentID})
	if data["total"] != float64(1) || len(docs) != 1 || docs[0]["id"] != documentID {
		t.Fatalf("document %s not uniquely retrievable: %v", documentID, data)
	}
	return docs[0]
}

func documentRESTIDs(docs []apiObject) []string {
	ids := make([]string, len(docs))
	for i, doc := range docs {
		ids[i] = doc["id"].(string)
	}
	return ids
}

func requireRESTDocumentIDs(t *testing.T, docs []apiObject, expected []string) {
	t.Helper()
	got := documentRESTIDs(docs)
	slices.Sort(got)
	want := slices.Clone(expected)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("document IDs=%v want=%v", got, want)
	}
}

func TestRESTDocumentUploadPersistenceAndNames(t *testing.T) {
	dataset := createRESTDataset(t, apiObject{"parse_type": 1, "parser_id": "general"})
	id := dataset["id"].(string)
	files := []documentUpload{
		{Name: "same.txt", Content: []byte("first content")},
		{Name: "same.txt", Content: []byte("second content is longer")},
		{Name: "empty.txt"},
		{Name: strings.Repeat("a", 251) + ".txt", Content: []byte("maximum byte length")},
		{Name: "notes.md", Content: []byte("# Notes\nCoriander evidence.")},
		{Name: "record.json", Content: []byte(`{"plant":"coriander"}`)},
		{Name: "page.html", Content: []byte("<!doctype html><title>Evidence</title><p>Coriander</p>")},
		{Name: "mail.eml", Content: []byte("From: botanist@example.invalid\r\nSubject: Evidence\r\n\r\nCoriander seeds.")},
	}
	uploaded := uploadRESTDocuments(t, TestConfig, id, files)
	if len(uploaded) != len(files) {
		t.Fatalf("uploaded=%d want=%d", len(uploaded), len(files))
	}
	for i, item := range uploaded {
		doc := object(t, item)
		expectedName := files[i].Name
		if i == 1 {
			expectedName = "same(1).txt"
		}
		if doc["name"] != expectedName || doc["dataset_id"] != id || doc["size"] != float64(len(files[i].Content)) {
			t.Fatalf("upload metadata mismatch: %v", doc)
		}
		persisted := getRESTDocument(t, id, doc["id"].(string))
		for _, key := range []string{"id", "name", "dataset_id", "size"} {
			if persisted[key] != doc[key] {
				t.Fatalf("persisted %s=%v upload=%v", key, persisted[key], doc[key])
			}
		}
		if persisted["ingestion_status"] != "UNSTART" {
			t.Fatalf("upload unexpectedly started ingestion: %v", persisted)
		}
	}
	repeated := object(t, uploadRESTDocuments(t, TestConfig, id, files[:1])[0])
	if repeated["name"] != "same(2).txt" {
		t.Fatalf("repeated upload name=%v", repeated["name"])
	}
	// Unique row names alone cannot prove duplicate uploads kept separate blobs.
	for i := range 2 {
		doc := object(t, uploaded[i])
		file := files[i]
		file.Name = doc["name"].(string)
		requireDocumentDownload(t, rawRESTCall(t, TestConfig, http.MethodGet, documentRESTPath(id)+"/"+doc["id"].(string), nil, ""), file)
	}
	repeatedFile := files[0]
	repeatedFile.Name = repeated["name"].(string)
	requireDocumentDownload(t, rawRESTCall(t, TestConfig, http.MethodGet, documentRESTPath(id)+"/"+repeated["id"].(string), nil, ""), repeatedFile)
	data, docs := listRESTDocuments(t, id, apiObject{"page_size": 100})
	if data["total"] != float64(len(files)+1) || len(docs) != len(files)+1 {
		t.Fatalf("persisted upload total=%v", data)
	}
}

func TestRESTDocumentUploadValidation(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	path := documentRESTPath(id)
	for _, tc := range []struct {
		name    string
		files   []documentUpload
		code    int
		message string
	}{
		{"missing_file", nil, 101, "No file part!"},
		{"empty_filename", []documentUpload{{Name: "", Content: []byte("x")}}, 101, "No file part!"},
		{"overlong_filename", []documentUpload{{Name: strings.Repeat("a", 252) + ".txt"}}, 101, "255 bytes"},
		{"multibyte_overlong_filename", []documentUpload{{Name: strings.Repeat("界", 84) + ".txt"}}, 101, "255 bytes"},
		{"unsupported_format", []documentUpload{{Name: "executable.exe", Content: []byte("not executable")}}, 500, "not been supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := multipartRESTDocuments(t, TestConfig, id, tc.files)
			restCode(t, r, 200, tc.code)
			if !strings.Contains(fmt.Sprint(r.JSON["message"]), tc.message) {
				t.Fatalf("message=%v want substring %q", r.JSON["message"], tc.message)
			}
			data, docs := listRESTDocuments(t, id, nil)
			if data["total"] != float64(0) || len(docs) != 0 {
				t.Fatalf("failed upload persisted documents: %v", data)
			}
		})
	}
	restCode(t, rawRESTCall(t, TestConfig, http.MethodPost, path, []byte("--broken\r\n"), "multipart/form-data; boundary=broken"), 200, 101)
	restCode(t, multipartRESTDocuments(t, TestConfig, TestConfig.InvalidID, []documentUpload{{Name: "valid.txt", Content: []byte("x")}}), 200, 102)
	// Mixed uploads explicitly report the rejected file while retaining the valid file.
	partial := multipartRESTDocuments(t, TestConfig, id, []documentUpload{{Name: "valid.txt", Content: []byte("valid content")}, {Name: "invalid.exe"}})
	restCode(t, partial, 200, 500)
	accepted := array(t, partial.JSON["data"])
	if len(accepted) != 1 || object(t, accepted[0])["name"] != "valid.txt" || !strings.Contains(fmt.Sprint(partial.JSON["message"]), "invalid.exe") {
		t.Fatalf("partial upload=%v", partial.JSON)
	}
	data, docs := listRESTDocuments(t, id, nil)
	if data["total"] != float64(1) {
		t.Fatalf("partial upload total=%v", data)
	}
	requireRESTDocumentIDs(t, docs, []string{object(t, accepted[0])["id"].(string)})
}

func TestRESTDocumentListingFiltersPaginationAndReads(t *testing.T) {
	id, uploaded := seedRESTDocuments(t, 3)
	ids := documentRESTIDs(uploaded)
	cases := []struct {
		name     string
		query    apiObject
		expected []string
		total    int
	}{
		{"all", nil, ids, 3},
		{"first_page", apiObject{"orderby": "name", "desc": false, "page": 1, "page_size": 2}, ids[:2], 3},
		{"last_page", apiObject{"orderby": "name", "desc": false, "page": 2, "page_size": 2}, ids[2:], 3},
		{"past_end", apiObject{"page": 3, "page_size": 2}, nil, 3},
		{"keyword", apiObject{"keywords": "record_01"}, ids[1:2], 1},
		{"keyword_no_match", apiObject{"keywords": "does_not_exist"}, nil, 0},
		{"id", apiObject{"id": ids[0]}, ids[:1], 1},
		{"name", apiObject{"name": uploaded[1]["name"]}, ids[1:2], 1},
		{"mismatched_existing_id_name", apiObject{"id": ids[0], "name": uploaded[1]["name"]}, nil, 0},
		{"unstarted", apiObject{"run": "UNSTART"}, ids, 3},
		{"future_creation", apiObject{"create_time_from": "9999999999000"}, nil, 3},
		{"past_creation", apiObject{"create_time_to": "1"}, nil, 3},
		{"invalid_page_defaults", apiObject{"page": "invalid", "page_size": -1}, ids, 3},
		{"unknown_query_ignored", apiObject{"unexpected": "ignored"}, ids, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, docs := listRESTDocuments(t, id, tc.query)
			if data["total"] != float64(tc.total) {
				t.Fatalf("total=%v want=%d", data["total"], tc.total)
			}
			requireRESTDocumentIDs(t, docs, tc.expected)
		})
	}
	for _, order := range []string{"name", "create_time", "update_time"} {
		for _, desc := range []bool{false, true} {
			_, docs := listRESTDocuments(t, id, apiObject{"orderby": order, "desc": desc})
			for i := 1; i < len(docs); i++ {
				less := false
				if order == "name" {
					less = docs[i-1][order].(string) > docs[i][order].(string)
				} else {
					less = docs[i-1][order].(float64) > docs[i][order].(float64)
				}
				equal := docs[i-1][order] == docs[i][order]
				if !equal && less != desc {
					t.Fatalf("incorrect order field=%s desc=%v documents=%v", order, desc, docs)
				}
			}
		}
	}
	for _, tc := range []struct {
		query apiObject
		code  int
	}{
		{apiObject{"orderby": "name; DROP TABLE document"}, 101},
		{apiObject{"metadata_condition": "{bad"}, 102},
		{apiObject{"metadata_condition": "[]"}, 102},
		{apiObject{"run": "DONE"}, 102},
		{apiObject{"id": "missing-document"}, 102},
		{apiObject{"name": "missing.txt"}, 102},
		{apiObject{"create_time_from": "invalid"}, 102},
		{apiObject{"create_time_to": "invalid"}, 102},
		{apiObject{"id": ids[0], "ids": ids[1]}, 102},
	} {
		restCode(t, restCall(t, TestConfig, http.MethodGet, documentRESTPath(id), nil, tc.query), 200, tc.code)
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, documentRESTPath(TestConfig.InvalidID), nil, nil), 200, 404)
	t.Run("concurrent_reads", func(t *testing.T) {
		for i := range 4 {
			t.Run(fmt.Sprint(i), func(t *testing.T) {
				t.Parallel()
				data, docs := listRESTDocuments(t, id, nil)
				if data["total"] != float64(3) {
					t.Fatalf("total=%v", data["total"])
				}
				requireRESTDocumentIDs(t, docs, ids)
			})
		}
	})
}

func TestRESTDocumentUpdatesPersistAndRejectAtomically(t *testing.T) {
	id, docs := seedRESTDocuments(t, 2)
	docID := docs[0]["id"].(string)
	path := documentRESTPath(id) + "/" + docID
	for _, name := range []string{"renamed.txt", strings.Repeat("a", 251) + ".txt", "UPPER.TXT"} {
		restOK(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"name": name}, nil))
		if got := getRESTDocument(t, id, docID)["name"]; got != name {
			t.Fatalf("name=%v want=%s", got, name)
		}
	}
	for _, tc := range []struct {
		name string
		body apiObject
		code int
	}{
		{"null_name", apiObject{"name": nil}, 102}, {"numeric_name", apiObject{"name": 1}, 102},
		{"empty_name", apiObject{"name": ""}, 101}, {"extension_change", apiObject{"name": "renamed.pdf"}, 101},
		{"too_long", apiObject{"name": strings.Repeat("a", 252) + ".txt"}, 101},
		{"duplicate", apiObject{"name": docs[1]["name"]}, 102},
		{"chunk_counter", apiObject{"name": "must_not_persist.txt", "chunk_count": 7}, 102},
		{"token_counter", apiObject{"token_count": 5}, 102}, {"progress", apiObject{"progress": 1}, 102},
		{"invalid_enabled", apiObject{"enabled": 2}, 102}, {"null_enabled", apiObject{"enabled": nil}, 102},
		{"meta_not_object", apiObject{"meta_fields": []any{}}, 102},
		{"nested_meta", apiObject{"meta_fields": apiObject{"nested": apiObject{"x": 1}}}, 102},
		{"nested_meta_list", apiObject{"meta_fields": apiObject{"nested": []any{apiObject{"x": 1}}}}, 102},
		{"invalid_chunk_method", apiObject{"chunk_method": "unknown"}, 102}, {"empty_chunk_method", apiObject{"chunk_method": ""}, 102},
		{"parser_not_object", apiObject{"parser_config": []any{}}, 102},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := getRESTDocument(t, id, docID)
			restCode(t, restCall(t, TestConfig, http.MethodPatch, path, tc.body, nil), 200, tc.code)
			after := getRESTDocument(t, id, docID)
			for _, key := range []string{"id", "dataset_id", "name", "size", "chunk_count", "token_count", "progress", "parser_config", "meta_fields", "status"} {
				if !reflect.DeepEqual(before[key], after[key]) {
					t.Fatalf("rejected update changed %s: before=%v after=%v", key, before[key], after[key])
				}
			}
		})
	}
	for _, method := range []string{"naive", "manual", "qa", "table", "paper", "book", "laws", "presentation", "picture", "one", "knowledge_graph", "email", "tag"} {
		restOK(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"chunk_method": method}, nil))
		persisted := getRESTDocument(t, id, docID)
		if persisted["chunk_method"] != method || persisted["parser_id"] != method {
			t.Fatalf("parser change not persisted: %v", persisted)
		}
	}
	restOK(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"enabled": 0}, nil))
	if getRESTDocument(t, id, docID)["status"] != "0" {
		t.Fatal("disable not persisted")
	}
	restOK(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"enabled": 1}, nil))
	if getRESTDocument(t, id, docID)["status"] != "1" {
		t.Fatal("enable not persisted")
	}
	before := getRESTDocument(t, id, docID)
	restOK(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"unexpected": "ignored", "id": "forged", "dataset_id": "forged", "size": 1}, nil))
	after := getRESTDocument(t, id, docID)
	for _, key := range []string{"id", "dataset_id", "size", "name", "parser_id"} {
		if before[key] != after[key] {
			t.Fatalf("unsupported field changed %s", key)
		}
	}
	otherID, _ := seedRESTDocuments(t, 1)
	restCode(t, restCall(t, TestConfig, http.MethodPatch, documentRESTPath(otherID)+"/"+docID, apiObject{"name": "foreign.txt"}, nil), 200, 102)
	restCode(t, restCall(t, TestConfig, http.MethodPatch, documentRESTPath(id)+"/"+TestConfig.InvalidID, apiObject{"name": "missing.txt"}, nil), 200, 102)
	restCode(t, restCall(t, TestConfig, http.MethodPatch, documentRESTPath(TestConfig.InvalidID)+"/"+docID, apiObject{"name": "missing.txt"}, nil), 200, 404)
	if getRESTDocument(t, id, docID)["name"] != before["name"] {
		t.Fatal("cross-dataset update renamed original")
	}
}

func TestRESTDocumentParserConfigurationAndMetadataConfig(t *testing.T) {
	id, docs := seedRESTDocuments(t, 1)
	docID := docs[0]["id"].(string)
	path := documentRESTPath(id) + "/" + docID
	original := getRESTDocument(t, id, docID)
	config := object(t, original["parser_config"])
	var component string
	for key, value := range config {
		if strings.Contains(key, "Chunker:") {
			if _, ok := value.(map[string]any); ok {
				component = key
				break
			}
		}
	}
	if component == "" {
		t.Fatalf("general parser has no component-scoped chunker: %v", config)
	}
	restOK(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"parser_config": apiObject{component: apiObject{"chunk_token_size": 128}}}, nil))
	persisted := getRESTDocument(t, id, docID)
	if object(t, object(t, persisted["parser_config"])[component])["chunk_token_size"] != float64(128) {
		t.Fatalf("component config not persisted: %v", persisted["parser_config"])
	}
	if persisted["name"] != original["name"] || persisted["size"] != original["size"] {
		t.Fatal("parser update changed unrelated fields")
	}
	for node := range config {
		if !strings.HasPrefix(node, "Parser:") {
			continue
		}
		for _, pages := range []any{"not-an-array", []any{[]any{0, 1}}, []any{[]any{3, 1}}, []any{[]any{1, "bad"}}} {
			before := getRESTDocument(t, id, docID)
			restCode(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"parser_config": apiObject{node: apiObject{"pdf": apiObject{"pages": pages}}}}, nil), 200, 102)
			after := getRESTDocument(t, id, docID)
			if !reflect.DeepEqual(before["parser_config"], after["parser_config"]) {
				t.Fatalf("invalid page ranges changed parser config: %v", after)
			}
		}
	}
	// Flat Python-era options are discarded by the component-scoped Go contract.
	restOK(t, restCall(t, TestConfig, http.MethodPatch, path, apiObject{"parser_config": apiObject{"chunk_token_num": -1, "raptor": apiObject{"use_raptor": true}}}, nil))
	persisted = getRESTDocument(t, id, docID)
	pc := object(t, persisted["parser_config"])
	for _, key := range []string{"chunk_token_num", "raptor"} {
		if _, ok := pc[key]; ok {
			t.Fatalf("flat parser key retained: %s", key)
		}
	}
	if object(t, pc[component])["chunk_token_size"] != object(t, config[component])["chunk_token_size"] {
		t.Fatalf("flat options did not restore component defaults: got=%v defaults=%v", pc[component], config[component])
	}
	metadata := apiObject{"author": "alice", "tags": []any{"one", "two"}}
	metadataPath := path + "/metadata/config"
	restCode(t, restCall(t, TestConfig, http.MethodPut, metadataPath, apiObject{}, nil), 200, 101)
	restOK(t, restCall(t, TestConfig, http.MethodPut, metadataPath, apiObject{"metadata": metadata}, nil))
	pc = object(t, getRESTDocument(t, id, docID)["parser_config"])
	if !reflect.DeepEqual(object(t, pc["Extractor:AutoExtractDefault"])["metadata"], metadata) {
		t.Fatalf("extractor metadata not persisted: %v", pc)
	}
	restCode(t, restCall(t, TestConfig, http.MethodPut, documentRESTPath(id)+"/"+TestConfig.InvalidID+"/metadata/config", apiObject{"metadata": metadata}, nil), 200, 102)
}

func waitRESTDocumentMetadata(t *testing.T, datasetID, documentID string, want apiObject) apiObject {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		r := restCallContext(t, ctx, TestConfig, http.MethodGet, documentRESTPath(datasetID), nil, apiObject{"id": documentID})
		data := object(t, restOK(t, r)["data"])
		docs := array(t, data["docs"])
		if len(docs) != 1 {
			t.Fatalf("metadata document missing: %v", data)
		}
		doc := object(t, docs[0])
		if reflect.DeepEqual(doc["meta_fields"], want) {
			return doc
		}
		select {
		case <-ctx.Done():
			t.Fatalf("metadata did not persist: got=%v want=%v", doc["meta_fields"], want)
		case <-ticker.C:
		}
	}
}

func TestRESTDocumentMetadataReplacementAndBatchUpdates(t *testing.T) {
	id, docs := seedRESTDocuments(t, 3)
	ids := documentRESTIDs(docs)
	path := documentRESTPath(id)
	initial := apiObject{"author": "alice", "tags": []any{"one", "two"}, "year": float64(2026)}
	restOK(t, restCall(t, TestConfig, http.MethodPatch, path+"/"+ids[0], apiObject{"meta_fields": initial}, nil))
	waitRESTDocumentMetadata(t, id, ids[0], initial)
	for _, empty := range []any{nil, apiObject{}} {
		restOK(t, restCall(t, TestConfig, http.MethodPatch, path+"/"+ids[0], apiObject{"meta_fields": empty}, nil))
		waitRESTDocumentMetadata(t, id, ids[0], apiObject{})
	}
	replacement := apiObject{"author": "bob"}
	restOK(t, restCall(t, TestConfig, http.MethodPatch, path+"/"+ids[0], apiObject{"meta_fields": replacement}, nil))
	waitRESTDocumentMetadata(t, id, ids[0], replacement)
	batch := func(selector apiObject, updates, deletes []any, matched, updated int) {
		t.Helper()
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodPatch, path+"/metadatas", apiObject{"selector": selector, "updates": updates, "deletes": deletes}, nil))["data"])
		if data["matched_docs"] != float64(matched) || data["updated"] != float64(updated) {
			t.Fatalf("batch counts=%v want matched=%d updated=%d", data, matched, updated)
		}
	}
	batch(apiObject{"document_ids": ids[:2]}, []any{apiObject{"key": "status", "value": "reviewed"}}, nil, 2, 2)
	waitRESTDocumentMetadata(t, id, ids[0], apiObject{"author": "bob", "status": "reviewed"})
	waitRESTDocumentMetadata(t, id, ids[1], apiObject{"status": "reviewed"})
	waitRESTDocumentMetadata(t, id, ids[2], apiObject{})
	batch(apiObject{"document_ids": ids[:2]}, []any{apiObject{"key": "status", "value": "reviewed"}}, nil, 2, 0)
	batch(apiObject{"document_ids": ids, "metadata_condition": apiObject{"conditions": []any{apiObject{"comparison_operator": "is", "name": "author", "value": "bob"}}}}, []any{apiObject{"key": "selected", "value": "yes"}}, []any{apiObject{"key": "status"}}, 1, 1)
	waitRESTDocumentMetadata(t, id, ids[0], apiObject{"author": "bob", "selected": "yes"})
	waitRESTDocumentMetadata(t, id, ids[1], apiObject{"status": "reviewed"})
	batch(apiObject{"document_ids": []string{}}, []any{apiObject{"key": "forbidden", "value": "no"}}, nil, 0, 0)
	batch(apiObject{"document_ids": ids, "metadata_condition": apiObject{"conditions": []any{apiObject{"comparison_operator": "is", "name": "missing", "value": "never"}}}}, []any{apiObject{"key": "forbidden", "value": "no"}}, nil, 0, 0)
	for _, body := range []apiObject{
		{"selector": []any{1}}, {"selector": apiObject{"document_ids": "not-list"}}, {"selector": apiObject{"document_ids": []any{1}}},
		{"selector": apiObject{"metadata_condition": []any{1}}}, {"updates": apiObject{"key": "x"}}, {"deletes": []any{apiObject{"x": "y"}}},
		{"updates": []any{apiObject{"key": ""}}}, {"updates": []any{apiObject{"key": "missing-value"}}},
		{"selector": apiObject{"document_ids": []string{ids[0], TestConfig.InvalidID}}, "updates": []any{apiObject{"key": "author", "value": "must_not_persist"}}},
	} {
		restCode(t, restCall(t, TestConfig, http.MethodPatch, path+"/metadatas", body, nil), 200, 102)
	}
	waitRESTDocumentMetadata(t, id, ids[0], apiObject{"author": "bob", "selected": "yes"})
	_, filtered := listRESTDocuments(t, id, apiObject{"metadata": "{\"author\":[\"bob\"]}"})
	requireRESTDocumentIDs(t, filtered, ids[:1])
	batch(apiObject{"document_ids": ids[1:2]}, []any{apiObject{"key": "tags", "value": []any{"one"}}}, nil, 1, 1)
	batch(apiObject{"document_ids": ids[1:2]}, []any{apiObject{"key": "tags", "value": []any{"one", "two"}}}, nil, 1, 1)
	waitRESTDocumentMetadata(t, id, ids[1], apiObject{"status": "reviewed", "tags": []any{"one", "two"}})
	batch(apiObject{"document_ids": ids[1:2]}, []any{apiObject{"key": "tags", "value": []any{"one", "two"}}}, nil, 1, 0)
}

func TestRESTDocumentAuthenticationCannotMutate(t *testing.T) {
	id, docs := seedRESTDocuments(t, 1)
	docID := docs[0]["id"].(string)
	path := documentRESTPath(id)
	before := getRESTDocument(t, id, docID)
	for _, token := range []string{"", TestConfig.InvalidAPIKey} {
		c := *TestConfig
		c.APIKey = token
		c.AuthToken = ""
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, path, nil}, {http.MethodPatch, path + "/" + docID, apiObject{"name": "forbidden.txt"}},
			{http.MethodDelete, path, apiObject{"ids": []string{docID}}},
			{http.MethodPost, path + "/parse", apiObject{"document_ids": []string{docID}}},
			{http.MethodPost, path + "/stop", apiObject{"document_ids": []string{docID}}},
			{http.MethodPatch, path + "/metadatas", apiObject{"selector": apiObject{"document_ids": []string{docID}}, "updates": []any{apiObject{"key": "forbidden", "value": "yes"}}}},
			{http.MethodPut, path + "/" + docID + "/metadata/config", apiObject{"metadata": apiObject{"forbidden": "yes"}}},
		} {
			restCode(t, restCall(t, &c, tc.method, tc.path, tc.body, nil), 401, 401)
		}
		restCode(t, multipartRESTDocuments(t, &c, id, []documentUpload{{Name: "forbidden.txt", Content: []byte("unauthorized")}}), 401, 401)
	}
	data, remaining := listRESTDocuments(t, id, nil)
	if data["total"] != float64(1) {
		t.Fatalf("unauthorized upload mutated total: %v", data)
	}
	requireRESTDocumentIDs(t, remaining, []string{docID})
	after := getRESTDocument(t, id, docID)
	for _, key := range []string{"name", "parser_config", "meta_fields", "ingestion_status"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			t.Fatalf("unauthorized mutation changed %s", key)
		}
	}
}

func TestRESTDocumentDeletionValidationIsolationAndAbsence(t *testing.T) {
	id, docs := seedRESTDocuments(t, 3)
	ids := documentRESTIDs(docs)
	otherID, otherDocs := seedRESTDocuments(t, 1)
	otherDocID := otherDocs[0]["id"].(string)
	path := documentRESTPath(id)
	for _, tc := range []struct {
		body any
		code int
	}{
		{apiObject{}, 102}, {apiObject{"ids": []string{}}, 102}, {apiObject{"ids": "wrong"}, 102},
		{apiObject{"ids": []string{ids[0], ids[0]}}, 101}, {apiObject{"ids": []string{ids[0]}, "delete_all": true}, 102},
		{apiObject{"ids": []string{ids[0], TestConfig.InvalidID}}, 102},
		{apiObject{"ids": []string{TestConfig.InvalidID, ids[0]}}, 102},
		{apiObject{"ids": []string{ids[0], otherDocID}}, 102}, {"not an object", 101},
	} {
		restCode(t, restCall(t, TestConfig, http.MethodDelete, path, tc.body, nil), 200, tc.code)
		data, remaining := listRESTDocuments(t, id, nil)
		if data["total"] != float64(3) {
			t.Fatalf("rejected delete changed count: %v", data)
		}
		requireRESTDocumentIDs(t, remaining, ids)
		getRESTDocument(t, otherID, otherDocID)
	}
	restCode(t, restCall(t, TestConfig, http.MethodDelete, documentRESTPath(TestConfig.InvalidID), apiObject{"ids": ids}, nil), 200, 404)
	for _, target := range [][]string{ids[:1], ids[1:]} {
		data := object(t, restOK(t, restCall(t, TestConfig, http.MethodDelete, path, apiObject{"ids": target}, nil))["data"])
		if data["deleted"] != float64(len(target)) {
			t.Fatalf("deleted=%v want=%d", data["deleted"], len(target))
		}
		for _, docID := range target {
			restCode(t, restCall(t, TestConfig, http.MethodGet, path, nil, apiObject{"id": docID}), 200, 102)
			restCode(t, restCall(t, TestConfig, http.MethodGet, path+"/"+docID, nil, nil), 200, 404)
		}
	}
	data, remaining := listRESTDocuments(t, id, nil)
	if data["total"] != float64(0) || len(remaining) != 0 {
		t.Fatalf("delete left documents: %v", data)
	}
	restCode(t, restCall(t, TestConfig, http.MethodDelete, path, apiObject{"ids": ids}, nil), 200, 102)
	getRESTDocument(t, otherID, otherDocID)
}

func waitRESTDocumentStatus(t *testing.T, datasetID string, ids []string, expected string) []apiObject {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		data := object(t, restOK(t, restCallContext(t, ctx, TestConfig, http.MethodGet, documentRESTPath(datasetID), nil, apiObject{"page_size": 100}))["data"])
		found := map[string]apiObject{}
		for _, item := range array(t, data["docs"]) {
			doc := object(t, item)
			found[doc["id"].(string)] = doc
		}
		all := true
		result := make([]apiObject, 0, len(ids))
		for _, id := range ids {
			doc, ok := found[id]
			if !ok {
				t.Fatalf("ingesting document %s disappeared", id)
			}
			status := fmt.Sprint(doc["ingestion_status"])
			switch status {
			case expected:
			case "FAILED", "STOPPED", "COMPLETED":
				t.Fatalf("document %s terminal=%s want=%s detail=%v", id, status, expected, doc)
			case "CREATED", "SCHEDULED", "RUNNING", "STOPPING":
				all = false
			default:
				t.Fatalf("unexpected ingestion status %q document=%v", status, doc)
			}
			result = append(result, doc)
		}
		if all {
			return result
		}
		select {
		case <-ctx.Done():
			t.Fatalf("ingestion timeout waiting for %s: %v", expected, result)
		case <-ticker.C:
		}
	}
}

func TestRESTDocumentParsingValidationCompletionAndRepeatedStart(t *testing.T) {
	id, docs := seedRESTDocuments(t, 3)
	ids := documentRESTIDs(docs)
	path := documentRESTPath(id)
	for _, operation := range []string{"parse", "stop"} {
		for _, body := range []any{apiObject{}, apiObject{"document_ids": []string{}}, apiObject{"document_ids": "wrong"}, "not an object"} {
			restCode(t, restCall(t, TestConfig, http.MethodPost, path+"/"+operation, body, nil), 200, 102)
		}
		restCode(t, restCall(t, TestConfig, http.MethodPost, documentRESTPath(TestConfig.InvalidID)+"/"+operation, apiObject{"document_ids": ids}, nil), 200, 404)
	}
	// The start endpoint reports a count of accepted documents, including partial batches.
	invalid := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/parse", apiObject{"document_ids": []string{TestConfig.InvalidID}}, nil))["data"])
	if invalid["success_count"] != float64(0) {
		t.Fatalf("invalid document accepted: %v", invalid)
	}
	for _, docID := range ids {
		if getRESTDocument(t, id, docID)["ingestion_status"] != "UNSTART" {
			t.Fatal("invalid parse mutated existing document")
		}
	}
	stopped := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/stop", apiObject{"document_ids": []string{ids[0], ids[0]}}, nil))["data"])
	if stopped["success_count"] != float64(0) || len(array(t, stopped["errors"])) != 1 {
		t.Fatalf("stop unstarted=%v", stopped)
	}
	if !strings.Contains(fmt.Sprint(stopped["errors"]), "has not started or already completed") {
		t.Fatalf("unexpected stop rejection: %v", stopped)
	}
	restCode(t, restCall(t, TestConfig, http.MethodPost, path+"/stop", apiObject{"document_ids": []string{ids[0], TestConfig.InvalidID}}, nil), 200, 100)
	parsed := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/parse", apiObject{"document_ids": append(slices.Clone(ids), ids[0], TestConfig.InvalidID)}, nil))["data"])
	if parsed["success_count"] != float64(3) {
		t.Fatalf("accepted unique valid count=%v", parsed)
	}
	completed := waitRESTDocumentStatus(t, id, ids, "COMPLETED")
	for _, doc := range completed {
		docID := doc["id"].(string)
		if doc["chunk_count"].(float64) <= 0 || doc["token_count"].(float64) <= 0 || doc["progress"].(float64) != 1 {
			t.Fatalf("completed document lacks parsed content: %v", doc)
		}
		chunks := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, path+"/"+docID+"/chunks", nil, nil))["data"])
		items := array(t, chunks["chunks"])
		if len(items) == 0 {
			t.Fatalf("completed document %s has no chunks", docID)
		}
		var content strings.Builder
		for _, item := range items {
			chunk := object(t, item)
			text, ok := chunk["content_with_weight"].(string)
			if !ok || text == "" {
				t.Fatalf("parsed chunk has no source text: %v", chunk)
			}
			content.WriteString(text)
		}
		if !strings.Contains(strings.ToLower(content.String()), "coriander") {
			t.Fatalf("parsed chunks lost source content: %s", content.String())
		}
	}
	before := completed[0]
	rerun := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/parse", apiObject{"document_ids": ids[:1]}, nil))["data"])
	if rerun["success_count"] != float64(0) {
		t.Fatalf("completed document unexpectedly queued again: %v", rerun)
	}
	after := waitRESTDocumentStatus(t, id, ids[:1], "COMPLETED")[0]
	if after["chunk_count"] != before["chunk_count"] || !reflect.DeepEqual(after["latest_ingestion_event"], before["latest_ingestion_event"]) {
		t.Fatalf("repeated start changed chunks: before=%v after=%v", before["chunk_count"], after["chunk_count"])
	}
}

func TestRESTDocumentStopReachesTerminalState(t *testing.T) {
	dataset := createRESTDataset(t, apiObject{"parse_type": 1, "parser_id": "general"})
	id := dataset["id"].(string)
	path := documentRESTPath(id)
	// A moderate text fixture leaves a useful stop window without making the
	// retry expensive. Only a proved completion race permits another attempt.
	content := []byte(strings.Repeat("Coriander leaves and seeds provide botanical evidence.\n", 1280))
	for attempt := range 3 {
		doc := object(t, uploadRESTDocuments(t, TestConfig, id, []documentUpload{{Name: fmt.Sprintf("stop_%d.txt", attempt), Content: content}})[0])
		docID := doc["id"].(string)
		started := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/parse", apiObject{"document_ids": []string{docID}}, nil))["data"])
		if started["success_count"] != float64(1) {
			t.Fatalf("parse not accepted: %v", started)
		}
		result := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/stop", apiObject{"document_ids": []string{docID, docID}}, nil))["data"])
		switch result["success_count"] {
		case float64(1):
			stopped := waitRESTDocumentStatus(t, id, []string{docID}, "STOPPED")[0]
			repeated := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/stop", apiObject{"document_ids": []string{docID}}, nil))["data"])
			if repeated["success_count"] != float64(1) {
				t.Fatalf("repeated stopped request=%v", repeated)
			}
			restarted := object(t, restOK(t, restCall(t, TestConfig, http.MethodPost, path+"/parse", apiObject{"document_ids": []string{docID}}, nil))["data"])
			if restarted["success_count"] != float64(1) {
				t.Fatalf("stopped document was not accepted for retry: %v", restarted)
			}
			after := waitRESTDocumentStatus(t, id, []string{docID}, "COMPLETED")[0]
			if after["chunk_count"].(float64) <= 0 || after["token_count"].(float64) <= 0 || after["progress"] != float64(1) || reflect.DeepEqual(stopped["latest_ingestion_event"], after["latest_ingestion_event"]) {
				t.Fatalf("retry failed to produce a new completed run: stopped=%v completed=%v", stopped, after)
			}
			return
		case float64(0):
			errors := array(t, result["errors"])
			if len(errors) != 1 || errors[0] != "Can't stop parsing document that has not started or already completed" {
				t.Fatalf("stop rejection=%v", result)
			}
			waitRESTDocumentStatus(t, id, []string{docID}, "COMPLETED")
			t.Logf("attempt %d completed before stop; retrying with a fresh document", attempt+1)
		default:
			t.Fatalf("unexpected stop count: %v", result)
		}
	}
	t.Fatal("no document reached STOPPED in three attempts; stopping was not demonstrated")
}

func TestRESTDocumentConcurrentUploadAndDeletion(t *testing.T) {
	dataset := createRESTDataset(t, nil)
	id := dataset["id"].(string)
	ids := make([]string, 3)
	t.Run("upload", func(t *testing.T) {
		for i := range ids {
			t.Run(fmt.Sprintf("file_%d", i), func(t *testing.T) {
				t.Parallel()
				name := fmt.Sprintf("parallel_%d.txt", i)
				content := []byte(fmt.Sprintf("parallel content %d", i))
				doc := object(t, uploadRESTDocuments(t, TestConfig, id, []documentUpload{{Name: name, Content: content}})[0])
				if doc["name"] != name || doc["size"] != float64(len(content)) {
					t.Fatalf("concurrent upload metadata=%v", doc)
				}
				ids[i] = doc["id"].(string)
			})
		}
	})
	data, docs := listRESTDocuments(t, id, nil)
	if data["total"] != float64(len(ids)) {
		t.Fatalf("concurrent upload lost documents: %v", data)
	}
	requireRESTDocumentIDs(t, docs, ids)
	t.Run("delete", func(t *testing.T) {
		for _, docID := range ids {
			t.Run(docID, func(t *testing.T) {
				t.Parallel()
				deleted := object(t, restOK(t, restCall(t, TestConfig, http.MethodDelete, documentRESTPath(id), apiObject{"ids": []string{docID}}, nil))["data"])
				if deleted["deleted"] != float64(1) {
					t.Fatalf("concurrent deletion=%v", deleted)
				}
				restCode(t, restCall(t, TestConfig, http.MethodGet, documentRESTPath(id), nil, apiObject{"id": docID}), 200, 102)
			})
		}
	})
	data, docs = listRESTDocuments(t, id, nil)
	if data["total"] != float64(0) || len(docs) != 0 {
		t.Fatalf("concurrent deletion left documents: %v", data)
	}
}

func TestRESTDocumentRejectedParserUpdatePreservesName(t *testing.T) {
	id, docs := seedRESTDocuments(t, 1)
	docID := docs[0]["id"].(string)
	before := getRESTDocument(t, id, docID)
	config := object(t, before["parser_config"])
	var parser string
	for key := range config {
		if strings.HasPrefix(key, "Parser:") {
			parser = key
			break
		}
	}
	if parser == "" {
		t.Fatalf("general parser component missing: %v", config)
	}
	payload := apiObject{
		"name":          "must_not_persist.txt",
		"parser_config": apiObject{parser: apiObject{"pdf": apiObject{"pages": []any{[]any{3, 1}}}}},
	}
	restCode(t, restCall(t, TestConfig, http.MethodPatch, documentRESTPath(id)+"/"+docID, payload, nil), 200, 102)
	after := getRESTDocument(t, id, docID)
	for _, key := range []string{"name", "parser_config", "meta_fields", "size", "dataset_id"} {
		if !reflect.DeepEqual(before[key], after[key]) {
			t.Errorf("rejected parser update changed %s: before=%v after=%v", key, before[key], after[key])
		}
	}
}
