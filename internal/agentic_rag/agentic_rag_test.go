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

package agentic_rag

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/common"
	"ragflow/internal/entity/models"
)

// TestRun_NilModel: Run must reject a nil model up front.
func TestRun_NilModel(t *testing.T) {
	_, err := Run(context.Background(), Input{Model: nil})
	if err == nil {
		t.Fatal("expected error for nil model")
	}
}

type retrievalToolDriver struct {
	models.ModelDriver
	respond func([]models.Message, *models.ChatConfig) *models.ChatResponse
}

func (d *retrievalToolDriver) Name() string { return "retrieval-tools-test" }

func (d *retrievalToolDriver) ChatWithMessages(_ context.Context, _ string, messages []models.Message, _ *models.APIConfig, config *models.ChatConfig, _ *common.ModelUsage) (*models.ChatResponse, error) {
	return d.respond(messages, config), nil
}

func (d *retrievalToolDriver) ChatStreamlyWithSender(_ context.Context, _ string, messages []models.Message, _ *models.APIConfig, config *models.ChatConfig, _ *common.ModelUsage, sender func(*string, *string) error) error {
	response := d.respond(messages, config)
	if len(response.ToolCalls) > 0 {
		config.ToolCallsResult = &response.ToolCalls
		return nil
	}
	return sender(response.Answer, response.ReasonContent)
}

func TestRunDispatchesModelSelectedRetrievalTools(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, datasets := range []bool{false, true} {
			for _, enableWeb := range []bool{false, true} {
				t.Run(fmt.Sprintf("stream=%t/datasets=%t/web=%t", stream, datasets, enableWeb), func(t *testing.T) {
					setupTestConfig(t)
					corpus := newCaptureService()
					runtime.SetRetrievalService(corpus)
					t.Cleanup(func() { runtime.SetRetrievalService(nil) })
					registry := NewEvidenceRegistry()
					var webCalls atomic.Int32
					webResult := WebResult{Title: "Public source", URL: "https://example.com/source", Content: "Public facts."}
					ctx := WithEvidenceRegistry(t.Context(), registry)
					if enableWeb {
						ctx = WithWebSearch(ctx, func(ctx context.Context, query string) ([]WebResult, error) {
							webCalls.Add(1)
							if query != "public keywords" {
								t.Errorf("web query = %q, want model-selected keywords", query)
							}
							if err := ctx.Err(); err != nil {
								return nil, err
							}
							return []WebResult{webResult}, nil
						})
					}
					var required []string
					var datasetIDs []string
					var tools []tool.BaseTool
					if datasets {
						datasetIDs = []string{"kb1"}
						required = append(required, "search_chunks")
					} else {
						tools = []tool.BaseTool{NewThinkTool()}
					}
					if enableWeb {
						required = append(required, webSearchToolName)
					}
					modelCalls := 0
					driver := &retrievalToolDriver{respond: func(messages []models.Message, config *models.ChatConfig) *models.ChatResponse {
						modelCalls++
						if modelCalls == 1 && (webCalls.Load() != 0 || len(corpus.reqs) != 0) {
							t.Error("retrieval ran before the model supplied a query")
						}
						advertised, _ := config.Tools.([]map[string]interface{})
						foundWeb := false
						for _, info := range advertised {
							function, _ := info["function"].(map[string]interface{})
							foundWeb = foundWeb || function["name"] == webSearchToolName
						}
						if foundWeb != enableWeb {
							t.Errorf("web tool advertised=%t, enabled=%t", foundWeb, enableWeb)
						}
						start := len(messages) - 1
						for start > 0 && messages[start].Role != "user" {
							start--
						}
						seen := map[string]bool{}
						for _, message := range messages[start:] {
							if message.Role != "tool" {
								continue
							}
							seen[message.ToolCallID] = true
							content, _ := message.Content.(string)
							var result struct {
								XMLName xml.Name `xml:"search_results"`
								Chunks  []struct {
									ID      string `xml:"chunk_id,attr"`
									Ref     *int   `xml:"ref,attr"`
									Source  string `xml:"source,attr"`
									URL     string `xml:"url,attr"`
									Snippet string `xml:"match_snippet"`
								} `xml:"chunk"`
							}
							if err := xml.Unmarshal([]byte(content), &result); err != nil || len(result.Chunks) != 1 {
								t.Errorf("tool %s did not return shared chunk XML: %s (err=%v)", message.ToolCallID, content, err)
								continue
							}
							chunk := result.Chunks[0]
							if chunk.Ref == nil {
								t.Error("retrieved chunk lacks a citation handle")
							} else if id, ok := registry.Resolve(*chunk.Ref); !ok || id != chunk.ID {
								t.Errorf("citation does not resolve to the returned chunk: %+v", chunk)
							}
							if message.ToolCallID == webSearchToolName {
								if chunk.Source != "web" || chunk.URL != webResult.URL || chunk.Snippet != webResult.Content {
									t.Errorf("web chunk lost source metadata: %+v", chunk)
								}
								if saved, ok := registry.WebResult(chunk.ID); !ok || saved != webResult {
									t.Errorf("web citation lost retained evidence: %+v, ok=%t", saved, ok)
								}
							} else if chunk.ID != "c1" || !strings.Contains(chunk.Snippet, "Dal Pescatore") {
								t.Errorf("unexpected corpus chunk: %+v", chunk)
							}
						}
						for i, name := range required {
							if seen[name] != (i < modelCalls-1) {
								t.Errorf("model turn %d has unexpected results: %v", modelCalls, seen)
							}
						}
						if modelCalls <= len(required) {
							choice, _ := config.ToolChoiceValue.(map[string]interface{})
							function, _ := choice["function"].(map[string]interface{})
							name := required[modelCalls-1]
							if choice["type"] != "function" || function["name"] != name {
								t.Errorf("model turn %d must require %s: %#v", modelCalls, name, config.ToolChoiceValue)
							}
							arguments := `{"queries":["Dal Pescatore"]}`
							if name == webSearchToolName {
								arguments = `{"queries":["public keywords"]}`
							}
							return &models.ChatResponse{ToolCalls: []map[string]interface{}{{"id": name, "type": "function", "function": map[string]interface{}{"name": name, "arguments": arguments}}}}
						}
						// Answer immediately when permitted: a missing requirement must fail
						// this test rather than being hidden by a helpful model's next search.
						if config.ToolChoiceValue != nil || config.ToolChoice == nil || *config.ToolChoice != "auto" {
							t.Errorf("after all sources return, answering must use auto: %+v", config)
						}
						answer := "Answer based on the available evidence."
						return &models.ChatResponse{Answer: &answer}
					}}
					name := driver.Name()
					counts := map[string]int{}
					previousCalls := []schema.ToolCall{
						{ID: "old-corpus", Type: "function", Function: schema.FunctionCall{Name: "search_chunks", Arguments: `{"queries":["old"]}`}},
						{ID: "old-web", Type: "function", Function: schema.FunctionCall{Name: webSearchToolName, Arguments: `{"queries":["old"]}`}},
					}
					answer, err := Run(ctx, Input{
						Model:      models.NewEinoChatModel(models.NewChatModel(driver, &name, &models.APIConfig{}), nil),
						TemplateID: "smart-reasoning", TenantID: "tenant", DatasetIDs: datasetIDs, Tools: tools,
						Messages: []*schema.Message{
							schema.UserMessage("An earlier question."),
							schema.AssistantMessage("", previousCalls),
							schema.ToolMessage("Earlier corpus evidence.", "old-corpus"),
							schema.ToolMessage("Earlier web evidence.", "old-web"),
							schema.AssistantMessage("The earlier answer.", nil),
							schema.UserMessage("Research a topic using available sources."),
						},
						Stream: stream, MaxIterations: 4, ToolCallCounts: counts, OnDelta: func(string, string) {},
					})
					if err != nil || answer != "Answer based on the available evidence." || modelCalls != len(required)+1 {
						t.Fatalf("Run: answer=%q err=%v model calls=%d", answer, err, modelCalls)
					}
					wantCorpus, wantWeb := 0, 0
					if datasets {
						wantCorpus = 1
					}
					if enableWeb {
						wantWeb = 1
					}
					if counts["search_chunks"] != wantCorpus || counts[webSearchToolName] != wantWeb || len(corpus.reqs) != wantCorpus || int(webCalls.Load()) != wantWeb {
						t.Errorf("unexpected retrieval calls: counts=%v corpus=%d web=%d", counts, len(corpus.reqs), webCalls.Load())
					}
					if datasets && len(corpus.reqs) > 0 {
						request := corpus.reqs[0]
						if request.TenantID != "tenant" || len(request.DatasetIDs) != 1 || request.DatasetIDs[0] != "kb1" {
							t.Errorf("corpus scope changed: %+v", request)
						}
					}
				})
			}
		}
	}
}
