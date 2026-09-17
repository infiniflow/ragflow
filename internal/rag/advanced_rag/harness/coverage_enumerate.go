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
	"sort"
	"strings"
	"unicode/utf8"
)

// The enumeration's bounds. They are spent in ONE place each: coverageRecallTopN is what
// one OPERAND may recall (per-operand, not per-pair), the window bounds decide how much
// of a passage a verdict is asked about, and the two caps keep the seed and the call list
// finite.
const (
	coverageRecallTopN      = 200
	coverageWindowsPerChunk = 3
	coverageWindowBefore    = 80
	coverageWindowAfter     = 60
	coverageWindowsMax      = 160
	coverageSeedMaxChars    = 8000
)

// CoverageWindow is one place the corpus states the deed: the chunk it is in, the words
// around the act, and which act word was found.
//
// The window is what a verdict is asked about (see ResolveCoverage), and its chunk id is
// where a member's citation comes from — the model names a member, the runtime names the
// passage, so a fabricated citation cannot reach the record.
type CoverageWindow struct {
	ChunkID string
	Quote   string
	Act     string
}

// CoverageSet is what one enumeration of the corpus produced: the operands it asked
// about, the windows the deed was stated in, and how many distinct passages came back.
//
// It is EVIDENCE, not advice: the windows are admitted to the shared pool (so a session
// can read them and the answer can cite them) and the seed states them as facts, which is
// what makes the session's job "read these windows" rather than "think of the queries I
// should have made".
type CoverageSet struct {
	Operands []string
	Windows  []CoverageWindow
	Recalled int
	// Truncated reports that the clock ran out mid-enumeration: what came back is real,
	// what did not come back is unknown rather than absent.
	Truncated bool
}

// Empty reports whether the enumeration found nothing worth a verdict.
func (s CoverageSet) Empty() bool { return len(s.Windows) == 0 }

// EnumerateCoverage asks the corpus about the actor and the act words and returns the
// windows where the deed is stated.
//
// ONE recall per OPERAND (the union of the actor's forms and the act words), then a TEXT
// test — a passage has to carry a declared act word, and one of the actor's forms when
// the actor was declared — which is the same set the per-pair patterns used to match,
// reached with a fraction of the searches (see Coverage.Operands).
//
// The evidence is admitted to the pool as it is found, so the enumeration is the whole of
// "检索充分": every member a verdict can name has its passage in hand before the question
// is asked, and no round has to re-enumerate to find it again.
func EnumerateCoverage(ctx context.Context, deps SearchDeps, cov Coverage, kb *Kbinfos) CoverageSet {
	set := CoverageSet{Operands: cov.Operands()}
	if len(set.Operands) == 0 {
		return set
	}
	actors := cov.Actors()
	byID := map[string]map[string]any{}
	for _, operand := range set.Operands {
		if ctx.Err() != nil {
			// Out of clock: what was recalled is real evidence, what was not recalled is
			// UNKNOWN, and the caller is told which (see CoverageSet.Truncated).
			set.Truncated = true
			break
		}
		chunks, err := GrepSearch(ctx, deps, SearchParams{
			Question: operand,
			Keywords: operand,
			TopN:     coverageRecallTopN,
			KbIDs:    deps.KbIDs,
			// What is asked here is the ACTOR and the ACT WORDS, never a member's name,
			// so the reach ledger must not read them as names a probe confirmed.
			SkipReachLedger: true,
		})
		if err != nil {
			// A failed recall is NOT an empty corpus: what came back is real, what did
			// not come back is UNKNOWN — the distinction this set's whole contract rests
			// on (see CoverageSet.Truncated). Dropping the error read a transport
			// failure as "the corpus does not state it".
			_LOG.Printf("[Coverage] operand %q: recall failed; the enumeration is partial: %v", operand, err)
			set.Truncated = true
			continue
		}
		for _, c := range chunks {
			if id := ChunkIDOf(c); id != "" {
				if _, seen := byID[id]; !seen {
					byID[id] = c
				}
			}
		}
	}
	set.Recalled = len(byID)

	// Sorted by chunk id: these windows are rendered into EVERY session's seed (and the
	// cap below keeps the first N), so walking the map made both the seed's wording and
	// which windows survived the cap differ from run to run.
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		c := byID[id]
		text := strings.Join(strings.Fields(ChunkTextOf(c)), " ")
		act := coverageFirstAct(text, cov.Acts)
		if act == "" {
			continue
		}
		if len(actors) > 0 && !coverageMentionsAny(text, actors) {
			continue
		}
		if kb != nil {
			kb.Admit(func(p *PoolAdmitter) { p.Add(c) })
		}
		for _, w := range coverageWindows(text, cov.Acts, coverageWindowsPerChunk) {
			if len(set.Windows) >= coverageWindowsMax {
				set.Truncated = true
				break
			}
			set.Windows = append(set.Windows, CoverageWindow{ChunkID: ChunkIDOf(c), Quote: w.quote, Act: w.act})
		}
	}
	return set
}

// Render is the seed text: the enumeration stated as a FACT with what came back.
//
// A list of queries in a prompt is advice — measured (2026-09-16, 三国/关羽) a round
// rendered ten act words with several aliases each into the seed of every session (2175
// characters of patterns) and not one session ran a single one of them. A window is
// evidence: it carries the chunk id a verdict cites, so the session reads rather than
// guesses, and `(nothing)` is the corpus answering about these WORDS rather than nobody
// having looked.
func (s CoverageSet) Render() string {
	var b strings.Builder
	b.WriteString("## The enumeration already ran for this direction\n\n")
	fmt.Fprintf(&b, "The corpus was asked about each operand below (%d operands, %d distinct passage(s)) — one search per operand. "+
		"Every window is already in the pool under the chunk id shown, and a member named in a window is a member you can cite with that id.\n\n",
		len(s.Operands), s.Recalled)
	if len(s.Operands) > 0 {
		// The operands are NAMED, not promised: the header said "below" while only the
		// windows followed it, so a session could not tell an operand nobody searched
		// from one the corpus came back empty on.
		b.WriteString("- operands: " + strings.Join(s.Operands, " / ") + "\n\n")
	}
	if s.Empty() {
		b.WriteString("(nothing)\n")
		return b.String()
	}
	for _, w := range s.Windows {
		line := fmt.Sprintf("- chunk_id=%s  %q\n", w.ChunkID, w.Quote)
		if b.Len()+len(line) > coverageSeedMaxChars {
			b.WriteString("(further windows omitted; all of them are in the pool)\n")
			break
		}
		b.WriteString(line)
	}
	if s.Truncated {
		b.WriteString("(the enumeration ran out of clock; the corpus holds more than is shown here)\n")
	}
	return b.String()
}

// coverageWindows returns up to n act-centred windows of one passage, in order.
func coverageWindows(text string, acts []string, n int) []struct{ quote, act string } {
	var out []struct{ quote, act string }
	if n <= 0 {
		return out
	}
	runes := []rune(text)
	from := 0
	for from < len(runes) && len(out) < n {
		idx, length, act := coverageNextAct(runes, from, acts)
		if idx < 0 {
			break
		}
		lo := idx - coverageWindowBefore
		if lo < 0 {
			lo = 0
		}
		hi := idx + length + coverageWindowAfter
		if hi > len(runes) {
			hi = len(runes)
		}
		out = append(out, struct{ quote, act string }{quote: strings.TrimSpace(string(runes[lo:hi])), act: act})
		from = idx + length
	}
	return out
}

// coverageNextAct finds the earliest declared act word at or after `from`, returning the
// rune index, its length and the word itself.
func coverageNextAct(runes []rune, from int, acts []string) (int, int, string) {
	bestIdx, bestLen, bestAct := -1, 0, ""
	for _, act := range acts {
		needle := []rune(strings.TrimSpace(act))
		if len(needle) == 0 {
			continue
		}
		if idx := runeIndexFrom(runes, needle, from); idx >= 0 && (bestIdx < 0 || idx < bestIdx) {
			bestIdx, bestLen, bestAct = idx, len(needle), act
		}
	}
	return bestIdx, bestLen, bestAct
}

// coverageFirstAct answers whether a passage states the deed at all.
func coverageFirstAct(text string, acts []string) string {
	if _, _, act := coverageNextAct([]rune(text), 0, acts); act != "" {
		return act
	}
	return ""
}

// coverageMentionsAny reports whether the passage carries one of the actor's forms.
func coverageMentionsAny(text string, forms []string) bool {
	for _, f := range forms {
		if strings.Contains(text, f) {
			return true
		}
	}
	return false
}

// runeIndexFrom is strings.Index over runes, starting at a rune index: the window
// arithmetic above is in runes, so a byte offset would not do, and the stdlib search
// beats a hand-written scan.
func runeIndexFrom(haystack, needle []rune, from int) int {
	if from < 0 {
		from = 0
	}
	if len(needle) == 0 || from >= len(haystack) {
		return -1
	}
	tail := string(haystack[from:])
	at := strings.Index(tail, string(needle))
	if at < 0 {
		return -1
	}
	return from + utf8.RuneCountInString(tail[:at])
}
