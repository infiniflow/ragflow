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
	"sort"
	"strings"
)

const wikipediaToolName = "wikipedia_search"

const wikipediaToolDescription = "A wide range of how-to and information pages are made available in wikipedia. Since 2001, it has grown rapidly to become the world's largest reference website. From Wikipedia, the free encyclopedia."

const wikipediaUserAgent = "Mozilla/5.0 (compatible; ragflow/1.0; +https://github.com/infiniflow/ragflow)"

const (
	defaultWikipediaTopN     = 10
	defaultWikipediaLanguage = "en"
)

var wikipediaLanguages = map[string]struct{}{
	"af": {}, "pl": {}, "ar": {}, "ast": {}, "az": {}, "bg": {}, "nan": {}, "bn": {}, "be": {}, "ca": {},
	"cs": {}, "cy": {}, "da": {}, "de": {}, "et": {}, "el": {}, "en": {}, "es": {}, "eo": {}, "eu": {},
	"fa": {}, "fr": {}, "gl": {}, "ko": {}, "hy": {}, "hi": {}, "hr": {}, "id": {}, "it": {}, "he": {},
	"ka": {}, "lld": {}, "la": {}, "lv": {}, "lt": {}, "hu": {}, "mk": {}, "arz": {}, "ms": {}, "min": {},
	"my": {}, "nl": {}, "ja": {}, "nb": {}, "nn": {}, "ce": {}, "uz": {}, "pt": {}, "kk": {}, "ro": {},
	"ru": {}, "ceb": {}, "sk": {}, "sl": {}, "sr": {}, "sh": {}, "fi": {}, "sv": {}, "ta": {}, "tt": {},
	"th": {}, "tg": {}, "azb": {}, "tr": {}, "uk": {}, "ur": {}, "vi": {}, "war": {}, "zh": {}, "yue": {},
}

// WikipediaLanguageSupported mirrors Python WikipediaParam.check's accepted
// language list.
func WikipediaLanguageSupported(language string) bool {
	_, ok := wikipediaLanguages[strings.TrimSpace(language)]
	return ok
}

// wikipediaParams is the JSON shape the model sends into InvokableRun.
// LLM only sees query via ToolMeta(); lang and max_results are accepted for
// non-agent callers and fall back to the tool instance's node-level
// defaults (WikipediaTool.lang / WikipediaTool.topN) when absent.
type wikipediaParams struct {
	Query      string `json:"query"`
	Lang       string `json:"lang,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

// wikipediaResult mirrors the fields Python passes to _retrieve_chunks:
// title, url, and summary content.
type wikipediaResult struct {
	Title   string `json:"title"`
	Content string `json:"content"`
	URL     string `json:"url"`
}

// wikipediaResponse is the upstream MediaWiki generator=search envelope.
type wikipediaResponse struct {
	Query struct {
		Pages map[string]struct {
			Index   int    `json:"index"`
			Title   string `json:"title"`
			Extract string `json:"extract"`
			FullURL string `json:"fullurl"`
		} `json:"pages"`
	} `json:"query"`
}

// wikipediaEnvelope is what the model sees. The formalized_content field
// is the canvas-facing equivalent of Python ToolBase._retrieve_chunks().
type wikipediaEnvelope struct {
	FormalizedContent string            `json:"formalized_content,omitempty"`
	Results           []wikipediaResult `json:"results,omitempty"`
	Error             string            `json:"_ERROR,omitempty"`
}

// WikipediaTool is the Wikipedia
// search tool. It calls the
// public MediaWiki action API via the shared HTTPHelper and returns the
// top N matches for the query.
type WikipediaTool struct {
	helper *HTTPHelper
	topN   int
	lang   string
}

// NewWikipediaTool returns a WikipediaTool using the default HTTPHelper.
func NewWikipediaTool() *WikipediaTool {
	return NewWikipediaToolWith(NewHTTPHelper())
}

// NewWikipediaToolWith returns a WikipediaTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewWikipediaToolWith(h *HTTPHelper) *WikipediaTool {
	return NewWikipediaToolWithParams(h, defaultWikipediaTopN, defaultWikipediaLanguage)
}

// NewWikipediaToolWithParams returns a WikipediaTool with node-level
// parameters matching Python's WikipediaParam.top_n and language fields.
func NewWikipediaToolWithParams(h *HTTPHelper, topN int, language string) *WikipediaTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if topN <= 0 {
		topN = defaultWikipediaTopN
	}
	if strings.TrimSpace(language) == "" {
		language = defaultWikipediaLanguage
	}
	return &WikipediaTool{helper: h, topN: topN, lang: strings.TrimSpace(language)}
}

// ToolMeta returns the tool's metadata for the chat model.
func (w *WikipediaTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        wikipediaToolName,
		Description: wikipediaToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "The search keyword to execute with wikipedia. The keyword MUST be a specific subject that can match the title.",
				Required:    true,
			},
		},
	}
}

// buildWikipediaURL constructs a MediaWiki generator=search URL that returns
// the same page fields Python reads from wikipedia.page(): title, url,
// and summary-like introductory extract.
func buildWikipediaURL(lang, query string, topN int) string {
	if lang == "" {
		lang = defaultWikipediaLanguage
	}
	if topN <= 0 {
		topN = defaultWikipediaTopN
	}
	return fmt.Sprintf(
		"https://%s.wikipedia.org/w/api.php?action=query&generator=search&format=json&gsrlimit=%d&gsrsearch=%s&prop=extracts%%7Cinfo&exintro=1&explaintext=1&inprop=url",
		lang,
		topN,
		url.QueryEscape(query),
	)
}

// InvokableRun performs the Wikipedia search.
func (w *WikipediaTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var p wikipediaParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return wikipediaErrJSON(fmt.Errorf("wikipedia: parse arguments: %w", err)),
			fmt.Errorf("wikipedia: parse arguments: %w", err)
	}
	if p.Query == "" {
		return wikipediaErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("wikipedia: query is required")
	}

	lang := p.Lang
	if lang == "" {
		lang = w.lang
	}
	maxResults := p.MaxResults
	if maxResults <= 0 {
		maxResults = w.topN
	}
	if !WikipediaLanguageSupported(lang) {
		return wikipediaErrJSON(fmt.Errorf("wikipedia: unsupported language %q", lang)),
			fmt.Errorf("wikipedia: unsupported language %q", lang)
	}
	endpoint := buildWikipediaURL(lang, p.Query, maxResults)
	resp, err := w.helper.Do(ctx, http.MethodGet, endpoint, "", "", map[string]string{
		"User-Agent": wikipediaUserAgent,
	})
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

	pages := make([]struct {
		Index   int
		Title   string
		Extract string
		FullURL string
	}, 0, len(raw.Query.Pages))
	for _, p := range raw.Query.Pages {
		pages = append(pages, struct {
			Index   int
			Title   string
			Extract string
			FullURL string
		}{
			Index:   p.Index,
			Title:   p.Title,
			Extract: p.Extract,
			FullURL: p.FullURL,
		})
	}
	sort.SliceStable(pages, func(i, j int) bool { return pages[i].Index < pages[j].Index })

	results := make([]wikipediaResult, 0, len(pages))
	for _, p := range pages {
		content := strings.TrimSpace(p.Extract)
		if content == "" {
			continue
		}
		if len(content) > 10000 {
			content = content[:10000]
		}
		fullURL := p.FullURL
		if fullURL == "" {
			fullURL = fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, url.PathEscape(p.Title))
		}
		results = append(results, wikipediaResult{
			Title:   p.Title,
			Content: content,
			URL:     fullURL,
		})
	}
	return wikipediaJSON(wikipediaEnvelope{
		FormalizedContent: renderWikipediaResults(results),
		Results:           results,
	}), nil
}

func renderWikipediaResults(results []wikipediaResult) string {
	if len(results) == 0 {
		return ""
	}
	blocks := make([]string, 0, len(results))
	for _, r := range results {
		blocks = append(blocks, fmt.Sprintf("Title: %s\nURL: %s\nContent: %s", r.Title, r.URL, r.Content))
	}
	return strings.Join(blocks, "\n\n")
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
