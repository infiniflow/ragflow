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

	"ragflow/internal/ingestion/component/schema"
)

// numberedHeadingLevels mirrors the last level family shipped in the Book,
// Paper, Laws and Manual templates. The final pattern uses Python's `{,n}`
// repetition, which Go's regexp does not accept.
var numberedHeadingLevels = [][]string{{
	`第[零一二三四五六七八九十百0-9]+章`,
	`第[零一二三四五六七八九十百0-9]+节`,
	`[零一二三四五六七八九十百]+[ 、]`,
	`[\(（][零一二三四五六七八九十百]+[\)）]`,
	`[\(（][0-9]{,2}[\)）]`,
}}

func numberedHeadingParam() *titleChunkerParam {
	p := &titleChunkerParam{}
	p.Update(map[string]any{"levels": numberedHeadingLevels, "hierarchy": 5})
	return p
}

// TestCompileLevelPatternAcceptsPythonRepetition pins the dialect fix: Python's
// re reads `{,n}` as `{0,n}`, Go's RE2 reads the braces literally, so a pattern
// copied from a Python config compiled but never matched.
func TestCompileLevelPatternAcceptsPythonRepetition(t *testing.T) {
	re := compileLevelPattern(`[\(（][0-9]{,2}[\)）]`)
	if re == nil {
		t.Fatal("compileLevelPattern returned nil")
	}
	for _, line := range []string{"(1) 概述", "（12） 方法"} {
		if !re.MatchString(line) {
			t.Errorf("pattern did not match %q; `{,n}` was treated literally", line)
		}
	}
	if re.MatchString("(123) 超出两位") {
		t.Error("pattern matched a three-digit marker; `{,2}` must bound the repetition")
	}
}

// TestResolveTitleLevels_NumberedHeadingNeedsShortText pins the second half of
// the fix. Enabling the `(1)` family makes every numbered paragraph a heading
// candidate, which is what #21585 reports: a ~300-rune numbered paragraph is
// body text, so it must stay at body level while a short numbered heading is
// promoted.
func TestResolveTitleLevels_NumberedHeadingNeedsShortText(t *testing.T) {
	long := "(1)社区口腔健康教育：是在社区范围内，以增进居民口腔健康为目标，通过有组织的社会活动，普及口腔健康知识，" +
		"提高居民自我口腔保健意识和能力，改善口腔健康状况的综合性健康教育活动，内容包括口腔健康检查、口腔卫生指导、" +
		"窝沟封闭、局部用氟、早期龋齿充填等适宜技术，并对重点人群实施分类管理。"
	records := []lineRecord{
		{text: "(三) 实施口腔卫生服务", docType: "text"},
		{text: "(1) 健康教育与健康促进", docType: "text"},
		{text: long, docType: "text"},
	}
	levels := resolveTitleLevels(records, numberedHeadingParam())
	if levels[0] == bodyLevel {
		t.Errorf("short numbered heading %q was not promoted", records[0].text)
	}
	if levels[1] == bodyLevel {
		t.Errorf("short numbered heading %q was not promoted", records[1].text)
	}
	if levels[2] != bodyLevel {
		t.Errorf("long numbered paragraph (level %d) must stay body text, not become a heading", levels[2])
	}
}

// TestHierarchyTitleChunker_LongNumberedParagraphNotRepeated pins the user
// visible outcome from #21585: with include_heading_content the heading text is
// prepended to every chunk under it, so a body paragraph promoted to a heading
// is copied into several chunks.
func TestHierarchyTitleChunker_LongNumberedParagraphNotRepeated(t *testing.T) {
	long := "(1)社区口腔健康教育：是在社区范围内，以增进居民口腔健康为目标，通过有组织的社会活动，普及口腔健康知识，" +
		"提高居民自我口腔保健意识和能力，改善口腔健康状况的综合性健康教育活动，内容包括口腔健康检查、口腔卫生指导、" +
		"窝沟封闭、局部用氟、早期龋齿充填等适宜技术，并对重点人群实施分类管理。"
	var b strings.Builder
	b.WriteString("(三) 实施口腔卫生服务\n")
	b.WriteString("基层医疗卫生机构应当组织开展口腔健康教育。\n")
	b.WriteString(long + "\n")
	for i := 0; i < 8; i++ {
		b.WriteString("后续段落" + strings.Repeat("内容", 60) + "，补充说明。\n")
	}

	c, err := NewHierarchyTitleChunker(map[string]any{
		"hierarchy":               5,
		"include_heading_content": true,
		"levels":                  numberedHeadingLevels,
	})
	if err != nil {
		t.Fatalf("NewHierarchyTitleChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "doc.txt", "text": b.String()})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, ok := schema.ChunkCarrierToMaps(out["chunks"])
	if !ok {
		t.Fatal("ChunkCarrierToMaps failed")
	}
	appearances := 0
	for _, ck := range chunks {
		text, _ := ck["text"].(string)
		appearances += strings.Count(text, long)
	}
	if appearances != 1 {
		t.Fatalf("long numbered paragraph appears in %d chunk texts, want exactly 1", appearances)
	}
}

// TestResolveTitleLevels_LongNumberedParagraphWithSpaceStaysBody: the marker
// delimiter is often the line's only space, so a space must not exempt a long
// numbered paragraph from the length guard.
func TestResolveTitleLevels_LongNumberedParagraphWithSpaceStaysBody(t *testing.T) {
	long := "(1) 社区口腔健康教育：是在社区范围内，以增进居民口腔健康为目标，通过有组织的社会活动，普及口腔健康知识，" +
		"提高居民自我口腔保健意识和能力，改善口腔健康状况的综合性健康教育活动，内容包括口腔健康检查、口腔卫生指导、" +
		"窝沟封闭、局部用氟、早期龋齿充填等适宜技术，并对重点人群实施分类管理。"
	records := []lineRecord{
		{text: "(1) 健康教育与健康促进", docType: "text"},
		{text: long, docType: "text"},
	}
	levels := resolveTitleLevels(records, numberedHeadingParam())
	if levels[0] == bodyLevel {
		t.Errorf("short numbered heading %q was not promoted", records[0].text)
	}
	if levels[1] != bodyLevel {
		t.Errorf("long numbered paragraph with a space after the marker (level %d) must stay body text", levels[1])
	}
}

// TestResolveTitleLevels_BulletFallbackSkipsLongNumberedParagraph: when no
// regex family wins, the Chunker-1.7 bullet fallback promotes numbered lines.
// It must apply the same length guard, otherwise a document with no short
// heading turns every long numbered paragraph into a heading.
func TestResolveTitleLevels_BulletFallbackSkipsLongNumberedParagraph(t *testing.T) {
	long := "(1) 社区口腔健康教育：是在社区范围内，以增进居民口腔健康为目标，通过有组织的社会活动，普及口腔健康知识，" +
		"提高居民自我口腔保健意识和能力，改善口腔健康状况的综合性健康教育活动。"
	records := []lineRecord{
		{text: "(2) 概述", docType: "text"},
		{text: long, docType: "text"},
	}
	p := &titleChunkerParam{}
	p.Update(map[string]any{"hierarchy": 3})
	levels := resolveTitleLevels(records, p)
	if levels[0] == bodyLevel {
		t.Errorf("short bullet %q was not promoted by the bullet fallback", records[0].text)
	}
	if levels[1] != bodyLevel {
		t.Errorf("long numbered paragraph (level %d) must stay body text in the bullet fallback", levels[1])
	}
}

// TestResolveTitleLevels_MultiWordNumberedHeadingStaysHeading: the space fix is
// scoped to space-free CJK runs, so a long multi-word numbered heading keeps
// its level.
func TestResolveTitleLevels_MultiWordNumberedHeadingStaysHeading(t *testing.T) {
	records := []lineRecord{
		{text: "(1) Overview of the System Architecture and Its Components", docType: "text"},
	}
	levels := resolveTitleLevels(records, numberedHeadingParam())
	if levels[0] == bodyLevel {
		t.Error("long multi-word numbered heading was demoted to body text")
	}
}

// TestResolveTitleLevels_LongMarkdownHeadingStaysHeading: an explicit ATX
// marker is structural, so the length guard must not demote it.
func TestResolveTitleLevels_LongMarkdownHeadingStaysHeading(t *testing.T) {
	p := &titleChunkerParam{}
	p.Update(map[string]any{"levels": [][]string{{`^#[^#]`, `^##[^#]`}}})
	records := []lineRecord{
		{text: "# A Comprehensive Survey of Methods for Efficient Retrieval Augmented Generation in Production Systems", docType: "text"},
		{text: "body text", docType: "text"},
	}
	levels := resolveTitleLevels(records, p)
	if levels[0] == bodyLevel {
		t.Error("long markdown heading was demoted to body text")
	}
	if levels[1] != bodyLevel {
		t.Errorf("body line level = %d, want body", levels[1])
	}
}
