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

package service

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

var (
	promptCache    = make(map[string]string)
	promptMu       sync.RWMutex
	promptsBaseDir string
)

// thinkBlockRE strips think blocks from LLM responses.
//
// This mirrors the Python original
//
//	re.sub(r"^.*</think>", "", ans, flags=re.DOTALL)
//
// exactly: `.*` with re.DOTALL is greedy, so the regex consumes
// everything from the start of the string up to and including the
// LAST </think> on the input. The Go form uses [\s\S] (Go's
// newline-aware any-char) and a greedy `*`. A non-greedy `*?` here
// would diverge for responses containing more than one </think>
// (e.g. malformed streams that re-emit a partial think block
// after the real answer), stripping only the first and leaving the
// rest of the response invisible to the caller.
var thinkBlockRE = regexp.MustCompile(`^[\s\S]*</think>`)

// jsonFenceRE matches markdown code fences around JSON responses.
// Mirrors Python's re.sub(r"(`{3}json\n|`{3}\n*$)", ..., flags=re.DOTALL).
// Note: `\n*` is intentionally narrower than Go's `\s*` — Python only
// matches newlines, not other whitespace, so a closing fence followed
// by trailing spaces (e.g. "```   \n") is left intact.
var jsonFenceRE = regexp.MustCompile("```json\\n|```\\n*$")

func init() {
	// Strategy 1: Check working directory first (most reliable during development/tests)
	cwd, err := os.Getwd()
	if err == nil {
		// Check if CWD has rag/prompts directly
		if _, err := os.Stat(filepath.Join(cwd, "rag", "prompts")); err == nil {
			promptsBaseDir = cwd
			return
		}
		// Walk up from CWD looking for rag/prompts
		dir := cwd
		for dir != "/" && dir != "" {
			if _, err := os.Stat(filepath.Join(dir, "rag", "prompts")); err == nil {
				promptsBaseDir = dir
				return
			}
			dir = filepath.Dir(dir)
		}
	}

	// Strategy 2: Walk up from executable (for production Docker where binary is in /ragflow/bin/)
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for dir != "/" && dir != "" {
			if _, err := os.Stat(filepath.Join(dir, "rag", "prompts")); err == nil {
				promptsBaseDir = dir
				return
			}
			dir = filepath.Dir(dir)
		}
	}

	// Final fallback
	promptsBaseDir = "/ragflow"
}

// LoadPrompt loads a prompt by name from the rag/prompts/ directory.
// It caches loaded prompts for subsequent calls.
// Corresponds to rag/prompts/template.py:load_prompt()
func LoadPrompt(name string) (string, error) {
	promptMu.RLock()
	if cached, ok := promptCache[name]; ok {
		promptMu.RUnlock()
		return cached, nil
	}
	promptMu.RUnlock()

	promptPath := filepath.Join(promptsBaseDir, "rag", "prompts", fmt.Sprintf("%s.md", name))
	content, err := os.ReadFile(promptPath)
	if err != nil {
		return "", fmt.Errorf("prompt file '%s.md' not found in rag/prompts/: %w", name, err)
	}

	cached := strings.TrimSpace(string(content))
	promptMu.Lock()
	promptCache[name] = cached
	promptMu.Unlock()

	return cached, nil
}

// RenderPrompt renders a prompt template with the given variables.
// Supports {{ variable }} and {{ variable | filter(args) }} syntax.
// Corresponds to rag/prompts/generator.py template rendering (Jinja2).
func RenderPrompt(template string, data map[string]interface{}) string {
	// Handle {{ variable | filter(args) }} syntax - capture filter arguments too
	filterPattern := regexp.MustCompile(`\{\{\s*(\w+)\s*\|\s*(\w+)\s*\(\s*([^)]*)\s*\)\s*\}\}`)
	result := filterPattern.ReplaceAllStringFunc(template, func(match string) string {
		matches := filterPattern.FindStringSubmatch(match)
		if len(matches) < 4 {
			return match
		}
		key := matches[1]
		filter := matches[2]
		args := matches[3]
		value := data[key]
		return applyFilter(value, filter, args)
	})

	// Handle simple {{ variable }} syntax
	varPattern := regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)
	result = varPattern.ReplaceAllStringFunc(result, func(match string) string {
		matches := varPattern.FindStringSubmatch(match)
		if len(matches) < 2 {
			return match
		}
		key := matches[1]
		if value, ok := data[key]; ok {
			return fmt.Sprintf("%v", value)
		}
		return match
	})

	return result
}

// applyFilter applies a filter to a value with optional arguments.
func applyFilter(value interface{}, filter string, args string) string {
	switch filter {
	case "join":
		// {{ variable | join(', ') }} - expects value to be a slice, args is the separator
		if slice, ok := value.([]string); ok {
			sep := stripQuotes(strings.TrimSpace(args))
			if sep == "" {
				sep = ", "
			}
			return strings.Join(slice, sep)
		}
		return fmt.Sprintf("%v", value)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// stripQuotes removes matching surrounding single or double quotes.
func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
