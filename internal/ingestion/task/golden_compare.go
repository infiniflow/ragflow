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
	"time"
)

// GoldenDataflowResult is the structured output used by the local golden tools.
type GoldenDataflowResult struct {
	NormalizedChunks []map[string]any `json:"normalized_chunks"`
	ProcessedChunks  []map[string]any `json:"processed_chunks"`
	MergedMetadata   map[string]any   `json:"merged_metadata"`
}

// ProcessPipelineOutputForGolden replays the deterministic dataflow post-processing
// steps from a pipeline.run()-style output without embedding or external writes.
func ProcessPipelineOutputForGolden(
	pipelineOutput map[string]any,
	docID string,
	kbID string,
	docName string,
) GoldenDataflowResult {
	normalized := NormalizeChunks(pipelineOutput)
	if normalized == nil {
		normalized = []map[string]any{}
	}

	processed := deepCopyChunks(normalized)
	metadata := ProcessChunksForDataflow(processed, docID, kbID, docName, time.Now())
	if metadata == nil {
		metadata = map[string]any{}
	}

	return GoldenDataflowResult{
		NormalizedChunks: normalized,
		ProcessedChunks:  processed,
		MergedMetadata:   metadata,
	}
}
