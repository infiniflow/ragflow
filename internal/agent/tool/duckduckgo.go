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
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"golang.org/x/net/html"
)

const duckduckgoToolName = "duckduckgo"

const duckduckgoToolDescription = "Search DuckDuckGo web or news results. Returns results[].{title, url, body}."

const duckduckgoChannelGeneral = "general"
const duckduckgoChannelNews = "news"

var duckduckgoSearchEndpoint = "https://duckduckgo.com/html/"
var duckduckgoNewsEndpoint = "https://duckduckgo.com/news.js"
var duckduckgoNewsBootstrapEndpoint = "https://duckduckgo.com/"

var duckduckgoVQDPattern = regexp.MustCompile(`vqd="([^"]+)"`)

type duckduckgoParams struct {
	Query   string `json:"query"`
	Channel string `json:"channel"`
	TopN    int    `json:"top_n"`
}

type duckduckgoResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Body  string `json:"body"`
}

type duckduckgoEnvelope struct {
	Results []duckduckgoResult `json:"results"`
	Error   string             `json:"_ERROR,omitempty"`
}

type duckduckgoNewsResponse struct {
	Results []duckduckgoNewsItem `json:"results"`
}

type duckduckgoNewsItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Excerpt string `json:"excerpt"`
}

type DuckDuckGoTool struct {
	helper *HTTPHelper
}

func NewDuckDuckGoTool() *DuckDuckGoTool {
	return NewDuckDuckGoToolWith(NewHTTPHelper())
}

func NewDuckDuckGoToolWith(h *HTTPHelper) *DuckDuckGoTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &DuckDuckGoTool{helper: h}
}

func (d *DuckDuckGoTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: duckduckgoToolName,
		Desc: duckduckgoToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query.",
				Required: true,
			},
			"channel": {
				Type:     schema.String,
				Desc:     "Search channel: general or news. Defaults to general.",
				Required: false,
			},
			"top_n": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 10.",
				Required: false,
			},
		}),
	}, nil
}

func buildDuckDuckGoSearchURL(query string, topN int) string {
	if topN <= 0 {
		topN = 10
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("kl", "wt-wt")
	q.Set("dc", strconv.Itoa(topN+1))
	return duckduckgoSearchEndpoint + "?" + q.Encode()
}

func buildDuckDuckGoNewsURL(query string, topN int) string {
	return buildDuckDuckGoNewsURLWithVQD(query, topN, "")
}

func buildDuckDuckGoNewsURLWithVQD(query string, topN int, vqd string) string {
	if topN <= 0 {
		topN = 10
	}
	q := url.Values{}
	q.Set("q", query)
	q.Set("l", "wt-wt")
	q.Set("o", "json")
	q.Set("p", "1")
	q.Set("s", "0")
	q.Set("dc", strconv.Itoa(topN))
	if strings.TrimSpace(vqd) != "" {
		q.Set("vqd", vqd)
		q.Set("u", "bing")
	}
	return duckduckgoNewsEndpoint + "?" + q.Encode()
}

func buildDuckDuckGoNewsBootstrapURL(query string) string {
	q := url.Values{}
	q.Set("q", query)
	q.Set("iar", "news")
	q.Set("ia", "news")
	q.Set("kl", "wt-wt")
	return duckduckgoNewsBootstrapEndpoint + "?" + q.Encode()
}

func (d *DuckDuckGoTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p duckduckgoParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: parse arguments: %w", err)),
			fmt.Errorf("duckduckgo: parse arguments: %w", err)
	}
	if strings.TrimSpace(p.Query) == "" {
		return duckduckgoErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("duckduckgo: query is required")
	}

	channel := normalizeDuckDuckGoChannel(p.Channel)
	topN := p.TopN
	if topN <= 0 {
		topN = 10
	}

	if channel == duckduckgoChannelNews {
		return d.runNewsSearch(ctx, p.Query, topN)
	}

	endpoint := buildDuckDuckGoSearchURL(p.Query, topN)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
		"Accept":     "text/html,application/xhtml+xml",
	}

	resp, err := d.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return duckduckgoErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("duckduckgo: upstream returned %d", resp.StatusCode)
	}

	results, err := parseDuckDuckGoHTML(resp.Body, channel, topN)
	if err != nil {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: parse html: %w", err)),
			fmt.Errorf("duckduckgo: parse html: %w", err)
	}
	return duckduckgoJSON(duckduckgoEnvelope{Results: results}), nil
}

func (d *DuckDuckGoTool) runNewsSearch(ctx context.Context, query string, topN int) (string, error) {
	vqd, err := d.fetchDuckDuckGoNewsVQD(ctx, query)
	if err != nil {
		return duckduckgoErrJSON(err), err
	}

	endpoint := buildDuckDuckGoNewsURLWithVQD(query, topN, vqd)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
		"Accept":     "application/json,text/javascript,*/*",
	}

	resp, err := d.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return duckduckgoErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("duckduckgo: upstream returned %d", resp.StatusCode)
	}

	var raw duckduckgoNewsResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: decode news response: %w", err)),
			fmt.Errorf("duckduckgo: decode news response: %w", err)
	}

	results := make([]duckduckgoResult, 0, min(topN, len(raw.Results)))
	for _, item := range raw.Results {
		if len(results) >= topN {
			break
		}
		title := normalizeWhitespace(htmlUnescape(item.Title))
		resultURL := strings.TrimSpace(item.URL)
		body := normalizeWhitespace(htmlUnescape(item.Excerpt))
		if title == "" || resultURL == "" {
			continue
		}
		results = append(results, duckduckgoResult{
			Title: title,
			URL:   resultURL,
			Body:  body,
		})
	}

	return duckduckgoJSON(duckduckgoEnvelope{Results: results}), nil
}

func (d *DuckDuckGoTool) fetchDuckDuckGoNewsVQD(ctx context.Context, query string) (string, error) {
	endpoint := buildDuckDuckGoNewsBootstrapURL(query)
	headers := map[string]string{
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
		"Accept":     "text/html,application/xhtml+xml",
	}

	resp, err := d.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("duckduckgo: news bootstrap returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("duckduckgo: read news bootstrap: %w", err)
	}
	matches := duckduckgoVQDPattern.FindSubmatch(body)
	if len(matches) != 2 {
		return "", fmt.Errorf("duckduckgo: news bootstrap missing vqd")
	}
	return string(matches[1]), nil
}

func normalizeDuckDuckGoChannel(channel string) string {
	value := strings.ToLower(strings.TrimSpace(channel))
	switch value {
	case "", "text", duckduckgoChannelGeneral:
		return duckduckgoChannelGeneral
	case duckduckgoChannelNews:
		return duckduckgoChannelNews
	default:
		return duckduckgoChannelGeneral
	}
}

func parseDuckDuckGoHTML(body interface {
	Read(p []byte) (int, error)
}, channel string, topN int) ([]duckduckgoResult, error) {
	if topN <= 0 {
		topN = 10
	}
	doc, err := html.Parse(body)
	if err != nil {
		return nil, err
	}
	if channel == duckduckgoChannelNews {
		results := extractDuckDuckGoNewsResults(doc, topN)
		if len(results) > 0 {
			return results, nil
		}
	}
	return extractDuckDuckGoGeneralResults(doc, topN), nil
}

func extractDuckDuckGoGeneralResults(doc *html.Node, topN int) []duckduckgoResult {
	results := make([]duckduckgoResult, 0, topN)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= topN {
			return
		}
		if n.Type == html.ElementNode && hasClassToken(n, "result") {
			if res, ok := extractDuckDuckGoGeneralResult(n); ok {
				results = append(results, res)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results
}

func extractDuckDuckGoGeneralResult(card *html.Node) (duckduckgoResult, bool) {
	var out duckduckgoResult
	titleNode := findFirstNode(card, func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == "a" && hasClassToken(n, "result__a")
	})
	if titleNode == nil {
		return out, false
	}
	out.Title = normalizeWhitespace(collectText(titleNode))
	out.URL = normalizeDuckDuckGoLink(attrValue(titleNode, "href"))
	out.Body = normalizeWhitespace(textByClass(card, "result__snippet"))
	if out.Title == "" || out.URL == "" {
		return duckduckgoResult{}, false
	}
	return out, true
}

func extractDuckDuckGoNewsResults(doc *html.Node, topN int) []duckduckgoResult {
	results := make([]duckduckgoResult, 0, topN)
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= topN {
			return
		}
		if n.Type == html.ElementNode && (hasClassToken(n, "result") || hasClassToken(n, "module--news")) {
			if res, ok := extractDuckDuckGoNewsResult(n); ok {
				results = append(results, res)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return dedupeDuckDuckGoResults(results, topN)
}

func extractDuckDuckGoNewsResult(card *html.Node) (duckduckgoResult, bool) {
	var out duckduckgoResult
	titleNode := findFirstNode(card, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return false
		}
		return hasClassToken(n, "result__title") || hasClassToken(n, "result-link") || hasClassToken(n, "result__a")
	})
	if titleNode == nil {
		return out, false
	}
	out.Title = normalizeWhitespace(collectText(titleNode))
	out.URL = normalizeDuckDuckGoLink(attrValue(titleNode, "href"))
	out.Body = normalizeWhitespace(firstNonEmpty(
		textByClass(card, "result__snippet"),
		textByClass(card, "result__body"),
		textByClass(card, "result__extras"),
	))
	if out.Title == "" || out.URL == "" {
		return duckduckgoResult{}, false
	}
	return out, true
}

func dedupeDuckDuckGoResults(in []duckduckgoResult, topN int) []duckduckgoResult {
	seen := make(map[string]struct{}, len(in))
	out := make([]duckduckgoResult, 0, min(topN, len(in)))
	for _, item := range in {
		key := item.URL + "\n" + item.Title
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
		if len(out) >= topN {
			break
		}
	}
	return out
}

func findFirstNode(root *html.Node, match func(*html.Node) bool) *html.Node {
	if root == nil {
		return nil
	}
	if match(root) {
		return root
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if hit := findFirstNode(c, match); hit != nil {
			return hit
		}
	}
	return nil
}

func textByClass(root *html.Node, className string) string {
	node := findFirstNode(root, func(n *html.Node) bool {
		return n.Type == html.ElementNode && hasClassToken(n, className)
	})
	if node == nil {
		return ""
	}
	return collectText(node)
}

func hasClassToken(n *html.Node, want string) bool {
	if n == nil || n.Type != html.ElementNode || want == "" {
		return false
	}
	for _, a := range n.Attr {
		if a.Key != "class" {
			continue
		}
		for _, token := range strings.Fields(a.Val) {
			if token == want {
				return true
			}
		}
	}
	return false
}

func attrValue(n *html.Node, key string) string {
	if n == nil {
		return ""
	}
	for _, a := range n.Attr {
		if a.Key == key {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func normalizeDuckDuckGoLink(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return raw
	}
	if strings.Contains(parsed.Host, "duckduckgo.com") {
		q := parsed.Query().Get("uddg")
		if q != "" {
			if decoded, err := url.QueryUnescape(q); err == nil && decoded != "" {
				return decoded
			}
			return q
		}
	}
	return raw
}

func normalizeWhitespace(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func htmlUnescape(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&quot;", `"`,
		"&#39;", "'",
		"&#x27;", "'",
		"&lt;", "<",
		"&gt;", ">",
	)
	return replacer.Replace(s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func duckduckgoJSON(env duckduckgoEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"duckduckgo: marshal result: %s"}`, err)
	}
	return string(b)
}

func duckduckgoErrJSON(err error) string {
	return duckduckgoJSON(duckduckgoEnvelope{Error: err.Error()})
}
