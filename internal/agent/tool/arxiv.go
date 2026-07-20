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
	"crypto/sha1"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
)

const arxivToolName = "arxiv_search"

const arxivToolDescription = "Search arXiv and return matching preprints as {title, authors, summary, pdf_url, entry_id}."

const defaultArxivTopN = 12

const defaultArxivSortBy = "submittedDate"

const arxivPromptMaxTokens = 200000

var arxivDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var arxivNewlinePattern = regexp.MustCompile(`\n+`)

// arxivParams carries the query and ArXiv search settings. Info exposes only
// query to match Python's tool meta; top_n and sort_by come from node params.
type arxivParams struct {
	Query  string `json:"query"`
	TopN   int    `json:"top_n"`
	SortBy string `json:"sort_by"`
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
	helper   *HTTPHelper
	defaults arxivParams
}

var _ ToolComponent = (*ArxivTool)(nil)
var _ ReferenceBuilder = (*ArxivTool)(nil)

// NewArxivTool returns an ArxivTool using the default HTTPHelper.
func NewArxivTool() *ArxivTool {
	return NewArxivToolWith(NewHTTPHelper())
}

// NewArxivToolWith returns an ArxivTool that uses the provided
// HTTPHelper. Useful for tests.
func NewArxivToolWith(h *HTTPHelper) *ArxivTool {
	return NewArxivToolWithParams(h, defaultArxivTopN, defaultArxivSortBy)
}

// NewArxivToolWithParams returns an ArxivTool with node-level search
// settings. Query remains the only model-provided argument.
func NewArxivToolWithParams(h *HTTPHelper, topN int, sortBy string) *ArxivTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if topN <= 0 {
		topN = defaultArxivTopN
	}
	if sortBy == "" {
		sortBy = defaultArxivSortBy
	}
	return &ArxivTool{helper: h, defaults: arxivParams{TopN: topN, SortBy: sortBy}}
}

// ToolMeta returns the tool's metadata for the chat model.
func (a *ArxivTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        arxivToolName,
		Description: arxivToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "The search keywords to execute with arXiv. The keywords should be the most important words/terms(includes synonyms) from the original request.",
				Required:    true,
			},
			"max_results": {
				Type:        ParamTypeInteger,
				Description: "Maximum number of results to return. Defaults to 5.",
				Required:    false,
			},
			"start_date": {
				Type:        ParamTypeString,
				Description: "Filter results by start date (YYYYMMDD format).",
				Required:    false,
			},
			"end_date": {
				Type:        ParamTypeString,
				Description: "Filter results by end date (YYYYMMDD format).",
				Required:    false,
			},
			"authors": {
				Type:        ParamTypeString,
				Description: "Filter results by author name(s).",
				Required:    false,
			},
			"journal": {
				Type:        ParamTypeString,
				Description: "Filter results by journal name.",
				Required:    false,
			},
			"categories": {
				Type:        ParamTypeString,
				Description: "Filter results by arXiv categories (e.g., cs.AI, math.ST).",
				Required:    false,
			},
			"id_list": {
				Type:        ParamTypeString,
				Description: "Comma-separated list of arXiv IDs to retrieve.",
				Required:    false,
			},
			"sort_by": {
				Type:        ParamTypeString,
				Description: "Sort results by field: 'relevance', 'lastUpdatedDate', or 'submittedDate'.",
				Required:    false,
			},
			"sort_order": {
				Type:        ParamTypeString,
				Description: "Sort order: 'ascending' or 'descending'.",
				Required:    false,
			},
		},
	}
}

func (a *ArxivTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{"query": "Search query."},
		Outputs: map[string]string{
			"formalized_content": "Rendered arXiv references for downstream prompts.",
			"json":               "arXiv paper list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
		},
	}
}

// buildArxivURL constructs the ArXiv /api/query URL.
func buildArxivURL(query string, topN int, sortBy string) string {
	if topN <= 0 {
		topN = defaultArxivTopN
	}
	if sortBy == "" {
		sortBy = defaultArxivSortBy
	}
	q := url.Values{}
	q.Set("search_query", "all:"+query)
	q.Set("max_results", fmt.Sprintf("%d", topN))
	q.Set("sortBy", sortBy)
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
func (a *ArxivTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var p arxivParams
	if err := json.Unmarshal([]byte(argsJSON), &p); err != nil {
		return arxivErrJSON(fmt.Errorf("arxiv: parse arguments: %w", err)),
			fmt.Errorf("arxiv: parse arguments: %w", err)
	}
	p = mergeArxivDefaults(a.defaults, p)
	if strings.TrimSpace(p.Query) == "" {
		return arxivJSON(arxivEnvelope{Results: []arxivResult{}}), nil
	}
	if p.TopN <= 0 {
		return arxivErrJSON(fmt.Errorf("top_n must be a positive integer")),
			fmt.Errorf("arxiv: top_n must be a positive integer")
	}
	if !ArxivSortBySupported(p.SortBy) {
		return arxivErrJSON(fmt.Errorf("unsupported sort_by %q", p.SortBy)),
			fmt.Errorf("arxiv: unsupported sort_by %q", p.SortBy)
	}

	endpoint := buildArxivURL(strings.TrimSpace(p.Query), p.TopN, p.SortBy)
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

func (a *ArxivTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildArxivReferences(envelope)
}

func (a *ArxivTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildArxivReferences(envelope)
	return map[string]any{
		"formalized_content": renderArxivReferences(chunks, arxivPromptMaxTokens),
		"json":               results,
	}
}

func buildArxivReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		paper, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := arxivDataImagePattern.ReplaceAllString(arxivText(paper["summary"]), "")
		content = truncateArxivRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(arxivHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(arxivHashInt(documentID, 500), 10)
		title := arxivText(paper["title"])
		resultURL := arxivText(paper["pdf_url"])
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

func renderArxivReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := arxivText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + arxivText(chunk["id"]),
			"├── Title: " + arxivNewlinePattern.ReplaceAllString(arxivText(chunk["document_name"]), " "),
			"├── URL: " + arxivNewlinePattern.ReplaceAllString(arxivText(chunk["url"]), " "),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func arxivText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func arxivHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncateArxivRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func mergeArxivDefaults(defaults, p arxivParams) arxivParams {
	if p.TopN == 0 {
		p.TopN = defaults.TopN
	}
	if p.SortBy == "" {
		p.SortBy = defaults.SortBy
	}
	return p
}

// ArxivSortBySupported reports whether sortBy is accepted by the ArXiv API.
func ArxivSortBySupported(sortBy string) bool {
	switch sortBy {
	case "submittedDate", "lastUpdatedDate", "relevance":
		return true
	default:
		return false
	}
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
