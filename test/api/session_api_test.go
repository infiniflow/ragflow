//go:build integration

package api

import (
	"encoding/json/v2"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func createRESTSession(t *testing.T, chatID, name string) apiObject {
	t.Helper()
	result := restOK(t, restCall(t, TestConfig, http.MethodPost, "/chats/"+chatID+"/sessions", apiObject{"name": name}, nil))
	session := object(t, result["data"])
	if session["id"] == "" || session["chat_id"] != chatID {
		t.Fatalf("session response lost identity: %v", session)
	}
	return session
}

func sessionIDs(t *testing.T, value any) []string {
	t.Helper()
	items := array(t, value)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id, ok := object(t, item)["id"].(string)
		if !ok || id == "" {
			t.Fatalf("session lacks id: %v", item)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestRESTSessionLifecyclePersistsAndDeletes(t *testing.T) {
	chat := createRESTChat(t, nil)
	chatID := chatID(t, chat)
	session := createRESTSession(t, chatID, uniqueName("session"))
	sessionID := session["id"].(string)

	listed := restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions", nil, apiObject{"page_size": 20}))
	items := array(t, listed["data"])
	if !slicesContainsID(t, items, sessionID) {
		t.Fatalf("created session missing from list: %v", listed)
	}
	fetched := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions/"+sessionID, nil, nil))["data"])
	if fetched["id"] != sessionID || fetched["chat_id"] != chatID {
		t.Fatalf("get returned wrong session: %v", fetched)
	}

	newName := uniqueName("renamed")
	updated := object(t, restOK(t, restCall(t, TestConfig, http.MethodPatch, "/chats/"+chatID+"/sessions/"+sessionID, apiObject{"name": newName}, nil))["data"])
	if updated["name"] != newName {
		t.Fatalf("update response name=%v want=%s", updated["name"], newName)
	}
	persisted := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions/"+sessionID, nil, nil))["data"])
	if persisted["name"] != newName {
		t.Fatalf("updated name was not persisted: %v", persisted)
	}

	deleted := restOK(t, restCall(t, TestConfig, http.MethodDelete, "/chats/"+chatID+"/sessions", apiObject{"ids": []string{sessionID}}, nil))["data"]
	if deleted != true {
		t.Fatalf("delete result=%v", deleted)
	}
	remaining := restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions", nil, apiObject{"page_size": 20}))
	if slicesContainsID(t, array(t, remaining["data"]), sessionID) {
		t.Fatal("deleted session remained listable")
	}
	repeated := restCall(t, TestConfig, http.MethodDelete, "/chats/"+chatID+"/sessions", apiObject{"ids": []string{sessionID}}, nil)
	restCode(t, repeated, http.StatusOK, 102)
}

func slicesContainsID(t *testing.T, items []any, id string) bool {
	t.Helper()
	for _, item := range items {
		if object(t, item)["id"] == id {
			return true
		}
	}
	return false
}

func TestRESTSessionValidationAndIsolation(t *testing.T) {
	chat := createRESTChat(t, nil)
	chatID := chatID(t, chat)
	for _, value := range []any{"", " \t", 1, true} {
		before := restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions", nil, apiObject{"page_size": 20}))
		result := restCall(t, TestConfig, http.MethodPost, "/chats/"+chatID+"/sessions", apiObject{"name": value}, nil)
		restCode(t, result, http.StatusOK, 102)
		after := restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions", nil, apiObject{"page_size": 20}))
		if !reflect.DeepEqual(before["data"], after["data"]) {
			t.Fatalf("invalid name mutated sessions: value=%v before=%v after=%v", value, before["data"], after["data"])
		}
	}
	session := createRESTSession(t, chatID, "original")
	for _, value := range []any{"", " \t", 1} {
		result := restCall(t, TestConfig, http.MethodPatch, "/chats/"+chatID+"/sessions/"+session["id"].(string), apiObject{"name": value}, nil)
		restCode(t, result, http.StatusOK, 102)
		persisted := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions/"+session["id"].(string), nil, nil))["data"])
		if persisted["name"] != "original" {
			t.Fatalf("invalid update changed name to %v", persisted["name"])
		}
	}
	other := createRESTChat(t, nil)
	otherID := other["id"].(string)
	cross := restCall(t, TestConfig, http.MethodGet, "/chats/"+otherID+"/sessions/"+session["id"].(string), nil, nil)
	restCode(t, cross, http.StatusOK, 102)
	if cross.JSON["message"] != "session does not belong to this chat" {
		t.Fatalf("cross-chat session error=%v", cross.JSON)
	}
	invalidChat := restCall(t, TestConfig, http.MethodPost, "/chats/"+uniqueName("missing")+"/sessions", apiObject{"name": "x"}, nil)
	restCode(t, invalidChat, http.StatusOK, 404)
}

func TestRESTSessionAuthorizationAndFiltering(t *testing.T) {
	chat := createRESTChat(t, nil)
	chatID := chatID(t, chat)
	session := createRESTSession(t, chatID, "private")
	for _, token := range []string{"", "invalid-api-key"} {
		config := *TestConfig
		config.APIKey = token
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/chats/" + chatID + "/sessions", nil},
			{http.MethodPost, "/chats/" + chatID + "/sessions", apiObject{"name": "denied"}},
			{http.MethodPatch, "/chats/" + chatID + "/sessions/" + session["id"].(string), apiObject{"name": "denied"}},
			{http.MethodDelete, "/chats/" + chatID + "/sessions", apiObject{"ids": []string{session["id"].(string)}}},
		} {
			result := restCall(t, &config, tc.method, tc.path, tc.body, nil)
			if result.Status != http.StatusUnauthorized || result.JSON["code"] != float64(401) {
				t.Fatalf("unauthorized %s %s: status=%d body=%v", tc.method, tc.path, result.Status, result.JSON)
			}
		}
	}
	if got := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions/"+session["id"].(string), nil, nil))["data"])["name"]; got != "private" {
		t.Fatalf("unauthorized update changed name: %v", got)
	}

	for i := 0; i < 5; i++ {
		createRESTSession(t, chatID, fmt.Sprintf("filter_%d", i))
	}
	filtered := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions", nil, apiObject{"name": "filter_2", "page_size": 20}))["data"])
	if len(filtered) != 1 || object(t, filtered[0])["name"] != "filter_2" {
		t.Fatalf("name filter returned %v", filtered)
	}
	page := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions", nil, apiObject{"page": 2, "page_size": 2, "orderby": "name", "desc": false}))["data"])
	if len(page) != 2 {
		t.Fatalf("page size was not applied: %v", page)
	}
	if object(t, page[0])["name"] != "filter_2" || object(t, page[1])["name"] != "filter_3" {
		t.Fatalf("page offset or ordering was not applied: %v", page)
	}
}

func TestRESTSessionConcurrentCreationAndListing(t *testing.T) {
	chat := createRESTChat(t, nil)
	chatID := chatID(t, chat)
	const count = 10
	results := make([]restResult, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = restCall(t, TestConfig, http.MethodPost, "/chats/"+chatID+"/sessions", apiObject{"name": fmt.Sprintf("concurrent_%d", i)}, nil)
		}(i)
	}
	wg.Wait()
	ids := map[string]struct{}{}
	for i, result := range results {
		if result.Status != http.StatusOK || result.JSON["code"] != float64(0) {
			t.Fatalf("concurrent create %d failed: status=%d body=%v", i, result.Status, result.JSON)
		}
		id := object(t, result.JSON["data"])["id"].(string)
		if _, exists := ids[id]; exists {
			t.Fatalf("duplicate session id %q", id)
		}
		ids[id] = struct{}{}
	}
	listed := array(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/chats/"+chatID+"/sessions", nil, apiObject{"page_size": count + 5}))["data"])
	if len(listed) != count {
		t.Fatalf("concurrent creates lost state: got %d sessions", len(listed))
	}
}

func TestRESTChatCompletionValidationAndStreamContract(t *testing.T) {
	chat := createRESTChat(t, nil)
	chatID := chatID(t, chat)
	missingMessages := restCall(t, TestConfig, http.MethodPost, "/chat/completions", apiObject{"chat_id": chatID, "stream": false}, nil)
	restCode(t, missingMessages, http.StatusOK, 101)
	missingChat := restCall(t, TestConfig, http.MethodPost, "/chat/completions", apiObject{"session_id": "missing", "messages": []apiObject{{"role": "user", "content": "hello"}}, "stream": false}, nil)
	restCode(t, missingChat, http.StatusOK, 102)
	invalidSession := restCall(t, TestConfig, http.MethodPost, "/chat/completions", apiObject{"chat_id": chatID, "session_id": "missing", "messages": []apiObject{{"role": "user", "content": "hello"}}, "stream": false}, nil)
	restCode(t, invalidSession, http.StatusOK, 102)
	if !strings.Contains(fmt.Sprint(invalidSession.JSON["message"]), "Session") {
		t.Fatalf("invalid session error lacked context: %v", invalidSession.JSON)
	}
	completion := restOK(t, restCall(t, TestConfig, http.MethodPost, "/chat/completions", apiObject{
		"chat_id": chatID, "messages": []apiObject{{"role": "user", "content": "hello"}}, "stream": false,
	}, nil))
	completionData := object(t, completion["data"])
	if completionData["session_id"] == "" || completionData["answer"] == nil {
		t.Fatalf("completion omitted answer or generated session identity: %v", completionData)
	}
	stream := restCall(t, TestConfig, http.MethodPost, "/chat/completions", apiObject{
		"chat_id": chatID, "messages": []apiObject{{"role": "user", "content": "hello"}}, "stream": true,
	}, nil)
	if stream.Status != http.StatusOK || !strings.Contains(stream.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream response status/content-type=%d/%q", stream.Status, stream.Header.Get("Content-Type"))
	}
	events, err := parseSSE(strings.NewReader(string(stream.Body)))
	if err != nil || len(events) < 2 {
		t.Fatalf("invalid completion SSE: events=%d err=%v", len(events), err)
	}
	var sawData, sawFinal, sawTerminal bool
	var streamSessionID string
	for i, event := range events {
		var payload apiObject
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			t.Fatalf("SSE event is not JSON: %q", event.Data)
		}
		if payload["code"] != float64(0) {
			t.Fatalf("SSE event returned an error: %v", payload)
		}
		if payload["data"] == true {
			if i != len(events)-1 {
				t.Fatal("SSE terminal event was followed by more events")
			}
			sawTerminal = true
			continue
		}
		data := object(t, payload["data"])
		sessionID, ok := data["session_id"].(string)
		if !ok || sessionID == "" || data["chat_id"] != chatID {
			t.Fatalf("SSE event lost chat/session identity: %v", data)
		}
		if streamSessionID == "" {
			streamSessionID = sessionID
		} else if sessionID != streamSessionID {
			t.Fatalf("SSE session identity changed: got=%s want=%s", sessionID, streamSessionID)
		}
		if _, ok := data["answer"].(string); !ok {
			t.Fatalf("SSE event omitted answer: %v", data)
		}
		sawData = true
		if data["final"] == true {
			sawFinal = true
		}
	}
	if !sawData || !sawFinal || !sawTerminal {
		t.Fatalf("SSE lacked answer, final, or terminal events: %v", events)
	}
}
