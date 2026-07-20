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
	"math"
	"strconv"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
)

// Factory builds a tool instance by DSL / Agent-visible name and
// optional node-level configuration. The config map belongs to the
// Agent node / DSL, not to the model-emitted function-call args.
type Factory func(params map[string]any) (einotool.BaseTool, error)

var registry = map[string]Factory{
	"akshare":               buildAkShareTool,
	"arxiv":                 buildArxivTool,
	"bgpt":                  buildBGPTTool,
	"code_exec":             noConfig("code_exec", func() einotool.BaseTool { return NewCodeExecTool() }),
	"crawler":               noConfig("crawler", func() einotool.BaseTool { return NewCrawlerTool() }),
	"deepl":                 noConfig("deepl", func() einotool.BaseTool { return NewDeepLTool() }),
	"duckduckgo":            buildDuckDuckGoTool,
	"email":                 buildEmailTool,
	"execute_sql":           buildExeSQLTool,
	"exesql":                buildExeSQLTool,
	"github":                buildGitHubTool,
	"google":                buildGoogleTool,
	"google_scholar":        buildGoogleScholarTool,
	"google_scholar_search": buildGoogleScholarTool,
	"jin10":                 noConfig("jin10", func() einotool.BaseTool { return NewJin10Tool() }),
	"keenable":              buildKeenableTool,
	"pubmed":                buildPubMedTool,
	"qweather":              noConfig("qweather", func() einotool.BaseTool { return NewQWeatherTool() }),
	"retrieval":             buildRetrievalTool,
	"search_my_dataset":     buildRetrievalTool,
	"search_my_dateset":     buildRetrievalTool,
	"searxng":               buildSearXNGTool,
	"tavily":                buildTavilyTool,
	"tavily_extract":        buildTavilyExtractTool,
	"tushare":               noConfig("tushare", func() einotool.BaseTool { return NewTushareTool() }),
	"wencai":                buildWencaiTool,
	"web_crawler":           noConfig("web_crawler", func() einotool.BaseTool { return NewCrawlerTool() }),
	"wikipedia":             buildWikipediaTool,
	"wikipedia_search":      buildWikipediaTool,
	"yahoo_finance":         buildYahooFinanceTool,
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

func buildAkShareTool(params map[string]any) (einotool.BaseTool, error) {
	topN := defaultAkShareTopN
	if len(params) != 0 {
		for key := range params {
			if key != "top_n" {
				return nil, fmt.Errorf("agent tool: tool %q only accepts node-level param top_n", "akshare")
			}
		}
		if v, ok := intParam(params, "top_n"); ok {
			topN = v
		}
		if topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "akshare")
		}
	}
	return NewAkShareToolWithTopN(nil, topN), nil
}

func buildArxivTool(params map[string]any) (einotool.BaseTool, error) {
	topN := defaultArxivTopN
	sortBy := defaultArxivSortBy
	if v, ok := intParam(params, "top_n"); ok {
		topN = v
	}
	if topN <= 0 {
		return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "arxiv")
	}
	if v, ok := stringParam(params, "sort_by"); ok {
		sortBy = v
	}
	if !ArxivSortBySupported(sortBy) {
		return nil, fmt.Errorf("agent tool: tool %q has unsupported sort_by %q", "arxiv", sortBy)
	}
	return NewArxivToolWithParams(nil, topN, sortBy), nil
}

func buildBGPTTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := bgptParams{}
	if value, ok := params["api_key"]; ok {
		apiKey, valid := value.(string)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires string node-level param api_key", "bgpt")
		}
		defaults.APIKey = apiKey
	}
	if value, ok := params["top_n"]; ok {
		topN, valid := strictInt(value)
		if text, isString := value.(string); isString {
			parsed, err := strconv.Atoi(strings.TrimSpace(text))
			topN, valid = parsed, err == nil
		}
		if !valid || topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "bgpt")
		}
		defaults.TopN = topN
	}
	if value, ok := params["days_back"]; ok && value != nil && value != "" {
		daysBack, valid := strictInt(value)
		if !valid || daysBack <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param days_back", "bgpt")
		}
		defaults.DaysBack = daysBack
	}
	return newBGPTTool(nil, defaults), nil
}

func buildExeSQLTool(params map[string]any) (einotool.BaseTool, error) {
	conn, err := decodeExeSQLConnParams(params)
	if err != nil {
		return nil, err
	}
	return NewExeSQLTool(conn), nil
}

func buildEmailTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := emailParams{SMTPPort: 465}
	stringFields := map[string]*string{
		"smtp_server":   &defaults.SMTPServer,
		"email":         &defaults.Email,
		"smtp_username": &defaults.SMTPUsername,
		"password":      &defaults.Password,
		"sender_name":   &defaults.SenderName,
		"to_email":      &defaults.ToEmail,
		"cc_email":      &defaults.CCEmail,
		"content":       &defaults.Content,
		"subject":       &defaults.Subject,
	}
	for key, destination := range stringFields {
		value, exists := params[key]
		if !exists {
			continue
		}
		text, valid := value.(string)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires string node-level param %s", "email", key)
		}
		*destination = text
	}
	if value, exists := params["smtp_port"]; exists {
		port, valid := strictInt(value)
		if text, isString := value.(string); isString {
			parsed, err := strconv.Atoi(strings.TrimSpace(text))
			port, valid = parsed, err == nil
		}
		if !valid || port <= 0 || port > 65535 {
			return nil, fmt.Errorf("agent tool: tool %q requires integer node-level param smtp_port in [1, 65535]", "email")
		}
		defaults.SMTPPort = port
	}
	return newEmailTool(defaults), nil
}

func buildDuckDuckGoTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := duckduckgoParams{}
	if value, ok := params["top_n"]; ok {
		topN, valid := strictInt(value)
		if !valid || topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "duckduckgo")
		}
		defaults.TopN = topN
	}
	if value, ok := params["channel"]; ok {
		channel, valid := value.(string)
		if !valid || (channel != "text" && channel != "general" && channel != "news") {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported channel %q", "duckduckgo", channel)
		}
		defaults.Channel = normalizeDuckDuckGoChannel(channel)
	}
	return newDuckDuckGoTool(nil, defaults), nil
}

func buildGoogleTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := googleParams{}
	if v, ok := stringParam(params, "api_key"); ok {
		defaults.APIKey = v
	}
	if v, ok := stringParam(params, "country"); ok {
		defaults.Country = v
	}
	if v, ok := stringParam(params, "language"); ok {
		defaults.Language = v
	}
	if v, ok := stringParam(params, "q"); ok {
		defaults.Q = v
	}
	if v, ok := intParam(params, "start"); ok {
		defaults.Start = v
	}
	if v, ok := intParam(params, "num"); ok {
		defaults.Num = v
	}
	return NewGoogleToolWithDefaults(nil, defaults), nil
}

func buildGitHubTool(params map[string]any) (einotool.BaseTool, error) {
	topN := defaultGitHubTopN
	if raw, exists := params["top_n"]; exists {
		value, ok := intParam(params, "top_n")
		if !ok {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "github")
		}
		if decimal, ok := raw.(float64); ok && math.Trunc(decimal) != decimal {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "github")
		}
		topN = value
	}
	if topN <= 0 {
		return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "github")
	}
	if topN > maxGitHubTopN {
		return nil, fmt.Errorf("agent tool: tool %q requires node-level param top_n to be at most %d", "github", maxGitHubTopN)
	}
	return NewGitHubToolWithDefaults(nil, githubParams{TopN: topN}), nil
}

func buildGoogleScholarTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := googleScholarParams{}
	if v, ok := stringParam(params, "query"); ok {
		defaults.Query = v
	}
	if value, ok := params["top_n"]; ok {
		topN, valid := strictInt(value)
		if !valid || topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "google_scholar")
		}
		defaults.TopN = topN
	}
	if value, ok := params["sort_by"]; ok {
		sortBy, valid := value.(string)
		if !valid || (sortBy != "date" && sortBy != "relevance") {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported sort_by %q", "google_scholar", sortBy)
		}
		defaults.SortBy = sortBy
	}
	if value, ok := params["year_low"]; ok && value != nil {
		yearLow, valid := strictInt(value)
		if !valid || yearLow <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param year_low", "google_scholar")
		}
		defaults.YearLow = yearLow
	}
	if value, ok := params["year_high"]; ok && value != nil {
		yearHigh, valid := strictInt(value)
		if !valid || yearHigh <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param year_high", "google_scholar")
		}
		defaults.YearHigh = yearHigh
	}
	if v, ok := boolParam(params, "patents"); ok {
		defaults.Patents = &v
	}
	if value, ok := params["patents"]; ok {
		if _, valid := value.(bool); !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param patents", "google_scholar")
		}
	}
	return NewGoogleScholarToolWithDefaults(nil, defaults), nil
}

func buildPubMedTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := pubmedParams{}
	if value, ok := params["top_n"]; ok {
		topN, valid := strictInt(value)
		if !valid || topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "pubmed")
		}
		defaults.TopN = topN
	}
	if value, ok := params["email"]; ok {
		email, valid := value.(string)
		if !valid || strings.TrimSpace(email) == "" {
			return nil, fmt.Errorf("agent tool: tool %q requires non-empty string node-level param email", "pubmed")
		}
		defaults.Email = email
	}
	return NewPubMedToolWithDefaults(nil, defaults), nil
}

func buildRetrievalTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := retrievalArgs{}
	for key := range params {
		switch key {
		case "dataset_ids", "kb_ids", "top_n", "top_k", "similarity_threshold",
			"keywords_similarity_weight", "use_kg", "rerank_id", "empty_response",
			"toc_enhance", "meta_data_filter", "retrieval_from", "memory_ids",
			"kb_vars", "cross_languages", "function_name", "description", "meta",
			"inputs", "outputs":
		default:
			return nil, fmt.Errorf("agent tool: retrieval tool does not accept node-level param %s", key)
		}
	}

	if ids, ok, err := stringSliceParam(params, "dataset_ids"); err != nil {
		return nil, fmt.Errorf("agent tool: retrieval config: %w", err)
	} else if ok {
		defaults.DatasetIDs = ids
	}
	if ids, ok, err := stringSliceParam(params, "kb_ids"); err != nil {
		return nil, fmt.Errorf("agent tool: retrieval config: %w", err)
	} else if ok {
		defaults.KBIDs = ids
		if len(defaults.DatasetIDs) == 0 {
			defaults.DatasetIDs = ids
		}
	}
	if v, ok := intParam(params, "top_n"); ok {
		defaults.TopN = v
	}
	if raw, exists := params["top_k"]; exists {
		value, ok := strictInt(raw)
		if !ok || value <= 0 {
			return nil, fmt.Errorf("agent tool: retrieval tool requires positive integer node-level param top_k")
		}
		defaults.TopK = value
	}
	if v, ok := boolParam(params, "use_kg"); ok {
		defaults.UseKG = v
	}
	if v, ok := floatParam(params, "similarity_threshold"); ok {
		defaults.SimilarityThreshold = v
	}
	if v, ok := floatParam(params, "keywords_similarity_weight"); ok {
		if v < 0 || v > 1 {
			return nil, fmt.Errorf("agent tool: retrieval tool requires node-level param keywords_similarity_weight in [0,1]")
		}
		defaults.KeywordsSimilarityWeight = &v
	}
	return NewRetrievalToolWithDefaults(defaults), nil
}

func buildSearXNGTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := defaultSearXNGParams()
	if value, ok := params["top_n"]; ok {
		topN, valid := parseSearXNGTopN(value)
		if !valid || topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "searxng")
		}
		defaults.TopN = topN
	}
	if value, ok := params["searxng_url"]; ok {
		searxngURL, valid := value.(string)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires string node-level param searxng_url", "searxng")
		}
		defaults.SearXNGURL = searxngURL
	}
	return newSearXNGToolWithDefaults(nil, defaults), nil
}

func buildWencaiTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := wencaiParams{}
	if value, ok := params["top_n"]; ok {
		topN, valid := strictInt(value)
		if !valid || topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "wencai")
		}
		defaults.TopN = topN
	}
	if value, ok := params["query_type"]; ok {
		queryType, valid := value.(string)
		if !valid || !isWencaiQueryTypeSupported(queryType) {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported query_type %q", "wencai", queryType)
		}
		defaults.QueryType = queryType
	}
	return newWencaiTool(defaults), nil
}

func buildTavilyExtractTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := tavilyExtractParams{}
	if value, ok := params["api_key"]; ok {
		apiKey, valid := value.(string)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires string node-level param api_key", "tavily_extract")
		}
		defaults.APIKey = apiKey
	}
	if value, ok := params["urls"]; ok {
		defaults.URLs = value
	}
	if value, ok := params["extract_depth"]; ok {
		extractDepth, valid := value.(string)
		if !valid || (extractDepth != "basic" && extractDepth != "advanced") {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported extract_depth %q", "tavily_extract", extractDepth)
		}
		defaults.ExtractDepth = extractDepth
	}
	if value, ok := params["format"]; ok {
		format, valid := value.(string)
		if !valid || (format != "markdown" && format != "text") {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported format %q", "tavily_extract", format)
		}
		defaults.Format = format
	}
	return newTavilyExtractTool(nil, nil, defaults), nil
}

func buildTavilyTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := tavilyParams{}
	if value, ok := params["api_key"]; ok {
		apiKey, valid := value.(string)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires string node-level param api_key", "tavily")
		}
		defaults.APIKey = apiKey
	}
	if value, ok := params["search_depth"]; ok {
		searchDepth, valid := value.(string)
		if !valid || (searchDepth != "basic" && searchDepth != "advanced") {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported search_depth %q", "tavily", searchDepth)
		}
		defaults.SearchDepth = searchDepth
	}
	if value, ok := params["max_results"]; ok {
		maxResults, valid := strictInt(value)
		if !valid || maxResults <= 0 || maxResults > 20 {
			return nil, fmt.Errorf("agent tool: tool %q requires integer node-level param max_results within [1, 20]", "tavily")
		}
		defaults.MaxResults = maxResults
	}
	if value, ok := params["days"]; ok {
		days, valid := strictInt(value)
		if !valid || days <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param days", "tavily")
		}
		defaults.Days = days
	}
	for _, key := range []string{"include_answer", "include_raw_content", "include_images", "include_image_descriptions"} {
		value, ok := params[key]
		if !ok {
			continue
		}
		flag, valid := value.(bool)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param %s", "tavily", key)
		}
		switch key {
		case "include_answer":
			defaults.IncludeAnswer = flag
		case "include_raw_content":
			defaults.IncludeRawContent = flag
		case "include_images":
			defaults.IncludeImages = flag
		case "include_image_descriptions":
			defaults.IncludeImageDescriptions = flag
		}
	}
	return newTavilyTool(nil, nil, defaults), nil
}

func buildKeenableTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := keenableParams{}
	apiKey := ""
	if value, ok := params["api_key"]; ok {
		var valid bool
		apiKey, valid = value.(string)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires string node-level param api_key", "keenable")
		}
	}
	if value, ok := params["mode"]; ok {
		mode, valid := value.(string)
		if !valid || (mode != "pro" && mode != "realtime") {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported mode %q", "keenable", mode)
		}
		defaults.Mode = mode
	}
	if value, ok := params["top_n"]; ok {
		topN, valid := strictInt(value)
		if !valid || topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "keenable")
		}
		defaults.TopN = topN
	}
	if value, ok := params["site"]; ok {
		site, valid := value.(string)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires string node-level param site", "keenable")
		}
		defaults.Site = site
	}
	if defaults.Mode == "realtime" && strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("agent tool: tool %q requires api_key for realtime mode", "keenable")
	}
	return newKeenableTool(nil, nil, apiKey, defaults), nil
}

func buildWikipediaTool(params map[string]any) (einotool.BaseTool, error) {
	topN := defaultWikipediaTopN
	language := defaultWikipediaLanguage
	if v, ok := intParam(params, "top_n"); ok {
		topN = v
	}
	if topN <= 0 {
		return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "wikipedia")
	}
	if v, ok := stringParam(params, "language"); ok {
		language = strings.TrimSpace(v)
	}
	if language == "" {
		return nil, fmt.Errorf("agent tool: tool %q requires non-empty string node-level param language", "wikipedia")
	}
	if !WikipediaLanguageSupported(language) {
		return nil, fmt.Errorf("agent tool: tool %q unsupported node-level param language %q", "wikipedia", language)
	}
	return NewWikipediaToolWithParams(nil, topN, language), nil
}

func buildYahooFinanceTool(params map[string]any) (einotool.BaseTool, error) {
	defaults := defaultYahooFinanceParams()
	boolFields := map[string]*bool{
		"info":                &defaults.Info,
		"history":             &defaults.History,
		"count":               &defaults.Count,
		"financials":          &defaults.Financials,
		"income_stmt":         &defaults.IncomeStmt,
		"balance_sheet":       &defaults.BalanceSheet,
		"cash_flow_statement": &defaults.CashFlowStatement,
		"news":                &defaults.News,
	}
	for key, target := range boolFields {
		value, exists := params[key]
		if !exists {
			continue
		}
		flag, valid := value.(bool)
		if !valid {
			return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param %s", "yahoo_finance", key)
		}
		*target = flag
	}
	return NewYahooFinanceToolWithDefaults(nil, defaults), nil
}

func decodeExeSQLConnParams(params map[string]any) (exesqlConnParams, error) {
	if len(params) == 0 {
		return exesqlConnParams{}, fmt.Errorf(
			"agent tool: execute_sql requires node-level params " +
				"(db_type/host/port/database/username/password)",
		)
	}
	conn, err := NewExeSQLConnParams(params)
	if err != nil {
		return exesqlConnParams{}, fmt.Errorf("agent tool: execute_sql config: %w", err)
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

func floatParam(params map[string]any, key string) (float64, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	default:
		return 0, false
	}
}

func stringSliceParam(params map[string]any, key string) ([]string, bool, error) {
	v, ok := params[key]
	if !ok {
		return nil, false, nil
	}
	switch x := v.(type) {
	case []string:
		return append([]string(nil), x...), true, nil
	case []any:
		out := make([]string, 0, len(x))
		for _, item := range x {
			s, ok := item.(string)
			if !ok {
				return nil, true, fmt.Errorf("%s must be a string list", key)
			}
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out, true, nil
	default:
		return nil, true, fmt.Errorf("%s must be a string list", key)
	}
}

func strictInt(value any) (int, bool) {
	switch x := value.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		if int64(int(x)) != x {
			return 0, false
		}
		return int(x), true
	case float64:
		maxInt := int(^uint(0) >> 1)
		minInt := -maxInt - 1
		if math.Trunc(x) != x || x >= float64(maxInt) || x <= float64(minInt) {
			return 0, false
		}
		return int(x), true
	default:
		return 0, false
	}
}

func boolParam(params map[string]any, key string) (bool, bool) {
	v, ok := params[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}
