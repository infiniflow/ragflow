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
	normalizeNewlines bool
	stripWhitespace   bool
	removeEmptyLines  bool
}

func NewPreprocessOperator() *PreprocessOperator {
	return &PreprocessOperator{}
}

func (o *PreprocessOperator) Prepare(config map[string]interface{}) error {
	if v, ok := config["normalize_newlines"]; ok {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("preprocess: normalize_newlines must be bool")
		}
		o.normalizeNewlines = b
	}
	if v, ok := config["strip_whitespace"]; ok {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("preprocess: strip_whitespace must be bool")
		}
		o.stripWhitespace = b
	}
	if v, ok := config["remove_empty_lines"]; ok {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("preprocess: remove_empty_lines must be bool")
		}
		o.removeEmptyLines = b
	}
	return nil
}

func (o *PreprocessOperator) Execute(ctx *Context) error {
	text := ctx.Text

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

	ctx.Text = text
	return nil
}

func (o *PreprocessOperator) Finish() error {
	return nil
}
