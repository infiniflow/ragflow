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

// Package prompts holds LLM prompt templates used by the agent
// components. The strings mirror the canonical Python templates under
// rag/prompts/ — keep them in sync when editing either side.
package prompts

import (
	"fmt"
	"strings"

	ragprompts "ragflow/internal/rag/prompts"
)

// CitationPrompt returns the citation-instruction text. The LLM
// component appends it to the system prompt when LLMParam.Cite is true.
//
// The text lives once, in internal/rag/prompts/citation_prompt.md (a copy of
// rag/prompts/citation_prompt.md), and is delegated to here so the two Go
// columns cannot drift apart again.
//
// Format: [ID:N] inline citation; max 4 per sentence; placed at
// sentence end before punctuation; forbidden format "[ID:0, ID:5, ...]"
// — must be space-separated as "[ID:0][ID:5]".
func CitationPrompt() string {
	return ragprompts.CitationPrompt("")
}

// citationPlusTemplate is the post-stream citation-grounding template,
// mirrored from rag/prompts/citation_plus.md.
//
// Two placeholders are substituted at render time:
//
//	{{ example }}  — output of CitationPrompt() (above)
//	{{ sources }}  — formatted retrieval chunks as
//	                `<ID>: <content>` blocks
//
// Kept as a string template rather than a Jinja2 render. Simple
// string replace is enough for the structured placeholders this
// template uses; the runtime/template_jinja.go gonja fallback is
// available for callers that need it.
const citationPlusTemplate = `You are an agent for adding correct citations to the given text by user.
You are given a piece of text within [ID:<ID>] tags, which was generated based on the provided sources.
However, the sources are not cited in the [ID:<ID>].
Your task is to enhance user trust by generating correct, appropriate citations for this report.

{{ example }}

<context>

{{ sources }}

</context>
`

// CitationPlusPrompt renders the citation-grounding prompt with the
// example + sources placeholders filled in. Returns the rendered
// prompt plus the list of chunk IDs that were injected (used by the
// caller to verify the LLM only cited within the supplied set).
func CitationPlusPrompt(sources []CitationSource) (rendered string, ids []string) {
	var srcBuf strings.Builder
	ids = make([]string, 0, len(sources))
	for _, s := range sources {
		if s.ID == "" || s.Content == "" {
			continue
		}
		fmt.Fprintf(&srcBuf, "ID: %s\n└── Content: %s\n\n", s.ID, s.Content)
		ids = append(ids, s.ID)
	}
	out := citationPlusTemplate
	out = strings.Replace(out, "{{ example }}", CitationPrompt(), 1)
	out = strings.Replace(out, "{{ sources }}", srcBuf.String(), 1)
	return out, ids
}

// CitationSource is the minimal shape CitationPlusPrompt needs to
// render the sources block. The full Chunk type (with document_id,
// score, etc.) lives in the RetrievalService; this stub lets the
// post-stream code compile against a future-compatible shape.
type CitationSource struct {
	ID      string
	Content string
}
