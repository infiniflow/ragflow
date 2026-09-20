package runtime

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/rag/agentic-rag/slots"
)

// typedMembersSlot / typedCountSlot build the slots the record reads: a slot holds
// members or a number because the model DECLARED it (see package slots), so these are
// what a patch now produces.
func typedMembersSlot(id int, typ string, names ...string) Variable {
	items := make([]slots.Item, 0, len(names))
	for _, n := range names {
		items = append(items, slots.Item{Value: n})
	}
	v := slots.Items(items...)
	rendered := slots.Render(v)
	return Variable{ID: id, Type: typ, Candidate: &rendered, Value: &v}
}

func typedCountSlot(id int, typ string, n int) Variable {
	v := slots.Number(n)
	rendered := slots.Render(v)
	return Variable{ID: id, Type: typ, Candidate: &rendered, Value: &v}
}

// TestCollectSessionRecordFlagsFoundButNotRecorded is the defect this record
// exists for: a name the session PROVED reachable (it has a passage) and never
// took a position on must be visible, not silent.
//
// Measured (fixrecall2, 2026-09-15): 庞德 was queried, a passage came back, and
// the model's own reasoning dropped him — the framework saw nothing.
func TestCollectSessionRecordFlagsFoundButNotRecorded(t *testing.T) {
	kb := &Kbinfos{}
	for i := 0; i < 5; i++ {
		id := "c" + strconv.Itoa(i)
		kb.Admit(func(p *PoolAdmitter) {
			p.Add(map[string]any{"chunk_id": id, "content": "prose"})
		})
	}
	kb.RecordReachedTerm("荀正", "c1")
	kb.RecordReachedTerm("庞德", "c2")
	kb.RecordProbedAbsent("杨龄")

	table := State{State: []Variable{
		typedCountSlot(0, "count", 13),
		typedMembersSlot(1, "person", "孔秀", "孟坦", "荀正"),
	}}
	rec := CollectSessionRecord(table, kb)
	// 13 is the COUNT, not a member: a member count inflated by the answer slot is
	// the number the model steers by, so the answer slot must not be in it.
	if len(rec.Members) != 3 || rec.Members[0] != "孔秀" {
		t.Fatalf("members = %v, want the 3 names the candidates list (no count)", rec.Members)
	}
	if len(rec.Reached) != 2 || len(rec.Absent) != 1 {
		t.Fatalf("ledger = reached %v absent %v, want 2 reached / 1 absent", rec.Reached, rec.Absent)
	}
	if len(rec.Undecided) != 1 || rec.Undecided[0] != "庞德" {
		t.Fatalf("undecided = %v, want [庞德]: 荀正 is recorded, 庞德 is not", rec.Undecided)
	}
	line := rec.Line()
	for _, want := range []string{"members=3", "FOUND BUT NOT RECORDED=庞德", "asked-nothing-back=1", "pool=5"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q missing %q", line, want)
		}
	}
}

// TestSessionRecordLineCarriesNoPoolJudgement pins what the line does NOT claim:
// it reports the pool's size as a number and nothing more. The turn budget is the
// model's decision now (see offerContinuation), so no runtime predicate may turn
// "the pool grew" into "keep going".
func TestSessionRecordLineCarriesNoPoolJudgement(t *testing.T) {
	rec := SessionRecord{Pool: 500, Members: []string{"华雄"}, Reached: []string{"华雄", "颜良"}}
	line := rec.Line()
	if !strings.Contains(line, "pool=500") {
		t.Fatalf("line %q, want the pool size reported", line)
	}
	for _, banned := range []string{"grew", "growth", "continue"} {
		if strings.Contains(strings.ToLower(line), banned) {
			t.Errorf("line %q must not advise on continuing", line)
		}
	}
}

// TestWorkingTableAppliesSessionPatches pins the table the record reads: the
// session's own branches, or a member the session just found would still read as
// missing.
func TestWorkingTableAppliesSessionPatches(t *testing.T) {
	s := &SessionState{
		ParentState: State{State: []Variable{
			{ID: 1, Type: "person", Candidate: nil},
			{ID: 2, Type: "person", Candidate: strPtr("华雄")},
		}},
		NewStates: []State{{State: []Variable{
			{ID: 1, Type: "person", Candidate: strPtr("孔秀、孟坦")},
		}}},
	}
	tbl := s.workingTable()
	if tbl.State[0].Candidate == nil || *tbl.State[0].Candidate != "孔秀、孟坦" {
		t.Fatalf("patched slot = %v, want the session's own candidate", tbl.State[0].Candidate)
	}
	if tbl.State[1].Candidate == nil || *tbl.State[1].Candidate != "华雄" {
		t.Fatalf("untouched slot = %v, want the parent candidate kept", tbl.State[1].Candidate)
	}
	// The parent table is NOT mutated: another session shares it.
	if s.ParentState.State[0].Candidate != nil {
		t.Fatal("workingTable must not write through to the shared parent table")
	}
}

// TestAppendRecordLineRidesOnTheLastToolMessage pins the channel: one line on the
// tool result the model is about to read, and nothing at all on a turn that ran no
// tool call (or has no pool to report on).
func TestAppendRecordLineRidesOnTheLastToolMessage(t *testing.T) {
	kb := &Kbinfos{}
	kb.Admit(func(p *PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content": "prose"})
	})
	s := &SessionState{
		KB: kb,
		ParentState: State{State: []Variable{
			typedCountSlot(0, "count", 13),
			typedMembersSlot(1, "person", "孔秀", "孟坦"),
		}},
		// The record line is gated on the session ENUMERATING, and the tell is the batch the
		// caller wrote — never a slot type (see enumerating).
		SearchQueries: []string{"关羽 斩 杀 颜良 文丑"},
		Messages:      []schema.Message{*schema.ToolMessage(`{"passages": []}`, "call_1")},
	}
	s.appendRecordLine(true)
	got := s.Messages[len(s.Messages)-1].Content
	if !strings.Contains(got, "[record]") || !strings.Contains(got, "pool=1") {
		t.Fatalf("tool message = %q, want the record line appended", got)
	}
	if !strings.HasPrefix(got, `{"passages": []}`) {
		t.Fatalf("tool message = %q, want the payload preserved ahead of the line", got)
	}
	if !strings.Contains(got, "members=2") {
		t.Errorf("line %q, want the members the table holds", got)
	}

	// A turn with no tool call must not touch the previous result.
	before := s.Messages[len(s.Messages)-1].Content
	s.appendRecordLine(false)
	if s.Messages[len(s.Messages)-1].Content != before {
		t.Fatal("a turn that ran no tool call must not append a line")
	}

	// No pool bound: the record is kept, the line is skipped rather than inventing
	// a message for it.
	empty := &SessionState{Messages: []schema.Message{*schema.ToolMessage("x", "call_2")}}
	empty.appendRecordLine(true)
	if empty.Messages[0].Content != "x" {
		t.Fatalf("no-pool message = %q, want it untouched", empty.Messages[0].Content)
	}
}

// TestOfferContinuationLetsTheModelDecide pins the turn-budget contract: past the
// mode's floor the session does NOT stop, and it does not extend itself on a
// runtime predicate either — the model is asked, once per turn, up to a hard cap.
//
// The cap is the part the runtime keeps: medium/high run 8 → 12, ultra 10 → 14,
// which is the "maximum run" the session may never exceed however eager the model
// is. The floors are asserted here because the enumeration path walks them: a
// session that is still turning names into evidence needs the turns these numbers
// buy (see the measurement on offerContinuation).
func TestOfferContinuationLetsTheModelDecide(t *testing.T) {
	for mode, floor := range map[string]int{"medium": 8, "high": 8, "ultra": 10} {
		if got := GetMode(mode).ActionMaxTurns; got != floor {
			t.Errorf("%s turn floor = %d, want %d (cap = floor + %d)", mode, got, floor, turnRunExtra)
		}
	}

	s := enumerationSession(4, 90)
	cap := s.turnRunCap()
	if cap != enumerationSession(0, 90).actionMaxTurns()+turnRunExtra {
		t.Fatalf("run cap = %d, want the floor + %d (the runaway guard)", cap, turnRunExtra)
	}
	if !s.offerContinuation() {
		t.Fatal("past the floor the session must offer the model another turn")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != schema.User {
		t.Fatalf("offer role = %v, want a user message the model answers", last.Role)
	}
	for _, want := range []string{"TURN BUDGET", fmt.Sprintf("up to %d", cap), fmt.Sprintf("%d turn(s) left", cap-4), "state patch NOW"} {
		if !strings.Contains(last.Content, want) {
			t.Errorf("offer %q missing %q", last.Content, want)
		}
	}

	// Idempotent within a turn: the model must not be asked twice for one turn.
	before := len(s.Messages)
	if !s.offerContinuation() {
		t.Fatal("re-asking for the same turn must still allow the turn")
	}
	if len(s.Messages) != before {
		t.Fatalf("messages grew to %d, want the offer appended once per turn", len(s.Messages))
	}

	// A new turn asks again (the model re-decides with the new tool result).
	s.Attempts = 5
	if !s.offerContinuation() || len(s.Messages) != before+1 {
		t.Fatal("each new turn must carry its own offer")
	}

	// The hard cap (a runaway guard, not the loop's bound): no offer, and the session finalizes.
	capped := enumerationSession(enumerationSession(0, 90).turnRunCap(), 90)
	if capped.offerContinuation() {
		t.Fatal("the run cap must not be exceedable")
	}
	if len(capped.Messages) != 0 {
		t.Fatal("no offer may be appended at the cap")
	}

	// The clock is the other hard bound: without room for the finalize step no
	// further turn is offered, however willing the model is.
	tight := enumerationSession(4, turnAskFloorS-1)
	if tight.offerContinuation() {
		t.Fatal("no turn may be offered without room for the finalize step")
	}
}

// enumerationSession is a session sent on a SET question — the parent table holds
// a count slot, which is the shape the continuation offer is gated on.
func enumerationSession(attempts int, deadlineLeft float64) *SessionState {
	return &SessionState{
		Attempts:     attempts,
		DeadlineLeft: deadlineLeft,
		// What makes this session an ENUMERATING one is the batch the caller wrote, not the type
		// of a slot (see enumerating): a count-typed slot says nothing about what the session is
		// doing, and it is the same shape for a count of events.
		SearchQueries: []string{"关羽 斩 杀 颜良 文丑"},
		ParentState:   State{State: []Variable{{ID: 0, Type: "count", Candidate: strPtr("12")}}},
	}
}

// TestOfferContinuationIsBoundedByTheCapAndTheClockNotByTheShape pins the turn-budget contract
// after the shape gate was removed.
//
// The offer used to require an ENUMERATING session, so a multi-hop VALUE question was cut off at
// its halved floor with the clock unspent — the log of a stopped session reads "167 seconds of
// research budget left" beside "TOOL BUDGET EXHAUSTED" (measured 2026-09-20, FRAMES: "turn floor
// 8 → 4" 15 times, 8 rounds finalized by the salvage prompt, ~150s of the 180s budget unspent, and
// the four questions that stayed at zero each hit that wall). What bounds the offer is what the
// runtime does not delegate — the run cap and the session clock — and the ask hands the model the
// record's own brief, so "continue" has to be justified by something the record shows is missing.
func TestOfferContinuationIsBoundedByTheCapAndTheClockNotByTheShape(t *testing.T) {
	value := &SessionState{
		Tools:        &Toolset{ThinkingMode: "high"},
		Attempts:     4,
		DeadlineLeft: 90,
		ParentState:  State{State: []Variable{{ID: 0, Type: "entity", Candidate: strPtr("白马坡")}}},
	}
	if !value.offerContinuation() {
		t.Fatal("a value question past its floor must be offered another turn (the model decides)")
	}
	last := value.Messages[len(value.Messages)-1]
	if last.Role != schema.User {
		t.Fatalf("offer role = %v, want a user message the model answers", last.Role)
	}
	// Asked once per turn, however often the route runs.
	if !value.offerContinuation() {
		t.Fatal("the offer must stand for the turn it was made on")
	}
	if len(value.Messages) != 1 {
		t.Fatalf("offer appended %d messages, want exactly one per turn", len(value.Messages))
	}

	// The two bounds the runtime keeps. First the run cap: at it, nothing more is offered.
	capped := &SessionState{
		Tools: &Toolset{ThinkingMode: "high"}, DeadlineLeft: 90,
	}
	capped.Attempts = capped.turnRunCap()
	if capped.offerContinuation() {
		t.Fatal("at the run cap no further turn may be offered")
	}
	// Then the clock: below the floor the finalize step must still fit, so nothing is offered.
	late := &SessionState{
		Tools: &Toolset{ThinkingMode: "high"}, Attempts: 4, DeadlineLeft: turnAskFloorS,
	}
	if late.offerContinuation() {
		t.Fatal("with no clock left for the answer turn, no further turn may be offered")
	}
}

// TestRouteAtTheFloorOffersTheModelTheDecision pins the routing wiring: the floor
// routes to another model turn WITH the offer attached, and the cap routes to
// finalize with nothing attached.
func TestRouteAtTheFloorOffersTheModelTheDecision(t *testing.T) {
	s := enumerationSession(4, 90)
	if got := s.route(); got != routeRunAction {
		t.Fatalf("route at the floor = %v, want routeRunAction (the model decides)", got)
	}
	if len(s.Messages) != 1 {
		t.Fatalf("messages = %d, want the offer appended", len(s.Messages))
	}

	// At the runaway cap there is nothing to offer — and the cap is not what usually stops a
	// session: the clock's answer reserve is (see runActionNode, answerReserveS).
	capped := enumerationSession(enumerationSession(0, 90).turnRunCap(), 90)
	if got := capped.route(); got != routeFinalize {
		t.Fatalf("route at the cap = %v, want routeFinalize", got)
	}

	// A terminal reply still ends the session immediately: the offer is for turns
	// that have not already concluded.
	done := enumerationSession(4, 90)
	done.Done = true
	if got := done.route(); got != routeEnd {
		t.Fatalf("route after a terminal reply = %v, want routeEnd", got)
	}
	// ...and a spent clock finalizes rather than offering.
	tight := enumerationSession(4, turnAskFloorS-1)
	if got := tight.route(); got != routeFinalize {
		t.Fatalf("route without clock = %v, want routeFinalize", got)
	}
}

func strPtr(s string) *string { return &s }

// TestUnreadPoolExcerptShowsTextTheSessionHasNotSeen pins the pool read.
//
// The pool is text the round has already paid for, and a session only ever sees
// the parts its own queries returned: everything else sits in hand, unread. That
// is where members are lost without anyone noticing — measured (2026-09-15,
// 三国演义/关羽): the passage naming 管亥 was fetched into the round's evidence and
// no session ever named it, because nothing had shown it.
func TestUnreadPoolExcerptShowsTextTheSessionHasNotSeen(t *testing.T) {
	kb := &Kbinfos{}
	kb.Admit(func(p *PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "seen01", "content": "关公温酒斩华雄，其酒尚温。"})
		p.Add(map[string]any{"chunk_id": "offtopic", "content": "那张角本是个不第秀才，因入山采药，遇一老人，碧眼童颜，手执藜杖，唤角至一洞中，以天书三卷授之。"})
		p.Add(map[string]any{"chunk_id": "unread01", "content": "关公大怒，拍马舞刀，直取管亥，管亥措手不及，被关公一刀劈于马下。"})
	})
	s := &SessionState{
		KB:                   kb,
		Direction:            "关羽斩杀了哪些有名有姓的人物",
		SearchQueries:        []string{"关公 斩 管亥"},
		RetrievedEvidenceIDs: []string{"seen01"},
	}
	got := s.unreadPoolExcerpt()
	if !strings.Contains(got, "unread01") {
		t.Fatalf("excerpt = %q, want the unread passage this session's own words point at", got)
	}
	if strings.Contains(got, "华雄") {
		t.Fatalf("excerpt = %q, want the passage this session has seen skipped", got)
	}
	// The passage that says the most the session has not seen is NOT the passage to
	// show: "most novel" and "about this question" are anti-correlated, and the
	// first version of this delivered nothing but chapter headings, 曹操's youth and
	// 张角 receiving the book. Only the session's own words tell the two apart.
	if strings.Contains(got, "张角") {
		t.Fatalf("excerpt = %q, want a passage the session's own words exclude", got)
	}
	// One excerpt per session: across the first two runs of this mechanism it
	// delivered twelve excerpts and none of them carried a member the record was
	// missing, so it stays a last resort rather than a per-turn routine.
	if again := s.unreadPoolExcerpt(); again != "" {
		t.Fatalf("a second excerpt was offered: %q", again)
	}
}

// TestCoverageFollowsThePlannersDeclaration pins the half of the gate that reads the
// table: what the planner TYPED, never what a candidate looks like. A list-shaped
// candidate under a scalar type is prose that happens to contain separators —
// measured (2026-09-15, FRAMES) a slot typed "dataset" carried
// `Grace's、High、Falls、Colonial、Creek`, ONE waterfall's name, and the permissive
// reading of it is how a "how much shorter" record came to say "enumerated
// members: 16" and how a `[count]` slot on a "how many times larger" question once
// bought 24 continuation offers.
func TestRecordReadsWhatTheTableHoldsNotItsTypeWords(t *testing.T) {
	// A type word is a LABEL THE PLAN CHOSE, so it decides nothing. Reading "count" as "the answer
	// is a set" is how a "how many times larger is A than B" question came to carry a member list
	// (measured: a `[count]` slot on that question bought 24 continuation offers).
	for _, typ := range []string{"entity", "count", "number", "dataset", "person"} {
		table := State{State: []Variable{{ID: 0, Type: typ, Candidate: strPtr("白马坡")}}}
		if got := len(ItemValues(&table)); got != 0 {
			t.Errorf("a %q slot holding one value: members = %d, want 0 (nothing is held)", typ, got)
		}
	}
	// The actor's forms come out of the TABLE's own Subject field, and they are what an item list
	// must not count as members of the deed.
	shaped := State{State: []Variable{
		{ID: 0, Type: "count", Terms: []string{"斩", "杀", "斩"}, Subject: "关羽|云长"},
		{ID: 1, Type: "dataset"},
	}}
	if forms := ActorForms(shaped); len(forms) != 2 || forms[0] != "关羽" || forms[1] != "云长" {
		t.Errorf("actor forms = %v, want the two declared spellings", forms)
	}
	// A table that declares no actor declares no forms — an empty list, never a guess.
	if forms := ActorForms(State{State: []Variable{{ID: 0, Type: "date", Candidate: strPtr("1858")}}}); len(forms) != 0 {
		t.Errorf("actor forms = %v, want none", forms)
	}
}

// TestEnumerationIsSeededWithTheMethod pins WHERE the enumeration method is delivered — in the
// seed, before the first turn — and, just as important, WHO gets it: only a table that declared the
// whole enumeration (a count/set/list slot, a NAME-carrying slot and the act words, see
// Coverage.Ok).
//
// Its first instruction — propose more candidates than you expect — is a decision taken before the
// first query: measured (2026-09-16, 三国/关羽) the same question answered eighteen members with the
// method in the seed of its `[count]` table and fourteen when it arrived a turn later. But the same
// instruction on a question whose answer is ONE value sends the session looking for members it does
// not need: measured (2026-09-16, FRAMES — 4 questions in flight, 300s deadline) the shape-only
// gate seeded the value questions that merely contain a count and the run finished 0.833 with two
// timeouts against 0.875 with none.
//
// What the seed carries alongside the method is the windows the enumeration FOUND, never the
// queries to make (see CoverageSet.Render): a query list is advice the model did not follow, a
// window is evidence with the chunk id a member is cited by.
// TestSessionStampsEvidenceRefsInFirstSeenOrder pins the registry the session's own citations
// index into (see stampEvidenceRefs).
//
// It is the mechanical half of "the answer may only cite what was read": the model can only
// write a number it was SHOWN, so every passage carries one, a passage reached twice keeps the
// first number it was given (the same chunk cited from two calls cites one place), and the
// registry is that same order — which is the list handed to the citation resolver afterwards.
func TestSessionStampsEvidenceRefsInFirstSeenOrder(t *testing.T) {
	s := &SessionState{}

	first := []any{
		map[string]any{"chunk_id": "c1", "content": "a"},
		map[string]any{"chunk_id": "c2", "content": "b"},
	}
	s.stampEvidenceRefs(first)
	if got := first[0].(map[string]any)["ref"]; got != 0 {
		t.Errorf("first passage ref = %v, want 0", got)
	}
	if got := first[1].(map[string]any)["ref"]; got != 1 {
		t.Errorf("second passage ref = %v, want 1", got)
	}

	// list_chunks names its chunk "id" rather than "chunk_id", and a chunk already shown keeps
	// the number it was shown with.
	second := []any{
		map[string]any{"id": "c3", "content": "c"},
		map[string]any{"chunk_id": "c1", "content": "a again"},
	}
	s.stampEvidenceRefs(second)
	if got := second[0].(map[string]any)["ref"]; got != 2 {
		t.Errorf("list_chunks passage ref = %v, want 2 (the registry reads both id spellings)", got)
	}
	if got := second[1].(map[string]any)["ref"]; got != 0 {
		t.Errorf("a chunk already in the registry got ref %v, want its original 0", got)
	}

	// A passage with no id at all is not evidence and takes no number.
	s.stampEvidenceRefs([]any{map[string]any{"content": "no id"}})

	want := []string{"c1", "c2", "c3"}
	if len(s.EvidenceRefs) != len(want) {
		t.Fatalf("registry = %v, want %v", s.EvidenceRefs, want)
	}
	for i, id := range want {
		if s.EvidenceRefs[i] != id {
			t.Errorf("registry[%d] = %q, want %q", i, s.EvidenceRefs[i], id)
		}
	}
}

// TestSessionFoldsEarlierToolResults pins the context bound (see compactEarlierToolResults): the
// LAST turn's results stay verbatim — the model is about to reason over them — while everything
// older becomes a digest that says how many passages were read and that their refs still resolve.
//
// The digest has to carry those two facts: without the count the model cannot tell "I read 12
// passages here" from "I read nothing", and without the ref note it would not know it may still
// cite what it read.
func TestSessionFoldsEarlierToolResults(t *testing.T) {
	big := `{"passages":[` + strings.Repeat(`{"ref":0,"chunk_id":"c1","content":"x"},`, 40) + `{"ref":41,"chunk_id":"c2","content":"y"}]}`
	small := func(id string) string { return `{"passages":[{"ref":0,"chunk_id":"` + id + `","content":"x"}]}` }
	s := &SessionState{}
	s.Messages = []schema.Message{
		*schema.SystemMessage("system"),
		*schema.UserMessage("q"),
		// turn 1: far enough back to be folded
		*schema.AssistantMessage("turn 1", nil),
		*schema.ToolMessage(big, "call-1"),
		// turns 2-4: the last `verbatimSessionTurns` turns stay verbatim — a multi-hop chain holds
		// the hop it read while it searches the next one.
		*schema.AssistantMessage("turn 2", nil),
		*schema.ToolMessage(small("c2"), "call-2"),
		*schema.AssistantMessage("turn 3", nil),
		*schema.ToolMessage(small("c3"), "call-3"),
		*schema.AssistantMessage("turn 4", nil),
		*schema.ToolMessage(small("c4"), "call-4"),
	}
	s.compactEarlierToolResults()

	if got := s.Messages[3].Content; got == big || !strings.Contains(got, "folded") {
		t.Errorf("turn 1 result = %q, want a digest", got)
	} else if !strings.Contains(got, `"passages":41`) {
		t.Errorf("digest = %q, want the number of passages it stood for", got)
	} else if !strings.Contains(got, "[ID:n]") {
		t.Errorf("digest = %q, want it to say the refs still resolve", got)
	}
	for _, i := range []int{5, 7, 9} {
		if got := s.Messages[i].Content; strings.Contains(got, "folded") {
			t.Errorf("message %d was folded; the last %d turns must stay verbatim: %q",
				i, verbatimSessionTurns, got)
		}
	}

	// A short result is not worth folding: the digest would cost more than it saves.
	short := &SessionState{Messages: []schema.Message{
		*schema.AssistantMessage("a", nil),
		*schema.ToolMessage(`{"passages":[]}`, "c1"),
		*schema.AssistantMessage("b", nil),
		*schema.ToolMessage(`{"passages":[]}`, "c2"),
		*schema.AssistantMessage("c", nil),
		*schema.ToolMessage(`{"passages":[]}`, "c3"),
		*schema.AssistantMessage("d", nil),
		*schema.ToolMessage(`{"passages":[]}`, "c4"),
	}}
	short.compactEarlierToolResults()
	if short.Messages[1].Content != `{"passages":[]}` {
		t.Errorf("a short result was folded: %q", short.Messages[1].Content)
	}
}

// The seed-time delivery of the SET method is GONE, and no test replaces it: the method now
// reaches a session on exactly one signal, the batch the CALLER writes (appendBatchProtocol,
// pinned by TestUnseededSetDirectionIsHandedTheMethodOnItsFirstBatch).
//
// Why the seed path was removed rather than kept as a second delivery: it had to decide, before
// the session had done anything, whether the question was an enumeration — from a slot type, which
// cannot tell "how many people did X kill" from "how many times larger is A than B" (measured: the
// shape-only version seeded value questions that merely contain a count, and the run finished 0.833
// with two timeouts against 0.875 with none). The batch tell needs no such guess.

// TestUnseededSetDirectionIsHandedTheMethodOnItsFirstBatch pins the second delivery:
// the CALLER's own batch, appended once, mid-session.
//
// It is the path for a set direction whose table was typed `number` rather than
// `count` — and the signal has no measured false positives: over one FRAMES run of 20
// questions the caller wrote zero batches, while one 三国 question wrote eighteen.
func TestUnseededSetDirectionIsHandedTheMethodOnItsFirstBatch(t *testing.T) {
	method := "SET / COUNT directions — the member list IS the work"
	newSession := func(queries ...string) *SessionState {
		return &SessionState{
			SearchQueries:       queries,
			EnumerationProtocol: method,
			Messages:            []schema.Message{*schema.ToolMessage(`{"passages": []}`, "call_1")},
		}
	}

	// An English question never writes a batch, so it is never handed the method.
	english := newSession("What was the age difference between Mike Tyson and Trevor Berbick")
	english.appendBatchProtocol(true)
	if strings.Contains(english.Messages[0].Content, method) {
		t.Fatalf("tool message = %q, want no method on a question that wrote no batch", english.Messages[0].Content)
	}
	if english.BatchProtocolShown {
		t.Fatal("the method must not be marked shown when it was not appended")
	}

	// A caller-written batch IS the tell (space-separated CJK, the shape the model
	// actually writes — see callerBatch).
	chinese := newSession("关羽 斩 杀 颜良 文丑 华雄 蔡阳")
	chinese.appendBatchProtocol(true)
	if !strings.Contains(chinese.Messages[0].Content, method) {
		t.Fatalf("tool message = %q, want the method appended", chinese.Messages[0].Content)
	}
	// Once per session: method repeated is prompt noise.
	before := chinese.Messages[0].Content
	chinese.appendBatchProtocol(true)
	if chinese.Messages[0].Content != before {
		t.Fatal("the method must be appended at most once")
	}
	// A turn that ran no tool call cannot carry it either.
	quiet := newSession("关羽 斩 颜良")
	quiet.appendBatchProtocol(false)
	if quiet.BatchProtocolShown {
		t.Fatal("a turn that ran no tool call must not carry the method")
	}
}

// TestAppendRecordLineReachesValueDirectionsToo pins the line's reach after the shape gate was
// removed.
//
// It used to be skipped on a value question, on the argument that every field of it is empty
// there (`members=0 | probed-reached=0`). The line also carries `asked-nothing-back` (the probes
// this session already ran and got nothing for) and `FOUND BUT NOT RECORDED` (names a passage
// offered that no patch accounts for) — which is exactly the feedback a multi-hop VALUE session
// needs to take its next hop instead of re-asking what it already knows, and those sessions were
// also the ones stopped at four turns (see actionMaxTurns).
func TestAppendRecordLineReachesValueDirectionsToo(t *testing.T) {
	kb := &Kbinfos{}
	kb.Admit(func(p *PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content": "prose"})
	})
	payload := `{"passages": []}`
	s := &SessionState{
		KB: kb,
		ParentState: State{State: []Variable{
			{ID: 0, Type: "date", Candidate: strPtr("1858")},
			{ID: 1, Type: "number", Candidate: strPtr("2452")},
		}},
		Messages: []schema.Message{*schema.ToolMessage(payload, "call_1")},
	}
	s.appendRecordLine(true)
	got := s.Messages[len(s.Messages)-1].Content
	if !strings.HasPrefix(got, payload) {
		t.Fatalf("tool message = %q, want the payload still first", got)
	}
	if !strings.Contains(got, "[record]") {
		t.Fatalf("tool message = %q, want the record line on a value direction too", got)
	}
	if s.Record.Pool != 1 {
		t.Fatalf("record pool = %d, want the record computed", s.Record.Pool)
	}
}
