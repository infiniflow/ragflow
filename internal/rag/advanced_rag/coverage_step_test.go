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
	"strings"
	"testing"

	"ragflow/internal/rag/advanced_rag/harness"
	"ragflow/internal/rag/advanced_rag/slots"
)

// TestCoverageResolveCandidatesJudgeWhatTheRunTouched pins the naming node's TWO sources: the
// enumeration's windows, and the names a probe already REACHED.
//
// A reached name has the pool passage that carries it, so it is a member with its evidence
// whatever the session did with it afterwards. A node that judges only the enumeration's own
// windows drops exactly those names: the run holds the passage, names it nowhere, and the count
// comes out short of what the corpus states.
func TestCoverageResolveCandidatesJudgeWhatTheRunTouched(t *testing.T) {
	kb := &harness.Kbinfos{}
	kb.Admit(func(p *harness.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content_with_weight": "夏侯存军至，见了云长，被云长砍死。"})
	})
	kb.RecordReachedTerm("夏侯存", "c1")
	kb.RecordReachedTerm("文丑", "c1")      // a slot already declares it: not asked again
	kb.RecordReachedTerm("杨龄", "nowhere") // no passage in the pool: no words behind it
	kb.StoreCoverageSet(harness.CoverageSet{
		Operands: []string{"云长", "斩"},
		Windows:  []harness.CoverageWindow{{ChunkID: "w1", Quote: "云长手起刀落，斩孔秀于马下"}},
	})
	value := slots.Members(slots.Member{Name: "文丑", ChunkID: "c9", Quote: "云长诛文丑"})
	table := harness.NewState([]harness.Variable{{ID: 0, Type: "person", Value: &value}}, 0, nil)

	set, added := coverageResolveCandidates(kb, &table)
	if added != 1 || len(set.Windows) != 2 {
		t.Fatalf("candidates = %d window(s) (+%d reached), want the enumeration's window plus the one reached name",
			len(set.Windows), added)
	}
	if got := set.Windows[1]; got.ChunkID != "c1" || !strings.Contains(got.Quote, "夏侯存") {
		t.Errorf("line = %+v, want the reached name with the words behind it", got)
	}
	for _, w := range set.Windows {
		if strings.Contains(w.Quote, "文丑") {
			t.Errorf("line = %+v, want a name a slot already declares left out", w)
		}
		if strings.TrimSpace(w.Quote) == "" || w.ChunkID == "" {
			t.Errorf("line = %+v, want both the passage and its words (a bare name cannot be cited)", w)
		}
	}
}

// TestCoverageResolveCandidatesSkipWhatIsAlreadyQuoted pins the dedup that keeps the node from
// asking about one passage twice: a window of the same chunk already carries those words.
func TestCoverageResolveCandidatesSkipWhatIsAlreadyQuoted(t *testing.T) {
	kb := &harness.Kbinfos{}
	kb.Admit(func(p *harness.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content_with_weight": "云长提华雄之头，掷于地上，其酒尚温。"})
	})
	kb.RecordReachedTerm("华雄", "c1")
	kb.StoreCoverageSet(harness.CoverageSet{
		Windows: []harness.CoverageWindow{{ChunkID: "c1", Quote: "云长提华雄之头，掷于地上"}},
	})

	set, added := coverageResolveCandidates(kb, nil)
	if added != 0 || len(set.Windows) != 1 {
		t.Fatalf("candidates = %d window(s) (+%d reached), want the window that already quotes it, once",
			len(set.Windows), added)
	}
}

// TestRunCoverageResolveWritesWhatTheRunTouched pins the WIRING the unit tests above cannot: the
// node's candidate set is the UNION, so a name no enumeration recalled but a probe DID reach is
// still judged, and still written.
//
// This is the case that loses members: the corpus states the deed, the run's own search brought
// the passage back, and the name lands in no list at all because the node only ever looked at its
// own enumeration.
func TestRunCoverageResolveWritesWhatTheRunTouched(t *testing.T) {
	st := NewAgenticState("关羽杀了多少有姓名的人物？", "", 3, nil)
	st.KB.Chunks = []map[string]any{
		{"chunk_id": "c1", "content": "夏侯存军至，见了云长，大怒，便与云长交锋，只一合，被云长砍死。"},
	}
	// The run's own probe reached this name; no enumeration ever recalled it (no set stored).
	st.KB.RecordReachedTerm("夏侯存", "c1")
	declared := slots.Members(slots.Member{Name: "孔秀", ChunkID: "c2", Quote: "云长斩孔秀于马下"})
	st.SlotTable = harness.NewState([]harness.Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽"},
		{ID: 1, Type: "person", Value: &declared},
	}, 0, nil)

	model := &scriptedModel{}
	model.push(`{"members": [{"i": 0, "name": "夏侯存"}], "not_members": []}`)
	stats := RunCoverageResolve(context.Background(), RAGTools{Model: model}, st, nil)

	if stats.Asked != 1 {
		t.Fatalf("asked = %d, want the ONE line the run held — the name its own search reached", stats.Asked)
	}
	if stats.Answered != 1 || stats.Unknown != 0 {
		t.Errorf("stats = %+v, want that line judged", stats)
	}
	names := harness.MemberNames(&st.SlotTable)
	found := false
	for _, name := range names {
		if strings.EqualFold(strings.TrimSpace(name), "夏侯存") {
			found = true
		}
	}
	if !found {
		t.Errorf("members = %v, want the reached name written into the table", names)
	}
	if !strings.Contains(st.KB.Record, "夏侯存") {
		t.Errorf("record does not carry the member the resolve just proved:\n%s", st.KB.Record)
	}
}
