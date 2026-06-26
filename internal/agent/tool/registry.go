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
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
)

// Factory builds a tool instance by DSL / Agent-visible name and
// optional node-level configuration. The config map belongs to the
// Agent node / DSL, not to the model-emitted function-call args.
type Factory func(params map[string]any) (einotool.BaseTool, error)

var registry = map[string]Factory{
	"akshare":           noConfig("akshare", func() einotool.BaseTool { return NewAkShareTool() }),
	"arxiv":             noConfig("arxiv", func() einotool.BaseTool { return NewArxivTool() }),
	"code_exec":         noConfig("code_exec", func() einotool.BaseTool { return NewCodeExecTool() }),
	"crawler":           noConfig("crawler", func() einotool.BaseTool { return NewCrawlerTool() }),
	"deepl":             noConfig("deepl", func() einotool.BaseTool { return NewDeepLTool() }),
	"duckduckgo":        noConfig("duckduckgo", func() einotool.BaseTool { return NewDuckDuckGoTool() }),
	"email":             noConfig("email", func() einotool.BaseTool { return NewEmailTool() }),
	"execute_sql":       buildExeSQLTool,
	"exesql":            buildExeSQLTool,
	"github":            noConfig("github", func() einotool.BaseTool { return NewGitHubTool() }),
	"google":            noConfig("google", func() einotool.BaseTool { return NewGoogleTool() }),
	"google_scholar":    noConfig("google_scholar", func() einotool.BaseTool { return NewGoogleScholarTool() }),
	"jin10":             noConfig("jin10", func() einotool.BaseTool { return NewJin10Tool() }),
	"pubmed":            noConfig("pubmed", func() einotool.BaseTool { return NewPubMedTool() }),
	"qweather":          noConfig("qweather", func() einotool.BaseTool { return NewQWeatherTool() }),
	"retrieval":         noConfig("retrieval", func() einotool.BaseTool { return NewRetrievalTool() }),
	"search_my_dataset": noConfig("search_my_dataset", func() einotool.BaseTool { return NewRetrievalTool() }),
	"search_my_dateset": noConfig("search_my_dateset", func() einotool.BaseTool { return NewRetrievalTool() }),
	"searxng":           noConfig("searxng", func() einotool.BaseTool { return NewSearXNGTool() }),
	"tavily":            noConfig("tavily", func() einotool.BaseTool { return NewTavilyTool() }),
	"tushare":           noConfig("tushare", func() einotool.BaseTool { return NewTushareTool() }),
	"wencai":            noConfig("wencai", func() einotool.BaseTool { return NewWencaiTool() }),
	"wikipedia":         noConfig("wikipedia", func() einotool.BaseTool { return NewWikipediaTool() }),
	"yahoo_finance":     noConfig("yahoo_finance", func() einotool.BaseTool { return NewYahooFinanceTool() }),
}

func noConfig(name string, fn func() einotool.BaseTool) Factory {
	return func(params map[string]any) (einotool.BaseTool, error) {
		if len(params) != 0 {
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level params", name)
		}
		return fn(), nil
	}
}

// BuildByName resolves a tool name into an Eino BaseTool.
func BuildByName(name string, params map[string]any) (einotool.BaseTool, error) {
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

// BuildAll resolves a list of tool names into Eino BaseTool instances.
// perToolParams is keyed by the Agent-visible tool name.
func BuildAll(names []string, perToolParams map[string]map[string]any) ([]einotool.BaseTool, error) {
	if len(names) == 0 {
		return nil, nil
	}
	tools := make([]einotool.BaseTool, 0, len(names))
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

func buildExeSQLTool(params map[string]any) (einotool.BaseTool, error) {
	conn, err := decodeExeSQLConnParams(params)
	if err != nil {
		return nil, err
	}
	return NewExeSQLTool(conn), nil
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
