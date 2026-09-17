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
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/rag/advanced_rag/slots"
)

// TestCoverageWindowsCentreOnTheAct pins the unit a verdict is asked about: the words
// around the deed, not the passage.
//
// One passage states the deed several times (a chapter lists several victims), and a
// window per occurrence is what lets each victim be named from its own words — the
// alternative, one passage per verdict, is how a three-kill chapter came back as one
// member.
func TestCoverageWindowsCentreOnTheAct(t *testing.T) {
	text := "关公马快，赶上文丑，脑后一刀，将文丑斩下马来。云长手起刀落，斩孔秀于马下。"
	got := coverageWindows(text, []string{"斩"}, 3)
	if len(got) != 2 {
		t.Fatalf("windows = %d, want one per occurrence of the act (%v)", len(got), got)
	}
	if !strings.Contains(got[0].quote, "斩下马来") || !strings.Contains(got[1].quote, "斩孔秀于马下") {
		t.Errorf("windows = %v, want each centred on its own act", got)
	}
	if got[0].act != "斩" || got[1].act != "斩" {
		t.Errorf("acts = %q/%q, want the act word that was found", got[0].act, got[1].act)
	}
	// The cap is the caller's: a chapter that states the deed nine times contributes the
	// first few windows, and the pass-through count says the corpus holds more.
	if got := coverageWindows(text, []string{"斩"}, 1); len(got) != 1 {
		t.Errorf("windows = %d, want the cap respected", len(got))
	}
	// A passage that does not state the deed contributes nothing.
	if got := coverageWindows("话说天下大势，分久必合。", []string{"斩", "杀"}, 3); len(got) != 0 {
		t.Errorf("windows = %v, want nothing from a passage without the act", got)
	}
	if act := coverageFirstAct("云长提华雄之头，掷于地上。", []string{"斩", "杀"}); act != "" {
		t.Errorf("act = %q, want no act in a passage that only reports the aftermath", act)
	}
}

// TestResolveCoverageAsksEveryWindowInBatches pins the enumeration's LAST node: every
// window is asked about, a failed batch is counted as UNKNOWN rather than read as an
// absence, a member's citation comes from the LINE, and one source's two spellings of one
// entity are one member.
//
// The single call it replaces loses a whole enumeration's members to one timeout: every
// candidate in one prompt, one clock shared by all of them.
func TestResolveCoverageAsksEveryWindowInBatches(t *testing.T) {
	set := CoverageSet{}
	for i := 0; i < 9; i++ {
		set.Windows = append(set.Windows, CoverageWindow{ChunkID: fmt.Sprintf("c%d", i), Quote: "云长斩之", Act: "斩"})
	}
	model := &stubCoverageModel{reply: func(user string) string {
		// The first batch answers, the second fails: the difference must show up as UNKNOWN.
		if strings.Contains(user, "chunk_id=c8") {
			return ""
		}
		return `{"members": [{"i": 0, "name": "孔秀"}, {"i": 1, "name": "mona lisa"}, {"i": 2, "name": "Mona Lisa"}], "not_members": [3]}`
	}}
	model.failIf = func(user string) bool { return strings.Contains(user, "chunk_id=c8") }

	members, stats := ResolveCoverage(context.Background(), model, "关羽杀了多少有姓名的人物？", Coverage{Actor: "关羽"}, set)
	// 2 batches carry the first pass (9 windows, batch of 8), and the 5 windows left without a
	// verdict are re-asked once per remaining pass (see coverageResolvePasses).
	if want := 2 + (coverageResolvePasses - 1); stats.Batches != want || stats.Asked != 9 {
		t.Fatalf("stats = %+v, want %d batch(es): 9 windows asked about, and the 5 that got no verdict asked again", stats, want)
	}
	// WHICH passages nobody read travels with the count: a reader can check the ids.
	if got := strings.Join(stats.Unjudged, ","); got != "c4,c5,c6,c7,c8" {
		t.Fatalf("unjudged = %q, want the ids of the windows that got no verdict", got)
	}
	// 4 of the 9 windows got a verdict (3 members + 1 not-a-member); the failed batch's
	// window AND the four lines the model skipped stay UNKNOWN, not absent. The re-ask puts
	// them in front of the model again, but a call that fails every time cannot judge them —
	// so the budget has to be the caller's to report, which is what the counters are for.
	// One failed call per pass over the batch that always fails (the first pass's batch plus one
	// per re-ask pass), so the counter tracks CALLS, as its contract says.
	if stats.Failed != coverageResolvePasses || stats.Answered != 4 || stats.Unknown != 5 {
		t.Fatalf("stats = %+v, want the failed batch and the skipped lines UNKNOWN, not absent", stats)
	}
	if len(members) != 2 {
		t.Fatalf("members = %+v, want the three named lines deduped to two entities", members)
	}
	if members[0].Name != "孔秀" || members[0].ChunkID != "c0" {
		t.Errorf("member = %+v, want the line's own chunk id as its citation", members[0])
	}
	if strings.ToLower(members[1].Name) != "mona lisa" {
		t.Errorf("member = %+v, want one member for two spellings", members[1])
	}
}

// TestResolveCoverageReasksTheWindowsWithoutAVerdict pins the CLOSURE of the point-of-naming
// step: a line the reply left out is asked again, and work that already has a verdict is
// spared.
//
// The member set has to be a function of the corpus, not of which call came back. A reply that
// answers some of its lines and omits the rest leaves those members unjudged, and a pass that
// stopped there left windows that were RECALLED and SHOWN ending up in no list at all — the
// count then moves with the provider's latency rather than with the source.
func TestResolveCoverageReasksTheWindowsWithoutAVerdict(t *testing.T) {
	set := CoverageSet{}
	for i := 0; i < 3; i++ {
		set.Windows = append(set.Windows, CoverageWindow{ChunkID: fmt.Sprintf("c%d", i), Quote: "云长斩之", Act: "斩"})
	}
	calls := 0
	model := &stubCoverageModel{reply: func(string) string {
		calls++
		if calls == 1 {
			// The first call answers ONE of its three lines and omits the other two.
			return `{"members": [{"i": 0, "name": "孔秀"}], "not_members": []}`
		}
		// The line numbers are BATCH-LOCAL: this call carries the two lines nobody judged.
		return `{"members": [], "not_members": [0, 1]}`
	}}

	members, stats := ResolveCoverage(context.Background(), model, "关羽杀了多少有姓名的人物？", Coverage{Actor: "关羽"}, set)
	if stats.Asked != 3 || stats.Answered != 3 || stats.Unknown != 0 {
		t.Fatalf("stats = %+v, want every window judged after the re-ask", stats)
	}
	if stats.Batches != 2 {
		t.Errorf("Batches = %d, want the two unjudged lines asked again in a second call", stats.Batches)
	}
	if len(members) != 1 || members[0].Name != "孔秀" || members[0].ChunkID != "c0" {
		t.Errorf("members = %+v, want the one member the first call named, cited by its line", members)
	}
}

// TestResolveCoverageWithoutAWindowCostsNothing pins the cheap path: an enumeration that
// found nothing asks nothing.
func TestResolveCoverageWithoutAWindowCostsNothing(t *testing.T) {
	model := &stubCoverageModel{reply: func(string) string { return "{}" }}
	members, stats := ResolveCoverage(context.Background(), model, "q", Coverage{}, CoverageSet{})
	if len(members) != 0 || stats.Asked != 0 || stats.Batches != 0 {
		t.Errorf("members=%v stats=%+v, want no call and nothing named", members, stats)
	}
	if len(model.prompts) != 0 {
		t.Errorf("model was asked %d time(s), want none", len(model.prompts))
	}
}

// stubCoverageModel is a SessionModel that answers with a canned reply, and can fail one
// batch so the UNKNOWN path is exercised.
type stubCoverageModel struct {
	mu      sync.Mutex
	prompts []string
	failIf  func(user string) bool
	reply   func(user string) string
}

func (m *stubCoverageModel) Complete(_ context.Context, messages []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	user := ""
	if len(messages) > 0 {
		user = messages[len(messages)-1].Content
	}
	m.mu.Lock()
	m.prompts = append(m.prompts, user)
	m.mu.Unlock()
	if m.failIf != nil && m.failIf(user) {
		return nil, fmt.Errorf("stub: transport failed")
	}
	return &ModelReply{Content: m.reply(user)}, nil
}

// TestEnumerateCoverageKeepsWhatTheRecallReturned pins the enumeration's recall loop: a
// passage the recall came back with IS the evidence, and nothing here is a clock
// exhaustion.
//
// GrepSearch's second value is its DOC AGGREGATIONS, not an error, and DocAggs builds that
// slice with make() — so it is non-nil whether or not anything came back. Read as an error,
// TestCoverageActsMeetActorPinsTheEnumerationFilter pins the probe against the SAME conjunction the
// enumeration's window builder uses: an act word AND, when the direction names one, the actor.
func TestCoverageActsMeetActorPinsTheEnumerationFilter(t *testing.T) {
	kb := &Kbinfos{}
	kb.Admit(func(p *PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content_with_weight": "Colin Beashel sailed the Soling class."})
		p.Add(map[string]any{"chunk_id": "c2", "content_with_weight": "Their partner was crewing that year."})
	})
	cov := Coverage{Acts: []string{"crewing"}}
	if !CoverageActsMeetActor(kb, cov) {
		t.Fatal("want true: an act word is held and no actor was declared")
	}
	withActor := cov
	withActor.Actor = "Colin Beashel"
	if CoverageActsMeetActor(kb, withActor) {
		t.Fatal("want false: the two words sit in DIFFERENT passages, so no window could carry both")
	}
	kb.Admit(func(p *PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c3", "content_with_weight": "Colin Beashel was crewing the Soling."})
	})
	if !CoverageActsMeetActor(kb, withActor) {
		t.Fatal("want true once ONE passage carries both")
	}
}

// TestCoverageOfKeepsTheDeclaredReadingNarrow pins the split the FRAMES regression forced: the value
// may declare the direction to the node that runs LAST, and to nothing else.
//
// Set is what the planner said, and two readers depend on that being all it is — the record's set
// block and a session's parentSet. Read from the value instead, a table whose slots happen to hold
// people reads as an enumeration: measured (2026-09-17, FRAMES q759) a one-person question's record
// lost its own draft answer and gained a member count to state.
func TestCoverageOfKeepsTheDeclaredReadingNarrow(t *testing.T) {
	items := slots.Items(slots.Item{Value: "Lanee Butler", ChunkID: "c1", Quote: "Mistral (sailboard) | Lanee Butler"})
	valueOnly := NewState([]Variable{
		{ID: 0, Type: "person", Terms: []string{"斩"}, Value: &items},
	}, 0, nil)
	cov := CoverageOf(valueOnly)
	if cov.Set || cov.Ok() {
		t.Fatalf("set=%v ok=%v, want the DECLARED reading still false for a person slot", cov.Set, cov.Ok())
	}
	if !cov.ItemsInValue() {
		t.Fatal("itemsInValue = false, want the value that holds items readable by the node with no plan-time word")
	}
	if cov.ItemKind != "person" {
		t.Fatalf("itemKind = %q, want the planner's word kept — the value only names the kind when the type did not", cov.ItemKind)
	}
	// A slot whose type names no kind still gets one from the value.
	nameless := NewState([]Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Value: &items},
	}, 0, nil)
	if cov := CoverageOf(nameless); cov.ItemKind != CoverageItemKindItems || !cov.ItemsInValue() {
		t.Fatalf("cov = %+v, want the value to name the kind when the type did not", cov)
	}

	// A declared shape is untouched: the planner's word still opens the gate.
	declared := NewState([]Variable{
		{ID: 0, Type: "list", Terms: []string{"斩"}},
	}, 0, nil)
	if cov := CoverageOf(declared); !cov.Ok() || cov.ItemsInValue() {
		t.Fatalf("cov = %+v, want a declared list slot to open the gate on the declaration alone", cov)
	}
}

// the branch fires on every operand: Truncated is set, every recall's passages are dropped,
// and the rendered seed tells each session the direction came back empty. Nothing caught
// that until this test, because the loop had no test of its own.
func TestEnumerateCoverageKeepsWhatTheRecallReturned(t *testing.T) {
	deps, kb := newTestSearchDeps(&stubRetriever{chunks: []map[string]any{
		{"chunk_id": "c1", "content": "关羽手起刀落，斩孔秀于马下。", "doc_id": "d1"},
	}})
	cov := Coverage{Actor: "关羽|关公", Acts: []string{"斩"}, Set: true, ItemKind: "person"}
	if !cov.Ok() {
		t.Fatalf("the fixture must be an enumeration: %+v", cov)
	}

	set := EnumerateCoverage(context.Background(), deps, cov, kb)
	if set.Recalled == 0 {
		t.Fatalf("Recalled = 0 over %d operand(s): the recalled passage(s) were dropped", len(set.Operands))
	}
	if len(set.Windows) == 0 {
		t.Fatalf("Windows = 0: a passage naming the actor and the act word is a window (%+v)", set)
	}
	if set.Truncated {
		t.Error("Truncated = true: nothing here ran out of clock, and saying so seeds every session with (nothing)")
	}
	if !strings.Contains(set.Windows[0].Quote, "斩孔秀") {
		t.Errorf("window = %q, want the words around the act", set.Windows[0].Quote)
	}
	if !strings.Contains(set.Render(), "chunk_id=c1") {
		t.Errorf("seed = %q, want the window's chunk id (a member is citable only through it)", set.Render())
	}
}
