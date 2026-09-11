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
// TextParser fills that gap with a real implementation: it splits the
// input into fine segments on the flow parser's default delimiter set and
// emits the python-compatible `{text, doc_type_kwd:"text"}` shape. Block
// boundaries converge to the Python flow TxtParser (which uses the same
// delimiter set); the OVER_CAP token merge that Python applies afterwards is
// intentionally NOT performed here — chunking ownership stays with the
// downstream Chunker (contract #17799), so
// item counts differ from Python's merged chunks and are reconciled by the
// stitch-compare alignment test (align_test.go).

package parser

import (
	"context"
	"regexp"
	"strings"
)

// TextParser is the text&code family parser. It implements the
// structured ParseResultProducer contract directly.
type TextParser struct{}

// NewTextParser constructs a TextParser.
func NewTextParser() *TextParser {
	return &TextParser{}
}

// ParseWithResult emits one item per non-empty paragraph. The
// output format is "json" to mirror the python TxtParser's
// behaviour (it emits a list of items with text + doc_type_kwd).
//
// The items slice is always non-nil so downstream chunkers see a
// non-empty JSON payload even for an empty input (mirrors the
// MarkdownParser convention at markdown_parser.go:71-76).
func (p *TextParser) ParseWithResult(ctx context.Context, filename string, data []byte) ParseResult {
	decoded, encName := DecodeToUTF8(data, "text/plain")
	items := textParserItems(decoded)
	if items == nil {
		items = []map[string]any{{"text": "", "doc_type_kwd": "text"}}
	}
	return ParseResult{
		OutputFormat: "json",
		File: map[string]any{
			"name":     filename,
			"size":     len(data),
			"encoding": encName,
		},
		JSON: items,
	}
}

func (p *TextParser) String() string {
	return "TextParser"
}

// defaultTextDelimiterPattern is the regexp alternation of the flow parser's
// default delimiter set DefaultTextCodeDelimiter (rag/flow/parser/parser.py:_code
// → deepdoc TxtParser default "\n!?;。；！？"), each rune re.escape'd to mirror
// rag/nlp/delim.compile_delimiter_pattern. The Parser component has no user-facing
// delimiter config entry, so this default
// is exactly what the python flow always splits on. The shared DefaultTextCodeDelimiter
// const lives in delimiter.go so production and the alignment tests use one source
// of truth. Go's regexp.Split drops captured delimiters, so splitCapturingDelims
// walks the match indexes manually to reproduce python's re.split(r"(%s)" % pattern, txt)
// interleaving.
var (
	defaultTextDelimiterPattern = buildDelimiterPattern(DefaultTextCodeDelimiter)
	textDelimiterSplitRe        = regexp.MustCompile(defaultTextDelimiterPattern)
	textDelimiterExactRe        = regexp.MustCompile("^(?:" + defaultTextDelimiterPattern + ")$")
)

// buildDelimiterPattern builds an alternation of re.escape'd delimiter runes
// (longest-first is a no-op here: every delimiter in the default set is a
// single rune, so insertion order is preserved like python's stable sort).
func buildDelimiterPattern(delims string) string {
	parts := make([]string, 0, len(delims))
	for _, r := range delims {
		parts = append(parts, regexp.QuoteMeta(string(r)))
	}
	return strings.Join(parts, "|")
}

// normalizeTextNewlines folds CRLF and standalone CR to LF, mirroring
// rag/nlp/delim.normalize_text_newlines so Windows-line-ending documents split
// identically to Unix ones.
func normalizeTextNewlines(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

// splitCapturingDelims reproduces python re.split(r"(%s)" % pattern, s): it
// splits on the regexp and includes each matched delimiter as its own element
// (with empty strings between adjacent delimiters) so callers can keep or drop
// them. Go's regexp.Split discards captured groups, hence the manual walk.
func splitCapturingDelims(s string, re *regexp.Regexp) []string {
	locs := re.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return []string{s}
	}
	out := make([]string, 0, 2*len(locs)+1)
	prev := 0
	for _, loc := range locs {
		out = append(out, s[prev:loc[0]])
		out = append(out, s[loc[0]:loc[1]])
		prev = loc[1]
	}
	out = append(out, s[prev:])
	return out
}

// textParserItems splits data into fine segments on the flow parser's default
// delimiter set, mirroring deepdoc.parser.txt_parser.TxtParser.parser_txt up to
// (but not including) the OVER_CAP token merge. The token merge is intentionally
// NOT performed here — chunking ownership stays with the downstream Chunker per
// contract #17799 — so item counts differ from the
// python flow's merged chunks and are reconciled by the stitch-compare alignment
// test (align_test.go).
//
// Unlike the python signature default keep_delimiters=False, the flow _code
// path calls TxtParser with keep_delimiters=True, so each segment keeps its
// trailing delimiter attached (sentence-ending punctuation preserved for code
// and prose).
//
// No per-item byte cap is applied: the parser is a pure delimiter splitter, just
// like python's parser_txt (which also does no size slicing). Sizing belongs to
// the chunker and the embedding truncation step, so a continuous run longer than
// the embedding token budget (e.g. a minified / no-newline file) is kept as one
// item here and collapsed to one chunk downstream — matching python's behaviour
// rather than diverging from it.
func textParserItems(data []byte) []map[string]any {
	txt := normalizeTextNewlines(string(data))
	secs := splitCapturingDelims(txt, textDelimiterSplitRe)

	var paras []string
	for i, sec := range secs {
		if textDelimiterExactRe.MatchString(sec) {
			continue
		}
		if sec == "" {
			continue
		}
		// keep_delimiters=True: append the delimiter to the segment it
		// follows, mirroring python's parser_txt.
		if i+1 < len(secs) && textDelimiterExactRe.MatchString(secs[i+1]) {
			sec += secs[i+1]
		}
		paras = append(paras, sec)
	}

	var items []map[string]any
	for _, para := range paras {
		text := strings.TrimSpace(para)
		if text == "" {
			continue
		}
		items = append(items, map[string]any{
			"text":         text,
			"doc_type_kwd": "text",
		})
	}
	return items
}
