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

package prompts

import (
	"strings"
	"testing"
)

func TestCitationPromptCarriesIllustrativeIDCaveat(t *testing.T) {
	// rag/prompts/generator.py::citation_prompt appends this after rendering
	// CITATION_PROMPT_TEMPLATE; without it the model can echo the examples' IDs.
	got := CitationPrompt("")
	if !strings.Contains(got, "illustrative only") {
		t.Errorf("citation prompt missing the illustrative-ID caveat; got tail %q", got[len(got)-80:])
	}
	if !strings.Contains(got, "actual chunk IDs") {
		t.Errorf("citation prompt missing the chunk-ID instruction: %q", got[:120])
	}
	if strings.TrimSpace(got) != got {
		t.Errorf("citation prompt has surrounding whitespace")
	}
}

func TestCitationPromptHonoursOverride(t *testing.T) {
	// Python generator.py:227-228 renders the override (citation_guidelines)
	// template and STILL appends the illustrative-IDs caveat unconditionally —
	// the suffix is concatenated after whatever template rendered, so the
	// override replaces the template but never the caveat.
	want := "Cite as [n]." + citationIDSuffix
	if got := CitationPrompt("Cite as [n]."); got != want {
		t.Errorf("CitationPrompt(override) = %q, want override + caveat %q", got, want)
	}
}
