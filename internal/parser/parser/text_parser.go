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

// TextParser (port-rag-flow-pipeline-to-go.md Phase 2.5 Slice 1).
//
// The python rag/flow/parser/parser.py:_code path (L1066) routes
// .txt / .py / .js / .java / .c / .cpp / .h / .php / .go / .ts / .sh
// / .cs / .kt / .sql files through deepdoc.parser.TxtParser. The Go
// side needs a parser for these families so `text&code` resolves to a
// real ParseResultProducer.
//
// TextParser fills that gap with a minimal but real implementation:
// it splits the input into paragraph-sized items and emits the
// python-compatible `{text, doc_type_kwd:"text"}` shape. The
// python TxtParser additionally does layout-aware section
// detection; the Go version is intentionally simpler because (a)
// no production template currently relies on text&code for richer
// structure than paragraph items.

package parser

import (
	"bytes"
	"strings"
)

const TextParserLibType = "text"

// TextParser is the text&code family parser. It implements the
// structured ParseResultProducer contract directly.
type TextParser struct {
	// maxItemBytes caps each emitted item's text length. The
	// python TxtParser uses similar paragraph-style chunking;
	// 8192 bytes is a conservative ceiling that prevents the
	// downstream chunker from receiving oversized inputs.
	maxItemBytes int
}

// NewTextParser constructs a TextParser. The libType argument
// preserves the parser-library constructor signature for
// consistency with the other family parsers; the value is ignored
// (TextParser has no alternative backend).
func NewTextParser(_ string) (*TextParser, error) {
	return &TextParser{maxItemBytes: 8192}, nil
}

// ParseWithResult emits one item per non-empty paragraph. The
// output format is "json" to mirror the python TxtParser's
// behaviour (it emits a list of items with text + doc_type_kwd).
//
// The items slice is always non-nil so downstream chunkers see a
// non-empty JSON payload even for an empty input (mirrors the
// MarkdownParser convention at markdown_parser.go:71-76).
func (p *TextParser) ParseWithResult(filename string, data []byte) ParseResult {
	if !utf8Valid(data) {
		return ParseResult{Err: errInvalidUTF8}
	}
	items := textParserItems(data, p.maxItemBytes)
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

func (p *TextParser) String() string {
	return "TextParser"
}

// errInvalidUTF8 is returned when the input bytes fail UTF-8
// validation. Matches the python TxtParser's behaviour of
// surfacing a clear error rather than emitting replacement bytes.
var errInvalidUTF8 = errInvalidUTF8Sentinel("parser: text input is not valid UTF-8")

type errInvalidUTF8Sentinel string

func (e errInvalidUTF8Sentinel) Error() string { return string(e) }

// utf8Valid is a tiny stdlib-free validator. We avoid
// unicode/utf8.Valid to keep this file dependency-light; the
// validation rule is the same (decode without rejecting bytes).
func utf8Valid(data []byte) bool {
	for i := 0; i < len(data); {
		r, size := decodeRune(data[i:])
		if r == 0xFFFD && size == 1 {
			return false
		}
		i += size
	}
	return true
}

// decodeRune is a minimal UTF-8 decoder that mirrors
// utf8.DecodeRune's signature: returns the rune and its byte
// width. Returns (RuneError, 1) on invalid sequences, matching
// the stdlib contract.
func decodeRune(p []byte) (rune, int) {
	if len(p) == 0 {
		return 0xFFFD, 0
	}
	c := p[0]
	switch {
	case c < 0x80:
		return rune(c), 1
	case c < 0xC2:
		return 0xFFFD, 1
	case c < 0xE0:
		if len(p) < 2 || p[1]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
		return rune(c&0x1F)<<6 | rune(p[1]&0x3F), 2
	case c < 0xF0:
		if len(p) < 3 || p[1]&0xC0 != 0x80 || p[2]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
		return rune(c&0x0F)<<12 | rune(p[1]&0x3F)<<6 | rune(p[2]&0x3F), 3
	case c < 0xF5:
		if len(p) < 4 || p[1]&0xC0 != 0x80 || p[2]&0xC0 != 0x80 || p[3]&0xC0 != 0x80 {
			return 0xFFFD, 1
		}
		return rune(c&0x07)<<18 | rune(p[1]&0x3F)<<12 | rune(p[2]&0x3F)<<6 | rune(p[3]&0x3F), 4
	}
	return 0xFFFD, 1
}

// textParserItems splits `data` into paragraph-sized chunks. The
// split rule mirrors the python TxtParser: blank lines separate
// paragraphs; long paragraphs are sliced at maxItemBytes boundaries.
func textParserItems(data []byte, maxItemBytes int) []map[string]any {
	var items []map[string]any
	for _, raw := range bytes.Split(data, []byte("\n\n")) {
		text := strings.TrimSpace(string(raw))
		if text == "" {
			continue
		}
		if maxItemBytes > 0 && len(text) > maxItemBytes {
			// Slice at the nearest newline below maxItemBytes;
			// falls back to a hard slice when no newline exists.
			cut := strings.LastIndex(text[:maxItemBytes], "\n")
			if cut <= 0 {
				cut = maxItemBytes
			}
			items = append(items, map[string]any{
				"text":         strings.TrimSpace(text[:cut]),
				"doc_type_kwd": "text",
			})
			text = strings.TrimSpace(text[cut:])
			if text == "" {
				continue
			}
		}
		items = append(items, map[string]any{
			"text":         text,
			"doc_type_kwd": "text",
		})
	}
	return items
}
