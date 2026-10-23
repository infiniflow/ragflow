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
	"embed"
	"fmt"
	"strings"
)

// Prompt templates for retrieval, chat, and ingestion are embedded here so
// callers can load them independently of the working directory.

//go:embed citation_prompt.md ask_summary.md keyword_prompt.md cross_languages_sys_prompt.md cross_languages_user_prompt.md meta_filter.md related_question.md vision_llm_describe_prompt.md vision_llm_figure_describe_prompt.md
var templatesFS embed.FS

// EmbeddedPromptLoader serves the bundled .md templates by name. Load returns
// an error when the named template is absent, so a caller's fallback path
// engages exactly as with the previous per-package loaders.
type EmbeddedPromptLoader struct{}

// Load reads "<name>.md" and returns its content trimmed.
func (EmbeddedPromptLoader) Load(name string) (string, error) {
	data, err := templatesFS.ReadFile(name + ".md")
	if err != nil {
		return "", fmt.Errorf("internal/rag/prompts: template %q: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}
