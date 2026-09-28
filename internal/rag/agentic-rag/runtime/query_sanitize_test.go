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
	"testing"
	"unicode/utf8"
)

// TestSanitizeRetrievalQueryUnwrapsTheSeedBlock pins the shape measured on 2026-09-20: the model
// handed the session's own seed block back as a query, and retrieval tokenized it into searches for
// "Clues", "to", "cover", "名单", "人物" — one round carried about ten such legs, and the clue they
// were meant to search was never named.
func TestSanitizeRetrievalQueryUnwrapsTheSeedBlock(t *testing.T) {
	block := "三国演义中，关羽杀了多少有姓名的人物？\n\n" +
		"Clues to cover:\n" +
		"- 三国演义 关羽 杀过哪些有名字的人\n" +
		"- 三国演义 关羽 斩杀的武将 名单\n" +
		"A gap still to close: 三国演义 关羽 斩 将 全部 列表 黄巾 程远志 邓茂"

	if got, want := sanitizeRetrievalQuery(block), "三国演义中，关羽杀了多少有姓名的人物？"; got != want {
		t.Errorf("block → %q, want the question (%q)", got, want)
	}

	// A proper query is untouched: this is the case that must not regress.
	for _, q := range []string{"关羽 温酒斩华雄", "Culdcept Saga writer birthplace", "1865"} {
		if got := sanitizeRetrievalQuery(q); got != q {
			t.Errorf("sanitizeRetrievalQuery(%q) = %q, want it unchanged", q, got)
		}
	}

	// A heading is skipped, and a list marker is stripped, so the content line is what is searched.
	if got, want := sanitizeRetrievalQuery("Clues to cover:\n2. 华雄 斩"), "华雄 斩"; got != want {
		t.Errorf("numbered clue → %q, want %q", got, want)
	}

	// A label that carries its INSTRUCTION after it is still a label. Match by prefix, or the
	// instruction becomes the content line and retrieval searches its own words: measured
	// 2026-09-20 (三国), `[BM25 search] Searching by keyword for "probe"` and asked-nothing-back=10
	// "Clues、to、cover、turn…" from one copied block.
	labelWithInstruction := "Clues to cover (turn each into your OWN SHORT probe query — a few words; " +
		"never pass this block or a whole clue list as a query):\n- 关羽 斩 名单"
	if got, want := sanitizeRetrievalQuery(labelWithInstruction), "关羽 斩 名单"; got != want {
		t.Errorf("labelled clue block → %q, want the first clue (%q)", got, want)
	}

	// An oversized single line is a paragraph, not a query: it is capped rather than sent whole.
	long := strings.Repeat("关", maxRetrievalQueryRunes+80)
	if got := sanitizeRetrievalQuery(long); utf8.RuneCountInString(got) != maxRetrievalQueryRunes {
		t.Errorf("oversized query = %d rune(s), want %d", utf8.RuneCountInString(got), maxRetrievalQueryRunes)
	}

	// Nothing content-like survives: the original is kept (capped) rather than searching nothing.
	if got := sanitizeRetrievalQuery("Clues to cover:\nA gap still to close:"); got == "" {
		t.Error("a heading-only block must not become an empty query")
	}
	if got := sanitizeRetrievalQuery("   "); got != "" {
		t.Errorf("blank query = %q, want empty", got)
	}
}
