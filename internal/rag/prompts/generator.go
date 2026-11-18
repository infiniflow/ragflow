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

// Package prompts serves the embedded prompt templates (see templates.go) and
// the citation-rules text the final answer must follow.
package prompts

import (
	"fmt"
	"strings"
)

// citationIDSuffix mirrors the tail rag/prompts/generator.citation_prompt
// appends after rendering CITATION_PROMPT_TEMPLATE: the examples in the
// template carry illustrative IDs, and without this the model can emit them
// instead of the real chunk IDs it was given.
const citationIDSuffix = "\n\nIMPORTANT: The example IDs above (45, 46, 78, etc.) are illustrative only. Use the actual chunk IDs from the provided knowledge blocks." +
	" Cite each source INDIVIDUALLY as [ID:i][ID:j]; NEVER merge consecutive citations into a range such as [ID:i-j]."

// CitationPrompt mirrors Python rag/prompts/generator.citation_prompt: return
// the citation-rules text the final answer must follow. The user-defined
// override (citation_guidelines) REPLACES the template but Python still
// appends the illustrative-IDs caveat to it (generator.py:227-228 — the
// suffix is concatenated after whatever template rendered), so the same
// caveat is appended here in both cases.
func CitationPrompt(userDefined string) string {
	if strings.TrimSpace(userDefined) != "" {
		return userDefined + citationIDSuffix
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
