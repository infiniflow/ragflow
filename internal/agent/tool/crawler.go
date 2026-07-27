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
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

const crawlerToolName = "web_crawler"

const crawlerToolDescription = "Crawls a web page and returns its extracted text content and links."

// crawlerArgs is the JSON shape the model sends in. query is the
// Python-compatible argument name; url is accepted for older Go
// callers. max_depth and max_pages are accepted for API symmetry with
// earlier Go callers, but the current implementation only supports
// depth=0 (single page fetch).
type crawlerArgs struct {
	Query    string `json:"query"`
	URL      string `json:"url,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
	MaxPages int    `json:"max_pages,omitempty"`
}

func (a crawlerArgs) targetURL() string {
	if url := strings.TrimSpace(a.Query); url != "" {
		return url
	}
	return strings.TrimSpace(a.URL)
}

// crawlerResult is the JSON envelope returned to the model. The shape
// mirrors the Python tool's `content` / `links` output.
type crawlerResult struct {
	URL     string   `json:"url"`
	Title   string   `json:"title,omitempty"`
	Content string   `json:"content,omitempty"`
	Links   []string `json:"links,omitempty"`
	Status  int      `json:"status,omitempty"`
	Error   string   `json:"_ERROR,omitempty"`
}

// Resolver validates a URL and returns the pinned IP for the host. The
// returned IP is dialed directly by HTTPHelper.DoPinned which defeats
// DNS rebinding.
type Resolver func(rawURL string) (host string, ip net.IP, err error)

// CrawlerTool is the Crawler tool. It fetches a single page
// (max_depth=0) via HTTPHelper and extracts text + links with
// golang.org/x/net/html.
type CrawlerTool struct {
	helper  *HTTPHelper
	resolve Resolver
}

// NewCrawlerTool returns a CrawlerTool using the default HTTPHelper.
func NewCrawlerTool() *CrawlerTool {
	return NewCrawlerToolWith(NewHTTPHelper())
}

// NewCrawlerToolWith returns a CrawlerTool that uses the provided HTTPHelper.
func NewCrawlerToolWith(h *HTTPHelper) *CrawlerTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &CrawlerTool{helper: h, resolve: ResolveAndValidate}
}

// WithResolver replaces the URL resolver with a custom function.
func (c *CrawlerTool) WithResolver(fn Resolver) *CrawlerTool {
	if fn != nil {
		c.resolve = fn
	}
	return c
}

// ToolMeta returns the tool's metadata for the chat model.
func (c *CrawlerTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        crawlerToolName,
		Description: crawlerToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "The absolute URL to crawl. Must include an http:// or https:// scheme.",
				Required:    true,
			},
			"max_depth": {
				Type:        ParamTypeInteger,
				Description: "Recursion depth. 0 only; >0 returns an error.",
				Required:    false,
			},
			"max_pages": {
				Type:        ParamTypeInteger,
				Description: "Maximum number of pages to fetch. Ignored (single page).",
				Required:    false,
			},
		},
	}
}

// ErrCrawlerDepthUnsupported is returned when the caller asks for max_depth>0.
var ErrCrawlerDepthUnsupported = errors.New(
	"crawler: max_depth > 0 is not supported; " +
		"use a single-page fetch (max_depth=0)",
)

// InvokableRun fetches a single page and returns extracted text + links.
func (c *CrawlerTool) InvokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args crawlerArgs
	if argumentsInJSON == "" {
		return crawlerStubResult(crawlerResult{Error: "arguments are required"}),
			errors.New("crawler: empty arguments")
	}
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return crawlerStubResult(crawlerResult{Error: "invalid JSON: " + err.Error()}),
			fmt.Errorf("crawler: parse arguments: %w", err)
	}

	targetURL := args.targetURL()
	if targetURL == "" {
		return crawlerStubResult(crawlerResult{Error: "query is required"}),
			errors.New("crawler: empty query")
	}
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		return crawlerStubResult(crawlerResult{URL: targetURL, Error: "url must be http or https"}),
			fmt.Errorf("crawler: unsupported url scheme: %s", targetURL)
	}
	if args.MaxDepth > 0 {
		return crawlerStubResult(crawlerResult{URL: targetURL, Error: ErrCrawlerDepthUnsupported.Error()}),
			ErrCrawlerDepthUnsupported
	}
	host, pinnedIP, resolveErr := c.resolve(targetURL)
	if resolveErr != nil {
		return crawlerStubResult(crawlerResult{URL: targetURL, Error: resolveErr.Error()}), resolveErr
	}

	resp, err := c.helper.DoPinned(ctx, http.MethodGet, targetURL, "", "", nil, host, pinnedIP)
	if err != nil {
		return crawlerStubResult(crawlerResult{URL: targetURL, Error: err.Error()}), err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return crawlerStubResult(crawlerResult{URL: targetURL, Status: resp.StatusCode, Error: "read body: " + err.Error()}),
			fmt.Errorf("crawler: read body: %w", err)
	}

	page, err := extractPage(body)
	if err != nil {
		return crawlerStubResult(crawlerResult{URL: targetURL, Status: resp.StatusCode, Error: err.Error()}),
			fmt.Errorf("crawler: extract: %w", err)
	}
	page.URL = targetURL
	page.Status = resp.StatusCode

	return crawlerJSON(page)
}

func extractPage(body []byte) (crawlerResult, error) {
	doc, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return crawlerResult{}, fmt.Errorf("parse html: %w", err)
	}

	var out crawlerResult
	var titleNodes []*html.Node
	var text strings.Builder

	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "script", "style", "noscript", "template", "svg":
				return
			case "head":
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					if c.Type == html.ElementNode && strings.EqualFold(c.Data, "title") {
						titleNodes = append(titleNodes, c)
					}
				}
				return
			case "title":
				titleNodes = append(titleNodes, n)
			case "a":
				for _, a := range n.Attr {
					if strings.EqualFold(a.Key, "href") {
						href := strings.TrimSpace(a.Val)
						if href != "" && !strings.HasPrefix(href, "#") {
							out.Links = append(out.Links, href)
						}
						break
					}
				}
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(t)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	for _, tn := range titleNodes {
		var t strings.Builder
		for c := tn.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				t.WriteString(c.Data)
			}
		}
		title := strings.TrimSpace(t.String())
		if title != "" {
			out.Title = title
			break
		}
	}

	out.Content = strings.Join(strings.Fields(text.String()), " ")
	return out, nil
}

func crawlerStubResult(r crawlerResult) string {
	b, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"crawler: marshal: %s"}`, err)
	}
	return string(b)
}

func crawlerJSON(r crawlerResult) (string, error) {
	b, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("crawler: marshal result: %w", err)
	}
	return string(b), nil
}
