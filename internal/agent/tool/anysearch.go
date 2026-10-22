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
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	neturl "net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"ragflow/internal/tokenizer"
)

const (
	anySearchToolName         = "anysearch_search"
	anySearchDiscoverToolName = "anysearch_discover"
	anySearchBaseURL          = "https://api.anysearch.com/v1"
	// anySearchMaxResults is the most results AnySearch returns for one search.
	anySearchMaxResults = 10
	// anySearchMaxQueries is the most independent searches one call may run.
	anySearchMaxQueries = 5
	// anySearchMaxResponseBytes bounds every upstream body read.
	anySearchMaxResponseBytes = 4 << 20
	// anySearchResultTextRunes caps each result's text so five queries of ten
	// results cannot flood the model's context.
	anySearchResultTextRunes = 1500
	// anySearchResultNestedBytes drops a nested result field larger than this.
	anySearchResultNestedBytes = 2048
	// anySearchMessageRunes caps the vendor message copied into an error.
	anySearchMessageRunes    = 200
	anySearchPromptMaxTokens = 200000
)

var anySearchZones = []string{"cn", "intl"}

const anySearchToolDescription = `AnySearch searches the web, or a specialised vertical such as finance, academic papers, code, legal or health, through one API. When searching:
   - Use a focused query of the most important terms (and synonyms).
   - Leave tag out for a general web search. For a vertical search call anysearch_discover first to learn the current tags and the params each one needs, then pass tag and params exactly as discovered.
   - To run several independent searches in parallel, put them in queries (1 to 5 items). Every result list stays attached to its own query, and one failing query does not lose the others.
   - zone (cn or intl), language and max_results are optional; leave them out to use the node's settings.`

const anySearchItemProperties = `
      "query":       {"type": "string", "description": "The search keywords. The most important words/terms (and synonyms) from the original request."},
      "tag":         {"type": "string", "description": "Vertical capability tag such as code.doc or finance.quote, exactly as returned by anysearch_discover. Leave it out for a general web search."},
      "params":      {"type": "object", "description": "Parameters the chosen tag declares, as an object. Pass every required one, with an empty string when it does not apply. Only valid together with tag."},
      "zone":        {"type": "string", "enum": ["cn", "intl"], "description": "Region preference. Leave it out to use the node's zone."},
      "language":    {"type": "string", "description": "Preferred result language such as en or zh-CN. Leave it out to use the node's language."},
      "max_results": {"type": "integer", "minimum": 1, "maximum": 10, "description": "Number of results, 1 to 10. Leave it out to use the Top N set on the node."}`

// anySearchItem is one search as the model sends it, either at the top level of
// the arguments or inside queries. max_results is decoded loosely: a missing,
// blank or non-numeric value means "use the node's Top N" rather than failing.
type anySearchItem struct {
	Query      string `json:"query"`
	Tag        string `json:"tag"`
	Params     any    `json:"params"`
	Zone       string `json:"zone"`
	Language   string `json:"language"`
	MaxResults any    `json:"max_results"`
}

// hasSearchFields reports whether any field other than query carries a value.
// Blank values (an empty params object, a blank max_results) are what a form
// sends for fields left unset, so they do not count.
func (i anySearchItem) hasSearchFields() bool {
	hasParams := i.Params != nil
	if params, ok := i.Params.(map[string]any); ok {
		hasParams = len(params) > 0
	}
	hasMaxResults := i.MaxResults != nil
	if text, ok := i.MaxResults.(string); ok {
		hasMaxResults = strings.TrimSpace(text) != ""
	}
	return strings.TrimSpace(i.Tag) != "" || hasParams || strings.TrimSpace(i.Zone) != "" ||
		strings.TrimSpace(i.Language) != "" || hasMaxResults
}

type anySearchArgs struct {
	anySearchItem
	Queries []anySearchItem `json:"queries"`
}

// anySearchDefaults holds the node-level configuration. The model's zone,
// language and max_results, when given, override the node's.
type anySearchDefaults struct {
	TopN     int
	Zone     string
	Language string
}

type anySearchRequestBody struct {
	Query      string         `json:"query"`
	Tag        string         `json:"tag,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
	Zone       string         `json:"zone,omitempty"`
	Language   string         `json:"language,omitempty"`
	MaxResults int            `json:"max_results"`
}

// anySearchQueryResult is one search's outcome. request_id is kept for
// diagnostics, including on failures.
type anySearchQueryResult struct {
	Index     int              `json:"index"`
	Query     string           `json:"query"`
	Tag       string           `json:"tag,omitempty"`
	RequestID string           `json:"request_id,omitempty"`
	Results   []map[string]any `json:"results"`
	Error     string           `json:"_ERROR,omitempty"`
}

// anySearchSearchEnvelope is what the model and the Canvas component see. The
// top-level _ERROR is set only when the call as a whole failed; a partial
// failure keeps the successful queries and their references intact.
type anySearchSearchEnvelope struct {
	Queries []anySearchQueryResult `json:"queries"`
	Error   string                 `json:"_ERROR,omitempty"`
}

// anySearchError is a failed AnySearch call. Its text is safe to hand to the
// model: it carries the HTTP status or business code, the vendor message
// (redacted and truncated) and the request id, never the key or the raw body.
type anySearchError struct {
	message   string
	requestID string
}

func (e *anySearchError) Error() string { return e.message }

// anySearchClient calls the AnySearch REST API. The API key is optional: a blank
// key means anonymous access with no Authorization header at all.
type anySearchClient struct {
	helper *HTTPHelper
	apiKey string
}

// newAnySearchClient never follows redirects, so a Bearer key cannot be
// forwarded to another host.
func newAnySearchClient(h *HTTPHelper, apiKey string) *anySearchClient {
	if h == nil || h.client == nil {
		h = NewHTTPHelper()
	}
	client := *h.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	bounded := *h
	bounded.client = &client
	return &anySearchClient{helper: &bounded, apiKey: strings.TrimSpace(apiKey)}
}

type anySearchReply struct {
	data      map[string]any
	requestID string
}

// call performs one request and returns the validated envelope's data. A POST is
// never retried (the helper only retries idempotent methods).
func (c *anySearchClient) call(ctx context.Context, method, path string, body []byte) (*anySearchReply, error) {
	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": "RAGFlow anysearch-integration/infiniflow-ragflow",
	}
	contentType := ""
	if body != nil {
		contentType = "application/json"
	}
	if c.apiKey != "" {
		headers["Authorization"] = "Bearer " + c.apiKey
	}
	resp, err := c.helper.Do(ctx, method, anySearchBaseURL+path, string(body), contentType, headers)
	if err != nil {
		// Transport errors embed the request URL and, behind a proxy, its
		// credentials, so only the category is reported.
		var netErr net.Error
		switch {
		case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
			return nil, &anySearchError{message: "anysearch: request canceled"}
		case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()):
			return nil, &anySearchError{message: "anysearch: request timed out"}
		default:
			return nil, &anySearchError{message: "anysearch: request failed"}
		}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, anySearchMaxResponseBytes+1))
	if err != nil {
		return nil, &anySearchError{message: "anysearch: response unreadable"}
	}
	if len(raw) > anySearchMaxResponseBytes {
		return nil, &anySearchError{message: "anysearch: response exceeds 4 MiB"}
	}

	var envelope struct {
		Code      any `json:"code"`
		Message   any `json:"message"`
		RequestID any `json:"request_id"`
		Data      any `json:"data"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decodeErr := decoder.Decode(&envelope)
	if decodeErr == nil && decoder.Decode(&struct{}{}) != io.EOF {
		decodeErr = errors.New("trailing data")
	}
	if decodeErr != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, &anySearchError{message: fmt.Sprintf("anysearch: HTTP %d", resp.StatusCode)}
		}
		return nil, &anySearchError{message: fmt.Sprintf("anysearch: invalid JSON response (HTTP %d)", resp.StatusCode)}
	}
	requestID := c.safeText(envelope.RequestID, 64)
	message := c.safeText(envelope.Message, anySearchMessageRunes)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, anySearchFailure(fmt.Sprintf("HTTP %d", resp.StatusCode), message, requestID)
	}
	code, ok := anySearchIntegerCode(envelope.Code)
	if !ok {
		return nil, anySearchFailure("missing business status", "", requestID)
	}
	if code != 0 {
		return nil, anySearchFailure(fmt.Sprintf("business error %d", code), message, requestID)
	}
	data, ok := envelope.Data.(map[string]any)
	if !ok {
		return nil, anySearchFailure("missing data", "", requestID)
	}
	return &anySearchReply{data: data, requestID: requestID}, nil
}

func anySearchFailure(what, message, requestID string) *anySearchError {
	text := "anysearch: " + what
	if message != "" {
		text += ": " + message
	}
	if requestID != "" {
		text += " (request_id " + requestID + ")"
	}
	return &anySearchError{message: text, requestID: requestID}
}

// anySearchIntegerCode accepts only a JSON integer: a string, fraction or
// missing code is a contract violation, never success.
func anySearchIntegerCode(value any) (int64, bool) {
	number, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	code, err := strconv.ParseInt(number.String(), 10, 64)
	return code, err == nil
}

// safeText turns an upstream string into a short single-line value with the API
// key removed.
func (c *anySearchClient) safeText(value any, limit int) string {
	text, ok := value.(string)
	if !ok {
		return ""
	}
	if c.apiKey != "" {
		text = strings.ReplaceAll(text, c.apiKey, "[redacted]")
	}
	return truncateAnySearchRunes(strings.Join(strings.Fields(text), " "), limit, true)
}

func truncateAnySearchRunes(value string, limit int, ellipsis bool) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	cut := string([]rune(value)[:limit])
	if ellipsis {
		return cut + "..."
	}
	return cut
}

// AnySearchTool is the AnySearch search tool. It POSTs to /v1/search for one
// query or, in parallel, for up to five independent queries, and returns each
// query's results next to its own request id and error. The API key is node
// configuration only, is optional, and is never model-visible.
type AnySearchTool struct {
	client   *anySearchClient
	defaults anySearchDefaults
}

var _ ToolComponent = (*AnySearchTool)(nil)
var _ ReferenceBuilder = (*AnySearchTool)(nil)

// NewAnySearchTool returns an anonymous AnySearchTool with the default
// HTTPHelper.
func NewAnySearchTool() *AnySearchTool {
	return newAnySearchTool(nil, "", anySearchDefaults{})
}

// NewAnySearchToolWith returns an AnySearchTool that uses the provided
// HTTPHelper and API key. Useful for tests that want to inject a custom
// transport.
func NewAnySearchToolWith(h *HTTPHelper, apiKey string) *AnySearchTool {
	return newAnySearchTool(h, apiKey, anySearchDefaults{})
}

func newAnySearchTool(h *HTTPHelper, apiKey string, defaults anySearchDefaults) *AnySearchTool {
	if defaults.TopN <= 0 {
		defaults.TopN = anySearchMaxResults
	}
	return &AnySearchTool{client: newAnySearchClient(h, apiKey), defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (a *AnySearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {` + anySearchItemProperties + `,
    "queries": {
      "type": "array",
      "minItems": 1,
      "maxItems": ` + strconv.Itoa(anySearchMaxQueries) + `,
      "description": "Independent searches to run in parallel, 1 to 5. Use instead of query. Each item takes the same fields as the top level.",
      "items": {
        "type": "object",
        "properties": {` + anySearchItemProperties + `
        },
        "required": ["query"]
      }
    }
  }
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("anysearch: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        anySearchToolName,
		Desc:        anySearchToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun performs the AnySearch search.
//
// Recoverable errors (bad arguments and API or network failures) are returned
// as a JSON envelope with an _ERROR field and a nil Go error, so the ReAct
// agent feeds them back to the model instead of failing the run, matching
// Search1APITool.
func (a *AnySearchTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args anySearchArgs
	if strings.TrimSpace(argsJSON) == "" {
		argsJSON = "{}"
	}
	decoder := json.NewDecoder(strings.NewReader(argsJSON))
	decoder.UseNumber()
	err := decoder.Decode(&args)
	if err == nil && decoder.Decode(&struct{}{}) != io.EOF {
		err = errors.New("trailing data")
	}
	if err != nil {
		return anySearchErrJSON(fmt.Errorf("anysearch: parse arguments: %w", err)), nil
	}
	requests, err := a.requests(args)
	if err != nil {
		return anySearchErrJSON(err), nil
	}
	if len(requests) == 0 {
		return anySearchJSON(anySearchSearchEnvelope{Queries: []anySearchQueryResult{}}), nil
	}

	results := make([]anySearchQueryResult, len(requests))
	if len(requests) == 1 {
		results[0] = a.search(ctx, 0, requests[0])
	} else {
		var wg sync.WaitGroup
		for i, request := range requests {
			wg.Add(1)
			go func() {
				defer wg.Done()
				// A panic in a goroutine would end the whole server process, so it
				// becomes this query's error instead.
				defer func() {
					if recover() != nil {
						results[i] = anySearchQueryResult{Index: i, Query: request.Query, Tag: request.Tag, Results: []map[string]any{}, Error: "anysearch: internal error"}
					}
				}()
				results[i] = a.search(ctx, i, request)
			}()
		}
		wg.Wait()
	}

	envelope := anySearchSearchEnvelope{Queries: results}
	failed := 0
	for _, result := range results {
		if result.Error != "" {
			failed++
		}
	}
	if failed == len(results) {
		envelope.Error = results[0].Error
		if len(results) > 1 {
			envelope.Error = fmt.Sprintf("anysearch: all %d queries failed, see each query's _ERROR", len(results))
		}
	}
	return anySearchJSON(envelope), nil
}

// requests validates the arguments and builds one upstream request per search.
// An entirely empty call (no query and no other field) is a no-op, like the
// neighbouring search tools. A call that sets another field but forgets the
// query is an error, so the model is not told "no results" for a mistake.
func (a *AnySearchTool) requests(args anySearchArgs) ([]anySearchRequestBody, error) {
	if len(args.Queries) > 0 && strings.TrimSpace(args.Query) != "" {
		return nil, errors.New("anysearch: provide either query or queries, not both")
	}
	if len(args.Queries) > anySearchMaxQueries {
		return nil, fmt.Errorf("anysearch: queries must contain at most %d items", anySearchMaxQueries)
	}
	if len(args.Queries) > 0 && args.anySearchItem.hasSearchFields() {
		return nil, errors.New("anysearch: with queries, set tag, params, zone, language and max_results inside each item, not at the top level")
	}
	if args.Queries != nil && len(args.Queries) == 0 && strings.TrimSpace(args.Query) == "" {
		return nil, errors.New("anysearch: queries must contain at least one item")
	}
	if len(args.Queries) == 0 {
		if strings.TrimSpace(args.Query) == "" {
			if args.anySearchItem.hasSearchFields() {
				return nil, errors.New("anysearch: query is required")
			}
			return nil, nil
		}
		request, err := a.request("", args.anySearchItem)
		if err != nil {
			return nil, err
		}
		return []anySearchRequestBody{request}, nil
	}
	requests := make([]anySearchRequestBody, 0, len(args.Queries))
	for i, item := range args.Queries {
		request, err := a.request(fmt.Sprintf("queries[%d].", i), item)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func (a *AnySearchTool) request(label string, item anySearchItem) (anySearchRequestBody, error) {
	query := strings.TrimSpace(item.Query)
	if query == "" {
		return anySearchRequestBody{}, fmt.Errorf("anysearch: %squery is required", label)
	}
	tag := strings.TrimSpace(item.Tag)
	var params map[string]any
	switch p := item.Params.(type) {
	case nil:
	case map[string]any:
		params = p
	default:
		return anySearchRequestBody{}, fmt.Errorf("anysearch: %sparams must be an object", label)
	}
	if len(params) > 0 && tag == "" {
		return anySearchRequestBody{}, fmt.Errorf("anysearch: %sparams require a tag", label)
	}
	zone := strings.ToLower(strings.TrimSpace(item.Zone))
	if zone == "" {
		zone = a.defaults.Zone
	}
	if zone != "" && !slices.Contains(anySearchZones, zone) {
		return anySearchRequestBody{}, fmt.Errorf("anysearch: %szone %q is not supported, it should be one of %v", label, item.Zone, anySearchZones)
	}
	language := strings.TrimSpace(item.Language)
	if language == "" {
		language = a.defaults.Language
	}
	return anySearchRequestBody{
		Query:      query,
		Tag:        tag,
		Params:     params,
		Zone:       zone,
		Language:   language,
		MaxResults: anySearchMaxResultsArg(item.MaxResults, a.defaults.TopN),
	}, nil
}

// anySearchMaxResultsArg returns the model's result count if it gave a usable
// one, else the node's Top N, clamped to [1, 10].
func anySearchMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case json.Number:
		if i, err := v.Int64(); err == nil {
			n = int(i)
		} else if f, err := v.Float64(); err == nil {
			n = int(min(max(f, 1), anySearchMaxResults))
		}
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), anySearchMaxResults)
}

// search runs one query. Failures stay inside the returned record.
func (a *AnySearchTool) search(ctx context.Context, index int, request anySearchRequestBody) anySearchQueryResult {
	record := anySearchQueryResult{Index: index, Query: request.Query, Tag: request.Tag, Results: []map[string]any{}}
	body, _ := json.Marshal(request)
	reply, err := a.client.call(ctx, http.MethodPost, "/search", body)
	if err != nil {
		var apiErr *anySearchError
		if errors.As(err, &apiErr) {
			record.RequestID = apiErr.requestID
		}
		record.Error = err.Error()
		return record
	}
	record.RequestID = reply.requestID
	rawResults, ok := reply.data["results"].([]any)
	if !ok {
		record.Error = anySearchFailure("invalid results", "", reply.requestID).Error()
		return record
	}
	results := make([]map[string]any, 0, len(rawResults))
	for _, raw := range rawResults {
		item, ok := raw.(map[string]any)
		if !ok {
			record.Error = anySearchFailure("invalid results", "", reply.requestID).Error()
			return record
		}
		results = append(results, anySearchModelResult(item))
	}
	if len(results) > request.MaxResults {
		results = results[:request.MaxResults]
	}
	record.Results = results
	return record
}

// anySearchModelResult copies a result for the model and bounds its size: every
// string is trimmed and capped, and a nested value larger than
// anySearchResultNestedBytes is dropped. Snippet and content are both kept,
// because for vertical results the snippet is a separate summary, not a prefix
// of the content.
func anySearchModelResult(item map[string]any) map[string]any {
	out := make(map[string]any, len(item))
	for k, v := range item {
		switch value := v.(type) {
		case string:
			out[k] = truncateAnySearchRunes(strings.TrimSpace(value), anySearchResultTextRunes, false)
		case map[string]any, []any:
			if encoded, err := json.Marshal(value); err == nil && len(encoded) <= anySearchResultNestedBytes {
				out[k] = value
			}
		default:
			out[k] = v
		}
	}
	return out
}

func anySearchText(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

// ComponentSpec returns the AnySearchSearch Canvas surface. Only query is
// required in the debug form.
func (a *AnySearchTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"query":       "The search keywords to execute with AnySearch.",
			"tag":         "Optional vertical capability tag such as code.doc.",
			"params":      "Optional parameters the tag declares, as an object.",
			"max_results": "Optional number of results, 1 to 10.",
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered AnySearch references for downstream prompts.",
			"json":               "AnySearch results per query, in input order.",
		},
		InputForm: map[string]any{
			"query":       map[string]any{"name": "Query", "type": "line"},
			"tag":         map[string]any{"name": "Tag", "type": "line", "optional": true},
			"params":      map[string]any{"name": "Params", "type": "object", "optional": true},
			"max_results": map[string]any{"name": "Max results", "type": "integer", "optional": true},
		},
		PreserveJSONNumbers: true,
	}
}

// BuildReferences turns every successful query's usable results into chunks.
func (a *AnySearchTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	_, chunks, docAggs := collectAnySearchReferences(envelope)
	return chunks, docAggs
}

// BuildComponentOutputs renders the references grouped by query and passes the
// ordered per-query records through as json.
func (a *AnySearchTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	queries, _, _ := collectAnySearchReferences(envelope)
	return map[string]any{
		"formalized_content": renderAnySearchReferences(queries, anySearchPromptMaxTokens),
		"json":               envelopeSlice(envelope, "queries"),
	}
}

// anySearchRenderQuery is one query's share of formalized_content.
type anySearchRenderQuery struct {
	Query  string
	Tag    string
	Error  string
	Chunks []map[string]any
}

// collectAnySearchReferences prefers a result's page content and falls back to
// its snippet. A result with no text, or whose URL is not an absolute http(s)
// URL with a host, is dropped rather than stored as a blank or unsafe citation.
func collectAnySearchReferences(envelope map[string]any) ([]anySearchRenderQuery, []map[string]any, []map[string]any) {
	records := envelopeSlice(envelope, "queries")
	queries := make([]anySearchRenderQuery, 0, len(records))
	var chunks, docAggs []map[string]any
	for _, rawRecord := range records {
		record, ok := rawRecord.(map[string]any)
		if !ok {
			continue
		}
		rq := anySearchRenderQuery{Query: anySearchText(record["query"]), Tag: anySearchText(record["tag"]), Error: anySearchText(record["_ERROR"])}
		for _, rawResult := range envelopeSlice(record, "results") {
			result, ok := rawResult.(map[string]any)
			if !ok {
				continue
			}
			resultURL := anySearchText(result["url"])
			if !isAnySearchHTTPURL(resultURL) {
				continue
			}
			content := anySearchText(result["content"])
			if content == "" {
				content = anySearchText(result["snippet"])
			}
			content = strings.Join(strings.Fields(content), " ")
			if content == "" {
				continue
			}
			documentID := strconv.FormatInt(anySearchHashInt(resultURL+"\x00"+content, 1000000000000000), 10)
			title := anySearchText(result["title"])
			// The run state keeps one reference per id, so the id must stay unique
			// across a 5 x 10 batch: a small hash bucket would silently drop
			// colliding results.
			chunk := map[string]any{
				"id":            documentID,
				"chunk_id":      documentID,
				"content":       content,
				"doc_id":        documentID,
				"document_id":   documentID,
				"docnm_kwd":     title,
				"document_name": title,
				"similarity":    1,
				"score":         1,
				"url":           resultURL,
			}
			rq.Chunks = append(rq.Chunks, chunk)
			chunks = append(chunks, chunk)
			docAggs = append(docAggs, map[string]any{
				"doc_name": title,
				"doc_id":   documentID,
				"count":    1,
				"url":      resultURL,
			})
		}
		queries = append(queries, rq)
	}
	return queries, chunks, docAggs
}

func isAnySearchHTTPURL(raw string) bool {
	u, err := neturl.ParseRequestURI(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != ""
}

// renderAnySearchReferences renders each query as a header followed by its
// blocks. The token budget is checked on the whole block, header included,
// before it is appended, so a large block cannot push the output past it.
func renderAnySearchReferences(queries []anySearchRenderQuery, maxTokens int) string {
	usedTokens := 0
	var blocks []string
	fits := func(block string) bool {
		tokens := tokenizer.NumTokensFromString(block)
		if maxTokens > 0 && float64(usedTokens+tokens) > float64(maxTokens)*0.97 {
			return false
		}
		usedTokens += tokens
		return true
	}
	for _, query := range queries {
		header := "Query: " + query.Query
		if query.Tag != "" {
			header += " (" + query.Tag + ")"
		}
		if query.Error != "" {
			if !fits(header + "\nError: " + query.Error) {
				return strings.Join(blocks, "\n")
			}
			blocks = append(blocks, header+"\nError: "+query.Error)
			continue
		}
		for i, chunk := range query.Chunks {
			block := strings.Join([]string{
				"\nID: " + anySearchText(chunk["id"]),
				"├── Title: " + strings.Join(strings.Fields(anySearchText(chunk["document_name"])), " "),
				"├── URL: " + anySearchText(chunk["url"]),
				"└── Content:\n" + anySearchText(chunk["content"]),
			}, "\n")
			if i == 0 {
				block = header + block
			}
			if !fits(block) {
				return strings.Join(blocks, "\n")
			}
			blocks = append(blocks, block)
		}
	}
	return strings.Join(blocks, "\n")
}

func anySearchHashInt(value string, modulus int64) int64 {
	digest := sha1.Sum([]byte(value))
	number := new(big.Int).SetBytes(digest[:])
	return new(big.Int).Mod(number, big.NewInt(modulus)).Int64()
}

func anySearchJSON(value any) string {
	b, err := json.Marshal(value)
	if err != nil {
		return `{"_ERROR":"anysearch: marshal result"}`
	}
	return string(b)
}

func anySearchErrJSON(err error) string {
	return anySearchJSON(anySearchSearchEnvelope{Queries: []anySearchQueryResult{}, Error: err.Error()})
}
