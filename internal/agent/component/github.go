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
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
	"ragflow/internal/tokenizer"
)

const githubPromptMaxTokens = 200000

type githubInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string) (string, error)
}

// githubComponent is the Canvas-facing GitHub repository search component.
// It mirrors agent/tools/github.py: query is a runtime input, while top_n is
// validated once from node parameters and defaults to ten.
type githubComponent struct {
	inner githubInvoker
}

func newGitHubComponent(params map[string]any) (Component, error) {
	toolParams := make(map[string]any, 1)
	for _, key := range []string{"top_n"} {
		if value, ok := params[key]; ok {
			toolParams[key] = value
		}
	}
	inner, err := agenttool.BuildByName("github", toolParams)
	if err != nil {
		return nil, err
	}
	invoker, ok := inner.(githubInvoker)
	if !ok {
		return nil, fmt.Errorf("GitHub: tool does not implement InvokableRun")
	}
	return newGitHubComponentWithInvoker(invoker), nil
}

func newGitHubComponentWithInvoker(inner githubInvoker) Component {
	return &githubComponent{inner: inner}
}

func (c *githubComponent) Name() string { return "GitHub" }

func (c *githubComponent) Inputs() map[string]string {
	return map[string]string{
		"query": "The search keywords to execute with GitHub. Use the most important terms and synonyms from the original request.",
	}
}

func (c *githubComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"type": "line",
			"name": "Query",
		},
	}
}

func (c *githubComponent) Outputs() map[string]string {
	return map[string]string{
		"formalized_content": "GitHub repositories formatted for downstream prompts.",
		"json":               "Raw GitHub repository items.",
	}
}

func (c *githubComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := stringParam(inputs["query"])
	if query == "" {
		return map[string]any{"formalized_content": "", "json": []any{}}, nil
	}
	argsJSON, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return nil, fmt.Errorf("canvas: GitHub: encode query: %w", err)
	}
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	items := anySlice(decoded["results"])
	if existing, _ := decoded["_ERROR"].(string); strings.TrimSpace(existing) != "" {
		return map[string]any{"formalized_content": "", "json": items, "_ERROR": existing}, nil
	}
	if err != nil {
		if len(decoded) > 0 {
			return map[string]any{"formalized_content": "", "json": items, "_ERROR": decoded["_ERROR"]}, nil
		}
		return nil, fmt.Errorf("canvas: GitHub: %w", err)
	}
	chunks, docAggs := buildGitHubReferences(items)
	if state, _, stateErr := runtime.GetStateFromContext[*runtime.CanvasState](ctx); stateErr == nil && state != nil {
		state.SetRetrievalReferences(chunks, docAggs)
	}
	return map[string]any{
		"formalized_content": renderGitHubReferences(chunks),
		"json":               items,
	}, nil
}

func (c *githubComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func buildGitHubReferences(items []any) ([]map[string]any, []map[string]any) {
	chunks := make([]map[string]any, 0, len(items))
	docAggs := make([]map[string]any, 0, len(items))
	for _, item := range items {
		repository, ok := item.(map[string]any)
		if !ok {
			continue
		}
		content := truncateRunes(githubValueString(repository["description"])+"\n stars:"+githubValueString(repository["watchers"]), 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(githubHashInt(content, 100000000), 10)
		title := githubValueString(repository["name"])
		url := githubValueString(repository["html_url"])
		displayID := strconv.FormatInt(githubHashInt(documentID, 500), 10)
		chunks = append(chunks, map[string]any{
			"id":            displayID,
			"chunk_id":      documentID,
			"content":       content,
			"doc_id":        documentID,
			"document_id":   documentID,
			"docnm_kwd":     title,
			"document_name": title,
			"similarity":    1,
			"score":         1,
			"url":           url,
		})
		docAggs = append(docAggs, map[string]any{
			"doc_name": title,
			"doc_id":   documentID,
			"count":    1,
			"url":      url,
		})
	}
	return chunks, docAggs
}

// renderGitHubReferences mirrors the Python
// "\n".join(kb_prompt({"chunks": chunks, "doc_aggs": aggs}, 200000, True))
// layout for the GitHub chunks built above.
func renderGitHubReferences(chunks []map[string]any) string {
	chunks = limitGitHubReferences(chunks, githubPromptMaxTokens)
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + githubValueString(chunk["id"]),
			"├── Title: " + githubValueString(chunk["docnm_kwd"]),
			"├── URL: " + githubValueString(chunk["url"]),
			"└── Content:\n" + githubValueString(chunk["content"]),
		}, "\n"))
	}
	return strings.Join(blocks, "\n")
}

// limitGitHubReferences mirrors kb_prompt's 200000-token guard. References
// are recorded before this step, as Python calls add_reference before
// formatting the prompt.
func limitGitHubReferences(chunks []map[string]any, maxTokens int) []map[string]any {
	if maxTokens <= 0 {
		return nil
	}
	usedTokens := 0
	for index, chunk := range chunks {
		content := githubValueString(chunk["content"])
		if content == "" {
			continue
		}
		usedTokens += tokenizer.NumTokensFromString(content)
		if float64(maxTokens)*0.97 < float64(usedTokens) {
			return chunks[:index+1]
		}
	}
	return chunks
}

func githubValueString(value any) string {
	if value == nil {
		return "None"
	}
	if boolean, ok := value.(bool); ok {
		if boolean {
			return "True"
		}
		return "False"
	}
	return fmt.Sprint(value)
}

func githubHashInt(value string, modulus int64) int64 {
	sum := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(sum[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
