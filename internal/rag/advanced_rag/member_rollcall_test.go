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
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/advanced_rag/slots"
)

// rollCallModel answers with one canned reply and counts the calls, so a test can assert that the
// roll call did NOT spend a call when it had nothing to ask.
type rollCallModel struct {
	calls int
	reply string
}

func (m *rollCallModel) Complete(_ context.Context, _ []schema.Message, _ []harness.ToolSpec) (*harness.ModelReply, error) {
	m.calls++
	return &harness.ModelReply{Content: m.reply}, nil
}

func memberSlotValue(names ...string) *slots.Value {
	items := make([]slots.Member, 0, len(names))
	for _, n := range names {
		items = append(items, slots.Member{Name: n, ChunkID: "seed-" + n})
	}
	v := slots.Members(items...)
	return &v
}

// TestRollCallWritesWhatTheModelNames pins the write-back the runtime owns: the candidates are the
// evidence the run ALREADY holds (the completeness pass's windows, and the names a probe reached),
// the model answers about every one of them, and the members land in the table whatever a session
// did or forgot.
//
// Measured (2026-09-16, 三国/关羽): runs of one build answered 18 / 16 / 15 / 14 / 12 members with the
// same corpus in hand — the same eleven names every time plus a different handful of the other nine —
// because what varied was what a session patched.
func TestRollCallWritesWhatTheModelNames(t *testing.T) {
	kb := &harness.Kbinfos{}
	kb.StorePatternFindings("## The completeness pass ALREADY RAN for this direction\n\n" +
		"- 关羽.*斩 →\n" +
		"    chunk_id=w1  \"云长手起刀落，斩杨龄于马下\"\n" +
		"    chunk_id=w2  \"关公所历关隘五处，斩将六员\"\n")
	// A name a probe reached, with the passage that carries it: a member with its evidence,
	// whatever the session did with it.
	kb.Admit(func(p *harness.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "w3", "content_with_weight": "只一合被云长砍死"})
	})
	kb.RecordReachedTerm("夏侯存", "w3")

	table := harness.NewState([]harness.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽|云长"},
		{ID: 1, Type: "dataset", Value: memberSlotValue("华雄"), Candidate: strPtr("华雄"), CandidateStrength: testFloatPtr(0.9)},
	}, 0, nil)
	st := &AgenticState{Question: "三国演义中，关羽杀了多少有姓名的人物？", KB: kb, SlotTable: table}
	model := &rollCallModel{reply: `{"members": [{"i": 0, "name": "杨龄"}, {"i": 2, "name": "夏侯存"}], "not_members": [1]}`}
	deps := RAGTools{Model: model}
	var buf bytes.Buffer

	res := RollCallMembers(context.Background(), deps, st, log.New(&buf, "", 0))

	if model.calls != 1 {
		t.Fatalf("model calls = %d, want exactly one", model.calls)
	}
	if res.Asked != 3 || res.Answered != 3 || res.Written != 2 {
		t.Errorf("roll call = asked %d / answered %d / written %d, want 3 / 3 / 2", res.Asked, res.Answered, res.Written)
	}
	got := map[string]bool{}
	for _, n := range memberUnion(&st.SlotTable) {
		got[n] = true
	}
	for _, want := range []string{"华雄", "杨龄", "夏侯存"} {
		if !got[want] {
			t.Errorf("members %v, want %q — the roll call's verdicts plus what the slot already held", got, want)
		}
	}
	// The chunk id comes from the LINE the model judged, so a citation cannot be invented.
	for _, v := range st.SlotTable.State {
		if v.ID != 1 {
			continue
		}
		byName := map[string]string{}
		for _, m := range v.Typed().Items {
			byName[m.Name] = m.ChunkID
		}
		if byName["杨龄"] != "w1" || byName["夏侯存"] != "w3" {
			t.Errorf("member evidence = %v, want each name cited by the line it was judged on", byName)
		}
	}
	// The count is derived from the members, so it has to follow them, and the record — which the
	// answer reads — must show what was just written.
	if c := st.SlotTable.State[0].Candidate; c == nil || *c != "3" {
		t.Errorf("count slot = %v, want 3 (the enumerated members' size)", c)
	}
	if !strings.Contains(kb.Record, "杨龄") {
		t.Errorf("record %q does not mention the member the roll call wrote", kb.Record)
	}
	if !strings.Contains(buf.String(), "3 candidate(s) asked, 3 answered, 2 member(s) written") {
		t.Errorf("log = %q, want the accounting line", buf.String())
	}
}

// TestRollCallIgnoresVerdictsForLinesItNeverHandedOver pins the one thing a reply cannot do: cite a
// passage of its own choosing. A verdict names a LINE, and a line number outside the list is dropped.
func TestRollCallIgnoresVerdictsForLinesItNeverHandedOver(t *testing.T) {
	kb := &harness.Kbinfos{}
	kb.StorePatternFindings("- 关羽.*斩 →\n    chunk_id=w1  \"云长手起刀落，斩杨龄于马下\"\n")
	table := harness.NewState([]harness.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽"},
		{ID: 1, Type: "dataset", Value: memberSlotValue("华雄"), Candidate: strPtr("华雄")},
	}, 0, nil)
	st := &AgenticState{Question: "关羽杀了多少有姓名的人物？", KB: kb, SlotTable: table}
	deps := RAGTools{Model: &rollCallModel{
		reply: `{"members": [{"i": 9, "name": "张凯"}, {"i": 0, "name": "杨龄"}], "not_members": [7]}`,
	}}

	res := RollCallMembers(context.Background(), deps, st, log.New(&bytes.Buffer{}, "", 0))
	if res.Asked != 1 || res.Answered != 1 || res.Written != 1 {
		t.Errorf("roll call = asked %d / answered %d / written %d, want 1 / 1 / 1", res.Asked, res.Answered, res.Written)
	}
	for _, n := range memberUnion(&st.SlotTable) {
		if n == "张凯" {
			t.Error("a verdict for a line that was never handed over was written into the table")
		}
	}
}

// TestRollCallSkipsWhenThereIsNothingToAsk pins that the step costs nothing when the evidence and the
// ledger have nothing the table does not already mention: no candidates, no call.
func TestRollCallSkipsWhenThereIsNothingToAsk(t *testing.T) {
	kb := &harness.Kbinfos{}
	table := harness.NewState([]harness.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽"},
		{ID: 1, Type: "dataset", Value: memberSlotValue("华雄"), Candidate: strPtr("华雄")},
	}, 0, nil)
	st := &AgenticState{Question: "关羽杀了多少有姓名的人物？", KB: kb, SlotTable: table}
	model := &rollCallModel{reply: `{"members": [], "not_members": []}`}

	if res := RollCallMembers(context.Background(), RAGTools{Model: model}, st, log.New(&bytes.Buffer{}, "", 0)); res.Asked != 0 {
		t.Errorf("asked = %d, want 0", res.Asked)
	}
	if model.calls != 0 {
		t.Errorf("model calls = %d, want none: an empty roll call must not spend a call", model.calls)
	}
}

// TestRollCallMembersOutrankAProseList pins what happens when the slot the members go into holds
// PROSE: the accountable list wins, because every name in it carries the passage that names it (a
// prose claim contributes no member and therefore no count at all — see memberUnion).
func TestRollCallMembersOutrankAProseList(t *testing.T) {
	kb := &harness.Kbinfos{}
	kb.StorePatternFindings("- 关羽.*斩 →\n    chunk_id=w1  \"云长手起刀落，斩杨龄于马下\"\n")
	prose := "据网文统计共14人：华雄、颜良、文丑……"
	table := harness.NewState([]harness.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽"},
		{ID: 1, Type: "dataset", Candidate: &prose, CandidateStrength: testFloatPtr(0.95)},
	}, 0, nil)
	st := &AgenticState{Question: "关羽杀了多少有姓名的人物？", KB: kb, SlotTable: table}
	deps := RAGTools{Model: &rollCallModel{reply: `{"members": [{"i": 0, "name": "杨龄"}], "not_members": []}`}}

	res := RollCallMembers(context.Background(), deps, st, log.New(&bytes.Buffer{}, "", 0))
	if res.Written != 1 {
		t.Fatalf("written = %d, want 1", res.Written)
	}
	v := st.SlotTable.State[1]
	if v.Typed().Kind != slots.KindMembers {
		t.Errorf("slot holds %v, want the member list the roll call verified", v.Typed().Kind)
	}
	if names := v.Typed().Names(); len(names) != 1 || names[0] != "杨龄" {
		t.Errorf("slot members = %v, want 杨龄", names)
	}
	// The prose is not thrown away: it survives as an alternate, exactly as a losing claim does.
	if !strings.Contains(strings.Join(v.DiscoveredClues, "|"), "14人") {
		t.Errorf("clues = %v, want the prose claim kept as an alternate", v.DiscoveredClues)
	}
}

func testFloatPtr(f float64) *float64 { return &f }
