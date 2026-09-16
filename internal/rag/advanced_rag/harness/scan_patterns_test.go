package harness

import (
	"context"
	"strings"
	"testing"
)

// TestScanPatternsAskForActorAndActTogether pins the shape of the completeness query.
//
// One clause per (actor, act) pair, never `A|B.*T`: `|` binds loosest in Go's regexp, so
// that would read as "A" OR "B.*T" and silently drop the actor from every clause but the
// last. And the actor is what makes the query do work the model's memory cannot: measured
// (2026-09-16, 三国/关羽) every run of one question missed 2-4 members, the missing names
// differed every run, and in one run four of them appeared ZERO times in the whole log.
func TestScanPatternsAskForActorAndActTogether(t *testing.T) {
	table := NewState([]Variable{
		{ID: 0, Type: "count", Terms: []string{"斩", "杀"}, Subject: "关羽|关公|云长"},
		{ID: 1, Type: "person"},
	}, 0, nil)

	got := ScanPatterns(table)
	if len(got) == 0 {
		t.Fatal("no patterns for a table that declared two act words and three aliases")
	}
	clauses := 0
	for _, q := range got {
		parts := strings.Split(q, "|")
		if len(parts) > GrepTermsMax/2 {
			t.Errorf("query %q carries %d clauses, want at most %d (each clause is two operands)", q, len(parts), GrepTermsMax/2)
		}
		for _, part := range parts {
			clauses++
			if !strings.Contains(part, ".*") {
				t.Errorf("clause %q does not ask for actor AND act together", part)
			}
		}
	}
	if clauses != 6 {
		t.Errorf("clauses = %d, want 6 (3 aliases × 2 act words)", clauses)
	}
	if !strings.Contains(strings.Join(got, "|"), "云长.*斩") {
		t.Errorf("patterns %v, want every declared alias paired with every act word", got)
	}

	// No subject declared: the act word itself, which is all the direction gave.
	bare := ScanPatterns(NewState([]Variable{{ID: 0, Type: "count", Terms: []string{"斩"}}}, 0, nil))
	if len(bare) != 1 || bare[0] != "斩" {
		t.Errorf("patterns = %v, want the act word alone when no actor was declared", bare)
	}

	// No declaration, no queries: a table that named no act word pays nothing.
	if p := ScanPatterns(NewState([]Variable{{ID: 0, Type: "date", Candidate: strPtr("1858")}}, 0, nil)); len(p) != 0 {
		t.Errorf("patterns = %v, want none without a declaration", p)
	}
}

// stubPatternRunner answers one completeness pattern with the chunks a test staged for it.
type stubPatternRunner struct {
	calls     []string
	byPattern map[string][]map[string]any
}

func (s *stubPatternRunner) RunPattern(_ context.Context, pattern string) []map[string]any {
	s.calls = append(s.calls, pattern)
	return s.byPattern[pattern]
}

// TestCompletenessPassRunsWhatItRendered pins the fix for a measured failure: the patterns
// were RENDERED, and nobody ran them.
//
// Measured (2026-09-16, 三国/关羽, one round): ten act words with several aliases each came
// to 2175 characters of patterns, present in the seed of every session, and the run's query
// log holds ZERO `.*` queries — the sessions improvised space-separated word lists instead
// (`关羽 斩华雄 温酒`, `关公 砍死 斩 杀`). A list of queries in a prompt is advice, and the
// completeness of an enumeration cannot rest on advice. So the runtime asks the corpus
// itself, admits the windows, and seeds what came back.
func TestCompletenessPassRunsWhatItRendered(t *testing.T) {
	table := NewState([]Variable{
		{ID: 0, Type: "count", Terms: []string{"斩", "劈"}, Subject: "关羽|云长|关公"},
		{ID: 1, Type: "dataset"},
	}, 0, nil)
	patterns := ScanPatterns(table)
	if len(patterns) < 2 {
		t.Fatalf("patterns = %v, want at least two queries (the declaration has six clauses)", patterns)
	}
	window := map[string]any{"chunk_id": "c1", "content_with_weight": "云长手起刀落，斩孔秀于马下"}
	runner := &stubPatternRunner{byPattern: map[string][]map[string]any{patterns[0]: {window}}}
	kb := &Kbinfos{}

	pass := RunCompletenessPass(context.Background(), runner, kb, table)

	if len(runner.calls) != len(patterns) {
		t.Fatalf("ran %d pattern(s), want every declared one (%d): %v", len(runner.calls), len(patterns), runner.calls)
	}
	if pass.Asked != len(patterns) || pass.Answered != 1 || pass.Admitted != 1 {
		t.Errorf("pass = asked %d / answered %d / admitted %d, want %d / 1 / 1",
			pass.Asked, pass.Answered, pass.Admitted, len(patterns))
	}
	if !strings.Contains(pass.Text, "c1") || !strings.Contains(pass.Text, "斩孔秀于马下") {
		t.Errorf("block %q, want the window with the chunk id a member cites", pass.Text)
	}
	if !strings.Contains(pass.Text, "(no window)") {
		t.Errorf("block %q, want the empty pattern stated as asked-and-empty — otherwise the session cannot tell "+
			"\"the corpus has no such passage\" from \"nobody looked\"", pass.Text)
	}
	for _, p := range patterns {
		if !strings.Contains(pass.Text, p) {
			t.Errorf("block %q does not name the pattern %q it ran", pass.Text, p)
		}
	}
	// The window is EVIDENCE: it must be in the pool, under the id the block cites, or the
	// session can read it and cite nothing.
	if kb.PoolSize() != 1 || ChunkIDOf(kb.Chunks[0]) != "c1" {
		t.Errorf("pool = %d chunk(s) %v, want the matched window admitted", kb.PoolSize(), kb.Chunks)
	}

	// A table that holds NO NAME pays nothing: the planner declares act words for "a count
	// of things someone DID" too, and no name a pass could return changes a count of events
	// (measured 2026-09-16, FRAMES: two such questions, ~18% of the run's tokens).
	countsOnly := NewState([]Variable{
		{ID: 0, Type: "count", Terms: []string{"won", "trophy"}, Subject: "Brazil"},
	}, 0, nil)
	quiet := &stubPatternRunner{}
	if pass := RunCompletenessPass(context.Background(), quiet, kb, countsOnly); pass.Asked != 0 || len(quiet.calls) != 0 {
		t.Errorf("ran %v for a table with no name slot, want nothing", quiet.calls)
	}
	// And a run with no runner is not a panic but a fallback: the seed keeps the list.
	if pass := RunCompletenessPass(context.Background(), nil, kb, table); pass.Text != "" {
		t.Errorf("pass without a runner = %q, want nothing", pass.Text)
	}
}

// TestEnumerationSeedCarriesFindingsInsteadOfTheList pins what the session reads when the
// pass has run: the windows, not the queries it would have to think of.
func TestEnumerationSeedCarriesFindingsInsteadOfTheList(t *testing.T) {
	loader := StringPromptLoader{"action_set": "SET / COUNT directions — the member list IS the work"}
	table := NewState([]Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽|云长"},
		{ID: 1, Type: "dataset"},
	}, 0, nil)

	// No pass yet: the fallback is the pattern list, marked as queries to make.
	seed := enumerationSeed(table, loader, "")
	if !strings.Contains(seed, "关羽.*斩|云长.*斩") {
		t.Fatalf("fallback seed %q, want the rendered patterns", seed)
	}

	findings := "## The completeness pass ALREADY RAN for this direction\n\n- 关羽.*斩 →\n    chunk_id=c1  \"斩孔秀\""
	seed = enumerationSeed(table, loader, findings)
	if !strings.Contains(seed, "chunk_id=c1") {
		t.Errorf("seed %q, want the pass's windows", seed)
	}
	if !strings.Contains(seed, "SET / COUNT directions") {
		t.Errorf("seed %q, want the method still travelling with the windows", seed)
	}
	if strings.Contains(seed, "one call per line") {
		t.Errorf("seed %q still lists patterns to make — the windows replaced them", seed)
	}
}
