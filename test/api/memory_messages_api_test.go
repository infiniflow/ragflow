//go:build integration

package api

import (
	"context"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	memoryEmbeddingModel = "BAAI/bge-small-en-v1.5@Local@Builtin"
	memoryLLMModel       = "glm-4-flash@CI@ZHIPU-AI"
)

func createRESTMemory(t *testing.T, name string) string {
	t.Helper()
	result := restCall(t, TestConfig, http.MethodPost, "/memories", apiObject{
		"name":        name,
		"memory_type": []string{"raw"},
		"embd_id":     memoryEmbeddingModel,
		"llm_id":      memoryLLMModel,
	}, nil)
	data := object(t, restOK(t, result)["data"])
	id, ok := data["id"].(string)
	if !ok || id == "" {
		t.Fatalf("created memory has no id: %v", data)
	}
	t.Cleanup(func() {
		restCode(t, restCall(t, TestConfig, http.MethodGet, "/memories/"+id+"/config", nil, nil), http.StatusOK, 404)
	})
	cleanupResource(t, TestConfig, "memories", id)
	if data["name"] != name || data["embd_id"] != memoryEmbeddingModel || data["llm_id"] != memoryLLMModel {
		t.Fatalf("created memory fields differ from request: %v", data)
	}
	return id
}

func memoryMessages(t *testing.T, result restResult) []any {
	t.Helper()
	data := object(t, restOK(t, result)["data"])
	messages := object(t, data["messages"])
	return array(t, messages["message_list"])
}

func messageID(t *testing.T, message apiObject) string {
	t.Helper()
	value, ok := message["message_id"]
	if !ok {
		t.Fatalf("message has no message_id: %v", message)
	}
	switch id := value.(type) {
	case string:
		return id
	case float64:
		return strconv.FormatInt(int64(id), 10)
	default:
		t.Fatalf("message_id has unexpected type %T: %v", value, value)
		return ""
	}
}

func waitForRESTMemoryMessage(t *testing.T, memoryID, expectedUserID string, expectedStatus bool) apiObject {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var last restResult
	for {
		last = restCallContext(t, ctx, TestConfig, http.MethodGet, "/memories/"+memoryID, nil, nil)
		items := memoryMessages(t, last)
		for _, item := range items {
			message := object(t, item)
			if message["user_id"] == expectedUserID && message["status"] == expectedStatus {
				return message
			}
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("timed out waiting for memory message %q with status=%t; last response=%v", expectedUserID, expectedStatus, last.JSON)
		}
	}
}

func containsMemory(t *testing.T, data apiObject, memoryID string) bool {
	t.Helper()
	for _, item := range array(t, data["memory_list"]) {
		if object(t, item)["id"] == memoryID {
			return true
		}
	}
	return false
}

func TestRESTMemoryAuthorization(t *testing.T) {
	for _, key := range []string{"", TestConfig.InvalidAPIKey} {
		config := *TestConfig
		config.APIKey = key
		for _, tc := range []struct {
			method string
			path   string
		}{
			{http.MethodGet, "/memories"},
			{http.MethodPost, "/memories"},
			{http.MethodPut, "/memories/" + TestConfig.InvalidID},
			{http.MethodDelete, "/memories/" + TestConfig.InvalidID},
			{http.MethodGet, "/messages"},
			{http.MethodGet, "/messages/search"},
			{http.MethodPost, "/messages"},
			{http.MethodPut, "/messages/" + TestConfig.InvalidID + ":1"},
			{http.MethodDelete, "/messages/" + TestConfig.InvalidID + ":1"},
		} {
			t.Run(tc.method+tc.path+"/empty-key-"+strconv.FormatBool(key == ""), func(t *testing.T) {
				result := restCall(t, &config, tc.method, tc.path, nil, nil)
				if result.Status != http.StatusUnauthorized {
					t.Fatalf("HTTP=%d want=%d", result.Status, http.StatusUnauthorized)
				}
				if result.JSON["code"] != float64(http.StatusUnauthorized) {
					t.Fatalf("authorization response=%v", result.JSON)
				}
			})
		}
	}
}

func TestRESTMemoryCRUDConfigAndPersistence(t *testing.T) {
	name := uniqueName("go_memory_crud")
	id := createRESTMemory(t, name)

	list := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/memories", nil, apiObject{
		"keywords":  name,
		"page":      1,
		"page_size": 10,
	}))["data"])
	if !containsMemory(t, list, id) {
		t.Fatalf("created memory %s missing from keyword-filtered list: %v", id, list)
	}

	config := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/memories/"+id+"/config", nil, nil))["data"])
	if config["id"] != id || config["name"] != name || config["storage_type"] != "table" ||
		config["permissions"] != "me" || config["forgetting_policy"] != "FIFO" {
		t.Fatalf("memory config lacks persisted fields: %v", config)
	}

	updatedName := uniqueName("go_memory_updated")
	update := restOK(t, restCall(t, TestConfig, http.MethodPut, "/memories/"+id, apiObject{
		"name":        updatedName,
		"permissions": "me",
	}, nil))
	if object(t, update["data"])["name"] != updatedName {
		t.Fatalf("update response=%v", update)
	}
	persisted := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/memories/"+id+"/config", nil, nil))["data"])
	if persisted["name"] != updatedName || persisted["permissions"] != "me" {
		t.Fatalf("update was not persisted: %v", persisted)
	}

	deleteResult := restCall(t, TestConfig, http.MethodDelete, "/memories/"+id, nil, nil)
	restCode(t, deleteResult, http.StatusOK, 0)
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/memories/"+id+"/config", nil, nil), http.StatusOK, 404)
}

func TestRESTMemoryValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		body apiObject
		code int
	}{
		{"missing_models", apiObject{"name": uniqueName("missing_models"), "memory_type": []string{"raw"}}, 400},
		{"missing_name", apiObject{"memory_type": []string{"raw"}, "embd_id": memoryEmbeddingModel, "llm_id": memoryLLMModel}, 400},
		{"empty_name", apiObject{"name": " ", "memory_type": []string{"raw"}, "embd_id": memoryEmbeddingModel, "llm_id": memoryLLMModel}, 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := restCall(t, TestConfig, http.MethodPost, "/memories", tc.body, nil)
			if result.Status != http.StatusOK || result.JSON["code"] != float64(tc.code) {
				t.Fatalf("validation response HTTP=%d body=%v", result.Status, result.JSON)
			}
		})
	}

	name := uniqueName("memory_validation")
	memoryID := createRESTMemory(t, name)
	result := restCall(t, TestConfig, http.MethodPut, "/memories/"+memoryID, apiObject{"name": " "}, nil)
	if result.Status != http.StatusOK || result.JSON["code"] != float64(101) {
		t.Fatalf("empty name response HTTP=%d body=%v", result.Status, result.JSON)
	}
	persisted := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/memories/"+memoryID+"/config", nil, nil))["data"])
	if persisted["name"] != name {
		t.Fatalf("rejected update changed memory name: %v", persisted)
	}
}

func TestRESTMessageLifecyclePreservesCallerUserID(t *testing.T) {
	memoryID := createRESTMemory(t, uniqueName("go_message_lifecycle"))
	userID := "external-" + uniqueName("user")
	agentID := uniqueName("agent")
	sessionID := uniqueName("session")
	add := restCall(t, TestConfig, http.MethodPost, "/messages", apiObject{
		"memory_id":      []string{memoryID},
		"agent_id":       agentID,
		"session_id":     sessionID,
		"user_id":        userID,
		"user_input":     "what is coriander?",
		"agent_response": "coriander has leaves and seeds",
	}, nil)
	restCode(t, add, http.StatusOK, 0)

	message := waitForRESTMemoryMessage(t, memoryID, userID, true)
	if message["agent_id"] != agentID || message["session_id"] != sessionID || message["user_id"] != userID {
		t.Fatalf("message identity was not preserved: %v", message)
	}
	id := messageID(t, message)
	path := "/messages/" + memoryID + ":" + id

	recent := restOK(t, restCall(t, TestConfig, http.MethodGet, "/messages", nil, apiObject{
		"memory_id": memoryID,
		"limit":     10,
	}))
	recentItems := array(t, recent["data"])
	if len(recentItems) != 1 || messageID(t, object(t, recentItems[0])) != id {
		t.Fatalf("recent message list does not contain only the created message: %v", recentItems)
	}
	content := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, path+"/content", nil, nil))["data"])
	text, ok := content["content"].(string)
	if !ok || !strings.Contains(text, "what is coriander?") || !strings.Contains(text, "coriander has leaves and seeds") ||
		content["user_id"] != userID || content["memory_id"] != memoryID || messageID(t, content) != id {
		t.Fatalf("message content or identity differs from request: %v", content)
	}

	searchQuery := apiObject{"memory_id": memoryID, "query": "coriander", "top_n": 3, "similarity_threshold": 0}
	search := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/messages/search", nil, searchQuery))["data"])
	if len(search) != 1 || messageID(t, object(t, search[0])) != id || object(t, search[0])["user_id"] != userID {
		t.Fatalf("search did not retrieve the created message: %v", search)
	}
	for _, status := range []any{"false", nil} {
		restCode(t, restCall(t, TestConfig, http.MethodPut, path, apiObject{"status": status}, nil), http.StatusOK, 101)
		unchanged := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, path+"/content", nil, nil))["data"])
		if unchanged["status"] != true || unchanged["content"] != text {
			t.Fatalf("rejected status update changed message: %v", unchanged)
		}
	}
	restCode(t, restCall(t, TestConfig, http.MethodPut, path, apiObject{"status": false}, nil), http.StatusOK, 0)
	waitForRESTMemoryMessage(t, memoryID, userID, false)

	if hidden := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/messages/search", nil, searchQuery))["data"]); len(hidden) != 0 {
		t.Fatalf("disabled message remained searchable: %v", hidden)
	}
	restCode(t, restCall(t, TestConfig, http.MethodPut, path, apiObject{"status": true}, nil), http.StatusOK, 0)
	waitForRESTMemoryMessage(t, memoryID, userID, true)
	visible := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/messages", nil, apiObject{"memory_id": memoryID}))["data"])
	if len(visible) != 1 || messageID(t, object(t, visible[0])) != id {
		t.Fatalf("target message was not visible before forgetting: %v", visible)
	}

	restCode(t, restCall(t, TestConfig, http.MethodDelete, path, nil, nil), http.StatusOK, 0)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	for {
		result := restCallContext(t, ctx, TestConfig, http.MethodGet, "/messages", nil, apiObject{
			"memory_id": memoryID,
			"limit":     10,
		})
		items := array(t, restOK(t, result)["data"])
		if len(items) == 0 {
			break
		}
		select {
		case <-time.After(500 * time.Millisecond):
		case <-ctx.Done():
			t.Fatalf("forgotten message %s remained visible: %v", id, items)
		}
	}
	if remaining := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/messages/search", nil, searchQuery))["data"]); len(remaining) != 0 {
		t.Fatalf("forgotten message remained searchable: %v", remaining)
	}
	forgotten := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, path+"/content", nil, nil))["data"])
	if stamp, ok := forgotten["forget_at"].(string); !ok || strings.TrimSpace(stamp) == "" || forgotten["content"] != text {
		t.Fatalf("forget did not persist a timestamp while preserving content: %v", forgotten)
	}
	restCode(t, restCall(t, TestConfig, http.MethodDelete, path, nil, nil), http.StatusOK, 0)
}

func TestRESTMessageValidationAndInvalidIdentifiers(t *testing.T) {
	memoryID := createRESTMemory(t, uniqueName("go_message_validation"))
	for _, query := range []apiObject{
		{"memory_id": memoryID, "query": "coriander"},
		{"query": "coriander"},
	} {
		items := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/messages/search", nil, query))["data"])
		if len(items) != 0 {
			t.Fatalf("search without searchable memories returned messages: %v", items)
		}
	}
	validMessage := apiObject{
		"memory_id": []string{memoryID}, "agent_id": uniqueName("agent"), "session_id": uniqueName("session"),
		"user_id": uniqueName("user"), "user_input": "hello", "agent_response": "hello",
	}
	for _, field := range []string{"memory_id", "agent_id", "session_id", "user_id", "user_input", "agent_response"} {
		t.Run("missing_"+field, func(t *testing.T) {
			body := maps.Clone(validMessage)
			delete(body, field)
			restCode(t, restCall(t, TestConfig, http.MethodPost, "/messages", body, nil), http.StatusOK, 101)
		})
	}
	if items := memoryMessages(t, restCall(t, TestConfig, http.MethodGet, "/memories/"+memoryID, nil, nil)); len(items) != 0 {
		t.Fatalf("rejected message creation persisted messages: %v", items)
	}
	restCode(t, restCall(t, TestConfig, http.MethodDelete, "/messages/missing-memory:1", nil, nil), http.StatusOK, 404)
	for _, tc := range []struct {
		name  string
		query apiObject
		code  int
	}{
		{"missing_memory_ids", nil, 101},
		{"invalid_limit", apiObject{"memory_id": memoryID, "limit": 101}, 101},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := restCall(t, TestConfig, http.MethodGet, "/messages", nil, tc.query)
			restCode(t, result, http.StatusOK, tc.code)
		})
	}
	for _, tc := range []struct {
		name string
		path string
		body apiObject
		code int
	}{
		{"non_boolean_status", "/messages/" + memoryID + ":1", apiObject{"status": "false"}, 101},
		{"bad_message_id", "/messages/" + memoryID + ":not-an-int", apiObject{"status": false}, 101},
		{"missing_message", "/messages/" + memoryID + ":999999/content", nil, 404},
		{"missing_memory", "/messages/missing-memory:1", apiObject{"status": false}, 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			method := http.MethodPut
			if tc.body == nil {
				method = http.MethodGet
			}
			result := restCall(t, TestConfig, method, tc.path, tc.body, nil)
			restCode(t, result, http.StatusOK, tc.code)
		})
	}
	for _, topN := range []int{0, 101} {
		result := restCall(t, TestConfig, http.MethodGet, "/messages/search", nil, apiObject{
			"memory_id": memoryID,
			"query":     "anything",
			"top_n":     topN,
		})
		restCode(t, result, http.StatusOK, 101)
	}
}
