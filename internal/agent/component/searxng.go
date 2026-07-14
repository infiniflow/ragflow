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

package component

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/tokenizer"
)

const searxngPromptTokenLimit = 200000

var searxngDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var searxngNewlinePattern = regexp.MustCompile(`\n+`)

type searxngComponent struct {
	inner toolInvoker
}

func newSearXNGComponentWithInvoker(inner toolInvoker) Component {
	return &searxngComponent{inner: inner}
}

func (c *searxngComponent) Name() string { return "SearXNG" }

func (c *searxngComponent) Inputs() map[string]string {
	return map[string]string{
		"query":       "The search keywords to execute with SearXNG.",
		"searxng_url": "The base URL of the SearXNG instance.",
	}
}

func (c *searxngComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "Rendered SearXNG references for downstream LLM prompts.",
		"json":               "Raw SearXNG result list.",
	}
}

func (c *searxngComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
		"searxng_url": map[string]any{
			"name":        "SearXNG URL",
			"type":        "line",
			"placeholder": "http://localhost:4000",
		},
	}
}

func (c *searxngComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := stringParam(inputs["query"])
	if strings.TrimSpace(query) == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	args := map[string]any{"query": query}
	if searxngURL, ok := inputs["searxng_url"].(string); ok {
		args["searxng_url"] = searxngURL
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("canvas: SearXNG: encode inputs: %w", err)
	}

	out, invokeErr := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	results := anySlice(decoded["results"])
	if message, _ := decoded["_ERROR"].(string); strings.TrimSpace(message) != "" {
		return map[string]any{
			"formalized_content": "",
			"json":               results,
			"_ERROR":             message,
		}, nil
	}
	if invokeErr != nil {
		return nil, fmt.Errorf("canvas: SearXNG: %w", invokeErr)
	}

	chunks, docAggs := buildSearXNGReferences(results)
	if state, _, stateErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); stateErr == nil && state != nil {
		state.SetRetrievalReferences(chunks, docAggs)
	}
	return map[string]any{
		"formalized_content": renderSearXNGReferences(chunks, searxngPromptTokenLimit),
		"json":               results,
	}, nil
}

func (c *searxngComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func buildSearXNGReferences(results []any) ([]map[string]any, []map[string]any) {
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content, _ := item["content"].(string)
		if content == "" {
			continue
		}
		content = searxngDataImagePattern.ReplaceAllString(content, "")
		runes := []rune(content)
		if len(runes) > 10000 {
			content = string(runes[:10000])
		}
		if content == "" {
			continue
		}

		documentID := strconv.Itoa(hashSearXNGString(content, 100000000))
		displayID := strconv.Itoa(hashSearXNGString(documentID, 500))
		title := searxngText(item["title"])
		resultURL := searxngText(item["url"])
		chunks = append(chunks, map[string]any{
			"id":                displayID,
			"chunk_id":          documentID,
			"content":           content,
			"doc_id":            documentID,
			"docnm_kwd":         title,
			"document_id":       documentID,
			"document_name":     title,
			"dataset_id":        nil,
			"image_id":          nil,
			"positions":         nil,
			"url":               resultURL,
			"similarity":        1,
			"vector_similarity": nil,
			"term_similarity":   nil,
			"row_id":            nil,
			"doc_type":          nil,
			"document_metadata": nil,
		})
		docAggs = append(docAggs, map[string]any{
			"doc_name": title,
			"doc_id":   documentID,
			"count":    1,
			"url":      resultURL,
		})
	}
	return chunks, docAggs
}

func renderSearXNGReferences(chunks []map[string]any, maxTokens int) string {
	if len(chunks) == 0 {
		return ""
	}
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := searxngText(chunk["content"])
		if content == "" {
			continue
		}
		usedTokens += tokenizer.NumTokensFromString(content)
		var block strings.Builder
		fmt.Fprintf(&block, "\nID: %s", searxngText(chunk["id"]))
		if title := searxngPromptField(chunk["document_name"]); title != "" {
			fmt.Fprintf(&block, "\n├── Title: %s", title)
		}
		if resultURL := searxngPromptField(chunk["url"]); resultURL != "" {
			fmt.Fprintf(&block, "\n├── URL: %s", resultURL)
		}
		block.WriteString("\n└── Content:\n")
		block.WriteString(content)
		blocks = append(blocks, block.String())
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func searxngText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func searxngPromptField(value any) string {
	return searxngNewlinePattern.ReplaceAllString(searxngText(value), " ")
}

func hashSearXNGString(value string, modulus int) int {
	digest := sha1.Sum([]byte(value))
	result := 0
	for _, part := range digest {
		result = (result*256 + int(part)) % modulus
	}
	return result
}

func init() {
	registerToolComponent(toolComponentSpec{
		componentName: "SearXNG",
		toolName:      "searxng",
		wrap:          newSearXNGComponentWithInvoker,
	})
}
