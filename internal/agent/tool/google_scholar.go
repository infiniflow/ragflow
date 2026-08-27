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
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/net/html"
)

const googleScholarToolName = "google_scholar"

const googleScholarToolDescription = "Google Scholar provides a simple way to broadly search for scholarly literature. From one place, you can search across many disciplines and sources: articles, theses, books, abstracts and court opinions, from academic publishers, professional societies, online repositories, universities and other web sites. Google Scholar helps you find relevant work across the world of scholarly research."

const defaultGoogleScholarTopN = 12

// googleScholarParams is the JSON shape the model sends into InvokableRun.
// All fields come from the canvas node form; only query is exposed to
// the LLM via Info() (matching Python's meta — the LLM only sees query).
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

// GoogleScholarTool is the
// Google Scholar search tool.
// There is no public Scholar API, so we fetch the search-results
// HTML and parse it with golang.org/x/net/html.
type GoogleScholarTool struct {
	helper   *HTTPHelper
	defaults googleScholarParams
}

// NewGoogleScholarTool returns a GoogleScholarTool using the default
// HTTPHelper.
func NewGoogleScholarTool() *GoogleScholarTool {
	return NewGoogleScholarToolWith(NewHTTPHelper())
}

// NewGoogleScholarToolWith returns a GoogleScholarTool that uses the
// provided HTTPHelper. Useful for tests.
func NewGoogleScholarToolWith(h *HTTPHelper) *GoogleScholarTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GoogleScholarTool{helper: h}
}

// NewGoogleScholarToolWithDefaults returns a GoogleScholarTool that
// uses the provided HTTPHelper and node-level default params.
func NewGoogleScholarToolWithDefaults(h *HTTPHelper, defaults googleScholarParams) *GoogleScholarTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &GoogleScholarTool{helper: h, defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
// Only query is exposed — matching Python's meta which does not
// include top_n / sort_by / year_low / year_high / patents.
// Those are canvas-form-only parameters passed by the component wrapper.
func (g *GoogleScholarTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: googleScholarToolName,
		Desc: googleScholarToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keyword to execute with Google Scholar. The keywords should be the most important words/terms(includes synonyms) from the original request.",
				Required: true,
			},
		}),
	}, nil
}

// buildGoogleScholarURL composes the Scholar query URL. Centralized
// for testability. sortBy: "relevance" (default) or "date".
// yearLow / yearHigh: 0 means no filter. patents: nil or true includes
// patents; false explicitly excludes them.
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
func (g *GoogleScholarTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p googleScholarParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return googleScholarErrJSON(fmt.Errorf("google_scholar: parse arguments: %w", err)),
			fmt.Errorf("google_scholar: parse arguments: %w", err)
	}
	p = mergeGoogleScholarDefaults(g.defaults, p)
	if strings.TrimSpace(p.Query) == "" {
		return googleScholarErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("google_scholar: query is required")
	}

	endpoint := buildGoogleScholarURL(p.Query, p.TopN, p.SortBy, p.YearLow, p.YearHigh, p.Patents)
	headers := map[string]string{
		// Scholar blocks obviously non-browser UAs.
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

// parseGoogleScholarHTML walks the Scholar search-results HTML and
// extracts the conventional .gs_rt / .gs_a / .gs_rs fields. We
// deliberately stay defensive: Scholar's markup changes without
// notice, so we tolerate missing fields and silently skip articles
// that are missing the title.
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
					// gs_ri wraps one Scholar result card
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

// extractScholarResult pulls title/link, snippet, and authors/year
// from a single .gs_ri node. Returns ok=false when the title anchor
// is missing (e.g. PDF / citation links the search layout omits).
func extractScholarResult(card *html.Node) (googleScholarResult, bool) {
	res := googleScholarResult{}

	// Title + link live inside .gs_rt > a
	title, link := findFirstAnchorInClassedAncestor(card, "gs_rt")
	if title == "" {
		return res, false
	}
	res.Title = strings.TrimSpace(title)
	res.Link = link

	// Authors + year live in .gs_a (a single line)
	if t := findTextWithClass(card, "gs_a"); t != "" {
		authors, year := splitScholarAuthorsYear(t)
		res.Authors = authors
		res.Year = year
	}

	// Snippet lives in .gs_rs
	if t := findTextWithClass(card, "gs_rs"); t != "" {
		res.Snippet = strings.TrimSpace(t)
	}

	return res, true
}

// findFirstAnchorInClassedAncestor returns the text and href of the
// first <a> descendant of n whose ancestor chain contains an element
// with `want` in its class list. The `want` argument lets callers
// pin the search to a specific Scholar sub-element (e.g. .gs_rt).
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

// findTextWithClass returns the concatenated text of the first
// descendant element that has `want` in its class list. If the
// matched element is empty, the search continues into its subtree.
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

// collectText concatenates all text nodes under n (trimmed of
// surrounding whitespace per node).
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

// splitScholarAuthorsYear parses the .gs_a line, which has the form
// "<authors> - <journal>, <year>" or "<authors> - <year>". We pull
// the first 4-digit year out and treat everything before " - " as
// the author list. Anything we can't parse is returned verbatim so
// the model can still see it.
func splitScholarAuthorsYear(line string) (authors, year string) {
	cleaned := strings.TrimSpace(line)
	// The hyphen between authors and venue is the unicode dash "-".
	if head, rest, ok := strings.Cut(cleaned, " - "); ok {
		authors = strings.TrimSpace(head)
		venue := strings.TrimSpace(rest)
		year = firstFourDigitYear(venue)
		return authors, year
	}
	year = firstFourDigitYear(cleaned)
	return cleaned, year
}

// firstFourDigitYear returns the first 4-digit year in s, or "" if
// none is found. Years 1900-2099 are recognized.
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
