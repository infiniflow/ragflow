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

// memory_compile.go — memory extraction runs the knowledge-compilation
// pipelines.
//
// There is no memory-specific extractor: a memory type is one of the built-in
// compilation templates that ship YAML next to the other kinds
// (internal/ingestion/knowledge_compile/templates/):
//
//	memory_notes  — entities, facts and relations of the conversation plus the
//	                per-entity essence paragraph. It replaces the old
//	                semantic / episodic / procedural split, which only differed
//	                in prompt wording and cost one LLM call each.
//	memory_facts  — the hindsight-style fact graph: one row per
//	                subject+predicate with polarity and a validity window, plus
//	                restates / changes / ends relations to the fact it supersedes.
//
// The conversation is fed as a single chunk into structure.Run, so everything
// the compiler already knows (batching, evidence gating, LLM-judged merge of
// duplicate rows, entity aliasing, relation rewriting) applies to memory too.

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
	"ragflow/internal/ingestion/component/knowledge_compiler/structure"
	"ragflow/internal/ingestion/component/progress"
)

// MemoryCompileKinds lists the built-in pipelines the memory feature runs, in
// canonical order. "raw" is not here: it stores the conversation and needs no
// compile.
var MemoryCompileKinds = []string{"memory_notes", "memory_facts"}

// resolveMemoryCompileKinds maps configured memory types to compile kinds.
//
// nil means "the caller did not choose": the UI no longer offers a choice, so an
// omitted memory_type runs every built-in pipeline. Legacy names
// (semantic/episodic/procedural) resolve to memory_notes. An explicit raw-only
// selection runs nothing, which is the historical "store raw messages only"
// behaviour.
func resolveMemoryCompileKinds(memoryTypes []string) []string {
	if memoryTypes == nil {
		return append([]string(nil), MemoryCompileKinds...)
	}
	wanted := make(map[string]struct{}, len(memoryTypes))
	for _, name := range memoryTypes {
		kind := strings.ToLower(strings.TrimSpace(name))
		if canonical, ok := dao.MemoryTypeAliases[kind]; ok {
			kind = canonical
		}
		if kind == "" || kind == "raw" {
			continue
		}
		wanted[kind] = struct{}{}
	}
	out := make([]string, 0, len(wanted))
	for _, kind := range MemoryCompileKinds {
		if _, ok := wanted[kind]; ok {
			out = append(out, kind)
		}
	}
	return out
}

// builtinMemoryTemplate returns the built-in compilation template for a memory
// kind.
func builtinMemoryTemplate(kind string) (*BuiltinTemplate, error) {
	templates, err := memoryTemplates()
	if err != nil {
		return nil, err
	}
	for _, template := range templates {
		if template != nil && strings.EqualFold(template.Kind, kind) {
			return template, nil
		}
	}
	return nil, fmt.Errorf("built-in memory template %q not found", kind)
}

// memoryTemplates loads the built-in compilation templates, resolving the
// template directory from the working directory or one of its ancestors. The
// shared loader assumes the process runs at the repository root; tests and
// desktop runs do not, and a missing template would silently disable every
// memory pipeline.
func memoryTemplates() ([]*BuiltinTemplate, error) {
	dir, err := findMemoryTemplateDir()
	if err != nil {
		return loadBuiltinTemplates()
	}
	templates, err := loadBuiltinTemplatesFromFS(os.DirFS(dir))
	if err != nil {
		return nil, err
	}
	return templates, nil
}

func findMemoryTemplateDir() (string, error) {
	relative := filepath.Join("internal", "ingestion", "knowledge_compile", "templates")
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for depth := 0; depth < 8; depth++ {
		candidate := filepath.Join(dir, relative)
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("memory template directory not found above %q", relative)
}

// memscope names the compile scope of one memory. The compiler keys its
// dataset-scoped stores (wiki pages, navigation) by dataset id; memory rows are
// not a dataset, so the memory id gives the compile a stable private scope.
func memoryCompileScope(memoryID string) string {
	return "memory:" + memoryID
}

// compileMemoryKind runs one memory pipeline over one conversation.
func (s *MemoryMessageService) compileMemoryKind(ctx context.Context, mem *CreateMemoryResponse, kind, conversation, llmRef, sourceID, taskID string) ([]extractedMemory, error) {
	template, err := builtinMemoryTemplate(kind)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errPermanentMemoryTask, err)
	}
	templateConfig := map[string]any(template.Config)

	deps, err := kccommon.ResolveDeps(ctx, mem.TenantID, llmRef, mem.EmbdID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: resolve compile deps: %w", errPermanentMemoryTask, errModelConfigUnavailable, err)
	}
	deps.TenantID = mem.TenantID
	deps.DatasetID = memoryCompileScope(mem.ID)
	deps.Progress = progress.Window{Span: 1}

	param := kccommon.Param{
		LLMID:          llmRef,
		EmbeddingModel: mem.EmbdID,
		TemplateID:     template.ID,
		TemplateConfig: templateConfig,
		Variant:        kccommon.VariantStructure,
	}.Defaults()

	inputs := kccommon.Inputs{
		Chunks: []kccommon.Chunk{{
			ID:   sourceID,
			Text: conversation,
		}},
		DocID:          sourceID,
		LLMID:          llmRef,
		EmbeddingModel: mem.EmbdID,
		VariantSpecific: map[string]any{
			"parser_config": templateConfig,
		},
	}

	outputs, err := structure.Run(ctx, deps, param, inputs)
	if err != nil {
		return nil, fmt.Errorf("%w: pipeline %s: %w", errMemoryChatCall, kind, err)
	}
	return memoryItemsFromProducts(outputs.Products, kind, sourceID, taskID), nil
}

// memoryItemsFromProducts turns compiled rows into memory items.
//
// memory_facts keeps one row per fact, with the validity window the template
// asked the model for. memory_notes keeps the descriptive text of the extracted
// entities: the essence paragraph, when the template's synthesis produced one,
// is just another row and is kept verbatim.
func memoryItemsFromProducts(products []kccommon.Product, kind, sourceID, taskID string) []extractedMemory {
	items := make([]extractedMemory, 0, len(products))
	for _, product := range products {
		payload := map[string]any{}
		if err := json.Unmarshal([]byte(product.Content), &payload); err != nil || payload == nil {
			continue
		}
		if kind == "memory_facts" {
			if item, ok := memoryFactItem(payload, sourceID); ok {
				items = append(items, item)
			}
			continue
		}
		if item, ok := memoryNoteItem(payload, kind); ok {
			items = append(items, item)
		}
	}
	if len(items) > 0 {
		common.Info(fmt.Sprintf("memory: %s compiled %d item(s) from %s (task %s)", kind, len(items), sourceID, taskID))
	}
	return items
}

// memoryFactItem reads one fact row: subject + predicate + object are the
// identity, polarity and the validity window are the hindsight state.
func memoryFactItem(payload map[string]any, sourceID string) (extractedMemory, bool) {
	if !strings.EqualFold(strings.TrimSpace(anyStringValue(payload["type"])), "fact") {
		return extractedMemory{}, false
	}
	subject := strings.TrimSpace(anyStringValue(payload["subject"]))
	predicate := strings.TrimSpace(anyStringValue(payload["predicate"]))
	object := strings.TrimSpace(anyStringValue(payload["object"]))
	if subject == "" || predicate == "" {
		return extractedMemory{}, false
	}
	text := strings.TrimSpace(subject + " " + predicate + " " + object)
	if strings.EqualFold(strings.TrimSpace(anyStringValue(payload["polarity"])), "negate") {
		text = "NOT: " + text
	}
	return extractedMemory{
		MessageType: "memory_facts",
		Content:     text,
		ValidAt:     strings.TrimSpace(anyStringValue(payload["valid_at"])),
		InvalidAt:   strings.TrimSpace(anyStringValue(payload["invalid_at"])),
	}, true
}

// memoryNoteItem reads one note row: the descriptive text of an extracted
// entity (or a synthesized essence row).
func memoryNoteItem(payload map[string]any, kind string) (extractedMemory, bool) {
	entityType := strings.ToLower(strings.TrimSpace(anyStringValue(payload["type"])))
	if entityType == "relation" {
		return extractedMemory{}, false
	}
	note := strings.TrimSpace(anyStringValue(payload["description"]))
	if note == "" {
		note = strings.TrimSpace(anyStringValue(payload["text"]))
	}
	if note == "" {
		name := strings.TrimSpace(anyStringValue(payload["name"]))
		if name == "" {
			return extractedMemory{}, false
		}
		note = name
	}
	return extractedMemory{
		MessageType: kind,
		Content:     note,
		ValidAt:     strings.TrimSpace(anyStringValue(payload["valid_at"])),
		InvalidAt:   strings.TrimSpace(anyStringValue(payload["invalid_at"])),
	}, true
}

// normalizeFactText is the identity of a fact's content for folding: case and
// whitespace differences are not new knowledge.
func normalizeFactText(text string) string {
	return strings.Join(strings.Fields(strings.ToLower(text)), " ")
}

// factKey identifies one stored fact: its type plus its normalised text.
func factKey(messageType, content string) string {
	return strings.ToLower(strings.TrimSpace(messageType)) + "\x00" + normalizeFactText(content)
}

// dedupeExtractedFacts drops degenerate items and repeats inside one
// extraction. Degenerate means the model returned whitespace or a one-character
// stub: it carries no statement and would only add a noisy row.
func dedupeExtractedFacts(items []extractedMemory) []extractedMemory {
	out := make([]extractedMemory, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if utf8.RuneCountInString(normalizeFactText(item.Content)) < 2 {
			continue
		}
		key := factKey(item.MessageType, item.Content)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		item.Content = strings.TrimSpace(item.Content)
		out = append(out, item)
	}
	return out
}

// factTypesOf returns the distinct message types an extraction produced, so the
// dedupe lookup runs once per type instead of once per item.
func factTypesOf(items []extractedMemory) []string {
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.ToLower(strings.TrimSpace(item.MessageType))
		if name == "" {
			continue
		}
		if _, duplicate := seen[name]; duplicate {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

// dropKnownFacts removes items the memory already stores verbatim.
func dropKnownFacts(items []extractedMemory, known map[string]struct{}) []extractedMemory {
	if len(known) == 0 {
		return items
	}
	out := make([]extractedMemory, 0, len(items))
	for _, item := range items {
		if _, exists := known[factKey(item.MessageType, item.Content)]; exists {
			continue
		}
		out = append(out, item)
	}
	return out
}

// anyStringValue reads a string out of a decoded JSON payload.
func anyStringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case float64:
		return fmt.Sprintf("%v", typed)
	}
	return ""
}
