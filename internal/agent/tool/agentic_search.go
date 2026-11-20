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
//
// Only the three retrieval modes are canvas tools. The harness' other names
// (web_search, structured_query) have no canvas counterpart and are therefore
// deliberately absent: Python's canvas reaches the web through its provider
// tools (tavily / duckduckgo / …) and registers neither name in agent/tools/.
const (
	toolHybridSearch = "hybrid_search"
	toolVectorSearch = "vector_search"
	toolBM25Search   = "bm25_search"
)

// hybridSearchArgs is the shared JSON schema for the three retrieval tools.
type hybridSearchArgs struct {
	Query    string   `json:"query" doc:"The search query." required:"true"`
	KbIDs    []string `json:"kb_ids,omitempty" doc:"Optional dataset ids to restrict to."`
	TopN     int      `json:"top_n,omitempty" doc:"Number of passages to return (default 12)."`
	DocScope []string `json:"doc_scope,omitempty" doc:"Optional doc ids to restrict to."`
	Keywords string   `json:"keywords,omitempty" doc:"Comma-separated keywords to narrow results."`
}

type agenticSearchResult struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs,omitempty"`
}

// AgenticSearchTool is the hybrid/vector/bm25 retrieval tool. The search mode
// selects the keyword share of the fusion weight used by the underlying
// retrieval service (RetrievalRequest.KeywordsSimilarityWeight; the vector
// share is its complement):
//
//	hybrid: 0.7 (hybrid of keyword + vector)
//	vector: 0.0 (vector-only)
//	bm25:   1.0 (keyword-only)
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
	weight := 0.7
	switch mode {
	case toolVectorSearch:
		weight = 0.0
	case toolBM25Search:
		weight = 1.0
	}
	return &AgenticSearchTool{mode: mode, weight: weight, defaults: hybridSearchArgs{TopN: 12}}
}

func (a *AgenticSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return BuildToolInfo(a.mode, fmt.Sprintf("Search the bound knowledge base(s) for the query (mode=%s). Returns relevant passages.", a.mode), hybridSearchArgs{})
}

// InvokableRun executes the retrieval. It returns JSON with "chunks" (array of
// chunk maps). Retrieval failures are returned so the caller can distinguish
// an unavailable backend from a successful search with no matches.
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
	// Route through canvasDatasetIDs so a trusted WithScope injection (when
	// present) intersects the model-supplied kb_ids, preventing escalation
	// outside the conversation's KBs. In the canvas runtime (no injection)
	// explicit kb_ids remain authoritative, preserving prior behavior.
	datasetIDs := canvasDatasetIDs(ctx, args.KbIDs)
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
		AllowDenseFallback:       new(false),
		KeywordsSimilarityWeight: &weight,
		DocScope:                 args.DocScope,
		TenantID:                 tenantID,
		ExcludeCompiled:          true,
	}
	chunks, err := svc.Search(ctx, nil, req)
	if err != nil {
		return "", fmt.Errorf("%s: retrieval: %w", a.mode, err)
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
	for k := range strings.SplitSeq(keywords, ",") {
		if k = strings.TrimSpace(k); k != "" {
			kwds = append(kwds, strings.ToLower(k))
		}
	}
	if len(kwds) < 3 {
		words := make([]string, 0, 8)
		for w := range strings.SplitSeq(keywords, " ") {
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

// splitSentences splits text into sentences, keeping each terminator attached
// like Python _split_plain. A period next to a digit is not a terminator
// ("3.14" / "v1.2"); RE2 has no lookaround, so that guard is applied here.
func splitSentences(content string) []string {
	var sents []string
	start := 0
	for _, m := range sentEnd.FindAllStringIndex(content, -1) {
		if content[m[0]] == '.' && (m[0] > 0 && isASCIIDigit(content[m[0]-1]) || m[1] < len(content) && isASCIIDigit(content[m[1]])) {
			continue
		}
		if seg := content[start:m[1]]; strings.TrimSpace(seg) != "" {
			sents = append(sents, seg)
		}
		start = m[1]
	}
	if tail := content[start:]; strings.TrimSpace(tail) != "" {
		sents = append(sents, tail)
	}
	return sents
}

func isASCIIDigit(b byte) bool {
	return b >= '0' && b <= '9'
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

// highlightKeywords stars keyword occurrences, mirroring Python
// _highlight_keywords' `*term*` marker (text_processing.py:412) — NOT an XML
// tag: search.py narrows through the same helper, so a starred span is what the
// Python canvas hands the model. (The <em> tags elsewhere in this port are the
// engine's highlight markup: rag/utils/*_conn.py, akshare.go.)
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
	return re.ReplaceAllString(text, "*${1}*")
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
