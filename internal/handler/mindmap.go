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

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
)

type mindMapRunConfig struct {
	Question      string
	KbIDs         common.StringSlice
	SearchID      string
	SearchConfig  map[string]interface{}
	AuthUserID    string
	ModelTenantID string
	ChunkSvc      service.Retriever
	LLM           *service.ModelProviderService
	TenantSvc     *service.TenantService
}

func runMindMap(config mindMapRunConfig) (mindMapNode, error) {
	if config.ChunkSvc == nil {
		return mindMapNode{}, fmt.Errorf("chunk service not configured")
	}
	if config.LLM == nil {
		return mindMapNode{}, fmt.Errorf("LLM not configured")
	}
	modelTenantID := config.ModelTenantID
	if modelTenantID == "" {
		modelTenantID = config.AuthUserID
	}
	retrievalReq := mindMapRetrievalRequest(config.Question, config.KbIDs, config.SearchID, config.SearchConfig)
	ranks, err := config.ChunkSvc.RetrievalTest(retrievalReq, config.AuthUserID)
	if err != nil {
		return mindMapNode{}, err
	}
	sections := mindMapSections(ranks)
	if len(sections) == 0 {
		return mindMapNode{ID: "root", Children: []mindMapNode{}}, nil
	}
	modelID, _ := config.SearchConfig["chat_id"].(string)
	messages := []modelModule.Message{{Role: "system", Content: mindMapPrompt(strings.Join(sections, "\n"))}, {Role: "user", Content: "Output:"}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// search_config chat_id can be a stale tenant_model ID that no longer
	// exists. ResolveModelConfig tries ID lookup first, then falls back to
	// composite-name parsing which fails for bare IDs. If the configured
	// model can't be resolved, fall back to the tenant's default chat model
	// (mirrors Python's gen_mindmap get_tenant_default_model_by_type).
	ch, streamErr := config.LLM.ChatStream(ctx, modelTenantID, modelID, messages, &modelModule.ChatConfig{})
	if streamErr != nil && config.TenantSvc != nil {
		if defaultModel, err := config.TenantSvc.GetDefaultModelName(modelTenantID, entity.ModelTypeChat); err == nil && defaultModel != "" && defaultModel != modelID {
			ch, streamErr = config.LLM.ChatStream(ctx, modelTenantID, defaultModel, messages, &modelModule.ChatConfig{})
		}
	}
	if streamErr != nil {
		return mindMapNode{}, streamErr
	}
	var sb strings.Builder
	for delta := range ch {
		sb.WriteString(delta)
	}
	fullText := sb.String()
	if fullText == "" {
		return mindMapNode{ID: "root", Children: []mindMapNode{}}, nil
	}
	return parseMindMapMarkdown(fullText), nil
}

func searchConfigFromDetail(detail map[string]interface{}) map[string]interface{} {
	if sc, ok := detail["search_config"].(map[string]interface{}); ok && sc != nil {
		return sc
	}
	if sc, ok := detail["search_config"].(entity.JSONMap); ok && sc != nil {
		return map[string]interface{}(sc)
	}
	return map[string]interface{}{}
}

func mindMapRetrievalRequest(question string, kbIDs common.StringSlice, searchID string, searchConfig map[string]interface{}) *service.RetrievalTestRequest {
	page := 1
	size := 12
	topK := intFromConfig(searchConfig, "top_k", 1024)
	similarityThreshold := floatFromConfig(searchConfig, "similarity_threshold", 0.2)
	vectorSimilarityWeight := floatFromConfig(searchConfig, "vector_similarity_weight", 0.3)
	req := &service.RetrievalTestRequest{
		Datasets:               kbIDs,
		Question:               question,
		Page:                   &page,
		Size:                   &size,
		TopK:                   &topK,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		DocIDs:                 stringSliceFromConfig(searchConfig, "doc_ids"),
		Filter:                 mapFromConfig(searchConfig, "meta_data_filter"),
	}
	if searchID != "" {
		req.SearchID = &searchID
	}
	if rerankID, _ := searchConfig["rerank_id"].(string); rerankID != "" {
		req.RerankID = &rerankID
	}
	return req
}

func mindMapSections(ranks *service.RetrievalTestResponse) []string {
	if ranks == nil {
		return nil
	}
	sections := make([]string, 0, len(ranks.Chunks))
	for _, chunk := range ranks.Chunks {
		if content, ok := chunk["content_with_weight"].(string); ok && strings.TrimSpace(content) != "" {
			sections = append(sections, content)
		}
	}
	return sections
}

func mergeMindMapKbIDs(saved []string, requested common.StringSlice) common.StringSlice {
	seen := map[string]bool{}
	merged := make(common.StringSlice, 0, len(saved)+len(requested))
	for _, id := range saved {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	for _, id := range requested {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	return merged
}

func intFromConfig(config map[string]interface{}, key string, fallback int) int {
	switch v := config[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return fallback
}

func floatFromConfig(config map[string]interface{}, key string, fallback float64) float64 {
	switch v := config[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return n
		}
	}
	return fallback
}

func stringSliceFromConfig(config map[string]interface{}, key string) []string {
	switch v := config[key].(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func mapFromConfig(config map[string]interface{}, key string) map[string]interface{} {
	if m, ok := config[key].(map[string]interface{}); ok {
		return m
	}
	if m, ok := config[key].(entity.JSONMap); ok {
		return map[string]interface{}(m)
	}
	return nil
}

func mindMapPrompt(inputText string) string {
	return `- Role: You're a talent text processor to summarize a piece of text into a mind map.

- Step of task:
  1. Generate a title for user's 'TEXT'.
  2. Classify the 'TEXT' into sections of a mind map.
  3. If the subject matter is really complex, split them into sub-sections and sub-subsections.
  4. Add a shot content summary of the bottom level section.

- Output requirement:
  - Generate at least 4 levels.
  - Always try to maximize the number of sub-sections.
  - In language of 'Text'
  - MUST IN FORMAT OF MARKDOWN

-TEXT-
` + inputText + "\n"
}

type mindMapNode struct {
	ID       string        `json:"id"`
	Children []mindMapNode `json:"children"`
}

var mindMapHeadingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
var mindMapListRe = regexp.MustCompile(`^(\s*)(?:[-*+]|\d+\.)\s+(.+)$`)
var mindMapThinkRe = regexp.MustCompile(`(?s)<think>.*?(?:</think>|$)`)
var mindMapFenceRe = regexp.MustCompile("(?m)^```[^\n]*$")

func parseMindMapMarkdown(text string) mindMapNode {
	text = mindMapThinkRe.ReplaceAllString(text, "")
	text = mindMapFenceRe.ReplaceAllString(text, "")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	root := mindMapNode{ID: "root", Children: []mindMapNode{}}
	stack := []*mindMapNode{&root}
	listBaseLevel := 1
	lastWasList := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			lastWasList = false
			continue
		}
		level := 0
		title := ""
		if m := mindMapHeadingRe.FindStringSubmatch(trimmed); len(m) == 3 {
			level = len(m[1])
			title = cleanMindMapText(m[2])
			lastWasList = false
		} else if m := mindMapListRe.FindStringSubmatch(line); len(m) == 3 {
			rawLevel := len(m[1])/2 + 1
			if !lastWasList {
				listBaseLevel = len(stack)
			}
			level = listBaseLevel + rawLevel - 1
			title = cleanMindMapText(m[2])
			lastWasList = true
		}
		if title == "" {
			lastWasList = false
			continue
		}
		for len(stack) > level {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		parent.Children = append(parent.Children, mindMapNode{ID: title, Children: []mindMapNode{}})
		stack = append(stack, &parent.Children[len(parent.Children)-1])
	}
	if len(root.Children) == 1 {
		return root.Children[0]
	}
	return root
}

func cleanMindMapText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`")
	text = strings.Trim(text, "*_ ")
	return strings.TrimSpace(text)
}
