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
	"fmt"
	"strings"
)

// Factory builds a tool instance by DSL / Agent-visible name and
// optional node-level configuration.
type Factory func(params map[string]any) (Tool, error)

var registry = map[string]Factory{
	"akshare":           noConfig("akshare", func() Tool { return NewAkShareTool() }),
	"arxiv":             noConfig("arxiv", func() Tool { return NewArxivTool() }),
	"code_exec":         noConfig("code_exec", func() Tool { return NewCodeExecTool() }),
	"crawler":           noConfig("crawler", func() Tool { return NewCrawlerTool() }),
	"deepl":             noConfig("deepl", func() Tool { return NewDeepLTool() }),
	"duckduckgo":        noConfig("duckduckgo", func() Tool { return NewDuckDuckGoTool() }),
	"email":             noConfig("email", func() Tool { return NewEmailTool() }),
	"execute_sql":       buildExeSQLTool,
	"exesql":            buildExeSQLTool,
	"github":            noConfig("github", func() Tool { return NewGitHubTool() }),
	"google":            noConfig("google", func() Tool { return NewGoogleTool() }),
	"google_scholar":    noConfig("google_scholar", func() Tool { return NewGoogleScholarTool() }),
	"jin10":             noConfig("jin10", func() Tool { return NewJin10Tool() }),
	"keenable":          buildKeenableTool,
	"pubmed":            noConfig("pubmed", func() Tool { return NewPubMedTool() }),
	"qweather":          noConfig("qweather", func() Tool { return NewQWeatherTool() }),
	"retrieval":         noConfig("retrieval", func() Tool { return NewRetrievalTool() }),
	"search_my_dataset": noConfig("search_my_dataset", func() Tool { return NewRetrievalTool() }),
	"search_my_dateset": noConfig("search_my_dateset", func() Tool { return NewRetrievalTool() }),
	"searxng":           noConfig("searxng", func() Tool { return NewSearXNGTool() }),
	"tavily":            noConfig("tavily", func() Tool { return NewTavilyTool() }),
	"tushare":           noConfig("tushare", func() Tool { return NewTushareTool() }),
	"wencai":            noConfig("wencai", func() Tool { return NewWencaiTool() }),
	"wikipedia":         noConfig("wikipedia", func() Tool { return NewWikipediaTool() }),
	"yahoo_finance":     noConfig("yahoo_finance", func() Tool { return NewYahooFinanceTool() }),
}

func noConfig(name string, fn func() Tool) Factory {
	return func(params map[string]any) (Tool, error) {
		if len(params) != 0 {
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level params", name)
		}
		return fn(), nil
	}
}

// BuildByName resolves a tool name into a Tool.
func BuildByName(name string, params map[string]any) (Tool, error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil, fmt.Errorf("agent tool: empty tool name")
	}
	factory, ok := registry[key]
	if !ok {
		return nil, fmt.Errorf("agent tool: unsupported tool %q", name)
	}
	if factory == nil {
		return nil, fmt.Errorf("agent tool: nil factory for %q", name)
	}
	return factory(params)
}

// BuildAll resolves a list of tool names into Tool instances.
func BuildAll(names []string, perToolParams map[string]map[string]any) ([]Tool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	tools := make([]Tool, 0, len(names))
	for _, name := range names {
		var params map[string]any
		if perToolParams != nil {
			params = perToolParams[strings.ToLower(strings.TrimSpace(name))]
			if params == nil {
				params = perToolParams[name]
			}
		}
		t, err := BuildByName(name, params)
		if err != nil {
			return nil, err
		}
		tools = append(tools, t)
	}
	return tools, nil
}

func buildExeSQLTool(params map[string]any) (Tool, error) {
	conn, err := decodeExeSQLConnParams(params)
	if err != nil {
		return nil, err
	}
	return NewExeSQLTool(conn), nil
}

func buildKeenableTool(params map[string]any) (Tool, error) {
	inner := NewKeenableTool()
	if len(params) == 0 {
		return &keenableToolAdapter{inner: inner}, nil
	}
	for key := range params {
		if key != "api_key" {
			return nil, fmt.Errorf("agent tool: tool %q only accepts node-level param api_key", "keenable")
		}
	}
	apiKey, ok := params["api_key"].(string)
	if !ok || strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("agent tool: tool %q requires non-empty string node-level param api_key", "keenable")
	}
	return &keenableToolAdapter{inner: NewKeenableToolWithAPIKey(nil, apiKey)}, nil
}

// keenableToolAdapter wraps *KeenableTool (which has an Eino-style
// InvokableRun with variadic tool.Option) into the harness Tool
// interface (InvokableRun with strict two-arg signature).
type keenableToolAdapter struct {
	inner *KeenableTool
}

func (a *keenableToolAdapter) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        keenableToolName,
		Description: keenableToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "Search keywords to execute with Keenable. The most important words/terms (and synonyms) from the original request.",
				Required:    true,
			},
			"site": {
				Type:        ParamTypeString,
				Description: "Optional. Restrict results to a single domain, e.g. 'techcrunch.com'.",
				Required:    false,
			},
			"mode": {
				Type:        ParamTypeString,
				Description: `Search mode: "pro" (default, deeper) or "realtime" (low latency; requires a server-configured API key).`,
				Required:    false,
			},
			"top_n": {
				Type:        ParamTypeInteger,
				Description: "Maximum number of results to return. Defaults to 10.",
				Required:    false,
			},
		},
	}
}

func (a *keenableToolAdapter) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	return a.inner.InvokableRun(ctx, argsJSON)
}

func decodeExeSQLConnParams(params map[string]any) (exesqlConnParams, error) {
	if len(params) == 0 {
		return exesqlConnParams{}, fmt.Errorf(
			"agent tool: execute_sql requires node-level params " +
				"(db_type/host/port/database/username/password)",
		)
	}
	conn := exesqlConnParams{}
	if v, ok := stringParam(params, "db_type"); ok {
		conn.DBType = v
	}
	if v, ok := stringParam(params, "database"); ok {
		conn.Database = v
	}
	if v, ok := stringParam(params, "username"); ok {
		conn.Username = v
	}
	if v, ok := stringParam(params, "host"); ok {
		conn.Host = v
	}
	if v, ok := intParam(params, "port"); ok {
		conn.Port = v
	}
	if v, ok := stringParam(params, "password"); ok {
		conn.Password = v
	}
	if v, ok := intParam(params, "max_records"); ok {
		conn.MaxRecords = v
	}
	if err := conn.check(); err != nil {
		return exesqlConnParams{}, fmt.Errorf("agent tool: execute_sql config: %w", err)
	}
	return conn, nil
}

func stringParam(params map[string]any, key string) (string, bool) {
	v, ok := params[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func intParam(params map[string]any, key string) (int, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	default:
		return 0, false
	}
}
