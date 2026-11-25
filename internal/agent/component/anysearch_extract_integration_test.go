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
	"ragflow/internal/tokenizer"
)

// The vendor extraction is live; the local HTTP model scripts tool selection
// and the final answer, without live model inference.
func TestAnySearchExtractAgentLive(t *testing.T) {
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
	for _, tc := range []struct {
		name, url string
		window    int
		omitted   bool
	}{
		{"html", "https://example.com/", 32768, false},
		{"long text with constrained context", "https://www.rfc-editor.org/rfc/rfc9110.txt", 2048, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type page struct {
				URL, Title, Content string
				RequestID           string `json:"request_id"`
				Error               string `json:"_ERROR"`
			}
			observed := make(chan page, 1)
			var calls, summaries atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Stream   bool            `json:"stream"`
					Messages json.RawMessage `json:"messages"`
					Tools    json.RawMessage `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Errorf("decode model request: %v", err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				var messages []struct{ Role, Content string }
				if err := json.Unmarshal(req.Messages, &messages); err != nil {
					t.Error("decode model messages")
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				reply := func(content string) {
					if req.Stream {
						w.Header().Set("Content-Type", "text/event-stream")
						chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": content}, "finish_reason": "stop"}}})
						_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}}})
				}
				for _, message := range messages {
					if message.Role == "system" && strings.Contains(message.Content, "memory summarizer") {
						summaries.Add(1)
						reply("Extraction attempted.")
						return
					}
				}
				switch calls.Add(1) {
				case 1:
					args, _ := json.Marshal(map[string]string{"url": tc.url})
					call := map[string]any{"id": "extract-page", "type": "function", "function": map[string]any{"name": "anysearch_extract", "arguments": string(args)}}
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{call}}, "finish_reason": "tool_calls"}}})
				case 2:
					var raw string
					for _, message := range messages {
						if message.Role == "tool" {
							raw = message.Content
						}
					}
					var result page
					if err := json.Unmarshal([]byte(raw), &result); err != nil {
						t.Error("model did not receive a JSON extraction result")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					if tc.omitted {
						if !strings.Contains(result.Error, "insufficient remaining context") || result.Content != "" {
							t.Error("long extraction did not reach the model as an explicit context omission")
						}
					} else if result.Error != "" || result.Title != "Example Domain" || result.URL != tc.url || !strings.Contains(result.Content, "documentation examples") || result.RequestID == "" {
						t.Error("model received an incomplete or failed HTML extraction")
					}
					if tokenizer.NumTokensFromString(string(req.Messages))+tokenizer.NumTokensFromString(string(req.Tools))+256 > tc.window {
						t.Error("model-visible messages still exceed the constrained window")
					}
					observed <- result
					if tc.omitted {
						reply("Page text was omitted because model context is insufficient.")
					} else {
						reply("Read " + result.Title + " at " + result.URL)
					}
				default:
					t.Error("unexpected extra ReAct model call")
					w.WriteHeader(http.StatusBadRequest)
				}
			}))
			defer server.Close()
			db := setupComponentTestDB(t)
			const modelID = "0123456789abcdef0123456789abcdef"
			for _, row := range []any{
				&entity.TenantModelProvider{ID: "extract-provider", UserID: "tenant-1", TenantID: "tenant-1", ProviderName: "OpenAI"},
				&entity.TenantModelInstance{ID: "extract-instance", ProviderID: "extract-provider", InstanceName: "default", APIKey: "synthetic", Status: "active"},
				&entity.TenantModel{ID: modelID, ProviderID: "extract-provider", InstanceID: "extract-instance", ModelName: "gpt-4o", ModelType: int(entity.ModelTypeChat), Status: "active", Extra: fmt.Sprintf(`{"max_tokens":%d}`, tc.window)},
			} {
				if err := db.Create(row).Error; err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			state := runtime.NewCanvasState("live-extract", "tenant-1")
			ctx = runtime.WithState(ctx, state)
			responseTokens := 256
			agent := NewAgentComponent(AgentParam{
				ModelID: modelID, Driver: "OpenAI-API-Compatible", BaseURL: server.URL, APIKey: "synthetic",
				MaxTokens: &responseTokens, MaxRounds: 1, UserPrompt: "Read the supplied web page. Report if its text is unavailable.",
				Tools: []string{"AnySearchExtract"}, ToolParams: map[string]map[string]any{"anysearch_extract": {"api_key": key}},
			})
			out, err := agent.Invoke(ctx, db, nil)
			if err != nil {
				t.Fatalf("Agent.Invoke: %v", err)
			}
			if calls.Load() != 2 || summaries.Load() != 1 {
				t.Fatalf("model calls=%d summaries=%d, want 2 and 1", calls.Load(), summaries.Load())
			}
			var seen page
			select {
			case seen = <-observed:
			default:
				t.Fatal("model did not receive the extraction outcome")
			}
			chunks := state.GetRetrievalChunks()
			if len(chunks) != 1 || chunks[0]["url"] != tc.url {
				t.Fatal("extraction reference URL or ID was lost")
			}
			id, _ := chunks[0]["id"].(string)
			if id == "" {
				t.Fatal("extraction reference has no citation ID")
			}
			content, _ := chunks[0]["content"].(string)
			wantAnswer := "Read " + seen.Title + " at " + seen.URL
			if tc.omitted {
				wantAnswer = "Page text was omitted because model context is insufficient."
				if utf8.RuneCountInString(content) < 30000 || !strings.HasSuffix(content, "...(content truncated)") {
					t.Fatal("long vendor text was lost from reference state")
				}
			} else if chunks[0]["document_name"] != seen.Title || content != strings.Join(strings.Fields(seen.Content), " ") {
				t.Fatal("reference title or text differs from the model-visible page")
			}
			sources := chunksFromState(ctx)
			if len(sources) != 1 || sources[0].ID != id || sources[0].Content != content {
				t.Fatal("citation source differs from the preserved reference")
			}
			if out["content"] != wantAnswer {
				t.Fatal("final answer differs from the scripted response to the model-visible outcome")
			}
			invocations, ok := out["tool_invocations"].([]map[string]any)
			if !ok || len(invocations) != 1 || invocations[0]["tool_name"] != "anysearch_extract" || invocations[0]["status"] != "success" {
				t.Fatal("missing successful extraction invocation receipt")
			}
			recorded, ok := invocations[0]["result"].(string)
			if !ok || utf8.RuneCountInString(recorded) > maxToolLogResultRunes || strings.Contains(recorded, "insufficient remaining context") {
				t.Fatal("receipt lost the original extraction or exceeded its log bound")
			}
			if tc.omitted && utf8.RuneCountInString(recorded) != maxToolLogResultRunes {
				t.Fatal("expected a bounded receipt for the long extraction")
			}
			if !tc.omitted {
				var receipt page
				if err := json.Unmarshal([]byte(recorded), &receipt); err != nil || receipt != seen {
					t.Fatal("receipt differs from the model-visible extraction")
				}
			}
			t.Logf("real AnySearch mode=%s context=%d omitted=%v references=%d reference_chars=%d; scripted HTTP model, no live model inference", mode, tc.window, tc.omitted, len(chunks), utf8.RuneCountInString(content))
		})
	}
}
