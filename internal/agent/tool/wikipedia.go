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
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const wikipediaToolName = "wikipedia_search"

const wikipediaToolDescription = "A wide range of how-to and information pages are made available in wikipedia. Since 2001, it has grown rapidly to become the world's largest reference website. From Wikipedia, the free encyclopedia."

const wikipediaUserAgent = "Mozilla/5.0 (compatible; ragflow/1.0; +https://github.com/infiniflow/ragflow)"

const (
	defaultWikipediaTopN     = 10
	defaultWikipediaLanguage = "en"
	wikipediaPromptMaxTokens = 200000
)

var wikipediaDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var wikipediaNewlinePattern = regexp.MustCompile(`\n+`)

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
// LLM only sees query via Info(); lang and max_results are accepted for
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
	Results           []wikipediaResult `json:"results"`
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

var _ ToolComponent = (*WikipediaTool)(nil)
var _ ReferenceBuilder = (*WikipediaTool)(nil)

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

// Info returns the tool's metadata for the chat model.
func (w *WikipediaTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: wikipediaToolName,
		Desc: wikipediaToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keyword to execute with wikipedia. The keyword MUST be a specific subject that can match the title.",
				Required: true,
			},
		}),
	}, nil
}

func (w *WikipediaTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query": "The search keyword to execute with wikipedia. The keyword MUST be a specific subject that can match the title.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Wikipedia references for downstream prompts.",
			"json":               "Wikipedia result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
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
func (w *WikipediaTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p wikipediaParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return wikipediaErrJSON(fmt.Errorf("wikipedia: parse arguments: %w", err)),
			fmt.Errorf("wikipedia: parse arguments: %w", err)
	}
	if p.Query == "" {
		return wikipediaJSON(wikipediaEnvelope{Results: []wikipediaResult{}}), nil
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

func (w *WikipediaTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildWikipediaReferences(envelope)
}

func (w *WikipediaTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildWikipediaReferences(envelope)
	return map[string]any{
		"formalized_content": renderWikipediaReferences(chunks, wikipediaPromptMaxTokens),
		"json":               results,
	}
}

func buildWikipediaReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := wikipediaDataImagePattern.ReplaceAllString(wikipediaText(item["content"]), "")
		content = truncateWikipediaRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(wikipediaHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(wikipediaHashInt(documentID, 500), 10)
		title := wikipediaText(item["title"])
		resultURL := wikipediaText(item["url"])
		chunks = append(chunks, map[string]any{
			"id":            displayID,
			"chunk_id":      documentID,
			"content":       content,
			"doc_id":        documentID,
			"document_id":   documentID,
			"docnm_kwd":     title,
			"document_name": title,
			"similarity":    1,
			"score":         1,
			"url":           resultURL,
		})
		docAggs = append(docAggs, map[string]any{"doc_name": title, "doc_id": documentID, "count": 1, "url": resultURL})
	}
	return chunks, docAggs
}

func renderWikipediaReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := wikipediaText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + wikipediaText(chunk["id"]),
			"├── Title: " + wikipediaNewlinePattern.ReplaceAllString(wikipediaText(chunk["document_name"]), " "),
			"├── URL: " + wikipediaNewlinePattern.ReplaceAllString(wikipediaText(chunk["url"]), " "),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func wikipediaText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func wikipediaHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateWikipediaRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
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
