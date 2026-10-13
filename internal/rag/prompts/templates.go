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
	"regexp"
	"strings"
)

// Prompt templates for retrieval, chat, and ingestion are embedded here so
// callers can load them independently of the working directory.
//
// action_set is the ONE template that is not part of a system prompt: the harness
// appends it to a session's seed message, and only for a direction that is a SET
// (see enumerates). Strategy text that is always in the prompt is paid for by
// every question, including the ones it cannot help.

//go:embed action_run.md action_set.md action_initialize_state.md sca_select.md sca_query_rewrite.md citation_prompt.md
//go:embed ask_summary.md keyword_prompt.md cross_languages_sys_prompt.md cross_languages_user_prompt.md meta_filter.md related_question.md vision_llm_describe_prompt.md vision_llm_figure_describe_prompt.md
var templatesFS embed.FS

// EmbeddedPromptLoader serves the bundled .md templates by name. Load returns
// an error when the named template is absent, so a caller's fallback path
// engages exactly as with the previous per-package loaders.
type EmbeddedPromptLoader struct{}

// Load implements the harness PromptLoader contract: Load(name string)
// (string, error), reading "<name>.md" and returning its content trimmed.
func (EmbeddedPromptLoader) Load(name string) (string, error) {
	data, err := templatesFS.ReadFile(name + ".md")
	if err != nil {
		return "", fmt.Errorf("internal/rag/prompts: template %q: %w", name, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// Loader is the minimal template-source contract used by Render. It mirrors the
// harness PromptLoader shape (Load(name string) (string, error)) so any loader
// wired into the harness (including EmbeddedPromptLoader and the in-memory test
// loader) satisfies it without the prompts package importing harness.
type Loader interface {
	Load(name string) (string, error)
}

// leftoverVarRE matches a Jinja variable placeholder that no provided var
// filled in. Python renders these through PROMPT_JINJA_ENV's default Undefined
// (SandboxedEnvironment without undefined=StrictUndefined), which interpolates
// as the empty string; the regex mirrors that instead of leaving {{ var }}
// verbatim in the prompt.
var leftoverVarRE = regexp.MustCompile(`\{\{\s*[A-Za-z_][A-Za-z0-9_.]*\s*\}\}`)

// Render loads `name` and substitutes the {{var}} placeholders. A nil loader
// defaults to the embedded templates; a loader that cannot resolve `name`
// falls back to `fallback`. Unprovided variables render as the empty string
// (Jinja default Undefined), not as a literal placeholder.
func Render(loader Loader, name, fallback string, vars map[string]string) string {
	if loader == nil {
		loader = EmbeddedPromptLoader{}
	}
	tmpl := fallback
	if t, err := loader.Load(name); err == nil && strings.TrimSpace(t) != "" {
		tmpl = t
	}
	for k, v := range vars {
		// Jinja tolerates both `{{var}}` and `{{ var }}`; the project's
		// templates use the spaced form, so substitute both.
		tmpl = strings.ReplaceAll(tmpl, "{{"+k+"}}", v)
		tmpl = strings.ReplaceAll(tmpl, "{{ "+k+" }}", v)
	}
	return leftoverVarRE.ReplaceAllString(tmpl, "")
}
