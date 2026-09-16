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

package advanced_rag

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/advanced_rag/slots"
)

// Member roll call: the enumeration's WRITE-BACK, done by the runtime.
//
// Measured (2026-09-16, 三国/关羽): the evidence was never the limit. The corpus holds nineteen named
// kills, the pool held 244 passages, and runs of ONE build answered 18 / 16 / 15 / 14 / 12 members —
// eleven of them the same every single time, the rest a different handful of the other nine. What
// varies is what a session WRITES: each session patched 12-14 names, and the round's union was
// whichever list happened to be longest. Two failures were measured on the way: a session probed a
// name with the verb glued to it (`斩夏侯存`) — the corpus does not word it that way, the probe came
// back empty, and the session read that as "the corpus does not carry this member"; and
// action_set.md's own note says sessions spend every turn searching and patch last, when the budget
// is already gone.
//
// So the write-back stops being a session's memory. The runtime takes the candidates it ALREADY
// holds — the completeness pass's windows (each with the chunk id it came from) and the ledger of
// terms a probe reached (Kbinfos.ReachedTerms) — asks the model about EVERY one of them in ONE call,
// and writes the members itself. The judgement stays the model's: nothing here decides what a kill
// is, or which words mean one. What the model can no longer do is forget to write a member down, or
// skip a candidate in silence — and a verdict names a LINE, so the chunk id always comes from the
// line the runtime handed it, never from the reply.

// MemberRollCallPrompt is the whole instruction: one verdict per line, both lists.
//
// It is written around the QUESTION rather than around any one kind of answer: the same step runs on
// "which awards did X win" and on "who did X kill", so what a line has to do is state a member of
// the set the question asks for — the question itself is in the user message. Nothing here decides
// what counts as a member; that judgement is the model's, and it is the same judgement it makes
// inside a session. What changes is only WHO WRITES IT DOWN.
const MemberRollCallPrompt = "You are given numbered evidence lines from one fixed corpus, each with the chunk id it came from.\n" +
	"For EVERY line decide one thing: does this line itself state a member of the set the question " +
	"asks for — and if it does, what is that member called?\n" +
	"Judge only what the line says, never what you know from elsewhere. A mention that is not a member " +
	"(a plan, a promise, a denial, a pursuit, something that did not happen, somebody else's action, or " +
	"a member the line does not name) is NOT one.\n" +
	"Answer with JSON only, and put EVERY line number in exactly one of the two lists:\n" +
	`{"members": [{"i": <line number>, "name": "<the member the line states>"}], "not_members": [<line numbers>]}` + "\n" +
	"Use the line's own words for the name. Never invent a name, and never answer for a line you were not given."

// rollCallCandidate is one line put in front of the model.
type rollCallCandidate struct {
	ChunkID string
	Quote   string
}

// RollCallResult is what one write-back produced, for the run log: how many candidates were asked
// about, how many the model took a position on, and how many members the runtime actually wrote.
type RollCallResult struct {
	Asked    int
	Answered int
	Written  int
}

// The bounds are the ones the retrieval layer already enforces, restated where they are spent:
// Kbinfos caps the pool and the completeness pass caps its block, so a candidate list built from
// both arrives bounded; rollCallMaxCands keeps ONE prompt to a sane size, and rollCallQuoteChars
// keeps one line to the sentence that names the victim.
const (
	rollCallQuoteChars = 300
	rollCallMaxCands   = 150
)

// RollCallMembers asks the model about every candidate the run already holds and writes the members
// it names. It returns nothing and writes nothing when there is no candidate the table does not
// already mention — an empty roll call costs no call at all.
func RollCallMembers(ctx context.Context, deps RAGTools, st *AgenticState, logger *log.Logger) RollCallResult {
	if logger == nil {
		logger = _LOG
	}
	res := RollCallResult{}
	if st == nil || deps.Model == nil || st.KB == nil || len(st.SlotTable.State) == 0 {
		return res
	}
	// Same gate as the completeness pass, for the same reason (see harness.MemberShaped): a table of
	// counts and dates has no members to roll-call, so a question whose answer is a number must not
	// pay for one.
	if !harness.MemberShaped(st.SlotTable) {
		return res
	}
	// And the stronger gate, which is the one that keeps the step off every question that is not a
	// set: the members go into a slot that ALREADY holds members, so a table whose slots hold values
	// (one name, one date, one number, one phrase) is not enumerated at all and this step spends
	// nothing — not even the call (see rollCallTargetSlot).
	slotID := rollCallTargetSlot(&st.SlotTable)
	if slotID < 0 {
		return res
	}
	cands := rollCallCandidates(st)
	if len(cands) == 0 {
		return res
	}
	res.Asked = len(cands)

	// Bounded like every other single-purpose call in this graph (see RewriteTimeoutS): the answer
	// composition still has to fit inside the request's clock, so this step may not spend it.
	t := min(RollCallTimeoutS, max(10.0, st.RemainingS()-10.0))
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(t*float64(time.Second)))
	defer cancel()
	reply, err := deps.Model.Complete(callCtx, []schema.Message{
		*schema.SystemMessage(MemberRollCallPrompt),
		*schema.UserMessage(rollCallQuestion(st, cands)),
	}, nil)
	if err != nil {
		// A failed call costs the members it would have written, not the answer: the table still
		// holds everything the sessions wrote.
		logger.Printf("[RollCall] %d candidate(s) could not be checked: %v", res.Asked, err)
		return res
	}
	members, answered := parseRollCallVerdicts(reply.Content, cands)
	res.Answered = answered
	if answered < res.Asked {
		// Said out loud because it is the one number that says whether "every line gets a verdict"
		// held: an unjudged line is a candidate nobody looked at, which is the failure this whole
		// step exists to remove.
		logger.Printf("[RollCall] %d of %d candidate(s) got no verdict", res.Asked-answered, res.Asked)
	}
	if len(members) == 0 {
		logger.Printf("[RollCall] %d candidate(s) asked, %d answered, 0 member(s) named", res.Asked, res.Answered)
		return res
	}

	before := len(memberUnion(&st.SlotTable))
	value := slots.Members(members...)
	rendered := slots.Render(value)
	// 1.0, and the number is a statement about the EVIDENCE rather than a claim: every member here
	// was just matched to the line that names it, so the list is accountable in a way a session's
	// prose list is not (see slots.Union for why a member list outranks prose).
	strength := 1.0
	branch := harness.NewState([]harness.Variable{{
		ID:                slotID,
		Value:             &value,
		Candidate:         &rendered,
		CandidateStrength: &strength,
	}}, st.SlotTable.Depth+1, nil)
	if merged := MergeSlotPatch(st.SlotTable, branch); merged != nil {
		st.SlotTable = *merged
	}
	res.Written = len(memberUnion(&st.SlotTable)) - before
	if raised := reconcileCountSlots(&st.SlotTable); len(raised) > 0 {
		logger.Printf("[RollCall] count slot(s) %v set to the enumerated members' size", raised)
	}
	// The answer reads the record, and the record was rendered BEFORE this write-back: refresh it,
	// or the members the roll call just proved would be invisible to the answer that must list them.
	st.KB.Record = RenderSlotRecord(st.SlotTable, st.CollectedAnswer)
	logger.Printf("[RollCall] %d candidate(s) asked, %d answered, %d member(s) written into slot %d",
		res.Asked, res.Answered, res.Written, slotID)
	return res
}

// rollCallCandidates collects what the run already holds and is not in a slot yet: the completeness
// pass's windows, and the terms a probe REACHED (a name whose passage came back — a member with its
// evidence, whatever the session did with it afterwards).
func rollCallCandidates(st *AgenticState) []rollCallCandidate {
	known := map[string]bool{}
	for _, name := range memberUnion(&st.SlotTable) {
		known[strings.ToLower(strings.TrimSpace(name))] = true
	}
	seen := map[string]bool{}
	out := make([]rollCallCandidate, 0, 32)
	add := func(id, quote string) {
		id, quote = strings.TrimSpace(id), strings.TrimSpace(quote)
		if id == "" || quote == "" || seen[id+"\x00"+quote] {
			return
		}
		seen[id+"\x00"+quote] = true
		out = append(out, rollCallCandidate{ChunkID: id, Quote: truncateRunes(quote, rollCallQuoteChars)})
	}
	if block, ok := st.KB.PatternFindings(); ok {
		for _, w := range parseBlockWindows(block) {
			add(w[0], w[1])
		}
	}
	for _, rt := range st.KB.ReachedTerms() {
		name := strings.TrimSpace(rt.Term)
		if name == "" || rt.ChunkID == "" || known[strings.ToLower(name)] {
			continue
		}
		quote := name
		if c := st.KB.ChunkByID(rt.ChunkID); c != nil {
			if text := strings.Join(strings.Fields(harness.ChunkTextOf(c)), " "); text != "" {
				quote = text
			}
		}
		add(rt.ChunkID, quote)
	}
	if len(out) > rollCallMaxCands {
		out = out[:rollCallMaxCands]
	}
	return out
}

// parseBlockWindows reads the windows out of a completeness-pass block, whose lines render as
// `    chunk_id=<id>  "<quote>"`.
func parseBlockWindows(block string) [][2]string {
	var out [][2]string
	for _, line := range strings.Split(block, "\n") {
		i := strings.Index(line, "chunk_id=")
		if i < 0 {
			continue
		}
		rest := line[i+len("chunk_id="):]
		end := strings.IndexAny(rest, " \t\"")
		if end <= 0 {
			continue
		}
		q1 := strings.Index(rest, "\"")
		q2 := strings.LastIndex(rest, "\"")
		if q1 < 0 || q2 <= q1 {
			continue
		}
		out = append(out, [2]string{rest[:end], rest[q1+1 : q2]})
	}
	return out
}

// rollCallQuestion renders the call's user message: the question, the actor's declared forms, and
// every candidate on its own numbered line.
func rollCallQuestion(st *AgenticState, cands []rollCallCandidate) string {
	actor := ""
	for _, v := range st.SlotTable.State {
		if s := strings.TrimSpace(v.Subject); s != "" {
			actor = s
			break
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n", st.Question)
	if actor != "" {
		fmt.Fprintf(&b, "Actor (the forms this direction named): %s\n", actor)
	}
	b.WriteString("Evidence lines:\n")
	for i, c := range cands {
		fmt.Fprintf(&b, "[%d] chunk_id=%s \"%s\"\n", i, c.ChunkID, c.Quote)
	}
	b.WriteString("\nAnswer with the JSON object and nothing else.")
	return b.String()
}

// parseRollCallVerdicts turns a reply into members, taking each member's chunk id from the LINE the
// model judged: a verdict only has to get the line number right, so a fabricated citation cannot
// reach the record. answered counts the lines a position was taken on, in either list.
func parseRollCallVerdicts(content string, cands []rollCallCandidate) ([]slots.Member, int) {
	obj, _ := extractJSONObject(content).(map[string]any)
	if obj == nil {
		return nil, 0
	}
	answered := map[int]bool{}
	seenName := map[string]bool{}
	var members []slots.Member
	for _, raw := range rollCallItems(obj["members"]) {
		entry, _ := raw.(map[string]any)
		if entry == nil {
			continue
		}
		i, ok := rollCallInt(entry["i"])
		name := strings.TrimSpace(anyString(entry["name"]))
		if !ok || name == "" || i < 0 || i >= len(cands) {
			continue
		}
		answered[i] = true
		if key := strings.ToLower(name); !seenName[key] {
			seenName[key] = true
			members = append(members, slots.Member{Name: name, ChunkID: cands[i].ChunkID, Quote: cands[i].Quote})
		}
	}
	for _, raw := range rollCallItems(obj["not_members"]) {
		if i, ok := rollCallInt(raw); ok && i >= 0 && i < len(cands) {
			answered[i] = true
		}
	}
	return members, len(answered)
}

// rollCallTargetSlot is the slot the members go into: the member slot that ALREADY holds the most
// members — the enumeration's own list slot, the one the record and the answer read. -1 when the
// table holds no members at all.
//
// The restraint is deliberate, and it is what keeps this step off every other kind of question: a
// table whose slots hold VALUES (a name, a date, a number, a phrase) is not enumerating anything, so
// there is nothing to add to and nothing is written — no prose is replaced, no single answer is
// turned into a list. Whether a table IS enumerating is therefore the model's own earlier decision
// (it wrote members), not a reading of a slot's type here.
func rollCallTargetSlot(table *harness.State) int {
	if table == nil {
		return -1
	}
	best, bestN := -1, -1
	for _, v := range table.State {
		if v.Typed().Kind != slots.KindMembers {
			continue
		}
		if n := len(v.Typed().Names()); n > bestN {
			best, bestN = v.ID, n
		}
	}
	return best
}

// rollCallItems normalises a JSON list, whichever shape the decoder produced for it.
func rollCallItems(v any) []any {
	switch t := v.(type) {
	case []any:
		return t
	case []map[string]any:
		out := make([]any, 0, len(t))
		for _, m := range t {
			out = append(out, m)
		}
		return out
	}
	return nil
}

// rollCallInt reads a line number, which a model may hand back as a number or as a string.
func rollCallInt(v any) (int, bool) {
	switch t := v.(type) {
	case float64:
		return int(t), t == float64(int(t))
	case int:
		return t, true
	case string:
		var n int
		if _, err := fmt.Sscanf(strings.TrimSpace(t), "%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}
