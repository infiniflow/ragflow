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
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const akshareToolName = "akshare_stock_news"

const akshareToolDescription = "Retrieves the latest East Money news articles for a Chinese A-share stock symbol."

const defaultAkShareTopN = 10

const maxAkShareResponseBytes = 4 << 20

var akshareStockNewsEndpoint = "https://search-api-web.eastmoney.com/search/jsonp"

// akshareParams is the JSON shape the model sends into InvokableRun.
type akshareParams struct {
	Query  string `json:"query"`
	Symbol string `json:"symbol,omitempty"`
	TopN   int    `json:"top_n,omitempty"`
}

func (p akshareParams) stockSymbol() string {
	if query := strings.TrimSpace(p.Query); query != "" {
		return query
	}
	return strings.TrimSpace(p.Symbol)
}

type akshareArticle struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	PublishedAt string `json:"published_at"`
	Source      string `json:"source"`
	URL         string `json:"url"`
}

type akshareEnvelope struct {
	Content  string           `json:"content,omitempty"`
	Articles []akshareArticle `json:"articles,omitempty"`
	Error    string           `json:"_ERROR,omitempty"`
}

type akshareSearchRequest struct {
	UID           string         `json:"uid"`
	Keyword       string         `json:"keyword"`
	Type          []string       `json:"type"`
	Client        string         `json:"client"`
	ClientType    string         `json:"clientType"`
	ClientVersion string         `json:"clientVersion"`
	Param         map[string]any `json:"param"`
}

type akshareSearchResponse struct {
	Result struct {
		Articles []akshareEastMoneyArticle `json:"cmsArticleWebOld"`
	} `json:"result"`
}

type akshareEastMoneyArticle struct {
	Code      string `json:"code"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Date      string `json:"date"`
	MediaName string `json:"mediaName"`
}

// AkShareTool retrieves East Money stock news using the same endpoint
// as AkShare's stock_news_em(symbol=...) helper.
type AkShareTool struct {
	helper *HTTPHelper
	topN   int
}

func NewAkShareTool() *AkShareTool {
	return NewAkShareToolWith(NewHTTPHelper())
}

func NewAkShareToolWith(h *HTTPHelper) *AkShareTool {
	return NewAkShareToolWithTopN(h, defaultAkShareTopN)
}

func NewAkShareToolWithTopN(h *HTTPHelper, topN int) *AkShareTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if topN <= 0 {
		topN = defaultAkShareTopN
	}
	return &AkShareTool{helper: h, topN: topN}
}

// Info returns the tool's metadata for the chat model.
func (a *AkShareTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: akshareToolName,
		Desc: akshareToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Stock symbol/code to fetch East Money news for, e.g. 600519.",
				Required: true,
			},
		}),
	}, nil
}

// InvokableRun fetches stock news from East Money and returns both a
// formatted content string and structured article records.
func (a *AkShareTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p akshareParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return akshareErrJSON(fmt.Errorf("akshare: parse arguments: %w", err)),
			fmt.Errorf("akshare: parse arguments: %w", err)
	}

	symbol := p.stockSymbol()
	if symbol == "" {
		return akshareErrJSON(fmt.Errorf("akshare: query is required")),
			fmt.Errorf("akshare: query is required")
	}

	topN := a.topN
	if p.TopN > 0 {
		topN = p.TopN
	}
	if topN <= 0 {
		topN = defaultAkShareTopN
	}

	endpoint, err := buildAkShareStockNewsURL(symbol, topN)
	if err != nil {
		return akshareErrJSON(err), err
	}
	headers := map[string]string{
		"Accept":     "*/*",
		"Referer":    "https://so.eastmoney.com/news/s?keyword=" + url.QueryEscape(symbol),
		"User-Agent": "Mozilla/5.0 (compatible; ragflow/1.0)",
	}

	resp, err := a.helper.Do(ctx, http.MethodGet, endpoint, "", "", headers)
	if err != nil {
		return akshareErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("akshare: upstream returned %d", resp.StatusCode)
		return akshareErrJSON(err), err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAkShareResponseBytes+1))
	if err != nil {
		err = fmt.Errorf("akshare: read response: %w", err)
		return akshareErrJSON(err), err
	}
	if len(body) > maxAkShareResponseBytes {
		err = fmt.Errorf("akshare: response too large")
		return akshareErrJSON(err), err
	}

	articles, err := parseAkShareStockNews(body, topN)
	if err != nil {
		return akshareErrJSON(err), err
	}

	env := akshareEnvelope{
		Content:  formatAkShareArticles(articles),
		Articles: articles,
	}
	return akshareJSON(env), nil
}

func buildAkShareStockNewsURL(symbol string, topN int) (string, error) {
	if topN <= 0 {
		topN = defaultAkShareTopN
	}
	inner := akshareSearchRequest{
		Keyword:       symbol,
		Type:          []string{"cmsArticleWebOld"},
		Client:        "web",
		ClientType:    "web",
		ClientVersion: "curr",
		Param: map[string]any{
			"cmsArticleWebOld": map[string]any{
				"searchScope": "default",
				"sort":        "default",
				"pageIndex":   1,
				"pageSize":    topN,
				"preTag":      "<em>",
				"postTag":     "</em>",
			},
		},
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return "", fmt.Errorf("akshare: build request: %w", err)
	}
	callback := fmt.Sprintf("jQuery%d_%d", time.Now().UnixNano(), time.Now().UnixMilli())
	q := url.Values{}
	q.Set("cb", callback)
	q.Set("param", string(innerJSON))
	q.Set("_", fmt.Sprint(time.Now().UnixMilli()))
	return akshareStockNewsEndpoint + "?" + q.Encode(), nil
}

func parseAkShareStockNews(body []byte, topN int) ([]akshareArticle, error) {
	if topN <= 0 {
		topN = defaultAkShareTopN
	}

	rawJSON, err := stripJSONP(string(body))
	if err != nil {
		return nil, err
	}

	var raw akshareSearchResponse
	if err := json.Unmarshal([]byte(rawJSON), &raw); err != nil {
		return nil, fmt.Errorf("akshare: decode response: %w", err)
	}

	if topN > len(raw.Result.Articles) {
		topN = len(raw.Result.Articles)
	}

	articles := make([]akshareArticle, 0, topN)
	for _, item := range raw.Result.Articles[:topN] {
		articleURL := ""
		if code := strings.TrimSpace(item.Code); code != "" {
			articleURL = "http://finance.eastmoney.com/a/" + code + ".html"
		}
		articles = append(articles, akshareArticle{
			Title:       cleanAkShareText(item.Title),
			Content:     cleanAkShareText(item.Content),
			PublishedAt: strings.TrimSpace(item.Date),
			Source:      strings.TrimSpace(item.MediaName),
			URL:         articleURL,
		})
	}
	return articles, nil
}

func stripJSONP(s string) (string, error) {
	s = strings.TrimSpace(s)
	start := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if start < 0 || end <= start {
		if strings.HasPrefix(s, "{") {
			return s, nil
		}
		return "", fmt.Errorf("akshare: invalid JSONP response")
	}
	return strings.TrimSpace(s[start+1 : end]), nil
}

func cleanAkShareText(s string) string {
	replacements := []struct {
		old string
		new string
	}{
		{old: "(<em>", new: ""},
		{old: "</em>)", new: ""},
		{old: "<em>", new: ""},
		{old: "</em>", new: ""},
		{old: "\u3000", new: ""},
		{old: "\r\n", new: " "},
	}
	for _, repl := range replacements {
		s = strings.ReplaceAll(s, repl.old, repl.new)
	}
	return strings.TrimSpace(s)
}

// formatAkShareArticles renders articles into a human-readable block.
// NOTE: the upstream Python PR's _invoke dropped 文章来源 from the
// format string (5 args but only 4 {} placeholders); we preserve all
// five fields here (link, title, content, date, source).
func formatAkShareArticles(articles []akshareArticle) string {
	if len(articles) == 0 {
		return ""
	}
	parts := make([]string, 0, len(articles))
	for _, item := range articles {
		parts = append(parts, fmt.Sprintf(
			`<a href="%s">%s</a>
 新闻内容: %s 
发布时间:%s 
文章来源: %s`,
			item.URL,
			item.Title,
			item.Content,
			item.PublishedAt,
			item.Source,
		))
	}
	return strings.Join(parts, "\n\n")
}

func akshareJSON(env akshareEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"akshare: marshal result: %s"}`, err)
	}
	return string(b)
}

func akshareErrJSON(err error) string {
	return akshareJSON(akshareEnvelope{Error: err.Error()})
}
