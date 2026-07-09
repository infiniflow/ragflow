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

package task

import (
	"fmt"

	"ragflow/internal/common"
)

// NormalizeChunks converts pipeline output into a uniform []map[string]any slice.
// Mirrors Python: DataflowService._normalize_chunks()
func NormalizeChunks(output map[string]any) []map[string]any {
	if output == nil {
		return nil
	}

	if chunks, ok := output["chunks"].([]map[string]any); ok {
		return deepCopyChunks(chunks)
	}
	if chunks, ok := toChunkMaps(output["chunks"]); ok {
		return deepCopyChunks(chunks)
	}
	if json, ok := output["json"].([]map[string]any); ok {
		return deepCopyChunks(json)
	}
	if json, ok := toChunkMaps(output["json"]); ok {
		return deepCopyChunks(json)
	}
	if md, ok := output["markdown"].(string); ok && md != "" {
		return []map[string]any{{"text": md}}
	}
	if txt, ok := output["text"].(string); ok && txt != "" {
		return []map[string]any{{"text": txt}}
	}
	if html, ok := output["html"].(string); ok && html != "" {
		return []map[string]any{{"text": html}}
	}
	return nil
}

func toChunkMaps(v any) ([]map[string]any, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, false
		}
		out = append(out, m)
	}
	return out, true
}

// deepCopyChunks returns a deep copy of the chunk slice and each chunk map.
// Slice values (e.g. []float64 vectors) are fully copied, not shared.
// Mirrors Python: copy.deepcopy()
func deepCopyChunks(chunks []map[string]any) []map[string]any {
	if chunks == nil {
		return nil
	}
	out := make([]map[string]any, len(chunks))
	for i, c := range chunks {
		cp := make(map[string]any, len(c))
		for k, v := range c {
			switch val := v.(type) {
			case []float64:
				vec := make([]float64, len(val))
				copy(vec, val)
				cp[k] = vec
			case []int:
				sl := make([]int, len(val))
				copy(sl, val)
				cp[k] = sl
			case []string:
				sl := make([]string, len(val))
				copy(sl, val)
				cp[k] = sl
			default:
				cp[k] = v
			}
		}
		out[i] = cp
	}
	return out
}

// PrepareTextsForDataflowEmbedding extracts texts for embedding from chunks.
// Priority: questions > summary > text.
// Mirrors Python: EmbeddingUtils.prepare_texts_for_dataflow_embedding()
func PrepareTextsForDataflowEmbedding(chunks []map[string]any) []string {
	if chunks == nil {
		return nil
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		text, _ := chunk["questions"].(string)
		if text == "" {
			text, _ = chunk["summary"].(string)
		}
		if text == "" {
			text = MustGetChunkTextString(chunk, "PrepareTextsForDataflowEmbedding")
		}
		texts = append(texts, text)
	}
	return texts
}

// MustGetChunkTextString returns chunk["text"] when it is a string.
// Missing text is allowed and returns empty string.
// FIXME: remove panic before production; current panic is intentional for dev/test
// so list-shaped text payloads are surfaced immediately instead of being written
// as silent bad data.
func MustGetChunkTextString(chunk map[string]any, where string) string {
	val, exists := chunk["text"]
	if !exists || val == nil {
		return ""
	}
	text, ok := val.(string)
	if ok {
		return text
	}

	msg := fmt.Sprintf("%s: invalid chunk text type %T, expected string, chunk=%v", where, val, chunk)
	common.Error(msg, nil)
	panic(msg)
}
