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
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/entity"
)

// processTemplateDSL extracts the DSL from raw template JSON content
// and patches the chunker params from parserConfig. This is the pure
// logic shared between the DB-based loader and tests.
//
// rawTemplate is the full template JSON as stored in agent/templates/
// (contains id, title, dsl, etc.).
//
// Returns (nil, nil) when the DSL key is missing or malformed.
func processTemplateDSL(rawTemplate []byte, parserConfig entity.JSONMap) ([]byte, error) {
	var tpl map[string]any
	if err := json.Unmarshal(rawTemplate, &tpl); err != nil {
		return nil, fmt.Errorf("parse template JSON: %w", err)
	}

	dslRaw, ok := tpl["dsl"]
	if !ok {
		return nil, nil
	}
	dsl, ok := dslRaw.(map[string]any)
	if !ok {
		return nil, nil
	}

	patchTemplateChunker(dsl, parserConfig)

	return json.Marshal(dsl)
}

// patchTemplateChunker finds the chunker component (TokenChunker or
// TitleChunker) in the DSL and applies parser_config overrides.
//
//   - TokenChunker: patches chunk_token_size, overlapped_percent, and
//     delimiters from ParserConfig, both in component params and in the
//     graph node form.
//   - TitleChunker: left as-is (structural params like levels and regex
//     are defined by the template).
func patchTemplateChunker(dsl map[string]any, parserConfig entity.JSONMap) {
	chunkTokenSize := readParserConfigInt(parserConfig, "chunk_token_num", 512)
	if chunkTokenSize < 32 {
		chunkTokenSize = 512
	}
	overlappedPct := readParserConfigFloat(parserConfig, "overlapped_percent", 0)
	delims := readParserConfigDelimiters(parserConfig)

	comps, _ := dsl["components"].(map[string]any)
	if comps == nil {
		return
	}

	for compID, compVal := range comps {
		compMap, ok := compVal.(map[string]any)
		if !ok {
			continue
		}
		obj, ok := compMap["obj"].(map[string]any)
		if !ok {
			continue
		}
		cn, _ := obj["component_name"].(string)
		if cn != "TokenChunker" {
			continue
		}

		params, ok := obj["params"].(map[string]any)
		if !ok {
			continue
		}

		// Patch component params.
		params["chunk_token_size"] = float64(chunkTokenSize)
		params["overlapped_percent"] = overlappedPct
		params["delimiters"] = delims

		// Patch matching graph node form.
		patchGraphChunkerForm(dsl, compID, chunkTokenSize, overlappedPct, delims)
		break
	}
}

// patchGraphChunkerForm updates the chunker node's form data in
// DSL.graph.nodes so it stays consistent with the component params.
func patchGraphChunkerForm(dsl map[string]any, chunkerID string, tokenSize int, overlappedPct float64, delims []string) {
	graph, _ := dsl["graph"].(map[string]any)
	if graph == nil {
		return
	}
	nodes, _ := graph["nodes"].([]any)
	for _, node := range nodes {
		nodeMap, _ := node.(map[string]any)
		if nodeMap == nil || nodeMap["id"] != chunkerID {
			continue
		}
		data, _ := nodeMap["data"].(map[string]any)
		if data == nil {
			continue
		}
		form, _ := data["form"].(map[string]any)
		if form == nil {
			continue
		}

		form["chunk_token_size"] = float64(tokenSize)
		form["overlapped_percent"] = overlappedPct

		formDelims := make([]map[string]string, 0, len(delims))
		for _, d := range delims {
			if d != "" {
				formDelims = append(formDelims, map[string]string{"value": d})
			}
		}
		form["delimiters"] = formDelims
		break
	}
}

// readParserConfigInt extracts an int value from the parser config.
// Falls back to defaultVal when the key is missing or untyped.
func readParserConfigInt(cfg entity.JSONMap, key string, defaultVal int) int {
	if cfg == nil {
		return defaultVal
	}
	raw, ok := cfg[key]
	if !ok {
		return defaultVal
	}
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	}
	return defaultVal
}

// readParserConfigFloat extracts a float64 value from the parser config.
func readParserConfigFloat(cfg entity.JSONMap, key string, defaultVal float64) float64 {
	if cfg == nil {
		return defaultVal
	}
	raw, ok := cfg[key]
	if !ok {
		return defaultVal
	}
	switch v := raw.(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		n, _ := v.Float64()
		return n
	}
	return defaultVal
}

// readParserConfigDelimiters extracts delimiter configuration from
// the parser config. Mirrors the Python chunk config where
// delimiters are either a list of strings or a single string that
// gets split.
func readParserConfigDelimiters(cfg entity.JSONMap) []string {
	if cfg == nil {
		return defaultDelimiters()
	}
	raw, ok := cfg["delimiters"]
	if !ok {
		return defaultDelimiters()
	}
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	case string:
		parts := strings.Split(v, "\n")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return defaultDelimiters()
}

func defaultDelimiters() []string {
	return []string{"\n"}
}
