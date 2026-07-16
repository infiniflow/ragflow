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
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"

	"ragflow/internal/tokenizer"
)

const googleScholarToolName = "google_scholar_search"

const googleScholarToolDescription = "Google Scholar provides a simple way to broadly search for scholarly literature. From one place, you can search across many disciplines and sources: articles, theses, books, abstracts and court opinions, from academic publishers, professional societies, online repositories, universities and other web sites. Google Scholar helps you find relevant work across the world of scholarly research."

const defaultGoogleScholarTopN = 12

const googleScholarPromptMaxTokens = 200000

var googleScholarDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var googleScholarNewlinePattern = regexp.MustCompile(`\n+`)

// googleScholarParams is the JSON shape the model sends into InvokableRun.
// All fields come from the canvas node form; only query is exposed to
// the LLM via ToolMeta() (matching Python's meta — the LLM only sees query).
// Patents uses *bool so the zero value (nil) means "not set" → default
// true (include patents). False means explicitly exclude patents.
type googleScholarParams struct {
	Query    string `json:"query"`
	TopN     int    `json:"top_n"`
	SortBy   string `json:"sort_by"`
	YearLow  int    `json:"year_low"`
	YearHigh int    `json:"year_high"`
	Patents  *bool  `json:"patents"`
}

// googleScholarResult is one row in the parsed result list.
type googleScholarResult struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
	Authors string `json:"authors"`
	Year    string `json:"year"`
}

// googleScholarEnvelope is what the model sees.
type googleScholarEnvelope struct {
	Results []googleScholarResult `json:"results"`
	Error   string                `json:"_ERROR,omitempty"`
}

// googleScholarEndpoint is the Google Scholar search URL. Exposed as
// a package var so tests can substitute a httptest.Server URL.
var googleScholarEndpoint = "https://scholar.google.com/scholar"

// GoogleScholarTool searches Google Scholar.
type GoogleScholarTool struct {
	helper   *HTTPHelper
	defaults googleScholarParams
}

var _ ToolComponent = (*GoogleScholarTool)(nil)
var _ ReferenceBuilder = (*GoogleScholarTool)(nil)

// NewGoogleScholarTool returns a GoogleScholarTool using the default
// HTTPHelper.
func NewGoogleScholarTool() *GoogleScholarTool {
	return NewGoogleScholarToolWith(NewHTTPHelper())
}

// NewGoogleScholarToolWith returns a GoogleScholarTool using the provided HTTPHelper.
func NewGoogleScholarToolWith(h *HTTPHelper) *GoogleScholarTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GoogleScholarTool{helper: h}
}

// NewGoogleScholarToolWithDefaults returns a GoogleScholarTool with node-level defaults.
func NewGoogleScholarToolWithDefaults(h *HTTPHelper, defaults googleScholarParams) *GoogleScholarTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GoogleScholarTool{helper: h, defaults: defaults}
}

// ToolMeta returns the tool's metadata for the chat model.
// Only query is exposed — matching Python's meta.
func (g *GoogleScholarTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        googleScholarToolName,
		Description: googleScholarToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "The search keyword to execute with Google Scholar. The keywords should be the most important words/terms(includes synonyms) from the original request.",
				Required:    true,
			},
		},
	}
}

func (g *GoogleScholarTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":     "Search query.",
			"top_n":     "Maximum number of results.",
			"sort_by":   "Sort order: relevance or date.",
			"year_low":  "Earliest publication year to include.",
			"year_high": "Latest publication year to include.",
			"patents":   "Whether to include patents.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Google Scholar references for downstream prompts.",
			"json":               "Google Scholar result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
		},
	}
}

// buildGoogleScholarURL composes the Scholar query URL. Centralized
// for testability. sortBy: "relevance" (default) or "date".
func buildGoogleScholarURL(query string, maxResults int, sortBy string, yearLow, yearHigh int, patents *bool) string {
	if maxResults <= 0 {
		maxResults = defaultGoogleScholarTopN
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("hl", "en")
	q.Set("num", strconv.Itoa(maxResults))

	if sortBy == "date" {
		q.Set("scisbd", "1")
	}
	if yearLow > 0 {
		q.Set("as_ylo", strconv.Itoa(yearLow))
	}
	if yearHigh > 0 {
		q.Set("as_yhi", strconv.Itoa(yearHigh))
	}
	if patents != nil && !*patents {
		q.Set("as_vis", "1")
	}

	return googleScholarEndpoint + "?" + q.Encode()
}

// InvokableRun performs the Google Scholar search.
func (g *GoogleScholarTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var p googleScholarParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return googleScholarErrJSON(fmt.Errorf("google_scholar: parse arguments: %w", err)),
			fmt.Errorf("google_scholar: parse arguments: %w", err)
	}
	p = mergeGoogleScholarDefaults(g.defaults, p)
	if strings.TrimSpace(p.Query) == "" {
		return googleScholarJSON(googleScholarEnvelope{Results: []googleScholarResult{}}), nil
	}

	endpoint := buildGoogleScholarURL(p.Query, p.TopN, p.SortBy, p.YearLow, p.YearHigh, p.Patents)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
		"Accept":     "text/html,application/xhtml+xml",
	}

	resp, err := g.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return googleScholarErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return googleScholarErrJSON(fmt.Errorf("google_scholar: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("google_scholar: upstream returned %d", resp.StatusCode)
	}

	results, err := parseGoogleScholarHTML(resp.Body, p.TopN)
	if err != nil {
		return googleScholarErrJSON(fmt.Errorf("google_scholar: parse html: %w", err)),
			fmt.Errorf("google_scholar: parse html: %w", err)
	}
	return googleScholarJSON(googleScholarEnvelope{Results: results}), nil
}

func (g *GoogleScholarTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildGoogleScholarReferences(envelope)
}

func (g *GoogleScholarTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildGoogleScholarReferences(envelope)
	return map[string]any{
		"formalized_content": renderGoogleScholarReferences(chunks, googleScholarPromptMaxTokens),
		"json":               results,
	}
}

func buildGoogleScholarReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		paper, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := strings.Join([]string{
			"Authors: " + googleScholarText(paper["authors"]),
			"Year: " + googleScholarText(paper["year"]),
			"Snippet: " + googleScholarText(paper["snippet"]),
		}, "\n")
		content = googleScholarDataImagePattern.ReplaceAllString(content, "")
		content = truncateGoogleScholarRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(googleScholarHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(googleScholarHashInt(documentID, 500), 10)
		title := googleScholarText(paper["title"])
		resultURL := googleScholarText(paper["link"])
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

func renderGoogleScholarReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := googleScholarText(chunk["content"])
		block := strings.Join([]string{
			"\nID: " + googleScholarText(chunk["id"]),
			"├── Title: " + googleScholarNewlinePattern.ReplaceAllString(googleScholarText(chunk["document_name"]), " "),
			"├── URL: " + googleScholarNewlinePattern.ReplaceAllString(googleScholarText(chunk["url"]), " "),
			"└── Content:\n" + content,
		}, "\n")
		blockTokens := tokenizer.NumTokensFromString(block)
		if maxTokens > 0 && float64(usedTokens+blockTokens) > float64(maxTokens)*0.97 {
			break
		}
		usedTokens += blockTokens
		blocks = append(blocks, block)
	}
	return strings.Join(blocks, "\n")
}

func googleScholarText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func googleScholarHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateGoogleScholarRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func mergeGoogleScholarDefaults(defaults, p googleScholarParams) googleScholarParams {
	if p.Query == "" {
		p.Query = defaults.Query
	}
	if p.TopN == 0 {
		p.TopN = defaults.TopN
	}
	if p.SortBy == "" {
		p.SortBy = defaults.SortBy
	}
	if p.YearLow == 0 {
		p.YearLow = defaults.YearLow
	}
	if p.YearHigh == 0 {
		p.YearHigh = defaults.YearHigh
	}
	if p.Patents == nil {
		p.Patents = defaults.Patents
	}
	return p
}

// parseGoogleScholarHTML walks the Scholar search-results HTML.
func parseGoogleScholarHTML(body interface {
	Read(p []byte) (int, error)
}, maxResults int) ([]googleScholarResult, error) {
	if maxResults <= 0 {
		maxResults = defaultGoogleScholarTopN
	}
	doc, err := html.Parse(body)
	if err != nil {
		return nil, err
	}

	var results []googleScholarResult
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= maxResults {
			return
		}
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Key == "class" && strings.Contains(a.Val, "gs_ri") {
					if r, ok := extractScholarResult(n); ok {
						results = append(results, r)
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results, nil
}

func extractScholarResult(card *html.Node) (googleScholarResult, bool) {
	res := googleScholarResult{}

	title, link := findFirstAnchorInClassedAncestor(card, "gs_rt")
	if title == "" {
		return res, false
	}
	res.Title = strings.TrimSpace(title)
	res.Link = link

	if t := findTextWithClass(card, "gs_a"); t != "" {
		authors, year := splitScholarAuthorsYear(t)
		res.Authors = authors
		res.Year = year
	}

	if t := findTextWithClass(card, "gs_rs"); t != "" {
		res.Snippet = strings.TrimSpace(t)
	}

	return res, true
}

func findFirstAnchorInClassedAncestor(n *html.Node, want string) (string, string) {
	var text, href string
	var found bool
	var walk func(*html.Node, bool)
	walk = func(node *html.Node, inTarget bool) {
		if found {
			return
		}
		here := inTarget
		if node.Type == html.ElementNode {
			for _, a := range node.Attr {
				if a.Key == "class" && strings.Contains(a.Val, want) {
					here = true
					break
				}
			}
			if here && node.Data == "a" {
				for _, a := range node.Attr {
					if a.Key == "href" {
						href = a.Val
					}
				}
				text = collectText(node)
				found = true
				return
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c, here)
		}
	}
	walk(n, false)
	return text, href
}

func findTextWithClass(n *html.Node, want string) string {
	var found string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if found != "" {
			return
		}
		if node.Type == html.ElementNode {
			for _, a := range node.Attr {
				if a.Key == "class" && strings.Contains(a.Val, want) {
					found = collectText(node)
					return
				}
			}
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return found
}

// collectText concatenates all text nodes under n.
func collectText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			b.WriteString(node.Data)
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

func splitScholarAuthorsYear(line string) (authors, year string) {
	cleaned := strings.TrimSpace(line)
	if head, rest, ok := strings.Cut(cleaned, " - "); ok {
		authors = strings.TrimSpace(head)
		year = firstFourDigitYear(strings.TrimSpace(rest))
		return authors, year
	}
	year = firstFourDigitYear(cleaned)
	return cleaned, year
}

func firstFourDigitYear(s string) string {
	for i := 0; i+4 <= len(s); i++ {
		candidate := s[i : i+4]
		n, err := strconv.Atoi(candidate)
		if err != nil {
			continue
		}
		if n >= 1900 && n <= 2099 {
			return candidate
		}
	}
	return ""
}

func googleScholarJSON(env googleScholarEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"google_scholar: marshal result: %s"}`, err)
	}
	return string(b)
}

func googleScholarErrJSON(err error) string {
	return googleScholarJSON(googleScholarEnvelope{Error: err.Error()})
}
