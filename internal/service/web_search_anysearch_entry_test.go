package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"go.uber.org/zap/zapcore"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

// These fixtures drive the real model adapter and Chat dispatcher. The KB is
// authorized but unindexed; this checks web retrieval, not corpus retrieval or
// model inference.
func TestAnySearchChatEntryPointModes(t *testing.T) {
	runAnySearchChatEntryPointModes(t, false)
}

func runAnySearchChatEntryPointModes(t *testing.T, live bool) {
	queryText, key := "entry query", "entry-test-key"
	if live {
		queryText = "RAGFlow official documentation"
		key = strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	}
	for _, tc := range []struct {
		name                         string
		kb, internet, agentic, saved bool
		reasoning                    int
	}{
		{name: "naive_kb", kb: true, internet: true},
		{name: "naive_web_only", internet: true},
		{name: "removed_level_standard", kb: true, internet: true, reasoning: 1},
		{name: "agentic_without_kb_falls_through", internet: true, reasoning: 5},
		{name: "internet_off", kb: true},
		{name: "request_overrides_saved", kb: true, internet: true, saved: true},
		{name: "saved_agentic", kb: true, internet: true, saved: true, agentic: true},
		{name: "agentic_kb", kb: true, internet: true, reasoning: 5, agentic: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if live && tc.name != "naive_kb" && tc.name != "agentic_kb" {
				t.Skip("live gate covers the two owning retrieval modes")
			}
			var searches, toolResults, modelCalls atomic.Int32
			var liveSource atomic.Value
			var liveRequestID atomic.Value
			logs := captureAnySearchLogs(t, zapcore.InfoLevel)
			llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				modelCalls.Add(1)
				var req struct {
					Stream   bool                             `json:"stream"`
					Messages []struct{ Role, Content string } `json:"messages"`
					Tools    []struct {
						Function struct {
							Name string `json:"name"`
						} `json:"function"`
					} `json:"tools"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				hasWeb, hasResult := false, false
				for _, tool := range req.Tools {
					if tool.Function.Name == "web_search" {
						hasWeb = true
					}
				}
				var sourceMarker string
				if source, ok := liveSource.Load().(anySearchWebSearchResult); ok {
					text := source.Content
					if strings.TrimSpace(text) == "" {
						text = source.Snippet
					}
					marker := []rune(strings.Join(strings.Fields(text), " "))
					if len(marker) > 80 {
						marker = marker[:80]
					}
					sourceMarker = string(marker)
				}
				var modelText []string
				for _, msg := range req.Messages {
					modelText = append(modelText, msg.Content)
					if msg.Role == "tool" && ((live && sourceMarker != "" && strings.Contains(strings.Join(strings.Fields(msg.Content), " "), sourceMarker)) || (!live && strings.Contains(msg.Content, "Entry evidence"))) {
						hasResult = true
						toolResults.Add(1)
					}
				}
				if tc.agentic && !hasWeb {
					t.Error("agentic model request did not advertise web_search")
				}
				if live && (!tc.agentic || hasResult) && (sourceMarker == "" || !strings.Contains(strings.Join(strings.Fields(strings.Join(modelText, " ")), " "), sourceMarker)) {
					t.Error("model request did not receive the actual upstream text marker")
				}
				message := map[string]any{"role": "assistant", "content": "Entry evidence [ID:0]"}
				if source, ok := liveSource.Load().(anySearchWebSearchResult); ok {
					message["content"] = "Observed source: " + source.Title + " " + source.URL + " [ID:0]"
				}
				finish := "stop"
				if tc.agentic && !hasResult {
					arguments, _ := json.Marshal(map[string]any{"queries": []string{queryText}})
					message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "entry-search", "type": "function", "function": map[string]any{"name": "web_search", "arguments": string(arguments)}}}}
					finish = "tool_calls"
				}
				if req.Stream {
					w.Header().Set("Content-Type", "text/event-stream")
					if calls, ok := message["tool_calls"].([]any); ok {
						calls[0].(map[string]any)["index"] = 0
					}
					body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": message, "finish_reason": finish}}})
					fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
				} else {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}})
				}
			}))
			defer llm.Close()
			db := setupChatRESTUpdateServiceTestDB(t)
			if live {
				// A failed INSERT must not print the saved vendor credential.
				db = db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})
			}
			if err := models.InitProviderManager("../../conf/models"); err != nil {
				t.Fatal(err)
			}
			t.Setenv(common.EnvAllowAnyHost, "true")
			t.Setenv("AGENTIC_RAG_CONFIG", filepath.Join("..", "..", "conf", "agentic_rag.yaml"))
			extra, _ := json.Marshal(map[string]any{"base_url": llm.URL})
			if err := db.Model(&entity.TenantModelInstance{}).Where("id = ?", "instance-a").Update("extra", string(extra)).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&entity.TenantModel{}).Where("id = ?", "model-a").Update("extra", `{"is_tools":true}`).Error; err != nil {
				t.Fatal(err)
			}
			chat := dialForTest("model-a")
			chat.TenantID = "user-1"
			chat.LLMSetting = entity.JSONMap{}
			chat.PromptConfig["system"] = "Answer using {knowledge}"
			chat.PromptConfig["parameters"] = []any{map[string]any{"key": "knowledge", "optional": false}}
			chat.PromptConfig["web_search_provider"] = "anysearch"
			chat.PromptConfig["anysearch_api_key"] = key
			chat.PromptConfig["tavily_api_key"] = "unused-test-key"
			if tc.saved {
				chat.PromptConfig["reasoning"] = 5
			}
			if tc.kb {
				status := string(entity.StatusValid)
				if err := db.Create(&entity.Knowledgebase{ID: "entry-kb", TenantID: "user-1", CreatedBy: "user-1", Name: "entry KB", Permission: "me", Status: &status}).Error; err != nil {
					t.Fatal(err)
				}
				chat.KBIDs = entity.JSONSlice{"entry-kb"}
			}
			if err := db.Create(chat).Error; err != nil {
				t.Fatal(err)
			}
			var saved entity.Chat
			if err := db.Where("id = ?", chat.ID).Take(&saved).Error; err != nil {
				t.Fatal(err)
			}
			original := anySearchWebSearchHTTPClient
			t.Cleanup(func() { anySearchWebSearchHTTPClient = original })
			anySearchWebSearchHTTPClient = &http.Client{Transport: anySearchTestTransport(func(r *http.Request) (*http.Response, error) {
				searches.Add(1)
				wantAuth := ""
				if key != "" {
					wantAuth = "Bearer " + key
				}
				if r.URL.String() != "https://api.anysearch.com/v1/search" || r.Method != http.MethodPost || r.Header.Get("Authorization") != wantAuth {
					t.Error("unexpected search destination or authorization")
				}
				if live {
					response, err := http.DefaultTransport.RoundTrip(r)
					if err != nil {
						return nil, err
					}
					body, readErr := io.ReadAll(io.LimitReader(response.Body, webSearchMaxResponseBytes+1))
					response.Body.Close()
					if readErr != nil {
						return nil, readErr
					}
					response.Body = io.NopCloser(bytes.NewReader(body))
					if len(body) <= webSearchMaxResponseBytes {
						var envelope struct {
							RequestID string `json:"request_id"`
						}
						if json.Unmarshal(body, &envelope) == nil {
							liveRequestID.Store(envelope.RequestID)
						}
						if hits, err := decodeAnySearchWebSearchResults(body); err == nil {
							for _, hit := range hits {
								if hit.URL != "" && utf8.RuneCountInString(strings.TrimSpace(hit.Content+hit.Snippet)) >= 10 {
									liveSource.Store(hit)
									break
								}
							}
						}
					}
					return response, nil
				}
				var query map[string]any
				if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
					t.Error(err)
				}
				if query["query"] != "entry query" {
					t.Errorf("search query = %v", query["query"])
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"results":[{"title":"Entry title","url":"https://example.com/entry","content":"Entry evidence"}]}}`))}, nil
			})}
			ctx, cancel := context.WithTimeout(serviceTestContext(t, "user-1", "user-1"), 20*time.Second)
			defer cancel()
			options := map[string]any{"internet": tc.internet}
			if !tc.agentic || !tc.saved {
				options["reasoning"] = tc.reasoning
			}
			out, err := NewChatPipelineService().AsyncChat(ctx, &saved, []map[string]any{{"role": "user", "content": queryText}}, false, options)
			if err != nil {
				t.Fatal(err)
			}
			var final AsyncChatResult
			for result := range out {
				if result.Final {
					final = result
				}
			}
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			wantAnswer := "Entry evidence"
			if live {
				source, ok := liveSource.Load().(anySearchWebSearchResult)
				if !ok || source.URL == "" {
					t.Fatal("live search delivered no usable source")
				}
				wantAnswer = "Observed source: " + source.Title + " " + source.URL
			}
			if strings.Contains(final.Answer, "**ERROR**") || !strings.Contains(final.Answer, wantAnswer) {
				t.Fatalf("final answer = %q", final.Answer)
			}
			wantSearches := int32(0)
			if tc.internet {
				wantSearches = 1
			}
			if searches.Load() != wantSearches {
				t.Fatalf("search calls = %d, want %d", searches.Load(), wantSearches)
			}
			if tc.agentic && toolResults.Load() == 0 {
				t.Error("model never received real web_search result")
			}
			if modelCalls.Load() == 0 {
				t.Error("model adapter was not called")
			}
			if tc.internet {
				ref, err := json.Marshal(final.Reference)
				if err != nil {
					t.Fatal(err)
				}
				wantFields := []string{"Entry title", "https://example.com/entry", "Entry evidence"}
				if live {
					source := liveSource.Load().(anySearchWebSearchResult)
					text := source.Content
					if strings.TrimSpace(text) == "" {
						text = source.Snippet
					}
					wantFields = []string{source.Title, source.URL, strings.TrimSpace(text)}
				}
				for _, want := range wantFields {
					encoded, _ := json.Marshal(want)
					if !strings.Contains(string(ref), string(encoded)) {
						t.Errorf("reference missing %q: %s", want, ref)
					}
				}
			}
			if live {
				events := logs.FilterMessage("AnySearch response").All()
				id, _ := liveRequestID.Load().(string)
				if len(events) != 1 || id == "" || events[0].ContextMap()["request_id"] != id || events[0].ContextMap()["http_status"] != int64(200) {
					t.Fatal("ordinary Chat diagnostics did not correlate with the real server response")
				}
				if key != "" && strings.Contains(fmt.Sprint(logs.All()), key) {
					t.Fatal("ordinary runtime logs exposed the configured key")
				}
				t.Logf("real AnySearch through saved Chat AsyncChat: agentic=%v requests=%d model_calls=%d; local HTTP model is scripted, KB is authorized but unindexed", tc.agentic, searches.Load(), modelCalls.Load())
			}
		})
	}
}
