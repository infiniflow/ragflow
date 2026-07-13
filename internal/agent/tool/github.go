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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const githubToolName = "github_search"

const githubToolDescription = "GitHub repository search finds repositories, projects, and codebases hosted on GitHub."

const (
	defaultGitHubTopN = 10
	maxGitHubTopN     = 100
)

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

// ToolMeta returns the tool's metadata for the chat model.
func (g *GitHubTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        githubToolName,
		Description: githubToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "The search keywords to execute with GitHub. Use the most important terms and synonyms from the original request.",
				Required:    true,
			},
		},
	}
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
func (g *GitHubTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var p githubParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return githubErrJSON(fmt.Errorf("github: parse arguments: %w", err)),
			fmt.Errorf("github: parse arguments: %w", err)
	}
	if p.Query == "" {
		return githubErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("github: query is required")
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
