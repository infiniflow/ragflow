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
	promptCache   = make(map[string]string)
	promptMu      sync.RWMutex
	promptsBaseDir string
)

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
			sep := strings.TrimSpace(args)
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
