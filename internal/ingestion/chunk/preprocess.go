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
	"regexp"
	"strings"
)

type PreprocessOperator struct {
	normalizeNewlines    bool
	stripWhitespace      bool
	removeEmptyLines     bool
	softLineBreakMerging bool
}

func NewPreprocessOperator(config map[string]interface{}) (*PreprocessOperator, error) {
	operator := &PreprocessOperator{}
	if v, ok := config["normalize_newlines"]; ok {
		operator.normalizeNewlines, ok = v.(bool)
		if !ok {
			return nil, fmt.Errorf("preprocess: normalize_newlines must be bool")
		}
	}
	if v, ok := config["strip_whitespace"]; ok {
		operator.stripWhitespace, ok = v.(bool)
		if !ok {
			return nil, fmt.Errorf("preprocess: strip_whitespace must be bool")
		}
	}
	if v, ok := config["remove_empty_lines"]; ok {
		operator.removeEmptyLines, ok = v.(bool)
		if !ok {
			return nil, fmt.Errorf("preprocess: remove_empty_lines must be bool")
		}
	}
	if v, ok := config["soft_line_break_merging"]; ok {
		operator.softLineBreakMerging, ok = v.(bool)
		if !ok {
			return nil, fmt.Errorf("preprocess: soft_line_break_merging must be bool")
		}
	}
	return operator, nil
}

func (o *PreprocessOperator) Prepare(chunkCtx *ChunkContext) error {
	return nil
}

func (o *PreprocessOperator) Execute(chunkCtx *ChunkContext) error {
	text := chunkCtx.Origin

	if o.normalizeNewlines {
		// \r\n → \n, \r → \n
		text = strings.ReplaceAll(text, "\r\n", "\n")
		text = strings.ReplaceAll(text, "\r", "\n")
		// Collapse multiple \n into one
		re := regexp.MustCompile(`\n{2,}`)
		text = re.ReplaceAllString(text, "\n")
	}

	if o.stripWhitespace {
		// Trim leading/trailing whitespace on each line
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimSpace(line)
		}
		text = strings.Join(lines, "\n")
	}

	if o.removeEmptyLines {
		lines := strings.Split(text, "\n")
		filtered := make([]string, 0, len(lines))
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				filtered = append(filtered, line)
			}
		}
		text = strings.Join(filtered, "\n")
	}

	if o.softLineBreakMerging {
		lines := strings.Split(text, "\n")
		var merged []string
		var current strings.Builder

		sentenceEnd := regexp.MustCompile(`[.!?][\s]*$`)

		for i, line := range lines {
			if current.Len() > 0 {
				current.WriteString(" ")
			}
			current.WriteString(line)

			if i == len(lines)-1 || sentenceEnd.MatchString(line) {
				merged = append(merged, current.String())
				current.Reset()
			}
		}
		text = strings.Join(merged, "\n")
	}

	chunkCtx.TextAfterPreprocess = text
	return nil
}

func (o *PreprocessOperator) Finish(chunkCtx *ChunkContext) error {
	return nil
}

func (o *PreprocessOperator) String() string {
	var buf strings.Builder
	buf.WriteString("preprocess:\n")
	fmt.Fprintf(&buf, "  normalize_newlines: %t\n", o.normalizeNewlines)
	fmt.Fprintf(&buf, "  strip_whitespace: %t\n", o.stripWhitespace)
	fmt.Fprintf(&buf, "  remove_empty_lines: %t\n", o.removeEmptyLines)
	return buf.String()
}
