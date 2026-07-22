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

package chunker

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// tocText
// ---------------------------------------------------------------------------

func TestTocText_Basic(t *testing.T) {
	r := lineRecord{text: "Hello World"}
	if got := tocText(r); got != "Hello World" {
		t.Errorf("tocText = %q, want %q", got, "Hello World")
	}
}

func TestTocText_StripsAtAtSuffix(t *testing.T) {
	r := lineRecord{text: "第一章 概述@@0.5"}
	if got := tocText(r); got != "第一章 概述" {
		t.Errorf("tocText = %q, want %q", got, "第一章 概述")
	}
}

func TestTocText_StripsWhitespace(t *testing.T) {
	r := lineRecord{text: "  第一章 概述  "}
	if got := tocText(r); got != "第一章 概述" {
		t.Errorf("tocText = %q, want %q", got, "第一章 概述")
	}
}

func TestTocText_AtAtOnly(t *testing.T) {
	r := lineRecord{text: "@@0.5"}
	if got := tocText(r); got != "" {
		t.Errorf("tocText = %q, want %q", got, "")
	}
}

func TestTocText_EmptyString(t *testing.T) {
	r := lineRecord{text: ""}
	if got := tocText(r); got != "" {
		t.Errorf("tocText = %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// tocPrefix
// ---------------------------------------------------------------------------

func TestTocPrefix_CJK_ThreeChars(t *testing.T) {
	// Python's get(i)[:3] slices code points, not bytes.
	// "第一章 概述"[:3] → "第一章" (3 characters, 9 bytes).
	// Go must use []rune conversion to get the same result.
	r := lineRecord{text: "第一章 概述"}
	if got := tocPrefix(r, false); got != "第一章" {
		t.Errorf("tocPrefix(eng=false) = %q, want %q", got, "第一章")
	}
}

func TestTocPrefix_CJK_ShortText(t *testing.T) {
	r := lineRecord{text: "AB"}
	if got := tocPrefix(r, false); got != "AB" {
		t.Errorf("tocPrefix(eng=false, short) = %q, want %q", got, "AB")
	}
}

func TestTocPrefix_CJK_EmptyText(t *testing.T) {
	r := lineRecord{text: ""}
	if got := tocPrefix(r, false); got != "" {
		t.Errorf("tocPrefix(eng=false, empty) = %q, want %q", got, "")
	}
}

func TestTocPrefix_English_TwoWords(t *testing.T) {
	r := lineRecord{text: "Chapter One Introduction"}
	got := tocPrefix(r, true)
	if got != "Chapter One" {
		t.Errorf("tocPrefix(eng=true) = %q, want %q", got, "Chapter One")
	}
}

func TestTocPrefix_English_SingleWord(t *testing.T) {
	r := lineRecord{text: "Introduction"}
	got := tocPrefix(r, true)
	if got != "Introduction" {
		t.Errorf("tocPrefix(eng=true, single) = %q, want %q", got, "Introduction")
	}
}

func TestTocPrefix_English_EmptyText(t *testing.T) {
	r := lineRecord{text: ""}
	got := tocPrefix(r, true)
	if got != "" {
		t.Errorf("tocPrefix(eng=true, empty) = %q, want %q", got, "")
	}
}

// ---------------------------------------------------------------------------
// isEnglishRecords
// ---------------------------------------------------------------------------

func TestIsEnglishRecords_AllEnglish(t *testing.T) {
	records := []lineRecord{
		{text: "Hello world"},
		{text: "This is a test"},
		{text: "Numbers 123 and symbols."},
		{text: `Text with "double quotes" inside`},
	}
	if !isEnglishRecords(records) {
		t.Error("expected English records to be detected as English")
	}
}

func TestIsEnglishRecords_AllChinese(t *testing.T) {
	records := []lineRecord{
		{text: "第一章 概述"},
		{text: "这是一个测试"},
		{text: "目录"},
	}
	if isEnglishRecords(records) {
		t.Error("expected Chinese records not to be detected as English")
	}
}

func TestIsEnglishRecords_MixedBelowThreshold(t *testing.T) {
	// 20% English, 80% Chinese — should NOT be English (>80% threshold).
	records := []lineRecord{
		{text: "Hello world"},
		{text: "第一章 概述"},
		{text: "第二章节"},
		{text: "第三部分"},
		{text: "第四章内容"},
	}
	if isEnglishRecords(records) {
		t.Error("expected mixed records with 20% English to not pass >80% threshold")
	}
}

func TestIsEnglishRecords_EmptyRecords(t *testing.T) {
	if isEnglishRecords(nil) {
		t.Error("nil records should return false")
	}
	if isEnglishRecords([]lineRecord{}) {
		t.Error("empty records should return false")
	}
}

func TestIsEnglishRecords_SkipsEmptyText(t *testing.T) {
	records := []lineRecord{
		{text: "Hello"},
		{text: ""},
		{text: "   "},
		{text: "World"},
	}
	if !isEnglishRecords(records) {
		t.Error("empty/blank lines should be skipped, both are English")
	}
}

func TestIsEnglishRecords_ExactlyAtThreshold(t *testing.T) {
	// 4 of 5 = 80% — NOT English (threshold is >0.8, strictly greater).
	records := []lineRecord{
		{text: "Hello"},
		{text: "World"},
		{text: "Test"},
		{text: "Line"},
		{text: "这是一个中文句子"},
	}
	if isEnglishRecords(records) {
		t.Error("80% English should NOT pass the >0.8 threshold")
	}
}

// ---------------------------------------------------------------------------
// removeContentsTable
// ---------------------------------------------------------------------------

func TestRemoveContentsTable_EmptyRecords(t *testing.T) {
	got := removeContentsTable(nil)
	if len(got) != 0 {
		t.Errorf("nil input: got %d records, want 0", len(got))
	}
	got = removeContentsTable([]lineRecord{})
	if len(got) != 0 {
		t.Errorf("empty input: got %d records, want 0", len(got))
	}
}

func TestRemoveContentsTable_NoTOC(t *testing.T) {
	records := []lineRecord{
		{text: "第一章 概述"},
		{text: "这是正文内容"},
		{text: "第二章 详情"},
	}
	got := removeContentsTable(records)
	if len(got) != 3 {
		t.Errorf("no TOC: got %d records, want 3", len(got))
	}
}

func TestRemoveContentsTable_CJK_RemovesTOCBlock(t *testing.T) {
	// Simulates a Chinese document with a TOC section.
	records := []lineRecord{
		{text: "第一章 概述"},
		{text: "这是正文内容"},
		{text: "目录"},
		{text: "第一章 概述"},
		{text: "第二章 详情"},
		{text: "第一节 背景"},
		{text: "第一章 概述"}, // This matches the prefix "第一章" — marks end of TOC
		{text: "真实正文内容"},
	}
	got := removeContentsTable(records)

	// Expected: "第一章 概述", "这是正文内容", "第一章 概述", "真实正文内容"
	// The TOC block (lines 2-6, 0-indexed) should be removed,
	// leaving: line 0, line 1, and from line 6 onward.
	if len(got) == 0 {
		t.Fatal("got empty result after TOC removal")
	}
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 4 {
		t.Errorf("want 4 records, got %d: %v", len(texts), texts)
	}
	if texts[0] != "第一章 概述" {
		t.Errorf("texts[0] = %q, want %q", texts[0], "第一章 概述")
	}
	if texts[1] != "这是正文内容" {
		t.Errorf("texts[1] = %q, want %q", texts[1], "这是正文内容")
	}
	if texts[2] != "第一章 概述" {
		t.Errorf("texts[2] = %q, want %q", texts[2], "第一章 概述")
	}
	if texts[3] != "真实正文内容" {
		t.Errorf("texts[3] = %q, want %q", texts[3], "真实正文内容")
	}
}

func TestRemoveContentsTable_English_RemovesTOCBlock(t *testing.T) {
	records := []lineRecord{
		{text: "Chapter 1 Overview"},
		{text: "Main body text here"},
		{text: "Contents"},
		{text: "Chapter 1 Overview"},
		{text: "Chapter 2 Details"},
		{text: "Section 1.1 Background"},
		{text: "Chapter 1 Overview"}, // Matches prefix "Chapter 1" — marks end of TOC
		{text: "Real body content"},
	}
	got := removeContentsTable(records)
	if len(got) == 0 {
		t.Fatal("got empty result after English TOC removal")
	}
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 4 {
		t.Errorf("want 4 records, got %d: %v", len(texts), texts)
	}
	if texts[2] != "Chapter 1 Overview" {
		t.Errorf("texts[2] = %q, want %q", texts[2], "Chapter 1 Overview")
	}
}

func TestRemoveContentsTable_TOCWithAtAtSuffix(t *testing.T) {
	// @@ suffixes should be stripped before matching.
	records := []lineRecord{
		{text: "前言"},
		{text: "目录@@0.8"},
		{text: "第一章 概述@@0.9"},
		{text: "第一章 概述"}, // Matched prefix — end of TOC
		{text: "正文开始"},
	}
	got := removeContentsTable(records)
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 3 {
		t.Errorf("want 3 records, got %d: %v", len(texts), texts)
	}
	// "前言" stays, TOC block removed, "第一章 概述" and "正文开始" remain.
	if texts[0] != "前言" {
		t.Errorf("texts[0] = %q", texts[0])
	}
	if texts[1] != "第一章 概述" {
		t.Errorf("texts[1] = %q", texts[1])
	}
}

func TestRemoveContentsTable_TOCAtBeginning(t *testing.T) {
	records := []lineRecord{
		{text: "目录"},
		{text: "第一章 概述"},
		{text: "第一章 概述"}, // End of TOC
		{text: "正文"},
	}
	got := removeContentsTable(records)
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 2 {
		t.Errorf("want 2 records, got %d: %v", len(texts), texts)
	}
	if texts[0] != "第一章 概述" || texts[1] != "正文" {
		t.Errorf("unexpected: %v", texts)
	}
}

func TestRemoveContentsTable_TOCAtEnd_NoMatch(t *testing.T) {
	// TOC at the end with no matching prefix within 128 lines.
	records := []lineRecord{
		{text: "正文内容"},
		{text: "目录"},
		{text: "第一章 概述"},
	}
	got := removeContentsTable(records)
	// TOC header popped, then first TOC entry popped, then
	// no matching prefix found within 128 lines → loop breaks at line 96.
	// The remaining records (nothing after the TOC block) stay.
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 1 {
		t.Errorf("want 1 record, got %d: %v", len(texts), texts)
	}
	if texts[0] != "正文内容" {
		t.Errorf("texts[0] = %q", texts[0])
	}
}

func TestRemoveContentsTable_128LineBoundary(t *testing.T) {
	// Build a TOC with >128 entries — the forward scan stops at 128.
	records := []lineRecord{
		{text: "前言"},
		{text: "目录"},
	}
	// Add first TOC entry after "目录"
	records = append(records, lineRecord{text: "第一章 概述"})
	// Add 150 TOC entries (all with different prefixes, so no match within 128).
	for range 150 {
		records = append(records, lineRecord{text: "其他内容"})
	}
	// The matching line is at position beyond 128 — won't be found.
	records = append(records, lineRecord{text: "第一章 概述"})
	records = append(records, lineRecord{text: "正文"})

	got := removeContentsTable(records)
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	// "前言" stays. TOC header popped, first entry popped.
	// 128-item scan finds no match (all "其他内容").
	// Loop breaks on i >= len(records) || prefix == "". Remaining records (150 + 2) stay.
	if len(texts) != 153 {
		t.Errorf("want 153 records, got %d", len(texts))
	}
	if texts[0] != "前言" {
		t.Errorf("texts[0] = %q", texts[0])
	}
}

func TestRemoveContentsTable_TOCWithoutPrefix(t *testing.T) {
	// TOC title exists but the line right after is empty — prefix will be "".
	records := []lineRecord{
		{text: "目录"},
	}
	got := removeContentsTable(records)
	if len(got) != 0 {
		t.Errorf("TOC-only: got %d records, want 0", len(got))
	}
}

func TestRemoveContentsTable_WhitespaceInTOCTitle(t *testing.T) {
	// Collapsed whitespace in TOC title still matches the pattern.
	records := []lineRecord{
		{text: "前言"},
		{text: "目  录"}, // Extra spaces — collapsing matches "目录"
		{text: "第一章 概述"},
		{text: "第一章 概述"}, // Matches
		{text: "正文"},
	}
	got := removeContentsTable(records)
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 3 {
		t.Errorf("want 3 records, got %d: %v", len(texts), texts)
	}
}

func TestRemoveContentsTable_MultipleTOCs(t *testing.T) {
	// A document with two TOC blocks — both should be removed.
	// CJK document (most records are Chinese) → eng=false, prefix is first 3 runes.
	records := []lineRecord{
		{text: "目录"},
		{text: "第一章 概述"},
		{text: "第二章 详情"},
		{text: "第一章 概述"}, // End of first TOC (prefix "第一章")
		{text: "正文插入"},
		{text: "Contents"},
		{text: "Chapter 1 Overview"},
		{text: "Chapter 1 Overview"}, // End of second TOC (prefix "Cha" in CJK mode)
		{text: "Final body"},
	}
	got := removeContentsTable(records)
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	// First TOC removes: "目录", "第一章 概述"(first entry), "第二章 详情".
	// Second TOC removes: "Contents", "Chapter 1 Overview"(first entry).
	// Remaining: "第一章 概述", "正文插入", "Chapter 1 Overview"(match), "Final body".
	if len(texts) != 4 {
		t.Errorf("want 4 records, got %d: %v", len(texts), texts)
	}
}

func TestRemoveContentsTable_AcknowledgeTitle(t *testing.T) {
	// "acknowledge" and "致谢" match tocTitlePattern.
	records := []lineRecord{
		{text: "前言"},
		{text: "致谢"},
		{text: "感谢 领导"}, // CJK prefix "感谢" (first 3 runes)
		{text: "感谢 领导"}, // Matches prefix — end of TOC
		{text: "正文内容"},
	}
	got := removeContentsTable(records)
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 3 {
		t.Errorf("want 3 records, got %d: %v", len(texts), texts)
	}
	if texts[0] != "前言" || texts[1] != "感谢 领导" || texts[2] != "正文内容" {
		t.Errorf("unexpected: %v", texts)
	}
}

func TestRemoveContentsTable_CaseInsensitive(t *testing.T) {
	records := []lineRecord{
		{text: "Chapter 1 Introduction"},
		{text: "Main body"},
		{text: "CONTENTS"}, // Must match case-insensitively
		{text: "Chapter 1 Introduction"},
		{text: "Chapter 1 Introduction"}, // Matches "Chapter 1"
		{text: "After TOC body"},
	}
	got := removeContentsTable(records)
	texts := make([]string, len(got))
	for i, r := range got {
		texts[i] = r.text
	}
	if len(texts) != 4 {
		t.Errorf("want 4 records, got %d: %v", len(texts), texts)
	}
}

// ---------------------------------------------------------------------------
// tocCollapseSpaceRe
// ---------------------------------------------------------------------------

func TestTocCollapseSpaceRe_CollapsesSpaces(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"hello world", "helloworld"},
		{"目  录", "目录"},
		{"a \u3000 b", "ab"},
		{"no-spaces", "no-spaces"},
	}
	for _, tt := range tests {
		got := tocCollapseSpaceRe.ReplaceAllString(tt.input, "")
		if got != tt.want {
			t.Errorf("tocCollapseSpaceRe(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// tocTitlePattern
// ---------------------------------------------------------------------------

func TestTocTitlePattern_Matches(t *testing.T) {
	matches := []string{
		"contents", "CONTENTS", "Contents",
		"目录", "目次",
		"table of contents", "TABLE OF CONTENTS",
		"致谢", "acknowledge", "ACKNOWLEDGE",
	}
	for _, m := range matches {
		if !tocTitlePattern.MatchString(m) {
			t.Errorf("tocTitlePattern should match %q", m)
		}
	}
}

func TestTocTitlePattern_NonMatches(t *testing.T) {
	nonMatches := []string{
		"table of content", // singular
		"catalog",
		"index",
		"目录 ",
		" 目录",
		"contents.", // trailing punctuation
	}
	for _, m := range nonMatches {
		if tocTitlePattern.MatchString(m) {
			t.Errorf("tocTitlePattern should NOT match %q", m)
		}
	}
}

// TestRemoveContentsTable_SpaceNormalization verifies the Python-faithful
// behaviour of removeContentsTable: Python's remove_contents_table collapses
// ALL whitespace (re.sub(r"( | |\u3000)+", "", ...)) before matching the TOC
// title pattern, not just trimming. So TOC headers padded with leading or
// trailing spaces (or full-width spaces) are still detected and removed in
// both Python and Go. This is the function-level counterpart to the raw
// tocTitlePattern anchoring test above.
func TestRemoveContentsTable_SpaceNormalization(t *testing.T) {
	paddedHeaders := []string{
		"目录 ",
		" 目录",
		" 目录 ",
		"目\u3000录", // full-width (ideographic) space
	}
	for _, header := range paddedHeaders {
		records := []lineRecord{
			{text: "正文"},
			{text: header}, // must be detected and removed
			{text: "第一章 总则"},
			{text: "其他内容"},
			{text: "正文内容"},
		}
		got := removeContentsTable(records)
		for _, r := range got {
			if strings.TrimSpace(r.text) == strings.TrimSpace(header) {
				t.Errorf("padded TOC header %q should be removed, got %v", header, got)
			}
		}
	}
}
