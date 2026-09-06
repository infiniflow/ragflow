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

package pipeline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const compilerComponentPrefix = "Compiler:"

// AugmentBuiltinDSLWithCompiler injects optional Compiler components referenced
// by parser_config into a built-in ingestion DSL. Built-in templates ship
// without a Compiler node; dataset settings may add one under a stable
// Compiler:* key so wiki compilation can run without a custom pipeline.
func AugmentBuiltinDSLWithCompiler(dslJSON string, parserConfig map[string]any) (string, error) {
	compilerIDs := compilerOperatorIDs(parserConfig)
	if len(compilerIDs) == 0 {
		return dslJSON, nil
	}

	var dsl map[string]any
	if err := json.Unmarshal([]byte(dslJSON), &dsl); err != nil {
		return "", fmt.Errorf("augment builtin dsl: decode: %w", err)
	}
	components, ok := dsl["components"].(map[string]any)
	if !ok || components == nil {
		return "", fmt.Errorf("augment builtin dsl: missing components")
	}

	for _, compilerID := range compilerIDs {
		if _, exists := components[compilerID]; exists {
			continue
		}
		if err := injectCompilerComponent(dsl, components, compilerID); err != nil {
			return "", err
		}
	}

	out, err := json.Marshal(dsl)
	if err != nil {
		return "", fmt.Errorf("augment builtin dsl: encode: %w", err)
	}
	return string(out), nil
}

func compilerOperatorIDs(parserConfig map[string]any) []string {
	if len(parserConfig) == 0 {
		return nil
	}
	ids := make([]string, 0, len(parserConfig))
	seen := make(map[string]struct{}, len(parserConfig))
	for key := range parserConfig {
		if !strings.HasPrefix(key, compilerComponentPrefix) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		ids = append(ids, key)
	}
	return ids
}

func injectCompilerComponent(dsl map[string]any, components map[string]any, compilerID string) error {
	predecessorID, tokenizerID := findCompilerInsertionPoint(components)
	if predecessorID == "" {
		return fmt.Errorf("augment builtin dsl: cannot locate insertion point for %s", compilerID)
	}

	components[compilerID] = map[string]any{
		"downstream": downstreamIDs(components, predecessorID, tokenizerID),
		"upstream":   []string{predecessorID},
		"obj": map[string]any{
			"component_name": "Compiler",
			"params": map[string]any{
				"compilation_template_group_id": "",
				"llm_id":                      "",
				"outputs": map[string]any{
					"chunks": map[string]any{
						"type":  "Array<Object>",
						"value": []any{},
					},
				},
			},
		},
	}

	if pred, ok := components[predecessorID].(map[string]any); ok {
		pred["downstream"] = []string{compilerID}
	}
	if tokenizerID != "" {
		if tok, ok := components[tokenizerID].(map[string]any); ok {
			tok["upstream"] = []string{compilerID}
		}
	}

	patchGraphForCompiler(dsl, predecessorID, compilerID, tokenizerID)
	return nil
}

func downstreamIDs(components map[string]any, predecessorID, tokenizerID string) []string {
	if tokenizerID != "" {
		return []string{tokenizerID}
	}
	if pred, ok := components[predecessorID].(map[string]any); ok {
		if raw, ok := pred["downstream"].([]any); ok {
			out := make([]string, 0, len(raw))
			for _, item := range raw {
				if s, ok := item.(string); ok && s != "" {
					out = append(out, s)
				}
			}
			return out
		}
		if raw, ok := pred["downstream"].([]string); ok {
			return raw
		}
	}
	return []string{}
}

func findCompilerInsertionPoint(components map[string]any) (predecessorID, tokenizerID string) {
	for id, raw := range components {
		comp, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if componentName(comp) != "Tokenizer" {
			continue
		}
		tokenizerID = id
		upstream := stringSliceField(comp, "upstream")
		if len(upstream) > 0 {
			predecessorID = upstream[0]
		}
		return predecessorID, tokenizerID
	}

	// Fallback: wire after the last non-terminal node in the chain.
	for id, raw := range components {
		comp, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := componentName(comp)
		if name == "File" || strings.HasPrefix(id, compilerComponentPrefix) {
			continue
		}
		downstream := stringSliceField(comp, "downstream")
		if len(downstream) == 0 {
			return id, ""
		}
	}
	return "", ""
}

func componentName(comp map[string]any) string {
	obj, ok := comp["obj"].(map[string]any)
	if !ok {
		return ""
	}
	name, _ := obj["component_name"].(string)
	return strings.TrimSpace(name)
}

func stringSliceField(comp map[string]any, key string) []string {
	raw, ok := comp[key]
	if !ok {
		return nil
	}
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func patchGraphForCompiler(dsl map[string]any, predecessorID, compilerID, tokenizerID string) {
	graph, ok := dsl["graph"].(map[string]any)
	if !ok || graph == nil {
		return
	}
	edges, ok := graph["edges"].([]any)
	if !ok {
		return
	}

	newEdges := make([]any, 0, len(edges)+1)
	replaced := false
	for _, raw := range edges {
		edge, ok := raw.(map[string]any)
		if !ok {
			newEdges = append(newEdges, raw)
			continue
		}
		source, _ := edge["source"].(string)
		target, _ := edge["target"].(string)
		if tokenizerID != "" && source == predecessorID && target == tokenizerID {
			predToCompiler := map[string]any{
				"id":           fmt.Sprintf("xy-edge__%sstart-%send", predecessorID, compilerID),
				"source":       predecessorID,
				"sourceHandle": "start",
				"target":       compilerID,
				"targetHandle": "end",
			}
			compilerToTokenizer := map[string]any{
				"id":           fmt.Sprintf("xy-edge__%sstart-%send", compilerID, tokenizerID),
				"source":       compilerID,
				"sourceHandle": "start",
				"target":       tokenizerID,
				"targetHandle": "end",
			}
			newEdges = append(newEdges, predToCompiler, compilerToTokenizer)
			replaced = true
			continue
		}
		newEdges = append(newEdges, edge)
	}
	if !replaced && tokenizerID == "" {
		newEdges = append(newEdges, map[string]any{
			"id":           fmt.Sprintf("xy-edge__%sstart-%send", predecessorID, compilerID),
			"source":       predecessorID,
			"sourceHandle": "start",
			"target":       compilerID,
			"targetHandle": "end",
		})
	}
	graph["edges"] = newEdges

	nodes, ok := graph["nodes"].([]any)
	if !ok {
		return
	}
	graph["nodes"] = append(nodes, map[string]any{
		"id": compilerID,
		"data": map[string]any{
			"label": "Compiler",
			"name":  "Wiki",
		},
		"type": "compilationNode",
	})
}
