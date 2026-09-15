package harness

import (
	"fmt"
	"strings"
	"unicode"
)

// SessionRecord is what a session has DONE so far, expressed as facts the ReAct
// loop cannot recover from its own conversation.
//
// The session's message list holds the model's queries and the passages that came
// back, and nothing else: which names were already probed (and which of those came
// back empty), which members the record now holds, and which confirmed members are
// still undecided — a name whose passage is in the pool and which no slot
// mentions. Every one of those facts is computed by the runtime anyway, and
// withholding them left the model driving its queries blind: it re-asked names it
// had already resolved, and it could not see that a name it found was still
// unrecorded.
//
// Measured (fixrecall2, 2026-09-15, mode=high): a session probed 15+ terms across
// 8 turns and recorded none of them until it was forced to, with 庞德 queried,
// found, and then dropped by the model's own reasoning — a decision the runtime
// could not see and therefore never asked about.
//
// The record is also the basis of the continuation decision: the offer the model
// answers (offerContinuation) carries it, so "is anything still missing?" is a
// question about facts the model can read, not about its appetite for searching.
type SessionRecord struct {
	// Pool is the shared evidence pool's size at this turn.
	Pool int
	// Members are the names the slot table currently records (split out of the
	// candidates, so "孔秀、孟坦" counts as two).
	Members []string
	// Reached are the named terms a probe reached, with evidence (Kbinfos.Reached).
	Reached []string
	// Absent are the named terms probes looked for and did not find
	// (Kbinfos.ProbedAbsent): "asked and nothing came back" is a result, and it
	// is the one that tells the model to change the spelling or the angle.
	Absent []string
	// Undecided are confirmed members (Reached) that the slot table does not
	// mention at all: the evidence exists, the decision does not.
	Undecided []string
}

// CollectSessionRecord gathers the record from the live pool plus the session's
// slot table. A nil pool yields the table half only.
//
// Everything here is derived from facts the runtime already holds — the slot
// table and the ledger of terms the session itself asked about — and needs no
// knowledge of the corpus's language, subject or relation.
func CollectSessionRecord(table State, kb *Kbinfos) SessionRecord {
	var r SessionRecord
	if kb != nil {
		r.Pool = kb.PoolSize()
		for _, rt := range kb.ReachedTerms() {
			r.Reached = append(r.Reached, rt.Term)
		}
		r.Absent = kb.ProbedAbsentTerms()
	}
	// Members come from the candidates, split on the separators a list answer
	// uses, so a multi-name candidate reads as its members.
	seen := map[string]bool{}
	for _, v := range table.State {
		if v.Candidate == nil {
			continue
		}
		for _, name := range SplitCandidateNames(*v.Candidate) {
			// A count is not a member. The slot that holds the answer to "how
			// many" is a number, and counting it here would both inflate the
			// members and put a digit in the list the model reads.
			if isCountValue(name) {
				continue
			}
			key := strings.ToLower(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			r.Members = append(r.Members, name)
		}
	}
	joined := strings.ToLower(strings.Join(r.Members, "\x00"))
	for _, term := range r.Reached {
		// Substring containment, not equality: a candidate is often a clause
		// ("庞德被周仓生擒") rather than a bare name, and the question is only
		// whether the record has taken a position on this term at all.
		if !strings.Contains(joined, strings.ToLower(term)) {
			r.Undecided = append(r.Undecided, term)
		}
	}
	return r
}

// Line renders the record as ONE line for the tool result the model reads.
//
// It is deliberately a single line of counts plus a few names, not a block: it is
// appended to every tool result, so its cost is paid once per turn, and it must
// stay small enough that it never crowds out the passages it annotates.
func (r SessionRecord) Line() string {
	var b strings.Builder
	b.WriteString("[record] ")
	fmt.Fprintf(&b, "members=%d", len(r.Members))
	if len(r.Members) > 0 {
		fmt.Fprintf(&b, " (%s)", shortList(r.Members, 6))
	}
	fmt.Fprintf(&b, " | probed-reached=%d", len(r.Reached))
	if len(r.Absent) > 0 {
		fmt.Fprintf(&b, " asked-nothing-back=%d (%s)", len(r.Absent), shortList(r.Absent, 4))
	}
	if len(r.Undecided) > 0 {
		fmt.Fprintf(&b, " | FOUND BUT NOT RECORDED=%s", shortList(r.Undecided, 4))
	}
	fmt.Fprintf(&b, " | pool=%d", r.Pool)
	return b.String()
}

// shortList joins up to max items on the list separator and marks the remainder.
func shortList(items []string, max int) string {
	if len(items) > max {
		return strings.Join(items[:max], "、") + fmt.Sprintf("…+%d", len(items)-max)
	}
	return strings.Join(items, "、")
}

// sessionRecordNow computes the session's record as of this turn.
func (s *SessionState) sessionRecordNow() SessionRecord {
	return CollectSessionRecord(s.workingTable(), s.KB)
}

// workingTable is the slot table as THIS session has patched it: the parent table
// with the session's own branch patches applied in order. Reading the parent table
// alone would report the record as it stood when the session started, so a member
// the session itself just found would still read as missing.
func (s *SessionState) workingTable() State {
	t := State{
		State:                append([]Variable(nil), s.ParentState.State...),
		Depth:                s.ParentState.Depth,
		ID:                   s.ParentState.ID,
		RetrievedEvidenceIDs: s.ParentState.RetrievedEvidenceIDs,
	}
	for _, branch := range s.NewStates {
		for _, patch := range branch.State {
			for i := range t.State {
				if t.State[i].ID != patch.ID || patch.Candidate == nil || *patch.Candidate == "" {
					continue
				}
				t.State[i].Candidate = patch.Candidate
				t.State[i].CandidateStrength = patch.CandidateStrength
			}
		}
	}
	return t
}

// Brief is the record's counts, for logs.
func (r SessionRecord) Brief() string {
	return fmt.Sprintf("members=%d reached=%d absent=%d undecided=%d",
		len(r.Members), len(r.Reached), len(r.Absent), len(r.Undecided))
}

// maxListedMemberRunes is how long a comma-separated piece may be and still read
// as a list item rather than a clause (see SplitCandidateNames).
const maxListedMemberRunes = 6

// SplitCandidateNames splits one slot candidate into the names it lists.
//
// Two classes of separator, because a candidate may be a LIST or a SENTENCE and
// the two must not be confused (a member count inflated by prose is worse than a
// missing one — it is the number the model steers by):
//
//   - strong separators (、；;/| and whitespace) always split;
//   - a comma splits only when EVERY comma-separated piece is short. "华雄,车胄"
//     is a list; "庞德被周仓生擒，非关羽所杀" is the model explaining a decision,
//     and cutting it would enter half a sentence as a member.
//
// A candidate with no separator at all is returned whole: guessing further would
// invent members.
func SplitCandidateNames(candidate string) []string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return nil
	}
	var out []string
	for _, field := range strings.FieldsFunc(candidate, isStrongListSeparator) {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		out = append(out, splitIfShortList(field)...)
	}
	if len(out) == 0 {
		return []string{candidate}
	}
	return out
}

// splitIfShortList splits one strong-separated piece on commas when the piece
// reads as a list (every part short), and otherwise keeps it whole.
func splitIfShortList(field string) []string {
	if !strings.ContainsAny(field, ",，") {
		if m := trimMemberSuffix(field); m != "" {
			return []string{m}
		}
		return nil
	}
	parts := strings.FieldsFunc(field, func(r rune) bool { return r == ',' || r == '，' })
	for _, p := range parts {
		if len([]rune(strings.TrimSpace(p))) > maxListedMemberRunes {
			if m := trimMemberSuffix(field); m != "" {
				return []string{m}
			}
			return nil
		}
	}
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if m := trimMemberSuffix(strings.TrimSpace(p)); m != "" {
			out = append(out, m)
		}
	}
	return out
}

func isStrongListSeparator(r rune) bool {
	switch r {
	case '、', ';', '；', '/', '／', '|', '\n', '\t', ' ':
		return true
	}
	return false
}

// trimMemberSuffix drops the trailing "etc." marker a list answer ends with
// (「… 等」), which is not part of any member. A piece that is ONLY the marker
// becomes empty — there is no member there — and the caller drops it.
func trimMemberSuffix(s string) string {
	s = strings.TrimSpace(s)
	for _, suffix := range []string{"等等", "等"} {
		if s == suffix {
			return ""
		}
		if strings.HasSuffix(s, suffix) {
			return strings.TrimSpace(strings.TrimSuffix(s, suffix))
		}
	}
	return s
}

// isCountValue reports whether a candidate is a quantity rather than a name.
//
// It reads DIGITS, not language: the slot that answers "how many" holds a number
// (13, 13人, 13个), and a number is not a member of the list it counts.
// Non-numeric quantity words (十三) stay in, because recognising those needs the
// corpus's language — which the framework does not have, and must not pretend to.
func isCountValue(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	digits := 0
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			digits++
		case r == '.' || r == ',' || r == '%' || r == '个' || r == '人' || r == '名' ||
			r == '位' || r == '次' || r == '条' || r == '岁' || r == '年' || r == '月':
		default:
			return false
		}
	}
	return digits > 0
}
