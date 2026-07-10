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

// JSONParser handles .json, .jsonl, and .ldjson files. It detects
// the payload shape:
//
//   - JSON array: each element becomes one item.
//   - Single JSON object: emitted as one item.
//   - JSONL (newline-delimited JSON): each non-empty line is
//     independently parsed; lines that fail to parse are skipped
//     with a warning — matching the Python JsonParser behaviour.
//
// For JSON objects, the parser serialises the full object into a
// `text` field so the downstream chunker receives a readable
// representation. Arrays of objects emit one item per element.

package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// JSONParser handles JSON / JSONL / LDJSON files.
type JSONParser struct{}

func NewJSONParser() *JSONParser {
	return &JSONParser{}
}

func (p *JSONParser) String() string {
	return "JSONParser"
}

// ParseWithResult implements ParseResultProducer. It detects the JSON
// shape (array, single object, or line-delimited) and emits one JSON
// item per logical record, with {text, doc_type_kwd:"text"}.
func (p *JSONParser) ParseWithResult(filename string, data []byte) ParseResult {
	text := string(bytes.TrimSpace(data))
	if text == "" {
		return ParseResult{
			OutputFormat: "json",
			File: map[string]any{
				"name":     filename,
				"size":     len(data),
				"encoding": "utf-8",
			},
			JSON: []map[string]any{{"text": "", "doc_type_kwd": "text"}},
		}
	}

	items := parseJSONContent(text)
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": "utf-8",
		},
		JSON: items,
	}
}

// parseJSONContent detects the JSON shape and dispatches.
func parseJSONContent(text string) []map[string]any {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	// 1. Try JSON array: [...]
	if strings.HasPrefix(trimmed, "[") {
		return parseJSONArray(trimmed)
	}

	// 2. Try single JSON object: {...}
	if strings.HasPrefix(trimmed, "{") {
		// Detect JSONL: if the second non-empty line starts with '{',
		// treat as line-delimited JSON.
		if isJSONL(trimmed) {
			return parseJSONLines(trimmed)
		}
		return parseSingleJSONObject(trimmed)
	}

	// 3. Fallback: treat as text.
	return []map[string]any{{"text": trimmed, "doc_type_kwd": "text"}}
}

// isJSONL returns true when the content looks like newline-delimited
// JSON (multiple lines each starting with '{').
func isJSONL(text string) bool {
	lines := strings.Split(text, "\n")
	objCount := 0
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "{") {
			objCount++
			if objCount >= 2 {
				return true
			}
		}
	}
	return false
}

// parseJSONArray unmarshals a JSON array and emits one item per element.
func parseJSONArray(text string) []map[string]any {
	var arr []any
	if err := json.Unmarshal([]byte(text), &arr); err != nil {
		// Fallback: emit as plain text.
		return []map[string]any{{"text": text, "doc_type_kwd": "text"}}
	}
	if len(arr) == 0 {
		return nil
	}
	items := make([]map[string]any, 0, len(arr))
	for _, elem := range arr {
		if s, ok := elem.(string); ok {
			items = append(items, map[string]any{
				"text":         s,
				"doc_type_kwd": "text",
			})
		} else {
			// Re-marshal non-string elements.
			b, err := json.Marshal(elem)
			if err != nil {
				items = append(items, map[string]any{
					"text":         fmt.Sprintf("%v", elem),
					"doc_type_kwd": "text",
				})
			} else {
				items = append(items, map[string]any{
					"text":         string(b),
					"doc_type_kwd": "text",
				})
			}
		}
	}
	return items
}

// parseSingleJSONObject unmarshals a single JSON object and emits one item.
func parseSingleJSONObject(text string) []map[string]any {
	var obj map[string]any
	if err := json.Unmarshal([]byte(text), &obj); err != nil {
		return []map[string]any{{"text": text, "doc_type_kwd": "text"}}
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return []map[string]any{{"text": fmt.Sprintf("%v", obj), "doc_type_kwd": "text"}}
	}
	return []map[string]any{{"text": string(b), "doc_type_kwd": "text"}}
}

// parseJSONLines parses line-delimited JSON. Each non-empty line is
// independently unmarshalled; lines that fail to parse are skipped.
func parseJSONLines(text string) []map[string]any {
	var items []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(text))
	scanner.Buffer(nil, 10*1024*1024) // 10 MB max line
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Each line should be a JSON object.
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			// Also try as a string or other standalone value.
			var val any
			if err2 := json.Unmarshal([]byte(line), &val); err2 != nil {
				// Unparseable line — skip, matching Python behaviour.
				continue
			} else {
				if s, ok := val.(string); ok {
					items = append(items, map[string]any{
						"text":         s,
						"doc_type_kwd": "text",
					})
				} else {
					b, _ := json.Marshal(val)
					items = append(items, map[string]any{
						"text":         string(b),
						"doc_type_kwd": "text",
					})
				}
			}
			continue
		}
		b, err := json.Marshal(obj)
		if err != nil {
			continue
		}
		items = append(items, map[string]any{
			"text":         string(b),
			"doc_type_kwd": "text",
		})
	}
	return items
}
