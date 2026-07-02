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

// ---------------------------------------------------------------------------
// Overlap condition
// ---------------------------------------------------------------------------

type overlapConfig struct {
	Size int    `json:"size"`
	Unit string `json:"unit,omitempty"` // "char" (default) or "sentence"
}

type overlapCondition struct {
	Name          string
	Condition     Expr // pre-compiled expression AST from CompileExpression
	OverlapConfig overlapConfig
}

type mergeConfig struct {
	TargetSize int    `json:"target_size"`
	Strategy   string `json:"strategy"` // "greedy"
}

type filterConfig struct {
	MinLength int `json:"min_length"`
	MaxLength int `json:"max_length"`
}

type metadataConfig struct {
	IncludeIndex bool              `json:"include_index"`
	CustomFields map[string]string `json:"custom_fields,omitempty"`
}

// ---------------------------------------------------------------------------
// PostprocessOperator
// ---------------------------------------------------------------------------

type PostprocessOperator struct {
	merge   *mergeConfig
	overlap struct {
		unit       string // "char" (default) or "sentence"
		conditions []overlapCondition
		defaultCfg overlapConfig
	}
	filter      *filterConfig
	addMetadata *metadataConfig
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
		if s, ok := m["strategy"].(string); ok {
			op.merge.Strategy = s
		} else {
			op.merge.Strategy = "greedy"
		}
	}

	// Overlap
	if ov, ok := config["overlap"].(map[string]interface{}); ok {
		if u, ok := ov["unit"].(string); ok {
			op.overlap.unit = u
		} else {
			op.overlap.unit = "char"
		}

		// Default
		if d, ok := ov["default"].(map[string]interface{}); ok {
			op.overlap.defaultCfg = parseOverlapConfig(d)
		}

		// Conditions
		if conds, ok := ov["conditions"].([]interface{}); ok {
			for _, ci := range conds {
				c, ok := ci.(map[string]interface{})
				if !ok {
					continue
				}
				cond := overlapCondition{}
				if n, ok := c["name"].(string); ok {
					cond.Name = n
				}
				if exprStr, ok := c["if"].(string); ok {
					expression, err := CompileExpression(exprStr)
					if err == nil {
						cond.Condition = expression
					}
				}
				if thenMap, ok := c["then"].(map[string]interface{}); ok {
					cond.OverlapConfig = parseOverlapConfig(thenMap)
				}
				op.overlap.conditions = append(op.overlap.conditions, cond)
			}
		}
	}

	// Filter
	if f, ok := config["filter"].(map[string]interface{}); ok {
		op.filter = &filterConfig{}
		if v, ok := f["min_length"].(float64); ok {
			op.filter.MinLength = int(v)
		}
		if v, ok := f["max_length"].(float64); ok {
			op.filter.MaxLength = int(v)
		}
	}

	// Add metadata
	if am, ok := config["add_metadata"].(map[string]interface{}); ok {
		op.addMetadata = &metadataConfig{}
		if inc, ok := am["include_index"].(bool); ok {
			op.addMetadata.IncludeIndex = inc
		}
		if cf, ok := am["custom_fields"].(map[string]interface{}); ok {
			m := make(map[string]string, len(cf))
			for k, v := range cf {
				m[k] = fmt.Sprintf("%v", v)
			}
			op.addMetadata.CustomFields = m
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

	// 2. Overlap
	chunks = o.applyOverlap(chunks)

	// 3. Filter
	if o.filter != nil {
		chunks = o.filterChunks(chunks)
	}

	// 4. Add metadata
	if o.addMetadata != nil {
		chunks = o.addChunkMetadata(chunks)
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
		fmt.Fprintf(&buf, "    strategy: %q\n", o.merge.Strategy)
	}

	fmt.Fprintf(&buf, "  overlap:\n")
	fmt.Fprintf(&buf, "    unit: %q\n", o.overlap.unit)
	fmt.Fprintf(&buf, "    default:\n")
	fmt.Fprintf(&buf, "      size: %d\n", o.overlap.defaultCfg.Size)
	if o.overlap.defaultCfg.Unit != "" {
		fmt.Fprintf(&buf, "      unit: %q\n", o.overlap.defaultCfg.Unit)
	}
	if len(o.overlap.conditions) > 0 {
		fmt.Fprintf(&buf, "    conditions:\n")
		for _, c := range o.overlap.conditions {
			fmt.Fprintf(&buf, "      - name: %q\n", c.Name)
			fmt.Fprintf(&buf, "        condition: %q\n", c.Condition.String())
			fmt.Fprintf(&buf, "        then:\n")
			fmt.Fprintf(&buf, "          size: %d\n", c.OverlapConfig.Size)
			if c.OverlapConfig.Unit != "" {
				fmt.Fprintf(&buf, "          unit: %q\n", c.OverlapConfig.Unit)
			}
		}
	}

	if o.filter != nil {
		fmt.Fprintf(&buf, "  filter:\n")
		fmt.Fprintf(&buf, "    min_length: %d\n", o.filter.MinLength)
		fmt.Fprintf(&buf, "    max_length: %d\n", o.filter.MaxLength)
	}

	if o.addMetadata != nil {
		fmt.Fprintf(&buf, "  add_metadata:\n")
		fmt.Fprintf(&buf, "    include_index: %t\n", o.addMetadata.IncludeIndex)
		if len(o.addMetadata.CustomFields) > 0 {
			fmt.Fprintf(&buf, "    custom_fields:\n")
			for k, v := range o.addMetadata.CustomFields {
				fmt.Fprintf(&buf, "      %s: %q\n", k, v)
			}
		}
	}

	return buf.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseOverlapConfig(m map[string]interface{}) overlapConfig {
	cfg := overlapConfig{}
	if size, ok := m["size"].(float64); ok {
		cfg.Size = int(size)
	}
	if u, ok := m["unit"].(string); ok {
		cfg.Unit = u
	}
	return cfg
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

// applyOverlap evaluates conditions on each chunk and prepends overlap from the previous chunk.
func (o *PostprocessOperator) applyOverlap(chunks []ChunkData) []ChunkData {
	if len(chunks) <= 1 {
		return chunks
	}

	result := make([]ChunkData, len(chunks))
	copy(result, chunks)

	for i := 1; i < len(chunks); i++ {
		// Determine overlap size for chunks[i]
		cfg := o.resolveOverlapConfig(chunks[i])
		overlapSize := cfg.Size
		if overlapSize <= 0 {
			continue
		}

		prevContent := result[i-1].Content
		prevRunes := []rune(prevContent)
		if len(prevRunes) == 0 {
			continue
		}

		// Which unit?
		unit := cfg.Unit
		if unit == "" {
			unit = o.overlap.unit
		}

		var overlapText string
		switch unit {
		case "sentence":
			// Take last N sentences
			sentences := splitSentencesGeneric(prevContent)
			if len(sentences) < overlapSize {
				overlapText = prevContent
			} else {
				overlapText = strings.Join(sentences[len(sentences)-overlapSize:], " ")
			}
		default: // "char"
			if overlapSize >= len(prevRunes) {
				overlapText = prevContent
			} else {
				overlapText = string(prevRunes[len(prevRunes)-overlapSize:])
			}
		}

		result[i].Content = overlapText + result[i].Content
	}

	return result
}

// resolveOverlapConfig evaluates overlap conditions for a chunk.
func (o *PostprocessOperator) resolveOverlapConfig(chunk ChunkData) overlapConfig {
	vars := buildExprContext(&chunk, chunk.Metadata)

	for _, cond := range o.overlap.conditions {
		if cond.Condition == nil {
			continue
		}
		result, err := EvalCompiled(cond.Condition, vars)
		if err != nil {
			continue
		}
		if result {
			cfg := cond.OverlapConfig
			if cfg.Unit == "" {
				cfg.Unit = o.overlap.unit
			}
			return cfg
		}
	}

	cfg := o.overlap.defaultCfg
	if cfg.Unit == "" {
		cfg.Unit = o.overlap.unit
	}
	return cfg
}

// filterChunks removes chunks outside the length bounds.
func (o *PostprocessOperator) filterChunks(chunks []ChunkData) []ChunkData {
	filtered := make([]ChunkData, 0, len(chunks))
	for _, c := range chunks {
		l := len([]rune(c.Content))
		if o.filter.MinLength > 0 && l < o.filter.MinLength {
			continue
		}
		if o.filter.MaxLength > 0 && l > o.filter.MaxLength {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// addChunkMetadata enriches chunks with metadata.
func (o *PostprocessOperator) addChunkMetadata(chunks []ChunkData) []ChunkData {
	result := make([]ChunkData, len(chunks))
	for i, c := range chunks {
		if c.Metadata == nil {
			c.Metadata = make(map[string]interface{})
		}
		if o.addMetadata.IncludeIndex {
			c.Metadata["index"] = i
		}
		for field, action := range o.addMetadata.CustomFields {
			switch action {
			case "auto_detect":
				switch field {
				case "has_media_url":
					c.Metadata[field] = reMediaURL.MatchString(c.Content)
				case "has_image_url":
					c.Metadata[field] = reImageURL.MatchString(c.Content)
				case "has_video_url":
					c.Metadata[field] = reVideoURL.MatchString(c.Content)
				case "language":
					c.Metadata[field] = DetectLanguage(c.Content)
				case "length":
					c.Metadata[field] = RuneCount(c.Content)
				default:
					// Unknown auto-detect field — check for URLs generically
					c.Metadata[field] = reAnyURL.MatchString(c.Content)
				}
			default:
				c.Metadata[field] = action
			}
		}
		result[i] = c
	}
	return result
}

// splitSentencesGeneric splits text into sentences using common punctuation.
var sentenceBoundaries = []rune{'。', '！', '？', '.', '!', '?'}

func splitSentencesGeneric(text string) []string {
	runes := []rune(text)
	boundSet := make(map[rune]bool)
	for _, r := range sentenceBoundaries {
		boundSet[r] = true
	}

	var sentences []string
	var buf strings.Builder
	for _, r := range runes {
		buf.WriteRune(r)
		if boundSet[r] {
			sentences = append(sentences, strings.TrimSpace(buf.String()))
			buf.Reset()
		}
	}
	if buf.Len() > 0 {
		sentences = append(sentences, strings.TrimSpace(buf.String()))
	}
	return sentences
}
