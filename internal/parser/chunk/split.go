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
	"unicode/utf8"
)

type SplitOperator struct {
	strategy       string
	boundaries     []string
	keepSeparators bool
	chunkSize      int
	overlap        int
}

// defaultLengthChunkSize is the window size (in runes) used by the "length"
// strategy when no positive chunk_size is configured.
const defaultLengthChunkSize = 256

func NewSplitOperator(config map[string]interface{}) (*SplitOperator, error) {
	op := &SplitOperator{}

	if v, ok := config["strategy"]; ok {
		if s, ok := v.(string); ok {
			op.strategy = s
		}
	}

	if params, ok := config["params"].(map[string]interface{}); ok {
		if b, ok := params["boundaries"]; ok {
			if boundStrs, ok := b.([]interface{}); ok {
				for _, bs := range boundStrs {
					if s, ok := bs.(string); ok {
						op.boundaries = append(op.boundaries, s)
					}
				}
			}
		}
		if ks, ok := params["keep_separators"]; ok {
			if b, ok := ks.(bool); ok {
				op.keepSeparators = b
			}
		}
		// chunk_size / overlap drive the "length" strategy. JSON numbers
		// decode as float64, so accept that alongside the integer types.
		if cs, ok := params["chunk_size"]; ok {
			op.chunkSize = toInt(cs)
		}
		if ov, ok := params["overlap"]; ok {
			op.overlap = toInt(ov)
		}
	}

	return op, nil
}

// toInt coerces a DSL numeric value (float64 from JSON, or a native integer)
// to int. Any other type yields 0.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	default:
		return 0
	}
}

func (o *SplitOperator) Prepare(ctx *ChunkContext) error {
	return nil
}

func (o *SplitOperator) Execute(ctx *ChunkContext) error {
	text := ctx.TextAfterPreprocess

	if o.strategy == "" {
		o.strategy = "sentence"
	}

	switch o.strategy {
	case "sentence":
		ctx.SplitChunks = o.splitSentences(text)
	case "char":
		ctx.SplitChunks = o.splitByChar(text)
	case "paragraph":
		ctx.SplitChunks = o.splitByParagraph(text)
	case "length":
		ctx.SplitChunks = o.splitByLength(text)
	default:
		ctx.SplitChunks = o.splitSentences(text)
	}

	return nil
}

func (o *SplitOperator) Finish(ctx *ChunkContext) error {
	return nil
}

func (o *SplitOperator) String() string {
	var buf strings.Builder
	buf.WriteString("split:\n")
	fmt.Fprintf(&buf, "  strategy: %q\n", o.strategy)
	fmt.Fprintf(&buf, "  boundaries:\n")
	for _, r := range o.boundaries {
		fmt.Fprintf(&buf, "    - %q\n", r)
	}
	fmt.Fprintf(&buf, "  keep_separators: %t\n", o.keepSeparators)
	fmt.Fprintf(&buf, "  chunk_size: %d\n", o.chunkSize)
	fmt.Fprintf(&buf, "  overlap: %d\n", o.overlap)
	return buf.String()
}

var sentenceBoundaries = []string{"。", "！", "？", ".", "!", "?", ";", "\n"}

// splitSentences splits text at the built-in sentence boundaries.
func (o *SplitOperator) splitSentences(text string) []ChunkData {
	var chunks []ChunkData
	var buf strings.Builder
	i := 0

	for i < len(text) {
		// Try to match any boundary at current position (first match wins)
		matchedBound := ""
		for _, bound := range sentenceBoundaries {
			if bound != "" && i+len(bound) <= len(text) && text[i:i+len(bound)] == bound {
				matchedBound = bound
				break
			}
		}

		if matchedBound != "" {
			if buf.Len() > 0 {
				content := strings.TrimSpace(buf.String())
				if content != "" {
					chunks = append(chunks, ChunkData{
						Content: content,
						Index:   len(chunks),
						Metadata: map[string]interface{}{
							"language": DetectLanguage(content),
						},
					})
				}
				buf.Reset()
			}
			i += len(matchedBound)
		} else {
			r, size := utf8.DecodeRuneInString(text[i:])
			buf.WriteRune(r)
			i += size
		}
	}

	// flush remaining text
	if buf.Len() > 0 {
		content := strings.TrimSpace(buf.String())
		if content != "" {
			chunks = append(chunks, ChunkData{
				Content: content,
				Index:   len(chunks),
				Metadata: map[string]interface{}{
					"language": DetectLanguage(content),
				},
			})
		}
	}

	return chunks
}

func (o *SplitOperator) splitByChar(text string) []ChunkData {
	var chunks []ChunkData
	for len(text) > 0 {
		r, size := utf8.DecodeRuneInString(text)
		chunks = append(chunks, ChunkData{
			Content:  string(r),
			Index:    len(chunks),
			Metadata: map[string]interface{}{"language": DetectLanguage(text)},
		})
		text = text[size:]
	}
	return chunks
}

// splitByLength splits text into fixed-size, rune-aware windows of chunkSize
// runes, carrying overlap runes from the end of each window into the start of
// the next. This is the canonical fixed-size-with-overlap chunking used by RAG
// pipelines to bound chunk length while preserving cross-boundary context.
//
// chunkSize defaults to defaultLengthChunkSize when not positive. overlap is
// clamped to [0, chunkSize-1] so the window always advances by at least one
// rune and the function terminates. Sizing is by rune count (not bytes), so
// multi-byte (e.g. CJK) text is windowed by character.
func (o *SplitOperator) splitByLength(text string) []ChunkData {
	chunkSize := o.chunkSize
	if chunkSize <= 0 {
		chunkSize = defaultLengthChunkSize
	}

	overlap := o.overlap
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= chunkSize {
		overlap = chunkSize - 1
	}
	step := chunkSize - overlap

	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}

	var chunks []ChunkData
	for start := 0; start < len(runes); start += step {
		end := start + chunkSize
		if end > len(runes) {
			end = len(runes)
		}

		content := string(runes[start:end])
		chunks = append(chunks, ChunkData{
			Content: content,
			Size:    end - start,
			Index:   len(chunks),
			Metadata: map[string]interface{}{
				"language": DetectLanguage(content),
			},
		})

		if end == len(runes) {
			break
		}
	}

	return chunks
}

func (o *SplitOperator) splitByParagraph(text string) []ChunkData {
	paragraphs := strings.Split(text, "\n")
	chunks := make([]ChunkData, 0, len(paragraphs))
	for i, p := range paragraphs {
		trimmed := strings.TrimSpace(p)
		if trimmed == "" {
			continue
		}
		chunks = append(chunks, ChunkData{
			Content:  trimmed,
			Index:    i,
			Metadata: map[string]interface{}{"language": DetectLanguage(trimmed)},
		})
	}
	return chunks
}
