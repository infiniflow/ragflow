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
	"reflect"
	"strings"
	"testing"
)

func docxRecordInput(paragraphs ...string) map[string]any {
	items := make([]map[string]any, 0, len(paragraphs))
	for _, p := range paragraphs {
		items = append(items, map[string]any{"text": p, "doc_type_kwd": "text"})
	}
	return map[string]any{
		"name": "qa.docx", "file_type": "docx", "output_format": "json",
		"json": items,
	}
}

// A record-oriented DOCX (Q&A export: one record spans several paragraphs)
// must chunk one record per chunk when a backtick record delimiter is
// configured, including under the default delimiter list that also carries a
// bare "\n": the "\n" joining a record's paragraphs must not re-split it.
func TestGeneralChunkerDOCXCustomDelimiterGroupsMultiParagraphRecords(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 512,
		"delimiters":       []string{"\n", "`问：`"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := component.Invoke(t.Context(), nil, docxRecordInput(
		"问：first question",
		"名　　称：item one",
		"答：first answer",
		"问：second question",
		"名　　称：item two",
		"答：second answer",
	))
	if err != nil {
		t.Fatal(err)
	}
	texts := outputTexts(t, out)
	if len(texts) != 2 {
		t.Fatalf("chunks = %q, want exactly 2 record chunks", texts)
	}
	for _, want := range []string{"问：first question", "名　　称：item one", "答：first answer"} {
		if !strings.Contains(texts[0], want) {
			t.Fatalf("first chunk %q missing %q: the answer paragraph must share the record chunk", texts[0], want)
		}
	}
	for _, want := range []string{"问：second question", "名　　称：item two", "答：second answer"} {
		if !strings.Contains(texts[1], want) {
			t.Fatalf("second chunk %q missing %q", texts[1], want)
		}
	}
}

// Records longer than chunk_token_size still pack at paragraph boundaries and
// never merge across a record boundary.
func TestGeneralChunkerDOCXRecordGroupingCapsAtParagraphBoundaries(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 7,
		"delimiters":       []string{"`问：`"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := component.Invoke(t.Context(), nil, docxRecordInput(
		"问：alpha alpha alpha",
		"答：beta beta beta",
		"问：gamma gamma gamma",
	))
	if err != nil {
		t.Fatal(err)
	}
	texts := outputTexts(t, out)
	if len(texts) != 3 {
		t.Fatalf("chunks = %q, want 3 paragraph-capped chunks", texts)
	}
	if !strings.HasPrefix(texts[0], "问：alpha") || !strings.Contains(texts[1], "答：beta") || !strings.HasPrefix(texts[2], "问：gamma") {
		t.Fatalf("chunks = %q, want paragraph-boundary packing inside each record", texts)
	}
}

// Several records inside ONE paragraph still split at the record prefix. A
// paragraph-leading prefix is kept (it names the record); an inline match is
// dropped, the custom-delimiter contract everywhere else.
func TestGeneralChunkerDOCXRecordBoundaryMidParagraph(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 512,
		"delimiters":       []string{"`问：`"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := component.Invoke(t.Context(), nil, docxRecordInput(
		"问：q1 答：a1 问：q2 答：a2",
	))
	if err != nil {
		t.Fatal(err)
	}
	texts := outputTexts(t, out)
	if !reflect.DeepEqual(texts, []string{"问：q1 答：a1 ", "q2 答：a2"}) {
		t.Fatalf("chunks = %q, want two records split at the mid-paragraph prefix", texts)
	}
}

// When no paragraph matches the record delimiter the grouping stays out of
// the way and the previous split-per-paragraph behavior is kept.
func TestGeneralChunkerDOCXRecordGroupingInactiveWithoutBoundary(t *testing.T) {
	component, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 512,
		"delimiters":       []string{"`问：`"},
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := component.Invoke(t.Context(), nil, docxRecordInput(
		"plain paragraph one",
		"plain paragraph two",
	))
	if err != nil {
		t.Fatal(err)
	}
	if texts := outputTexts(t, out); !reflect.DeepEqual(texts, []string{"plain paragraph one", "plain paragraph two"}) {
		t.Fatalf("chunks = %q, want unchanged per-paragraph chunks", texts)
	}
}
