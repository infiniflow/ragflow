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
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const pubmedToolName = "pubmed"

const pubmedToolDescription = "Search PubMed via NCBI E-utilities. Returns {pmid, title, authors, journal, year}."

// pubmedParams is the JSON shape the model sends into InvokableRun.
type pubmedParams struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

// pubmedResult is one row in the returned record list.
type pubmedResult struct {
	PMID    string `json:"pmid"`
	Title   string `json:"title"`
	Authors string `json:"authors"`
	Journal string `json:"journal"`
	Year    string `json:"year"`
}

// pubmedEnvelope is what the model sees.
type pubmedEnvelope struct {
	Results []pubmedResult `json:"results"`
	Error   string         `json:"_ERROR,omitempty"`
}

// pubmedUserAgent is the User-Agent that NCBI requires for all
// E-utilities requests. Without it, requests are silently dropped
// or rate-limited to a single IP.
const pubmedUserAgent = "ragflow/1.0"

// pubmedESearchEndpoint is the E-utilities esearch URL. Exposed as a
// package var so tests can substitute a httptest.Server URL.
var pubmedESearchEndpoint = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi"

// pubmedESummaryEndpoint is the E-utilities esummary URL. Exposed
// as a package var so tests can substitute a httptest.Server URL.
var pubmedESummaryEndpoint = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esummary.fcgi"

// PubMedTool is the PubMed
// search tool. It uses NCBI
// E-utilities: esearch returns a list of PMIDs, then esummary fetches
// the full records for those PMIDs.
type PubMedTool struct {
	helper *HTTPHelper
}

// NewPubMedTool returns a PubMedTool using the default HTTPHelper.
func NewPubMedTool() *PubMedTool {
	return NewPubMedToolWith(NewHTTPHelper())
}

// NewPubMedToolWith returns a PubMedTool that uses the provided
// HTTPHelper. Useful for tests.
func NewPubMedToolWith(h *HTTPHelper) *PubMedTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	return &PubMedTool{helper: h}
}

// Info returns the tool's metadata for the chat model.
func (p *PubMedTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: pubmedToolName,
		Desc: pubmedToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "PubMed search query (full PubMed query syntax supported).",
				Required: true,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Maximum number of records to return. Defaults to 5 (max 100 per request).",
				Required: false,
			},
		}),
	}, nil
}

// buildPubMedESearchURL composes the esearch URL. Centralized for
// testability.
func buildPubMedESearchURL(query string, maxResults int) string {
	if maxResults <= 0 {
		maxResults = 5
	}
	if maxResults > 100 {
		maxResults = 100
	}
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("term", query)
	q.Set("retmax", strconv.Itoa(maxResults))
	q.Set("retmode", "json")
	return pubmedESearchEndpoint + "?" + q.Encode()
}

// buildPubMedESummaryURL composes the esummary URL for a list of
// PMIDs. Centralized for testability.
func buildPubMedESummaryURL(pmids []string) string {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("id", strings.Join(pmids, ","))
	q.Set("retmode", "json")
	return pubmedESummaryEndpoint + "?" + q.Encode()
}

// pubmedESearchResponse is the upstream esearch envelope.
type pubmedESearchResponse struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

// pubmedESummaryAuthor is one author in the esummary author list.
type pubmedESummaryAuthor struct {
	Name string `json:"name"`
}

// pubmedESummaryArticle is one article in the esummary result map.
type pubmedESummaryArticle struct {
	Title           string                 `json:"title"`
	Authors         []pubmedESummaryAuthor `json:"authors"`
	FullJournalName string                 `json:"fulljournalname"`
	PubDate         string                 `json:"pubdate"`
}

// pubmedESummaryResponse is the upstream esummary envelope. The
// `result` value mixes per-PMID article objects with a string-list
// `uids` key, so we decode it as a map of RawMessage and then walk
// the entries to extract the article objects (skipping the `uids`
// array). See decodePubMedESummary.
type pubmedESummaryResponse struct {
	Result map[string]json.RawMessage `json:"result"`
}

// mustReadAll is a tiny helper to read the entire response body. We
// keep it private because pubmed.go owns the only two-step HTTP dance.
func mustReadAll(r io.Reader) []byte {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil
	}
	return b
}

// decodePubMedESummary parses the raw esummary response and returns
// a PMID → article map. The upstream response is a flat object whose
// keys are PMIDs (article values) plus a `uids` key (string list).
// A single JSON decode into map[string]Article would fail on `uids`
// because Go's encoding/json cannot store arrays in a struct-typed
// map; the RawMessage indirection sidesteps that.
func decodePubMedESummary(body []byte) (map[string]pubmedESummaryArticle, error) {
	var raw pubmedESummaryResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]pubmedESummaryArticle, len(raw.Result))
	for k, v := range raw.Result {
		// The `uids` key is a JSON array of strings; skip it.
		if len(v) > 0 && v[0] == '[' {
			continue
		}
		var article pubmedESummaryArticle
		if err := json.Unmarshal(v, &article); err != nil {
			continue
		}
		out[k] = article
	}
	return out, nil
}

// InvokableRun performs the two-step PubMed lookup.
func (p *PubMedTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var params pubmedParams
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		return pubmedErrJSON(fmt.Errorf("pubmed: parse arguments: %w", err)),
			fmt.Errorf("pubmed: parse arguments: %w", err)
	}
	if strings.TrimSpace(params.Query) == "" {
		return pubmedErrJSON(fmt.Errorf("query is required")),
			fmt.Errorf("pubmed: query is required")
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 5
	}

	headers := map[string]string{
		"User-Agent": pubmedUserAgent,
		"Accept":     "application/json",
	}

	// Step 1: esearch → list of PMIDs
	searchURL := buildPubMedESearchURL(params.Query, params.MaxResults)
	searchResp, err := p.helper.Do(ctx, http.MethodGet, searchURL, "", "", headers)
	if err != nil {
		return pubmedErrJSON(err), err
	}
	var searchBody pubmedESearchResponse
	if decErr := json.NewDecoder(searchResp.Body).Decode(&searchBody); decErr != nil {
		_ = searchResp.Body.Close()
		return pubmedErrJSON(fmt.Errorf("pubmed: decode esearch: %w", decErr)),
			fmt.Errorf("pubmed: decode esearch: %w", decErr)
	}
	_ = searchResp.Body.Close()
	if searchResp.StatusCode < 200 || searchResp.StatusCode >= 300 {
		return pubmedErrJSON(fmt.Errorf("pubmed: esearch returned %d", searchResp.StatusCode)),
			fmt.Errorf("pubmed: esearch returned %d", searchResp.StatusCode)
	}

	pmids := searchBody.ESearchResult.IDList
	if len(pmids) == 0 {
		return pubmedJSON(pubmedEnvelope{Results: []pubmedResult{}}), nil
	}

	// Step 2: esummary → full records
	summaryURL := buildPubMedESummaryURL(pmids)
	summaryResp, err := p.helper.Do(ctx, http.MethodGet, summaryURL, "", "", headers)
	if err != nil {
		return pubmedErrJSON(err), err
	}
	defer summaryResp.Body.Close()
	if summaryResp.StatusCode < 200 || summaryResp.StatusCode >= 300 {
		return pubmedErrJSON(fmt.Errorf("pubmed: esummary returned %d", summaryResp.StatusCode)),
			fmt.Errorf("pubmed: esummary returned %d", summaryResp.StatusCode)
	}

	articles, err := decodePubMedESummary(mustReadAll(summaryResp.Body))
	if err != nil {
		return pubmedErrJSON(fmt.Errorf("pubmed: parse esummary: %w", err)),
			fmt.Errorf("pubmed: parse esummary: %w", err)
	}

	results := make([]pubmedResult, 0, len(pmids))
	for _, pmid := range pmids {
		article, ok := articles[pmid]
		if !ok {
			continue
		}
		results = append(results, pubmedResult{
			PMID:    pmid,
			Title:   strings.TrimSpace(article.Title),
			Authors: joinAuthorNames(article.Authors),
			Journal: article.FullJournalName,
			Year:    firstFourDigitYear(article.PubDate),
		})
	}
	return pubmedJSON(pubmedEnvelope{Results: results}), nil
}

// joinAuthorNames joins the first N authors with ", " and adds
// "et al." for any beyond N. We use N=3 to mirror the convention
// common in academic citation styles.
func joinAuthorNames(authors []pubmedESummaryAuthor) string {
	const cap = 3
	if len(authors) == 0 {
		return ""
	}
	if len(authors) <= cap {
		names := make([]string, len(authors))
		for i, a := range authors {
			names[i] = a.Name
		}
		return strings.Join(names, ", ")
	}
	names := make([]string, 0, cap+1)
	for i := range cap {
		names = append(names, authors[i].Name)
	}
	names = append(names, "et al.")
	return strings.Join(names, ", ")
}

func pubmedJSON(env pubmedEnvelope) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"pubmed: marshal result: %s"}`, err)
	}
	return string(b)
}

func pubmedErrJSON(err error) string {
	return pubmedJSON(pubmedEnvelope{Error: err.Error()})
}
