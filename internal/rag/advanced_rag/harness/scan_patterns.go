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
)

// scanTermsMax bounds the act words one direction may declare. Ten because a source words
// one deed many ways (斩 / 杀 / 劈 / 挥为两段 …), and a phrasing no term covers is a passage
// no query ever names.
const scanTermsMax = 10

// The completeness pass's budget. One pattern contributes at most patternWindowsPerAct
// windows, each quoted to patternWindowChars, and the whole block stays under
// patternFindingsMaxChars — the block is EVIDENCE the session reads (each window with the
// chunk id a member cites), so it is worth its characters in a way a list of queries the
// session may or may not run is not (see RunCompletenessPass).
const (
	patternWindowsPerAct    = 3
	patternWindowChars      = 120
	patternFindingsMaxChars = 6000
)

// MemberShaped reports whether the table declares a slot whose answer is a NAME, or a set
// of them (the planner's vocabulary is entity / person / dataset), as opposed to numbers,
// dates and phrases.
//
// It gates the completeness pass, and the difference it makes is measured (2026-09-16,
// FRAMES, mode high). The pass exists to find names nobody thought of, so its store legs
// are only worth their time when names are what the answer is made of. The planner is told
// to declare act words for "a SET or a COUNT of things that someone DID" — and it did, on
// questions whose answer is a NUMBER: `won / victory / champion / title / trophy /
// championship / rings` for "how many times had Brazil won the World Cup" and `writer /
// creator / author` for "the population of the birthplace of a novel's writer". Both were
// enumerated (7–10 store legs each, sessions handed 8 passages a turn from a 100-passage
// reading list) and both were among the run's most expensive questions — 433k tokens for
// the first, 200-320k for the second, together about 18% of the run. No name either pass
// could return changes a count of EVENTS. The declared terms STAY recorded — the record can
// still report what was asked for — but nothing is run for a question no name can change.
func MemberShaped(table State) bool {
	for _, v := range table.State {
		switch strings.ToLower(strings.TrimSpace(v.Type)) {
		case "entity", "person", "dataset", "list", "set":
			return true
		}
	}
	return false
}

// PatternRunner gives one completeness pattern to the corpus and returns what matched.
//
// A one-method interface on purpose: the runtime must be able to run the block
// ScanPatterns renders WITHOUT asking the model, and the only thing it needs from the tool
// layer is one query. The implementation (searchExecutor.RunPattern) goes through the same
// grep path a retrieve call uses, so a pattern gets a pattern's treatment — operands
// recalled individually, the pattern deciding which candidates are windows, the windows
// narrowed to the match.
type PatternRunner interface {
	RunPattern(ctx context.Context, pattern string) []map[string]any
}

// CompletenessPass is what the runtime's pattern run produced.
type CompletenessPass struct {
	// Text is the block the sessions are seeded with: one line per declared pattern, with
	// the windows that came back and the chunk id each one is cited by. Empty when nothing
	// ran (no declaration, no runner, or a table that holds no name — see MemberShaped).
	Text string
	// Asked is how many patterns were given to the corpus, Answered how many came back
	// with at least one window, and Admitted how many windows entered the pool.
	Asked    int
	Answered int
	Admitted int
}

// RunCompletenessPass runs the direction's declared act patterns over the corpus and
// returns the block the sessions are seeded with.
//
// The patterns used to be rendered into the seed as QUERIES TO MAKE, and the measurement is
// why they are now run HERE instead: measured (2026-09-16, 三国/关羽) a round declared ten
// act words with several aliases each — 2175 characters of patterns, in the seed of every
// session — and not one session ran a single one of them. The run's whole query log holds
// zero `.*` queries; the sessions improvised space-separated word lists instead
// (`关羽 斩华雄 温酒`, `关公 砍死 斩 杀`, `云长手起刀落`), which is exactly the shape the
// patterns exist to replace. A list of queries in a prompt is ADVICE, and the completeness
// of an enumeration cannot rest on advice — every run of this question missed 2-4 members
// and the missing names differed every run.
//
// Run here, what comes back is EVIDENCE: the windows are admitted to the shared pool (so
// they are citable, and the record and the answer can point at them) and the seed states
// them as facts, so the session's job becomes "read these windows and write the members
// they name" rather than "think of the queries I should have made".
//
// Gated on MemberShaped: a table of counts and dates declares act words too (see
// MemberShaped), and spending store legs on a question whose answer is a number is a cost
// with no possible payoff.
func RunCompletenessPass(ctx context.Context, runner PatternRunner, kb *Kbinfos, table State) CompletenessPass {
	pass := CompletenessPass{}
	if runner == nil || !MemberShaped(table) {
		return pass
	}
	patterns := ScanPatterns(table)
	if len(patterns) == 0 {
		return pass
	}
	var b strings.Builder
	b.WriteString("## The completeness pass ALREADY RAN for this direction\n\n")
	b.WriteString("Each act pattern below was given to the corpus by the runtime, and the matched " +
		"windows are in the pool under the chunk ids shown. Every pattern WAS asked, so " +
		"`(no window)` is the corpus answering about those WORDS — re-word it (see the " +
		"method's relation step) rather than running it again. A name inside a window is a " +
		"member you can cite with that chunk id: record it as one.\n\n")
	for _, pattern := range patterns {
		if ctx.Err() != nil {
			// The clock ran out mid-pass. The patterns nobody got to ask would render as
			// `(no window)` — a statement about the CORPUS the runtime never earned, and the
			// one reading the block forbids. So the pass yields nothing at all and the seed
			// keeps the pattern list (the fallback, see enumerationSeed); whatever windows
			// did come back are already in the pool, so no evidence is lost.
			return CompletenessPass{}
		}
		pass.Asked++
		chunks := runner.RunPattern(ctx, pattern)
		var lines []string
		for _, c := range chunks {
			if len(lines) >= patternWindowsPerAct {
				break
			}
			id := ChunkIDOf(c)
			quote := truncateRunes(strings.Join(strings.Fields(ChunkTextOf(c)), " "), patternWindowChars)
			if id == "" || quote == "" {
				continue
			}
			if kb != nil {
				kb.Admit(func(p *PoolAdmitter) {
					if p.Add(c) {
						pass.Admitted++
					}
				})
			}
			lines = append(lines, fmt.Sprintf("    chunk_id=%s  \"%s\"", id, quote))
		}
		if len(lines) == 0 {
			fmt.Fprintf(&b, "- %s → (no window)\n", pattern)
			continue
		}
		pass.Answered++
		fmt.Fprintf(&b, "- %s →\n%s\n", pattern, strings.Join(lines, "\n"))
		if b.Len() >= patternFindingsMaxChars {
			b.WriteString("(further patterns omitted; what came back is in the pool)\n")
			break
		}
	}
	if pass.Asked == 0 || ctx.Err() != nil {
		// Same guard at the end: a search cancelled mid-flight returns what it had, so the
		// LAST pattern's emptiness is not the corpus answering (see the loop-head guard).
		return CompletenessPass{}
	}
	pass.Text = b.String()
	return pass
}

// ScanPatterns renders the completeness queries of an enumeration: for each act word a
// direction DECLARED, the actor and the act asked for TOGETHER, as a pattern.
//
// It is a string builder, not a mechanism. The grep path already reads a pattern the way
// this needs — grepPatternOf compiles it, GrepPatternOperands searches each literal operand
// on its own with the wide recall a pattern gets (patternRecallTopN), matchGrepPattern
// decides which candidates are windows, and the windows are narrowed to what carries the
// match (GrepOutCharsPerChunk / GrepOutTotalChars). So completeness comes from asking the
// CORPUS the right question once per act word — which is the one thing a session that only
// knows the names it can REMEMBER cannot do: measured (2026-09-16, 三国/关羽) every run of
// one question missed 2-4 members, the missing names differed every run, and in one run
// 车胄 / 夏侯存 / 程远志 / 管亥 appeared ZERO times in the whole log.
//
// The shape is `actor.*act`, one clause per pair, never `A|B.*T`: `|` binds loosest in Go's
// regexp, so that would read as "A" OR "B.*T" and lose the actor on every clause but the
// last. A subject that is itself an alternation (关羽|关公|云长) is used as declared; an empty
// subject falls back to the act word alone.
//
// Queries are grouped so that one call stays inside the operand budget the keyword leg
// already enforces (GrepTermsMax): each clause contributes its two literal operands.
func ScanPatterns(table State) []string {
	perQuery := max(1, GrepTermsMax/2)
	var out []string
	for _, v := range table.State {
		actors := []string{""}
		if s := strings.TrimSpace(v.Subject); s != "" {
			actors = nil
			for _, a := range strings.Split(s, "|") {
				if a = strings.TrimSpace(a); a != "" {
					actors = append(actors, a)
				}
			}
			if len(actors) == 0 {
				actors = []string{""}
			}
		}
		var clauses []string
		for _, t := range v.Terms {
			t = strings.TrimSpace(t)
			if t == "" {
				continue
			}
			if actors[0] == "" {
				clauses = append(clauses, regexp.QuoteMeta(t))
				continue
			}
			for _, a := range actors {
				clauses = append(clauses, regexp.QuoteMeta(a)+".*"+regexp.QuoteMeta(t))
			}
		}
		for i := 0; i < len(clauses); i += perQuery {
			out = append(out, strings.Join(clauses[i:min(i+perQuery, len(clauses))], "|"))
		}
	}
	return out
}
