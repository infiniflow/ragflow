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

package agentic_rag

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/eino-contrib/jsonschema"
	"go.uber.org/zap"

	"ragflow/internal/common"
)

// webSearchToolName is the ReAct loop's only door to the public internet. It is
// deliberately NOT in the template tool registry: whether it exists is a
// property of the CONVERSATION (the chat has a web search provider configured
// and the request enabled internet), not of the agent template, so Run appends
// it whenever the caller supplies one.
const webSearchToolName = "web_search"

const webSearchInstruction = `### Web search enabled for this turn
The user enabled internet search. An initial web_search query for the current question has already run; its result is included as a tool message. Use that web evidence alongside corpus retrieval, even when the corpus returns a match. Call web_search again with refined queries when the initial results are insufficient.
Use web results to supplement the corpus, check current facts, and distinguish public meanings of a name from unrelated corpus matches.
Deep-read corpus evidence with list_chunks; web results are snippets and cannot be opened with list_chunks. Ground web-backed claims in the returned snippets and cite each result's ref attribute as [ID:n], just like corpus evidence. These handles become clickable citations to the source URLs. Clearly distinguish corpus facts from web facts. If web search fails or finds no useful evidence, state that limitation and answer from the evidence available.`

const webSearchToolDescription = `Web search tool: queries the public internet through the conversation's configured search provider and returns the top snippets with their URLs.

## When to use it
When this tool is available, the user enabled internet search and an initial query for the current question runs before your first model turn. Use this tool for additional or refined queries when the initial results do not answer the question. Use the web alongside corpus evidence to check current facts and resolve public meanings of ambiguous names.

## What the Tool Does NOT Do
- Does NOT search the conversation's datasets (use grep_chunks / search_bm25_chunks / search_chunks)
- Does NOT return full pages: hits are provider SNIPPETS, and they are not corpus chunks — list_chunks cannot open them
- Does NOT support facts absent from its snippet: ground web-backed claims in the returned text and cite its ref handle

## Citing a hit in your answer
A hit has no corpus chunk_id and list_chunks cannot open it. Cite web-backed claims with [ID:n], where n is the result's ref attribute. Never invent a chunk_id or a citation handle. Use only the handles actually supplied by the tool, and distinguish web facts from corpus facts.

## Output (XML)
Returns an XML <web_results count="N" query="..."> document. Each hit is a <result> element with rank, url, title and ref attributes and the snippet as its text.`

// WebResult is one hit handed back by the conversation's web search provider.
// Content is the provider's snippet/passage text — web hits are snippets and
// not corpus chunks, so nothing in the retrieval toolset can deep-read them.
type WebResult struct {
	Title   string
	URL     string
	Content string
}

// WebSearchFunc runs one web search query and returns the provider's hits. The
// agentic package never contacts a provider itself: the caller injects this,
// built from the chat's configured provider, so the tool stays a per-conversation
// capability instead of a global dependency.
type WebSearchFunc func(ctx context.Context, query string) ([]WebResult, error)

// webSearchCtxKey is the context key under which a conversation's web search
// provider travels into a run.
type webSearchCtxKey struct{}

// WithWebSearch returns a copy of ctx carrying search, the conversation's web
// search provider. The explorer a run builds takes it back with webSearchFrom.
// It rides on the context rather than on Input because the provider is a
// property of the conversation, not of a call's arguments: runs for different
// chats (different providers, or none) interleave in one process, and
// package-level state would leak one conversation's capability into another's.
func WithWebSearch(ctx context.Context, search WebSearchFunc) context.Context {
	if search == nil {
		return ctx
	}
	return context.WithValue(ctx, webSearchCtxKey{}, search)
}

// webSearchFrom returns the provider carried by ctx, or nil when this
// conversation has no internet search configured.
func webSearchFrom(ctx context.Context) WebSearchFunc {
	search, _ := ctx.Value(webSearchCtxKey{}).(WebSearchFunc)
	return search
}

// webSearchTools returns the web_search tool when ctx carries a provider and
// nothing when it does not — an absent capability is never advertised to the
// model, because a tool it cannot answer is worse than a tool it never sees.
func webSearchTools(ctx context.Context) []einotool.BaseTool {
	search := webSearchFrom(ctx)
	if search == nil {
		return nil
	}
	return []einotool.BaseTool{NewWebSearchTool(search)}
}

const (
	// webSearchMaxQueries bounds the queries one call may issue — the provider
	// is an external, rate-limited API, so a batch stays small.
	webSearchMaxQueries = 3
	// webSearchMaxResults caps the merged hit list, and webSearchSnippetRunes
	// caps each snippet, so a verbose provider cannot flood the ReAct context.
	webSearchMaxResults   = 12
	webSearchSnippetRunes = 1500
)

// webSearchArgs is the JSON the model sends into InvokableRun.
type webSearchArgs struct {
	Queries []string `json:"queries"`
}

// WebSearchTool runs web searches through an injected provider. It holds the
// provider only — no tenant or dataset scope, since the internet is not scoped.
type WebSearchTool struct {
	search WebSearchFunc
}

// NewWebSearchTool returns a WebSearchTool backed by the given provider.
func NewWebSearchTool(search WebSearchFunc) *WebSearchTool {
	return &WebSearchTool{search: search}
}

// Info returns the tool's metadata for the chat model.
func (w *WebSearchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	schemaJSON := `{
  "type": "object",
  "properties": {
    "queries": {
      "type": "array",
      "description": "REQUIRED: 1-3 short web search queries. Phrase each as you would type it into a search engine: distinctive proper nouns, exact titles or dates first, no question-length prose.",
      "items": { "type": "string" },
      "minItems": 1,
      "maxItems": 3
    }
  },
  "required": ["queries"]
}`
	s := &jsonschema.Schema{}
	if err := json.Unmarshal([]byte(schemaJSON), s); err != nil {
		return nil, fmt.Errorf("web_search: parse schema: %w", err)
	}
	return &schema.ToolInfo{
		Name:        webSearchToolName,
		Desc:        webSearchToolDescription,
		ParamsOneOf: schema.NewParamsOneOfByJSONSchema(s),
	}, nil
}

// webHit is one rendered hit: the result plus the query that produced it.
type webHit struct {
	query   string
	title   string
	url     string
	content string
}

// InvokableRun searches the web for each query and returns the merged hits as
// one XML document. Queries run sequentially (the provider is rate-limited) and
// a single failing query is reported inline instead of aborting the whole call:
// one bad query must not cost the model the evidence the others returned. Only
// when every query fails is an error returned, so the model learns the
// capability itself is down.
func (w *WebSearchTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...einotool.Option) (string, error) {
	return guardedToolRun(ctx, webSearchToolName, w.invokableRun, argumentsInJSON)
}

func (w *WebSearchTool) invokableRun(ctx context.Context, argumentsInJSON string) (string, error) {
	if w.search == nil {
		return "", fmt.Errorf("web_search: no web search provider is configured for this conversation")
	}
	var args webSearchArgs
	if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
		return "", fmt.Errorf("web_search: parse arguments: %w", err)
	}

	queries := make([]string, 0, len(args.Queries))
	for _, q := range args.Queries {
		if q = strings.TrimSpace(q); q != "" {
			queries = append(queries, q)
		}
	}
	queries = dedupStrings(queries)
	if len(queries) == 0 {
		return "", fmt.Errorf("web_search: queries must contain 1-%d non-empty queries", webSearchMaxQueries)
	}
	if len(queries) > webSearchMaxQueries {
		return "", fmt.Errorf("web_search: queries must contain at most %d queries, got %d", webSearchMaxQueries, len(queries))
	}

	var hits []webHit
	var notices []string
	var failures []string
	for _, q := range queries {
		results, err := w.search(ctx, q)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s", q, err.Error()))
			// Canonical failure element (severity="warn"): a per-query error
			// is partial — the surviving hits stay usable.
			notices = append(notices, toolErrorXML(webSearchToolName, "warn",
				err.Error(), [2]string{"query", q}))
			continue
		}
		for _, r := range results {
			content := collapseSpaces(strings.TrimSpace(r.Content))
			if content == "" {
				continue
			}
			hits = append(hits, webHit{
				query:   q,
				title:   collapseSpaces(r.Title),
				url:     strings.TrimSpace(r.URL),
				content: truncateRunes(content, webSearchSnippetRunes),
			})
		}
	}
	if len(hits) == 0 && len(failures) > 0 {
		return "", fmt.Errorf("web_search: every query failed: %s", strings.Join(failures, "; "))
	}
	if len(hits) > webSearchMaxResults {
		hits = hits[:webSearchMaxResults]
	}

	common.DebugCtx(ctx, "agentic_rag: web_search result",
		zap.Strings("queries", queries),
		zap.Int("hits", len(hits)),
		zap.Strings("failures", failures),
	)
	return formatWebResultsXML(ctx, strings.Join(queries, " | "), hits, notices), nil
}

// formatWebResultsXML renders the web_search payload: the same compact XML
// vocabulary as the locate tools, with url/title attributes instead of
// corpus identifiers, since a web hit has no chunk_id or doc_id to deep-read.
// notices (non-empty only when a query errored) are emitted right after the
// opening tag so a partial failure is visible rather than fatal.
func formatWebResultsXML(ctx context.Context, query string, hits []webHit, notices []string) string {
	registry := evidenceRegistryFrom(ctx)
	var b strings.Builder
	fmt.Fprintf(&b, "<web_results count=\"%d\" query=\"%s\">\n", len(hits), xmlEscape(query))
	for _, n := range notices {
		b.WriteString(n)
		b.WriteString("\n")
	}
	for i, h := range hits {
		fmt.Fprintf(&b, "<result rank=\"%d\" query=\"%s\" url=\"%s\" title=\"%s\"",
			i+1, xmlEscape(h.query), xmlEscape(h.url), xmlEscape(h.title))
		if ref := registry.StampWebResult(WebResult{Title: h.title, URL: h.url, Content: h.content}); ref >= 0 {
			fmt.Fprintf(&b, " ref=\"%d\"", ref)
		}
		fmt.Fprintf(&b, ">%s</result>\n", xmlEscape(h.content))
	}
	b.WriteString("</web_results>")
	return b.String()
}
