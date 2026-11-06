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

package service

import "testing"

// TestOntologyEvidenceOfKeepsOnlyVerifiedQuotes: the writer stores a quote
// together with the span it was located at, so an entry without one never
// passed the gate and must not reach the reader as if it had.
func TestOntologyEvidenceOfKeepsOnlyVerifiedQuotes(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"name":"Ada","type":"person","evidence":[` +
			`{"quote":"Ada Lovelace was born in London.","chunk_id":"c1","start":0,"end":33},` +
			`{"chunk_id":"c2","start":5,"end":9}]}`,
	}
	got := ontologyEvidenceOf(row)
	if len(got) != 1 {
		t.Fatalf("kept %d entries, want 1 (the quote-less entry is not evidence)", len(got))
	}
	if got[0]["quote"] != "Ada Lovelace was born in London." {
		t.Fatalf("quote = %v", got[0]["quote"])
	}
	if got[0]["chunk_id"] != "c1" || got[0]["start"] != 0.0 || got[0]["end"] != 33.0 {
		t.Fatalf("span = %v/%v/%v, want c1/0/33", got[0]["chunk_id"], got[0]["start"], got[0]["end"])
	}
}

// TestOntologyEvidenceOfIgnoresRowsWithoutQuotes: a row compiled before its
// template asked for evidence, or one whose quotes were all rejected, reports
// nothing rather than an empty list — the panel says "no source text" either
// way, and an empty list would read as "evidence found, but blank".
func TestOntologyEvidenceOfIgnoresRowsWithoutQuotes(t *testing.T) {
	if got := ontologyEvidenceOf(map[string]interface{}{"content_with_weight": "not json"}); got != nil {
		t.Fatalf("malformed payload yielded %v", got)
	}
	noEvidence := map[string]interface{}{
		"content_with_weight": `{"name":"Ada","type":"person","source_chunk_ids":["c1"]}`,
	}
	if got := ontologyEvidenceOf(noEvidence); got != nil {
		t.Fatalf("a row without an evidence key yielded %v", got)
	}
	allRejected := map[string]interface{}{
		"content_with_weight": `{"name":"Ada","type":"person","evidence":[{"chunk_id":"c1"}]}`,
	}
	if got := ontologyEvidenceOf(allRejected); got != nil {
		t.Fatalf("a row whose only evidence carries no quote yielded %v", got)
	}
}
