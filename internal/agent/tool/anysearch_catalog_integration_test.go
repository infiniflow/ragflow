//go:build integration

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

type anySearchCatalogScenario struct {
	Name, Tag, Query string
	Params           map[string]any
}

// Adjacent complete fields bind both identity parts to one participant, even
// when the remaining match JSON is explicitly truncated by the tool.
var anySearchCatalogRiotIdentity = regexp.MustCompile(`(?:\{|,)\s*"riotIdGameName"\s*:\s*"Hide on bush"\s*,\s*"riotIdTagline"\s*:\s*"KR1"\s*[,}]`)

func TestAnySearchLiveRegionLanguage(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH=1 to call the real AnySearch API")
	}
	key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	for _, tc := range []struct{ zone, language string }{{"intl", "en"}, {"cn", "zh-CN"}} {
		t.Run(tc.zone, func(t *testing.T) {
			search, _, transport := liveAnySearchTools(key)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			args := map[string]any{"query": "RAGFlow official documentation", "max_results": 2, "zone": tc.zone, "language": tc.language}
			out, err := search.InvokableRun(ctx, mustJSON(t, args))
			if err != nil {
				t.Fatal(err)
			}
			env := decodeAnySearchEnvelope(t, out)
			requests := transport.take()
			if len(requests) != 1 || requests[0].Status != http.StatusOK || mustJSON(t, requests[0].Body) != mustJSON(t, args) {
				t.Fatal("region/language did not reach a successful real request unchanged")
			}
			if env.Error != "" || len(env.Queries) != 1 || env.Queries[0].Error != "" || len(env.Queries[0].Results) == 0 || len(env.Queries[0].Results) > 2 || env.Queries[0].RequestID != requests[0].RequestID {
				t.Fatal("regional search failed or lost its results/request ID")
			}
			t.Logf("zone=%s language=%s requests=1 HTTP=200 request_id=%s results=%d", tc.zone, tc.language, requests[0].RequestID, len(env.Queries[0].Results))
		})
	}
}

// TestAnySearchCatalogLive exercises actual tool calls, not the controlled
// catalog fixtures. Empty results and vendor failures fail the scenario. A
// passing request establishes delivery; source/filter semantics still require
// inspecting the saved results against the particular catalog description.
// RAGFLOW_ANYSEARCH_CATALOG_RECEIPTS optionally saves those results privately.
func TestAnySearchCatalogLive(t *testing.T) {
	if os.Getenv("RAGFLOW_TEST_ANYSEARCH_CATALOG") != "1" {
		t.Skip("set RAGFLOW_TEST_ANYSEARCH_CATALOG=1 to run the real catalog scenarios")
	}
	key := strings.TrimSpace(os.Getenv("ANYSEARCH_TEST_API_KEY"))
	cases := anySearchCatalogScenarios(time.Now().UTC())
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			search, _, transport := liveAnySearchTools(key)
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			args := map[string]any{"query": tc.Query, "tag": tc.Tag, "max_results": 3}
			if len(tc.Params) > 0 {
				args["params"] = tc.Params
			}
			started := time.Now().UTC()
			out, err := search.InvokableRun(ctx, mustJSON(t, args))
			if err != nil {
				t.Fatal("tool returned a Go error")
			}
			if key != "" && strings.Contains(out, key) {
				t.Fatal("tool result exposed the configured credential")
			}
			env := decodeAnySearchEnvelope(t, out)
			requests := transport.take()
			if dir := os.Getenv("RAGFLOW_ANYSEARCH_CATALOG_RECEIPTS"); dir != "" {
				// This directory is an explicit local evidence destination. It must
				// stay outside committed fixtures; upstream replies are not goldens.
				receipt := map[string]any{"started_utc": started.Format(time.RFC3339Nano), "scenario": tc.Name, "request": args, "http": requests, "delivered": json.RawMessage(out), "scope_verified": false}
				data, marshalErr := json.MarshalIndent(receipt, "", "  ")
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				if key != "" && strings.Contains(string(data), key) {
					t.Fatal("receipt would expose the configured credential")
				}
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.Name+".json"), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if len(requests) != 1 {
				t.Fatalf("requests=%d, want exactly one POST", len(requests))
			}
			r := requests[0]
			if r.Method != http.MethodPost || r.Path != "/v1/search" || r.HasAuthorization != (key != "") || (key != "" && !r.BearerMatches) {
				t.Fatal("request destination, method or authentication changed")
			}
			if mustJSON(t, r.Body) != mustJSON(t, args) {
				t.Fatalf("wire request changed: got=%s want=%s", mustJSON(t, r.Body), mustJSON(t, args))
			}
			if r.Status != http.StatusOK || env.Error != "" || len(env.Queries) != 1 {
				t.Fatalf("HTTP=%d tool_error=%q query_records=%d request_id=%s", r.Status, env.Error, len(env.Queries), r.RequestID)
			}
			q := env.Queries[0]
			if q.Error != "" || q.Index != 0 || q.Query != tc.Query || q.Tag != tc.Tag || len(q.Results) == 0 || len(q.Results) > 3 {
				t.Fatalf("query_error=%q results=%d query_association=%v", q.Error, len(q.Results), q.Index == 0 && q.Query == tc.Query && q.Tag == tc.Tag)
			}
			if r.RequestID == "" || q.RequestID != r.RequestID {
				t.Fatal("delivered diagnostics did not preserve the real server request ID")
			}
			if tc.Tag == "code.snippet" && tc.Params["repo"] == "facebook/react" {
				for _, result := range q.Results {
					u, err := url.Parse(anySearchText(result["url"]))
					if err != nil || !strings.EqualFold(u.Hostname(), "github.com") || !strings.HasPrefix(strings.ToLower(u.Path), "/facebook/react/") {
						t.Fatal("repository-filtered snippet did not identify the requested GitHub repository")
					}
				}
			}
			if tc.Tag == "gaming.esports" && tc.Params["game_name"] == "Hide on bush#KR1" {
				identified := false
				for _, result := range q.Results {
					text := anySearchText(result["title"]) + " " + anySearchText(result["content"]) + " " + anySearchText(result["url"])
					decoded, _ := url.QueryUnescape(text)
					decoded = strings.ToLower(decoded)
					identified = identified || strings.Contains(decoded, "hide on bush#kr1") || strings.Contains(decoded, "hide on bush-kr1") || anySearchCatalogRiotIdentity.MatchString(anySearchText(result["content"]))
				}
				if !identified {
					t.Fatal("player operation did not identify the requested Riot account")
				}
			}
			// The tool only returns these records after validating business code
			// zero and a structured results array. The catalog specifies no
			// universal provenance field, so do not manufacture one here.
			t.Logf("tag=%s requests=1 HTTP=%d business_code=0 request_id=%s results=%d source_scope=unverified", tc.Tag, r.Status, r.RequestID, len(q.Results))
		})
	}
}

func anySearchCatalogScenarios(now time.Time) []anySearchCatalogScenario {
	p := func(pairs ...string) map[string]any {
		m := make(map[string]any, len(pairs)/2)
		for i := 0; i < len(pairs); i += 2 {
			m[pairs[i]] = pairs[i+1]
		}
		return m
	}
	cases := []anySearchCatalogScenario{
		{"academic-citation", "academic.citation", "10.1038/s41586-021-03819-2", p("id", "10.1038/s41586-021-03819-2")},
		{"academic-dataset", "academic.dataset", "climate", nil},
		{"academic-paper", "academic.paper", "transformer attention mechanism", nil},
		{"academic-person", "academic.person", "Geoffrey Hinton", nil},
		{"agriculture-fao", "agriculture.fao", "wheat production", p("type", "GlobalAgriculture", "domain", "production", "keyword", "wheat production", "location", "World", "date_start", "2023")},
		{"business-company", "business.company", "Apple Inc.", p("type", "USFilings", "keyword", "Apple Inc.")},
		{"business-jobs", "business.jobs", "software engineer", p("type", "GeneralJobs", "keyword", "software engineer", "location", "us")},
		{"business-people", "business.people", "stripe.com", p("type", "EmailLookup", "keyword", "stripe.com")},
		{"business-trade", "business.trade", "China tea exports", p("type", "GlobalBilateralTrade", "hs_code", "0902", "location", "China", "date_start", "2023", "flow", "export")},
		{"code-doc", "code.doc", "useEffect cleanup", p("library", "react")},
		{"code-snippet", "code.snippet", "useState", p("repo", "facebook/react", "lang", "TypeScript")},
		{"energy-electricity", "energy.electricity", "France electricity generation", p("type", "EUElectricity", "location", "France", "metric", "generation", "date_start", "2025")},
		{"energy-production", "energy.production", "crude oil production", p("type", "USEnergyData", "keyword", "crude oil production", "date_start", "2020", "frequency", "annual")},
		{"environment-aqi", "environment.aqi", "Beijing air quality", p("type", "GlobalAirQuality", "location", "Beijing")},
		{"film-torrent", "film.torrent", "Night of the Living Dead 1968", nil},
		{"finance-calendar", "finance.calendar", "AAPL earnings", p("type", "earnings", "symbol", "AAPL", "period", "90d")},
		{"finance-fundamental", "finance.fundamental", "AAPL fundamentals", p("type", "overview", "symbol", "AAPL", "cn_code", "", "period", "annual")},
		{"finance-macro", "finance.macro", "GDP", p("type", "gdp")},
		{"finance-news", "finance.news", "financial headlines", p("type", "flash", "news_src", "sina")},
		{"finance-quote", "finance.quote", "AAPL", p("type", "stock", "symbol", "AAPL")},
		{"finance-screen", "finance.screen", "technology stocks", p("type", "stock", "sector", "Technology", "country", "US")},
		{"gaming-esports", "gaming.esports", "Hide on bush#KR1", p("type", "player", "game_name", "Hide on bush#KR1", "region", "kr")},
		{"gaming-store", "gaming.store", "Counter-Strike 2", nil},
		{"general-general", "general.general", "RAGFlow official documentation", nil},
		{"health-drug", "health.drug", "aspirin", p("type", "name")},
		{"health-stats", "health.stats", "life expectancy France", nil},
		{"health-trial", "health.trial", "NCT04280705", nil},
		{"ip-global", "ip.global", "solar cell", p("type", "GlobalPatent", "keyword", "solar cell")},
		{"legal-enforcement", "legal.enforcement", "Google data protection penalty", p("jurisdiction", "FR")},
		{"legal-judicial", "legal.judicial", "Brown v. Board of Education", p("jurisdiction", "US")},
		{"legal-legislation", "legal.legislation", "Clean Air Act", p("jurisdiction", "US")},
		{"resource-image", "resource.image", "mountain landscape", nil},
		{"security-intel", "security.intel", "8.8.8.8", p("ioc", "8.8.8.8")},
		{"security-noise", "security.noise", "8.8.8.8", p("ip", "8.8.8.8")},
		{"security-scan", "security.scan", "8.8.8.8", p("ioc", "8.8.8.8")},
		{"security-vuln", "security.vuln", "CVE-2021-44228", p("type", "cve", "value", "CVE-2021-44228")},
		{"social-media", "social_media.social_media", "AI large models", p("type", "weibo", "keyword", "AI large models")},
		{"travel-flight", "travel.flight", "PEK to LAX flights", p("arrival", "LAX", "departure", "PEK", "date", now.AddDate(0, 0, 30).Format("2006-01-02"))},
		{"travel-flight-status", "travel.flight_status", "CA981 flight status", p("flight_number", "CA981", "departure", "")},
	}
	for i := range cases {
		cases[i].Name = "baseline-" + cases[i].Name
	}
	add := func(name, tag, query string, params map[string]any) {
		cases = append(cases, anySearchCatalogScenario{name, tag, query, params})
	}
	// These are named catalog values, not an inferred inventory of upstream
	// providers. Provider routing and ignored filters remain vendor contracts.
	selectors := []struct {
		tag, field, query string
		values            []string
	}{
		{"academic.dataset", "client_id", "climate", []string{"cern.zenodo", "figshare.ars", "bl.dryad"}},
		{"academic.paper", "source", "cancer", []string{"MED", "PMC", "PPR", "AGR", "PAT"}},
		{"academic.paper", "data_source", "transformer", []string{"arxiv", "openalex"}},
		{"finance.news", "news_src", "financial headlines", []string{"sina", "wallstreetcn", "10jqka", "eastmoney", "cls", "yicai", "yuncaijing", "fenghuang", "jinrongjie"}},
		{"social_media.social_media", "type", "AI", []string{"weibo", "weibo_hot", "zhihu", "zhihu_hot", "x_top", "x_latest", "x_media", "x_people", "x_lists", "reddit_post", "reddit_community", "reddit_comment", "reddit_media", "reddit_people", "linkedin_people", "linkedin_jobs", "linkedin_company", "linkedin_posts", "wechatmp"}},
		{"business.company", "type", "Apple Inc.", []string{"GlobalLEI", "USFilings"}},
		{"business.jobs", "type", "software engineer", []string{"GeneralJobs", "RemoteJobs", "USFederalJobs"}},
		{"business.people", "type", "stripe.com", []string{"PeopleSearch", "EmailLookup"}},
		{"business.trade", "type", "tea imports", []string{"GlobalBilateralTrade", "USImportExport"}},
		{"energy.electricity", "type", "electricity generation", []string{"EUElectricity", "AustralianElectricity", "USEnergyData", "GlobalElectricity"}},
	}
	for _, selector := range selectors {
		for _, value := range selector.values {
			params := p(selector.field, value)
			if selector.tag == "finance.news" {
				params["type"] = "flash"
			}
			if selector.tag == "business.trade" {
				params["hs_code"], params["date_start"] = "0902", "2023"
			}
			if selector.tag == "energy.electricity" {
				params["metric"] = "generation"
				params["location"] = map[string]string{"EUElectricity": "France", "AustralianElectricity": "New South Wales", "USEnergyData": "California", "GlobalElectricity": "China"}[value]
			}
			query := selector.query
			if value == "PeopleSearch" {
				query = "software engineer"
			}
			add(strings.ReplaceAll(selector.tag, ".", "-")+"-"+selector.field+"-"+value, selector.tag, query, params)
		}
	}
	add("company-ChineseEnterprise", "business.company", "Alibaba", p("type", "ChineseEnterprise", "keyword", "Alibaba"))
	for _, kind := range []string{"income", "balance", "cashflow", "indicator", "holder"} {
		params := p("type", kind, "symbol", "", "cn_code", "600519.SH")
		if kind == "holder" {
			params["period"] = "1y"
		}
		add("fundamental-"+kind, "finance.fundamental", "600519.SH", params)
	}
	add("fundamental-quarter", "finance.fundamental", "AAPL fundamentals", p("type", "overview", "symbol", "AAPL", "cn_code", "", "period", "quarter"))
	for _, kind := range []string{"earnings", "dividends", "ipos", "economic"} {
		params := p("type", kind)
		if kind == "earnings" {
			params["cn_code"], params["period"] = "600519.SH", "90d"
		}
		add("calendar-"+kind, "finance.calendar", "financial calendar", params)
	}
	for _, kind := range []string{"general", "stock", "announcement"} {
		params := p("type", kind)
		if kind == "stock" {
			params["symbol"] = "AAPL"
		}
		if kind == "announcement" {
			params["cn_code"] = "600519.SH"
		}
		add("news-"+kind, "finance.news", "financial news", params)
	}
	for _, kind := range []string{"cpi", "fed_funds", "treasury", "unemployment", "nonfarm", "shibor", "lpr", "money_supply", "social_finance"} {
		add("macro-"+kind, "finance.macro", kind, p("type", kind))
	}
	add("screen-etf", "finance.screen", "SPDR", p("type", "etf"))
	for _, asset := range []struct{ kind, field, value string }{{"stock", "cn_code", "600519.SH"}, {"forex", "symbol", "EURUSD"}, {"crypto", "symbol", "BTCUSD"}, {"commodity", "symbol", "CLUSD"}, {"index", "cn_code", "399300.SZ"}, {"etf", "cn_code", "510300.SH"}} {
		add("quote-"+asset.kind+"-"+asset.field, "finance.quote", asset.value, p("type", asset.kind, asset.field, asset.value))
	}
	for _, kind := range []string{"ranked", "mastery", "live_game", "leaderboard", "champion", "item", "champion_rotation"} {
		params := p("type", kind)
		query := "Ahri"
		switch kind {
		case "ranked", "mastery", "live_game":
			query = "Hide on bush#KR1"
			params["game_name"], params["region"] = query, "kr"
		case "leaderboard":
			params["region"] = "kr"
		case "item":
			query = "Infinity Edge"
		}
		add("esports-"+kind, "gaming.esports", query, params)
	}
	add("esports-match-detail", "gaming.esports", "KR_7482819248", p("type", "match_detail", "match_id", "KR_7482819248"))
	add("esports-leaderboard-division", "gaming.esports", "Korea ranked leaderboard", p("type", "leaderboard", "region", "kr", "tier", "DIAMOND", "division", "II"))
	for _, region := range []string{"kr", "na", "euw", "eune", "jp", "br", "la1", "la2", "oc", "tr", "ru", "sg", "sea"} {
		add("esports-region-"+region, "gaming.esports", "ranked leaderboard", p("type", "leaderboard", "region", region))
	}
	for _, item := range []struct{ kind, value string }{{"commit", "6879efc2c1596d11a6a6ad296f80063b558d5e0f"}, {"package", "PyPI:jinja2@2.4.1"}} {
		add("vuln-"+item.kind, "security.vuln", item.value, p("type", item.kind, "value", item.value))
	}
	// Each name/version below is grounded in its registry or a published
	// vulnerability record. The catalog supplies AnySearch's wire grammar;
	// the underlying databases do not establish AnySearch routing behavior.
	for _, item := range []struct{ ecosystem, name, version string }{
		{"npm", "elliptic", "6.5.3"},
		{"PyPI", "jinja2", "2.4.1"},
		{"Maven", "org.apache.logging.log4j:log4j-core", "2.14.1"},
		{"NuGet", "System.Text.Encodings.Web", "4.7.1"},
		{"RubyGems", "actionpack", "5.2.4.2"},
		{"crates.io", "iana-time-zone", "0.1.43"},
		{"Go", "golang.org/x/text", "v0.3.5"},
		{"Packagist", "symfony/http-foundation", "4.4.6"},
		{"Hex", "plug", "1.3.0"},
		{"Pub", "archive", "3.3.7"},
		{"CRAN", "readxl", "1.4.1"},
	} {
		value := item.ecosystem + ":" + item.name + "@" + item.version
		add("vuln-ecosystem-"+item.ecosystem, "security.vuln", value, p("type", "package", "value", value))
	}
	// Public FDA label/recall records establish these identifiers. The UPC
	// record is historical; its current vendor availability is tested here.
	add("drug-ndc", "health.drug", "50580-590-01", p("type", "ndc"))
	add("drug-upc", "health.drug", "311917134925", p("type", "upc"))
	// OpenCitations' published examples supply compatible operation/ID pairs.
	// AnySearch expects bare IDs and id_type, unlike OpenCitations' prefixes.
	for _, item := range []struct{ operation, idType, id string }{
		{"metadata", "doi", "10.1007/978-1-4020-9632-7"},
		{"author", "orcid", "0000-0002-8420-0696"},
		{"editor", "orcid", "0000-0003-2098-4759"},
		{"venue-citation-count", "issn", "0138-9130"},
		{"citation-count", "doi", "10.1108/jd-12-2013-0166"},
		{"reference-count", "doi", "10.7717/peerj-cs.421"},
		{"citations", "doi", "10.1108/jd-12-2013-0166"},
		{"references", "doi", "10.7717/peerj-cs.421"},
	} {
		add("citation-op-"+item.operation, "academic.citation", item.id, p("op", item.operation, "id_type", item.idType, "id", item.id))
	}
	add("citation-op-citation", "academic.citation", "06101801781-06180334099", p("op", "citation", "id", "06101801781-06180334099"))
	add("citation-pmid", "academic.citation", "33817056", p("op", "metadata", "id_type", "pmid", "id", "33817056"))
	add("citation-isbn", "academic.citation", "9781402096327", p("op", "metadata", "id_type", "isbn", "id", "9781402096327"))
	add("citation-omid", "academic.citation", "br/06101801781", p("op", "metadata", "id_type", "omid", "id", "br/06101801781"))
	add("legal-canlii-case", "legal.judicial", "Dunsmuir v. New Brunswick", p("jurisdiction", "CA", "database_id", "csc-scc", "case_id", "2008scc9"))
	add("legal-cfr-title", "legal.legislation", "National Primary and Secondary Ambient Air Quality Standards", p("jurisdiction", "US", "collection", "CFR", "title", "40"))
	add("flight-status-airport", "travel.flight_status", "CA981 flight status", p("flight_number", "CA981", "departure", "PEK"))
	flight := p("arrival", "LAX", "departure", "PEK", "date", now.AddDate(0, 0, 30).Format("2006-01-02"), "return_date", now.AddDate(0, 0, 37).Format("2006-01-02"))
	add("flight-return", "travel.flight", "PEK to LAX return flights", flight)
	// Prevent accidental receipt overwrites and loss of a tag during edits.
	names, tags := map[string]bool{}, map[string]bool{}
	for _, tc := range cases {
		if names[tc.Name] {
			panic(fmt.Sprintf("duplicate AnySearch scenario %q", tc.Name))
		}
		names[tc.Name], tags[tc.Tag] = true, true
	}
	if len(tags) != 39 {
		panic(fmt.Sprintf("AnySearch scenarios contain %d tags, want 39", len(tags)))
	}
	return cases
}
