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

package runtime

import (
	"strings"
	"unicode/utf8"
)

// maxRetrievalQueryRunes is the longest a retrieval query may be. Beyond it a string is a paragraph
// someone pasted, not a query.
const maxRetrievalQueryRunes = 120

// queryHeadings are the labels the research session's own seed uses ("Clues to cover:", "A gap
// still to close:", …). They are the lines a model hands back when it copies the seed's block.
var queryHeadings = []string{
	"clues to cover",
	"a gap still to close",
	"previously searched",
	"evidence currently at hand",
	"terms already asked for",
	"passages that carry names",
	"candidate aspects already identified",
	"account for the record",
}

// sanitizeRetrievalQuery turns something that is not a query into one.
//
// Reviewed 2026-09-20: the research session is seeded with the question, the plan's clues under
// "Clues to cover:" and the review's gaps under "A gap still to close:". A model that copies that
// BLOCK into `retrieve`'s query produces a multi-line string, and retrieval tokenizes it — so one
// call becomes searches for "Clues", "to", "cover", "名单", "人物": single words that match almost
// anything and rank nothing (measured: about ten such legs in ONE 三国 round, and the clue those
// legs were supposed to search was never named as a query at all).
//
// The sanitation is mechanical on purpose. It takes the block apart, drops the heading and bullet
// lines, and keeps the first content line — for the seed that is the QUESTION, which is a sound
// probe. It does not decide WHICH clue matters: that stays the model's call, and a model that sends
// a proper short single-line query is unaffected, because such a query passes through unchanged.
func sanitizeRetrievalQuery(q string) string {
	q = strings.TrimSpace(q)
	if q == "" {
		return ""
	}
	if !strings.ContainsAny(q, "\n\r\v\f") && utf8.RuneCountInString(q) <= maxRetrievalQueryRunes {
		return q
	}

	lines := strings.FieldsFunc(q, func(r rune) bool {
		switch r {
		case '\n', '\r', '\v', '\f':
			return true
		}
		return false
	})
	for _, raw := range lines {
		ln := trimQueryMarker(raw)
		if ln == "" || utf8.RuneCountInString(ln) < 2 {
			continue
		}
		if isQueryHeading(ln) {
			continue
		}
		// A label without a recognized name: short and ending in a colon ("Sources:").
		if strings.HasSuffix(ln, ":") && utf8.RuneCountInString(ln) <= 40 {
			continue
		}
		return truncateRunes(ln, maxRetrievalQueryRunes)
	}
	// Nothing content-like: keep the original, capped, rather than searching nothing.
	return truncateRunes(strings.ReplaceAll(strings.ReplaceAll(q, "\n", " "), "\r", " "), maxRetrievalQueryRunes)
}

// trimQueryMarker strips a list marker ("- ", "• ", "3. ") and the surrounding space, so the
// content line is what the marker was pointing at.
func trimQueryMarker(s string) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"- ", "– ", "— ", "• ", "* ", "· "} {
		if strings.HasPrefix(s, prefix) {
			s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
		}
	}
	// "12. " / "12) " / "12、"
	i := strings.IndexAny(s, ".)、")
	if i > 0 && i <= 3 && allDigits(s[:i]) {
		s = strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s)
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// isQueryHeading reports whether a line is a label the seed uses rather than its content.
//
// The match is a PREFIX, not equality: a label that carries a qualifier after it ("Clues to cover
// (turn each into your OWN short probe query — a few words; never pass this block…):") is still a
// label, and treating it as content handed retrieval the INSTRUCTION — measured 2026-09-20 三国:
// `[BM25 search] Searching by keyword for "probe"`, and asked-nothing-back=10
// "Clues、to、cover、turn…" from one copied block.
func isQueryHeading(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	lower = strings.TrimSuffix(lower, ":")
	for _, h := range queryHeadings {
		if lower == h || strings.HasPrefix(lower, h) {
			return true
		}
	}
	return false
}
