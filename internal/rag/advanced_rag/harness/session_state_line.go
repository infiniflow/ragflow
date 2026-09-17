package harness

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
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
// Without them a session can probe term after term and record none of them until it is
// forced to, dropping a name it queried and found by its own reasoning — a decision the
// runtime cannot see and therefore never asks about.
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
	// Members come from the slots that DECLARE members (slots.KindItems), each
	// item carrying its evidence. Nothing is split out of text: a slot holding a
	// sentence, a date or a count contributes no members, and the text is never
	// inspected to find out whether it might have been a list (see package slots for
	// the measurement that made this fail closed).
	// ONE definition of the table's members (see MemberNames): the count the answer
	// reports and the list this line shows read the same fact.
	r.Members = ItemValues(&table)
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
	out := fmt.Sprintf("members=%d reached=%d absent=%d undecided=%d",
		len(r.Members), len(r.Reached), len(r.Absent), len(r.Undecided))
	return out
}

// poolExcerptRunes bounds how much pool text one turn may add to the model's
// context: about one sentence plus its neighbours, which is where a name and the
// deed that makes it a member sit together.
const poolExcerptRunes = 220

// poolScanMax bounds how many pool chunks one turn examines while choosing that
// excerpt. The scan is local — one substring test and one token count per chunk —
// so this bound is about latency discipline, not about money.
const poolScanMax = 60

// subjectWordsMax bounds the words used to recognise "a passage about the same
// subject". They select a passage to READ; they never decide anything.
const subjectWordsMax = 8

// unreadPoolExcerpt returns a short excerpt from ONE pool passage this session has
// never been shown, chosen for what it says that the record does not.
//
// The pool is text the round has ALREADY paid for, and a session only ever sees
// the parts its own queries returned — everything else sat in hand, unread. That
// gap is where members are lost without anyone noticing: a passage can sit in the round's
// evidence, unread by every session, because nothing showed it — and the names an answer is
// missing are all in passages of exactly that kind.
//
// The framework does not name anybody here. It reads the unread passages that
// mention a word THIS SESSION has used — the direction it was sent on, the slot's
// clues, the candidates, and its own queries, which is what carries the corpus's
// own aliases (a session that asked about 关公 gets passages that say 关公) — and
// hands over a bounded excerpt around the first term of that passage the record
// does not contain. The division of labour is the usual one: the runtime supplies
// a fact (this text exists, you have not read it), the model decides what is in it.
//
// Mentioning one of those words is a FILTER, not a preference. Ranking unread passages by
// how much of their vocabulary is new delivers passages the question has nothing to do
// with, because "says the most you have not seen" and "is about this question" are
// anti-correlated: the passages that carry the subject share their vocabulary with what the
// session already read, so they score LOW. The
// words the session itself used are what tells the two apart, and they are the
// model's words, not a lexicon.
func (s *SessionState) unreadPoolExcerpt() string {
	// ONE per session, and that is deliberate. The premise holds — such a passage really is
	// in the pool and really is unread — but the selection is not reliable enough to spend
	// context on every flat turn, so it stays available as the session's last resort rather
	// than a routine.
	if s.KB == nil || s.PoolRead {
		return ""
	}
	subject := s.subjectWords()
	if len(subject) == 0 {
		return ""
	}
	seen := make(map[string]bool, len(s.RetrievedEvidenceIDs))
	for _, id := range s.RetrievedEvidenceIDs {
		seen[id] = true
	}
	known := s.recordVocabulary()
	chunks := s.KB.ChunksFrom(s.PoolWalk, poolScanMax)
	s.PoolWalk += len(chunks)

	var bestID, bestText, bestAnchor string
	bestNovel := 0
	for _, c := range chunks {
		text := ChunkTextOf(c)
		if text == "" {
			continue
		}
		if id := ChunkIDOf(c); id != "" && seen[id] {
			continue
		}
		if !mentionsAny(text, subject) {
			continue
		}
		anchor, novel := novelAnchor(text, known)
		if novel == 0 {
			continue
		}
		if novel > bestNovel {
			bestID, bestText, bestAnchor, bestNovel = ChunkIDOf(c), text, anchor, novel
		}
	}
	if bestNovel == 0 {
		return ""
	}
	s.PoolRead = true
	return fmt.Sprintf("[pool] unread passage already in evidence, never shown to you (id=%s): %s",
		bestID, excerptAround(bestText, bestAnchor, poolExcerptRunes))
}

// mentionsAny reports whether text carries any of the words — the relevance test
// the excerpt is gated on.
func mentionsAny(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

// subjectWords returns the words that stand for what this session is working on:
// the direction it was sent on, the slot's own question clues, the candidates so
// far, and the queries the session itself wrote.
//
// They are the model's (or the planner's) own words, read with the corpus's own
// tokenizer, so no list of names or verbs is involved anywhere.
func (s *SessionState) subjectWords() []string {
	var out []string
	seen := map[string]bool{}
	add := func(text string) {
		if text == "" || len(out) >= subjectWordsMax {
			return
		}
		for _, w := range append(GrepWordsFromQuery(text), tokenizerWords(text)...) {
			w = strings.TrimSpace(w)
			if r := utf8.RuneCountInString(w); r < 2 || r > cjkPhraseRunes {
				continue
			}
			low := strings.ToLower(w)
			if seen[low] {
				continue
			}
			seen[low] = true
			out = append(out, w)
			if len(out) >= subjectWordsMax {
				return
			}
		}
	}
	// The session's OWN queries first: they are the most specific thing it has said, and
	// they are where the corpus's aliases enter — asking with one alias retrieves passages
	// that use another, while the direction may use only one.
	for _, q := range s.SearchQueries {
		add(q)
	}
	for _, v := range s.ParentState.State {
		if v.Candidate != nil {
			add(*v.Candidate)
		}
		for _, clue := range v.QuestionClues {
			add(clue)
		}
	}
	add(s.Direction)
	return out
}

// grewFrom reports whether this record knows something the previous turn's did.
//
// It is the same signal the continuation offer is built on (see
// offerContinuation), used for the opposite decision: an enumeration session whose
// record is still growing is already finding things, so the pool read stays out of
// its way and only steps in when the record has gone flat.
func (r SessionRecord) grewFrom(prev SessionRecord) bool {
	return len(r.Members) > len(prev.Members) ||
		len(r.Reached) > len(prev.Reached) ||
		len(r.Absent) > len(prev.Absent)
}

// tokenizerWords runs the corpus's own tokenizer over a text (see
// tokenizer.Tokenize): the same segmentation the index was built with, which is
// what makes it usable without a lexicon.
//
// When that tokenizer is not available — a unit test, an engine configured
// differently — it falls back to the two-rune windows the grep path uses to
// LOCATE a term inside unbroken text. As a vocabulary that is noisier, but it needs
// nothing but the text, and a passage is only ever SELECTED by this score, never
// decided on it.
func tokenizerWords(text string) []string {
	if toks, err := tokenizer.Tokenize(text); err == nil && strings.TrimSpace(toks) != "" {
		return strings.Fields(toks)
	}
	return cjkWindowsOf(text, tokenWindowsMax)
}

// tokenWindowsMax bounds the fallback vocabulary of one passage.
const tokenWindowsMax = 400

// novelAnchor returns the first term of the text that the record does not contain,
// and how many such terms the text has. The anchor is what the excerpt is centred
// on — the part of the passage that is new to this session is the part worth
// reading.
func novelAnchor(text string, known map[string]bool) (string, int) {
	anchor, n := "", 0
	seen := map[string]bool{}
	for _, t := range tokenizerWords(text) {
		if r := utf8.RuneCountInString(t); r < 2 || r > cjkPhraseRunes {
			continue
		}
		low := strings.ToLower(t)
		if seen[low] || known[low] {
			continue
		}
		seen[low] = true
		if anchor == "" {
			anchor = t
		}
		n++
	}
	return anchor, n
}

// recordVocabulary is everything the record already accounts for: its members, the
// terms probes reached, and the terms probes asked about and did not find. A term
// in here is not new information.
func (s *SessionState) recordVocabulary() map[string]bool {
	out := map[string]bool{}
	for _, group := range [][]string{s.Record.Members, s.Record.Reached, s.Record.Absent, s.Record.Undecided} {
		for _, t := range group {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				out[t] = true
			}
		}
	}
	for _, q := range s.SearchQueries {
		for _, w := range GrepWordsFromQuery(q) {
			out[strings.ToLower(w)] = true
		}
	}
	return out
}

// excerptAround returns a one-line window of at most maxRunes runes around the
// first occurrence of word, so the excerpt carries both the mention and the
// sentence around it.
func excerptAround(text, word string, maxRunes int) string {
	flat := []rune(strings.Join(strings.Fields(text), " "))
	at := []rune(word)
	pos := -1
	for i := 0; i+len(at) <= len(flat); i++ {
		if string(flat[i:i+len(at)]) == word {
			pos = i
			break
		}
	}
	if pos < 0 {
		pos = 0
	}
	start := pos - maxRunes/3
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(flat) {
		end = len(flat)
	}
	out := string(flat[start:end])
	if start > 0 {
		out = "…" + out
	}
	if end < len(flat) {
		out += "…"
	}
	return out
}
