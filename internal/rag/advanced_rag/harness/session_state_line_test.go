package harness

import (
	"strconv"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestSplitCandidateNames pins the one parsing rule the state line depends on: a
// list candidate reads as its members, and a candidate with no separator is left
// whole — guessing further would invent members.
func TestSplitCandidateNames(t *testing.T) {
	got := SplitCandidateNames("孔秀、孟坦、韩福、卞喜")
	if len(got) != 4 || got[0] != "孔秀" || got[3] != "卞喜" {
		t.Fatalf("list candidate = %v, want 孔秀/孟坦/韩福/卞喜", got)
	}
	got = SplitCandidateNames("华雄,车胄 蔡阳")
	if len(got) != 3 {
		t.Fatalf("mixed separators = %v, want 3 members", got)
	}
	got = SplitCandidateNames("颜良、文丑 等")
	if len(got) != 2 || got[1] != "文丑" {
		t.Fatalf("trailing 等 = %v, want 颜良/文丑", got)
	}
	got = SplitCandidateNames("庞德被周仓生擒，非关羽所杀")
	if len(got) != 1 {
		t.Fatalf("separator-free candidate = %v, want the candidate whole", got)
	}
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
		{ID: 0, Type: "count", Candidate: strPtr("13")},
		{ID: 1, Type: "person", Candidate: strPtr("孔秀、孟坦、荀正")},
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
			{ID: 0, Type: "count", Candidate: strPtr("13")},
			{ID: 1, Type: "person", Candidate: strPtr("孔秀、孟坦")},
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
// The cap is the part the runtime keeps: medium/high run 4 → 8, which is the
// "maximum run" the session may never exceed however eager the model is.
func TestOfferContinuationLetsTheModelDecide(t *testing.T) {
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
// mechanism. The gate is the TABLE (a count/number slot, or a list candidate),
// not a mode flag, so a single-value question runs on the mode's floor exactly as
// it did before the mechanism existed.
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

	// A candidate that IS a list is the same tell as a count-typed slot.
	set := &SessionState{
		Attempts:     4,
		DeadlineLeft: 90,
		ParentState:  State{State: []Variable{{ID: 0, Type: "entity", Candidate: strPtr("孔秀、孟坦")}}},
	}
	if !set.offerContinuation() {
		t.Fatal("a table that already holds a list must be offered the turn")
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
