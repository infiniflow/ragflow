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

import "unicode"

// DetectLanguage returns a best-effort language code based on the
// proportion of CJK characters.
func DetectLanguage(text string) string {
	cjk, total := 0, 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			cjk++
		}
		if unicode.IsLetter(r) {
			total++
		}
	}
	return languageFromCounts(cjk, total)
}

// languageFromCounts is the counting half of DetectLanguage, split out so a
// caller that already has CJK/letter counts (e.g. suffix scans) can decide the
// language without re-scanning the text. The threshold is identical.
func languageFromCounts(cjk, total int) string {
	if total > 0 && float64(cjk)/float64(total) > 0.3 {
		return "zh"
	}
	return "en"
}

// RuneCount returns the number of runes in text.
func RuneCount(text string) int {
	return len([]rune(text))
}
