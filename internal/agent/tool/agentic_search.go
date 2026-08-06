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
	"regexp"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// Agentic search tool names mirror Python rag/advanced_rag/harness/tools/search.py.
const (
	toolHybridSearch    = "hybrid_search"
	toolVectorSearch    = "vector_search"
	toolBM25Search      = "bm25_search"
	toolWebSearch       = "web_search"
	toolStructuredQuery = "structured_query"
)

// hybridSearchArgs is the shared JSON schema for the three retrieval tools.
type hybridSearchArgs struct {
	Query       string   `json:"query"`
	KbIDs       []string `json:"kb_ids,omitempty"`
	TopN        int      `json:"top_n,omitempty"`
	DocScope    []string `json:"doc_scope,omitempty"`
	Keywords    string   `json:"keywords,omitempty"`
	UseCompiled bool     `json:"use_compiled,omitempty"`
}

type agenticSearchResult struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs,omitempty"`
}

// AgenticSearchTool is the hybrid/vector/bm25 retrieval tool. The search mode
// selects the vector-similarity weight used by the underlying retrieval service:
//
//	hybrid: 0.3 (hybrid of keyword + vector)
//	vector: 1.0 (vector-only)
//	bm25:   0.0 (keyword-only)
//
// It backs onto GetRetrievalService() (the same singleton the agent Retrieval
// tool uses), so DocScope and KB scoping carry through automatically.
type AgenticSearchTool struct {
	mode     string // hybrid_search | vector_search | bm25_search
	weight   float64
	defaults hybridSearchArgs
}

// NewAgenticSearchTool returns the retrieval tool for the given mode.
func NewAgenticSearchTool(mode string) *AgenticSearchTool {
	weight := 0.3
	switch mode {
	case toolVectorSearch:
		weight = 1.0
	case toolBM25Search:
		weight = 0.0
	}
	return &AgenticSearchTool{mode: mode, weight: weight, defaults: hybridSearchArgs{TopN: 12}}
}

func (a *AgenticSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: a.mode,
		Desc: fmt.Sprintf("Search the bound knowledge base(s) for the query (mode=%s). Returns relevant passages.", a.mode),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"query": {
				Type: schema.String, Required: true, Desc: "The search query.",
			},
			"kb_ids":       {Type: schema.Array, Desc: "Optional dataset ids to restrict to."},
			"top_n":        {Type: schema.Number, Desc: "Number of passages to return (default 12)."},
			"doc_scope":    {Type: schema.Array, Desc: "Optional doc ids to restrict to."},
			"keywords":     {Type: schema.String, Desc: "Comma-separated keywords to narrow results."},
			"use_compiled": {Type: schema.Boolean, Desc: "Whether to enrich with compiled products."},
		}),
	}, nil
}

// InvokableRun executes the retrieval. It returns JSON with "chunks" (array of
// chunk maps). Never returns a hard error for retrieval failures — it returns an
// empty result so the agent can fall back.
func (a *AgenticSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	var args hybridSearchArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("%s: parse arguments: %w", a.mode, err)
	}
	if args.TopN <= 0 {
		args.TopN = a.defaults.TopN
	}

	svc := GetRetrievalService()
	tenantID := canvasTenantID(ctx)
	datasetIDs := args.KbIDs
	if len(datasetIDs) == 0 {
		datasetIDs = canvasDatasetIDs(ctx, nil)
	}
	if svc == nil || tenantID == "" || len(datasetIDs) == 0 {
		return jsonChunksEmpty(), nil
	}

	weight := a.weight
	similarityThreshold := 0.2
	req := RetrievalRequest{
		Query:                    strings.TrimSpace(args.Query + " " + args.Keywords),
		DatasetIDs:               datasetIDs,
		TopN:                     args.TopN,
		TopK:                     args.TopN * 4,
		SimilarityThreshold:      &similarityThreshold,
		KeywordsSimilarityWeight: &weight,
		DocScope:                 args.DocScope,
	}
	chunks, err := svc.Search(ctx, nil, req)
	if err != nil {
		return jsonChunksEmpty(), nil // agent falls back on failure
	}

	// Keyword narrowing (mirrors Python _narrow_by_keywords).
	if args.Keywords != "" {
		chunks = narrowByKeywords(chunks, args.Keywords)
	}
	return marshalSearchResult(chunks), nil
}

// narrowByKeywords narrows each chunk to keyword-bearing sentences (+/-1
// neighbour) and drops keyword-less chunks. A simplified port of Python's
// _narrow_by_keywords.
func narrowByKeywords(chunks []RetrievalChunk, keywords string) []RetrievalChunk {
	kwds := splitKeywords(keywords)
	if len(kwds) == 0 {
		return chunks
	}
	seen := map[string]struct{}{}
	out := make([]RetrievalChunk, 0, len(chunks))
	for _, c := range chunks {
		nc, ok := narrowContent(c.Content, kwds)
		if !ok {
			continue
		}
		hash := md5Hex(nc)
		if _, dup := seen[hash]; dup {
			continue
		}
		seen[hash] = struct{}{}
		c.Content = nc
		out = append(out, c)
	}
	return out
}

// splitKeywords normalizes a keyword string into a list of terms. When fewer
// than 3 comma terms exist, falls back to space-split bigrams (mirrors Python).
func splitKeywords(keywords string) []string {
	if strings.TrimSpace(keywords) == "" {
		return nil
	}
	kwds := make([]string, 0, 8)
	for _, k := range strings.Split(keywords, ",") {
		if k = strings.TrimSpace(k); k != "" {
			kwds = append(kwds, strings.ToLower(k))
		}
	}
	if len(kwds) < 3 {
		words := make([]string, 0, 8)
		for _, w := range strings.Split(keywords, " ") {
			if w = strings.TrimSpace(w); w != "" {
				words = append(words, strings.ToLower(w))
			}
		}
		bigrams := make([]string, 0, len(words))
		for i := 0; i+1 < len(words); i++ {
			bigrams = append(bigrams, words[i]+" "+words[i+1])
		}
		if len(bigrams) > 0 {
			return bigrams
		}
	}
	return kwds
}

var sentEnd = regexp.MustCompile(`[。！？；!?;]+|\.`)

// splitSentences splits text into sentences at sentence terminators, keeping a
// digit-guarded period intact ("3.14" / "v1.2" are not split). Implemented with
// a simple splitter because RE2 (Go) does not support lookbehind/lookahead.
func splitSentences(content string) []string {
	raw := sentEnd.Split(content, -1)
	sents := make([]string, 0, len(raw))
	for _, s := range raw {
		if strings.TrimSpace(s) == "" {
			continue
		}
		// Re-join a trailing digit-period-digit that the splitter cut apart:
		// if s ends with a digit and content had ".<digit>" following, reattach.
		sents = append(sents, s)
	}
	return rejoinDigitPeriods(sents)
}

// rejoinDigitPeriods merges "…1" + "2…" back into "…1.2…" when a decimal point
// separated two digit groups.
func rejoinDigitPeriods(sents []string) []string {
	out := make([]string, 0, len(sents))
	for i := 0; i < len(sents); i++ {
		cur := sents[i]
		// If current ends with a digit and next begins with a digit, the split
		// point was a decimal point — merge them.
		for i+1 < len(sents) && hasTrailingDigit(cur) && hasLeadingDigit(sents[i+1]) {
			cur = strings.TrimRight(cur, " \t") + "." + sents[i+1]
			i++
		}
		out = append(out, cur)
	}
	return out
}

func hasTrailingDigit(s string) bool {
	s = strings.TrimRight(s, " \t")
	return len(s) > 0 && s[len(s)-1] >= '0' && s[len(s)-1] <= '9'
}

func hasLeadingDigit(s string) bool {
	s = strings.TrimLeft(s, " \t")
	return len(s) > 0 && s[0] >= '0' && s[0] <= '9'
}

// narrowContent returns the keyword-bearing sentences (+/-1 neighbour) with the
// keyword highlighted, or (_, false) if no keyword occurs.
func narrowContent(content string, kwds []string) (string, bool) {
	if strings.TrimSpace(content) == "" {
		return "", false
	}
	sents := splitSentences(content)
	if len(sents) == 0 {
		return "", false
	}
	keep := map[int]bool{}
	matched := false
	for i, s := range sents {
		low := strings.ToLower(s)
		for _, kw := range kwds {
			if kw != "" && strings.Contains(low, kw) {
				matched = true
				if i > 0 {
					keep[i-1] = true
				}
				keep[i] = true
				if i+1 < len(sents) {
					keep[i+1] = true
				}
				break
			}
		}
	}
	if !matched {
		return "", false
	}
	var b strings.Builder
	for i := 0; i < len(sents); i++ {
		if keep[i] {
			b.WriteString(sents[i])
		}
	}
	return "..." + highlightKeywords(b.String(), kwds) + "...", true
}

// highlightKeywords wraps keyword occurrences in <em>.
func highlightKeywords(text string, kwds []string) string {
	if len(kwds) == 0 {
		return text
	}
	// Sort by length desc so longer terms match first.
	terms := make([]string, len(kwds))
	copy(terms, kwds)
	for i := 1; i < len(terms); i++ {
		for j := i; j > 0 && len(terms[j]) > len(terms[j-1]); j-- {
			terms[j], terms[j-1] = terms[j-1], terms[j]
		}
	}
	pattern := "("
	for i, t := range terms {
		if t == "" {
			continue
		}
		if i > 0 {
			pattern += "|"
		}
		pattern += regexp.QuoteMeta(t)
	}
	pattern += ")"
	re := regexp.MustCompile(`(?i)` + pattern)
	return re.ReplaceAllString(text, "<em>${1}</em>")
}

func md5Hex(s string) string {
	h := uint32(2166136261)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return fmt.Sprintf("%08x", h)
}

func jsonChunksEmpty() string {
	return `{"chunks":[]}`
}

func marshalSearchResult(chunks []RetrievalChunk) string {
	type outChunk struct {
		ID         string  `json:"id"`
		Content    string  `json:"content"`
		DocumentID string  `json:"doc_id"`
		DocName    string  `json:"docnm_kwd"`
		Score      float64 `json:"similarity"`
	}
	out := make([]outChunk, 0, len(chunks))
	for _, c := range chunks {
		out = append(out, outChunk{
			ID: c.ID, Content: c.Content, DocumentID: c.DocumentID,
			DocName: c.DocumentName, Score: c.Score,
		})
	}
	b, err := json.Marshal(map[string]interface{}{"chunks": out})
	if err != nil {
		return jsonChunksEmpty()
	}
	return string(b)
}
