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
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const githubToolName = "github"

const githubToolDescription = "Search GitHub repositories. Returns items[].{full_name, html_url, description, stargazers_count}."

// githubParams is the JSON shape the model sends into InvokableRun. token
// is optional — anonymous requests succeed but are heavily rate-limited
// (60/hr vs 5000/hr with a PAT).
type githubParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
	Token      string `json:"token"`
}

// githubResult mirrors one element of the upstream `items` array.
type githubResult struct {
	FullName        string `json:"full_name"`
	HTMLURL         string `json:"html_url"`
	Description     string `json:"description"`
	StargazersCount int    `json:"stargazers_count"`
}

// githubResponse is the upstream GitHub Search envelope.
type githubResponse struct {
	Items []githubResult `json:"items"`
}

// githubEnvelope is what the model sees.
type githubEnvelope struct {
	Results []githubResult `json:"results"`
	Error   string         `json:"_ERROR,omitempty"`
}

// GitHubTool is the GitHub
// repository search tool. It
// GETs the GitHub Search API via the shared HTTPHelper and returns the
// top N repository matches.
type GitHubTool struct {
	helper *HTTPHelper
}

// NewGitHubTool returns a GitHubTool using the default HTTPHelper.
func NewGitHubTool() *GitHubTool {
	return NewGitHubToolWith(NewHTTPHelper())
}

// NewGitHubToolWith returns a GitHubTool that uses the provided
// HTTPHelper. Useful for tests.
func NewGitHubToolWith(h *HTTPHelper) *GitHubTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GitHubTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (g *GitHubTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: githubToolName,
		Desc: githubToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query (GitHub search syntax).",
				Required: true,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 5 (max 30 per page).",
				Required: false,
			},
			"token": {
				Type:     schema.String,
				Desc:     "Optional GitHub personal access token. Increases rate limit from 60 to 5000 req/hr.",
				Required: false,
			},
		}),
	}, nil
}

// buildGitHubURL constructs the GitHub repository search URL. Centralized
// so the test suite can verify URL encoding without spinning up a server.
func buildGitHubURL(query string, maxResults int) string {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 30 {
		maxResults = 30
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("per_page", fmt.Sprintf("%d", maxResults))
	return "https://api.github.com/search/repositories?" + q.Encode()
}

// InvokableRun performs the GitHub repository search.
func (g *GitHubTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p githubParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return githubErrJSON(fmt.Errorf("github: parse arguments: %w", err)),
			fmt.Errorf("github: parse arguments: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return githubErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("github: query is required")
	}

	endpoint := buildGitHubURL(p.Query, p.MaxResults)
	headers := map[string]string{
		"Accept": "application/vnd.github+json",
	}
	if p.Token != "" {
		headers["Authorization"] = "Bearer " + p.Token
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
