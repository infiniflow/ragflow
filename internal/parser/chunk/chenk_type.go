//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package chunk

// Operator defines the interface for all chunking pipeline stages.
type Operator interface {
	// Prepare configures the operator from a DSL stage config map.
	Prepare(ctx *ChunkContext) error
	// Execute runs the operator on the shared context.
	Execute(ctx *ChunkContext) error
	// Finish performs any cleanup.
	Finish(ctx *ChunkContext) error

	String() string
}

// ChunkData represents a single chunk produced by the pipeline.
type ChunkData struct {
	Content  string                 `json:"content"`
	Size     int                    `json:"size"`
	Index    int                    `json:"index,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

func (c *ChunkData) GetContent() string {
	if c == nil {
		return ""
	}
	return c.Content
}

// ChunkContext flows through the pipeline, carrying text and chunks.
type ChunkContext struct {
	Origin string // raw text

	TextAfterPreprocess string // text after preprocess operator

	SplitChunks []ChunkData // chunks after split operator

	ResultChunks []ChunkData // final or intermediate chunks
}
