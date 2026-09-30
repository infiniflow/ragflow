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

// Package agentic_rag centralises the smart-reasoning (ReAct) conversation
// mode: the agent entrypoint, its system prompt, and the tools (think,
// todo_write, grep_chunks, search_chunks, run_javascript) plus the GrepAdapter.
// It does not depend on internal/agent/tool (the canvas DSL tools); the shared
// retrieval contract (RetrievalChunk / GrepService / scope resolution) lives in
// internal/agent/runtime, which both agent layers depend on.
package agentic_rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"

	"ragflow/internal/agent/runtime"
)

// grepChunksToolName regex-matches chunk content and returns a scored XML
// document of matching chunks with short snippet windows.
const grepChunksToolName = "grep_chunks"

const grepChunksToolDescription = `Search dataset chunk content with a single POSIX regular expression, case-insensitive (behaves like grep -E -i).
Pack multiple concepts into ONE regex using | alternation — do not call this tool repeatedly for synonyms.
Returns an XML <search_results count="N" query="..."> document (unified with search_bm25_chunks / search_chunks). Each matching chunk is a <chunk> element with rank, chunk_id, doc_id (owning document id), doc_name, dataset_id (owning dataset id), page_num, chunk_index and score attributes, and a <match_snippet> element — a single-line window spanning from snippetContextRunes runes BEFORE your earliest match to that many runes AFTER your latest match across the whole chunk, NOT the full text. The snippet is for fast relevance judgement only. To read a located document's complete text, call list_chunks with anchor_chunk_ids.
IMPORTANT — keep the regex BROAD: use bare keywords and names combined with |, and DO NOT anchor it to a specific subject/verb chain. A regex like "何进.*斩" misses "帝召何进擒马元义，斩之" (何进 is the object, not the subject). Prefer listing the key people, objects and verbs directly, e.g. "马元义|董重|董太后|蹇硕|鸩杀|自刎|斩|诛" — this matches regardless of grammatical role.
Examples:
- Alternation (RECOMMENDED): "stardust|skyvault|psionic" (matches any of the words)
- Multiple terms in order: "psionic.*engine"
- Plain text: "engine" (matches literal substring anywhere in chunk content)
IMPORTANT — JSON escaping: every backslash in a regex MUST be written as \\ inside the JSON tool arguments (e.g. "\\d+" for a digit, "C\\+\\+" for literal "C++"). Plain "\+" / "\d" are invalid JSON escapes and will fail to parse.
REGEX LIMITS — keep the pattern CHEAP: bare keywords joined with | or .*. The engine compiles a pattern into an automaton with a fixed state budget and already wraps every pattern in .*(...).*, so:
- NEVER write bounded repetition such as .{0,20} — "关羽.{0,20}杀" fails with "would require more than 10000 effort"; write "关羽.*杀" or "关羽.*杀|关公.*斩" instead.
- Backreferences (\1) and lookaround ((?=, (?<!) are not supported — repeat the alternation or express the constraint as plain terms.
- If a query returns an error instead of results, FIX THE PATTERN (usually by dropping the repetition) and call again — an error is not "no such content in the corpus".
USE CASES — reserve this tool for what keyword ranking cannot express: exact literals (hyphenated forms, IDs, codes), character classes, anchors, CJK substrings, and ordered co-occurrence patterns that require two or more terms to appear near each other in one chunk (e.g. "co-lead.*newsletter|newsletter.*co-lead" — BM25 matches terms anywhere, never within-window adjacency). Do NOT use it for plain keyword recall: a pattern that is only a bare "w1|w2|w3" alternation with no positional structure is a BM25 query in disguise — send those terms to search_bm25_chunks instead.
Use this to locate candidate chunks by exact identifiers, error codes, product names, or recurring terms. After grep_chunks returns doc ids, read the full text with list_chunks.`

// grepChunksArgs is the JSON the model sends into InvokableRun.
type grepChunksArgs struct {
	Query      string   `json:"query"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
	DocScope   []string `json:"doc_scope,omitempty"`
}

// grepChunksDefaultLimit caps the number of matching chunks returned.
const grepChunksDefaultLimit = 30

// GrepChunksTool regex-matches chunk content via GrepService. It is stateless:
// no per-session seen-chunk tracking (memory is intentionally not ported). The
// tenant and dataset scope are injected at construction from the session, so the
// tool never needs a canvas runtime.
type GrepChunksTool struct {
	tenantID   string
	datasetIDs []string
}

// NewGrepChunksTool returns a GrepChunksTool scoped to the given tenant and
// datasets, implementing eino's runtime.InvokableTool.
func NewGrepChunksTool(tenantID string, datasetIDs []string) *GrepChunksTool {
	return &GrepChunksTool{tenantID: tenantID, datasetIDs: datasetIDs}
}

// Info returns the tool's metadata for the chat model. The parameter schema is
// declared as a single JSON string (like ListChunksTool) so the model sees the
// array length bounds (minItems/maxItems) and field contracts in one readable
// block, then parsed into *jsonschema.Schema.
func (g *GrepChunksTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "REQUIRED: a single POSIX regex applied to chunk content (case-insensitive). Combine multiple concepts with | alternation in ONE regex, e.g. \\\"马元义|董重|董太后|鸩杀|自刎|斩|诛\\\". Keep it cheap: NO bounded repetition such as .{0,20} (it exceeds the engine's automaton state budget), NO backreferences, NO lookaround - join terms with .* or list them as alternatives."
    },
    "dataset_ids": {
      "type": "array",
      "description": "Optional dataset ids to restrict the search to (at most 10). When omitted, the current conversation's bound datasets are used.",
      "items": { "type": "string" },
      "maxItems": 10
    },
    "doc_scope": {
      "type": "array",
      "description": "Optional document ids to restrict the search to (at most 10).",
      "items": { "type": "string" },
      "maxItems": 10
    }
  },
  "required": ["query"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("grep_chunks: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        grepChunksToolName,
		Desc:        grepChunksToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// InvokableRun validates the regex, greps chunk content, scores matches, and
// returns an XML <grep_results> document. Stateless.
func (g *GrepChunksTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, grepChunksToolName, g.invokableRun, argumentsInJSON)
}

func (g *GrepChunksTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	var args grepChunksArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("grep_chunks: parse arguments: %w", err)
	}

	query := strings.TrimSpace(args.Query)
	if query == "" {
		return "", fmt.Errorf("grep_chunks: query is required and must be a non-empty regex string")
	}

	// Compile with (?i) for case-insensitive matching; also validates syntax.
	re, err := regexp.Compile("(?i)" + query)
	if err != nil {
		return "", fmt.Errorf("grep_chunks: invalid regex %q: %w", query, err)
	}

	datasetIDs, err := resolveDatasetScope(g.datasetIDs, args.DatasetIDs)
	if err != nil {
		return "", fmt.Errorf("grep_chunks: %w", err)
	}
	if len(datasetIDs) == 0 {
		// Bound scope is empty: short-circuit before touching the backend so the
		// tool never reads outside the conversation's allowed datasets.
		return formatGrepResults(ctx, query, nil, re), nil
	}

	svc := runtime.GetGrepService()
	tenantID := g.tenantID
	req := runtime.GrepRequest{
		Pattern:      query,
		DatasetIDs:   datasetIDs,
		DocScope:     args.DocScope,
		Limit:        grepChunksDefaultLimit,
		Sort:         grepChunksSortFields, // order by doc_id, page_num_int, chunk_order_int
		SelectFields: grepChunksSelectFields,
		TenantID:     tenantID,
	}
	chunks, err := svc.Grep(ctx, req)
	degradedNotice := ""
	if err != nil {
		// The engine refused the pattern (unsupported construct or a blown
		// automaton budget). Recover rather than dead-end the turn: sanitize
		// and retry, then fall back to a lexical prefilter — the regexp filter
		// below re-checks every candidate, so only recall can degrade, and the
		// notice says so.
		var degradeErr error
		chunks, degradedNotice, degradeErr = degradeGrep(ctx, svc, req, err)
		if degradeErr != nil {
			return "", fmt.Errorf("grep_chunks: %w", degradeErr)
		}
	}

	// Dedupe, then keep only real matches: the pushdown path returns matching
	// chunks already, but the lexical prefilter (degradeGrep) hands back
	// CANDIDATES that the local regexp must still approve — a zero score means
	// it did not match, and shipping it would put prose the query never matched
	// in front of the model as a "hit".
	scored := scoreGrepChunks(chunks, re)
	matched := make([]grepScoredChunk, 0, len(scored))
	for _, s := range scored {
		if s.score > 0 {
			matched = append(matched, s)
		}
	}
	scored = matched
	sort.SliceStable(scored, func(i, j int) bool {
		return readingOrderLess(scored[i].chunk, scored[j].chunk)
	})
	if len(scored) > grepChunksDefaultLimit {
		scored = scored[:grepChunksDefaultLimit]
	}

	out := formatGrepResults(ctx, query, scored, re)
	if degradedNotice != "" {
		// Canonical graceful-degradation shape: the model must know the hits it
		// sees are complete-by-prefilter, not complete-by-corpus.
		out = toolErrorXML(grepChunksToolName, "warn", degradedNotice) + "\n" + out
	}
	return out, nil
}

// grepScoredChunk pairs a retrieval chunk with its regex match score.
type grepScoredChunk struct {
	chunk runtime.RetrievalChunk
	score float64
}

// scoreGrepChunks scores each chunk by match count + earliest-position bonus,
// and dedupes by chunk ID.
func scoreGrepChunks(chunks []runtime.RetrievalChunk, re *regexp.Regexp) []grepScoredChunk {
	seen := map[string]struct{}{}
	out := make([]grepScoredChunk, 0, len(chunks))
	for _, c := range chunks {
		if _, dup := seen[c.ID]; dup {
			continue
		}
		seen[c.ID] = struct{}{}

		content := c.Content
		score := 0.0
		if content != "" && re != nil {
			locs := re.FindAllStringIndex(content, -1)
			matchCount := len(locs)
			earliestPos := len(content)
			if len(locs) > 0 && locs[0][0] < earliestPos {
				earliestPos = locs[0][0]
			}
			if matchCount > 0 {
				base := float64(matchCount) / float64(matchCount+1) // 0.5..1.0 range
				positionBonus := 0.0
				if earliestPos < len(content) {
					positionRatio := 1.0 - float64(earliestPos)/float64(len(content))
					positionBonus = positionRatio * 0.1
				}
				score = math.Min(base+positionBonus, 1.0)
			}
		}
		out = append(out, grepScoredChunk{chunk: c, score: score})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].score > out[j].score
	})
	return out
}

// formatGrepResults emits scored grep hits through the UNIFIED locate payload:
// a <search_results> root with the query echoed as an attribute and one
// <match_snippet> per chunk spanning earliest-match − N .. latest-match + N
// runes (shared snippetContextRunes window).
func formatGrepResults(ctx context.Context, query string, results []grepScoredChunk, re *regexp.Regexp) string {
	hits := make([]snippetHit, 0, len(results))
	for _, r := range results {
		snippet, truncated := "", false
		if r.chunk.Content != "" && re != nil {
			if first, last, ok := regexMatchSpan(re, r.chunk.Content); ok {
				snippet, truncated = snippetForMatches(r.chunk.Content, first, last)
			}
		}
		hits = append(hits, snippetHit{chunk: r.chunk, snippet: snippet, truncated: truncated})
	}
	return formatLocateResultsXML(ctx, grepChunksToolName, query, hits)
}

// xmlEscape escapes characters that would break simple XML attribute/element
// values. The output is consumed by the LLM (forgiving parser).
func xmlEscape(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(s)
}
