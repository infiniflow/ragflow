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

// Prompt template loading, the Go mirror of Python rag/prompts/template.py
// ::load_prompt. Python reads {name}.md from rag/prompts/ at import time; the
// Go column keeps the authoritative copy of each template as a .md embedded
// here so the binary is self-contained and survives the Python source being
// deleted.
//
// The templates mirrored by this package (action_run, action_initialize_state,
// consumed by the harness action session; sca_select and sca_query_rewrite,
// consumed by the orchestrator) were previously duplicated as copies under
// harness/prompts/. They are now unified here, with the harness wiring its
// PromptLoader default to this package's embedded loader.

//go:embed action_run.md action_initialize_state.md sca_select.md sca_query_rewrite.md citation_prompt.md
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

// Render loads `name` and substitutes the {{var}} placeholders the way
// PROMPT_JINJA_ENV.from_string(...).render(...) does on the Python side. A nil
// loader defaults to the embedded canonical .md templates (1:1 copies of the
// Python rag/prompts/*.md); a loader that cannot resolve `name` falls back to
// `fallback`. Unprovided variables render as the empty string (Jinja default
// Undefined), not as a literal placeholder.
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
