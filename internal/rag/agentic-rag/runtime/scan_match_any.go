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
	"time"
)

// ScanMatchAny is the enumeration's SCAN channel: one call that asks the corpus "which passages carry
// ANY of these terms", and delivers the matching windows.
//
// It exists because every other channel in this runtime is a RANKED search, and a ranked search is the
// wrong instrument for "list them all": the ranking answers "what matches best", while an enumeration
// asks "where does this occur", and the passages past a ranking's head are never looked at. Measured
// 2026-09-20 (三国/关羽): the answer equalled the pool's coverage every time (6 runs: 10/10/10/15/11/13
// members) while the missing members' passages were candidates the ranking cut — 程远志 ranked 9 with a
// per-query cap of 8, twice in a row.
//
// Two properties make it a scan rather than one more ranked leg:
//
//	the MATCH is containment   — a passage is a hit when it literally carries the words of one of the
//	                             probes, so nothing is dropped for being ranked low; the probes are the
//	                             plan's own declaration (DeclaredProbes), never a list this runtime
//	                             invented. A probe is read as the PLANNER wrote it: the words of one
//	                             probe are a CONJUNCTION ("关羽 斩" = a passage carrying an actor form
//	                             AND that act word), and the probes are a DISJUNCTION — so the actor's
//	                             own alternatives ("关羽|关公|云长") become the same question asked in the
//	                             spellings the source uses.
//	the CUT is a budget        — over the delivery budget the windows are truncated, and the truncation
//	                             is REPORTED (Matched/Delivered/Remaining, see Line) instead of looking
//	                             like "the corpus does not carry them".
//
// What it deliberately does NOT do: decide which terms matter, judge whether a hit is a member, or keep
// a ledger of names. The terms come from the plan; whether a passage that mentions a name makes it a
// member is the model's judgement, made on the windows this delivers.
const (
	// scanRecallTopN is how wide the keyword call is allowed to be. It is a RECALL bound on candidates
	// that are only matched and windowed afterwards, not a delivery bound: what the model pays for is
	// bounded by the budget below.
	scanRecallTopN = 600
	// scanOutTotalChars is the delivery budget of ONE scan: the whole point of the channel is that the
	// cut is here and not in a ranking, and it is spent on windows of matching text. 72000 rather than
	// 24000 because the material for "what did X do" is every window naming X — measured 2026-09-21
	// against 三国演义.txt: 266 of its 1718 chunks name 关羽, and a budget below that silently became the
	// thing that decided which members existed (133 delivered, 29 left over, the answer listing 11).
	scanOutTotalChars = 72000
	// scanPerChunkChars bounds ONE delivered window: the sentence around the match plus its neighbour
	// lines (see NarrowByTerms' line-context expansion).
	scanPerChunkChars = 400
	// scanDocOrderMax bounds how many documents the scan names as its reading order.
	scanDocOrderMax = 8
	// scanTermsMax bounds the alternation: each term is a probe the planner declared, and a scan whose
	// alternation is longer than this is a plan, not a probe.
	scanTermsMax = 24
)

// ScanWindow is one delivered match: the passage's id, its document, and the window of text around the
// term that matched.
type ScanWindow struct {
	ChunkID string
	DocID   string
	Text    string
	// Full is the recalled passage the window was cut from. A caller that admits the hit to the pool
	// admits THIS, not the window: the window is what the model is SHOWN now, and the passage behind it
	// is what it can read later (list_chunks), so pooling the window would leave the document
	// unreadable past the match.
	Full map[string]any
}

// ScanResult is what one scan produced, including how much of it was DELIVERED.
type ScanResult struct {
	// Probes are the probes as given (the plan's declaration), and Recalled is how many candidate
	// passages the keyword legs returned in total before matching — the two numbers that tell a blind
	// "no window carries the probes" apart from "the index recalled nothing".
	Probes []string
	// ActWords are the plan's declared deed words, the filter the material had to carry.
	ActWords []string
	// ProbesUnasked is how many of the declared probes the scan's own clock did not fit: they were NOT
	// asked, so what the scan reports is the coverage of the probes it did ask (see scanProbeBudgetS).
	ProbesUnasked int
	Recalled      int
	// ActOnly is true when the strict pass (every word of a probe in ONE passage) matched nothing and
	// the scan fell back to the act words alone: the passage states the deed, while the actor's spelling
	// sits in a neighbouring passage. The windows are labelled rather than dropped, and the judgement
	// stays with the model.
	ActOnly   bool
	Terms     []string
	Windows   []ScanWindow
	Matched   int      // windows that carry one of the terms, in the recalled candidate set
	Delivered int      // of those, the ones this call delivers
	Remaining int      // Matched - Delivered: still in the corpus, not in the answer's hands
	Docs      []string // documents with the most hits first: the reading order (D)
}

// Line renders the scan's coverage as ONE line, for the log and for the tool result.
//
// It is the mechanical half of "you have not seen it all": the numbers are counts about the delivery,
// and deciding whether the remaining windows matter is the model's (see the note on SessionRecord).
func (r ScanResult) Line() string {
	if r.Matched == 0 {
		if r.Recalled == 0 {
			return "[scan] the declared probe(s) recalled no passage at all from the keyword leg: " +
				strings.Join(r.Probes, " / ")
		}
		return fmt.Sprintf("[scan] recalled %s, and none of them carries every word of a probe — probed: %s",
			CountOf(r.Recalled, "passage"), strings.Join(r.Probes, " / "))
	}
	var b strings.Builder
	if r.ActOnly {
		b.WriteString("[scan][act-words only] ")
	} else {
		b.WriteString("[scan] ")
	}
	b.WriteString("probed ")
	b.WriteString(strings.Join(r.Probes, " / "))
	if len(r.ActWords) > 0 {
		b.WriteString(" filtered by the declared act words ")
		b.WriteString(strings.Join(r.ActWords, " / "))
	}
	b.WriteString("; matched ")
	b.WriteString(CountOf(r.Matched, "window"))
	b.WriteString(" carrying the declared probe term(s) in ")
	b.WriteString(CountOf(len(r.Docs), "document"))
	b.WriteString("; delivered ")
	b.WriteString(CountOf(r.Delivered, "window"))
	if r.ProbesUnasked > 0 {
		fmt.Fprintf(&b, " (the scan's %.0fs probe clock cut %s)", scanProbeBudgetS,
			CountOf(r.ProbesUnasked, "probe"))
	}
	if r.Remaining > 0 {
		b.WriteString("; ")
		b.WriteString(CountOf(r.Remaining, "window"))
		b.WriteString(" remaining — the cut is this channel's DELIVERY BUDGET, not a ranking: read on with " +
			"list_chunks on the documents below, or call retrieve again for a narrower pairing")
	}
	return b.String()
}

// DeclaredProbesMax bounds the entity spellings a plan contributes (see DeclaredProbes). It is a bound on
// retrieval calls, not a selection: the plan writes the spellings the question itself uses.
const DeclaredProbesMax = 16

// scanProbeBudgetS is the wall on the probe loop: the probes are issued one after another, and the
// scan's own slice of the opening is what pays for them. When it is spent the scan delivers what the
// probes it managed to ask reached, and SAYS how many it did not ask.
const scanProbeBudgetS = 10.0

// DeclaredProbes are the probes the scan asks with: the ENTITY the plan declared, one probe per spelling
// the source uses for it (Variable.Subjects) — and nothing else.
//
// Nothing is derived here. The plan writes the spellings as a LIST, so the runtime copies a list; it does
// not split a string (which would be guessing the writer's punctuation, and "（字云长）" is not a
// punctuation question at all), and it does not build probes out of guessed act words: which verb a
// source uses for a deed is a fact about the source — it is 挥 in one place, 劈 in another, 刺, 刀起, 砍
// in others — so a probe built from a guess reaches the passages phrased the way the guess predicted and
// misses the rest. Measured 2026-09-21 against 三国演义.txt (1718 chunks): the verb-guessed probes
// reached 162 of the 266 chunks naming the actor, and "how many named people did he kill" came back as
// ELEVEN, FOURTEEN, SEVENTEEN or FOUR members on consecutive runs of the same probe set.
//
// A probe that IS the entity has no such hole: the passages that mention him are exactly the passages
// that mention him, and the reader's question decides which of them count.
func DeclaredProbes(table State) []string {
	var out []string
	for _, v := range table.State {
		for _, sp := range v.Subjects {
			if sp = strings.TrimSpace(sp); sp != "" {
				out = append(out, sp)
			}
		}
	}
	return dedupeTerms(out, DeclaredProbesMax)
}

// DeclaredActWords are the act words the plan declared (Variable.Terms), one entry per word the SOURCE
// uses for the deed. They are NOT probes: the scan asks the corpus for the entity, and these words
// FILTER what the entity's passages are about.
//
// Without them the material is everything that mentions the entity — measured 2026-09-21 (三国/关羽):
// 249 windows matched, the answer carried a 249-line evidence list (33824 characters of which the prose
// was ~2000) and the reader's complaint was that there was far too much. A question is not its entity:
// "which named people did he kill" is about the entity AND the deed, and the deed's words are the
// plan's own declaration, not a guess this code makes.
func DeclaredActWords(table State) []string {
	var out []string
	for _, v := range table.State {
		if len(v.Subjects) == 0 {
			continue
		}
		for _, t := range v.Terms {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		}
	}
	return dedupeTerms(out, scanTermsMax)
}

// ScanMatchAny runs the scan: one keyword recall with the terms, then the containment match, then the
// line-context windows, then the budget.
//
// The keyword call is the DENSE-LEG-FREE one (BM25Search): a scan must not depend on an embedding
// service, and containment is a lexical question. docScope restricts it to the documents in play.
func ScanMatchAny(ctx context.Context, deps SearchDeps, terms []string, acts []string, docScope []string, budgetChars int) ScanResult {
	var res ScanResult
	terms = dedupeTerms(terms, scanTermsMax)
	if len(terms) == 0 || deps.Backend == nil {
		return res
	}
	res.Terms = terms
	if budgetChars <= 0 {
		budgetChars = scanOutTotalChars
	}
	// THE ENGINE OFFERS BM25 AND NOTHING ELSE in this project: a scan therefore widens its recall by
	// ASKING MORE, not by handing the index a filter it does not have. One BM25 call per DECLARED probe
	// (the plan's own words, at most DeclaredProbesMax of them), each on its own, and the union is then
	// matched locally — which is also the only way to keep the rare word from being diluted: measured
	// 2026-09-20, `关羽 劈` ranked 管亥 3rd on its own and 15th as part of a batch, and every call's own
	// head is what the union keeps.
	//
	// Nothing about the grep happens in the engine: the BM25 leg only RECALLS, and the containment
	// match, the windowing and the budget all run here, over what BM25 returned.
	query := strings.Join(terms, " ")
	recalled := make([]map[string]any, 0, len(terms)*8)
	seen := map[string]bool{}
	// The probes are issued one after another on this scan's own clock: measured 2026-09-21 00:52,
	// twenty-three of them took 44s and the opening's share is 45s, so the round that followed had no
	// time to read anything (the answer enumerated five members of the sixteen). What the clock does not
	// fit is NOT asked, and the scan says so rather than reporting a coverage it did not have.
	probeDeadline := time.Now().Add(time.Duration(scanProbeBudgetS * float64(time.Second)))
	asked := 0
	for _, probe := range terms {
		if time.Since(probeDeadline) > 0 {
			break
		}
		asked++
		// Question only: the leg's effective query is Question + Keywords, and passing the probe twice
		// asks it twice ("关羽 斩 关羽 斩"), which is not the probe.
		chunks, _ := BM25Search(ctx, deps, SearchParams{
			Question: probe,
			KbIDs:    deps.KbIDs,
			DocScope: docScope,
			TopN:     scanRecallTopN,
		})
		for _, c := range chunks {
			id := ChunkIDOf(c)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			recalled = append(recalled, c)
		}
	}
	chunks := recalled
	if asked == 0 {
		return res
	}
	res.Probes = append([]string(nil), terms[:asked]...)
	res.ActWords = append([]string(nil), acts...)
	res.ProbesUnasked = len(terms) - asked
	if len(chunks) == 0 {
		return res
	}
	res.Recalled = len(chunks)
	// The MATCH: a passage is a hit only if it carries the words of one of the probes (see the file's
	// note on conjunction/disjunction). NarrowByTerms alone is not this test — it keeps "fact-dense
	// sentences" as a fallback for a chunk with no term in it, which is right for a search result and
	// wrong for a scan's count.
	probes := probeParts(terms)
	parts := flattenProbes(probes)
	// The declaration's act words filter the entity's passages: a window is material for THIS question
	// when it carries the entity AND one of the words the source uses for the deed. The words are the
	// plan's (see DeclaredActWords); nothing here invents, caps or re-orders them, and an empty list
	// means the plan declared none — then the entity's passages are the material, as before.
	actProbes := make([][]string, 0, len(acts))
	for _, a := range acts {
		if a = strings.TrimSpace(a); a != "" {
			actProbes = append(actProbes, []string{a})
			parts = append(parts, a)
		}
	}
	hits := make([]map[string]any, 0, len(chunks))
	for _, c := range chunks {
		text := ChunkTextOf(c)
		if firstProbeHit(text, probes) < 0 {
			continue
		}
		if len(actProbes) > 0 && firstProbeHit(text, actProbes) < 0 {
			continue
		}
		hits = append(hits, c)
	}
	if len(hits) == 0 {
		// The actor's spelling is the fragile half of a probe: the source may write 关公 where the plan
		// wrote 关羽, and then no passage carries "both words". The act words alone still land on the
		// deed, so the scan delivers THOSE windows, labelled (ActOnly) rather than dropped — what the
		// windows mean stays the answer's judgement, and the strict pass is reported either way.
		if acts := actWordsOf(probes); len(acts) > 0 {
			// One word per probe: the act words are ALTERNATIVES (斩 / 劈 / 砍 — the source's own
			// vocabulary for the deed), so the fallback is a disjunction of them, not a conjunction.
			actProbes := make([][]string, 0, len(acts))
			for _, a := range acts {
				actProbes = append(actProbes, []string{a})
			}
			for _, c := range chunks {
				if firstProbeHit(ChunkTextOf(c), actProbes) >= 0 {
					hits = append(hits, c)
				}
			}
			if len(hits) > 0 {
				res.ActOnly = true
				parts = acts
			}
		}
	}
	res.Matched = len(hits)
	if res.Matched == 0 {
		return res
	}
	// The WINDOWS (and the neighbour lines that make a match a sentence rather than a phrase).
	// The windows are centred on the probe's WORDS, not on the probe string: "关羽 斩" is not a phrase a
	// passage carries (the actor and the act are apart in the sentence), so an alternation of the parts
	// is what finds the sentence to show.
	narrowed := NarrowByTerms(hits, parts, nil, query,
		NarrowContext{Before: 1, After: 1}, scanPerChunkChars, budgetChars)
	fullOf := make(map[string]map[string]any, len(hits))
	for _, c := range hits {
		if id := ChunkIDOf(c); id != "" {
			if _, dup := fullOf[id]; !dup {
				fullOf[id] = c
			}
		}
	}
	for _, c := range narrowed.Kept {
		text := ChunkTextOf(c)
		if strings.TrimSpace(text) == "" {
			continue
		}
		id := ChunkIDOf(c)
		res.Windows = append(res.Windows, ScanWindow{
			ChunkID: id,
			DocID:   DocIDOf(c),
			Text:    text,
			Full:    fullOf[id],
		})
	}
	res.Delivered = len(res.Windows)
	if res.Remaining = res.Matched - res.Delivered; res.Remaining < 0 {
		res.Remaining = 0
	}
	res.Docs = docOrderByHits(hits, scanDocOrderMax)
	return res
}

// spaceLess drops the whitespace: a name the corpus spells with spaces (or decoration the probe
// lacks) still matches the probe's own spelling.
func spaceLess(s string) string {
	if !strings.ContainsAny(s, " \t\n\r\v\f") {
		return s
	}
	return strings.Join(strings.Fields(s), "")
}

// actWordsOf is the act words of a probe set: the deed's own vocabulary, which is what is left of
// "关羽 斩" when the ACTOR's spelling is the half that failed.
//
// It reads the convention DeclaredProbes writes — a multi-word probe is "<actor> <act word>", with the
// act word LAST (see DeclaredProbes) — so a probe's act word is its final word. Across the actor's
// alternatives the act words are the part that DIFFERS (关羽 斩 / 关羽 劈), which is exactly why the
// strict pass can fail while the act words still land on the deed.
func actWordsOf(probes [][]string) []string {
	var out []string
	for _, p := range probes {
		if len(p) < 2 {
			continue
		}
		out = append(out, p[len(p)-1])
	}
	return dedupeTerms(out, scanTermsMax)
}

// probeParts splits each probe into the words it is made of: "关羽 斩" -> ["关羽","斩"], "斩" -> ["斩"].
//
// Whitespace is the planner's own spelling of conjunction (see the file's note): it wrote the actor and
// the act word as two words in one probe, which is a question about a passage carrying both.
func probeParts(terms []string) [][]string {
	out := make([][]string, 0, len(terms))
	for _, t := range terms {
		parts := strings.Fields(t)
		if len(parts) == 0 {
			continue
		}
		out = append(out, parts)
	}
	return out
}

// flattenProbes is every part of every probe, in order and deduped: what the windowing searches for.
func flattenProbes(probes [][]string) []string {
	var out []string
	for _, p := range probes {
		out = append(out, p...)
	}
	return dedupeTerms(out, scanTermsMax*4)
}

// firstProbeHit is the index of the first probe whose EVERY word the text carries, or -1.
//
// Containment is case-insensitive and space-insensitive: the corpus's own spelling may carry
// decoration a probe does not ("*车胄*"), and whitespace inside a CJK name is not a difference in
// wording.
func firstProbeHit(text string, probes [][]string) int {
	if text == "" {
		return -1
	}
	flat := spaceLess(strings.ToLower(text))
	for i, parts := range probes {
		all := true
		for _, p := range parts {
			p = spaceLess(strings.ToLower(p))
			if p == "" || !strings.Contains(flat, p) {
				all = false
				break
			}
		}
		if all {
			return i
		}
	}
	return -1
}

// docOrderByHits is the scan's reading order: the documents carrying the most hits first.
func docOrderByHits(chunks []map[string]any, max int) []string {
	counts := map[string]int{}
	for _, c := range chunks {
		if id := DocIDOf(c); id != "" {
			counts[id]++
		}
	}
	docs := make([]string, 0, len(counts))
	for id := range counts {
		docs = append(docs, id)
	}
	sort.Slice(docs, func(i, j int) bool {
		if counts[docs[i]] != counts[docs[j]] {
			return counts[docs[i]] > counts[docs[j]]
		}
		return docs[i] < docs[j]
	})
	if len(docs) > max {
		docs = docs[:max]
	}
	return docs
}

// dedupeTerms trims, drops empties and dedupes case-insensitively, preserving order, bounded by max.
func dedupeTerms(in []string, max int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, t := range in {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		low := strings.ToLower(t)
		if seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, t)
		if len(out) >= max {
			break
		}
	}
	return out
}
