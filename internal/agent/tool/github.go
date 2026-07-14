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

package tool

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const githubToolName = "github_search"

const githubToolDescription = "GitHub repository search finds repositories, projects, and codebases hosted on GitHub."

const (
	defaultGitHubTopN     = 10
	maxGitHubTopN         = 100
	githubPromptMaxTokens = 200000
)

const githubQueryDescription = "The search keywords to execute with GitHub. Use the most important terms and synonyms from the original request."

// githubParams mirrors Python GitHubParam. Info() exposes only Query to the
// model, while TopN is a canvas-side configuration value merged with defaults.
type githubParams struct {
	Query string `json:"query"`
	TopN  int    `json:"top_n"`
}

// githubResponse keeps GitHub's raw repository objects intact. The Python
// component stores response["items"] in its json output, so narrowing this to
// selected fields would change downstream DSL behaviour.
type githubResponse struct {
	Items []map[string]any `json:"items"`
}

// githubEnvelope is the shared tool-to-component transport shape.
type githubEnvelope struct {
	Results []map[string]any `json:"results"`
	Error   string           `json:"_ERROR,omitempty"`
}

// GitHubTool is the GitHub
// repository search tool. It
// GETs the GitHub Search API via the shared HTTPHelper and returns the
// top N repository matches.
type GitHubTool struct {
	helper   *HTTPHelper
	defaults githubParams
}

var _ ToolInvoker = (*GitHubTool)(nil)
var _ ToolComponent = (*GitHubTool)(nil)
var _ ReferenceBuilder = (*GitHubTool)(nil)
var _ ComponentOutputBuilder = (*GitHubTool)(nil)

// NewGitHubTool returns a GitHubTool using the default HTTPHelper.
func NewGitHubTool() *GitHubTool {
	return NewGitHubToolWithDefaults(nil, githubParams{})
}

// NewGitHubToolWith returns a GitHubTool that uses the provided
// HTTPHelper. Useful for tests.
func NewGitHubToolWith(h *HTTPHelper) *GitHubTool {
	return NewGitHubToolWithDefaults(h, githubParams{})
}

// NewGitHubToolWithDefaults returns a GitHubTool with component-level
// defaults. It follows NewPubMedToolWithDefaults: the constructor owns
// defaults while Info() exposes only the model-call input schema.
func NewGitHubToolWithDefaults(h *HTTPHelper, defaults githubParams) *GitHubTool {
	if h == nil {
		// Python uses DEFAULT_TIMEOUT (15 seconds) and ToolParamBase starts
		// with max_retries=0, so a default GitHub component makes one request.
		h = NewHTTPHelperWithRetry(RetryConfig{MaxAttempts: 1})
		h.client.Timeout = 15 * time.Second
	}
	if defaults.TopN == 0 {
		defaults.TopN = defaultGitHubTopN
	}
	return &GitHubTool{helper: h, defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (g *GitHubTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: githubToolName,
		Desc: githubToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     githubQueryDescription,
				Required: true,
			},
		}),
	}, nil
}

// buildGitHubURL constructs the repository search URL used by the Python
// GitHub component: most-starred repositories first, with per_page=top_n.
func buildGitHubURL(query string, topN int) string {
	if topN <= 0 {
		topN = defaultGitHubTopN
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("sort", "stars")
	q.Set("order", "desc")
	q.Set("per_page", fmt.Sprintf("%d", topN))
	return "https://api.github.com/search/repositories?" + q.Encode()
}

// InvokableRun performs the GitHub repository search.
func (g *GitHubTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p githubParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return githubErrJSON(fmt.Errorf("github: parse arguments: %w", err)),
			fmt.Errorf("github: parse arguments: %w", err)
	}
	if p.Query == "" {
		return githubJSON(githubEnvelope{Results: []map[string]any{}}), nil
	}
	p = mergeGitHubDefaults(g.defaults, p)

	endpoint := buildGitHubURL(p.Query, p.TopN)
	headers := map[string]string{
		"Content-Type":         "application/vnd.github+json",
		"X-GitHub-Api-Version": "2022-11-28",
	}

	resp, err := g.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return githubErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return githubErrJSON(fmt.Errorf("github: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("github: upstream returned %d", resp.StatusCode)
	}

	var raw githubResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return githubErrJSON(fmt.Errorf("github: decode response: %w", err)),
			fmt.Errorf("github: decode response: %w", err)
	}
	return githubJSON(githubEnvelope{Results: raw.Items}), nil
}

func mergeGitHubDefaults(defaults, params githubParams) githubParams {
	if params.TopN == 0 {
		params.TopN = defaults.TopN
	}
	return params
}

func githubJSON(env githubEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"github: marshal result: %s"}`, err)
	}
	return string(b)
}

func githubErrJSON(err error) string {
	return githubJSON(githubEnvelope{Error: err.Error()})
}

// ComponentSpec returns the Python-compatible GitHub Canvas surface.
func (g *GitHubTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query": githubQueryDescription,
		},
		Outputs: map[string]string{
			"formalized_content": "GitHub repositories formatted for downstream prompts.",
			"json":               "Raw GitHub repository items.",
		},
		InputForm: map[string]any{
			"query": map[string]any{
				"type": "line",
				"name": "Query",
			},
		},
	}
}

// BuildReferences creates the chunks and document aggregates Python's
// ToolBase._retrieve_chunks records for GitHub results.
func (g *GitHubTool) BuildReferences(_ context.Context, results []any) ([]map[string]any, []map[string]any) {
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		repository, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := truncateGitHubRunes(githubValueString(repository["description"])+"\n stars:"+githubValueString(repository["watchers"]), 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(githubHashInt(content, 100000000), 10)
		title := githubValueString(repository["name"])
		resultURL := githubValueString(repository["html_url"])
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
			"url":           resultURL,
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

// BuildComponentOutputs constructs GitHub's complete Canvas output map.
func (g *GitHubTool) BuildComponentOutputs(results []any, chunks []map[string]any) map[string]any {
	return map[string]any{
		"json":               results,
		"formalized_content": g.RenderResults(results, chunks),
	}
}

// RenderResults renders the same chunks that the Canvas adapter records.
func (g *GitHubTool) RenderResults(_ []any, chunks []map[string]any) string {
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

func truncateGitHubRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
