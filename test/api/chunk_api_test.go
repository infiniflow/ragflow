//go:build integration

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

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"
)

type chunkAPIResult struct {
	status  int
	url     string
	payload map[string]interface{}
	body    string
	err     error
}

var chunkHTTPClient = &http.Client{Timeout: 60 * time.Second}

func requestChunkAPI(t *testing.T, client *ServerTestConfig, method, path string, body, params map[string]interface{}) chunkAPIResult {
	t.Helper()

	requestURL, err := chunkRequestURL(client, path, params)
	if err != nil {
		return chunkAPIResult{err: err}
	}

	var requestBody io.Reader
	payloadText := "<empty>"
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return chunkAPIResult{url: requestURL, err: err}
		}
		payloadText = string(encoded)
		requestBody = bytes.NewReader(encoded)
	}

	t.Logf("request: %s %s\npayload: %s", method, requestURL, payloadText)
	request, err := http.NewRequest(method, requestURL, requestBody)
	if err != nil {
		return chunkAPIResult{url: requestURL, err: err}
	}
	request.Header = client.buildHeaders(nil)

	response, err := chunkHTTPClient.Do(request)
	if err != nil {
		return chunkAPIResult{url: requestURL, err: err}
	}
	return readChunkAPIResponse(t, requestURL, response)
}

func chunkRequestURL(client *ServerTestConfig, path string, params map[string]interface{}) (string, error) {
	requestURL, err := url.Parse(client.apiBase() + "/" + strings.TrimLeft(path, "/"))
	if err != nil {
		return "", err
	}
	query := requestURL.Query()
	for key, value := range params {
		if value != nil {
			query.Set(key, fmt.Sprint(value))
		}
	}
	requestURL.RawQuery = query.Encode()
	return requestURL.String(), nil
}

func readChunkAPIResponse(t *testing.T, requestURL string, response *http.Response) chunkAPIResult {
	t.Helper()
	defer response.Body.Close()

	rawBody, err := io.ReadAll(response.Body)
	if err != nil {
		return chunkAPIResult{status: response.StatusCode, url: requestURL, err: err}
	}
	body := string(rawBody)
	t.Logf("response: HTTP %d\nbody: %s", response.StatusCode, body)

	var payload map[string]interface{}
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return chunkAPIResult{
			status: response.StatusCode,
			url:    requestURL,
			body:   body,
			err:    fmt.Errorf("decode response JSON: %w", err),
		}
	}
	return chunkAPIResult{status: response.StatusCode, url: requestURL, payload: payload, body: body}
}

func callChunkAPI(t *testing.T, client *ServerTestConfig, method, path string, body, params map[string]interface{}, wantStatus int) map[string]interface{} {
	t.Helper()
	result := requestChunkAPI(t, client, method, path, body, params)
	if result.err != nil {
		t.Fatalf("%s %s failed: %v; response body: %s", method, result.url, result.err, result.body)
	}
	if result.status != wantStatus {
		t.Fatalf("%s %s status = %d, want %d; response: %s", method, result.url, result.status, wantStatus, result.body)
	}
	return result.payload
}

func requireChunkCode(t *testing.T, payload map[string]interface{}, want int) {
	t.Helper()
	code, ok := payload["code"].(float64)
	if !ok || int(code) != want {
		t.Fatalf("response code = %v, want %d; payload: %v", payload["code"], want, payload)
	}
}

func requireChunkMessage(t *testing.T, payload map[string]interface{}, want string) {
	t.Helper()
	message, ok := payload["message"].(string)
	if !ok || message != want {
		t.Fatalf("response message = %q, want %q; payload: %v", message, want, payload)
	}
}

func chunkData(t *testing.T, payload map[string]interface{}) map[string]interface{} {
	t.Helper()
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data is not an object: %v", payload)
	}
	return data
}

func chunkString(t *testing.T, object map[string]interface{}, key string) string {
	t.Helper()
	value, ok := object[key].(string)
	if !ok {
		t.Fatalf("%s is not a string: %v", key, object[key])
	}
	return value
}

func chunkPath(datasetID, documentID string) string {
	return fmt.Sprintf("/datasets/%s/documents/%s/chunks", datasetID, documentID)
}

func chunkClientWithToken(token string) *ServerTestConfig {
	client := *TestConfig
	client.AuthToken = token
	client.APIKey = ""
	return &client
}

func requireChunkAuthError(t *testing.T, client *ServerTestConfig, method, path string, body map[string]interface{}, scenario string) {
	t.Helper()
	payload := callChunkAPI(t, client, method, path, body, nil, http.StatusUnauthorized)
	requireChunkCode(t, payload, http.StatusUnauthorized)

	message := chunkString(t, payload, "message")
	switch scenario {
	case "missing token":
		if message != "Missing Authorization header" {
			t.Fatalf("missing-token message = %q", message)
		}
	case "invalid token":
		if message != "Invalid access token" && message != "Invalid auth credentials" {
			t.Fatalf("invalid-token message = %q", message)
		}
	default:
		t.Fatalf("unknown auth scenario %q", scenario)
	}
}

func createChunkTestDocument(t *testing.T, filename string) (string, string) {
	t.Helper()
	datasetName := fmt.Sprintf("chunk_api_%d", time.Now().UnixNano())
	datasetPayload := callChunkAPI(t, TestConfig, http.MethodPost, "/datasets",
		map[string]interface{}{"name": datasetName, "parse_type": 1, "parser_id": "general"}, nil, http.StatusOK)
	requireChunkCode(t, datasetPayload, 0)
	datasetID := chunkString(t, chunkData(t, datasetPayload), "id")

	t.Cleanup(func() {
		deleteDataset := requestChunkAPI(t, TestConfig, http.MethodDelete, "/datasets",
			map[string]interface{}{"ids": []interface{}{datasetID}}, nil)
		if deleteDataset.err != nil {
			t.Errorf("delete test dataset failed: %v", deleteDataset.err)
		} else if deleteDataset.status != http.StatusOK {
			t.Errorf("delete test dataset status = %d, want %d; response: %s", deleteDataset.status, http.StatusOK, deleteDataset.body)
		} else if code, ok := deleteDataset.payload["code"].(float64); !ok || code != 0 {
			t.Errorf("delete test dataset returned unexpected response: %s", deleteDataset.body)
		}
	})
	return datasetID, uploadChunkTestDocument(t, datasetID, filename)
}

func uploadChunkTestDocument(t *testing.T, datasetID, filename string) string {
	t.Helper()

	items := uploadRESTDocuments(t, TestConfig, datasetID, []documentUpload{{Name: filename, Content: []byte("ragflow test upload")}})
	documentID := chunkString(t, object(t, items[0]), "id")
	t.Cleanup(func() {
		deleted := requestChunkAPI(t, TestConfig, http.MethodDelete,
			fmt.Sprintf("/datasets/%s/documents", datasetID),
			map[string]interface{}{"ids": []string{documentID}}, nil)
		if deleted.err != nil {
			t.Errorf("delete test document failed: %v", deleted.err)
		} else if deleted.status != http.StatusOK || deleted.payload["code"] != float64(0) {
			t.Errorf("delete test document failed: HTTP %d, %s", deleted.status, deleted.body)
		}
	})
	return documentID
}

func TestChunkAuthRequiresCredentials(t *testing.T) {
	path := chunkPath(TestConfig.InvalidID, TestConfig.InvalidID)
	chunkIDPath := path + "/" + TestConfig.InvalidID
	requests := []struct {
		name   string
		method string
		path   string
		body   map[string]interface{}
	}{
		{name: "add", method: http.MethodPost, path: path, body: map[string]interface{}{"content": "chunk"}},
		{name: "list", method: http.MethodGet, path: path},
		{name: "get", method: http.MethodGet, path: chunkIDPath},
		{name: "update", method: http.MethodPatch, path: chunkIDPath, body: map[string]interface{}{"content": "chunk"}},
		{name: "delete", method: http.MethodDelete, path: path, body: map[string]interface{}{"chunk_ids": []interface{}{}}},
	}
	scenarios := []struct {
		name   string
		client *ServerTestConfig
	}{
		{name: "missing token", client: chunkClientWithToken("")},
		{name: "invalid token", client: chunkClientWithToken(TestConfig.InvalidAPIKey)},
	}

	for _, request := range requests {
		for _, scenario := range scenarios {
			t.Run(request.name+"/"+scenario.name, func(t *testing.T) {
				requireChunkAuthError(t, scenario.client, request.method, request.path, request.body, scenario.name)
			})
		}
	}
}

func TestChunkListRejectsInvalidQueryValues(t *testing.T) {
	path := chunkPath(TestConfig.InvalidID, TestConfig.InvalidID)
	cases := []struct {
		name    string
		key     string
		value   string
		message string
	}{
		{name: "page zero", key: "page", value: "0", message: "page must be a positive integer"},
		{name: "page not numeric", key: "page", value: "first", message: "page must be a positive integer"},
		{name: "page_size zero", key: "page_size", value: "0", message: "page_size must be a positive integer"},
		{name: "page_size not numeric", key: "page_size", value: "many", message: "page_size must be a positive integer"},
		{name: "available invalid", key: "available", value: "sometimes", message: "available must be one of: true, false, 1, 0"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := callChunkAPI(t, TestConfig, http.MethodGet, path, nil,
				map[string]interface{}{testCase.key: testCase.value}, http.StatusBadRequest)
			requireChunkCode(t, payload, 101)
			requireChunkMessage(t, payload, testCase.message)
		})
	}
}

func TestChunkAddRejectsMalformedPayloadFields(t *testing.T) {
	path := chunkPath(TestConfig.InvalidID, TestConfig.InvalidID)
	cases := []struct {
		name    string
		body    map[string]interface{}
		code    int
		message string
	}{
		{
			name:    "content must be a string",
			body:    map[string]interface{}{"content": 42},
			code:    101,
			message: "`content` must be a string",
		},
		{
			name:    "important_keywords must be a list",
			body:    map[string]interface{}{"content": "chunk", "important_keywords": "keyword"},
			code:    102,
			message: "`important_keywords` is required to be a list",
		},
		{
			name:    "questions must contain strings",
			body:    map[string]interface{}{"content": "chunk", "questions": []interface{}{42}},
			code:    102,
			message: "`questions` must be a list of strings",
		},
		{
			name:    "tag_kwd cannot be null",
			body:    map[string]interface{}{"content": "chunk", "tag_kwd": nil},
			code:    102,
			message: "`tag_kwd` is required to be a list",
		},
		{
			name:    "image_base64 must be a string",
			body:    map[string]interface{}{"content": "chunk", "image_base64": 42},
			code:    101,
			message: "`image_base64` must be a string",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := callChunkAPI(t, TestConfig, http.MethodPost, path, testCase.body, nil, http.StatusOK)
			requireChunkCode(t, payload, testCase.code)
			requireChunkMessage(t, payload, testCase.message)
		})
	}
}

func TestChunkUpdateRejectsUnsupportedAndMalformedFields(t *testing.T) {
	path := chunkPath(TestConfig.InvalidID, TestConfig.InvalidID) + "/" + TestConfig.InvalidID
	cases := []struct {
		name       string
		body       map[string]interface{}
		status     int
		code       int
		message    string
		messageHas string
	}{
		{
			name:       "unsupported field",
			body:       map[string]interface{}{"unknown_field": true},
			status:     http.StatusBadRequest,
			code:       http.StatusBadRequest,
			messageHas: "Update field 'unknown_field' is not supported.",
		},
		{
			name:    "tag_kwd scalar is rejected",
			body:    map[string]interface{}{"tag_kwd": "tag"},
			status:  http.StatusBadRequest,
			code:    102,
			message: "`tag_kwd` is required to be a list",
		},
		{
			name:    "tag_kwd elements must be strings",
			body:    map[string]interface{}{"tag_kwd": []interface{}{"tag", 42}},
			status:  http.StatusBadRequest,
			code:    102,
			message: "`tag_kwd` must be a list of strings",
		},
		{
			name:    "image_base64 must be a string",
			body:    map[string]interface{}{"image_base64": 42},
			status:  http.StatusBadRequest,
			code:    102,
			message: "`image_base64` must be a non-empty string",
		},
		{
			name:    "image_update_mode must be a string",
			body:    map[string]interface{}{"image_update_mode": false},
			status:  http.StatusBadRequest,
			code:    102,
			message: "`image_update_mode` must be a string",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := callChunkAPI(t, TestConfig, http.MethodPatch, path, testCase.body, nil, testCase.status)
			requireChunkCode(t, payload, testCase.code)
			if testCase.messageHas != "" {
				message := chunkString(t, payload, "message")
				if !strings.HasPrefix(message, testCase.messageHas) {
					t.Fatalf("response message = %q, want prefix %q", message, testCase.messageHas)
				}
				return
			}
			requireChunkMessage(t, payload, testCase.message)
		})
	}
}

func TestChunkAddRejectsMissingOrBlankContent(t *testing.T) {
	datasetID, documentID := createChunkTestDocument(t, "chunk_missing_content.txt")
	path := chunkPath(datasetID, documentID)
	cases := []struct {
		name string
		body map[string]interface{}
	}{
		{name: "missing content", body: map[string]interface{}{}},
		{name: "blank content", body: map[string]interface{}{"content": "  \n "}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := callChunkAPI(t, TestConfig, http.MethodPost, path, testCase.body, nil, http.StatusOK)
			requireChunkCode(t, payload, 102)
			requireChunkMessage(t, payload, "`content` is required")
		})
	}
}

func TestChunkAddRejectsInvalidDatasetAndDocument(t *testing.T) {
	datasetID, documentID := createChunkTestDocument(t, "chunk_invalid_targets.txt")

	t.Run("dataset does not exist", func(t *testing.T) {
		payload := callChunkAPI(t, TestConfig, http.MethodPost,
			chunkPath(TestConfig.InvalidID, documentID),
			map[string]interface{}{"content": "chunk"}, nil, http.StatusOK)
		requireChunkCode(t, payload, 404)
		requireChunkMessage(t, payload, "Resource not found")
	})

	t.Run("document does not exist", func(t *testing.T) {
		payload := callChunkAPI(t, TestConfig, http.MethodPost,
			chunkPath(datasetID, TestConfig.InvalidID),
			map[string]interface{}{"content": "chunk"}, nil, http.StatusOK)
		requireChunkCode(t, payload, 102)
		requireChunkMessage(t, payload, fmt.Sprintf("you don't own the document %s", TestConfig.InvalidID))
	})

	t.Run("document belongs to another dataset", func(t *testing.T) {
		otherDatasetID, _ := createChunkTestDocument(t, "chunk_other_dataset.txt")
		payload := callChunkAPI(t, TestConfig, http.MethodPost,
			chunkPath(otherDatasetID, documentID),
			map[string]interface{}{"content": "chunk"}, nil, http.StatusOK)
		requireChunkCode(t, payload, 102)
		requireChunkMessage(t, payload, fmt.Sprintf("you don't own the document %s", documentID))
	})
}

func TestChunkLifecycle(t *testing.T) {
	datasetID, documentID := createChunkTestDocument(t, "chunk_lifecycle.txt")
	path := chunkPath(datasetID, documentID)
	var ids []string
	for _, content := range []string{
		"Oranges are citrus fruits.", "Bicycles have two wheels.", "Galaxies contain billions of stars.",
	} {
		added := callChunkAPI(t, TestConfig, http.MethodPost, path, map[string]interface{}{
			"content": content, "important_keywords": []string{"lifecycle"},
			"questions": []string{"What must persist?"}, "tag_kwd": []string{"contract"},
		}, nil, http.StatusOK)
		requireChunkCode(t, added, 0)
		chunk, ok := chunkData(t, added)["chunk"].(map[string]interface{})
		if !ok {
			t.Fatalf("created chunk is not an object: %v", added)
		}
		id := chunkString(t, chunk, "id")
		if id == "" || slices.Contains(ids, id) {
			t.Fatalf("created chunk ID is empty or reused: %q; previous IDs: %v", id, ids)
		}
		ids = append(ids, id)
		get := callChunkAPI(t, TestConfig, http.MethodGet, path+"/"+id, nil, nil, http.StatusOK)
		requireChunkCode(t, get, 0)
		stored := chunkData(t, get)
		for key, want := range map[string]string{
			"id": id, "doc_id": documentID, "kb_id": datasetID, "content_with_weight": content,
		} {
			if got := chunkString(t, stored, key); got != want {
				t.Fatalf("stored %s = %q, want %q", key, got, want)
			}
		}
		requireChunkStrings(t, stored, "important_kwd", "lifecycle")
		requireChunkStrings(t, stored, "question_kwd", "What must persist?")
		requireChunkStrings(t, stored, "tag_kwd", "contract")
		for key := range stored {
			if strings.HasSuffix(key, "_vec") || strings.HasSuffix(key, "_tks") || strings.HasSuffix(key, "_ltks") {
				t.Fatalf("chunk response exposes internal field %q", key)
			}
		}
	}
	assertChunkIDs(t, listChunkIDs(t, path, nil, 3), ids)
	var paged []string
	for page := 1; page <= 2; page++ {
		pageIDs := listChunkIDs(t, path, map[string]interface{}{"page": page, "page_size": 2}, 3)
		if want := 3 - page; len(pageIDs) != want {
			t.Fatalf("page %d contains %d chunks, want %d", page, len(pageIDs), want)
		}
		paged = append(paged, pageIDs...)
	}
	assertChunkIDs(t, paged, ids)
	assertChunkIDs(t, listChunkIDs(t, path, map[string]interface{}{"page": 3, "page_size": 2}, 3), nil)
	assertChunkIDs(t, listChunkIDs(t, path, map[string]interface{}{"chunk_ids": ids[0] + "," + ids[2]}, 2), []string{ids[0], ids[2]})
	assertChunkIDs(t, listChunkIDs(t, path, map[string]interface{}{"chunk_ids": TestConfig.InvalidID}, 0), nil)
	assertChunkIDs(t, listChunkIDs(t, path, map[string]interface{}{"keywords": "galaxies"}, 1), ids[2:])

	chunkURL := path + "/" + ids[0]
	const updated = "The updated content must also persist."
	patch := callChunkAPI(t, TestConfig, http.MethodPatch, chunkURL,
		map[string]interface{}{"content": updated, "available": false}, nil, http.StatusOK)
	requireChunkCode(t, patch, 0)
	get := callChunkAPI(t, TestConfig, http.MethodGet, chunkURL, nil, nil, http.StatusOK)
	requireChunkCode(t, get, 0)
	stored := chunkData(t, get)
	if got := chunkString(t, stored, "content_with_weight"); got != updated {
		t.Fatalf("updated content = %q, want %q", got, updated)
	}
	if stored["available_int"] != float64(0) {
		t.Fatalf("updated available_int = %v, want 0", stored["available_int"])
	}
	// Omitted lists must survive a partial update; explicit empty lists clear them.
	requireChunkStrings(t, stored, "important_kwd", "lifecycle")
	requireChunkStrings(t, stored, "question_kwd", "What must persist?")
	requireChunkStrings(t, stored, "tag_kwd", "contract")
	assertChunkIDs(t, listChunkIDs(t, path, nil, 3), ids)
	assertChunkIDs(t, listChunkIDs(t, path, map[string]interface{}{"available": true}, 2), ids[1:])
	assertChunkIDs(t, listChunkIDs(t, path, map[string]interface{}{"available": false}, 1), ids[:1])
	patch = callChunkAPI(t, TestConfig, http.MethodPatch, chunkURL, map[string]interface{}{
		"important_keywords": []string{}, "questions": []string{}, "tag_kwd": []string{},
	}, nil, http.StatusOK)
	requireChunkCode(t, patch, 0)
	get = callChunkAPI(t, TestConfig, http.MethodGet, chunkURL, nil, nil, http.StatusOK)
	requireChunkCode(t, get, 0)
	stored = chunkData(t, get)
	for _, key := range []string{"important_kwd", "question_kwd", "tag_kwd"} {
		requireChunkStrings(t, stored, key)
	}
	if got := chunkString(t, stored, "content_with_weight"); got != updated {
		t.Fatalf("metadata update changed content to %q", got)
	}

	// Keep another chunk alive so an implementation that deletes everything fails.
	deleted := callChunkAPI(t, TestConfig, http.MethodDelete, path,
		map[string]interface{}{"chunk_ids": []string{ids[0], ids[1], ids[0]}}, nil, http.StatusOK)
	requireChunkCode(t, deleted, 0)
	if deleted["data"] != float64(2) {
		t.Fatalf("deleted count = %v, want 2 unique chunks", deleted["data"])
	}
	assertChunkIDs(t, listChunkIDs(t, path, nil, 1), ids[2:])
	deleted = callChunkAPI(t, TestConfig, http.MethodDelete, path,
		map[string]interface{}{"chunk_ids": ids[:2]}, nil, http.StatusOK)
	requireChunkCode(t, deleted, 0)
	if deleted["data"] != float64(0) {
		t.Fatalf("repeated deletion count = %v, want 0", deleted["data"])
	}
	deleted = callChunkAPI(t, TestConfig, http.MethodDelete, path,
		map[string]interface{}{"delete_all": true}, nil, http.StatusOK)
	requireChunkCode(t, deleted, 0)
	if deleted["data"] != float64(1) {
		t.Fatalf("delete_all count = %v, want 1", deleted["data"])
	}
	assertChunkIDs(t, listChunkIDs(t, path, nil, 0), nil)
}

func requireChunkStrings(t *testing.T, object map[string]interface{}, key string, want ...string) {
	t.Helper()
	items, ok := object[key].([]interface{})
	if !ok {
		t.Fatalf("%s is not an array: %v", key, object[key])
	}
	got := make([]string, len(items))
	for i, item := range items {
		value, ok := item.(string)
		if !ok {
			t.Fatalf("%s[%d] is not a string: %v", key, i, item)
		}
		got[i] = value
	}
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", key, got, want)
	}
}

func listChunkIDs(t *testing.T, path string, params map[string]interface{}, wantTotal int) []string {
	t.Helper()
	listed := callChunkAPI(t, TestConfig, http.MethodGet, path, nil, params, http.StatusOK)
	requireChunkCode(t, listed, 0)
	data := chunkData(t, listed)
	if data["total"] != float64(wantTotal) {
		t.Fatalf("list total = %v, want %d; response: %v", data["total"], wantTotal, listed)
	}
	chunks, ok := data["chunks"].([]interface{})
	if !ok {
		t.Fatalf("listed chunks is not an array: %v", data)
	}
	ids := make([]string, len(chunks))
	for i, raw := range chunks {
		chunk, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("listed chunk is not an object: %v", raw)
		}
		ids[i] = chunkString(t, chunk, "chunk_id")
	}
	return ids
}

func assertChunkIDs(t *testing.T, got, want []string) {
	t.Helper()
	got, want = slices.Clone(got), slices.Clone(want)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("chunk IDs = %v, want %v", got, want)
	}
}

func TestChunkReadUpdateAndDeleteStayWithinDocument(t *testing.T) {
	datasetID, documentID := createChunkTestDocument(t, "chunk_isolation_source.txt")
	otherDocumentID := uploadChunkTestDocument(t, datasetID, "chunk_isolation_other.txt")
	if otherDocumentID == documentID {
		t.Fatal("isolation test requires two different documents")
	}
	path := chunkPath(datasetID, documentID)
	otherPath := chunkPath(datasetID, otherDocumentID)
	const content = "This chunk belongs only to the source document."
	added := callChunkAPI(t, TestConfig, http.MethodPost, path,
		map[string]interface{}{"content": content}, nil, http.StatusOK)
	requireChunkCode(t, added, 0)
	chunk, ok := chunkData(t, added)["chunk"].(map[string]interface{})
	if !ok {
		t.Fatalf("created chunk is not an object: %v", added)
	}
	id := chunkString(t, chunk, "id")
	assertChunkIDs(t, listChunkIDs(t, path, nil, 1), []string{id})
	assertChunkIDs(t, listChunkIDs(t, otherPath, nil, 0), nil)
	for _, method := range []string{http.MethodGet, http.MethodPatch} {
		t.Run(method+" rejects another document's chunk", func(t *testing.T) {
			var body map[string]interface{}
			if method == http.MethodPatch {
				body = map[string]interface{}{"content": "Must not overwrite the source."}
			}
			result := requestChunkAPI(t, TestConfig, method, otherPath+"/"+id, body, nil)
			if result.err != nil {
				t.Fatal(result.err)
			}
			// Check the specific rejection rather than allowing any server error.
			if code, ok := result.payload["code"].(float64); !ok || code == 0 {
				t.Fatalf("another document's chunk was accepted: %s", result.body)
			}
			requireChunkMessage(t, result.payload, "chunk not found")
			if result.payload["data"] != nil {
				t.Fatalf("rejected request exposes chunk data: %s", result.body)
			}
		})
	}
	deleted := callChunkAPI(t, TestConfig, http.MethodDelete, otherPath,
		map[string]interface{}{"chunk_ids": []string{id}}, nil, http.StatusOK)
	requireChunkCode(t, deleted, 0)
	if deleted["data"] != float64(0) {
		t.Fatalf("deleted another document's chunk: %v", deleted)
	}
	get := callChunkAPI(t, TestConfig, http.MethodGet, path+"/"+id, nil, nil, http.StatusOK)
	requireChunkCode(t, get, 0)
	if got := chunkString(t, chunkData(t, get), "content_with_weight"); got != content {
		t.Fatalf("source chunk changed after rejected mutations: %q", got)
	}
	assertChunkIDs(t, listChunkIDs(t, path, nil, 1), []string{id})
}

func TestChunkDeleteRejectsMismatchedDataset(t *testing.T) {
	datasetID, documentID := createChunkTestDocument(t, "chunk_delete_dataset_source.txt")
	otherDatasetID, _ := createChunkTestDocument(t, "chunk_delete_dataset_other.txt")
	path := chunkPath(datasetID, documentID)
	added := callChunkAPI(t, TestConfig, http.MethodPost, path,
		map[string]interface{}{"content": "A mismatched dataset path must not delete this chunk."}, nil, http.StatusOK)
	requireChunkCode(t, added, 0)
	chunk, ok := chunkData(t, added)["chunk"].(map[string]interface{})
	if !ok {
		t.Fatalf("created chunk is not an object: %v", added)
	}
	id := chunkString(t, chunk, "id")
	assertChunkIDs(t, listChunkIDs(t, path, nil, 1), []string{id})
	for _, tc := range []struct {
		name string
		body map[string]interface{}
	}{
		{name: "specific chunks", body: map[string]interface{}{"chunk_ids": []string{id}}},
		{name: "all chunks", body: map[string]interface{}{"delete_all": true}},
		{name: "body cannot override route", body: map[string]interface{}{"dataset_id": datasetID, "doc_id": documentID, "chunk_ids": []string{id}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := callChunkAPI(t, TestConfig, http.MethodDelete, chunkPath(otherDatasetID, documentID), tc.body, nil, http.StatusOK)
			requireChunkCode(t, payload, 404)
			requireChunkMessage(t, payload, "Resource not found")
			if payload["data"] != nil {
				t.Fatalf("resource error returned data: %v", payload)
			}
			assertChunkIDs(t, listChunkIDs(t, path, nil, 1), []string{id})
		})
	}
	// A rejection must leave the source data intact.
	assertChunkIDs(t, listChunkIDs(t, path, nil, 1), []string{id})
}

func TestChunkListEmptyDocument(t *testing.T) {
	datasetID, documentID := createChunkTestDocument(t, "chunk_list_empty.txt")
	payload := callChunkAPI(t, TestConfig, http.MethodGet, chunkPath(datasetID, documentID), nil, nil, http.StatusOK)
	requireChunkCode(t, payload, 0)

	data := chunkData(t, payload)
	chunks, ok := data["chunks"].([]interface{})
	if !ok {
		t.Fatalf("response data.chunks is not an array: %v", data["chunks"])
	}
	if len(chunks) != 0 {
		t.Fatalf("empty document returned %d chunks: %v", len(chunks), chunks)
	}
	if total, ok := data["total"].(float64); !ok || total != 0 {
		t.Fatalf("response data.total = %v, want 0", data["total"])
	}
	doc, ok := data["doc"].(map[string]interface{})
	if !ok {
		t.Fatalf("response data.doc is not an object: %v", data["doc"])
	}
	if got := chunkString(t, doc, "id"); got != documentID {
		t.Fatalf("response document id = %q, want %q", got, documentID)
	}
	if got := chunkString(t, doc, "dataset_id"); got != datasetID {
		t.Fatalf("response document dataset_id = %q, want %q", got, datasetID)
	}
	if count, ok := doc["chunk_count"].(float64); !ok || count != 0 {
		t.Fatalf("response document chunk_count = %v, want 0", doc["chunk_count"])
	}
}
