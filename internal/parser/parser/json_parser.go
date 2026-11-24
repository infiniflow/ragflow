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

// JSONParser handles JSON, JSON Lines, and Docling document exports.

package parser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"ragflow/internal/parser/charset"
)

// JSONParser handles JSON / JSONL / LDJSON files.
type JSONParser struct{}

func NewJSONParser() *JSONParser {
	return &JSONParser{}
}

func (p *JSONParser) String() string {
	return "JSONParser"
}

// ParseWithResult reads JSON Lines one value per line, and detects the shape
// of other JSON input before building text items.
func (p *JSONParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	decoded, encName := charset.DecodeToUTF8(data, "application/json")
	file := map[string]any{
		"name":     filename,
		"size":     len(data),
		"encoding": encName,
	}
	text := string(bytes.TrimSpace(decoded))
	if text == "" {
		return ParseResult{
			File: file,
			JSON: []DocItem{NewTextJSONItem("")},
		}
	}

	var items []map[string]any
	switch strings.ToLower(filepath.Ext(filename)) {
	case ".jsonl", ".ldjson":
		// A line can hold any JSON value, including an array or scalar.
		items = parseJSONLines(text)
	}
	if items == nil {
		items = parseJSONContent(text, file)
	}
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	return ParseResult{
		File: file,
		JSON: MapsToItems(items),
	}
}

// parseJSONContent detects the JSON shape and dispatches.
func parseJSONContent(text string, file map[string]any) []map[string]any {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	// 1. Try JSON array: [...]
	if strings.HasPrefix(trimmed, "[") {
		return parseJSONArray(trimmed)
	}

	// 2. Try single JSON object: {...}. Use a syntax-only framing check before
	// building the map: a multi-object JSONL payload or trailing data is
	// rejected here without allocating/decoding the trailing value (the old
	// standard-library probe did this too, but by decoding twice).
	if strings.HasPrefix(trimmed, "{") {
		if !json.Valid([]byte(trimmed)) {
			if isJSONL(trimmed) {
				return parseJSONLines(trimmed)
			}
			return []map[string]any{{"text": trimmed, "doc_type_kwd": "text"}}
		}
		// Scan raw tokens for out-of-range numbers BEFORE decoding into a map:
		// decoding would overwrite an earlier overflowing number with a later
		// duplicate key, hiding it from a post-decode check.
		if !jsonNumbersInFloat64Range([]byte(trimmed)) {
			return []map[string]any{{"text": trimmed, "doc_type_kwd": "text"}}
		}
		obj, ok := parseSingleJSONObjectValue(trimmed)
		if !ok {
			return []map[string]any{{"text": trimmed, "doc_type_kwd": "text"}}
		}
		if items, ok := parseDoclingJSONItems(obj); ok {
			if pageCount := doclingPageCount([]doclingDocument{{JSONContent: obj}}, 0); pageCount <= doclingMaxSourcePageCount {
				file["page_count"] = pageCount
			}
			return items
		}
		return singleObjectItems(obj)
	}

	// 3. Fallback: treat as text.
	return []map[string]any{{"text": trimmed, "doc_type_kwd": "text"}}
}

// isJSONL returns true when the content looks like newline-delimited
// JSON (multiple lines each starting with '{').
func isJSONL(text string) bool {
	objCount := 0
	for line := range strings.Lines(text) {
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
	if err := unmarshalJSON([]byte(text), &arr); err != nil {
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
			b, err := marshalJSON(elem)
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

// parseSingleJSONObjectValue unmarshals one JSON object with the
// number-preserving decoder, returning (value, true) on success.
func parseSingleJSONObjectValue(text string) (map[string]any, bool) {
	var obj map[string]any
	if err := unmarshalJSON([]byte(text), &obj); err != nil {
		return nil, false
	}
	return obj, true
}

// singleObjectItems renders a decoded object into the single text item.
func singleObjectItems(obj map[string]any) []map[string]any {
	b, err := marshalJSON(obj)
	if err != nil {
		return []map[string]any{{"text": fmt.Sprintf("%v", obj), "doc_type_kwd": "text"}}
	}
	return []map[string]any{{"text": string(b), "doc_type_kwd": "text"}}
}

// parseJSONLines parses line-delimited JSON. Each non-empty line is
// independently unmarshalled; lines that fail to parse are skipped.
func parseJSONLines(text string) []map[string]any {
	var items []map[string]any
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Each line should be a JSON object.
		var obj map[string]any
		if err := unmarshalJSON([]byte(line), &obj); err != nil {
			// Also try as a string or other standalone value.
			var val any
			if err2 := unmarshalJSON([]byte(line), &val); err2 != nil {
				// Unparseable line — skip, matching Python behaviour.
				continue
			} else {
				if s, ok := val.(string); ok {
					items = append(items, map[string]any{
						"text":         s,
						"doc_type_kwd": "text",
					})
				} else {
					b, _ := marshalJSON(val)
					items = append(items, map[string]any{
						"text":         string(b),
						"doc_type_kwd": "text",
					})
				}
			}
			continue
		}
		b, err := marshalJSON(obj)
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

// jsonNumbersInFloat64Range scans every numeric literal in raw JSON, checking
// it against the float64 range BEFORE map construction. The old float64 probe
// rejected documents containing a number like 1e400; decoding first would let
// a duplicate key overwrite that number, so the check must see the raw bytes.
// A byte scan with a string-state machine avoids the per-token interface
// allocations of json.Decoder.Token(); json.Valid already guarantees syntax.
func jsonNumbersInFloat64Range(data []byte) bool {
	inString, escaped := false, false
	for i := 0; i < len(data); {
		c := data[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			i++
			continue
		}
		if c == '"' {
			inString = true
			i++
			continue
		}
		if c == '-' || (c >= '0' && c <= '9') {
			start := i
			i++ // sign or first digit
			for i < len(data) {
				ch := data[i]
				if (ch >= '0' && ch <= '9') || ch == '.' || ch == 'e' || ch == 'E' || ch == '+' || ch == '-' {
					i++
				} else {
					break
				}
			}
			if _, err := strconv.ParseFloat(string(data[start:i]), 64); err != nil {
				return false
			}
			continue
		}
		i++
	}
	return true
}

// unmarshalJSON is json.Unmarshal with each number kept as written, as a
// json.Number. Decoded into a float64, an id above 2^53 changed digits
// (12345678901234567890 became 12345678901234567000) and 1.50 became 1.5.
// Data after the value is an error, as it is for json.Unmarshal.
func unmarshalJSON(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	// Reject trailing data at its opening byte without decoding/allocating the
	// trailing value (Token() would decode it). InputOffset is the byte count
	// consumed by Decode; anything but JSON whitespace after it is trailing data.
	if len(bytes.TrimSpace(data[decoder.InputOffset():])) > 0 {
		return errors.New("json: data after the top-level value")
	}
	return nil
}

// marshalJSON is json.Marshal without HTML escaping. json.Marshal writes
// <, > and & as \u003c, \u003e and \u0026, which is meant for embedding in
// HTML and garbles the text a chunk is searched and read as.
func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
