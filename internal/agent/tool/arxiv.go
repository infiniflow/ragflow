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
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const arxivToolName = "arxiv"

const arxivToolDescription = "Search arXiv and return matching preprints as {title, authors, summary, pdf_url, entry_id}."

// arxivParams is the JSON shape the model sends into InvokableRun.
type arxivParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// arxivResult is one entry in the model-facing result list.
type arxivResult struct {
	Title   string   `json:"title"`
	Authors []string `json:"authors"`
	Summary string   `json:"summary"`
	PDFURL  string   `json:"pdf_url"`
	EntryID string   `json:"entry_id"`
}

// arxivEnvelope is the JSON shape the model sees.
type arxivEnvelope struct {
	Results []arxivResult `json:"results"`
	Error   string        `json:"_ERROR,omitempty"`
}

// arxivAtom is the upstream Atom 1.0 envelope. We only model the fields
// we care about. Authors are stored as a flat list of <name> elements;
// pdf_url is derived from the first <link> with rel="related" and
// type="application/pdf", falling back to the entry id.
type arxivAtom struct {
	XMLName xml.Name     `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string       `xml:"title"`
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	Title   string        `xml:"title"`
	ID      string        `xml:"id"`
	Summary string        `xml:"summary"`
	Authors []arxivAuthor `xml:"author"`
	Links   []arxivLink   `xml:"link"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

type arxivLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

// ArxivTool is the ArXiv academic search tool. It performs a GET
// against the public ArXiv API and parses the Atom XML response
// using the stdlib encoding/xml package.
type ArxivTool struct {
	helper *HTTPHelper
}

// NewArxivTool returns an ArxivTool using the default HTTPHelper.
func NewArxivTool() *ArxivTool {
	return NewArxivToolWith(NewHTTPHelper())
}

// NewArxivToolWith returns an ArxivTool that uses the provided
// HTTPHelper. Useful for tests.
func NewArxivToolWith(h *HTTPHelper) *ArxivTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &ArxivTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (a *ArxivTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: arxivToolName,
		Desc: arxivToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "Search query (matches the arXiv `all:` field).",
				Required: true,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of results to return. Defaults to 5.",
				Required: false,
			},
		}),
	}, nil
}

// buildArxivURL constructs the ArXiv /api/query URL.
func buildArxivURL(query string, maxResults int) string {
	if maxResults <= 0 {
		maxResults = 5
	}
	q := url.Values{}
	q.Set("search_query", "all:"+query)
	q.Set("max_results", fmt.Sprintf("%d", maxResults))
	return "http://export.arxiv.org/api/query?" + q.Encode()
}

// parseArxivAtom decodes the upstream Atom XML and returns the model-
// facing results. Exposed at package scope for unit testing with canned
// XML.
func parseArxivAtom(body []byte) ([]arxivResult, error) {
	var feed arxivAtom
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&feed); err != nil {
		return nil, fmt.Errorf("arxiv: decode atom: %w", err)
	}
	results := make([]arxivResult, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		authors := make([]string, 0, len(e.Authors))
		for _, a := range e.Authors {
			if a.Name != "" {
				authors = append(authors, a.Name)
			}
		}
		pdfURL := pickArxivPDF(e)
		results = append(results, arxivResult{
			Title:   normalizeArxivWhitespace(e.Title),
			Authors: authors,
			Summary: normalizeArxivWhitespace(e.Summary),
			PDFURL:  pdfURL,
			EntryID: e.ID,
		})
	}
	return results, nil
}

// pickArxivPDF returns the entry's PDF link, preferring the explicit
// rel="related" type="application/pdf" link and falling back to the
// canonical abs/.../pdf URL derived from the entry id.
func pickArxivPDF(e arxivEntry) string {
	for _, l := range e.Links {
		if l.Rel == "related" && l.Type == "application/pdf" && l.Href != "" {
			return l.Href
		}
	}
	// Fallback: derive from the abs id. The arXiv API returns ids like
	// "http://arxiv.org/abs/2501.12345v1"; we transform the abs path
	// to a /pdf/ path.
	id := e.ID
	if id == "" {
		return ""
	}
	u, err := url.Parse(id)
	if err != nil || u.Path == "" {
		return id
	}
	newPath := u.Path
	if i := strings.LastIndex(newPath, "abs"); i >= 0 {
		newPath = newPath[:i] + "pdf" + newPath[i+len("abs"):]
	} else {
		newPath = "/pdf" + newPath
	}
	u2 := *u
	u2.Path = newPath
	return u2.String()
}

// normalizeArxivWhitespace collapses internal whitespace and trims.
// The arXiv XML wraps long fields across multiple <p> blocks, leaving
// spurious newlines and double spaces when concatenated.
func normalizeArxivWhitespace(s string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(s), " "))
}

// InvokableRun performs the ArXiv search.
func (a *ArxivTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var p arxivParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return arxivErrJSON(fmt.Errorf("arxiv: parse arguments: %w", err)),
			fmt.Errorf("arxiv: parse arguments: %w", err)
	}
	if p.Query == "" {
		return arxivErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("arxiv: query is required")
	}

	endpoint := buildArxivURL(p.Query, p.MaxResults)
	resp, err := a.helper.Do(ctx, http.MethodGet, endpoint, "", "", nil)
	if err != nil {
		return arxivErrJSON(err), err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return arxivErrJSON(fmt.Errorf("arxiv: upstream returned %d", resp.StatusCode)),
			fmt.Errorf("arxiv: upstream returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return arxivErrJSON(fmt.Errorf("arxiv: read body: %w", err)),
			fmt.Errorf("arxiv: read body: %w", err)
	}

	results, err := parseArxivAtom(body)
	if err != nil {
		return arxivErrJSON(err), err
	}
	return arxivJSON(arxivEnvelope{Results: results}), nil
}

func arxivJSON(env arxivEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"arxiv: marshal result: %s"}`, err)
	}
	return string(b)
}

func arxivErrJSON(err error) string {
	return arxivJSON(arxivEnvelope{Error: err.Error()})
}
