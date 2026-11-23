//go:build integration

// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/utility"
)

// This exercises the production Agent/registry/ReAct path against real AnySearch.
// The local HTTP model is scripted; this is not live model inference.
func TestAnySearchAgentBatchItemIsolationLive(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch API")
	}
	key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	mode := "anonymous"
	if key != "" {
		mode = "keyed"
	}
	setupAllowAnyHost(t, true)
	if err := models.InitProviderManager("../../../conf/models"); err != nil {
		t.Fatal(err)
	}

	type queryResult struct {
		Index     int              `json:"index"`
		Query     string           `json:"query"`
		RequestID string           `json:"request_id"`
		Error     string           `json:"_ERROR"`
		Results   []map[string]any `json:"results"`
	}
	type envelope struct {
		Queries []queryResult `json:"queries"`
		Error   string        `json:"_ERROR"`
	}
	type observation struct {
		raw, answer, url, title, content string
		requestID, localError            string
	}
	observed := make(chan observation, 1)
	var calls, summaries atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream   bool `json:"stream"`
			Messages []struct {
				Role, Content string
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode model request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		for _, message := range req.Messages {
			if message.Role == "system" && strings.Contains(message.Content, "memory summarizer") {
				summaries.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "Searched RAGFlow; flight number missing."}, "finish_reason": "stop"}}})
				return
			}
		}
		switch calls.Add(1) {
		case 1:
			args := `{"queries":[{"query":"RAGFlow"},{"query":"flight status","tag":"travel.flight_status","params":{"departure":""}}]}`
			call := map[string]any{"id": "batch-search", "type": "function", "function": map[string]any{"name": "anysearch_search", "arguments": args}}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{call}}, "finish_reason": "tool_calls"}}})
		case 2:
			var raw string
			for _, message := range req.Messages {
				if message.Role == "tool" {
					raw = message.Content
				}
			}
			var result envelope
			if err := json.Unmarshal([]byte(raw), &result); err != nil {
				t.Errorf("decode model-visible tool result: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if result.Error != "" || len(result.Queries) != 2 {
				t.Errorf("model received whole-batch failure=%v or wrong record count=%d", result.Error != "", len(result.Queries))
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			good, bad := result.Queries[0], result.Queries[1]
			if good.Index != 0 || good.Query != "RAGFlow" || good.Error != "" || good.RequestID == "" || len(good.Results) != 1 ||
				bad.Index != 1 || bad.Query != "flight status" || !strings.Contains(bad.Error, "flight_number") || bad.RequestID != "" || len(bad.Results) != 0 {
				t.Error("model received incorrect success/local-error records or ordering")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			hit := good.Results[0]
			url, _ := hit["url"].(string)
			title, _ := hit["title"].(string)
			content, _ := hit["content"].(string)
			if strings.TrimSpace(content) == "" {
				content, _ = hit["snippet"].(string)
			}
			if url == "" || title == "" || strings.TrimSpace(content) == "" {
				t.Error("real search did not supply a usable titled reference")
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			answer := "Observed source: " + title + " " + url + "; flight_number is required."
			observed <- observation{raw: raw, answer: answer, url: url, title: title, content: strings.Join(strings.Fields(content), " "), requestID: good.RequestID, localError: bad.Error}
			if req.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": answer}, "finish_reason": "stop"}}})
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": answer}, "finish_reason": "stop"}}})
		default:
			t.Error("unexpected extra ReAct model call")
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	db := setupComponentTestDB(t)
	const modelID = "0123456789abcdef0123456789abcdef"
	for _, row := range []any{
		&entity.TenantModelProvider{ID: "batch-provider", UserID: "tenant-1", TenantID: "tenant-1", ProviderName: "OpenAI"},
		&entity.TenantModelInstance{ID: "batch-instance", ProviderID: "batch-provider", InstanceName: "default", APIKey: "synthetic", Status: "active"},
		&entity.TenantModel{ID: modelID, ProviderID: "batch-provider", InstanceID: "batch-instance", ModelName: "gpt-4o", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: `{"max_tokens":32768}`},
	} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	state := runtime.NewCanvasState("live-batch-isolation", "tenant-1")
	ctx = runtime.WithState(ctx, state)
	responseTokens := 256
	agent := NewAgentComponent(AgentParam{
		ModelID: modelID, Driver: "OpenAI-API-Compatible", BaseURL: server.URL, APIKey: "synthetic",
		MaxTokens: &responseTokens, MaxRounds: 1, UserPrompt: "Search RAGFlow and report the available source and missing flight parameter.",
		Tools: []string{"AnySearchSearch"}, ToolParams: map[string]map[string]any{"anysearch_search": {"api_key": key, "top_n": 1}},
	})
	out, err := agent.Invoke(ctx, db, nil)
	if err != nil {
		t.Fatalf("Agent.Invoke: %v", err)
	}
	if calls.Load() != 2 || summaries.Load() != 1 {
		t.Fatalf("model calls=%d summaries=%d, want two ReAct calls and one memory summary", calls.Load(), summaries.Load())
	}
	var seen observation
	select {
	case seen = <-observed:
	default:
		t.Fatal("model did not receive the successful sibling and local item error")
	}
	if out["content"] != seen.answer {
		t.Fatal("final answer differs from the answer derived from the model-visible source")
	}
	invocations, ok := out["tool_invocations"].([]map[string]any)
	if !ok || len(invocations) != 1 || invocations[0]["tool_name"] != "anysearch_search" || invocations[0]["status"] != "success" {
		t.Fatal("expected one successful anysearch_search invocation receipt")
	}
	recorded, ok := invocations[0]["result"].(string)
	if !ok {
		t.Fatal("invocation receipt has no textual result")
	}
	// The recorder normalizes/redacts JSON and can retain only a bounded prefix.
	normalized := redactToolLogText(seen.raw)
	if utf8.RuneCountInString(normalized) > maxToolLogResultRunes {
		if recorded != utility.TruncateRunes(normalized, maxToolLogResultRunes) {
			t.Fatal("invocation receipt differs from the bounded original tool result")
		}
		t.Log("invocation receipt reached the log limit; full source association checked in the model message and run state")
	} else {
		var receipt envelope
		if err := json.Unmarshal([]byte(recorded), &receipt); err != nil {
			t.Fatalf("decode invocation receipt: %v", err)
		}
		if receipt.Error != "" || len(receipt.Queries) != 2 {
			t.Fatal("invocation receipt lost the ordered partial-success envelope")
		}
		good, bad := receipt.Queries[0], receipt.Queries[1]
		if good.Index != 0 || good.Query != "RAGFlow" || good.Error != "" || good.RequestID != seen.requestID || len(good.Results) != 1 ||
			bad.Index != 1 || bad.Query != "flight status" || bad.Error != seen.localError || bad.RequestID != "" || len(bad.Results) != 0 {
			t.Fatal("invocation receipt lost the successful request ID or local item error")
		}
		if good.Results[0]["url"] != seen.url || good.Results[0]["title"] != seen.title {
			t.Fatal("invocation receipt lost the successful source URL or title")
		}
	}
	chunks := state.GetRetrievalChunks()
	if len(chunks) != 1 || chunks[0]["url"] != seen.url || chunks[0]["document_name"] != seen.title || chunks[0]["content"] != seen.content {
		t.Fatal("successful sibling reference was not preserved with matching URL, title and content")
	}
	t.Logf("real AnySearch mode=%s request_id=%s; success and local error reached the scripted HTTP model; references=%d; actual LLM inference not exercised", mode, seen.requestID, len(chunks))
}
