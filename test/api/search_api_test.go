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
	"encoding/json/v2"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Search App REST contract tests.
//
// Every request goes through restCall, which logs the (redacted) method, URL,
// payload, HTTP status, application code, response body and duration; SSE
// responses are logged as an event count plus a preview of the first frames.
// Assertions therefore only cover what the log cannot prove: persistence,
// isolation, and the shape of the payload.

func createRESTSearch(t *testing.T, name string) string {
	t.Helper()
	response := restCall(t, TestConfig, http.MethodPost, "/searches", apiObject{"name": name, "description": "restful search"}, nil)
	if data, ok := response.JSON["data"].(map[string]any); ok {
		if id, ok := data["search_id"].(string); ok && id != "" {
			cleanupResource(t, TestConfig, "searches", id)
		}
	}
	envelope := restOK(t, response)
	id, ok := object(t, envelope["data"])["search_id"].(string)
	if !ok || id == "" {
		t.Fatalf("created search has no ID: %v", envelope)
	}
	return id
}

func getRESTSearch(t *testing.T, id string) apiObject {
	t.Helper()
	return object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/searches/"+id, nil, nil))["data"])
}

func listRESTSearches(t *testing.T, query apiObject) ([]any, float64) {
	t.Helper()
	data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/searches", nil, query))["data"])
	total, ok := data["total"].(float64)
	if !ok {
		t.Fatalf("list response has no numeric total: %v", data)
	}
	return array(t, data["search_apps"]), total
}

func searchAppIDs(t *testing.T, apps []any) []string {
	t.Helper()
	ids := make([]string, 0, len(apps))
	for _, app := range apps {
		id, ok := object(t, app)["id"].(string)
		if !ok || id == "" {
			t.Fatalf("list row has no id: %v", app)
		}
		ids = append(ids, id)
	}
	return ids
}

func updateRESTSearch(t *testing.T, id, name string, config apiObject) apiObject {
	t.Helper()
	body := apiObject{"name": name}
	if config != nil {
		body["search_config"] = config
	}
	return object(t, restOK(t, restCall(t, TestConfig, http.MethodPut, "/searches/"+id, body, nil))["data"])
}

func createRESTSearchDataset(t *testing.T) string {
	t.Helper()
	response := restCall(t, TestConfig, http.MethodPost, "/datasets", apiObject{
		"name": uniqueName("search_dataset"), "parse_type": 1, "parser_id": "general",
	}, nil)
	if data, ok := response.JSON["data"].(map[string]any); ok {
		if id, ok := data["id"].(string); ok && id != "" {
			cleanupResource(t, TestConfig, "datasets", id)
		}
	}
	id, ok := object(t, restOK(t, response)["data"])["id"].(string)
	if !ok || id == "" {
		t.Fatal("dataset has no id")
	}
	return id
}

func TestRESTSearchLifecycle(t *testing.T) {
	name := uniqueName("search_lifecycle")
	id := createRESTSearch(t, name)

	detail := getRESTSearch(t, id)
	for _, key := range []string{"id", "tenant_id", "name", "description", "created_by", "create_time", "update_time", "search_config"} {
		if _, ok := detail[key]; !ok {
			t.Fatalf("detail omitted %s: %v", key, detail)
		}
	}
	if detail["name"] != name || detail["description"] != "restful search" {
		t.Fatalf("detail did not persist the creation payload: %v", detail)
	}
	if _, ok := detail["search_config"].(map[string]any); !ok {
		t.Fatalf("search_config must be an object: %v", detail["search_config"])
	}

	apps, _ := listRESTSearches(t, nil)
	listed := false
	for _, app := range apps {
		row := object(t, app)
		if row["id"] != id {
			continue
		}
		listed = true
		for _, key := range []string{"name", "status", "nickname", "search_config"} {
			if _, ok := row[key]; !ok {
				t.Fatalf("list row omitted %s: %v", key, row)
			}
		}
		if row["name"] != name {
			t.Fatalf("list row name = %v, want %s", row["name"], name)
		}
	}
	if !listed {
		t.Fatalf("list omitted the created search %s: %v", id, apps)
	}

	// Identity fields are immutable: a PUT echoing them back must not take.
	updated := updateRESTSearch(t, id, name+"_renamed", apiObject{"top_k": 3})
	if updated["name"] != name+"_renamed" {
		t.Fatalf("update response name = %v, want %s_renamed", updated["name"], name)
	}
	after := getRESTSearch(t, id)
	if after["name"] != name+"_renamed" {
		t.Fatalf("update did not persist: %v", after)
	}
	if after["id"] != id || after["tenant_id"] != detail["tenant_id"] || after["created_by"] != detail["created_by"] {
		t.Fatalf("identity fields changed: before=%v after=%v", detail, after)
	}
	if config := object(t, after["search_config"]); config["top_k"] != float64(3) {
		t.Fatalf("search_config did not persist top_k: %v", config)
	}

	deleted := restOK(t, restCall(t, TestConfig, http.MethodDelete, "/searches/"+id, nil, nil))
	if deleted["data"] != true {
		t.Fatalf("delete returned %v, want true", deleted["data"])
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/searches/"+id, nil, nil), http.StatusOK, 404)
	// Deletion is a hard delete, so a repeat must be invisible rather than a no-op success.
	restCode(t, restCall(t, TestConfig, http.MethodDelete, "/searches/"+id, nil, nil), http.StatusOK, 404)
}

func TestRESTSearchNameRules(t *testing.T) {
	t.Run("create_rejects_empty_and_oversized", func(t *testing.T) {
		for _, tc := range []struct{ name, value, want string }{
			{"empty", "", "empty"},
			{"whitespace", " \t ", "empty"},
			{"oversized", strings.Repeat("s", 257), "large than"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				result := restCall(t, TestConfig, http.MethodPost, "/searches", apiObject{"name": tc.value}, nil)
				restCode(t, result, http.StatusOK, 102)
				if message, _ := result.JSON["message"].(string); !strings.Contains(message, tc.want) {
					t.Fatalf("message = %q, want %q", message, tc.want)
				}
			})
		}
	})

	name := uniqueName("search_dup")
	first := createRESTSearch(t, name)
	second := createRESTSearch(t, name)
	if got := getRESTSearch(t, first)["name"]; got != name {
		t.Fatalf("first search name = %v, want %s", got, name)
	}
	if got := getRESTSearch(t, second)["name"]; got != name+"(1)" {
		t.Fatalf("duplicate creation name = %v, want %s(1)", got, name)
	}

	t.Run("update_rejects_duplicate_name_atomically", func(t *testing.T) {
		result := restCall(t, TestConfig, http.MethodPut, "/searches/"+second,
			apiObject{"name": name, "search_config": apiObject{}}, nil)
		restCode(t, result, http.StatusOK, 102)
		if message, _ := result.JSON["message"].(string); !strings.Contains(message, "Duplicated search name") {
			t.Fatalf("message = %q, want a duplicate-name rejection", message)
		}
		if got := getRESTSearch(t, second)["name"]; got != name+"(1)" {
			t.Fatalf("rejected update mutated the search: %v", got)
		}
	})

	t.Run("case_only_rename_allowed", func(t *testing.T) {
		other := createRESTSearch(t, uniqueName("search_case"))
		restOK(t, restCall(t, TestConfig, http.MethodPut, "/searches/"+other,
			apiObject{"name": strings.ToUpper(uniqueName("search_case")), "search_config": apiObject{}}, nil))
	})
}

func TestRESTSearchListFiltering(t *testing.T) {
	prefix := uniqueName("search_filter")
	alpha := createRESTSearch(t, prefix+"_alpha")
	beta := createRESTSearch(t, prefix+"_beta")
	tenantID, _ := getRESTSearch(t, alpha)["tenant_id"].(string)
	if tenantID == "" {
		t.Fatal("detail has no tenant_id")
	}

	apps, total := listRESTSearches(t, apiObject{"keywords": prefix})
	if ids := searchAppIDs(t, apps); len(ids) != 2 || total != 2 {
		t.Fatalf("keyword filter = %v total=%v, want both apps", ids, total)
	}
	if apps, total = listRESTSearches(t, apiObject{"keywords": strings.ToUpper(prefix)}); len(apps) != 2 || total != 2 {
		t.Fatalf("keyword filter must be case-insensitive: %v total=%v", apps, total)
	}
	if apps, _ = listRESTSearches(t, apiObject{"keywords": uniqueName("missing_search")}); len(apps) != 0 {
		t.Fatalf("absent keyword matched %v", apps)
	}

	for _, tc := range []struct {
		page  int
		desc  bool
		want  string
		total float64
	}{
		{1, false, alpha, 2},
		{2, false, beta, 2},
		{1, true, beta, 2},
	} {
		t.Run("pagination", func(t *testing.T) {
			apps, total := listRESTSearches(t, apiObject{
				"keywords": prefix, "orderby": "name", "desc": tc.desc, "page": tc.page, "page_size": 1,
			})
			if len(apps) != 1 || total != tc.total {
				t.Fatalf("page=%d desc=%v rows=%d total=%v, want 1 row of %v", tc.page, tc.desc, len(apps), total, tc.total)
			}
			if got := object(t, apps[0])["id"]; got != tc.want {
				t.Fatalf("page=%d desc=%v first row = %v, want %s", tc.page, tc.desc, got, tc.want)
			}
		})
	}

	if apps, _ = listRESTSearches(t, apiObject{"keywords": prefix, "owner_ids": tenantID}); len(apps) != 2 {
		t.Fatalf("owner_ids filter dropped the owner's own apps: %v", apps)
	}
	if apps, _ = listRESTSearches(t, apiObject{"keywords": prefix, "owner_ids": "00000000000000000000000000000000"}); len(apps) != 0 {
		t.Fatalf("owner_ids filter leaked apps to a foreign owner: %v", apps)
	}
}

func TestRESTSearchConfigContract(t *testing.T) {
	name := uniqueName("search_config")
	id := createRESTSearch(t, name)

	// A partial search_config merges into the stored one instead of replacing it.
	updateRESTSearch(t, id, name, apiObject{"top_k": 3})
	updateRESTSearch(t, id, name, apiObject{"similarity_threshold": 0.2})
	config := object(t, getRESTSearch(t, id)["search_config"])
	if config["top_k"] != float64(3) || config["similarity_threshold"] != float64(0.2) {
		t.Fatalf("search_config must merge across updates: %v", config)
	}

	// The public payload carries the keyword weight only; the internal vector
	// weight is persisted as its complement and hidden from responses.
	updateRESTSearch(t, id, name, apiObject{"keywords_similarity_weight": 0.3})
	config = object(t, getRESTSearch(t, id)["search_config"])
	if config["keywords_similarity_weight"] != float64(0.3) {
		t.Fatalf("keywords_similarity_weight = %v, want 0.3", config["keywords_similarity_weight"])
	}
	if _, leaked := config["vector_similarity_weight"]; leaked {
		t.Fatalf("response must not expose vector_similarity_weight: %v", config)
	}

	rejected := restCall(t, TestConfig, http.MethodPut, "/searches/"+id,
		apiObject{"name": name, "search_config": apiObject{"keywords_similarity_weight": 1.5}}, nil)
	restCode(t, rejected, http.StatusOK, 102)
	if message, _ := rejected.JSON["message"].(string); !strings.Contains(message, "must be between 0 and 1") {
		t.Fatalf("message = %q, want a range rejection", message)
	}
	config = object(t, getRESTSearch(t, id)["search_config"])
	if config["keywords_similarity_weight"] != float64(0.3) {
		t.Fatalf("rejected update persisted: %v", config)
	}
}

func TestRESTSearchCompletionRequestContract(t *testing.T) {
	name := uniqueName("completion_request")
	id := createRESTSearch(t, name)
	path := "/searches/" + id + "/completions"

	t.Run("question_required", func(t *testing.T) {
		for _, body := range []apiObject{{}, {"question": "   "}, {"messages": []any{}}} {
			result := restCall(t, TestConfig, http.MethodPost, path, body, nil)
			restCode(t, result, http.StatusOK, 101)
			if message, _ := result.JSON["message"].(string); !strings.Contains(message, "question is required") {
				t.Fatalf("message = %q, want %q", message, "question is required")
			}
		}
	})

	// Each alias must reach the next gate (the kb_ids check), which proves the
	// question was resolved rather than rejected.
	t.Run("question_aliases", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			body apiObject
			code int
		}{
			{"query", apiObject{"query": "what is coriander?"}, 102},
			{"openai_messages", apiObject{"messages": []any{apiObject{"role": "user", "content": "what is coriander?"}}}, 102},
			{"question_beats_bad_messages", apiObject{"question": "what is coriander?", "messages": []any{apiObject{"role": "assistant", "content": "hi"}}}, 102},
			{"trailing_assistant_message", apiObject{"messages": []any{apiObject{"role": "assistant", "content": "hi"}}}, 101},
		} {
			t.Run(tc.name, func(t *testing.T) {
				result := restCall(t, TestConfig, http.MethodPost, path, tc.body, nil)
				restCode(t, result, http.StatusOK, tc.code)
				if tc.code == 101 {
					if message, _ := result.JSON["message"].(string); !strings.Contains(message, "not from user") {
						t.Fatalf("message = %q, want a trailing-user rejection", message)
					}
				}
			})
		}
	})

	t.Run("kb_ids_gate", func(t *testing.T) {
		result := restCall(t, TestConfig, http.MethodPost, path, apiObject{"question": "what is coriander?"}, nil)
		restCode(t, result, http.StatusOK, 102)
		if message, _ := result.JSON["message"].(string); !strings.Contains(message, "`kb_ids` is required") {
			t.Fatalf("message = %q, want %q", message, "`kb_ids` is required")
		}
		// A dataset the caller cannot use must stay hidden behind 404.
		result = restCall(t, TestConfig, http.MethodPost, path,
			apiObject{"question": "what is coriander?", "kb_ids": []string{"nonexistent_dataset"}}, nil)
		restCode(t, result, http.StatusOK, 404)
		if result.JSON["message"] != "Resource not found" {
			t.Fatalf("message = %v, want hidden resource-not-found response", result.JSON["message"])
		}
	})

	t.Run("saved_config_supplies_kb_ids", func(t *testing.T) {
		updateRESTSearch(t, id, name, apiObject{"kb_ids": []string{"nonexistent_dataset"}})
		restCode(t, restCall(t, TestConfig, http.MethodPost, path, apiObject{"question": "what is coriander?"}, nil), http.StatusOK, 404)
	})

	t.Run("unknown_search", func(t *testing.T) {
		restCode(t, restCall(t, TestConfig, http.MethodPost, "/searches/"+uniqueName("missing")+"/completions",
			apiObject{"question": "what is coriander?"}, nil), http.StatusOK, 404)
	})
}

// TestRESTSearchCompletionStream locks the SSE contract of the completion
// endpoint: once the request is accepted the handler must stop emitting JSON
// envelopes and stream newline delimited data frames, each of them parseable,
// and it must terminate with either a final answer frame, the trailing
// `{"data": true}` frame, or an error frame that repeats its message in the
// answer. Retrieval and generation failures are tolerated because they depend
// on the configured chat model; the frame shape is not.
func TestRESTSearchCompletionStream(t *testing.T) {
	id := createRESTSearch(t, uniqueName("completion_stream"))
	datasetID := createRESTSearchDataset(t)

	result := restCall(t, TestConfig, http.MethodPost, "/searches/"+id+"/completions",
		apiObject{"question": "what is coriander?", "kb_ids": []string{datasetID}}, nil)
	if result.Status != http.StatusOK || !strings.Contains(result.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("accepted completion must stream SSE: status=%d content-type=%q body=%s",
			result.Status, result.Header.Get("Content-Type"), result.Body)
	}
	events, err := parseSSE(bytes.NewReader(result.Body))
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("stream carried no event")
	}

	var answer strings.Builder
	terminal, errored := false, false
	for i, event := range events {
		var payload apiObject
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			t.Fatalf("SSE frame %d is not a JSON envelope: %v (%q)", i, err, event.Data)
		}
		code, ok := payload["code"].(float64)
		if !ok {
			t.Fatalf("SSE frame %d has no numeric code: %v", i, payload)
		}
		// The trailing frame is `data: {"code":0,"message":"","data":true}`.
		if done, isBool := payload["data"].(bool); isBool {
			if !done || code != 0 {
				t.Fatalf("terminal frame %d = %v, want code 0 with data true", i, payload)
			}
			terminal = true
			continue
		}
		data := object(t, payload["data"])
		if code != 0 {
			errored = true
			message, _ := payload["message"].(string)
			if message == "" {
				t.Fatalf("error frame %d has no message: %v", i, payload)
			}
			if data["answer"] != "**ERROR**: "+message {
				t.Fatalf("error frame %d answer = %v, want the message echoed with the **ERROR** prefix", i, data["answer"])
			}
			t.Logf("completion stream degraded to an application error: %s", message)
			continue
		}
		if value, ok := data["answer"].(string); ok {
			answer.WriteString(value)
		}
		if data["final"] == true {
			terminal = true
		}
	}
	if !terminal && !errored {
		t.Fatalf("stream ended without a terminal frame: %d events", len(events))
	}
	if !errored && answer.Len() == 0 {
		t.Fatal("successful stream produced no answer text")
	}
}

func TestRESTSearchAuthorization(t *testing.T) {
	id := createRESTSearch(t, uniqueName("auth"))
	anonymous := *TestConfig
	anonymous.APIKey = ""
	for _, tc := range []struct {
		method string
		path   string
		body   apiObject
	}{
		{http.MethodGet, "/searches", nil},
		{http.MethodPost, "/searches", apiObject{"name": uniqueName("denied")}},
		{http.MethodGet, "/searches/" + id, nil},
		{http.MethodPut, "/searches/" + id, apiObject{"name": "denied", "search_config": apiObject{}}},
		{http.MethodDelete, "/searches/" + id, nil},
		{http.MethodPost, "/searches/" + id + "/completions", apiObject{"question": "denied"}},
	} {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			restCode(t, restCall(t, &anonymous, tc.method, tc.path, tc.body, nil), http.StatusUnauthorized, 401)
		})
	}
	if got := getRESTSearch(t, id)["name"]; got == "denied" {
		t.Fatal("unauthenticated update persisted")
	}
	t.Run("foreign_owner", func(t *testing.T) {
		token := os.Getenv("RAGFLOW_TEST_OTHER_API_KEY")
		if token == "" {
			t.Skip("set RAGFLOW_TEST_OTHER_API_KEY for an unrelated user to test ownership")
		}
		other := *TestConfig
		other.APIKey = token
		for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
			restCode(t, restCall(t, &other, method, "/searches/"+id,
				apiObject{"name": "denied", "search_config": apiObject{}}, nil), http.StatusOK, 404)
		}
		restCode(t, restCall(t, &other, http.MethodPost, "/searches/"+id+"/completions",
			apiObject{"question": "denied"}, nil), http.StatusOK, 404)
		restOK(t, restCall(t, TestConfig, http.MethodGet, "/searches/"+id, nil, nil))
	})
}

func TestRESTSearchValidation(t *testing.T) {
	t.Run("update_invalid_search_id", func(t *testing.T) {
		result := restCall(t, TestConfig, http.MethodPut, "/searches/invalid_search_id",
			apiObject{"name": "invalid", "search_config": apiObject{}}, nil)
		restCode(t, result, http.StatusOK, 404)
		if result.JSON["message"] != "Resource not found" {
			t.Fatalf("message = %v, want hidden resource-not-found response", result.JSON["message"])
		}
	})
	t.Run("update_requires_name_and_search_config", func(t *testing.T) {
		result := restCall(t, TestConfig, http.MethodPut, "/searches/invalid_search_id", apiObject{}, nil)
		restCode(t, result, http.StatusBadRequest, 400)
	})
}
