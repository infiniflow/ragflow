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
		for _, selectWeb := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%t/web=%t", stream, selectWeb), func(t *testing.T) {
				setupTestConfig(t)
				corpus := newCaptureService()
				runtime.SetRetrievalService(corpus)
				t.Cleanup(func() { runtime.SetRetrievalService(nil) })
				registry := NewEvidenceRegistry()
				var webCalls atomic.Int32
				webResult := WebResult{Title: "Public source", URL: "https://example.com/source", Content: "Public facts."}
				ctx := WithWebSearch(WithEvidenceRegistry(t.Context(), registry), func(ctx context.Context, query string) ([]WebResult, error) {
					webCalls.Add(1)
					if query != "public keywords" {
						t.Errorf("web query = %q, want model-selected keywords", query)
					}
					if err := ctx.Err(); err != nil {
						return nil, err
					}
					return []WebResult{webResult}, nil
				})
				modelCalls := 0
				driver := &retrievalToolDriver{respond: func(messages []models.Message, config *models.ChatConfig) *models.ChatResponse {
					modelCalls++
					if modelCalls == 1 {
						if webCalls.Load() != 0 || len(corpus.reqs) != 0 {
							t.Error("retrieval ran before the model selected a tool")
						}
						advertised, _ := config.Tools.([]map[string]interface{})
						foundWeb := false
						for _, info := range advertised {
							function, _ := info["function"].(map[string]interface{})
							foundWeb = foundWeb || function["name"] == webSearchToolName
						}
						if !foundWeb {
							t.Error("configured web search was not advertised to the model")
						}
						calls := []map[string]interface{}{{"id": "corpus-call", "type": "function", "function": map[string]interface{}{
							"name": "search_chunks", "arguments": `{"queries":["Dal Pescatore"]}`,
						}}}
						if selectWeb {
							calls = append(calls, map[string]interface{}{"id": "web-call", "type": "function", "function": map[string]interface{}{
								"name": webSearchToolName, "arguments": `{"queries":["public keywords"]}`,
							}})
						}
						return &models.ChatResponse{ToolCalls: calls}
					}
					seen := map[string]bool{}
					for _, message := range messages {
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
							t.Errorf("citation handle does not resolve to the returned chunk: %+v", chunk)
						}
						if message.ToolCallID == "web-call" {
							if chunk.Source != "web" || chunk.URL != webResult.URL || chunk.Snippet != webResult.Content {
								t.Errorf("web chunk lost source metadata: %+v", chunk)
							}
							if saved, ok := registry.WebResult(chunk.ID); !ok || saved != webResult {
								t.Errorf("web citation lost its retained evidence: %+v, ok=%t", saved, ok)
							}
						} else if chunk.ID != "c1" || !strings.Contains(chunk.Snippet, "Dal Pescatore") {
							t.Errorf("unexpected corpus chunk: %+v", chunk)
						}
					}
					if !seen["corpus-call"] || seen["web-call"] != selectWeb || len(seen) != 1+int(webCalls.Load()) {
						t.Errorf("next model turn received unexpected tool results: %v", seen)
					}
					answer := "Answer based on the retrieved evidence."
					return &models.ChatResponse{Answer: &answer}
				}}
				name := driver.Name()
				counts := map[string]int{}
				answer, err := Run(ctx, Input{
					Model:      models.NewEinoChatModel(models.NewChatModel(driver, &name, &models.APIConfig{}), nil),
					TemplateID: "smart-reasoning", TenantID: "tenant", DatasetIDs: []string{"kb1"},
					Messages: []*schema.Message{schema.UserMessage("Research a topic using available sources.")},
					Stream:   stream, MaxIterations: 4, ToolCallCounts: counts,
					OnDelta: func(string, string) {},
				})
				if err != nil || answer != "Answer based on the retrieved evidence." || modelCalls != 2 {
					t.Fatalf("Run: answer=%q err=%v model calls=%d", answer, err, modelCalls)
				}
				wantWeb := 0
				if selectWeb {
					wantWeb = 1
				}
				if counts["search_chunks"] != 1 || counts[webSearchToolName] != wantWeb || int(webCalls.Load()) != wantWeb {
					t.Errorf("unexpected retrieval calls: tool counts=%v web calls=%d", counts, webCalls.Load())
				}
				if len(corpus.reqs) != 1 || corpus.reqs[0].TenantID != "tenant" || len(corpus.reqs[0].DatasetIDs) != 1 || corpus.reqs[0].DatasetIDs[0] != "kb1" {
					t.Errorf("corpus retrieval scope changed: %+v", corpus.reqs)
				}
			})
		}
	}
}
