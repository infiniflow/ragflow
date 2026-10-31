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
	"sort"
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
	out.EntityTypes = ontologyMatchedTerms(haystack, declared.classOrder, maxClasses, func(name string) []string {
		c := declared.classes[name]
		return append([]string{name, c.label}, c.aliases...)
	})
	out.Properties = ontologyMatchedTerms(haystack, declared.propOrder, maxProperties, func(name string) []string {
		p := declared.props[name]
		return append([]string{name, p.label}, p.aliases...)
	})
	return out
}

// ontologyMatchedTerms keeps the declared terms the question names, strongest
// match first, then restores declaration order.
//
// surfaces lists every form the template says names one term — the declared name,
// its label and its declared aliases. Aliases are read here for the same reason
// compilation resolves them: one declaration must mean one vocabulary on both
// sides, and an alias the writer accepts but the reader ignores silently splits
// them.
//
// The ranking matters because the candidate set is capped: without it the cap is
// decided by the order the template happens to list classes in, so a model with
// thirty classes can spend all six slots on short generic names that happened to
// appear in the question and drop the one class the question is actually about.
// A longer matched term is the stronger claim, and a tie keeps declaration order
// so the result stays reproducible.
func ontologyMatchedTerms(haystack ontologyTermHaystack, order []string, limit int, surfaces func(string) []string) []string {
	type match struct {
		name  string
		index int
		runes int
	}
	matches := make([]match, 0, len(order))
	for i, name := range order {
		best := 0
		for _, term := range surfaces(name) {
			if n := haystack.matchLength(term); n > best {
				best = n
			}
		}
		if best > 0 {
			matches = append(matches, match{name: name, index: i, runes: best})
		}
	}
	if limit > 0 && len(matches) > limit {
		sort.SliceStable(matches, func(i, j int) bool {
			if matches[i].runes != matches[j].runes {
				return matches[i].runes > matches[j].runes
			}
			return matches[i].index < matches[j].index
		})
		matches = matches[:limit]
		sort.SliceStable(matches, func(i, j int) bool { return matches[i].index < matches[j].index })
	}
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.name)
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
	return h.matchLength(term) > 0
}

// matchLength reports how long a form of this term the question contains, and 0
// when it contains none. The length is the strength of the lexical claim, and the
// candidate cap is applied on it rather than on declaration order.
func (h ontologyTermHaystack) matchLength(term string) int {
	best := 0
	for _, variant := range ontologyTermVariants(term) {
		if !h.contains(variant) {
			continue
		}
		if n := len([]rune(variant)); n > best {
			best = n
		}
	}
	return best
}

// contains is the exact-form test: whole word for Latin, containment for CJK.
func (h ontologyTermHaystack) contains(term string) bool {
	if ontologyTermIsCJK(term) {
		return strings.Contains(h.raw, term)
	}
	return strings.Contains(h.spaced, " "+term+" ")
}

// ontologyTermVariants lists the surface forms of a declared term that count as
// naming it.
//
// Every form here is a spelling convention, not a synonym: separators in a
// declared name (`created_by` is also written "created by"), camelCase, and the
// plural/singular pair (`work` / "works"). Synonyms are NOT guessed — a lexical
// candidate is only useful while a reader can predict which word produced it, and
// an expansion that invents relationships between words is exactly the
// unreproducible narrowing this function exists to avoid.
func ontologyTermVariants(term string) []string {
	raw := strings.TrimSpace(term)
	if raw == "" {
		return nil
	}
	lower := strings.ToLower(raw)
	out := make([]string, 0, 5)
	seen := make(map[string]bool, 5)
	add := func(variant string) {
		variant = strings.ToLower(strings.TrimSpace(variant))
		if variant == "" || seen[variant] {
			return
		}
		// The floor applies to every form: a two-letter form is in almost every
		// question, and matching it would narrow the search for a reason nobody
		// can see.
		if len([]rune(variant)) < ontologyTermMinRunes(variant) {
			return
		}
		seen[variant] = true
		out = append(out, variant)
	}
	add(lower)
	if spaced := strings.NewReplacer("_", " ", "-", " ").Replace(lower); spaced != lower {
		add(spaced)
	}
	if camel := splitOntologyCamel(raw); camel != lower {
		add(camel)
	}
	if !ontologyTermIsCJK(lower) {
		for _, variant := range ontologyLatinNumberForms(lower) {
			add(variant)
		}
	}
	return out
}

// splitOntologyCamel turns "createdBy" into "created by" so a camelCase
// declaration is matched the way a question writes it. Only the last word of a
// run of capitals starts a new word, which keeps acronyms ("ISBNCode" →
// "isbn code") from being split inside themselves.
func splitOntologyCamel(term string) string {
	runes := []rune(strings.TrimSpace(term))
	if len(runes) < 2 {
		return strings.ToLower(string(runes))
	}
	var b strings.Builder
	b.Grow(len(runes) + 4)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextIsLower) {
				b.WriteByte(' ')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// ontologyLatinNumberForms lists the other grammatical number of a Latin term.
//
// Declared names are usually singular (`work`) while questions are usually plural
// ("which works were written by…"), and without this pair the most common way of
// asking a knowledge question fails to name any declared class. Only the last
// word is inflected, and only regular English forms are produced: an irregular
// plural has to be declared as its own label.
func ontologyLatinNumberForms(term string) []string {
	words := strings.Fields(term)
	if len(words) == 0 {
		return nil
	}
	last := words[len(words)-1]
	head := strings.Join(words[:len(words)-1], " ")
	with := func(replacement string) string {
		if head == "" {
			return replacement
		}
		return head + " " + replacement
	}
	switch {
	case strings.HasSuffix(last, "ies") && len(last) > 4:
		return []string{with(last[:len(last)-3] + "y")}
	case strings.HasSuffix(last, "ss"), strings.HasSuffix(last, "us"), strings.HasSuffix(last, "is"):
		// "address", "status", "analysis": no regular plural worth guessing, and
		// stripping the ending would produce a word that matches nothing useful.
		return nil
	case strings.HasSuffix(last, "es") && len(last) > 4:
		return []string{with(last[:len(last)-2]), with(last[:len(last)-1])}
	case strings.HasSuffix(last, "s") && len(last) > 3:
		return []string{with(last[:len(last)-1])}
	case strings.HasSuffix(last, "y") && len(last) > 3:
		return []string{with(last[:len(last)-1] + "ies")}
	case strings.HasSuffix(last, "ch"), strings.HasSuffix(last, "sh"),
		strings.HasSuffix(last, "x"), strings.HasSuffix(last, "z"):
		return []string{with(last + "es")}
	default:
		return []string{with(last + "s")}
	}
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
