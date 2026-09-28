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
// TextParser decodes and normalizes text&code input, then emits one parser
// unit. Chunk boundaries belong to GeneralChunker, not to this parser.

package parser

import (
	"context"
	"strings"
)

// TextParser is the text&code family parser. It implements the
// structured ParseResultProducer contract directly.
type TextParser struct{}

// NewTextParser constructs a TextParser.
func NewTextParser() *TextParser {
	return &TextParser{}
}

// ParseWithResult emits the complete normalized text as one parser unit. The
// output format is "json" to mirror the parser component boundary. No fixed
// delimiter or token budget is applied here.
func (p *TextParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	decoded, encName := DecodeToUTF8(data, "text/plain")
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": encName,
		},
		JSON: []map[string]any{NewTextJSONItem(normalizeTextNewlines(string(decoded)))},
	}
}

func (p *TextParser) String() string {
	return "TextParser"
}

// normalizeTextNewlines folds CRLF and standalone CR to LF while preserving
// the rest of the document content for GeneralChunker.
func normalizeTextNewlines(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}
