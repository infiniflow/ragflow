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
	boundaries     []rune
	keepSeparators bool
}

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
						for _, r := range s {
							op.boundaries = append(op.boundaries, r)
						}
					}
				}
			}
		}
		if ks, ok := params["keep_separators"]; ok {
			if b, ok := ks.(bool); ok {
				op.keepSeparators = b
			}
		}
	}

	return op, nil
}

func (o *SplitOperator) Prepare(config map[string]interface{}) error {
	return nil
}

func (o *SplitOperator) Execute(ctx *Context) error {
	text := ctx.Text

	if o.strategy == "" {
		o.strategy = "sentence"
	}

	switch o.strategy {
	case "sentence":
		ctx.Chunks = o.splitSentences(text)
	case "char":
		ctx.Chunks = o.splitByChar(text)
	case "paragraph":
		ctx.Chunks = o.splitByParagraph(text)
	default:
		ctx.Chunks = o.splitSentences(text)
	}

	return nil
}

func (o *SplitOperator) Finish() error {
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
	return buf.String()
}

// splitSentences splits text at boundary runes, optionally keeping separators.
func (o *SplitOperator) splitSentences(text string) []ChunkData {
	if len(o.boundaries) == 0 {
		o.boundaries = []rune{'。', '！', '？', '\n'}
	}

	boundSet := make(map[rune]bool, len(o.boundaries))
	for _, r := range o.boundaries {
		boundSet[r] = true
	}

	runes := []rune(text)
	var chunks []ChunkData
	var buf strings.Builder
	index := 0

	flush := func(sep rune) {
		if buf.Len() > 0 || sep != 0 {
			if sep != 0 && o.keepSeparators {
				buf.WriteRune(sep)
			}
			if buf.Len() > 0 {
				chunks = append(chunks, ChunkData{
					Content: buf.String(),
					Index:   index,
					Metadata: map[string]interface{}{
						"language": DetectLanguage(buf.String()),
					},
				})
				index++
				buf.Reset()
			}
		}
	}

	for _, r := range runes {
		if boundSet[r] {
			flush(r)
		} else {
			buf.WriteRune(r)
		}
	}
	// flush remaining
	flush(0)

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
