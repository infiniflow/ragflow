//go:build e2e

package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// These saved Agent workflows require genuine inference and a private wire
// observer. The observer records actual vendor calls, not expected examples.
func TestAnySearchAgentExtensionsE2E(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH_EXTENSIONS_E2E") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH_EXTENSIONS_E2E=1 for real Agent discovery and batch")
	}
	required := func(name string) string {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			t.Fatalf("%s is required", name)
		}
		return value
	}
	origin := required("RAGFLOW_TEST_API_URL")
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") || u.User != nil || u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
		t.Fatal("RAGFLOW_TEST_API_URL must be a local HTTP API origin")
	}
	wirePath := required("RAGFLOW_ANYSEARCH_WIRE_RECEIPTS")
	h := &anySearchE2EHTTP{t: t, origin: strings.TrimRight(origin, "/"), token: required("RAGFLOW_TEST_API_TOKEN"), client: &http.Client{Timeout: 10 * time.Minute}}
	model := required("RAGFLOW_TEST_LLM_MODEL")
	observer, base := newAnySearchModelObserver(t, required("RAGFLOW_TEST_LLM_BASE_URL"))
	key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	provider := "/api/v1/providers/OpenAI-API-Compatible"
	h.call(http.MethodPut, "/api/v1/providers", map[string]any{"provider_name": "OpenAI-API-Compatible"})
	instance := fmt.Sprintf("anysearch_extensions_%x", time.Now().UnixNano())
	h.call(http.MethodPost, provider+"/instances", map[string]any{"instance_name": instance, "api_key": "", "base_url": base,
		"model_info": []any{map[string]any{"model_name": model, "model_type": []string{"chat"}, "max_tokens": 16384, "extra": map[string]any{"is_tools": true}}}})
	t.Cleanup(func() {
		h.cleanup(http.MethodDelete, provider+"/instances", map[string]any{"instances": []string{instance}})
	})
	var rows []struct {
		ID       string `json:"model_id"`
		Instance string `json:"instance_name"`
	}
	h.decode(h.call(http.MethodGet, "/api/v1/models?type=chat", nil), &rows)
	modelID := ""
	for _, row := range rows {
		if row.Instance == instance {
			modelID = row.ID
		}
	}
	if modelID == "" {
		t.Fatal("created extension model was not listed")
	}
	const docsQuery = "RAGFlow official documentation"
	const licenseQuery = "RAGFlow GitHub repository software license"
	for _, name := range []string{"discovery", "batch"} {
		t.Run(name, func(t *testing.T) {
			local := *h
			local.t = t
			observer.reset()
			dsl, nodeID := anySearchRealModelDSL("workflow_agent", modelID, key, 2048)
			params := dsl["components"].(map[string]any)[nodeID].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
			params["max_rounds"] = 4
			question := "In one anysearch_search batch call, look up " + docsQuery + " and " + licenseQuery + ". Report one returned source URL for each question."
			params["sys_prompt"] = "Use one anysearch_search call with queries containing exactly two objects, in this order: {\"query\":\"" + docsQuery + "\",\"max_results\":1} and {\"query\":\"" + licenseQuery + "\",\"max_results\":1}. Omit query at the top level, tag and params. After receiving both results, report a returned source URL for each query. Do not repeat a completed call."
			if name == "discovery" {
				question = "Discover the available AnySearch domains, then inspect the code domain and explain the returned documentation and code-search tags and their library/repo parameters."
				params["tools"] = []any{map[string]any{"component_name": "AnySearchDiscover", "params": map[string]any{"api_key": key}}}
				params["sys_prompt"] = "First call anysearch_discover with {} to list domains. After reading that result, call anysearch_discover with {\"domains\":[\"code\"]}. Read both actual results. In your final answer name code.doc, code.snippet, library and repo, explaining their returned meanings and required flags. Do not perform a search or repeat a completed discovery call."
			}
			var created struct {
				ID string `json:"id"`
			}
			local.decode(local.call(http.MethodPost, "/api/v1/agents", map[string]any{"title": "AnySearch " + name + " " + instance, "canvas_category": "agent_canvas", "dsl": dsl}), &created)
			if created.ID == "" {
				t.Fatal("extension workflow was not saved")
			}
			t.Cleanup(func() { local.cleanup(http.MethodDelete, "/api/v1/agents/"+created.ID, nil) })
			t.Cleanup(func() {
				local.cleanup(http.MethodDelete, "/api/v1/agents/"+created.ID+"/sessions", map[string]any{"delete_all": true})
			})
			savedConfig := local.call(http.MethodGet, "/api/v1/agents/"+created.ID, nil)
			if key != "" {
				savedConfig = json.RawMessage(strings.ReplaceAll(string(savedConfig), key, "[REDACTED]"))
			}
			local.receipt("extensions-"+name+"-config", savedConfig)
			started := time.Now().UTC()
			events := local.stream("/api/v1/agents/chat/completions", map[string]any{"agent_id": created.ID, "query": question, "stream": true})
			var answer, session string
			finished := false
			for _, event := range events {
				if id, ok := event["session_id"].(string); ok {
					session = id
				}
				data, _ := event["data"].(map[string]any)
				if event["event"] == "error" {
					t.Fatal("extension workflow emitted an error")
				}
				if event["event"] == "message" {
					text, _ := data["content"].(string)
					answer += text
				}
				finished = finished || (event["event"] == "node_finished" && data["component_id"] == nodeID)
			}
			local.receipt("extensions-"+name+"-run", events)
			local.receipt("extensions-"+name+"-model", observer.snapshot())
			if !finished || session == "" || strings.TrimSpace(answer) == "" {
				t.Fatal("saved extension workflow did not deliver a nonempty answer")
			}
			observer.assertInference(t)
			toolName := "anysearch_search"
			if name == "discovery" {
				toolName = "anysearch_discover"
			}
			replies := anySearchExtensionToolReplies(t, observer, toolName)
			wire := anySearchExtensionWireRows(t, wirePath, started)
			if len(wire) != 2 {
				t.Fatalf("extension made %d observed vendor calls, want exactly two", len(wire))
			}
			if name == "discovery" {
				assertAnySearchDiscoveryExtension(t, replies, wire, answer)
			} else {
				assertAnySearchBatchExtension(t, replies, wire, answer, []string{docsQuery, licenseQuery})
			}
			local.receipt("extensions-"+name+"-wire", wire)
			saved := local.call(http.MethodGet, "/api/v1/agents/"+created.ID+"/sessions/"+session+"?include_reference=true", nil)
			local.receipt("extensions-"+name+"-saved", json.RawMessage(saved))
			var history struct {
				Messages []struct{ Role, Content string } `json:"message"`
			}
			local.decode(saved, &history)
			found := false
			for _, message := range history.Messages {
				found = found || (message.Role == "assistant" && message.Content == answer)
			}
			if !found {
				t.Fatal("saved extension session lost the exact delivered assistant answer")
			}
			t.Log("real saved Agent used actual AnySearch tools; vendor wire records correlate with model tool replies and saved answer")
		})
	}
}

type anySearchExtensionReply struct {
	Arguments json.RawMessage
	Body      json.RawMessage
}

func anySearchExtensionToolReplies(t *testing.T, observer *anySearchModelObserver, toolName string) []anySearchExtensionReply {
	t.Helper()
	replies := map[string]anySearchExtensionReply{}
	for _, raw := range observer.snapshot()["requests"] {
		var decoded struct {
			Messages []struct {
				Role       string          `json:"role"`
				ToolCallID string          `json:"tool_call_id"`
				Content    json.RawMessage `json:"content"`
				ToolCalls  []struct {
					ID       string                           `json:"id"`
					Function struct{ Name, Arguments string } `json:"function"`
				} `json:"tool_calls"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(raw), &decoded) != nil {
			t.Fatal("invalid observed extension model request")
		}
		calls := map[string]string{}
		for _, message := range decoded.Messages {
			for _, call := range message.ToolCalls {
				if call.Function.Name == toolName && call.ID != "" {
					calls[call.ID] = call.Function.Arguments
				}
			}
			arguments, matched := calls[message.ToolCallID]
			if message.Role != "tool" || !matched {
				continue
			}
			var content string
			if json.Unmarshal(message.Content, &content) != nil || !json.Valid([]byte(content)) {
				t.Fatal("extension tool reply was not complete structured data")
			}
			reply := anySearchExtensionReply{Arguments: json.RawMessage(arguments), Body: json.RawMessage(content)}
			if previous, ok := replies[message.ToolCallID]; ok && (string(previous.Arguments) != arguments || string(previous.Body) != content) {
				t.Fatal("one tool-call ID carried inconsistent extension results")
			}
			replies[message.ToolCallID] = reply
		}
	}
	out := make([]anySearchExtensionReply, 0, len(replies))
	for _, reply := range replies {
		out = append(out, reply)
	}
	return out
}

func anySearchExtensionWireRows(t *testing.T, path string, started time.Time) map[string]map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal("cannot read private extension wire receipts")
	}
	defer file.Close()
	rows := map[string]map[string]any{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 8<<20)
	for scanner.Scan() {
		var row map[string]any
		if json.Unmarshal(scanner.Bytes(), &row) != nil {
			t.Fatal("invalid private extension wire receipt")
		}
		at, _ := row["started_utc"].(string)
		when, err := time.Parse(time.RFC3339Nano, at)
		if err != nil || when.Before(started) {
			continue
		}
		id, _ := row["request_id"].(string)
		if id == "" || row["status"] != float64(http.StatusOK) || row["business_code"] != float64(0) {
			t.Fatal("extension wire receipt lacked successful status or request ID")
		}
		if _, exists := rows[id]; exists {
			t.Fatal("extension request ID was reused")
		}
		rows[id] = row
	}
	if scanner.Err() != nil {
		t.Fatal("cannot read complete extension wire receipts")
	}
	return rows
}

func assertAnySearchDiscoveryExtension(t *testing.T, replies []anySearchExtensionReply, wire map[string]map[string]any, answer string) {
	t.Helper()
	if len(replies) != 2 {
		t.Fatal("Agent did not consume exactly two discovery replies")
	}
	list, code := false, false
	for _, reply := range replies {
		var args struct {
			Domains []string `json:"domains"`
		}
		var result struct {
			Error     string          `json:"_ERROR"`
			RequestID string          `json:"request_id"`
			Domains   json.RawMessage `json:"domains"`
		}
		if json.Unmarshal(reply.Arguments, &args) != nil || json.Unmarshal(reply.Body, &result) != nil || result.Error != "" || len(result.Domains) == 0 {
			t.Fatal("Agent consumed a failed discovery result")
		}
		row := wire[result.RequestID]
		if row == nil || row["method"] != http.MethodGet {
			t.Fatal("discovery result ID did not match an observed GET")
		}
		body, bodyOK := row["body"].(map[string]any)
		data, dataOK := body["data"].(map[string]any)
		var delivered any
		if !bodyOK || !dataOK || json.Unmarshal(result.Domains, &delivered) != nil {
			t.Fatal("discovery response body is missing")
		}
		actualJSON, _ := json.Marshal(data["domains"])
		deliveredJSON, _ := json.Marshal(delivered)
		if string(actualJSON) != string(deliveredJSON) {
			t.Fatal("discovery replaced the observed vendor data")
		}
		if len(args.Domains) == 0 {
			list = row["url"] == "https://api.anysearch.com/v1/domains" && strings.Contains(string(result.Domains), `"code"`)
		} else if len(args.Domains) == 1 && args.Domains[0] == "code" {
			var groups []struct {
				Domain     string `json:"domain"`
				SubDomains []struct {
					Tag    string `json:"sub_domain"`
					Params map[string]struct {
						Required bool `json:"required"`
					} `json:"params"`
				} `json:"sub_domains"`
			}
			if json.Unmarshal(result.Domains, &groups) != nil {
				t.Fatal("invalid code discovery response")
			}
			doc, snippet := false, false
			for _, group := range groups {
				for _, tag := range group.SubDomains {
					library, libraryOK := tag.Params["library"]
					repo, repoOK := tag.Params["repo"]
					doc = doc || (group.Domain == "code" && tag.Tag == "code.doc" && libraryOK && library.Required)
					snippet = snippet || (group.Domain == "code" && tag.Tag == "code.snippet" && repoOK && !repo.Required)
				}
			}
			code = row["url"] == "https://api.anysearch.com/v1/sub-domains?domain=code" && doc && snippet
		}
	}
	if !list || !code {
		t.Fatal("saved Agent did not discover actual domains and current code parameter contracts")
	}
	for _, term := range []string{"code.doc", "code.snippet", "library", "repo"} {
		if !strings.Contains(answer, term) {
			t.Fatal("Agent did not deliver the discovered code contracts to the user")
		}
	}
}

func assertAnySearchBatchExtension(t *testing.T, replies []anySearchExtensionReply, wire map[string]map[string]any, answer string, queries []string) {
	t.Helper()
	if len(replies) != 1 {
		t.Fatal("Agent did not consume exactly one batch tool reply")
	}
	var args struct {
		Queries []struct {
			Query string `json:"query"`
		} `json:"queries"`
	}
	var result struct {
		Error   string `json:"_ERROR"`
		Queries []struct {
			Index     int              `json:"index"`
			Query     string           `json:"query"`
			RequestID string           `json:"request_id"`
			Error     string           `json:"_ERROR"`
			Results   []map[string]any `json:"results"`
		} `json:"queries"`
	}
	if json.Unmarshal(replies[0].Arguments, &args) != nil || json.Unmarshal(replies[0].Body, &result) != nil || result.Error != "" || len(args.Queries) != 2 || len(result.Queries) != 2 {
		t.Fatal("Agent did not issue and consume the actual two-query batch")
	}
	for i, query := range queries {
		item := result.Queries[i]
		row := wire[item.RequestID]
		request, _ := row["request"].(map[string]any)
		if args.Queries[i].Query != query || item.Index != i || item.Query != query || item.Error != "" || len(item.Results) != 1 || row == nil || row["method"] != http.MethodPost || row["url"] != "https://api.anysearch.com/v1/search" || request["query"] != query || request["max_results"] != float64(1) {
			t.Fatal("batch query, result or observed request association changed")
		}
		assertAnySearchObservedResults(t, row, item.Results, 1)
		resultURL, _ := item.Results[0]["url"].(string)
		u, err := url.Parse(resultURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || !strings.Contains(answer, resultURL) {
			t.Fatal("Agent answer omitted a source URL from a successful batch item")
		}
	}
}
