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

package harness

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestNormKeyword(t *testing.T) {
	cases := map[string]string{
		"Brown  County":  "brown county",
		"  OMIYA  Soft ": "omiya soft",
		"":               "",
	}
	for in, want := range cases {
		if got := normKeyword(in); got != want {
			t.Errorf("normKeyword(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAspectsDedupsAcrossCategories(t *testing.T) {
	// ONE dedup set spans all four categories: a term emitted as both an entity
	// and an alias must not collect a second share of the query's mass purely
	// for having been named twice.
	raw := `{"entity": ["Brown County", "Kansas"], "aliases": ["brown county", "Brown County, Kansas"],
	         "fact_type": ["population", "census"], "qualifiers": ["2020"]}`
	got := parseAspects(raw)
	if len(got["entity"]) != 2 {
		t.Errorf("entity = %v, want 2 terms", got["entity"])
	}
	// "brown county" (alias) is dropped: already seen as an entity.
	if len(got["aliases"]) != 1 || got["aliases"][0] != "Brown County, Kansas" {
		t.Errorf("aliases = %v, want only the qualified form", got["aliases"])
	}
	if len(got["fact_type"]) != 2 || len(got["qualifiers"]) != 1 {
		t.Errorf("fact_type/qualifiers = %v/%v", got["fact_type"], got["qualifiers"])
	}
}

func TestParseAspectsToleratesShapesAndGarbage(t *testing.T) {
	// A comma-separated string instead of a list.
	got := parseAspects(`{"entity": "OmiyaSoft, Culdcept", "aliases": []}`)
	if len(got["entity"]) != 2 {
		t.Errorf("string form = %v, want 2 terms", got["entity"])
	}
	// Unparseable / empty input yields empty aspects, never a panic.
	for _, raw := range []string{"", "not json", "```json {broken```", "[]", "null"} {
		got := parseAspects(raw)
		for _, aspect := range keywordAspects {
			if len(got[aspect]) != 0 {
				t.Errorf("parseAspects(%q) produced %s=%v, want empty", raw, aspect, got[aspect])
			}
		}
	}
}

func TestExtractWeightedKeywordsWeighting(t *testing.T) {
	mdl := &fakeModel{replies: []*ModelReply{{
		Content: `{"entity": ["Brown County"], "aliases": ["Brown County, Kansas"],
		           "fact_type": ["population", "census"], "qualifiers": ["2020"]}`,
	}}}
	query, keywords := ExtractWeightedKeywords(context.Background(), mdl, "What is the population of Brown County?")

	// query: entity x3, qualifiers x3, then aliases + fact_type once each.
	// NOTE: the alias "Brown County, Kansas" itself contains a comma, so a naive
	// split on ", " over-counts — Python joins the same way and BM25 tokenises
	// on whitespace, so this is expected. Assert by prefix/containment.
	if !strings.HasPrefix(query, "Brown County, Brown County, Brown County, ") {
		t.Errorf("query = %q, want the entity repeated x3 first", query)
	}
	if !strings.Contains(query, "2020, 2020, 2020") {
		t.Errorf("query = %q, want the qualifier repeated x3", query)
	}
	for _, want := range []string{"Brown County, Kansas", "population", "census"} {
		if !strings.Contains(query, want) {
			t.Errorf("query = %q, missing %q (aliases/fact_type appear once)", query, want)
		}
	}
	// keywords: plain union, one copy each, in aspect order.
	wantKeys := "Brown County, Brown County, Kansas, population, census, 2020"
	if keywords != wantKeys {
		t.Errorf("keywords = %q, want %q", keywords, wantKeys)
	}
}

func TestExtractWeightedKeywordsFallsBackToQuestion(t *testing.T) {
	q := "Who created Culdcept?"
	// No model at all.
	query, keywords := ExtractWeightedKeywords(context.Background(), nil, q)
	if query != q || keywords != q {
		t.Errorf("no model = (%q,%q), want the question for both", query, keywords)
	}
	// A model that fails.
	query, keywords = ExtractWeightedKeywords(context.Background(), &errModel{}, q)
	if query != q || keywords != q {
		t.Errorf("failed model = (%q,%q), want the question for both", query, keywords)
	}
	// A model that returns nothing usable.
	query, keywords = ExtractWeightedKeywords(context.Background(),
		&fakeModel{replies: []*ModelReply{{Content: "garbage"}}}, q)
	if query != q || keywords != q {
		t.Errorf("garbage reply = (%q,%q), want the question for both", query, keywords)
	}
	// Empty question.
	if a, b := ExtractWeightedKeywords(context.Background(), &fakeModel{}, ""); a != "" || b != "" {
		t.Errorf("empty question = (%q,%q), want empty", a, b)
	}
}

func TestExtractWeightedKeywordsCapsLength(t *testing.T) {
	// A pathological term set must be capped, not truncate mid-term into
	// something that still pollutes the query.
	var terms []string
	for i := 0; i < 100; i++ {
		terms = append(terms, "term"+strings.Repeat("x", 10)+string(rune('a'+i%26)))
	}
	list := `["` + strings.Join(terms, `", "`) + `"]`
	mdl := &fakeModel{replies: []*ModelReply{{
		Content: `{"entity": ` + list + `, "aliases": [], "fact_type": [], "qualifiers": []}`,
	}}}
	query, keywords := ExtractWeightedKeywords(context.Background(), mdl, "q")
	if len(query) > keywordMaxChars {
		t.Errorf("query = %d chars, want <= %d", len(query), keywordMaxChars)
	}
	if len(keywords) > keywordMaxChars {
		t.Errorf("keywords = %d chars, want <= %d", len(keywords), keywordMaxChars)
	}
}

func TestTemperatureModelExtension(t *testing.T) {
	// Keyword extraction runs at 0.1 in Python. The Go seam exposes temperature
	// through an OPTIONAL interface so existing SessionModel implementations
	// keep working unchanged.
	im := &InvokerSessionModel{}
	var _ TemperatureModel = im // must implement the extension

	// A model that is not a TemperatureModel must still work (falls back).
	mdl := &fakeModel{replies: []*ModelReply{{
		Content: `{"entity": ["Culdcept"], "aliases": [], "fact_type": [], "qualifiers": []}`,
	}}}
	query, _ := ExtractWeightedKeywords(context.Background(), mdl, "q")
	if !strings.Contains(query, "Culdcept") {
		t.Errorf("non-temperature model: query = %q, want the entity weighted in", query)
	}
}

// errModel implements SessionModel but always fails Complete, so the
// keyword-extraction fallback path (no model usable) can be exercised.
type errModel struct{}

func (errModel) Complete(_ context.Context, _ []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	return nil, errors.New("simulated model failure")
}

// fakeModel replays a scripted sequence of replies so control flow can be
// driven without a provider. Shared by the keyword and arithmetic tests.
type fakeModel struct {
	replies  []*ModelReply
	calls    int
	messages []schema.Message
}

func (f *fakeModel) Complete(_ context.Context, msgs []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	f.messages = msgs
	if f.calls >= len(f.replies) {
		return &ModelReply{Content: `<state>{"new_states": []}</state>`}, nil
	}
	r := f.replies[f.calls]
	f.calls++
	return r, nil
}
