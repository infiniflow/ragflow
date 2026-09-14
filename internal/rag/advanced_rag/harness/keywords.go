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
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
)

// Four-aspect keyword extraction with entity weighting.
//
// Mirrors Python harness/keywords.py.
//
// A keyword search matches only the surface forms you give it, so a single flat
// bag of terms is a poor retrieval driver: the model does not know how the corpus
// phrases a fact. This extraction asks the LLM for FOUR aspects — `entity` (what
// the fact is about), `aliases` (its surface variants), `fact_type` (words the
// corpus might use for this kind of fact, including table column abbreviations)
// and `qualifiers` (year / edition / jurisdiction / revision).
//
// `entity` is what discriminates, so it is repeated in the search query to weight
// BM25 toward it; the plain deduped union of all four aspects is used to narrow
// retrieved chunks to their keyword-bearing sentences.

const (
	// keywordEntityRepeat is copies of each entity term in the query
	// (Python: _KEYWORD_ENTITY_REPEAT).
	keywordEntityRepeat = 3
	// keywordQualifierRepeat is copies of each qualifier, weighted up like entity
	// (Python: _KEYWORD_QUALIFIER_REPEAT).
	keywordQualifierRepeat = 3
	// keywordMaxChars caps both strings (Python: _KEYWORD_MAX_CHARS).
	keywordMaxChars = 400
	// keywordExtractionTemperature pins Python keywords.py:141's
	// {"temperature": 0.1} for the extraction call.
	keywordExtractionTemperature = 0.1
)

// keywordAspects is the aspect order. Entity and qualifiers are the weighted
// pair; aliases and fact_type find/boost but must not dominate.
var keywordAspects = []string{"entity", "aliases", "fact_type", "qualifiers"}

// KeywordsSystem mirrors keywords.py::_KEYWORDS_SYSTEM.
const KeywordsSystem = `You turn ONE question into search terms for a keyword/BM25 search engine.

Emit the terms that would appear VERBATIM in a document that answers the question, sorted into FOUR
categories. Every term must come from the question itself or be a surface form of something in it.

A. "entity" — the specific thing the fact is ABOUT: proper nouns, titles, identifiers. Keep a
   multi-word entity whole, as ONE term ("Brown County", "Treaty of Versailles"); split across
   several terms its tokens match independently and drag in noise. A bare identifier — a serial,
   patent, catalogue or case number — is a complete entity on its own; never glue it to the words
   around it.
B. "aliases" — the engine matches ONLY the surface forms you supply, so emit the plausible variants
   of A: full vs. short name, native-language and transliterated forms, official vs. common name,
   acronym and its expansion, and the qualified form ("Brown County" -> "Brown County, Kansas").
C. "fact_type" — 3 to 6 words the corpus might use for this KIND of fact, since you cannot know how
   it is phrased. Spread them across registers:
     quantity of people -> population, inhabitants, residents, census, demographics, headcount
     time of an event   -> founded, established, opened, dated, began
     role of a person   -> served, appointed, elected, held, director
   SOURCES TABULATE WHAT QUESTIONS SPELL OUT: a statistic named in prose is usually written in a
   table as a column abbreviation, and the prose wording may not appear in the document at all. So
   include the abbreviation a table would use — "points per game" -> "PPG", "PTS"; "earnings per
   share" -> "EPS"; "games played" -> "GP" — and reach a superlative through its plain column too:
   "leading scorer" is found by looking for "PTS" and "PPG", not for the phrase itself.
D. "qualifiers" — year, edition, jurisdiction, revision. Worth emitting even when it looks
   redundant: the qualifier often sits in a table header or a document title that chunking has
   severed from the value. Include EVERY alternative expression of a DATE or NUMBER in the
   question — ordinals and their words ("21st" -> "twenty-first"), digits and their words
   ("2000000" -> "two million", "2 million"), and each common date format ("Aug 2nd" -> "August 2",
   "2 August", "08-02").

A and B are what FINDS the document; C and D only boost the ranking. So never withhold an entity
because you are unsure of it, and never pad C or D to reach a count.

DROP entirely: question words ("which", "who", "when", "how many"), relational scaffolding, and
generic high-frequency nouns ("year", "number", "city", "total", "list", "information"). They cost
ranking quality and retrieve nothing.

Output ONLY JSON, no prose, no code fences:
{"entity": ["<term>", ...], "aliases": ["<term>", ...], "fact_type": ["<term>", ...], "qualifiers": ["<term>", ...]}
Any category may be empty.`

// normKeyword mirrors Python _norm_keyword: normalise a term for cross-category
// dedup (lowercase, whitespace-collapsed).
func normKeyword(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// parseAspects mirrors Python _parse_aspects: parse the LLM's JSON into one
// deduped list per aspect.
//
// ONE dedup set spans all four categories: a term the model emits as both an
// entity and an alias must not collect a second share of the query's mass on the
// strength of having been named twice.
func parseAspects(raw string) map[string][]string {
	data, _ := ExtractJSON(StripThinkAndFences(raw)).(map[string]any)
	aspects := map[string][]string{}
	seen := map[string]bool{}
	for _, aspect := range keywordAspects {
		var terms []string
		for _, k := range asAspectList(data[aspect]) {
			term := strings.TrimSpace(fmt.Sprint(k))
			key := normKeyword(term)
			if term == "" || key == "" || seen[key] {
				continue
			}
			seen[key] = true
			terms = append(terms, term)
		}
		aspects[aspect] = terms
	}
	return aspects
}

// asAspectList tolerates the shapes models emit for an aspect: a list of
// strings, a list of mixed values, or a single comma-separated string.
func asAspectList(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []string:
		out := make([]any, 0, len(t))
		for _, s := range t {
			out = append(out, s)
		}
		return out
	case string:
		out := make([]any, 0, 4)
		for _, s := range strings.Split(t, ",") {
			out = append(out, s)
		}
		return out
	}
	return nil
}

// reThinkWrap matches a leading <think>...</think> preamble.
var reThinkWrap = regexp.MustCompile(`(?s)^.*</think>`)

// StripThinkAndFences removes a leading thinking preamble and Markdown fences.
// Exported so the harness (compute.go, keywords.go) and the advanced_rag package's
// moved Formalize can share one implementation.
func StripThinkAndFences(s string) string {
	s = reThinkWrap.ReplaceAllString(s, "")
	s = reFencedJSON.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// ExtractWeightedKeywords mirrors Python extract_weighted_keywords: extract the
// four aspects and return (query, keywords).
//
//   - query is the weighted search string: every entity term repeated x3 and
//     every qualifier repeated x3, so BM25 weights both inside the same query;
//     aliases and fact-type vocabulary appear once each.
//   - keywords is the plain deduped union of all four aspects (one copy each),
//     used to narrow retrieved chunks to their keyword-bearing sentences.
//
// Falls back to (question, question) when extraction fails — keyword extraction
// is an enhancement, never a precondition for retrieval.
func ExtractWeightedKeywords(ctx context.Context, model SessionModel, question string) (string, string) {
	if question == "" {
		return "", ""
	}
	aspects := map[string][]string{}
	if model != nil {
		// Mirror Python keywords.py:extract_weighted_keywords — form_message(_KEYWORDS_SYSTEM,
		// question) is run through message_fit_in(..., llm.max_length) before
		// the call, so an over-long question is trimmed to the model context
		// window exactly as in Python. The chat seam exposes llm.max_length via
		// ContextLengthModel; when no context window is available the
		// chat.EffectiveContextLength(0) default (8192) applies, exactly as
		// Python's LLM.max_length default.
		budget := 0
		if cl, ok := model.(ContextLengthModel); ok {
			budget = cl.ContextLength()
		}
		if budget <= 0 {
			budget = chat.EffectiveContextLength(0)
		}
		fitted, fitErr := chat.FitMessages(KeywordsSystem, []schema.Message{
			*schema.UserMessage(question),
		}, budget)
		if fitErr != "" {
			_LOG.Printf("[Keywords] prompt fitting failed: %s", fitErr)
		}
		// FitMessages may prepend/trim a system message; re-extract it so the
		// model call is exactly [system, user...] as Python sends msg[0] then
		// msg[1:].
		systemPrompt := KeywordsSystem
		if len(fitted) > 0 && fitted[0].Role == schema.System {
			systemPrompt = fitted[0].Content
		}
		userContent := question
		for _, m := range fitted {
			if m.Role == schema.User {
				userContent = m.Content
				break
			}
		}
		msgs := []schema.Message{
			*schema.SystemMessage(systemPrompt),
			*schema.UserMessage(userContent),
		}
		// Python hardcodes {"temperature": 0.1} (keywords.py:141) — a mechanical
		// rewrite, not a reasoning task, so it must be stable. The value is a
		// pinned constant: every production carrier implements TemperatureModel
		// (compile-time assertion on InvokerSessionModel), so the temperature is
		// always sent; a carrier without per-call temperature support falls back
		// to its own default (Go-only provider limitation, flagged).
		var reply *ModelReply
		var err error
		if tm, ok := model.(TemperatureModel); ok {
			reply, err = tm.CompleteWithTemperature(ctx, msgs, nil, keywordExtractionTemperature)
		} else {
			_LOG.Printf("[Keywords] model %T cannot carry per-call temperature; using its default (Python would send %v)", model, keywordExtractionTemperature)
			reply, err = model.Complete(ctx, msgs, nil)
		}
		if err == nil {
			aspects = parseAspects(reply.Content)
		} else {
			_LOG.Printf("[Keywords] extraction failed: %v", err)
		}
	}

	// Plain union, one copy each — used for narrowing.
	var union []string
	for _, aspect := range keywordAspects {
		union = append(union, aspects[aspect]...)
	}
	keywords := strings.Join(union, ", ")
	if keywords == "" {
		keywords = question
	}

	// Weighted query: entity x3, qualifiers x3, then aliases + fact_type once.
	var weighted []string
	for _, t := range aspects["entity"] {
		for i := 0; i < keywordEntityRepeat; i++ {
			weighted = append(weighted, t)
		}
	}
	for _, t := range aspects["qualifiers"] {
		for i := 0; i < keywordQualifierRepeat; i++ {
			weighted = append(weighted, t)
		}
	}
	for _, aspect := range []string{"aliases", "fact_type"} {
		weighted = append(weighted, aspects[aspect]...)
	}
	query := strings.Join(weighted, ", ")
	if query == "" {
		query = keywords
	}

	// Python keywords.py has NO term-count cap — only the _KEYWORD_MAX_CHARS (400)
	// hard cap on the final joined strings. We mirror exactly that: only the
	// character cap is applied, never a per-term limit.
	query = truncateRunes(query, keywordMaxChars)
	keywords = truncateRunes(keywords, keywordMaxChars)

	_LOG.Printf("[Keywords] entity x%d: %s | aliases: %s | fact-type: %s | qualifiers x%d: %s",
		keywordEntityRepeat, joinOrDash(aspects["entity"]), joinOrDash(aspects["aliases"]),
		joinOrDash(aspects["fact_type"]), keywordQualifierRepeat, joinOrDash(aspects["qualifiers"]))
	return query, keywords
}

func joinOrDash(ss []string) string {
	if len(ss) == 0 {
		return "-"
	}
	return strings.Join(ss, "; ")
}
