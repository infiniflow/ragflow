package harness

import (
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/rag/advanced_rag/slots"
)

// typedMembersSlot / typedCountSlot build the slots the record reads: a slot holds
// members or a number because the model DECLARED it (see package slots), so these are
// what a patch now produces.
func typedMembersSlot(id int, typ string, names ...string) Variable {
	items := make([]slots.Member, 0, len(names))
	for _, n := range names {
		items = append(items, slots.Member{Name: n})
	}
	v := slots.Members(items...)
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
		Messages: []schema.Message{*schema.ToolMessage(`{"passages": []}`, "call_1")},
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
	if got := s.turnRunCap(); got != 8 {
		t.Fatalf("run cap = %d, want 8 (the mode's floor 4 + the model's %d)", got, turnRunExtra)
	}
	if !s.offerContinuation() {
		t.Fatal("past the floor the session must offer the model another turn")
	}
	last := s.Messages[len(s.Messages)-1]
	if last.Role != schema.User {
		t.Fatalf("offer role = %v, want a user message the model answers", last.Role)
	}
	for _, want := range []string{"TURN BUDGET", "up to 8", "4 turn(s) left", "state patch NOW"} {
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

	// The hard cap: no offer, and the session finalizes.
	capped := enumerationSession(8, 90)
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
		ParentState:  State{State: []Variable{{ID: 0, Type: "count", Candidate: strPtr("12")}}},
	}
}

// TestOfferContinuationIsGatedOnTheQuestionsShape pins the cost rule. The offer is
// an extra model call plus up to turnRunExtra more turns, and a question that is
// not assembling a set has nothing for those turns to find: measured on
// 2026-09-15, one enumeration question was offered four extra turns while the
// sessions still recorded nothing, and every question in the mode paid for that
// mechanism. The gate is the session ENUMERATING — the caller wrote a batch, or the
// planner declared a count/list — not a mode flag, and not a candidate's separators:
// measured the same day, with the gate reading a list-shaped CANDIDATE as a set, a
// FRAMES run took 24 offers (and 8 extra rounds over its baseline) on questions
// whose answer is one number.
func TestOfferContinuationIsGatedOnTheQuestionsShape(t *testing.T) {
	value := &SessionState{
		Attempts:     4,
		DeadlineLeft: 90,
		ParentState:  State{State: []Variable{{ID: 0, Type: "entity", Candidate: strPtr("白马坡")}}},
	}
	if value.offerContinuation() {
		t.Fatal("a single-value question must not be offered extra turns")
	}
	if len(value.Messages) != 0 {
		t.Fatal("no offer message may be appended for a value question")
	}
	// And the route at the floor finalizes it, exactly as the mode's turn count
	// alone used to.
	if got := value.route(); got != routeFinalize {
		t.Fatalf("route at the floor on a value question = %v, want routeFinalize", got)
	}

	// A candidate that only LOOKS like a list is not a set: the measured FRAMES
	// table held `Grace's、High、Falls、Colonial、Creek` — one waterfall's name cut at
	// its separators — under a slot typed "dataset", and the offer built on it is
	// what ran that benchmark eight rounds long.
	prose := &SessionState{
		Attempts:     4,
		DeadlineLeft: 90,
		ParentState:  State{State: []Variable{{ID: 0, Type: "dataset", Candidate: strPtr("Grace's、High、Falls")}}},
	}
	if prose.offerContinuation() {
		t.Fatal("a separator-bearing candidate under a scalar type must not buy extra turns")
	}

	// A count-typed slot is the other half of the tell — the shape the 三国
	// enumeration direction's own table carries (slot 0 [count]).
	counting := &SessionState{
		Attempts:     4,
		DeadlineLeft: 90,
		ParentState:  State{State: []Variable{{ID: 0, Type: "count", Candidate: strPtr("10")}}},
	}
	if !counting.offerContinuation() {
		t.Fatal("a count slot must be offered the turn")
	}

	// And the tell that survives contact: a caller-written batch, even on a table
	// whose slots say nothing about a set.
	batched := &SessionState{
		Attempts:      4,
		DeadlineLeft:  90,
		SearchQueries: []string{"关羽 斩 杀 颜良 文丑 华雄 蔡阳"},
		ParentState:   State{State: []Variable{{ID: 0, Type: "entity", Candidate: strPtr("白马坡")}}},
	}
	if !batched.offerContinuation() {
		t.Fatal("a session that wrote a batch is enumerating and must be offered the turn")
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

	capped := enumerationSession(8, 90)
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

// TestSetShapedFollowsThePlannersDeclaration pins the half of the gate that reads the
// table: what the planner TYPED, never what a candidate looks like. A list-shaped
// candidate under a scalar type is prose that happens to contain separators —
// measured (2026-09-15, FRAMES) a slot typed "dataset" carried
// `Grace's、High、Falls、Colonial、Creek`, ONE waterfall's name, and the permissive
// reading of it is how a "how much shorter" record came to say "enumerated
// members: 16" and how a `[count]` slot on a "how many times larger" question once
// bought 24 continuation offers.
func TestSetShapedFollowsThePlannersDeclaration(t *testing.T) {
	value := State{State: []Variable{{ID: 0, Type: "entity", Candidate: strPtr("白马坡")}}}
	if SetShaped(value) {
		t.Error("a single-value table asks for no set")
	}
	counted := State{State: []Variable{{ID: 0, Type: "count", Candidate: strPtr("10")}}}
	if !SetShaped(counted) {
		t.Error("a count slot asks for a set")
	}
	quantity := State{State: []Variable{{ID: 0, Type: "number", Candidate: strPtr("2452 feet")}}}
	if SetShaped(quantity) {
		t.Error("a number slot is a value, not a set")
	}
	prose := State{State: []Variable{{ID: 0, Type: "dataset", Candidate: strPtr("Grace's、High、Falls")}}}
	if SetShaped(prose) {
		t.Error("the table must not be read through a candidate's separators")
	}
}

// TestSetDirectionIsSeededWithTheMethod pins WHERE the enumeration method is delivered — in the
// seed, before the first turn — and, just as important, WHO gets it: only a table that declared the
// whole ENUMERATION shape (see EnumerationShaped).
//
// Its first instruction — propose more candidates than you expect — is a decision taken before the
// first query: measured (2026-09-16, 三国/关羽) the same question answered eighteen members with the
// method in the seed of its `[count]` table and fourteen when it arrived a turn later. But the same
// instruction on a question whose answer is ONE value sends the session looking for members it does
// not need: measured (2026-09-16, FRAMES — 4 questions in flight, 300s deadline) the SetShaped-only
// gate seeded the value questions that merely contain a count and the run finished 0.833 with two
// timeouts against 0.875 with none.
func TestSetDirectionIsSeededWithTheMethod(t *testing.T) {
	loader := StringPromptLoader{"action_set": "SET / COUNT directions — the member list IS the work"}

	// A table that declared count/set/list AND a NAME-carrying slot AND its act words: the
	// enumeration strategy, seeded with the method and the corpus queries those words render into
	// (see ScanPatterns) — the seed is where the session is told to ask the corpus for the act,
	// which is the one thing its memory cannot do.
	declared := State{State: []Variable{
		{ID: 0, Type: "count", Candidate: strPtr("18"), Terms: []string{"斩", "杀"}, Subject: "关羽|云长"},
		{ID: 1, Type: "dataset"},
	}}
	got := setProtocolFor(declared, loader)
	if !strings.Contains(got, "关羽.*斩|云长.*斩") {
		t.Errorf("a direction with declared act words must be seeded with their queries, got %q", got)
	}
	if !strings.Contains(got, "SET / COUNT directions") {
		t.Errorf("the method must travel with them, got %q", got)
	}

	// And now the three ways a table FAILS to be an enumeration, each of which must leave a value
	// question alone. A count with no names: a count of EVENTS, which the planner declares act
	// words for as well ("how many times had Brazil won the World Cup").
	countsOnly := State{State: []Variable{
		{ID: 0, Type: "count", Candidate: strPtr("5"), Terms: []string{"won", "trophy"}, Subject: "Brazil"},
	}}
	if got := setProtocolFor(countsOnly, loader); got != "" {
		t.Errorf("a count of events must not be seeded with the member method, got %q", got)
	}
	// Names with no count/set/list: a question about ONE named thing that also declared act words
	// (the shape a multi-hop value question takes).
	namesOnly := State{State: []Variable{
		{ID: 0, Type: "person", Terms: []string{"wrote", "published"}, Subject: "the writer"},
		{ID: 1, Type: "date"},
	}}
	if got := setProtocolFor(namesOnly, loader); got != "" {
		t.Errorf("a value question holding one name must not be seeded with the member method, got %q", got)
	}
	// A count with no act words: nothing was declared to enumerate.
	counted := State{State: []Variable{{ID: 0, Type: "count", Candidate: strPtr("18")}}}
	if got := setProtocolFor(counted, loader); got != "" {
		t.Errorf("a count with no declared act words must not be seeded, got %q", got)
	}
	// `number` is the planner's label for a measured QUANTITY, and it typed the same
	// question both ways on two runs of 2026-09-16: not a seed trigger, and exactly
	// what the batch path exists for.
	quantity := State{State: []Variable{{ID: 0, Type: "number", Candidate: strPtr("14")}}}
	if got := setProtocolFor(quantity, loader); got != "" {
		t.Errorf("a number-typed table must not be seeded, got %q", got)
	}
	// Nor is a candidate's punctuation a declaration: a list-shaped candidate under a
	// scalar type is how the permissive gate seeded 44 of 67 FRAMES sessions.
	prose := State{State: []Variable{{ID: 1, Type: "entity", Candidate: strPtr("孔秀、孟坦")}}}
	if got := setProtocolFor(prose, loader); got != "" {
		t.Errorf("a list-shaped candidate must not seed the method, got %q", got)
	}
	value := State{State: []Variable{{ID: 0, Type: "date", Candidate: strPtr("1858")}}}
	if got := setProtocolFor(value, loader); got != "" {
		t.Errorf("a single-value direction must not be seeded, got %q", got)
	}
	// A loader that predates the template yields nothing rather than panicking (see
	// loadOptionalPrompt): its sessions run exactly as they did before it existed.
	if got := setProtocolFor(counted, StringPromptLoader{}); got != "" {
		t.Errorf("a loader without action_set must yield nothing, got %q", got)
	}
}

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

// TestAppendRecordLineSkipsValueDirections pins the line's gate: it carries a set's
// to-do list, and on a value question every field of it is empty (`members=0 |
// probed-reached=0`) while it still costs a recomputation and a line of prompt on
// every turn. Measured (2026-09-15, FRAMES): 214 such lines across 20 questions,
// on a benchmark whose baseline run carried none.
func TestAppendRecordLineSkipsValueDirections(t *testing.T) {
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
	if got := s.Messages[len(s.Messages)-1].Content; got != payload {
		t.Fatalf("tool message = %q, want it untouched on a value direction", got)
	}
	// The record itself is still computed and kept: the continuation ask reads it,
	// and only the model-facing line is skipped.
	if s.Record.Pool != 1 {
		t.Fatalf("record pool = %d, want the record still computed", s.Record.Pool)
	}
}
