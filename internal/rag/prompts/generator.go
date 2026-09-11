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

// Package prompts is the Go mirror of Python rag/prompts/generator.py — the
// prompt helpers the agentic loop, the chat seam and the deep parsers share
// (chunk formatting, token-budget fitting, knowledge-block rendering, tool
// schema serialization, history formatting, and the named prompt templates
// that load from rag/prompts/*.md).
//
// Porting is staged function-by-function: pure helpers that do not touch an
// LLM are mirrored first and covered by tests; helpers that call the model
// (Python chat_mdl.async_chat) are left as declared entry points until the Go
// LLM-call seam is confirmed, so they are not part of the compile surface yet.
//
// Template loading mirrors Python rag/prompts/template.py::load_prompt, which
// reads {name}.md from rag/prompts/ at import time. The Go seam for that is a
// named-template loader passed in by callers so this package stays free of a
// hard dependency on the service layer (see the Render* helpers).
package prompts

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"
)

var _LOG = common.StdLogger()

var reNewlines = regexp.MustCompile(`\n+`)

// GetValue mirrors Python get_value(d, k1, k2): d.get(k1, d.get(k2)). The first
// key present in the map wins; if neither is present it returns nil. Unlike a
// plain two-lookup, a key that is present with a nil value is returned as nil
// (it is not skipped), matching Python's dict.get semantics.
func GetValue(d map[string]any, k1, k2 string) any {
	if v, ok := d[k1]; ok {
		return v
	}
	if v, ok := d[k2]; ok {
		return v
	}
	return nil
}

// ChunksFormat mirrors Python chunks_format(reference): it projects a
// knowledge block's chunk list into a canonical per-chunk reference map,
// applying Python's dual-name fallback for each field. A nil or non-dict
// reference (nil in Go), or a chunks field that is not a list, yields an empty
// slice.
func ChunksFormat(reference map[string]any) []map[string]any {
	if reference == nil {
		return []map[string]any{}
	}
	raw, ok := reference["chunks"].([]any)
	if !ok {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		chunk, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"id":                GetValue(chunk, "chunk_id", "id"),
			"content":           GetValue(chunk, "content", "content_with_weight"),
			"document_id":       GetValue(chunk, "doc_id", "document_id"),
			"document_name":     GetValue(chunk, "docnm_kwd", "document_name"),
			"dataset_id":        GetValue(chunk, "kb_id", "dataset_id"),
			"image_id":          GetValue(chunk, "image_id", "img_id"),
			"positions":         GetValue(chunk, "positions", "position_int"),
			"url":               chunk["url"],
			"similarity":        chunk["similarity"],
			"vector_similarity": chunk["vector_similarity"],
			"term_similarity":   chunk["term_similarity"],
			"row_id":            chunk["row_id"],
			"doc_type":          GetValue(chunk, "doc_type_kwd", "doc_type"),
			"document_metadata": chunk["document_metadata"],
		})
	}
	return out
}

// DefaultCiteRules mirrors rag/prompts/generator.citation_prompt's
// CITATION_PROMPT_TEMPLATE output. Python loads citation_prompt.md via
// load_prompt("citation_prompt"); the Go build keeps the same .md embedded
// (citation_prompt.md) and reads it here so the template has a single source of
// truth. A user-defined override (RAGConfig.CiteRules / user_defined_prompts)
// replaces it verbatim when present — see CitationPrompt.
//
// Loaded from the embedded md rather than a string literal so edits to
// citation_prompt.md propagate without a second copy to keep in sync.
var DefaultCiteRules = CitationPrompt("")

// citationIDSuffix mirrors the tail rag/prompts/generator.citation_prompt
// appends after rendering CITATION_PROMPT_TEMPLATE: the examples in the
// template carry illustrative IDs, and without this the model can emit them
// instead of the real chunk IDs it was given.
const citationIDSuffix = "\n\nIMPORTANT: The example IDs above (45, 46, 78, etc.) are illustrative only. Use the actual chunk IDs from the provided knowledge blocks."

// CitationPrompt mirrors Python rag/prompts/generator.citation_prompt: return
// the citation-rules text the final answer must follow. With an empty
// userDefined it returns the embedded citation_prompt.md plus the illustrative
// IDs caveat; a non-empty userDefined (the rendered user_defined_prompts)
// replaces it verbatim, exactly as Python returns
// citation_prompt(self.user_defined_prompts).
func CitationPrompt(userDefined string) string {
	if strings.TrimSpace(userDefined) != "" {
		return userDefined
	}
	data, err := templatesFS.ReadFile("citation_prompt.md")
	if err != nil {
		// The md is embedded in the binary; a load failure is a build/packaging
		// defect, not a runtime condition. Panic so it surfaces loudly in tests
		// rather than silently shipping empty citation rules.
		panic(fmt.Sprintf("internal/rag/prompts: citation_prompt.md: %v", err))
	}
	return strings.TrimSpace(string(data)) + citationIDSuffix
}

// kbpBlock renders one knowledge block (id / title / url / metadata / content)
// for the given 1-based index. ok is false when the chunk carries no content,
// which KBPrompt treats as "skip" (mirrors Python's `if not c: continue`).
//
// The block starts with a newline, mirroring Python's `"\nID: {}".format(...)`,
// so a caller that joins the blocks with "\n" renders the same prompt Python
// does.
func kbpBlock(c map[string]any, index int) (string, bool) {
	content := chunkText(c)
	if strings.TrimSpace(content) == "" {
		return "", false
	}
	block := fmt.Sprintf("\nID: %d", index)
	if title := chunkTitle(c); title != "" {
		block += "\n├── Title: " + flattenNewlines(title)
	}
	if url, _ := c["url"].(string); url != "" {
		block += "\n├── URL: " + flattenNewlines(url)
	}
	if meta, ok := c["document_metadata"].(map[string]any); ok {
		keys := make([]string, 0, len(meta))
		for k := range meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if mv := meta[k]; mv != nil {
				if v := flattenNewlines(fmt.Sprint(mv)); v != "" {
					block += "\n├── " + k + ": " + v
				}
			}
		}
	}
	block += "\n└── Content:\n" + content
	return block, true
}

// KBPrompt mirrors rag/prompts/generator.kb_prompt: render chunks into the
// numbered knowledge blocks the citation rules refer to.
//
// Blocks are numbered by position (1-based) so the model's [n] citations match
// the block order it sees, matching Python's `ID: {i}` rendering (hash_id=False).
//
// The budget is applied to the COMPLETE rendered block (title / url / metadata /
// content), measured in TOKENS with the same cl100k_base encoder Python's
// num_tokens_from_string uses: budgeting only the content in a char approximation
// left the decoration unaccounted, so the rendered prompt could exceed the budget
// it claims to enforce.
//
// maxTokens must be positive: Python's caller applies its own budget
// (min(chat_mdl.max_length, _EVIDENCE_BUDGET_TOKENS)) before calling.
func KBPrompt(chunks []map[string]any, maxTokens int) []string {
	blocks, _ := KBPromptWithSourceIndices(chunks, maxTokens)
	return blocks
}

// KBPromptWithSourceIndices is KBPrompt plus, for every rendered block, the
// index in chunks it was rendered from.
//
// Callers that need "the blocks of these chunks" must use the indices instead of
// assuming one block per chunk: a chunk with no content is skipped and the loop
// stops at the token budget, so len(blocks) <= len(chunks) and blocks[i] is not
// chunks[i]. Slicing by a chunk count can therefore drop readable blocks or
// point past the end.
func KBPromptWithSourceIndices(chunks []map[string]any, maxTokens int) ([]string, []int) {
	out := make([]string, 0, len(chunks))
	sources := make([]int, 0, len(chunks))
	used := 0
	for idx, c := range chunks {
		if c == nil {
			continue
		}
		block, ok := kbpBlock(c, len(out)+1)
		if !ok {
			continue
		}
		n := tokenizer.NumTokensFromString(block)
		if float64(maxTokens)*0.97 < float64(used+n) {
			_LOG.Printf("[KBPrompt] Not all the retrieval into prompt: %d/%d", len(out), len(chunks))
			break
		}
		used += n
		out = append(out, block)
		sources = append(sources, idx)
	}
	return out, sources
}

// flattenNewlines mirrors Python kb_prompt's `re.sub(r"\n+", " ", line)`.
func flattenNewlines(s string) string {
	return strings.TrimSpace(reNewlines.ReplaceAllString(s, " "))
}

// chunkText mirrors Python kb_prompt's
// get_value(ck, "content", "content_with_weight").
func chunkText(c map[string]any) string {
	v := GetValue(c, "content", "content_with_weight")
	if v == nil {
		// Neither content field is present: return empty so KBPrompt skips the
		// chunk (Python's `if not c: continue`). fmt.Sprint(nil) would yield the
		// literal "<nil>", which would both render into the prompt and consume
		// the token budget.
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

// chunkTitle mirrors Python kb_prompt's
// get_value(ck, "docnm_kwd", "document_name"), widened with the aliases Go's
// own document aggregations emit (doc_title / title / doc_name) so a title
// stored under any of them still renders.
func chunkTitle(c map[string]any) string {
	for _, k := range []string{"docnm_kwd", "doc_title", "title", "document_name", "doc_name"} {
		if v, ok := c[k]; ok && v != nil {
			if s := fmt.Sprint(v); s != "" {
				return s
			}
		}
	}
	return ""
}
