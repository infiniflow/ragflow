package tool

import (
	"fmt"
	"math"
	"strings"
)

// Factory builds a tool instance by DSL / Agent-visible name and
// optional node-level configuration. The config map belongs to the
// Agent node / DSL, not to the model-emitted function-call args.
type Factory func(params map[string]any) (Tool, error)

var registry = map[string]Factory{
	"akshare":               buildAkShareTool,
	"arxiv":                 buildArxivTool,
	"bgpt":                  noConfig("bgpt", func() Tool { return NewBGPTTool() }),
	"code_exec":             noConfig("code_exec", func() Tool { return NewCodeExecTool() }),
	"crawler":               noConfig("crawler", func() Tool { return NewCrawlerTool() }),
	"deepl":                 noConfig("deepl", func() Tool { return NewDeepLTool() }),
	"duckduckgo":            noConfig("duckduckgo", func() Tool { return NewDuckDuckGoTool() }),
	"email":                 noConfig("email", func() Tool { return NewEmailTool() }),
	"execute_sql":           buildExeSQLTool,
	"exesql":                buildExeSQLTool,
	"github":                buildGitHubTool,
	"google":                buildGoogleTool,
	"google_scholar":        buildGoogleScholarTool,
	"google_scholar_search": buildGoogleScholarTool,
	"jin10":                 noConfig("jin10", func() Tool { return NewJin10Tool() }),
	"keenable":              buildKeenableTool,
	"pubmed":                buildPubMedTool,
	"qweather":              noConfig("qweather", func() Tool { return NewQWeatherTool() }),
	"retrieval":             noConfig("retrieval", func() Tool { return NewRetrievalTool() }),
	"search_my_dataset":     noConfig("search_my_dataset", func() Tool { return NewRetrievalTool() }),
	"search_my_dateset":     noConfig("search_my_dateset", func() Tool { return NewRetrievalTool() }),
	"searxng":               buildSearXNGTool,
	"tavily":                noConfig("tavily", func() Tool { return NewTavilyTool() }),
	"tavily_extract":        noConfig("tavily_extract", func() Tool { return NewTavilyExtractTool() }),
	"tushare":               noConfig("tushare", func() Tool { return NewTushareTool() }),
	"wencai":                buildWencaiTool,
	"web_crawler":           noConfig("web_crawler", func() Tool { return NewCrawlerTool() }),
	"wikipedia":             buildWikipediaTool,
	"wikipedia_search":      buildWikipediaTool,
	"yahoo_finance":         noConfig("yahoo_finance", func() Tool { return NewYahooFinanceTool() }),
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
// perToolParams is keyed by the Agent-visible tool name.
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

func buildAkShareTool(params map[string]any) (Tool, error) {
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

func buildArxivTool(params map[string]any) (Tool, error) {
	topN := defaultArxivTopN
	sortBy := defaultArxivSortBy
	for key := range params {
		switch key {
		case "top_n", "sort_by":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "arxiv", key)
		}
	}
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

func buildExeSQLTool(params map[string]any) (Tool, error) {
	conn, err := decodeExeSQLConnParams(params)
	if err != nil {
		return nil, err
	}
	return NewExeSQLTool(conn), nil
}

func buildGoogleTool(params map[string]any) (Tool, error) {
	if len(params) == 0 {
		return NewGoogleTool(), nil
	}
	for key := range params {
		switch key {
		case "api_key", "country", "language", "q", "start", "num":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "google", key)
		}
	}
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

func buildGitHubTool(params map[string]any) (Tool, error) {
	topN := defaultGitHubTopN
	for key := range params {
		if key != "top_n" {
			return nil, fmt.Errorf("agent tool: tool %q only accepts node-level param top_n", "github")
		}
	}
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

func buildGoogleScholarTool(params map[string]any) (Tool, error) {
	if len(params) == 0 {
		return NewGoogleScholarTool(), nil
	}
	for key := range params {
		switch key {
		case "query", "top_n", "sort_by", "year_low", "year_high", "patents":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "google_scholar", key)
		}
	}
	defaults := googleScholarParams{}
	if v, ok := stringParam(params, "query"); ok {
		defaults.Query = v
	}
	if v, ok := intParam(params, "top_n"); ok {
		defaults.TopN = v
	}
	if v, ok := stringParam(params, "sort_by"); ok {
		defaults.SortBy = v
	}
	if v, ok := intParam(params, "year_low"); ok {
		defaults.YearLow = v
	}
	if v, ok := intParam(params, "year_high"); ok {
		defaults.YearHigh = v
	}
	if v, ok := boolParam(params, "patents"); ok {
		defaults.Patents = &v
	}
	return NewGoogleScholarToolWithDefaults(nil, defaults), nil
}

func buildPubMedTool(params map[string]any) (Tool, error) {
	defaults := pubmedParams{}
	for key := range params {
		switch key {
		case "top_n", "email":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "pubmed", key)
		}
	}
	if topN, ok := intParam(params, "top_n"); ok {
		if topN <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "pubmed")
		}
		defaults.TopN = topN
	}
	if email, ok := stringParam(params, "email"); ok {
		if strings.TrimSpace(email) == "" {
			return nil, fmt.Errorf("agent tool: tool %q requires non-empty string node-level param email", "pubmed")
		}
		defaults.Email = email
	}
	return NewPubMedToolWithDefaults(nil, defaults), nil
}

func buildSearXNGTool(params map[string]any) (Tool, error) {
	defaults := defaultSearXNGParams()
	for key := range params {
		switch key {
		case "top_n", "searxng_url":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "searxng", key)
		}
	}
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

func buildWencaiTool(params map[string]any) (Tool, error) {
	defaults := wencaiParams{}
	for key := range params {
		switch key {
		case "top_n", "query_type":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "wencai", key)
		}
	}
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

func buildKeenableTool(params map[string]any) (Tool, error) {
	if len(params) == 0 {
		return NewKeenableTool(), nil
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
	return NewKeenableToolWithAPIKey(nil, apiKey), nil
}

func buildWikipediaTool(params map[string]any) (Tool, error) {
	topN := defaultWikipediaTopN
	language := defaultWikipediaLanguage
	for key := range params {
		if key != "top_n" && key != "language" {
			return nil, fmt.Errorf("agent tool: tool %q only accepts node-level params top_n/language", "wikipedia")
		}
	}
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
