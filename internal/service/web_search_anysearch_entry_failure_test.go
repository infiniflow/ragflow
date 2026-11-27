package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
)

// The controlled model acknowledges failed retrieval. This verifies delivery
// and persistence of provider failures, not a model's ability to avoid invention.
func TestAnySearchChatEntryPointFailures(t *testing.T) {
	const key = "entry-failure-secret"
	const poison = "Rejected AnySearch evidence"
	const poisonURL = "https://example.com/rejected-anysearch"
	const answer = "I could not verify the web sources."
	for _, mode := range []struct {
		name      string
		reasoning int
	}{{"standard", 0}, {"agentic", 5}} {
		for _, failure := range []struct {
			name         string
			status, code int
		}{{"unauthorized", 401, 401}, {"rate_limited", 429, 429}, {"business_error", 200, 23}} {
			t.Run(mode.name+"/"+failure.name, func(t *testing.T) {
				var searches, modelCalls, failedToolResults atomic.Int32
				logs := captureAnySearchLogs(t, zapcore.InfoLevel)
				llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					modelCalls.Add(1)
					var request struct {
						Stream   bool                             `json:"stream"`
						Messages []struct{ Role, Content string } `json:"messages"`
						Tools    []struct {
							Function struct{ Name string } `json:"function"`
						} `json:"tools"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					hasWeb, hasToolResult := false, false
					for _, tool := range request.Tools {
						hasWeb = hasWeb || tool.Function.Name == "web_search"
					}
					for _, message := range request.Messages {
						if strings.Contains(message.Content, poison) || strings.Contains(message.Content, poisonURL) || strings.Contains(message.Content, key) {
							t.Error("rejected source or provider credential reached the model")
						}
						if message.Role == "tool" {
							hasToolResult = true
							if strings.Contains(message.Content, "web_search") && strings.Contains(message.Content, "tool_error") {
								failedToolResults.Add(1)
							}
						}
					}
					message := map[string]any{"role": "assistant", "content": answer}
					finish := "stop"
					if hasWeb && !hasToolResult {
						message = map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "failed-entry-search", "type": "function", "function": map[string]any{"name": "web_search", "arguments": `{"queries":["entry failure query"]}`}}}}
						finish = "tool_calls"
					}
					if request.Stream {
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
				if err := db.AutoMigrate(&entity.ChatSession{}, &entity.ConversationMessage{}, &entity.ConversationReference{}); err != nil {
					t.Fatal(err)
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
				status := string(entity.StatusValid)
				if err := db.Create(&entity.Knowledgebase{ID: "failure-kb", TenantID: "user-1", CreatedBy: "user-1", Name: "failure KB", Permission: "me", Status: &status}).Error; err != nil {
					t.Fatal(err)
				}
				chat := dialForTest("model-a")
				chat.ID, chat.TenantID, chat.Status = "failure-chat", "user-1", &status
				chat.KBIDs = entity.JSONSlice{"failure-kb"}
				chat.LLMSetting = entity.JSONMap{}
				chat.PromptConfig["system"] = "Answer using {knowledge}"
				chat.PromptConfig["parameters"] = []any{map[string]any{"key": "knowledge", "optional": false}}
				chat.PromptConfig["web_search_provider"] = "anysearch"
				chat.PromptConfig["anysearch_api_key"] = key
				chat.PromptConfig["reasoning"] = mode.reasoning
				if err := db.Create(chat).Error; err != nil {
					t.Fatal(err)
				}
				requestID := "failed-entry-" + mode.name + "-" + failure.name
				original := anySearchWebSearchHTTPClient
				t.Cleanup(func() { anySearchWebSearchHTTPClient = original })
				anySearchWebSearchHTTPClient = &http.Client{Transport: anySearchTestTransport(func(r *http.Request) (*http.Response, error) {
					searches.Add(1)
					if r.Method != http.MethodPost || r.URL.String() != anySearchWebSearchEndpoint || r.Header.Get("Authorization") != "Bearer "+key {
						t.Error("saved provider selection, request method or Bearer authentication changed")
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["query"] != "entry failure query" {
						t.Error("saved Chat query did not reach AnySearch")
					}
					response, _ := json.Marshal(map[string]any{"code": failure.code, "message": "search refused " + key, "request_id": requestID,
						"data": map[string]any{"results": []any{map[string]any{"title": "Rejected source", "url": poisonURL, "content": poison}}}})
					return &http.Response{StatusCode: failure.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(response)))}, nil
				})}
				ctx, cancel := context.WithTimeout(serviceTestContext(t, "user-1", "user-1"), 20*time.Second)
				defer cancel()
				sessions := NewChatSessionService()
				result, err := sessions.ChatCompletions(ctx, chat.ID, "", nil, "entry failure query", nil, "", nil, map[string]any{"internet": true}, false, false, nil)
				if err != nil {
					t.Fatal(err)
				}
				if searches.Load() != 1 || modelCalls.Load() == 0 {
					t.Fatalf("provider calls=%d model calls=%d; want one provider attempt and model execution", searches.Load(), modelCalls.Load())
				}
				if mode.reasoning == 5 && failedToolResults.Load() == 0 {
					t.Error("Agentic model did not receive an explicit tool failure")
				}
				response, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(response), answer) {
					t.Fatal("final answer did not acknowledge failed retrieval")
				}
				sessionID, _ := result["session_id"].(string)
				if sessionID == "" {
					t.Fatal("completion did not create a saved session")
				}
				saved, err := dao.NewChatSessionDAO().GetByID(ctx, db, sessionID, dao.ConversationHistoryOptions{IncludeReference: true})
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(saved.Message), answer) {
					t.Fatal("failure-aware assistant answer was not persisted")
				}
				for name, payload := range map[string][]byte{"response": response, "saved history": saved.Message} {
					for _, rejected := range []string{poison, poisonURL, "Rejected source", key, "anysearch-"} {
						if strings.Contains(string(payload), rejected) {
							t.Errorf("%s contains rejected evidence, citation or credential %q", name, rejected)
						}
					}
					var decoded any
					if err := json.Unmarshal(payload, &decoded); err != nil {
						t.Fatal(err)
					}
					var inspect func(any)
					inspect = func(value any) {
						switch item := value.(type) {
						case map[string]any:
							for field, child := range item {
								if (field == "answer" || field == "content") && strings.Contains(fmt.Sprint(child), "[ID:") {
									t.Errorf("%s contains a citation despite all web retrieval failing", name)
								}
								if field == "chunks" || field == "doc_aggs" {
									if records, ok := child.([]any); ok && len(records) != 0 {
										t.Errorf("%s retained sources despite all web retrieval failing", name)
									}
								}
								inspect(child)
							}
						case []any:
							for _, child := range item {
								inspect(child)
							}
						}
					}
					inspect(decoded)
				}
				entries := logs.FilterMessage("AnySearch response").All()
				if len(entries) != 1 || entries[0].ContextMap()["request_id"] != requestID {
					t.Fatal("failed request did not retain exactly one observable diagnostic ID")
				}
				for _, entry := range logs.All() {
					encoded, err := json.Marshal(map[string]any{"message": entry.Message, "fields": entry.ContextMap()})
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(encoded), key) || strings.Contains(string(encoded), poison) || strings.Contains(string(encoded), poisonURL) {
						t.Error("provider credential or rejected raw payload reached ordinary logs")
					}
				}
			})
		}
	}
}
