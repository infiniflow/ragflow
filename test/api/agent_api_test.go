//go:build integration

package api

import (
	"bytes"
	"encoding/json/v2"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
)

func echoAgentDSL() apiObject {
	return apiObject{
		"components": apiObject{
			"begin": apiObject{
				"obj":        apiObject{"component_name": "Begin", "params": apiObject{}},
				"downstream": []any{"message"}, "upstream": []any{},
			},
			"message": apiObject{
				"obj":        apiObject{"component_name": "Message", "params": apiObject{"content": []any{"{sys.query}"}}},
				"downstream": []any{}, "upstream": []any{"begin"},
			},
		},
		"history": []any{}, "retrieval": []any{}, "path": []any{}, "variables": apiObject{},
		"globals": apiObject{"sys.query": "", "sys.user_id": "", "sys.conversation_turns": 0, "sys.files": []any{}},
	}
}

func createRESTAgent(t *testing.T, title string, dsl apiObject) string {
	t.Helper()
	response := restCall(t, TestConfig, http.MethodPost, "/agents", apiObject{"title": title, "dsl": dsl}, nil)
	if data, ok := response.JSON["data"].(map[string]any); ok {
		if id, ok := data["id"].(string); ok && id != "" {
			cleanupResource(t, TestConfig, "agents", id)
		}
	}
	envelope := restOK(t, response)
	id, ok := object(t, envelope["data"])["id"].(string)
	if !ok || id == "" {
		t.Fatalf("created agent has no ID: %v", envelope)
	}
	return id
}

func createRESTAgentSession(t *testing.T, agentID, name string) string {
	t.Helper()
	path := "/agents/" + agentID + "/sessions"
	response := restCall(t, TestConfig, http.MethodPost, path, apiObject{"name": name}, nil)
	if data, ok := response.JSON["data"].(map[string]any); ok {
		if id, ok := data["id"].(string); ok && id != "" {
			t.Cleanup(func() {
				restOK(t, restCall(t, TestConfig, http.MethodDelete, path+"/"+id, nil, nil))
			})
		}
	}
	envelope := restOK(t, response)
	id, ok := object(t, envelope["data"])["id"].(string)
	if !ok || id == "" {
		t.Fatalf("created session has no ID: %v", envelope)
	}
	return id
}

func getRESTAgent(t *testing.T, id string) apiObject {
	t.Helper()
	return object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/agents/"+id, nil, nil))["data"])
}

func TestRESTAgentCRUD(t *testing.T) {
	title := uniqueName("agent_crud")
	id := createRESTAgent(t, title, echoAgentDSL())
	path := "/agents/" + id
	if got := getRESTAgent(t, id); got["id"] != id || got["title"] != title {
		t.Fatalf("GET did not persist created agent: %v", got)
	}
	duplicateID := createRESTAgent(t, title, echoAgentDSL())
	if got := getRESTAgent(t, duplicateID)["title"]; got != title+"(1)" {
		t.Fatalf("duplicate title = %v, want %s(1)", got, title)
	}

	for _, tc := range []struct {
		name  string
		query apiObject
		want  int
	}{
		{"keyword_filter", apiObject{"keywords": title, "canvas_category": "agent_canvas", "orderby": "title", "desc": false}, 2},
		{"first_page", apiObject{"keywords": title, "canvas_category": "agent_canvas", "page": 1, "page_size": 1, "orderby": "title", "desc": false}, 1},
		{"second_page", apiObject{"keywords": title, "canvas_category": "agent_canvas", "page": 2, "page_size": 1, "orderby": "title", "desc": false}, 1},
		{"missing_keyword", apiObject{"keywords": uniqueName("missing_agent"), "canvas_category": "agent_canvas"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/agents", nil, tc.query))["data"])
			items := array(t, data["canvas"])
			if len(items) != tc.want {
				t.Fatalf("canvas count = %d, want %d; data=%v", len(items), tc.want, data)
			}
			wantTotal := float64(2)
			if tc.want == 0 {
				wantTotal = 0
			}
			if data["total"] != wantTotal {
				t.Fatalf("filtered total = %v, want %v", data["total"], wantTotal)
			}
			if len(items) > 0 && tc.name != "second_page" && object(t, items[0])["id"] != id {
				t.Fatalf("ascending title order first agent = %v, want %s", items[0], id)
			}
			if tc.name == "second_page" && object(t, items[0])["id"] != duplicateID {
				t.Fatalf("second page = %v, want duplicate %s", items, duplicateID)
			}
		})
	}

	newTitle := uniqueName("agent_updated")
	restOK(t, restCall(t, TestConfig, http.MethodPut, path, apiObject{"title": newTitle, "description": "persisted update"}, nil))
	got := getRESTAgent(t, id)
	if got["title"] != newTitle || got["description"] != "persisted update" {
		t.Fatalf("partial update did not persist: %v", got)
	}
	if _, ok := got["dsl"].(map[string]any); !ok {
		t.Fatalf("partial update discarded DSL: %v", got)
	}
	t.Run("duplicate_update_is_atomic", func(t *testing.T) {
		restCode(t, restCall(t, TestConfig, http.MethodPut, path,
			apiObject{"title": title + "(1)", "description": "must not persist"}, nil), http.StatusOK, 102)
		after := getRESTAgent(t, id)
		if after["title"] != newTitle || after["description"] != "persisted update" {
			t.Fatalf("rejected duplicate update mutated agent: %v", after)
		}
	})
	restOK(t, restCall(t, TestConfig, http.MethodDelete, path, nil, nil))
	restCode(t, restCall(t, TestConfig, http.MethodGet, path, nil, nil), http.StatusOK, 103)
	restCode(t, restCall(t, TestConfig, http.MethodDelete, path, nil, nil), http.StatusOK, 103)
}

func TestRESTAgentValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		body apiObject
		text string
	}{
		{"missing_dsl", apiObject{"title": uniqueName("no_dsl")}, "no dsl data"},
		{"missing_title", apiObject{"dsl": echoAgentDSL()}, "no title"},
		{"empty_title", apiObject{"title": "", "dsl": echoAgentDSL()}, "no title"},
		{"whitespace_title", apiObject{"title": " \t ", "dsl": echoAgentDSL()}, "no title"},
		{"null_dsl", apiObject{"title": uniqueName("null_dsl"), "dsl": nil}, "no dsl data"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := restCall(t, TestConfig, http.MethodPost, "/agents", tc.body, nil)
			restCode(t, result, http.StatusOK, 101)
			if message, _ := result.JSON["message"].(string); !strings.Contains(strings.ToLower(message), tc.text) {
				t.Fatalf("validation message = %q, want %q", message, tc.text)
			}
		})
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		t.Run("missing_agent_"+method, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, method, "/agents/"+uniqueName("missing"), apiObject{}, nil), http.StatusOK, 103)
		})
	}
	id := createRESTAgent(t, uniqueName("invalid_dsl"), echoAgentDSL())
	before := getRESTAgent(t, id)
	restCode(t, restCall(t, TestConfig, http.MethodPut, "/agents/"+id, apiObject{"dsl": "invalid"}, nil), http.StatusOK, 102)
	after := getRESTAgent(t, id)
	if !reflect.DeepEqual(before["dsl"], after["dsl"]) {
		t.Fatalf("rejected DSL update changed graph: before=%v after=%v", before["dsl"], after["dsl"])
	}
}

func TestRESTAgentSessions(t *testing.T) {
	id := createRESTAgent(t, uniqueName("agent_sessions"), echoAgentDSL())
	otherID := createRESTAgent(t, uniqueName("other_agent"), echoAgentDSL())
	sessionID := createRESTAgentSession(t, id, "first_session")
	secondID := createRESTAgentSession(t, id, "second_session")
	path := "/agents/" + id + "/sessions"
	session := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, path+"/"+sessionID, nil, nil))["data"])
	if session["id"] != sessionID || session["name"] != "first_session" {
		t.Fatalf("session creation did not persist: %v", session)
	}
	envelope := restOK(t, restCall(t, TestConfig, http.MethodGet, path, nil, apiObject{"page": 1, "page_size": 1, "keywords": "first_session"}))
	items := array(t, envelope["data"])
	if len(items) != 1 || object(t, items[0])["id"] != sessionID || envelope["total"] != float64(1) {
		t.Fatalf("session filtering/pagination did not isolate first session: %v", envelope)
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/agents/"+otherID+"/sessions/"+sessionID, nil, nil), http.StatusOK, 404)
	wrongDelete := restOK(t, restCall(t, TestConfig, http.MethodDelete, "/agents/"+otherID+"/sessions/"+sessionID, nil, nil))
	if wrongDelete["data"] != false {
		t.Fatalf("cross-agent session delete must be a no-op: %v", wrongDelete)
	}
	restOK(t, restCall(t, TestConfig, http.MethodGet, path+"/"+sessionID, nil, nil))
	deleted := restOK(t, restCall(t, TestConfig, http.MethodDelete, path+"/"+sessionID, nil, nil))
	if deleted["data"] != true {
		t.Fatalf("session deletion returned %v", deleted)
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, path+"/"+sessionID, nil, nil), http.StatusOK, 404)
	repeated := restOK(t, restCall(t, TestConfig, http.MethodDelete, path+"/"+sessionID, nil, nil))
	if repeated["data"] != false {
		t.Fatalf("repeated deletion must be a no-op: %v", repeated)
	}
	restOK(t, restCall(t, TestConfig, http.MethodGet, path+"/"+secondID, nil, nil))
	for _, tc := range []struct {
		method string
		code   int
	}{{http.MethodGet, 103}, {http.MethodPost, 103}, {http.MethodDelete, 102}} {
		t.Run("missing_agent_"+tc.method, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, tc.method, "/agents/"+uniqueName("missing")+"/sessions", apiObject{}, nil), http.StatusOK, tc.code)
		})
	}
	t.Run("invalid_date_filter", func(t *testing.T) {
		restCode(t, restCall(t, TestConfig, http.MethodGet, path, nil, apiObject{"from_date": "not-a-date"}), http.StatusOK, 101)
	})
}

func TestRESTAgentCompletionValidation(t *testing.T) {
	id := createRESTAgent(t, uniqueName("completion_validation"), echoAgentDSL())
	for _, tc := range []struct {
		name string
		body apiObject
		code int
	}{
		{"missing_agent", apiObject{"query": "hello", "stream": false}, 101},
		{"empty_openai_messages", apiObject{"agent_id": id, "openai-compatible": true, "messages": []any{}}, 102},
		{"unknown_agent", apiObject{"agent_id": uniqueName("missing"), "query": "hello"}, 103},
		{"unknown_session", apiObject{"agent_id": id, "session_id": uniqueName("missing"), "query": "hello"}, 103},
	} {
		t.Run(tc.name, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, http.MethodPost, "/agents/chat/completions", tc.body, nil), http.StatusOK, tc.code)
		})
	}
}

func TestRESTAgentCompletions(t *testing.T) {
	for _, mode := range []struct {
		name   string
		openAI bool
		stream bool
	}{
		{"native_nonstream", false, false},
		{"native_stream", false, true},
		{"openai_nonstream", true, false},
		{"openai_stream", true, true},
	} {
		t.Run(mode.name, func(t *testing.T) {
			id := createRESTAgent(t, uniqueName(mode.name), echoAgentDSL())
			sessionID := createRESTAgentSession(t, id, mode.name)
			query := uniqueName("echo")
			body := apiObject{"agent_id": id, "session_id": sessionID, "stream": mode.stream}
			if mode.openAI {
				body["openai-compatible"] = true
				body["model"] = "agent"
				body["messages"] = []any{apiObject{"role": "user", "content": query}}
			} else {
				body["query"] = query
				body["return_trace"] = true
			}
			result := restCall(t, TestConfig, http.MethodPost, "/agents/chat/completions", body, nil)
			if mode.stream {
				assertRESTAgentStream(t, result, sessionID, query, mode.openAI)
			} else if mode.openAI {
				if result.Status != http.StatusOK || result.JSON["object"] != "chat.completion" || result.JSON["id"] != sessionID {
					t.Fatalf("invalid OpenAI completion: %s", result.Body)
				}
				choices := array(t, result.JSON["choices"])
				if len(choices) != 1 {
					t.Fatalf("choices = %v, want one", choices)
				}
				choice := object(t, choices[0])
				message := object(t, choice["message"])
				if choice["finish_reason"] != "stop" || message["role"] != "assistant" || message["content"] != query {
					t.Fatalf("invalid OpenAI answer: %v", choice)
				}
				assertRESTAgentUsage(t, result.JSON["usage"])
			} else {
				data := object(t, restOK(t, result)["data"])
				if data["session_id"] != sessionID || object(t, data["data"])["content"] != query {
					t.Fatalf("invalid native answer: %v", data)
				}
				if data["event"] != "message_end" {
					t.Fatalf("native final event = %v, want message_end", data["event"])
				}
			}
			persisted := object(t, restOK(t, restCall(t, TestConfig, http.MethodGet, "/agents/"+id+"/sessions/"+sessionID, nil, nil))["data"])
			messages := array(t, persisted["message"])
			if len(messages) < 2 {
				t.Fatalf("completion did not persist conversation: %v", messages)
			}
			last := object(t, messages[len(messages)-1])
			if last["role"] != "assistant" || last["content"] != query {
				t.Fatalf("persisted assistant output = %v, want %s", last, query)
			}
		})
	}
}

func assertRESTAgentUsage(t *testing.T, value any) {
	t.Helper()
	usage := object(t, value)
	prompt, pOK := usage["prompt_tokens"].(float64)
	completion, cOK := usage["completion_tokens"].(float64)
	total, tOK := usage["total_tokens"].(float64)
	if !pOK || !cOK || !tOK || prompt <= 0 || completion <= 0 || total != prompt+completion {
		t.Fatalf("invalid token usage: %v", usage)
	}
}

func assertRESTAgentStream(t *testing.T, result restResult, sessionID, query string, openAI bool) {
	t.Helper()
	if result.Status != http.StatusOK || !strings.Contains(result.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected SSE response, status=%d content-type=%s body=%s", result.Status, result.Header.Get("Content-Type"), result.Body)
	}
	events, err := parseSSE(bytes.NewReader(result.Body))
	if err != nil {
		t.Fatalf("parse SSE: %v", err)
	}
	if len(events) < 2 || events[len(events)-1].Data != "[DONE]" {
		t.Fatalf("stream lacks final [DONE]: %v", events)
	}
	var output strings.Builder
	finished, messageEnd := false, false
	for i, event := range events[:len(events)-1] {
		if event.Data == "[DONE]" {
			t.Fatalf("premature [DONE] at frame %d", i)
		}
		var payload apiObject
		if err := json.Unmarshal([]byte(event.Data), &payload); err != nil {
			t.Fatalf("invalid JSON at SSE frame %d: %v", i, err)
		}
		if code, ok := payload["code"].(float64); ok && code != 0 {
			t.Fatalf("SSE application error at frame %d: %v", i, payload)
		}
		if openAI {
			if payload["object"] != "chat.completion.chunk" || payload["id"] != sessionID {
				t.Fatalf("invalid OpenAI frame %d: %v", i, payload)
			}
			choices := array(t, payload["choices"])
			if len(choices) != 1 {
				t.Fatalf("invalid choices in frame %d: %v", i, payload)
			}
			choice := object(t, choices[0])
			delta := object(t, choice["delta"])
			if value, ok := delta["content"].(string); ok {
				output.WriteString(value)
			}
			if choice["finish_reason"] == "stop" {
				if i != len(events)-2 {
					t.Fatalf("stop chunk is not final JSON frame: %d", i)
				}
				assertRESTAgentUsage(t, payload["usage"])
				finished = true
			}
		} else {
			if payload["session_id"] != sessionID {
				t.Fatalf("frame %d lost session identity: %v", i, payload)
			}
			kind, ok := payload["event"].(string)
			if !ok || kind == "" {
				t.Fatalf("frame %d has no event type: %v", i, payload)
			}
			data := object(t, payload["data"])
			if kind == "message" {
				if value, ok := data["content"].(string); ok {
					output.WriteString(value)
				}
			}
			if kind == "message_end" {
				messageEnd = true
			}
			if kind == "workflow_finished" {
				finished = true
			}
		}
	}
	if !finished || (!openAI && !messageEnd) || output.String() != query {
		t.Fatalf("incomplete stream: finished=%v message_end=%v content=%q want=%q", finished, messageEnd, output.String(), query)
	}
}

func TestRESTAgentSupport(t *testing.T) {
	id := createRESTAgent(t, uniqueName("support"), echoAgentDSL())
	for _, tc := range []struct {
		name string
		path string
	}{
		{"prompts", "/agents/prompts"},
		{"templates", "/agents/templates"},
		{"versions", "/agents/" + id + "/versions"},
		{"missing_logs", "/agents/" + id + "/logs/" + uniqueName("missing_message")},
		{"webhook_empty", "/agents/" + id + "/webhook/logs"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := apiObject(nil)
			if tc.name == "webhook_empty" {
				query = apiObject{"since_ts": 0}
			}
			envelope := restOK(t, restCall(t, TestConfig, http.MethodGet, tc.path, nil, query))
			switch tc.name {
			case "prompts":
				data := object(t, envelope["data"])
				for _, key := range []string{"task_analysis", "output_format", "citation_guidelines", "few_shots_examples"} {
					if value, ok := data[key].(string); !ok || value == "" {
						t.Fatalf("prompt %s must be nonempty: %v", key, data)
					}
				}
			case "templates", "versions":
				array(t, envelope["data"])
			case "missing_logs":
				if len(object(t, envelope["data"])) != 0 {
					t.Fatalf("missing message must return an empty object: %v", envelope)
				}
			case "webhook_empty":
				data := object(t, envelope["data"])
				if len(array(t, data["events"])) != 0 || data["finished"] != false {
					t.Fatalf("fresh agent has webhook events: %v", data)
				}
				if _, ok := data["next_since_ts"].(float64); !ok {
					t.Fatalf("missing numeric poll cursor: %v", data)
				}
			}
		})
	}
	for _, suffix := range []string{"/versions", "/logs/missing"} {
		t.Run("missing_agent"+suffix, func(t *testing.T) {
			restCode(t, restCall(t, TestConfig, http.MethodGet, "/agents/"+uniqueName("missing")+suffix, nil, nil), http.StatusOK, 103)
		})
	}
	restCode(t, restCall(t, TestConfig, http.MethodGet, "/agents/"+uniqueName("missing")+"/webhook/logs", nil, nil), http.StatusOK, 102)
	for _, body := range []apiObject{{}, {"db_type": "mysql"}, {"db_type": "mysql", "host": "localhost", "port": 3306}} {
		result := restCall(t, TestConfig, http.MethodPost, "/agents/test_db_connection", body, nil)
		restCode(t, result, http.StatusOK, 101)
		if message, _ := result.JSON["message"].(string); !strings.Contains(message, "required argument are missing") {
			t.Fatalf("missing DB fields did not produce validation error: %v", result.JSON)
		}
	}
}

func TestRESTAgentAuthorization(t *testing.T) {
	id := createRESTAgent(t, uniqueName("auth"), echoAgentDSL())
	sessionID := createRESTAgentSession(t, id, "private_session")
	anonymous := *TestConfig
	anonymous.APIKey = ""
	for _, tc := range []struct {
		method string
		path   string
		body   apiObject
	}{
		{http.MethodGet, "/agents", nil},
		{http.MethodPost, "/agents", apiObject{"title": uniqueName("denied"), "dsl": echoAgentDSL()}},
		{http.MethodGet, "/agents/" + id, nil},
		{http.MethodPut, "/agents/" + id, apiObject{"title": "denied"}},
		{http.MethodDelete, "/agents/" + id, nil},
		{http.MethodGet, "/agents/prompts", nil},
		{http.MethodGet, "/agents/templates", nil},
		{http.MethodGet, "/agents/" + id + "/versions", nil},
		{http.MethodGet, "/agents/" + id + "/sessions", nil},
		{http.MethodPost, "/agents/" + id + "/sessions", apiObject{"name": "denied"}},
		{http.MethodGet, "/agents/" + id + "/sessions/" + sessionID, nil},
		{http.MethodDelete, "/agents/" + id + "/sessions/" + sessionID, nil},
		{http.MethodGet, "/agents/" + id + "/logs/missing", nil},
		{http.MethodGet, "/agents/" + id + "/webhook/logs", nil},
		{http.MethodPost, "/agents/test_db_connection", apiObject{}},
		{http.MethodPost, "/agents/chat/completions", apiObject{"agent_id": id, "query": "denied"}},
	} {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			restCode(t, restCall(t, &anonymous, tc.method, tc.path, tc.body, nil), http.StatusUnauthorized, 401)
		})
	}
	if got := getRESTAgent(t, id)["title"]; got == "denied" {
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
			restCode(t, restCall(t, &other, method, "/agents/"+id, apiObject{"title": "denied"}, nil), http.StatusOK, 103)
		}
		for _, method := range []string{http.MethodGet, http.MethodDelete} {
			restCode(t, restCall(t, &other, method, "/agents/"+id+"/sessions/"+sessionID, nil, nil), http.StatusOK, 103)
		}
		restCode(t, restCall(t, &other, http.MethodPost, "/agents/"+id+"/sessions", apiObject{"name": "denied"}, nil), http.StatusOK, 103)
		restCode(t, restCall(t, &other, http.MethodPost, "/agents/chat/completions", apiObject{"agent_id": id, "query": "denied"}, nil), http.StatusOK, 103)
		restOK(t, restCall(t, TestConfig, http.MethodGet, "/agents/"+id+"/sessions/"+sessionID, nil, nil))
	})
}

func TestRESTAgentV2DSLRoundTrip(t *testing.T) {
	dsl := echoAgentDSL()
	dsl["components"] = apiObject{
		"begin": apiObject{
			"obj":        apiObject{"component_name": "Begin", "params": apiObject{}},
			"downstream": []any{"answer:0"}, "upstream": []any{},
		},
		"answer:0": apiObject{
			"obj":        apiObject{"component_name": "Answer", "params": apiObject{}},
			"downstream": []any{}, "upstream": []any{"begin"},
		},
	}
	dsl["variables"] = []any{}
	dsl["graph"] = apiObject{
		"nodes": []any{
			apiObject{"id": "begin", "type": "beginNode", "position": apiObject{"x": 50, "y": 200}, "data": apiObject{"label": "Begin", "name": "begin"}},
			apiObject{"id": "answer:0", "type": "messageNode", "position": apiObject{"x": 400, "y": 200}, "data": apiObject{"label": "Answer", "name": "answer"}},
		},
		"edges": []any{apiObject{"id": "xy-edge__begin-answer:0", "source": "begin", "target": "answer:0", "sourceHandle": "end", "targetHandle": "start"}},
	}
	id := createRESTAgent(t, uniqueName("v2_dsl"), dsl)
	assertGraph := func(t *testing.T, want apiObject) {
		t.Helper()
		got := object(t, getRESTAgent(t, id)["dsl"])
		if _, exists := got["_layout"]; exists {
			t.Fatalf("v2 DSL exposed obsolete _layout: %v", got)
		}
		// JSON-normalize expectations so integer coordinates compare with decoded numbers.
		raw, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		var normalized apiObject
		if err := json.Unmarshal(raw, &normalized); err != nil {
			t.Fatal(err)
		}
		// NormalizeForCanvas intentionally repairs the Python fixture's inverted handles.
		edge := object(t, array(t, object(t, normalized["graph"])["edges"])[0])
		edge["sourceHandle"], edge["targetHandle"] = "start", "end"
		for _, key := range []string{"graph", "components"} {
			if !reflect.DeepEqual(got[key], normalized[key]) {
				t.Fatalf("%s changed during v2 DSL round trip: got=%v want=%v", key, got[key], normalized[key])
			}
		}
	}
	assertGraph(t, dsl)
	graph := object(t, dsl["graph"])
	nodes := array(t, graph["nodes"])
	object(t, nodes[0])["position"] = apiObject{"x": 777, "y": 888}
	restOK(t, restCall(t, TestConfig, http.MethodPut, "/agents/"+id, apiObject{"dsl": dsl}, nil))
	assertGraph(t, dsl)
}
