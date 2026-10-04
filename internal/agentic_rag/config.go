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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"ragflow/internal/common"
)

// Template is one agent configuration: its identity plus the tunables that were
// previously hardcoded — the system prompt (content) and the ordered tool set.
// Putting these in a file lets operators experiment with different prompts
// and tool subsets without recompiling (or even restarting) RAGFlow.
type Template struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	// Temperature pins the sampling temperature of the model this agent runs
	// on. A producer's declaration is the operator's reasoning-strategy choice
	// for that template - smart-reasoning pins 0.5 after the un-pinned
	// model-default runs proved noisier than the exploration its sampling
	// buys. An undeclared template leaves the field unset: the model's own
	// default applies.
	Temperature *float64 `json:"temperature" yaml:"temperature"`
	Tools       []string `json:"tools" yaml:"tools"`
	Content     string   `json:"content" yaml:"content"`
}

// configFile is the on-disk shape: a plain list of templates. Which template
// runs is decided solely by each request's agent_mode (see resolveTemplateFor).
// The yaml tags are required: without them yaml.v3 matches field names
// case-insensitively rather than via snake_case keys.
type configFile struct {
	Templates []Template `json:"templates" yaml:"templates"`
}

// toolFactory builds a tool.BaseTool scoped to the given tenant/datasets. The
// retrieval tools need tenantID/datasetIDs; the reasoning/sandbox tools ignore
// them.
type toolFactory func(tenantID string, datasetIDs []string) tool.BaseTool

// toolRegistry maps every supported tool name to its constructor. Keeping this
// here means adding a new tool is a one-line registration, and the config drives
// which subset is actually loaded. web_search is NOT in this map and must not
// appear in a template's tools list: it is injected by Run when the conversation
// has a search provider.
func toolRegistry() map[string]toolFactory {
	return map[string]toolFactory{
		"think":              func(_ string, _ []string) tool.BaseTool { return NewThinkTool() },
		"todo_write":         func(_ string, _ []string) tool.BaseTool { return NewTodoWriteTool() },
		"run_javascript":     func(_ string, _ []string) tool.BaseTool { return NewRunJavascriptTool() },
		"grep_chunks":        func(t string, d []string) tool.BaseTool { return NewGrepChunksTool(t, d) },
		"search_chunks":      func(t string, d []string) tool.BaseTool { return NewSearchChunksTool(t, d) },
		"search_bm25_chunks": func(t string, d []string) tool.BaseTool { return NewSearchBm25ChunksTool(t, d) },
		// The pure-vector leg: same payload as search_chunks, no keyword leg at
		// all (see tool_search_semantic_chunks.go).
		"search_semantic_chunks": func(t string, d []string) tool.BaseTool { return NewSearchSemanticChunksTool(t, d) },
		"list_chunks":            func(t string, d []string) tool.BaseTool { return NewListChunksTool(t, d) },
	}
}

// defaultConfigPath is the on-disk location of the agent config. It can be
// overridden with the AGENTIC_RAG_CONFIG environment variable (e.g. to point at
// a scratch file while iterating on benchmarks).
func defaultConfigPath() string {
	if p := os.Getenv("AGENTIC_RAG_CONFIG"); p != "" {
		return p
	}
	return "conf/agentic_rag.yaml"
}

// cachedConfig reloads the config file when its mtime changes, so editing the file
// takes effect on the next agent run without a process restart. A single
// misbehaving read (missing/unparseable file) keeps serving the last good
// config — we never let a bad config break the agent.
var (
	configMu      sync.Mutex
	cachedFile    *configFile
	cachedModTime time.Time
	cachedPath    string
)

// LoadConfigFile reads and parses the agent config file. On any error it
// returns nil so the caller falls back to the embedded defaults.
func LoadConfigFile(path string) (*configFile, error) {
	common.Debug("agentic_rag: load config", zap.String("path", path))
	data, err := os.ReadFile(path)
	if err != nil {
		common.Warn("agentic_rag: cannot read config, using fallback",
			zap.String("path", path), zap.Error(err))
		return nil, err
	}
	var f configFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		common.Warn("agentic_rag: cannot parse config, using fallback",
			zap.String("path", path), zap.Error(err))
		return nil, err
	}
	return &f, nil
}

// GetConfigFile returns the parsed config file, reloading from disk when the
// file has changed since the last successful read.
func GetConfigFile() *configFile {
	path := defaultConfigPath()
	info, statErr := os.Stat(path)

	configMu.Lock()
	defer configMu.Unlock()

	changed := cachedFile == nil ||
		statErr != nil ||
		filepath.Clean(path) != cachedPath ||
		(statErr == nil && info.ModTime().After(cachedModTime))
	if !changed {
		return cachedFile
	}

	f, err := LoadConfigFile(path)
	if err != nil || f == nil {
		// Keep the last good config if we have one; otherwise leave nil so the
		// caller uses fallbackConfig().
		if cachedFile != nil {
			return cachedFile
		}
		return nil
	}
	cachedFile = f
	if statErr == nil {
		cachedModTime = info.ModTime()
		cachedPath = filepath.Clean(path)
	}
	common.Info("agentic_rag: config file (re)loaded",
		zap.String("path", path),
		zap.Int("templates", len(f.Templates)))
	return cachedFile
}

// resolveTemplateFor returns the template whose id was explicitly requested
// (from the request's agent_mode). There is intentionally no default_id, env
// override, or first-template fallback: if the request does not name an id
// that exists in conf/agentic_rag.yaml, this is a hard error so misconfiguration
// surfaces loudly instead of silently running a different agent.
func resolveTemplateFor(id string) (Template, error) {
	if id == "" {
		return Template{}, errors.New("agentic_rag: no template requested (empty agent_mode)")
	}
	if f := GetConfigFile(); f != nil {
		for _, t := range f.Templates {
			if t.ID == id {
				return t, nil
			}
		}
	}
	return Template{}, fmt.Errorf("agentic_rag: template %q not found in %s", id, defaultConfigPath())
}

// TemplateTemperature resolves a template's declared sampling temperature:
// nil when the template does not declare one - the caller then leaves the
// temperature field unset and the model's own default applies. The value is
// the template's own (an operator knob in agentic_rag.yaml, reloaded from
// disk like every other template field), never a number hardcoded here.
func TemplateTemperature(templateID string) *float64 {
	tmpl, err := resolveTemplateFor(templateID)
	if err != nil || tmpl.Temperature == nil {
		return nil
	}
	t := *tmpl.Temperature
	return &t
}

// instructionFor resolves the system instruction: prefer the JSON-configured
// prompt, otherwise the embedded Prompt().
func instructionFor(t Template) string {
	if t.Content != "" {
		return t.Content
	}
	return Prompt()
}

// toolsFor builds the agent tool set from the template's tool list, falling back
// to the full default set when the template lists none. Unknown names are
// skipped with a warning so a typo doesn't silently drop a tool.
func toolsFor(t Template, tenantID string, datasetIDs []string) []tool.BaseTool {
	reg := toolRegistry()
	names := t.Tools
	if len(names) == 0 {
		names = make([]string, 0, len(reg))
		for n := range reg {
			names = append(names, n)
		}
	}
	out := make([]tool.BaseTool, 0, len(names))
	for _, name := range names {
		f, ok := reg[name]
		if !ok {
			common.Warn("agentic_rag: unknown tool name in config, skipping",
				zap.String("tool", name))
			continue
		}
		out = append(out, f(tenantID, datasetIDs))
	}
	return out
}
