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
	"html"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	xhtml "golang.org/x/net/html"

	"ragflow/internal/tokenizer"
)

const duckduckgoToolName = "duckduckgo_search"

const duckduckgoToolDescription = "Search DuckDuckGo web or news results. Returns results[].{title, url, body}."

const duckduckgoChannelGeneral = "general"
const duckduckgoChannelNews = "news"

const duckduckgoPromptMaxTokens = 200000

var duckduckgoSearchEndpoint = "https://duckduckgo.com/html/"
var duckduckgoNewsEndpoint = "https://duckduckgo.com/news.js"
var duckduckgoNewsBootstrapEndpoint = "https://duckduckgo.com/"

var duckduckgoVQDPattern = regexp.MustCompile(`vqd="([^"]+)"`)

var duckduckgoDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var duckduckgoNewlinePattern = regexp.MustCompile(`\n+`)

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
	helper   *HTTPHelper
	defaults duckduckgoParams
}

var _ ToolComponent = (*DuckDuckGoTool)(nil)
var _ ReferenceBuilder = (*DuckDuckGoTool)(nil)

func NewDuckDuckGoTool() *DuckDuckGoTool {
	return newDuckDuckGoTool(nil, duckduckgoParams{})
}

func NewDuckDuckGoToolWith(h *HTTPHelper) *DuckDuckGoTool {
	return newDuckDuckGoTool(h, duckduckgoParams{})
}

func newDuckDuckGoTool(h *HTTPHelper, defaults duckduckgoParams) *DuckDuckGoTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.TopN == 0 {
		defaults.TopN = 10
	}
	if defaults.Channel == "" {
		defaults.Channel = duckduckgoChannelGeneral
	}
	return &DuckDuckGoTool{helper: h, defaults: defaults}
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
		}),
	}, nil
}

func (d *DuckDuckGoTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":   "Search query.",
			"channel": "Search channel: general or news.",
			"top_n":   "Maximum number of results.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered DuckDuckGo references for downstream prompts.",
			"json":               "DuckDuckGo result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
			"channel": map[string]any{
				"name": "Channel", "type": "options", "value": "general", "options": []string{"general", "news"},
			},
		},
	}
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
		return duckduckgoJSON(duckduckgoEnvelope{Results: []duckduckgoResult{}}), nil
	}
	p = mergeDuckDuckGoParams(d.defaults, p)

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

	results, err := parseDuckDuckGoHTML(resp.Body, topN)
	if err != nil {
		return duckduckgoErrJSON(fmt.Errorf("duckduckgo: parse html: %w", err)),
			fmt.Errorf("duckduckgo: parse html: %w", err)
	}
	return duckduckgoJSON(duckduckgoEnvelope{Results: results}), nil
}

func mergeDuckDuckGoParams(defaults, params duckduckgoParams) duckduckgoParams {
	if params.Channel == "" {
		params.Channel = defaults.Channel
	}
	if params.TopN == 0 {
		params.TopN = defaults.TopN
	}
	return params
}

func (d *DuckDuckGoTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildDuckDuckGoReferences(envelope)
}

func (d *DuckDuckGoTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildDuckDuckGoReferences(envelope)
	return map[string]any{
		"formalized_content": renderDuckDuckGoReferences(chunks, duckduckgoPromptMaxTokens),
		"json":               results,
	}
}

func buildDuckDuckGoReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := duckduckgoDataImagePattern.ReplaceAllString(duckduckgoText(item["body"]), "")
		content = truncateDuckDuckGoRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(duckduckgoHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(duckduckgoHashInt(documentID, 500), 10)
		title := duckduckgoText(item["title"])
		resultURL := duckduckgoText(item["url"])
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

func renderDuckDuckGoReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := duckduckgoText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + duckduckgoText(chunk["id"]),
			"├── Title: " + duckduckgoNewlinePattern.ReplaceAllString(duckduckgoText(chunk["document_name"]), " "),
			"├── URL: " + duckduckgoNewlinePattern.ReplaceAllString(duckduckgoText(chunk["url"]), " "),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func duckduckgoText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func duckduckgoHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateDuckDuckGoRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
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
		title := normalizeWhitespace(html.UnescapeString(item.Title))
		resultURL := strings.TrimSpace(item.URL)
		body := normalizeWhitespace(html.UnescapeString(item.Excerpt))
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
}, topN int) ([]duckduckgoResult, error) {
	if topN <= 0 {
		topN = 10
	}
	doc, err := xhtml.Parse(body)
	if err != nil {
		return nil, err
	}
	return extractDuckDuckGoGeneralResults(doc, topN), nil
}

func extractDuckDuckGoGeneralResults(doc *xhtml.Node, topN int) []duckduckgoResult {
	results := make([]duckduckgoResult, 0, topN)
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if len(results) >= topN {
			return
		}
		if n.Type == xhtml.ElementNode && hasClassToken(n, "result") {
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

func extractDuckDuckGoGeneralResult(card *xhtml.Node) (duckduckgoResult, bool) {
	var out duckduckgoResult
	titleNode := findFirstNode(card, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && n.Data == "a" && hasClassToken(n, "result__a")
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

func findFirstNode(root *xhtml.Node, match func(*xhtml.Node) bool) *xhtml.Node {
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

func textByClass(root *xhtml.Node, className string) string {
	node := findFirstNode(root, func(n *xhtml.Node) bool {
		return n.Type == xhtml.ElementNode && hasClassToken(n, className)
	})
	if node == nil {
		return ""
	}
	return collectText(node)
}

func hasClassToken(n *xhtml.Node, want string) bool {
	if n == nil || n.Type != xhtml.ElementNode || want == "" {
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

func attrValue(n *xhtml.Node, key string) string {
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
