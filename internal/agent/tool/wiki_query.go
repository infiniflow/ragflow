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
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/service/wikisearch"
)

// WikiQueryTool is the wiki_query agent tool (Python harness/tools/exploration.py
// wiki_query). It hybrid-searches the compiled wiki/artifact pages of the bound
// datasets and returns each page's rendered markdown as a chunk, narrowed by
// keywords. Input keeps query + keywords so the LLM's tool schema matches the
// other search tools.
//
// The tool is scoped to the calling tenant and bound datasets, so results never
// leak across tenants/KBs. When the wiki-search service is not configured, or the
// bound datasets have no wiki artifacts, it returns an empty result so the agent
// falls back to general hybrid search.
type WikiQueryTool struct {
	service wikisearch.Service // nil => resolve lazily via wikisearch.GetService()
	topN    int
}

// NewWikiQueryTool returns the wiki_query tool. The service is resolved lazily
// from the wikisearch singleton unless overridden for tests.
func NewWikiQueryTool() *WikiQueryTool {
	return &WikiQueryTool{topN: 12}
}

// newWikiQueryToolWithService builds a tool with an injected service, for tests.
func newWikiQueryToolWithService(service wikisearch.Service) *WikiQueryTool {
	return &WikiQueryTool{service: service, topN: 12}
}

type wikiQueryArgs struct {
	Query    string `json:"query"`
	Keywords string `json:"keywords,omitempty"`
}

func (w *WikiQueryTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "wiki_query",
		Desc: "Search the compiled wiki of the bound knowledge base(s). Returns rendered wiki page content as passages.",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query":    {Type: schema.String, Required: true, Desc: "The search query."},
			"keywords": {Type: schema.String, Desc: "Comma-separated keywords to narrow results."},
		}),
	}, nil
}

// InvokableRun executes a wiki lookup. It never returns a hard error for an
// empty/unconfigured backend so the agent can fall back to hybrid search.
func (w *WikiQueryTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	var args wikiQueryArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("wiki_query: parse arguments: %w", err)
	}
	svc := w.service
	if svc == nil {
		svc = wikisearch.GetService()
	}
	tenantID := canvasTenantID(ctx)
	datasetIDs := canvasDatasetIDs(ctx, nil)
	if svc == nil || tenantID == "" || len(datasetIDs) == 0 {
		return emptyWikiResult(), nil
	}
	if !svc.AvailableFor(ctx, tenantID, datasetIDs) {
		return emptyWikiResult(), nil
	}
	topN := w.topN
	if topN <= 0 {
		topN = 12
	}
	res, err := svc.QueryPages(ctx, tenantID, datasetIDs, strings.TrimSpace(args.Query), args.Keywords, topN)
	if err != nil {
		return emptyWikiResult(), nil
	}
	if res.Chunks == nil {
		res.Chunks = []map[string]interface{}{}
	}
	if res.DocAggs == nil {
		res.DocAggs = []map[string]interface{}{}
	}
	out, err := json.Marshal(map[string]interface{}{"answer": "", "chunks": res.Chunks, "doc_aggs": res.DocAggs})
	if err != nil {
		return emptyWikiResult(), nil
	}
	return string(out), nil
}

func emptyWikiResult() string {
	return `{"answer":"","chunks":[],"doc_aggs":[]}`
}
