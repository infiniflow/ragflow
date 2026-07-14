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

package utility

import (
	"regexp"
	"strings"
)

// keywordsSplitRE matches the delimiters used to split a keywords string into
// the important_kwd array: ASCII and CJK comma, ASCII and CJK semicolon, the
// CJK enumeration comma (、), and newlines. Mirrors Python
// task_executor.run_dataflow:879 re.split(r"[,，;；、\r\n]+", keywords).
var keywordsSplitRE = regexp.MustCompile(`[,，;；、\r\n]+`)

// nonEmpty drops empty strings from parts and returns nil if none remain. It is
// the shared tail of SplitKeywords and SplitQuestions: split by whatever
// delimiter, then prune empties and collapse an all-empty result to nil so a
// _kwd array is absent (nil) rather than [""].
func nonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SplitKeywords splits a keywords string by common (ASCII + CJK) delimiters,
// dropping empty elements. Returns nil for an empty input. It is the single
// authority for materializing the important_kwd array from the keywords
// string, shared by the Tokenizer component (in-pipeline) and the executor's
// persist-schema mapping. Mirrors Python task_executor.run_dataflow:879.
func SplitKeywords(keywords string) []string {
	if keywords == "" {
		return nil
	}
	return nonEmpty(keywordsSplitRE.Split(keywords, -1))
}

// SplitQuestions splits a questions string by newline, dropping empty lines.
// Returns nil for an empty input. It is the authority for materializing the
// question_kwd array from the questions string.
//
// NOTE: this intentionally filters empty parts (including trailing-newline
// empties), diverging from Python's str.split("\n"), which yields [""] for an
// empty input and keeps trailing empties. Filtering aligns with SplitKeywords
// and the Infinity engine path (internal/engine/infinity/chunk.go), which all
// treat "no real question" as a nil array rather than [""].
func SplitQuestions(s string) []string {
	if s == "" {
		return nil
	}
	return nonEmpty(strings.Split(s, "\n"))
}
