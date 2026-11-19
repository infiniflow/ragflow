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

package component

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/common"
)

// EmbedChunkText computes the embedding of a single chunk the way the
// tokenizer component does for a whole document, so a vector produced outside
// the ingestion pipeline stays comparable with the ones it wrote:
//
//  1. table-structure tags are stripped and the text trimmed (cleanEmbedText),
//     which is both the emptiness check and the embedded input;
//  2. the text is truncated to the embedder's own input window
//     (trimForEmbedding), the same cut the pipeline applies;
//  3. the document title is embedded and merged with the content vector using
//     filename_embd_weight (mergeEmbeddingVectors), skipping the title when the
//     document name is blank — exactly the call sites' rule.
//
// The embedded field is the chunk text, i.e. the tokenizer's default
// fields=["text"]. A knowledgebase that configured the tokenizer to embed other
// fields (questions, summary, …) is not reproduced here: those inputs are not
// part of the chunk row an editor changes.
//
// It returns the engine vector field name ("q_<dim>_vec") together with the
// vector. An empty field name means the chunk carried nothing to embed (a
// table-only or blank chunk), which the pipeline also skips: the caller has no
// vector to write.
func EmbedChunkText(
	ctx context.Context,
	embedder Embedder,
	name, content string,
	titleWeight float64,
) (string, []float64, error) {
	if embedder == nil {
		return "", nil, fmt.Errorf("embed chunk: no embedder")
	}
	txt := cleanEmbedText(content)
	if txt == "" {
		return "", nil, nil
	}
	trunc := trimForEmbedding(embedder, txt)
	// A trimmer that returns nothing for non-empty input must not reach the
	// provider, which rejects an empty input (and the whole batch with it).
	// Embedding the pre-trim text mirrors the pipeline's handling.
	if strings.TrimSpace(trunc) == "" {
		trunc = txt
	}
	contentResults, err := embedder.Encode(ctx, []string{trunc})
	if err != nil {
		return "", nil, fmt.Errorf("embed chunk: encode content: %w", err)
	}
	if len(contentResults) != 1 {
		return "", nil, fmt.Errorf("embed chunk: encode returned %d vectors for 1 input", len(contentResults))
	}
	merged := contentResults[0].Vector

	// A blank document name contributes no title vector: the pipeline skips the
	// title weighting entirely rather than embedding an empty title.
	if strings.TrimSpace(name) != "" {
		titleResults, err := embedder.Encode(ctx, []string{name})
		if err != nil {
			return "", nil, fmt.Errorf("embed chunk: encode title: %w", err)
		}
		if len(titleResults) != 1 {
			return "", nil, fmt.Errorf("embed chunk: encode title returned %d vectors for 1 input", len(titleResults))
		}
		merged, err = mergeEmbeddingVectors(titleResults[0].Vector, contentResults[0].Vector, titleWeight)
		if err != nil {
			return "", nil, fmt.Errorf("embed chunk: merge vectors: %w", err)
		}
	}
	if len(merged) == 0 {
		return "", nil, fmt.Errorf("embed chunk: empty vector")
	}
	return common.VectorColumnName(len(merged)), merged, nil
}
