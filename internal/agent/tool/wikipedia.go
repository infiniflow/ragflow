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

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const wikipediaToolName = "wikipedia"

const wikipediaToolDescription = "Search Wikipedia and return matching articles as {title, snippet, url}."

// wikipediaParams is the JSON shape the model sends into InvokableRun.
// lang is the language subdomain (e.g. "en", "zh", "de"); max_results
// defaults to 5 when unset or non-positive.
type wikipediaParams struct {
	Query      string `json:"query"`
	Lang       string `json:"lang"`
	MaxResults int    `json:"max_results"`
}

// wikipediaResult is one row in the upstream `query.search` array.
type wikipediaResult struct {
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
	URL     string `json:"url"`
}

// wikipediaResponse is the upstream MediaWiki API envelope.
type wikipediaResponse struct {
	Query struct {
		Search []wikipediaResult `json:"search"`
	} `json:"query"`
}

// wikipediaEnvelope is what the model sees. It mirrors the Python tool's
// output: a flat list of {title, snippet, url} entries.
type wikipediaEnvelope struct {
	Results []wikipediaResult `json:"results"`
	Error   string            `json:"_ERROR,omitempty"`
}

// WikipediaTool is the Wikipedia
// search tool. It calls the
// public MediaWiki action API via the shared HTTPHelper and returns the
// top N matches for the query.
type WikipediaTool struct {
	helper *HTTPHelper
}

// NewWikipediaTool returns a WikipediaTool using the default HTTPHelper.
func NewWikipediaTool() *WikipediaTool {
	return NewWikipediaToolWith(NewHTTPHelper())
}

// NewWikipediaToolWith returns a WikipediaTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewWikipediaToolWith(h *HTTPHelper) *WikipediaTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &WikipediaTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (w *WikipediaTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: wikipediaToolName,
		Desc: wikipediaToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query",
				Required: true,
			},
			"lang": {
				Type:     schema.String,
				Desc:     `Language subdomain. Defaults to "en".`,
				Required: false,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 5.",
				Required: false,
			},
		}),
	}, nil
}

// buildWikipediaURL constructs the MediaWiki action=query URL. Centralized
// so the test suite can verify URL encoding without spinning up a server.
func buildWikipediaURL(lang, query string, maxResults int) string {
	if lang == "" {
		lang = "en"
	}
	if maxResults <= 0 {
		maxResults = 5
	}
	return fmt.Sprintf(
		"https://%s.wikipedia.org/w/api.php?action=query&list=search&format=json&srlimit=%d&srsearch=%s",
		lang,
		maxResults,
		url.QueryEscape(query),
	)
}

// InvokableRun performs the Wikipedia search.
func (w *WikipediaTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p wikipediaParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return wikipediaErrJSON(fmt.Errorf("wikipedia: parse arguments: %w", err)),
			fmt.Errorf("wikipedia: parse arguments: %w", err)
	}
	if p.Query == "" {
		return wikipediaErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("wikipedia: query is required")
	}

	endpoint := buildWikipediaURL(p.Lang, p.Query, p.MaxResults)
	resp, err := w.helper.Do(ctx, http.MethodGet, endpoint, "", "", nil)
	if err != nil {
		return wikipediaErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return wikipediaErrJSON(fmt.Errorf("wikipedia: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("wikipedia: upstream returned %d", resp.StatusCode)
	}

	var raw wikipediaResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return wikipediaErrJSON(fmt.Errorf("wikipedia: decode response: %w", err)),
			fmt.Errorf("wikipedia: decode response: %w", err)
	}

	// Compose canonical Wikipedia URLs for the snippets. The MediaWiki
	// search endpoint doesn't include a URL; the conventional one is
	// https://<lang>.wikipedia.org/wiki/<Title>.
	lang := p.Lang
	if lang == "" {
		lang = "en"
	}
	results := make([]wikipediaResult, 0, len(raw.Query.Search))
	for _, r := range raw.Query.Search {
		results = append(results, wikipediaResult{
			Title:   r.Title,
			Snippet: r.Snippet,
			URL:     fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(r.Title)),
		})
	}
	return wikipediaJSON(wikipediaEnvelope{Results: results}), nil
}

func wikipediaJSON(env wikipediaEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"wikipedia: marshal result: %s"}`, err)
	}
	return string(b)
}

func wikipediaErrJSON(err error) string {
	return wikipediaJSON(wikipediaEnvelope{Error: err.Error()})
}
