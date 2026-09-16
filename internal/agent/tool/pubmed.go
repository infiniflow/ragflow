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
	"encoding/xml"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/tokenizer"
)

const (
	pubmedToolName        = "pubmed_search"
	pubmedToolDescription = "Search PubMed for life sciences and biomedical references."
	defaultPubMedTopN     = 12
	defaultPubMedEmail    = "A.N.Other@example.com"
	pubmedPromptMaxTokens = 200000
)

var pubmedDataImagePattern = regexp.MustCompile(`!?\[[a-z]+\]\(data:image/png;base64,[ 0-9A-Za-z/_=+\-]+\)`)

var pubmedNewlinePattern = regexp.MustCompile(`\n+`)

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

var _ ToolComponent = (*PubMedTool)(nil)
var _ ReferenceBuilder = (*PubMedTool)(nil)

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

// Info exposes only query to the LLM. Node parameters belong to canvas setup,
// not model-emitted runtime arguments.
func (p *PubMedTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: pubmedToolName,
		Desc: pubmedToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type:     schema.String,
				Desc:     "The search keywords to execute with PubMed. The keywords should be the most important words or terms, including synonyms, from the original request.",
				Required: true,
			},
		}),
	}, nil
}

func (p *PubMedTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{"query": "PubMed search query."},
		Outputs: map[string]string{
			"formalized_content": "Rendered PubMed references for downstream prompts.",
			"json":               "PubMed result list.",
		},
		InputForm: map[string]any{
			"query": map[string]any{"name": "Query", "type": "line"},
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

func (p *PubMedTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var params pubmedParams
	if err := json.Unmarshal([]byte(argsJSON), &params); err != nil {
		err = fmt.Errorf("pubmed: parse arguments: %w", err)
		return pubmedErrJSON(err), err
	}
	params = mergePubMedDefaults(p.defaults, params)
	params.Query = strings.TrimSpace(params.Query)
	if params.Query == "" {
		return pubmedJSON(pubmedEnvelope{Results: []pubmedResult{}}), nil
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

func (p *PubMedTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildPubMedReferences(envelope)
}

func (p *PubMedTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "results")
	chunks, _ := buildPubMedReferences(envelope)
	return map[string]any{
		"formalized_content": renderPubMedReferences(chunks, pubmedPromptMaxTokens),
		"json":               results,
	}
}

func buildPubMedReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	results := envelopeSlice(envelope, "results")
	chunks := make([]map[string]any, 0, len(results))
	docAggs := make([]map[string]any, 0, len(results))
	for _, result := range results {
		item, ok := result.(map[string]any)
		if !ok {
			continue
		}
		content := pubmedDataImagePattern.ReplaceAllString(pubmedText(item["content"]), "")
		content = truncatePubMedRunes(content, 10000)
		if content == "" {
			continue
		}
		documentID := strconv.FormatInt(pubmedHashInt(content, 100000000), 10)
		displayID := strconv.FormatInt(pubmedHashInt(documentID, 500), 10)
		title := pubmedText(item["title"])
		resultURL := pubmedText(item["url"])
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

func renderPubMedReferences(chunks []map[string]any, maxTokens int) string {
	usedTokens := 0
	blocks := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		content := pubmedText(chunk["content"])
		usedTokens += tokenizer.NumTokensFromString(content)
		blocks = append(blocks, strings.Join([]string{
			"\nID: " + pubmedText(chunk["id"]),
			"├── Title: " + pubmedNewlinePattern.ReplaceAllString(pubmedText(chunk["document_name"]), " "),
			"├── URL: " + pubmedNewlinePattern.ReplaceAllString(pubmedText(chunk["url"]), " "),
			"└── Content:\n" + content,
		}, "\n"))
		if maxTokens > 0 && float64(maxTokens)*0.97 < float64(usedTokens) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func pubmedText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func pubmedHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func truncatePubMedRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
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
