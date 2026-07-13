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
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	pubmedToolName        = "pubmed"
	pubmedToolDescription = "Search PubMed for life sciences and biomedical references."
	defaultPubMedTopN     = 12
	defaultPubMedEmail    = "A.N.Other@example.com"
)

// pubmedParams mirrors Python PubMedParam. Info() still only exposes query to
// the LLM; top_n and email are canvas-side params merged with constructor
// defaults at runtime.
type pubmedParams struct {
	Query string `json:"query"`
	TopN  int    `json:"top_n"`
	Email string `json:"email"`
}

// pubmedResult is one PubMed reference rendered with the same fields and
// fallbacks as Python PubMed._format_pubmed_content.
type pubmedResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

type pubmedEnvelope struct {
	Results []pubmedResult `json:"results"`
	Error   string         `json:"_ERROR,omitempty"`
}

const pubmedUserAgent = "ragflow/1.0"

var pubmedESearchEndpoint = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi"

var pubmedEFetchEndpoint = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"

// PubMedTool queries NCBI E-utilities with esearch followed by efetch XML,
// which is the same retrieval path as the Python PubMed component.
type PubMedTool struct {
	helper   *HTTPHelper
	defaults pubmedParams
}

func NewPubMedTool() *PubMedTool {
	return NewPubMedToolWith(NewHTTPHelper())
}

func NewPubMedToolWith(h *HTTPHelper) *PubMedTool {
	return NewPubMedToolWithDefaults(h, pubmedParams{})
}

func NewPubMedToolWithDefaults(h *HTTPHelper, defaults pubmedParams) *PubMedTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.TopN == 0 {
		defaults.TopN = defaultPubMedTopN
	}
	if strings.TrimSpace(defaults.Email) == "" {
		defaults.Email = defaultPubMedEmail
	}
	return &PubMedTool{helper: h, defaults: defaults}
}

// ToolMeta returns the tool's metadata for the chat model.
func (p *PubMedTool) ToolMeta() ToolMeta {
	return ToolMeta{
		Name:        pubmedToolName,
		Description: pubmedToolDescription,
		Parameters: map[string]ParameterInfo{
			"query": {
				Type:        ParamTypeString,
				Description: "The search keywords to execute with PubMed. The keywords should be the most important words or terms, including synonyms, from the original request.",
				Required:    true,
			},
			"max_results": {
				Type:        ParamTypeInteger,
				Description: "Maximum number of records to return. Defaults to 5 (max 100 per request).",
				Required:    false,
			},
			"date_from": {
				Type:        ParamTypeString,
				Description: "Filter results by publication start date (YYYY/MM/DD format).",
				Required:    false,
			},
			"date_to": {
				Type:        ParamTypeString,
				Description: "Filter results by publication end date (YYYY/MM/DD format).",
				Required:    false,
			},
			"authors": {
				Type:        ParamTypeString,
				Description: "Filter results by author name(s).",
				Required:    false,
			},
		},
	}
}

func buildPubMedESearchURL(query string, topN int, email string) string {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("term", query)
	q.Set("retmax", strconv.Itoa(topN))
	q.Set("retmode", "json")
	q.Set("email", email)
	return pubmedESearchEndpoint + "?" + q.Encode()
}

func buildPubMedEFetchURL(pmids []string, email string) string {
	q := url.Values{}
	q.Set("db", "pubmed")
	q.Set("id", strings.Join(pmids, ","))
	q.Set("retmode", "xml")
	q.Set("email", email)
	return pubmedEFetchEndpoint + "?" + q.Encode()
}

type pubmedESearchResponse struct {
	ESearchResult struct {
		IDList []string `json:"idlist"`
	} `json:"esearchresult"`
}

type pubmedXMLAuthor struct {
	LastName string `xml:"LastName"`
	ForeName string `xml:"ForeName"`
}

type pubmedXMLArticleID struct {
	Type  string `xml:"IdType,attr"`
	Value string `xml:",chardata"`
}

type pubmedXMLArticle struct {
	PMID    string `xml:"MedlineCitation>PMID"`
	Article struct {
		Title    string `xml:"ArticleTitle"`
		Abstract struct {
			Text []string `xml:"AbstractText"`
		} `xml:"Abstract"`
		Journal struct {
			Title string `xml:"Title"`
			Issue struct {
				Volume string `xml:"Volume"`
				Number string `xml:"Issue"`
			} `xml:"JournalIssue"`
		} `xml:"Journal"`
		Pages struct {
			MedlinePgn string `xml:"MedlinePgn"`
		} `xml:"Pagination"`
		Authors []pubmedXMLAuthor `xml:"AuthorList>Author"`
	} `xml:"MedlineCitation>Article"`
	ArticleIDs []pubmedXMLArticleID `xml:"PubmedData>ArticleIdList>ArticleId"`
}

type pubmedXMLResponse struct {
	Articles []pubmedXMLArticle `xml:"PubmedArticle"`
}

// InvokableRun performs the two-step PubMed lookup.
func (p *PubMedTool) InvokableRun(ctx context.Context, argsJSON string) (string, error) {
	var params pubmedParams
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		err = fmt.Errorf("pubmed: parse arguments: %w", err)
		return pubmedErrJSON(err), err
	}
	params = mergePubMedDefaults(p.defaults, params)
	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		err := fmt.Errorf("pubmed: query is required")
		return pubmedErrJSON(err), err
	}

	headers := map[string]string{
		"User-Agent": pubmedUserAgent,
		"Accept":     "application/json",
	}
	searchResp, err := p.helper.Do(ctx, http.MethodGet, buildPubMedESearchURL(params.Query, params.TopN, params.Email), "", "", headers)
	if err != nil {
		return pubmedErrJSON(err), err
	}
	defer searchResp.Body.Close()
	if searchResp.StatusCode < http.StatusOK || searchResp.StatusCode >= http.StatusMultipleChoices {
		err := fmt.Errorf("pubmed: esearch returned %d", searchResp.StatusCode)
		return pubmedErrJSON(err), err
	}

	var searchBody pubmedESearchResponse
	if err := json.NewDecoder(searchResp.Body).Decode(&searchBody); err != nil {
		err = fmt.Errorf("pubmed: decode esearch: %w", err)
		return pubmedErrJSON(err), err
	}
	if len(searchBody.ESearchResult.IDList) == 0 {
		return pubmedJSON(pubmedEnvelope{Results: []pubmedResult{}}), nil
	}

	headers["Accept"] = "application/xml"
	fetchResp, err := p.helper.Do(ctx, http.MethodGet, buildPubMedEFetchURL(searchBody.ESearchResult.IDList, params.Email), "", "", headers)
	if err != nil {
		return pubmedErrJSON(err), err
	}
	defer fetchResp.Body.Close()
	if fetchResp.StatusCode < http.StatusOK || fetchResp.StatusCode >= http.StatusMultipleChoices {
		err := fmt.Errorf("pubmed: efetch returned %d", fetchResp.StatusCode)
		return pubmedErrJSON(err), err
	}

	var fetched pubmedXMLResponse
	if err := xml.NewDecoder(fetchResp.Body).Decode(&fetched); err != nil {
		err = fmt.Errorf("pubmed: decode efetch: %w", err)
		return pubmedErrJSON(err), err
	}
	results := make([]pubmedResult, 0, len(fetched.Articles))
	for _, article := range fetched.Articles {
		results = append(results, formatPubMedResult(article))
	}
	return pubmedJSON(pubmedEnvelope{Results: results}), nil
}

func formatPubMedResult(article pubmedXMLArticle) pubmedResult {
	title := fallbackPubMedField(article.Article.Title, "No title")
	abstract := fallbackPubMedField(strings.Join(article.Article.Abstract.Text, " "), "No abstract available")
	journal := fallbackPubMedField(article.Article.Journal.Title, "Unknown Journal")
	volume := fallbackPubMedField(article.Article.Journal.Issue.Volume, "-")
	issue := fallbackPubMedField(article.Article.Journal.Issue.Number, "-")
	pages := fallbackPubMedField(article.Article.Pages.MedlinePgn, "-")
	authors := formatPubMedAuthors(article.Article.Authors)
	doi := "-"
	for _, id := range article.ArticleIDs {
		if id.Type == "doi" && strings.TrimSpace(id.Value) != "" {
			doi = strings.TrimSpace(id.Value)
			break
		}
	}
	content := strings.Join([]string{
		"Title: " + title,
		"Authors: " + authors,
		"Journal: " + journal,
		"Volume: " + volume,
		"Issue: " + issue,
		"Pages: " + pages,
		"DOI: " + doi,
		"Abstract: " + abstract,
	}, "\n")
	return pubmedResult{
		Title:   title,
		URL:     "https://pubmed.ncbi.nlm.nih.gov/" + strings.TrimSpace(article.PMID),
		Content: content,
	}
}

func mergePubMedDefaults(defaults, params pubmedParams) pubmedParams {
	if params.Query == "" {
		params.Query = defaults.Query
	}
	if params.TopN == 0 {
		params.TopN = defaults.TopN
	}
	if strings.TrimSpace(params.Email) == "" {
		params.Email = defaults.Email
	}
	return params
}

func fallbackPubMedField(value, fallback string) string {
	if value = strings.TrimSpace(value); value != "" {
		return value
	}
	return fallback
}

func formatPubMedAuthors(authors []pubmedXMLAuthor) string {
	names := make([]string, 0, len(authors))
	for _, author := range authors {
		name := strings.TrimSpace(strings.TrimSpace(author.ForeName) + " " + strings.TrimSpace(author.LastName))
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "Unknown Authors"
	}
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
