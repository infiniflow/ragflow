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

package runtime

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
	// 200 was the HEAD OF A RANKING sold as a total (measured 2026-09-17, 三国, 1718 chunks:
	// `operand "斩"/"杀": recall hit its bound (200 passage(s))`, both runs). A member stated
	// in a passage ranked past 200 could then never become a window, never be judged, never be
	// named — and the pass-through set came out as "the famous ones", which is why an answer
	// named the right members while citing only a handful of them. The bound is per OPERAND
	// (actor forms + act words, see Coverage.Operands), so this is a corpus-scale ask, not a
	// per-pair one.
	coverageRecallTopN      = 1000
	coverageWindowsPerChunk = 3
	coverageWindowBefore    = 80
	coverageWindowAfter     = 60
	// coverageWindowsMax bounds the lines the point-of-naming node is handed, over BOTH of its
	// sources: this enumeration's windows and the names a probe already reached (the reached
	// ledger's own cap, reachedTermsMax). It is spent against that node's capacity — workers ×
	// clock / a slow call × batch — so the two move together: a cap above the capacity is a
	// ceiling that can never be reached, and the member set then moves with the provider's
	// latency instead of with the corpus.
	//
	// 288 is the capacity the resolve node already documents for itself (coverage_resolve.go:
	// six workers × (CoverageResolveTimeoutS / a five-second call) × batch = 6 × 6 × 8 = 288).
	// At 192 the cap was BELOW that capacity, and every window past it was reported as UNKNOWN
	// (ResolveCoverage: "nobody judged them"), i.e. evidence the enumeration had paid a search
	// for was dropped without ever being read.
	coverageWindowsMax = 288
	// coverageSeedMaxChars is the PROSE budget of the seed, not a cap on what a session can
	// cite: past it the quote is dropped and the chunk id is still written (see Render), because
	// a window omitted outright is a passage no session can name, and a member nothing can name
	// is a member that only exists in the model's own memory.
	coverageSeedMaxChars = 32000
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
	// Source is the line's PROVENANCE, and it is the whole difference between what used to be
	// three pipelines (see CoverageSource): the enumeration's windows, the names a probe reached
	// and the names a session asserted are judged side by side, in one list with one budget —
	// not in three lists with three caps, three dedup rules and three counters.
	Source CoverageSource
}

// CoverageSource says where one ledger line came from. Nothing downstream may branch on it
// except a report: a line is judged, or it is not, and a member named in it is a member.
type CoverageSource string

const (
	// CoverageSourceEnumeration: a window this run's own enumeration built; it carries an Act.
	CoverageSourceEnumeration CoverageSource = "enumeration"
	// CoverageSourceReached: a name a probe asked about and DID reach, with the pool chunk that
	// carries it (see Kbinfos.RecordReachedTerm).
	CoverageSourceReached CoverageSource = "reached"
	// CoverageSourceClaim: a name a session asserted with no passage. The pool is asked what it
	// can show about it, so a claim the run can refute is refuted rather than counted.
	CoverageSourceClaim CoverageSource = "claim"
)

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
		// The recall's SECOND value is its doc aggregations, NOT an error: GrepSearch
		// cannot fail, and DocAggs builds its slice with make() so it is non-nil whether or
		// not anything came back. Read as an error, this branch fires on EVERY operand and
		// discards each recall's passages as though the corpus had answered nothing — the
		// opposite of what this set is for, and a seed that tells every session the
		// direction is empty.
		//
		// A transport failure is therefore indistinguishable from an empty recall HERE;
		// when that distinction is needed, GrepSearch has to grow an error. Until then the
		// recall's result IS the evidence.
		chunks, _ := GrepSearch(ctx, deps, SearchParams{
			Question: operand,
			Keywords: operand,
			TopN:     coverageRecallTopN,
			KbIDs:    deps.KbIDs,
			// What is asked here is the ACTOR and the ACT WORDS, never a member's name,
			// so the reach ledger must not read them as names a probe confirmed.
			SkipReachLedger: true,
		})
		if len(chunks) >= coverageRecallTopN {
			// The recall came back at its whole bound: what the corpus holds for this operand
			// MAY be more than this, and nothing downstream can tell the difference — the member
			// set would be the head of a ranking presented as a total. It is logged as a
			// measurement so that "was the recall complete" is a fact in the run's own record
			// instead of a guess (see CoverageSet.Truncated for the clock-and-cap case).
			_LOG.Printf("[Coverage] operand %q: recall hit its bound (%d passage(s)) — the corpus may hold more", operand, coverageRecallTopN)
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

	// The vocabulary the filter below asks about. The plan's words are a guess about a text
	// nobody had read when they were written (see Coverage.ActsAll): the passages THIS recall just
	// brought back through the actor's own name are read here, once, so that a death stated in a
	// phrasing the plan had no word for — 第一回's "被云长刀起处，挥为两段" — becomes a window
	// instead of being dropped before any node ever sees it.
	acts := cov.ActsAll()
	sampleIDs := make([]string, 0, len(byID))
	for id := range byID {
		sampleIDs = append(sampleIDs, id)
	}
	// Sorted by chunk id: which passages are sampled is what the induced vocabulary is read from,
	// and that vocabulary becomes a filter, so it must not vary with the map's iteration order.
	sort.Strings(sampleIDs)
	var samples []string
	var probe strings.Builder
	probe.WriteString("Passages:\n")
	for _, id := range sampleIDs {
		if len(samples) >= coverageVocabSamples {
			break
		}
		text := strings.Join(strings.Fields(ChunkTextOf(byID[id])), " ")
		if text == "" || (len(actors) > 0 && !coverageMentionsAny(text, actors)) {
			continue
		}
		if r := []rune(text); len(r) > coverageVocabSampleChars {
			text = string(r[:coverageVocabSampleChars])
		}
		samples = append(samples, text)
		fmt.Fprintf(&probe, "[%d] %s\n", len(samples), text)
	}
	if induced := induceActWords(ctx, deps.Model, cov.Acts, probe.String(), samples); len(induced) > 0 {
		for _, w := range induced {
			acts = appendUnique(acts, w)
		}
		_LOG.Printf("[Coverage] vocabulary probe: read %d word(s) out of the corpus that the plan had not declared (%s) — the filter knows them now.", len(induced), strings.Join(induced, " / "))
	}

	// Sorted by chunk id: these windows are rendered into EVERY session's seed (and the
	// cap below keeps the first N), so walking the map made both the seed's wording and
	// which windows survived the cap differ from run to run.
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	// An actor form that matches NOTHING in what the recall just brought back is a suspect
	// DECLARATION, not a fact about the corpus: 2026-09-18 the plan's subject came through as
	// "None", the actor test rejected all 852 passages this recall had returned, and the run lost
	// every citation it could have had (the point-of-naming node was handed no window at all). A
	// test that rejects 100% of the recall has stopped being a filter, so the passages below are
	// judged on the act words alone — the model is the node that decides whether the deed is
	// stated — and the miss is said out loud instead of turning into silence.
	if len(actors) > 0 {
		matched := false
		for _, id := range ids {
			if coverageMentionsAny(strings.Join(strings.Fields(ChunkTextOf(byID[id])), " "), actors) {
				matched = true
				break
			}
		}
		if !matched {
			_LOG.Printf("[Coverage] actor %q matches NONE of the %d recalled passage(s): dropping the actor test (the declaration is suspect; the act words still stand) instead of judging nothing.", cov.Actor, len(ids))
			actors = nil
		}
	}
	for _, id := range ids {
		c := byID[id]
		text := strings.Join(strings.Fields(ChunkTextOf(c)), " ")
		act := coverageFirstAct(text, acts)
		if act == "" {
			continue
		}
		if len(actors) > 0 && !coverageMentionsAny(text, actors) {
			continue
		}
		if kb != nil {
			kb.Admit(func(p *PoolAdmitter) { p.Add(c) })
		}
		for _, w := range coverageWindows(text, acts, coverageWindowsPerChunk) {
			if len(set.Windows) >= coverageWindowsMax {
				set.Truncated = true
				break
			}
			set.Windows = append(set.Windows, CoverageWindow{ChunkID: ChunkIDOf(c), Quote: w.quote, Act: w.act, Source: CoverageSourceEnumeration})
		}
	}
	return set
}

// CoverageActsMeetActor reports whether a passage ALREADY HELD carries both an act word and the
// actor — the conjunction every window the enumeration builds is filtered by (see EnumerateCoverage:
// a recalled passage is kept only when an act word is in it AND, when the direction declares an
// actor, the actor is named in it too).
//
// Read only by the last-resort enrollment (see enrollEnumeration), where the recall it would pay for
// comes out of the ANSWER's own clock: measured (2026-09-17, FRAMES, resolve node) 13 operands asked,
// 724 passages recalled, 0 windows found — the direction's words had never met in what the run held,
// so nothing a recall brought back could survive the filter.
//
// It is a probe, not a verdict: a recall may still return the first passage where they do meet, which
// is exactly what the enrollment is for. It answers only "have we ever seen these words together".
func CoverageActsMeetActor(kb *Kbinfos, cov Coverage) bool {
	if kb == nil {
		return false
	}
	actors := cov.Actors()
	for _, c := range kb.Chunks {
		text := ChunkTextOf(c)
		if coverageFirstAct(text, cov.ActsAll()) == "" {
			continue
		}
		if len(actors) > 0 && !coverageMentionsAny(text, actors) {
			continue
		}
		return true
	}
	return false
}

// Render is the seed text: the enumeration stated as a FACT with what came back.
//
// A list of queries in a prompt is advice, and advice may simply not be taken. A window is
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
	quotesDropped, idsDropped := 0, 0
	for _, w := range s.Windows {
		line := fmt.Sprintf("- chunk_id=%s  %q\n", w.ChunkID, w.Quote)
		if b.Len()+len(line) > coverageSeedMaxChars {
			// Over the prose budget: the passage is still NAMED, only its text is left out.
			// Dropping the line outright would hide a window the enumeration already paid a
			// search for, and a member that no line names is a member the session can only
			// answer from its own memory — the very thing the seed exists to replace.
			line = fmt.Sprintf("- chunk_id=%s\n", w.ChunkID)
			if b.Len()+len(line) > coverageSeedMaxChars {
				idsDropped++
				continue
			}
			quotesDropped++
		}
		b.WriteString(line)
	}
	if quotesDropped > 0 {
		fmt.Fprintf(&b, "(the %d window(s) above list a chunk id without its quote; the text is in the pool under that id)\n", quotesDropped)
	}
	if idsDropped > 0 {
		fmt.Fprintf(&b, "(%d further window(s) are in the pool but are not listed here)\n", idsDropped)
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
