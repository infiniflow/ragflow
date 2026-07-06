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

import (
	"fmt"
	"strings"
)

type mergeConfig struct {
	TargetSize int `json:"target_size"`
}

type filterConfig struct {
	MinLength      int  `json:"min_length"`
	DropEmpty      bool `json:"drop_empty"`      // drop chunks that are empty or whitespace-only
	DropDuplicates bool `json:"drop_duplicates"` // drop chunks whose content already appeared
}

// ---------------------------------------------------------------------------
// PostprocessOperator
// ---------------------------------------------------------------------------

type PostprocessOperator struct {
	merge  *mergeConfig
	filter *filterConfig
}

func NewPostprocessOperator(config map[string]interface{}) (*PostprocessOperator, error) {
	op := &PostprocessOperator{}

	// Merge
	if m, ok := config["merge"].(map[string]interface{}); ok {
		op.merge = &mergeConfig{}
		if ts, ok := m["target_size"].(float64); ok {
			op.merge.TargetSize = int(ts)
		} else {
			op.merge.TargetSize = 500
		}
	}

	// Filter
	if f, ok := config["filter"].(map[string]interface{}); ok {
		op.filter = &filterConfig{}
		if v, ok := f["min_length"].(float64); ok {
			op.filter.MinLength = int(v)
		}
		if v, ok := f["drop_empty"].(bool); ok {
			op.filter.DropEmpty = v
		}
		if v, ok := f["drop_duplicates"].(bool); ok {
			op.filter.DropDuplicates = v
		}
	}

	return op, nil
}

func (o *PostprocessOperator) Prepare(chunkCtx *ChunkContext) error {

	return nil
}

func (o *PostprocessOperator) Execute(chunkCtx *ChunkContext) error {
	chunks := chunkCtx.SplitChunks
	if len(chunks) == 0 {
		return nil
	}

	// 1. Merge
	if o.merge != nil {
		chunks = o.mergeChunks(chunks)
	}

	// 2. Filter
	if o.filter != nil {
		chunks = o.filterChunks(chunks)
	}

	// Re-index
	for i := range chunks {
		chunks[i].Index = i
		chunks[i].Size = len(chunks[i].GetContent())
	}

	chunkCtx.ResultChunks = chunks
	return nil
}

func (o *PostprocessOperator) Finish(chunkCtx *ChunkContext) error {
	return nil
}

func (o *PostprocessOperator) String() string {
	var buf strings.Builder
	buf.WriteString("postprocess:\n")

	if o.merge != nil {
		fmt.Fprintf(&buf, "  merge:\n")
		fmt.Fprintf(&buf, "    target_size: %d\n", o.merge.TargetSize)
	}

	if o.filter != nil {
		fmt.Fprintf(&buf, "  filter:\n")
		fmt.Fprintf(&buf, "    min_length: %d\n", o.filter.MinLength)
		fmt.Fprintf(&buf, "    drop_empty: %t\n", o.filter.DropEmpty)
		fmt.Fprintf(&buf, "    drop_duplicates: %t\n", o.filter.DropDuplicates)
	}

	return buf.String()
}

// mergeChunks greedily merges small chunks into larger ones up to target_size.
func (o *PostprocessOperator) mergeChunks(chunks []ChunkData) []ChunkData {
	target := o.merge.TargetSize
	if target <= 0 {
		target = 500
	}

	var merged []ChunkData
	var buf strings.Builder
	var bufMeta map[string]interface{}
	firstIndex := 0

	for i, c := range chunks {
		// If this single chunk already exceeds target, flush first then add
		if len([]rune(c.Content)) >= target {
			if buf.Len() > 0 {
				merged = append(merged, ChunkData{
					Content:  buf.String(),
					Index:    firstIndex,
					Metadata: bufMeta,
				})
				buf.Reset()
				bufMeta = nil
			}
			merged = append(merged, c)
			firstIndex = i + 1
			continue
		}

		if buf.Len() == 0 {
			buf.WriteString(c.Content)
			bufMeta = c.Metadata
			firstIndex = c.Index
		} else {
			nextLen := len([]rune(c.Content))
			// If adding this chunk would exceed target, flush current and start new
			if buf.Len()+nextLen+1 > target {
				merged = append(merged, ChunkData{
					Content:  buf.String(),
					Index:    firstIndex,
					Metadata: bufMeta,
				})
				buf.Reset()
				buf.WriteString(c.Content)
				bufMeta = c.Metadata
				firstIndex = c.Index
			} else {
				buf.WriteString(" ")
				buf.WriteString(c.Content)
				// Merge metadata (last wins for overlapping keys)
				if c.Metadata != nil && bufMeta == nil {
					bufMeta = make(map[string]interface{})
				}
				for k, v := range c.Metadata {
					bufMeta[k] = v
				}
			}
		}
	}

	// Flush remaining
	if buf.Len() > 0 {
		merged = append(merged, ChunkData{
			Content:  buf.String(),
			Index:    firstIndex,
			Metadata: bufMeta,
		})
	}

	return merged
}

// filterChunks removes chunks outside the length bounds and, when configured,
// drops empty/whitespace-only chunks and exact-duplicate chunks. Duplicate
// detection is order-preserving: the first occurrence is kept and later chunks
// with identical content are dropped.
func (o *PostprocessOperator) filterChunks(chunks []ChunkData) []ChunkData {
	var seen map[string]struct{}
	if o.filter.DropDuplicates {
		seen = make(map[string]struct{}, len(chunks))
	}

	filtered := make([]ChunkData, 0, len(chunks))
	for _, c := range chunks {
		if o.filter.DropEmpty && strings.TrimSpace(c.Content) == "" {
			continue
		}
		l := len([]rune(c.Content))
		if o.filter.MinLength > 0 && l < o.filter.MinLength {
			continue
		}
		if o.filter.DropDuplicates {
			if _, dup := seen[c.Content]; dup {
				continue
			}
			seen[c.Content] = struct{}{}
		}
		filtered = append(filtered, c)
	}
	return filtered
}
