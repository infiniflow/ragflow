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

func buildPubMedTool(params map[string]any) (Tool, error) {
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

func buildWencaiTool(params map[string]any) (Tool, error) {
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

func buildKeenableTool(params map[string]any) (Tool, error) {
	if len(params) == 0 {
		return NewKeenableTool(), nil
	}
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

func buildSearXNGTool(params map[string]any) (Tool, error) {
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

func buildWikipediaTool(params map[string]any) (Tool, error) {
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
