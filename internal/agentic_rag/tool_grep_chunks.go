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
Returns an XML <grep_results> document. Each matching chunk is a <chunk> element with chunk_id, doc_id (owning document id), doc_title, dataset_id (owning dataset id), page_num, chunk_index and score attributes, and a <match_snippet> element — a SHORT window (~80 chars on each side of the first match), NOT the full chunk text. The snippet is for fast relevance judgement only. To read a located document's complete text, call list_chunks with the returned doc_id.
IMPORTANT — keep the regex BROAD: use bare keywords and names combined with |, and DO NOT anchor it to a specific subject/verb chain. A regex like "何进.*斩" misses "帝召何进擒马元义，斩之" (何进 is the object, not the subject). Prefer listing the key people, objects and verbs directly, e.g. "马元义|董重|董太后|蹇硕|鸩杀|自刎|斩|诛" — this matches regardless of grammatical role.
Examples:
- Alternation (RECOMMENDED): "stardust|skyvault|psionic" (matches any of the words)
- Multiple terms in order: "psionic.*engine"
- Plain text: "engine" (matches literal substring anywhere in chunk content)
IMPORTANT — JSON escaping: every backslash in a regex MUST be written as \\ inside the JSON tool arguments (e.g. "\\d+" for a digit, "C\\+\\+" for literal "C++"). Plain "\+" / "\d" are invalid JSON escapes and will fail to parse.
Use this to locate candidate chunks by exact identifiers, error codes, product names, or recurring terms. After grep_chunks returns doc ids, read the full text with list_chunks.`

// grepChunksArgs is the JSON the model sends into InvokableRun.
type grepChunksArgs struct {
	Query      string   `json:"query"`
	DatasetIDs []string `json:"dataset_ids,omitempty"`
	DocScope   []string `json:"doc_scope,omitempty"`
}

// grepChunksDefaultLimit caps the number of matching chunks returned.
const grepChunksDefaultLimit = 30

// snippetContextRunes bounds the context on each side of a match snippet.
// Because grep_chunks returns only this truncated window (not the full
// content_with_weight), the model must call list_chunks to deep-read a
// located document's complete text.
const snippetContextRunes = 80

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
      "description": "REQUIRED: a single POSIX regex applied to chunk content (case-insensitive). Combine multiple concepts with | alternation in ONE regex, e.g. \"马元义|董重|董太后|鸩杀|自刎|斩|诛\"."
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
		return formatGrepResults(query, nil, re), nil
	}

	svc := runtime.GetGrepService()
	tenantID := g.tenantID
	chunks, err := svc.Grep(ctx, runtime.GrepRequest{
		Pattern:      query,
		DatasetIDs:   datasetIDs,
		DocScope:     args.DocScope,
		Limit:        grepChunksDefaultLimit,
		Sort:         grepChunksSortFields, // order by doc_id, page_num_int, chunk_order_int
		SelectFields: grepChunksSelectFields,
		TenantID:     tenantID,
	})
	if err != nil {
		return "", fmt.Errorf("grep_chunks: %w", err)
	}

	// Dedupe + cap. Results are ordered by reading order (doc_id, page,
	// chunk_index) from the engine.
	scored := scoreGrepChunks(chunks, re)
	sort.SliceStable(scored, func(i, j int) bool {
		return readingOrderLess(scored[i].chunk, scored[j].chunk)
	})
	if len(scored) > grepChunksDefaultLimit {
		scored = scored[:grepChunksDefaultLimit]
	}

	return formatGrepResults(query, scored, re), nil
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

// formatGrepResults emits the scored grep results as a compact XML document.
func formatGrepResults(query string, results []grepScoredChunk, re *regexp.Regexp) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<grep_results chunk_count=\"%d\">\n", len(results))
	fmt.Fprintf(&b, "<query>%s</query>\n", xmlEscape(query))

	if len(results) == 0 {
		b.WriteString("</grep_results>")
		return b.String()
	}

	for _, r := range results {
		c := r.chunk
		snippet := extractSnippet(c.Content, re)
		fmt.Fprintf(&b,
			"<chunk chunk_id=\"%s\" doc_id=\"%s\" doc_title=\"%s\" dataset_id=\"%s\" page_num=\"%d\" chunk_index=\"%d\" score=\"%.3f\">\n",
			xmlEscape(c.ID), xmlEscape(c.DocumentID), xmlEscape(c.DocumentName), xmlEscape(c.DatasetID),
			c.PageNum, c.ChunkIndex, r.score)
		if snippet != "" {
			fmt.Fprintf(&b, "<match_snippet>%s</match_snippet>\n", xmlEscape(snippet))
		}
		b.WriteString("</chunk>\n")
	}
	b.WriteString("</grep_results>")
	return b.String()
}

// extractSnippet returns a short single-line context snippet (snippetContextRunes
// chars on each side of the earliest regex match) from the chunk's full
// content_with_weight. It is intentionally NOT the full chunk text — grep_chunks
// is for locating/relevance, list_chunks is for deep-reading the full text.
func extractSnippet(content string, re *regexp.Regexp) string {
	if content == "" || re == nil {
		return ""
	}
	loc := re.FindStringIndex(content)
	if loc == nil {
		return ""
	}
	earliest, earliestEnd := loc[0], loc[1]

	matchStr := content[earliest:earliestEnd]
	before := content[:earliest]
	after := content[earliestEnd:]

	beforeRunes := []rune(before)
	if len(beforeRunes) > snippetContextRunes {
		beforeRunes = beforeRunes[len(beforeRunes)-snippetContextRunes:]
	}
	afterRunes := []rune(after)
	if len(afterRunes) > snippetContextRunes {
		afterRunes = afterRunes[:snippetContextRunes]
	}

	snippet := string(beforeRunes) + matchStr + string(afterRunes)
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	for strings.Contains(snippet, "  ") {
		snippet = strings.ReplaceAll(snippet, "  ", " ")
	}
	return strings.TrimSpace(snippet)
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
