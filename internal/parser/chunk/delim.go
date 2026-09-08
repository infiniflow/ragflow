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

// backtickWrappedRE matches a backtick-wrapped multi-character token.
// Case-sensitive on purpose (see #17384).
var backtickWrappedRE = regexp.MustCompile("`([^`]+)`")

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

// CompileDelimiterPatternList builds an alternation regex from a
// TokenChunker-style []string delimiter list, with a shared policy for how
// backtick-wrapped and bare entries are treated. Every non-empty entry is
// regexp.QuoteMeta'd and the result is sorted longest-first (stable by rune
// count) so a longer delimiter always wins over a shorter prefix inside it.
//
//   - Backtick-wrapped entries (“ `abc` “) contribute their INNER content as
//     the split pattern; the backticks are stripped. The wrapping is the
//     delimiter-list quoting syntax that opts an entry into the active set.
//   - Bare entries (e.g. ". ") behave according to keepBare:
//     keepBare=false (the main `delimiters` list): a bare entry is IGNORED for
//     the active pattern — it only influences merge paths when no custom
//     pattern exists.
//     keepBare=true (the `children_delimiters` list): a bare entry is KEPT
//     active, because every non-empty children delimiter splits — including
//     bare ones.
//
// Returns nil when no active entry remains. This helper exists for the
// dataflow list API. The single-string parser_config.delimiter field is not
// parsed in Go; only the []string list API is consumed.
func CompileDelimiterPatternList(delims []string, keepBare bool) *regexp.Regexp {
	var out []string
	seen := make(map[string]struct{})
	for _, d := range delims {
		if d == "" {
			continue
		}
		if strings.HasPrefix(d, "`") && strings.HasSuffix(d, "`") && len(d) >= 2 {
			inner := d[1 : len(d)-1]
			if inner == "" {
				continue
			}
			if _, ok := seen[inner]; ok {
				continue
			}
			seen[inner] = struct{}{}
			out = append(out, inner)
			continue
		}
		if !keepBare {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	if len(out) == 0 {
		return nil
	}
	sort.SliceStable(out, func(i, j int) bool {
		return utf8.RuneCountInString(out[i]) > utf8.RuneCountInString(out[j])
	})
	escaped := make([]string, 0, len(out))
	for _, d := range out {
		escaped = append(escaped, regexp.QuoteMeta(d))
	}
	return regexp.MustCompile(strings.Join(escaped, "|"))
}

// CompileDelimiterListPattern compiles a TokenChunker-style []string delimiter
// list. It is CompileDelimiterPatternList with keepBare=false: backtick entries
// contribute their inner content and bare entries are ignored for the active
// pattern (they only steer merge paths when no custom pattern exists).
//
// This helper exists for the dataflow list API.
func CompileDelimiterListPattern(delims []string) *regexp.Regexp {
	return CompileDelimiterPatternList(delims, false)
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
