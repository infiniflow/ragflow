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

import "strings"

// Porter stemming, a faithful port of nltk.stem.porter.PorterStemmer in its
// default NLTK_EXTENSIONS mode — the mode Python's text_processing.py:221
// exercises in production (`PorterStemmer().stem`, no mode argument). Python's
// _stem calls it for every stemmable keyword token, so the keyword narrowing in
// this package must produce the SAME stems (the previous suffix-stripping
// fallback diverged: "nominations"→"nominat" vs "nomin", "flies"→"fle" vs
// "fli", "dies"→"dy" vs "die", ...).
//
// The port follows the 1980 paper's five steps plus NLTK's extensions: the
// irregular-form pool consulted first, the <=2-letter circuit breaker, the
// 4-letter "ies"/"ied" reductions, the consonant-conditional step-1c, the
// "bli"/"fulli"/recursive-"alli" step-2 rules, the "logi" measure quirk and
// the 2-letter *o exception.

// porterPool mirrors nltk's _pool: irregular forms keyed by their restored
// form (NLTK_EXTENSIONS only). Consulted before any rule runs.
var porterPool = map[string][]string{
	"sky":     {"sky", "skies"},
	"die":     {"dying"},
	"lie":     {"lying"},
	"tie":     {"tying"},
	"news":    {"news"},
	"inning":  {"innings", "inning"},
	"outing":  {"outings", "outing"},
	"canning": {"cannings", "canning"},
	"howe":    {"howe"},
	"proceed": {"proceed"},
	"exceed":  {"exceed"},
	"succeed": {"succeed"},
}

func porterStem(word string) string {
	if word == "" {
		return word
	}
	for restored, forms := range porterPool {
		for _, form := range forms {
			if word == form {
				return restored
			}
		}
	}
	// NLTK_EXTENSIONS short-word circuit breaker (nltk stem(): length <= 2 is
	// returned unchanged; the paper never mentions it).
	if len(word) <= 2 {
		return word
	}
	w := porterStep1a(word)
	w = porterStep1b(w)
	w = porterStep1c(w)
	w = porterStep2(w)
	w = porterStep3(w)
	w = porterStep4(w)
	w = porterStep5a(w)
	w = porterStep5b(w)
	return w
}

// porterIsConsonant mirrors nltk _is_consonant with the iterative y-chain
// resolution: 'y' at position 0 is a consonant; a 'y' counts as a consonant
// when the character before it is one (y-runs resolved iteratively).
func porterIsConsonant(word string, i int) bool {
	if i < 0 || i >= len(word) {
		return false
	}
	switch c := word[i]; c {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		negate := false
		for i > 0 && word[i] == 'y' {
			negate = !negate
			i--
		}
		if i == 0 {
			// The y-run reaches position 0, where 'y' is a consonant.
			return !negate
		}
		return (strings.IndexByte("aeiou", word[i]) < 0) != negate
	default:
		return true
	}
}

// porterConsonantFlags classifies the whole word once (nltk
// _consonant_flags): true = consonant. 'y' is a consonant at position 0 and
// otherwise the complement of the previous character's classification.
func porterConsonantFlags(word string) []bool {
	flags := make([]bool, len(word))
	for i := range word {
		switch {
		case i == 0:
			flags[i] = porterIsConsonant(word, 0)
		case word[i] == 'y':
			flags[i] = !flags[i-1]
		default:
			flags[i] = porterIsConsonant(word, i)
		}
	}
	return flags
}

// porterMeasure mirrors nltk _measure: the number of VC sequences in the
// word's consonant/vowel shape ("cvccvc" → 2, "cvc" → 1).
func porterMeasure(stem string) int {
	flags := porterConsonantFlags(stem)
	m := 0
	for i := 1; i < len(flags); i++ {
		if flags[i] && !flags[i-1] {
			m++
		}
	}
	return m
}

// porterContainsVowel mirrors nltk _contains_vowel.
func porterContainsVowel(stem string) bool {
	for _, cons := range porterConsonantFlags(stem) {
		if !cons {
			return true
		}
	}
	return false
}

// porterEndsDoubleConsonant mirrors nltk _ends_double_consonant (*d).
func porterEndsDoubleConsonant(word string) bool {
	return len(word) >= 2 && word[len(word)-1] == word[len(word)-2] &&
		porterIsConsonant(word, len(word)-1)
}

// porterEndsCVC mirrors nltk _ends_cvc (*o): consonant-vowel-consonant with
// the final consonant not w/x/y — plus NLTK's 2-letter cv special case.
func porterEndsCVC(word string) bool {
	if len(word) >= 3 &&
		porterIsConsonant(word, len(word)-3) &&
		!porterIsConsonant(word, len(word)-2) &&
		porterIsConsonant(word, len(word)-1) &&
		word[len(word)-1] != 'w' && word[len(word)-1] != 'x' && word[len(word)-1] != 'y' {
		return true
	}
	return len(word) == 2 && !porterIsConsonant(word, 0) && porterIsConsonant(word, 1)
}

// porterRule is one (suffix, replacement, condition) rule; condition nil means
// unconditional. A condition receives the word with the suffix already
// removed. The replacement replaces the SUFFIX (the stem keeps its letters);
// the two special step-1b rules append to the stem instead, marked by the
// replacement markers below.
type porterRule struct {
	suffix      string
	replacement string
	condition   func(stem string) bool
}

// porterAppendE marks the step-1b (m=1 and *o) rule: append "e" to the stem.
const porterAppendE = "\x00e"

// porterApplyRuleList mirrors nltk _apply_rule_list: the FIRST suffix that
// matches wins — even when its condition fails (the walk stops and the word is
// returned unchanged). The one exception is nltk's "*d" rule: when the word
// ends in a doubled consonant the stem is word[:-2] and a FAILED condition
// falls through to the next rule instead of stopping (which is how
// "falling"→"fall" passes the *d branch and reaches the +e branch's guard).
func porterApplyRuleList(word string, rules []porterRule) string {
	for _, r := range rules {
		if r.suffix == "*d" {
			if !porterEndsDoubleConsonant(word) {
				continue
			}
			stem := word[:len(word)-2]
			if r.condition(stem) {
				return stem + word[len(word)-1:]
			}
			continue
		}
		if !strings.HasSuffix(word, r.suffix) {
			continue
		}
		stem := strings.TrimSuffix(word, r.suffix)
		if r.condition != nil && !r.condition(stem) {
			return word
		}
		if r.replacement == porterAppendE {
			return stem + "e"
		}
		return stem + r.replacement
	}
	return word
}

func porterHasPositiveMeasure(stem string) bool { return porterMeasure(stem) > 0 }

func porterStep1a(word string) string {
	// NLTK_EXTENSIONS: a 4-letter "ies" reduces to "ie" (dies→die, ties→tie).
	if strings.HasSuffix(word, "ies") && len(word) == 4 {
		return strings.TrimSuffix(word, "ies") + "ie"
	}
	return porterApplyRuleList(word, []porterRule{
		{suffix: "sses", replacement: "ss"},
		{suffix: "ies", replacement: "i"},
		{suffix: "ss", replacement: "ss"},
		{suffix: "s", replacement: ""},
	})
}

// porterStep1b mirrors nltk _step1b.
func porterStep1b(word string) string {
	// NLTK_EXTENSIONS: "ied" reduces by one letter for a 4-letter word
	// (died→die), by two otherwise (spied→spi) — before the eed/ed/ing walk.
	if strings.HasSuffix(word, "ied") {
		if len(word) == 4 {
			return strings.TrimSuffix(word, "ied") + "ie"
		}
		return strings.TrimSuffix(word, "ied") + "i"
	}
	if strings.HasSuffix(word, "eed") {
		if porterHasPositiveMeasure(strings.TrimSuffix(word, "eed")) {
			return strings.TrimSuffix(word, "eed") + "ee"
		}
		return word
	}
	intermediate := ""
	if strings.HasSuffix(word, "ed") && porterContainsVowel(strings.TrimSuffix(word, "ed")) {
		intermediate = strings.TrimSuffix(word, "ed")
	} else if strings.HasSuffix(word, "ing") && porterContainsVowel(strings.TrimSuffix(word, "ing")) {
		intermediate = strings.TrimSuffix(word, "ing")
	} else {
		return word
	}
	// nltk's _step1b_helper: the four post-removal rules are an if/else CHAIN
	// (first hit wins, and the *d branch is exclusive with the +e branch —
	// "falling"→"fall" passes the *d guard on l/s/z and then fails the +e
	// guard, staying "fall").
	switch {
	case strings.HasSuffix(intermediate, "at"):
		return intermediate + "e"
	case strings.HasSuffix(intermediate, "bl"):
		return intermediate + "e"
	case strings.HasSuffix(intermediate, "iz"):
		return intermediate + "e"
	case porterEndsDoubleConsonant(intermediate) &&
		intermediate[len(intermediate)-1] != 'l' &&
		intermediate[len(intermediate)-1] != 's' &&
		intermediate[len(intermediate)-1] != 'z':
		// *d and not (*L or *S or *Z): collapse the doubled consonant
		// (hopp→hop, tann→tan).
		return intermediate[:len(intermediate)-1]
	case porterMeasure(intermediate) == 1 && porterEndsCVC(intermediate):
		// (m=1 and *o) → + E (fil→file; fail→fail stays).
		return intermediate + "e"
	}
	return intermediate
}

// porterStep1c mirrors nltk _step1c in NLTK_EXTENSIONS mode: y→i when the word
// (minus y) is longer than one letter AND the character before the y is a
// consonant — so happy→happi but enjoy→enjoy, and spy/fly/try merge with their
// -ied/-ies forms (spy→spi, spied→spi).
func porterStep1c(word string) string {
	if !strings.HasSuffix(word, "y") {
		return word
	}
	stem := strings.TrimSuffix(word, "y")
	if len(stem) > 1 && porterIsConsonant(stem, len(stem)-1) {
		return stem + "i"
	}
	return word
}

// porterStep2 mirrors nltk _step2: map double suffixes to single ones when the
// stem has positive measure. NLTK_EXTENSIONS first unwraps a trailing "alli"
// (radicalli→radical→radic) by recursing.
func porterStep2(word string) string {
	if strings.HasSuffix(word, "alli") {
		stem := strings.TrimSuffix(word, "alli")
		if porterHasPositiveMeasure(stem) {
			return porterStep2(stem + "al")
		}
	}
	measure := func(stem string) bool { return porterHasPositiveMeasure(stem) }
	rules := []porterRule{
		{suffix: "ational", replacement: "ate", condition: measure},
		{suffix: "tional", replacement: "tion", condition: measure},
		{suffix: "enci", replacement: "ence", condition: measure},
		{suffix: "anci", replacement: "ance", condition: measure},
		{suffix: "izer", replacement: "ize", condition: measure},
		{suffix: "bli", replacement: "ble", condition: measure},
		{suffix: "alli", replacement: "al", condition: measure},
		{suffix: "entli", replacement: "ent", condition: measure},
		{suffix: "eli", replacement: "e", condition: measure},
		{suffix: "ousli", replacement: "ous", condition: measure},
		{suffix: "ization", replacement: "ize", condition: measure},
		{suffix: "ation", replacement: "ate", condition: measure},
		{suffix: "ator", replacement: "ate", condition: measure},
		{suffix: "alism", replacement: "al", condition: measure},
		{suffix: "iveness", replacement: "ive", condition: measure},
		{suffix: "fulness", replacement: "ful", condition: measure},
		{suffix: "ousness", replacement: "ous", condition: measure},
		{suffix: "aliti", replacement: "al", condition: measure},
		{suffix: "iviti", replacement: "ive", condition: measure},
		{suffix: "biliti", replacement: "ble", condition: measure},
		// NLTK_EXTENSIONS additions: "fulli", and the "logi" quirk whose
		// condition measures the stem INCLUDING its trailing "l" (word minus
		// "log"), so geo/theo endings (archaeologi, philologi) reduce.
		{suffix: "fulli", replacement: "ful", condition: measure},
		{suffix: "logi", replacement: "log", condition: func(stem string) bool {
			return len(stem) >= 1 && porterHasPositiveMeasure(stem[:len(stem)-1])
		}},
	}
	return porterApplyRuleList(word, rules)
}

// porterStep3 mirrors nltk _step3: -ic-, -full, -ness etc. (all measure > 0).
func porterStep3(word string) string {
	measure := func(stem string) bool { return porterHasPositiveMeasure(stem) }
	return porterApplyRuleList(word, []porterRule{
		{suffix: "icate", replacement: "ic", condition: measure},
		{suffix: "ative", replacement: "", condition: measure},
		{suffix: "alize", replacement: "al", condition: measure},
		{suffix: "iciti", replacement: "ic", condition: measure},
		{suffix: "ical", replacement: "ic", condition: measure},
		{suffix: "ful", replacement: "", condition: measure},
		{suffix: "ness", replacement: "", condition: measure},
	})
}

// porterStep4 mirrors nltk _step4: drop -ant/-ence etc. when the stem's
// measure exceeds 1 ("ion" additionally requires an s/t before the suffix).
func porterStep4(word string) string {
	m := func(stem string) bool { return porterMeasure(stem) > 1 }
	rules := []porterRule{
		{suffix: "al", condition: m},
		{suffix: "ance", condition: m},
		{suffix: "ence", condition: m},
		{suffix: "er", condition: m},
		{suffix: "ic", condition: m},
		{suffix: "able", condition: m},
		{suffix: "ible", condition: m},
		{suffix: "ant", condition: m},
		{suffix: "ement", condition: m},
		{suffix: "ment", condition: m},
		{suffix: "ent", condition: m},
		{suffix: "ion", condition: func(stem string) bool {
			return porterMeasure(stem) > 1 && len(stem) > 0 &&
				(stem[len(stem)-1] == 's' || stem[len(stem)-1] == 't')
		}},
		{suffix: "ou", condition: m},
		{suffix: "ism", condition: m},
		{suffix: "ate", condition: m},
		{suffix: "iti", condition: m},
		{suffix: "ous", condition: m},
		{suffix: "ive", condition: m},
		{suffix: "ize", condition: m},
	}
	return porterApplyRuleList(word, rules)
}

// porterStep5a mirrors nltk _step5a — deliberately NOT a rule list (the paper
// lets BOTH conditions apply in turn here, unlike every other step):
//   - (m>1) E →            probate→probat
//   - (m=1 and not *o) E → cease→ceas
func porterStep5a(word string) string {
	if strings.HasSuffix(word, "e") {
		stem := strings.TrimSuffix(word, "e")
		if porterMeasure(stem) > 1 {
			return stem
		}
		if porterMeasure(stem) == 1 && !porterEndsCVC(stem) {
			return stem
		}
	}
	return word
}

// porterStep5b mirrors nltk _step5b: (m>1, measured on word minus its last
// letter) LL → L (controll→control; roll→roll stays).
func porterStep5b(word string) string {
	if strings.HasSuffix(word, "ll") && porterMeasure(word[:len(word)-1]) > 1 {
		return strings.TrimSuffix(word, "l")
	}
	return word
}
