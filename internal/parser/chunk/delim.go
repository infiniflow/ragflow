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
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Canonical parser for the parser_config.delimiter field.
//
// Mirrors Python rag/nlp/delim.py (#17383). A delimiter field is a string
// with the grammar:
//
//	delimiter_field  := token*
//	token            := backtick_wrapped | bare_char
//	backtick_wrapped := "`" bare_char+ "`"
//	bare_char        := any single Unicode character except "`"
//
// Semantics:
//  1. Characters between matching backticks form one multi-character delimiter.
//  2. Any character outside backticks is its own single-character delimiter.
//  3. Results are deduplicated and sorted longest-first (stable for equal length).
//  4. CRLF and standalone CR are normalized to LF before parsing.
//  5. Matching is case-sensitive.

// backtickWrappedRE matches a backtick-wrapped multi-character token.
// Case-sensitive on purpose (see #17384).
var backtickWrappedRE = regexp.MustCompile("`([^`]+)`")

// NormalizeTextNewlines converts CRLF and standalone CR to LF.
func NormalizeTextNewlines(text string) string {
	if text == "" {
		return text
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.ReplaceAll(text, "\r", "\n")
}

// HasWrappedDelimiter reports whether the delimiter field contains at least
// one backtick-wrapped token. Used to decide the historical "custom delimiter"
// mode that bypasses chunk_token_num. Separate from whether any delimiter is
// present after parsing (bare single-character delimiters still split).
func HasWrappedDelimiter(s string) bool {
	if s == "" {
		return false
	}
	return backtickWrappedRE.MatchString(s)
}

// ParseDelimiterField parses the delimiter field into delimiter strings.
//
// Returns nil for an empty field. Whitespace characters are valid
// single-character delimiters. Output is sorted longest-first and
// deduplicated while preserving first-occurrence order for equal-length
// items (the sort is stable). CRLF/CR inside the field are normalized to LF.
func ParseDelimiterField(s string) []string {
	if s == "" {
		return nil
	}
	normalized := NormalizeTextNewlines(s)

	var delimiters []string
	seen := make(map[string]struct{})
	cursor := 0
	for _, loc := range backtickWrappedRE.FindAllStringSubmatchIndex(normalized, -1) {
		// loc: [fullStart, fullEnd, groupStart, groupEnd]
		start, end := loc[0], loc[1]
		for _, ch := range runesOf(normalized[cursor:start]) {
			if _, ok := seen[ch]; ok {
				continue
			}
			seen[ch] = struct{}{}
			delimiters = append(delimiters, ch)
		}
		token := normalized[loc[2]:loc[3]]
		if token != "" {
			if _, ok := seen[token]; !ok {
				seen[token] = struct{}{}
				delimiters = append(delimiters, token)
			}
		}
		cursor = end
	}
	for _, ch := range runesOf(normalized[cursor:]) {
		if _, ok := seen[ch]; ok {
			continue
		}
		seen[ch] = struct{}{}
		delimiters = append(delimiters, ch)
	}

	sort.SliceStable(delimiters, func(i, j int) bool {
		return utf8.RuneCountInString(delimiters[i]) > utf8.RuneCountInString(delimiters[j])
	})
	return delimiters
}

// CompileDelimiterPattern builds an alternation regex from delimiter strings.
//
// Each delimiter is regexp.QuoteMeta'd so whitespace and metacharacters match
// literally. Returns nil when delimiters is empty or yields no non-empty
// entries. The pattern is intended for use with a capturing split (so
// delimiters appear in the split output) or FindAllStringIndex.
func CompileDelimiterPattern(delimiters []string) *regexp.Regexp {
	if len(delimiters) == 0 {
		return nil
	}
	escaped := make([]string, 0, len(delimiters))
	for _, d := range delimiters {
		if d == "" {
			continue
		}
		escaped = append(escaped, regexp.QuoteMeta(d))
	}
	if len(escaped) == 0 {
		return nil
	}
	return regexp.MustCompile(strings.Join(escaped, "|"))
}

// CompileDelimiterListPattern compiles a TokenChunker-style []string delimiter
// list. Entries wrapped in backticks contribute their inner content as a
// split pattern (Python token_chunker / historical Go compileDelimPattern).
// Bare list entries are ignored for the active pattern — they are only used
// by merge paths when no custom pattern exists.
//
// Prefer ParseDelimiterField for the single-string parser_config.delimiter
// field. This helper exists for the dataflow list API.
func CompileDelimiterListPattern(delims []string) *regexp.Regexp {
	var custom []string
	for _, d := range delims {
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "`") && strings.HasSuffix(d, "`") && len(d) >= 2 {
			inner := d[1 : len(d)-1]
			if inner == "" {
				continue
			}
			custom = append(custom, inner)
		}
	}
	if len(custom) == 0 {
		return nil
	}
	sort.SliceStable(custom, func(i, j int) bool {
		return utf8.RuneCountInString(custom[i]) > utf8.RuneCountInString(custom[j])
	})
	escaped := make([]string, 0, len(custom))
	for _, d := range custom {
		escaped = append(escaped, regexp.QuoteMeta(d))
	}
	return regexp.MustCompile(strings.Join(escaped, "|"))
}

// HasCustomDelimiterList reports whether any entry in a TokenChunker-style
// delimiter list uses backtick syntax.
func HasCustomDelimiterList(delims []string) bool {
	for _, d := range delims {
		if strings.HasPrefix(d, "`") && strings.HasSuffix(d, "`") && len(d) >= 2 {
			return true
		}
	}
	return false
}

// runesOf splits s into individual Unicode characters (as strings).
func runesOf(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0, utf8.RuneCountInString(s))
	for _, r := range s {
		out = append(out, string(r))
	}
	return out
}
