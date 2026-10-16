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

import (
	"strings"
	"unicode"
)

// OntologyQueryCandidates is the TBox half of query interpretation: it reads
// which declared classes and properties the question's own words name, so the
// knowledge search can be narrowed to them without asking a model to parse the
// question into a structure.
//
// It is deterministic and cheap by design. An LLM that read the question and
// returned a "graph pattern" would produce a narrowing nobody can reproduce,
// and a wrong narrowing is invisible: the search simply returns less. A lexical
// match is explainable ("the question said `shipment`, the model declares
// `shipment`"), reproducible run to run, and — because the caller gets the
// match in AppliedFilter — correctable by a reader who sees it went wrong.
//
// The output is a CANDIDATE set, never a verdict. An empty result means the
// question named nothing the model declares, which is a reason to search
// without narrowing rather than a reason to search for nothing.
func OntologyQueryCandidates(question string, declared declaredOntology, maxClasses, maxProperties int) OntologyStructureConstraints {
	var out OntologyStructureConstraints
	if strings.TrimSpace(question) == "" {
		return out
	}
	haystack := newOntologyTermHaystack(question)
	for _, name := range declared.classOrder {
		if len(out.EntityTypes) >= maxClasses {
			break
		}
		if haystack.matches(name) || haystack.matches(declared.classes[name].label) {
			out.EntityTypes = append(out.EntityTypes, name)
		}
	}
	for _, name := range declared.propOrder {
		if len(out.Properties) >= maxProperties {
			break
		}
		if haystack.matches(name) || haystack.matches(declared.props[name].label) {
			out.Properties = append(out.Properties, name)
		}
	}
	return out
}

// ontologyTermHaystack is a prepared form of the question for repeated term
// lookups: lowercase, with every non-letter/digit boundary turned into a space
// so a Latin term only matches as a whole word, while CJK terms still match by
// containment (CJK does not delimit words with spaces, so requiring a boundary
// would never match).
type ontologyTermHaystack struct {
	// spaced is the lowercase question with non-alphanumerics collapsed to
	// single spaces, padded so a term at either end is still delimited.
	spaced string
	// raw is the lowercase question, used for the CJK containment test.
	raw string
}

func newOntologyTermHaystack(question string) ontologyTermHaystack {
	lower := strings.ToLower(question)
	var b strings.Builder
	b.Grow(len(lower) + 2)
	b.WriteByte(' ')
	lastSpace := true
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	if !lastSpace {
		b.WriteByte(' ')
	}
	return ontologyTermHaystack{spaced: b.String(), raw: lower}
}

// matches reports whether the question names this declared term.
func (h ontologyTermHaystack) matches(term string) bool {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		return false
	}
	if len([]rune(term)) < ontologyTermMinRunes(term) {
		return false
	}
	if ontologyTermIsCJK(term) {
		return strings.Contains(h.raw, term)
	}
	return strings.Contains(h.spaced, " "+term+" ")
}

// ontologyTermMinRunes is the shortest term worth matching. A one- or two-letter
// Latin term ("id", "by") appears in almost every question, and matching it
// would narrow the search to a class because the question contained a common
// word — the failure mode that makes a lexical match worse than no match.
func ontologyTermMinRunes(term string) int {
	if ontologyTermIsCJK(term) {
		return 2
	}
	return 3
}

// ontologyTermIsCJK reports whether a term contains a CJK character, which is
// what decides between containment and whole-word matching.
func ontologyTermIsCJK(term string) bool {
	for _, r := range term {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			return true
		}
	}
	return false
}

// OntologyQueryCandidateLimits bound the candidate set. A question that names
// more than a handful of declared classes is a question about the model rather
// than through it, and widening the narrowing past these bounds returns the
// same rows as not narrowing at all while pretending to be specific.
const (
	OntologyQueryMaxCandidateClasses    = 6
	OntologyQueryMaxCandidateProperties = 6
)
