package runtime

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
)

// sessionRecord is what a session can be told about its OWN progress. Every field is mechanical: the
// notes the model itself wrote, and counters about what it has been shown.
//
// It used to carry a member list the runtime had DERIVED — the terms probes reached, the names the
// slots did not mention, the items a slot TYPE declared — and recall was decided by that derivation:
// a name behind the one-line record's "+15" was a name no answer could have (measured 2026-09-20,
// 三国/关羽: nineteen names in the derived ledger, ten in the answer). Deciding what counts as a member
// is a SEMANTIC judgement and the runtime does not make it. The model's own notes are the ledger (its
// <state> patches are this loop's scratchpad), and the runtime's job is to render them whole and to
// say what it has shown.
type sessionRecord struct {
	// Pool is the shared evidence pool's size at this turn.
	Pool int
	// Notes are the model's OWN notes, verbatim and in the order it wrote them: the slot candidates
	// its patches produced. Nothing is split, counted, filtered or truncated — the ledger belongs to
	// the model (see sessionState.noteValues).
	Notes []string
	// ShownSinceNote is how many passages the run has shown the model since it last wrote a note: the
	// mechanical face of "you have read more than you have written down", and the replacement for the
	// reach-ledger comparison the runtime used to make on the model's behalf.
	ShownSinceNote int
	// Asked are this session's own queries, verbatim, so it can see it has already run them. The
	// strings are the model's, not the runtime's reading of them.
	Asked []string
}

// sessionRecord gathers the record from the live pool plus the session's own notes.
func (s *sessionState) sessionRecord() sessionRecord {
	var r sessionRecord
	if s == nil {
		return r
	}
	if s.KB != nil {
		r.Pool = s.KB.PoolSize()
	}
	r.Notes = s.noteValues(s.workingTable())
	r.ShownSinceNote = max(0, len(s.RetrievedEvidenceIDs)-s.notesAtEvidence)
	r.Asked = append([]string(nil), s.SearchQueries...)
	return r
}

// noteValues are the values the MODEL has written into the table, verbatim: the candidates its patches
// produced and the claims they displaced. The table is the session's scratchpad, so this is its
// notebook — and the runtime reads it only to render it back (see sessionRecord.Notes).
func (s *sessionState) noteValues(table State) []string {
	var out []string
	for _, v := range table.State {
		if v.Candidate != nil {
			if c := strings.TrimSpace(*v.Candidate); c != "" {
				out = append(out, c)
			}
		}
		for _, a := range v.Alternates {
			if a = strings.TrimSpace(a); a != "" {
				out = append(out, a)
			}
		}
	}
	return out
}

// Line renders the record as ONE line for the tool result the model reads.
//
// Counts and the model's own queries only. It may NOT summarize the model's own notes: a "+15" of the
// model's own writing is a line the model cannot act on, and that is exactly how recall was lost (see
// the type's note). The notes themselves are already in the conversation — the model wrote them — and
// the places that have to account for them render them whole (see Verbose).
func (r sessionRecord) Line() string {
	var b strings.Builder
	fmt.Fprintf(&b, "[record] your notes=%d item(s)", len(r.Notes))
	if r.ShownSinceNote > 0 {
		fmt.Fprintf(&b, " | %d passage(s) shown since your last note", r.ShownSinceNote)
	}
	fmt.Fprintf(&b, " | pool=%d", r.Pool)
	if len(r.Asked) > 0 {
		fmt.Fprintf(&b, " | already asked: %s", strings.Join(r.Asked, " ; "))
	}
	return b.String()
}

// Verbose renders the model's notes in FULL — one per line, verbatim — for the places where the model
// is asked to account for what it has: the answer turn, and the composition that follows it.
func (r sessionRecord) Verbose() string {
	if len(r.Notes) == 0 {
		return ""
	}
	lines := make([]string, 0, len(r.Notes))
	for i, n := range r.Notes {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, n))
	}
	return fmt.Sprintf("Your own notes so far (%s, verbatim — what you wrote down as you worked):\n%s",
		CountOf(len(r.Notes), "item"), strings.Join(lines, "\n"))
}

// Brief is the record's counts, for logs.
func (r sessionRecord) Brief() string {
	return fmt.Sprintf("notes=%d shown_since_note=%d pool=%d asked=%d",
		len(r.Notes), r.ShownSinceNote, r.Pool, len(r.Asked))
}

// workingTable is the slot table as THIS session has patched it: the parent table
// with the session's own branch patches applied in order. Reading the parent table
// alone would report the record as it stood when the session started, so a member
// the session itself just found would still read as missing.
func (s *sessionState) workingTable() State {
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
func (s *sessionState) unreadPoolExcerpt() string {
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
func (s *sessionState) subjectWords() []string {
	var out []string
	seen := map[string]bool{}
	add := func(text string) {
		if text == "" || len(out) >= subjectWordsMax {
			return
		}
		for _, w := range append(grepWordsFromQuery(text), tokenizerWords(text)...) {
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

// recordVocabulary is everything the model's OWN record already accounts for: the notes it wrote and
// the words of the queries it ran. A term in here is not new information — and the judgement is
// mechanical, because what is "already known" is exactly what the model has written and asked.
func (s *sessionState) recordVocabulary() map[string]bool {
	out := map[string]bool{}
	for _, t := range s.Record.Notes {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out[t] = true
		}
	}
	for _, q := range s.SearchQueries {
		for _, w := range grepWordsFromQuery(q) {
			out[strings.ToLower(w)] = true
		}
	}
	return out
}

// excerptAround returns a one-line window of at most maxRunes runes around the
// first occurrence of word, so the excerpt carries both the mention and the
// sentence around it.
func excerptAround(text, word string, maxRunes int) string {
	flat := []rune(FlattenLine(text))
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
