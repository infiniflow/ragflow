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
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const datacircleToolName = "datacircle"

const datacircleToolDescription = `Datacircle benchmarks GTM APIs (email finders, LinkedIn profile and company data, phone finders, enrichment, web search, scraping and more) and lists every plan of each, with its price per 1,000 calls and its score out of 100. Use it to find the best or the cheapest provider for a data need.
   - Leave out api and query to list every API with its best plan.
   - Give api, an API's name or slug as that list gives them, to get its plans, best first.
   - Give query, words of a provider's or a plan's name, to find a provider's plans.
   - Sort by "score" (best first), "price" (lowest price per 1,000 first) or "provider" (A to Z).`

// datacircleMaxResults is the most plans the API returns for one call.
const datacircleMaxResults = 50

const datacirclePromptMaxTokens = 200000

var datacircleSortValues = []string{"score", "price", "provider"}

// datacircleBaseURL is the Datacircle API's base URL. Exposed as a package var
// so tests can substitute it through rewriteHostTransport.
var datacircleBaseURL = "https://api.datacircle.dev/v1"

// datacirclePlanFields are the plan fields handed to the model and the Canvas
// json output. A plan has about a hundred fields, most of them long (terms of
// use, request fields, sources), so only those that compare plans are kept.
var datacirclePlanFields = []string{
	"provider", "plan", "api", "call", "rank", "score", "perThousand", "notFoundPerThousand",
	"billing", "pay", "currency", "units", "unit", "calls", "renews", "freshness",
	"latencyMs", "perSecond", "perDay", "strengths", "pitch", "website", "pricingPage", "docs", "page",
}

// datacircleArgs is the JSON shape the model sends into InvokableRun.
// max_results is decoded loosely: a missing, blank or non-numeric value means
// "use the node's Top N" rather than failing the run.
type datacircleArgs struct {
	API        string `json:"api"`
	Query      string `json:"query"`
	Sort       string `json:"sort"`
	MaxResults any    `json:"max_results"`
}

// datacircleDefaults holds the node-level configuration. API is used when a
// call names no API, so a Canvas node set to one API searches only its plans.
type datacircleDefaults struct {
	API  string
	Sort string
	TopN int
}

type datacirclePlansResponse struct {
	Total int              `json:"total"`
	Plans []map[string]any `json:"plans"`
}

// DatacircleTool searches the GTM API plans Datacircle benchmarks. It GETs
// https://api.datacircle.dev/v1/apis when given no API and no words, else
// /v1/plans. The API is open and takes no key.
type DatacircleTool struct {
	helper   *HTTPHelper
	defaults datacircleDefaults
}

var _ ToolComponent = (*DatacircleTool)(nil)
var _ ReferenceBuilder = (*DatacircleTool)(nil)

// NewDatacircleTool returns a DatacircleTool with the default HTTPHelper.
func NewDatacircleTool() *DatacircleTool {
	return newDatacircleTool(nil, datacircleDefaults{})
}

// NewDatacircleToolWith returns a DatacircleTool that uses the provided
// HTTPHelper. Useful for tests that want to inject a custom transport.
func NewDatacircleToolWith(h *HTTPHelper) *DatacircleTool {
	return newDatacircleTool(h, datacircleDefaults{})
}

func newDatacircleTool(h *HTTPHelper, defaults datacircleDefaults) *DatacircleTool {
	if h == nil {
		h = NewHTTPHelper()
	}
	if defaults.Sort == "" {
		defaults.Sort = "score"
	}
	if defaults.TopN <= 0 {
		defaults.TopN = 10
	}
	return &DatacircleTool{helper: h, defaults: defaults}
}

// Info returns the tool's metadata for the chat model.
func (d *DatacircleTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: datacircleToolName,
		Desc: datacircleToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"api": {
				Type:     schema.String,
				Desc:     `The API whose plans to give, its name or slug, such as "Employee email" or "email-finder". Leave it out to list every API.`,
				Required: false,
			},
			"query": {
				Type:     schema.String,
				Desc:     "Words that must all appear in the provider's or the plan's name, such as a provider's name.",
				Required: false,
			},
			"sort": {
				Type:     schema.String,
				Desc:     `How to order the plans: "score" (default) best first, "price" lowest price per 1,000 first, "provider" A to Z.`,
				Enum:     datacircleSortValues,
				Required: false,
			},
			"max_results": {
				Type:     schema.Integer,
				Desc:     "Number of plans to return, 1 to 50. Leave it out to use the Top N set on the node.",
				Required: false,
			},
		}),
	}, nil
}

// InvokableRun lists the APIs or searches the plans.
//
// Recoverable errors (bad arguments, API or network failures) are returned as
// a JSON envelope with an _ERROR field and a nil Go error, so the ReAct agent
// feeds them back to the model instead of failing the run, matching SofyaTool.
func (d *DatacircleTool) InvokableRun(ctx context.Context, argsJSON string, _ ...tool.Option) (string, error) {
	var args datacircleArgs
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return datacircleErrJSON(fmt.Errorf("datacircle: parse arguments: %w", err)), nil
	}
	api := strings.TrimSpace(args.API)
	if api == "" {
		api = d.defaults.API
	}
	query := strings.TrimSpace(args.Query)
	if api == "" && query == "" {
		return d.listAPIs(ctx), nil
	}

	sortBy, err := datacircleSortArg(args.Sort, d.defaults.Sort)
	if err != nil {
		return datacircleErrJSON(err), nil
	}
	params := url.Values{}
	if api != "" {
		params.Set("api", api)
	}
	if query != "" {
		params.Set("query", query)
	}
	params.Set("sort", sortBy)
	params.Set("limit", strconv.Itoa(datacircleMaxResultsArg(args.MaxResults, d.defaults.TopN)))

	var raw datacirclePlansResponse
	if err := d.get(ctx, "/plans?"+params.Encode(), &raw); err != nil {
		return datacircleErrJSON(err), nil
	}
	plans := make([]map[string]any, 0, len(raw.Plans))
	for _, plan := range raw.Plans {
		plans = append(plans, datacircleTrimPlan(plan))
	}
	return datacircleJSON(map[string]any{"total": raw.Total, "plans": plans}), nil
}

func (d *DatacircleTool) listAPIs(ctx context.Context) string {
	var apis []map[string]any
	if err := d.get(ctx, "/apis", &apis); err != nil {
		return datacircleErrJSON(err)
	}
	if apis == nil {
		apis = []map[string]any{}
	}
	return datacircleJSON(map[string]any{"apis": apis})
}

// get calls the Datacircle API and decodes its JSON answer into out. An
// answer other than 2xx carries the API's own error message, which says what
// to change (an unknown api lists the names it takes).
func (d *DatacircleTool) get(ctx context.Context, path string, out any) error {
	resp, err := d.helper.Do(ctx, http.MethodGet, datacircleBaseURL+path, "", "",
		map[string]string{
			"Accept": "application/json",
			// Identifies RAGFlow to Datacircle.
			"User-Agent": "RAGFlow datacircle-integration/infiniflow-ragflow",
		},
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var apiErr struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&apiErr) == nil && strings.TrimSpace(apiErr.Error) != "" {
			return fmt.Errorf("datacircle: upstream returned %d: %s", resp.StatusCode, apiErr.Error)
		}
		return fmt.Errorf("datacircle: upstream returned %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("datacircle: decode response: %w", err)
	}
	return nil
}

// datacircleSortArg normalizes a model-supplied sort. Blank means the node's
// sort; anything else must be a known value, because it is forwarded as-is.
func datacircleSortArg(value, fallback string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return fallback, nil
	}
	for _, s := range datacircleSortValues {
		if v == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("datacircle: sort %q is not supported, it should be one of %v", value, datacircleSortValues)
}

// datacircleMaxResultsArg returns the model's plan count if it gave a usable
// one, else the node's Top N, clamped to [1, 50].
func datacircleMaxResultsArg(value any, topN int) int {
	n := topN
	switch v := value.(type) {
	case float64:
		n = int(v)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			n = parsed
		}
	}
	return min(max(n, 1), datacircleMaxResults)
}

func datacircleTrimPlan(plan map[string]any) map[string]any {
	out := make(map[string]any, len(datacirclePlanFields))
	for _, key := range datacirclePlanFields {
		if value, ok := plan[key]; ok && value != nil {
			out[key] = value
		}
	}
	return out
}

// ComponentSpec returns the Datacircle Canvas surface.
func (d *DatacircleTool) ComponentSpec() ComponentSpec {
	return ComponentSpec{
		Inputs: map[string]string{
			"api":   "The API whose plans to give, its name or slug. Empty lists every API.",
			"query": "Words of a provider's or a plan's name.",
			"sort":  `Plan order: "score", "price" or "provider".`,
		},
		Outputs: map[string]string{
			"formalized_content": "Rendered Datacircle references for downstream prompts.",
			"json":               "The APIs or the plans found.",
		},
		InputForm: map[string]any{
			"api":   map[string]any{"name": "API", "type": "line"},
			"query": map[string]any{"name": "Query", "type": "line"},
			"sort": map[string]any{
				"name":    "Sort",
				"type":    "options",
				"value":   "score",
				"options": datacircleSortValues,
			},
		},
	}
}

func (d *DatacircleTool) BuildReferences(_ context.Context, envelope map[string]any) ([]map[string]any, []map[string]any) {
	return buildDatacircleReferences(envelope)
}

func (d *DatacircleTool) BuildComponentOutputs(envelope map[string]any) map[string]any {
	results := envelopeSlice(envelope, "plans")
	if _, listed := envelope["apis"]; listed {
		results = envelopeSlice(envelope, "apis")
	}
	chunks, _ := buildDatacircleReferences(envelope)
	return map[string]any{
		"formalized_content": renderSofyaReferences(chunks, datacirclePromptMaxTokens),
		"json":               results,
	}
}

// buildDatacircleReferences makes one reference per plan, linking the plan's
// page on datacircle.dev, or one per API, linking its best page or its sheet.
func buildDatacircleReferences(envelope map[string]any) ([]map[string]any, []map[string]any) {
	type entry struct{ title, url, content string }
	var entries []entry
	for _, item := range envelopeSlice(envelope, "plans") {
		plan, ok := item.(map[string]any)
		if !ok {
			continue
		}
		title := strings.Join(datacircleNonEmpty(datacircleText(plan["provider"]), datacircleText(plan["plan"])), ", ")
		if api := datacircleText(plan["api"]); api != "" {
			title += " (" + api + ")"
		}
		entries = append(entries, entry{title, datacircleText(plan["page"]), datacirclePlanText(plan)})
	}
	for _, item := range envelopeSlice(envelope, "apis") {
		api, ok := item.(map[string]any)
		if !ok {
			continue
		}
		page := datacircleText(api["bestPage"])
		if page == "" {
			page = datacircleText(api["sheetPage"])
		}
		entries = append(entries, entry{datacircleText(api["api"]), page, datacircleAPIText(api)})
	}

	chunks := make([]map[string]any, 0, len(entries))
	docAggs := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		if e.title == "" || e.content == "" {
			continue
		}
		documentID, displayID := referenceIDs(e.url, e.content)
		chunks = append(chunks, map[string]any{
			"id":            displayID,
			"chunk_id":      documentID,
			"content":       e.content,
			"doc_id":        documentID,
			"document_id":   documentID,
			"docnm_kwd":     e.title,
			"document_name": e.title,
			"similarity":    1,
			"score":         1,
			"url":           e.url,
		})
		docAggs = append(docAggs, map[string]any{
			"doc_name": e.title,
			"doc_id":   documentID,
			"count":    1,
			"url":      e.url,
		})
	}
	return chunks, docAggs
}

// datacirclePlanText renders a plan as the sentences a prompt reads.
func datacirclePlanText(plan map[string]any) string {
	parts := datacircleNonEmpty(
		datacircleLabeled("API: ", datacircleText(plan["api"])),
		datacircleLabeled("Provider: ", datacircleText(plan["provider"])),
		datacircleLabeled("Plan: ", datacircleText(plan["plan"])),
		datacircleLabeled("Call: ", datacircleText(plan["call"])),
		datacircleLabeled("Rank: ", datacircleText(plan["rank"])),
		datacircleLabeled("Score out of 100: ", datacircleText(plan["score"])),
		datacircleLabeled("Price per 1,000 in USD: ", datacircleText(plan["perThousand"])),
		datacircleLabeled("Price per 1,000 not found in USD: ", datacircleText(plan["notFoundPerThousand"])),
		datacircleLabeled("Billing: ", datacircleText(plan["billing"])),
		datacircleLabeled("Pay: ", datacirclePay(plan)),
		datacircleLabeled("Calls: ", datacircleText(plan["calls"])),
		datacircleLabeled("Renews: ", datacircleText(plan["renews"])),
		datacircleLabeled("Freshness: ", datacircleText(plan["freshness"])),
		datacircleLabeled("Median latency in ms: ", datacircleText(plan["latencyMs"])),
		datacircleLabeled("Calls per second: ", datacircleText(plan["perSecond"])),
		datacircleLabeled("Strengths: ", datacircleList(plan["strengths"])),
		datacircleLabeled("Pitch: ", datacircleText(plan["pitch"])),
		datacircleLabeled("Website: ", datacircleText(plan["website"])),
		datacircleLabeled("Pricing page: ", datacircleText(plan["pricingPage"])),
		datacircleLabeled("Docs: ", datacircleText(plan["docs"])),
	)
	return strings.Join(parts, "\n")
}

// datacircleAPIText renders an API of the list with its best plan.
func datacircleAPIText(api map[string]any) string {
	parts := datacircleNonEmpty(
		datacircleLabeled("API: ", datacircleText(api["api"])),
		datacircleLabeled("Slug: ", datacircleText(api["slug"])),
		datacircleLabeled("Plans: ", datacircleText(api["plans"])),
		datacircleLabeled("Providers: ", datacircleText(api["providers"])),
	)
	if best, ok := api["best"].(map[string]any); ok {
		parts = append(parts, datacircleNonEmpty(
			datacircleLabeled("Best provider: ", datacircleText(best["provider"])),
			datacircleLabeled("Best plan: ", datacircleText(best["plan"])),
			datacircleLabeled("Best score out of 100: ", datacircleText(best["score"])),
			datacircleLabeled("Best price per 1,000 in USD: ", datacircleText(best["perThousand"])),
		)...)
	}
	return strings.Join(parts, "\n")
}

// datacircleText renders a field's value; numbers keep their plain decimal
// form (300000, not 3e+05).
func datacircleText(value any) string {
	switch v := value.(type) {
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case json.Number:
		return v.String()
	}
	return sofyaText(value)
}

// datacirclePay renders the plan's price in its own currency, "39 USD".
func datacirclePay(plan map[string]any) string {
	pay := datacircleText(plan["pay"])
	if pay == "" {
		return ""
	}
	return strings.TrimSpace(pay + " " + datacircleText(plan["currency"]))
}

func datacircleList(value any) string {
	items, ok := value.([]any)
	if !ok {
		return datacircleText(value)
	}
	texts := make([]string, 0, len(items))
	for _, item := range items {
		if text := datacircleText(item); text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, ", ")
}

func datacircleLabeled(label, value string) string {
	if value == "" {
		return ""
	}
	return label + value
}

func datacircleNonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// datacircleJSON encodes the tool's result: {"apis": [...]} when it lists the
// APIs, {"total": n, "plans": [...]} when it searches the plans, or {"_ERROR": "..."}.
func datacircleJSON(env map[string]any) string {
	b, err := json.Marshal(env)
	if err != nil {
		return fmt.Sprintf(`{"_ERROR":"datacircle: marshal result: %s"}`, err)
	}
	return string(b)
}

func datacircleErrJSON(err error) string {
	return datacircleJSON(map[string]any{"_ERROR": err.Error()})
}
