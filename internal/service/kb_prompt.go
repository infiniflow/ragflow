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
	"strings"

	"ragflow/internal/tokenizer"
)

// ChunksFormat normalizes retrieval chunks into the response format expected by
// the ask endpoint.  It matches the Python chunks_format() in rag/prompts/generator.py.
// Returns an empty slice for nil or empty input.
func ChunksFormat(chunks []SourcedChunk) []map[string]interface{} {
	if len(chunks) == 0 {
		return []map[string]interface{}{}
	}
	out := make([]map[string]interface{}, len(chunks))
	for i, ck := range chunks {
		out[i] = map[string]interface{}{
			"id":                ck.ID,
			"content":           ck.Content,
			"document_id":       ck.DocID,
			"document_name":     ck.DocName,
			"dataset_id":        ck.DatasetID,
			"image_id":          ck.ImageID,
			"positions":         ck.Positions,
			"url":               ck.URL,
			"similarity":        ck.Similarity,
			"vector_similarity": ck.VectorSimilarity,
			"term_similarity":   ck.TermSimilarity,
			"row_id":            ck.ID, // row_id == ID for consistency with Python
			"doc_type":          ck.DocType,
			"document_metadata": ck.DocumentMetadata,
		}
	}
	return out
}

// KbPrompt builds a knowledge-base context string from retrieved chunks for use
// in the LLM system prompt.  Truncation uses the C++ tokenizer when available;
// falls back to a character-based approximation on systems where the tokenizer
// dictionary is not installed.
//
// Corresponds to kb_prompt() in rag/prompts/generator.py.
func KbPrompt(chunks []SourcedChunk, maxTokens int) string {
	if len(chunks) == 0 || maxTokens <= 0 {
		return ""
	}
	const tokenRatio = 0.97
	limit := int(float64(maxTokens) * tokenRatio)

	var b strings.Builder
	used := 0
	for _, ck := range chunks {
		entry := formatChunkEntry(ck)
		tokens := tokenizer.NumTokensFromString(entry)
		if used+tokens > limit {
			break
		}
		b.WriteString(entry)
		used += tokens
	}
	return b.String()
}

// formatChunkEntry renders a single chunk as a tree-structured entry for the
// LLM prompt.  Format matches Python kb_prompt() in rag/prompts/generator.py:
//
//	ID: <id>
//	├── Title: <doc_name>
//	├── URL: <url>
//	├── <metadata_key>: <metadata_value>
//	└── Content:
//	<chunk content>
func formatChunkEntry(ck SourcedChunk) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ID: %s\n", ck.ID)
	if ck.DocName != "" {
		fmt.Fprintf(&b, "├── Title: %s\n", ck.DocName)
	}
	if ck.URL != "" {
		fmt.Fprintf(&b, "├── URL: %s\n", ck.URL)
	}
	if ck.DocumentMetadata != nil {
		for k, v := range ck.DocumentMetadata {
			fmt.Fprintf(&b, "├── %s: %v\n", k, v)
		}
	}
	b.WriteString("└── Content:\n")
	b.WriteString(ck.Content)
	b.WriteString("\n\n")
	return b.String()
}
