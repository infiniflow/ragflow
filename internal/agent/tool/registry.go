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
	"bgpt":                  buildBGPTTool,
	"code_exec":             noConfig("code_exec", func() Tool { return NewCodeExecTool() }),
	"crawler":               noConfig("crawler", func() Tool { return NewCrawlerTool() }),
	"deepl":                 noConfig("deepl", func() Tool { return NewDeepLTool() }),
	"duckduckgo":            buildDuckDuckGoTool,
	"email":                 buildEmailTool,
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
	"retrieval":             buildRetrievalTool,
	"search_my_dataset":     buildRetrievalTool,
	"search_my_dateset":     buildRetrievalTool,
	"searxng":               buildSearXNGTool,
	"tavily":                buildTavilyTool,
	"tavily_extract":        buildTavilyExtractTool,
	"tushare":               noConfig("tushare", func() Tool { return NewTushareTool() }),
	"wencai":                buildWencaiTool,
	"web_crawler":           noConfig("web_crawler", func() Tool { return NewCrawlerTool() }),
	"wikipedia":             buildWikipediaTool,
	"wikipedia_search":      buildWikipediaTool,
	"yahoo_finance":         buildYahooFinanceTool,
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
		value, ok := strictInt(raw)
		if !ok {
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
		case "query", "top_n", "sort_by", "year_low", "year_high", "patents", "outputs", "inputs":
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

func buildEmailTool(params map[string]any) (Tool, error) {
	defaults := emailParams{SMTPPort: 465}
	for key := range params {
		switch key {
		case "smtp_server", "smtp_port", "email", "sender_name", "password", "outputs", "inputs", "setups":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "email", key)
		}
	}
	switch v := params["smtp_server"].(type) {
	case string:
		defaults.SMTPServer = v
	case nil:
		// optional
	default:
		return nil, fmt.Errorf("agent tool: tool %q requires string node-level param smtp_server", "email")
	}
	if value, exists := params["smtp_port"]; exists {
		var v int
		switch x := value.(type) {
		case int:
			v = x
		case float64:
			if math.Trunc(x) != x {
				return nil, fmt.Errorf("agent tool: tool %q requires valid integer node-level param smtp_port (1-65535)", "email")
			}
			v = int(x)
		case string:
			if _, err := fmt.Sscanf(x, "%d", &v); err != nil {
				return nil, fmt.Errorf("agent tool: tool %q requires valid integer node-level param smtp_port (1-65535)", "email")
			}
		default:
			return nil, fmt.Errorf("agent tool: tool %q requires valid integer node-level param smtp_port (1-65535)", "email")
		}
		if v <= 0 || v > 65535 {
			return nil, fmt.Errorf("agent tool: tool %q requires valid integer node-level param smtp_port (1-65535)", "email")
		}
		defaults.SMTPPort = v
	}
	if v, ok := stringParam(params, "email"); ok {
		defaults.Email = v
	}
	if v, ok := stringParam(params, "sender_name"); ok {
		defaults.SenderName = v
	}
	if v, ok := stringParam(params, "password"); ok {
		defaults.Password = v
	}
	return newEmailTool(defaults), nil
}

func buildRetrievalTool(params map[string]any) (Tool, error) {
	defaults := retrievalArgs{}
	for key := range params {
		switch key {
		case "kb_ids", "dataset_ids", "top_n", "top_k", "keywords_similarity_weight", "similarity_threshold",
			"rerank_id", "toc_enhance", "meta_data_filter", "empty_response",
			"retrieval_from", "memory_ids", "kb_vars", "cross_languages",
			"inputs", "outputs":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "retrieval", key)
		}
	}
	if v, ok, err := stringSliceParam(params, "kb_ids"); err != nil {
		return nil, fmt.Errorf("agent tool: tool %q: %w", "retrieval", err)
	} else if ok {
		defaults.KBIDs = v
	}
	if v, ok := intParam(params, "top_n"); ok {
		defaults.TopN = v
	}
	if v, ok := intParam(params, "top_k"); ok {
		defaults.TopK = v
	}
	if v, ok := floatParam(params, "keywords_similarity_weight"); ok {
		defaults.KeywordsSimilarityWeight = &v
	}
	if v, ok := floatParam(params, "similarity_threshold"); ok {
		defaults.SimilarityThreshold = v
	}
	return NewRetrievalToolWithDefaults(defaults), nil
}

func buildBGPTTool(params map[string]any) (Tool, error) {
	defaults := bgptParams{}
	for key := range params {
		switch key {
		case "api_key", "top_n", "days_back", "outputs", "inputs":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "bgpt", key)
		}
	}
	switch v := params["api_key"].(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, fmt.Errorf("agent tool: tool %q requires non-empty string node-level param api_key", "bgpt")
		}
		defaults.APIKey = v
	case nil:
		// optional
	default:
		return nil, fmt.Errorf("agent tool: tool %q requires string node-level param api_key", "bgpt")
	}
	if value, exists := params["top_n"]; exists {
		var v int
		switch x := value.(type) {
		case int:
			v = x
		case float64:
			if math.Trunc(x) != x {
				return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "bgpt")
			}
			v = int(x)
		case string:
			if _, err := fmt.Sscanf(x, "%d", &v); err != nil {
				return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "bgpt")
			}
		default:
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "bgpt")
		}
		if v <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "bgpt")
		}
		defaults.TopN = v
	}
	if value, exists := params["days_back"]; exists {
		v, ok := strictInt(value)
		if !ok || v <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param days_back", "bgpt")
		}
		defaults.DaysBack = v
	}
	return newBGPTTool(nil, defaults), nil
}

func buildTavilyTool(params map[string]any) (Tool, error) {
	defaults := tavilyParams{}
	for key := range params {
		switch key {
		case "api_key", "query", "search_depth", "max_results", "days",
			"include_answer", "include_raw_content", "include_images", "include_image_descriptions",
			"outputs", "inputs":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "tavily", key)
		}
	}
	if v, ok := stringParam(params, "api_key"); ok {
		defaults.APIKey = v
	} else if _, exists := params["api_key"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires string node-level param api_key", "tavily")
	}
	if v, ok := stringParam(params, "search_depth"); ok {
		if v != "basic" && v != "advanced" {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported search_depth %q", "tavily", v)
		}
		defaults.SearchDepth = v
	}
	if value, exists := params["max_results"]; exists {
		v, ok := strictInt(value)
		if !ok || v <= 0 || v > 20 {
			return nil, fmt.Errorf("agent tool: tool %q requires valid integer node-level param max_results (1-20)", "tavily")
		}
		defaults.MaxResults = v
	}
	if value, exists := params["days"]; exists {
		v, ok := strictInt(value)
		if !ok || v <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param days", "tavily")
		}
		defaults.Days = v
	}
	if v, ok := boolParam(params, "include_answer"); ok {
		defaults.IncludeAnswer = v
	} else if _, exists := params["include_answer"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param include_answer", "tavily")
	}
	if v, ok := boolParam(params, "include_raw_content"); ok {
		defaults.IncludeRawContent = v
	} else if _, exists := params["include_raw_content"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param include_raw_content", "tavily")
	}
	if v, ok := boolParam(params, "include_images"); ok {
		defaults.IncludeImages = v
	} else if _, exists := params["include_images"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param include_images", "tavily")
	}
	if v, ok := boolParam(params, "include_image_descriptions"); ok {
		defaults.IncludeImageDescriptions = v
	} else if _, exists := params["include_image_descriptions"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param include_image_descriptions", "tavily")
	}
	return newTavilyTool(nil, nil, defaults), nil
}

func buildDuckDuckGoTool(params map[string]any) (Tool, error) {
	defaults := duckduckgoParams{}
	for key := range params {
		switch key {
		case "top_n", "channel", "outputs", "inputs":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "duckduckgo", key)
		}
	}
	if value, exists := params["top_n"]; exists {
		v, ok := strictInt(value)
		if !ok || v <= 0 {
			return nil, fmt.Errorf("agent tool: tool %q requires positive integer node-level param top_n", "duckduckgo")
		}
		defaults.TopN = v
	}
	if v, ok := stringParam(params, "channel"); ok {
		if v != duckduckgoChannelGeneral && v != duckduckgoChannelNews {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported channel %q", "duckduckgo", v)
		}
		defaults.Channel = v
	} else if _, exists := params["channel"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires string node-level param channel", "duckduckgo")
	}
	return newDuckDuckGoTool(nil, defaults), nil
}

func buildYahooFinanceTool(params map[string]any) (Tool, error) {
	defaults := defaultYahooFinanceParams()
	for key := range params {
		switch key {
		case "stock_code", "info", "history", "balance_sheet", "news", "outputs", "inputs":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "yahoo_finance", key)
		}
	}
	if v, ok := boolParam(params, "info"); ok {
		defaults.Info = v
	} else if _, exists := params["info"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires boolean node-level param info", "yahoo_finance")
	}
	return NewYahooFinanceToolWithDefaults(nil, defaults), nil
}

func buildTavilyExtractTool(params map[string]any) (Tool, error) {
	defaults := tavilyExtractParams{}
	for key := range params {
		switch key {
		case "api_key", "urls", "extract_depth", "format", "outputs", "inputs":
		default:
			return nil, fmt.Errorf("agent tool: tool %q does not accept node-level param %s", "tavily_extract", key)
		}
	}
	if v, ok := stringParam(params, "api_key"); ok {
		defaults.APIKey = v
	} else if _, exists := params["api_key"]; exists {
		return nil, fmt.Errorf("agent tool: tool %q requires string node-level param api_key", "tavily_extract")
	}
	if v, ok := stringParam(params, "urls"); ok {
		defaults.URLs = v
	}
	if v, ok := stringParam(params, "extract_depth"); ok {
		if v != "basic" && v != "advanced" {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported extract_depth %q", "tavily_extract", v)
		}
		defaults.ExtractDepth = v
	}
	if v, ok := stringParam(params, "format"); ok {
		if v != "markdown" && v != "text" {
			return nil, fmt.Errorf("agent tool: tool %q has unsupported format %q", "tavily_extract", v)
		}
		defaults.Format = v
	}
	return newTavilyExtractTool(nil, nil, defaults), nil
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
