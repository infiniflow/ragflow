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
	value := slots.Items(slots.Item{Value: "文丑", ChunkID: "c9", Quote: "云长诛文丑"})
	table := harness.NewState([]harness.Variable{{ID: 0, Type: "person", Value: &value}}, 0, nil)

	set, added, claims := coverageResolveCandidates(kb, &table, harness.Coverage{})
	if added != 1 || len(set.Windows) != 2 || claims != 0 {
		t.Fatalf("candidates = %d window(s) (+%d reached, %d claimed), want the enumeration's window plus the one reached name",
			len(set.Windows), added, claims)
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

	set, added, claims := coverageResolveCandidates(kb, nil, harness.Coverage{})
	if added != 0 || len(set.Windows) != 1 || claims != 0 {
		t.Fatalf("candidates = %d window(s) (+%d reached, %d claimed), want the window that already quotes it, once",
			len(set.Windows), added, claims)
	}
}

// TestCoverageResolveCandidatesReopenAClaimNobodyJudged pins what "already known" means: JUDGED,
// not written down.
//
// A name a session wrote with no passage has been asserted, not ruled on, so it stays a candidate
// for the point-of-naming node whenever the run holds a passage about it. Treating the claim as
// settled is how 于禁 (taken at 水淹七军 and released, never killed) stayed in the table
// unexamined while the passage that refutes it sat in the pool; once the name carries a passage,
// the same call leaves it alone.
func TestCoverageResolveCandidatesReopenAClaimNobodyJudged(t *testing.T) {
	kb := &harness.Kbinfos{}
	kb.Admit(func(p *harness.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "c1", "content_with_weight": "于禁拜伏于地，乞哀请命。关公曰：汝怎敢抗吾？"})
	})
	kb.RecordReachedTerm("于禁", "c1")
	claimed := slots.Items(slots.Item{Value: "于禁"})
	table := harness.NewState([]harness.Variable{{ID: 1, Type: "person", Value: &claimed}}, 0, nil)

	set, added, claims := coverageResolveCandidates(kb, &table, harness.Coverage{})
	if added != 1 || len(set.Windows) != 1 {
		t.Fatalf("candidates = %d window(s) (+%d), want the unjudged claim handed to a verdict",
			len(set.Windows), added)
	}
	// Reached, not claimed: the ledger already knew this name, so the claim source left it alone
	// rather than asking about it twice.
	if claims != 0 {
		t.Fatalf("claimed = %d, want the claim covered by the reached ledger", claims)
	}
	anchored := slots.Items(slots.Item{Value: "于禁", ChunkID: "c1", Quote: "于禁拜伏于地"})
	settled := harness.NewState([]harness.Variable{{ID: 1, Type: "person", Value: &anchored}}, 0, nil)
	if _, again, _ := coverageResolveCandidates(kb, &settled, harness.Coverage{}); again != 0 {
		t.Fatalf("added = %d, want a member that already carries a passage left alone", again)
	}
}

// TestCoverageResolveCandidatesHandAClaimToTheVerdict pins the THIRD source: a name a session
// asserted with no passage is handed over as a candidate, with whatever the pool can show about
// it, so the node that decides what a member is gets to decide about it too.
//
// The alternatives both lose. Left in the table, the claim is counted without ever being read
// (于禁 taken at 水淹七军 and released, never killed, counted beside sixteen real members). Dropped
// outright, a name the corpus does state is lost. Handed over, it is a candidate like any other —
// and the passage that carries the deed is offered first, because a name is mentioned in many
// places and only one of them is the killing.
func TestCoverageResolveCandidatesHandAClaimToTheVerdict(t *testing.T) {
	kb := &harness.Kbinfos{}
	kb.Admit(func(p *harness.PoolAdmitter) {
		p.Add(map[string]any{"chunk_id": "m1", "content_with_weight": "关公问曰：谁人敢去？于禁在阵前绰刀出马，关公便拨马回阵。"})
		p.Add(map[string]any{"chunk_id": "k1", "content_with_weight": "于禁拜伏于地，乞哀请命。关公大怒曰：汝竟敢抗吾？喝令推出斩之。"})
	})
	// 于禁 has passages in the pool; 刘延 does not (in this fixture): one claim can be judged,
	// the other has nothing to be judged against and must not be invented into a list.
	claimed := slots.Items(slots.Item{Value: "于禁"}, slots.Item{Value: "刘延"})
	table := harness.NewState([]harness.Variable{{ID: 1, Type: "person", Value: &claimed}}, 0, nil)

	set, reached, claims := coverageResolveCandidates(kb, &table, harness.Coverage{Acts: []string{"斩"}, Actor: "关羽"})
	if reached != 0 || claims != 1 {
		t.Fatalf("reached=%d claimed=%d, want exactly one claim handed over", reached, claims)
	}
	if len(set.Windows) != 2 || set.Windows[0].ChunkID != "k1" {
		t.Fatalf("windows = %+v, want the passage carrying the deed offered first", set.Windows)
	}
	// The choice is marked, not silent: the window says which act word the passage carries, so a
	// reader of the log can see why it was offered first (see CoverageWindow.Act).
	if set.Windows[0].Act != "斩" {
		t.Fatalf("windows[0].Act = %q, want the act word the passage carries", set.Windows[0].Act)
	}
	if set.Windows[1].Act != "" {
		t.Fatalf("windows[1].Act = %q, want no act word claimed for a passage that carries none", set.Windows[1].Act)
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
	declared := slots.Items(slots.Item{Value: "孔秀", ChunkID: "c2", Quote: "云长斩孔秀于马下"})
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
	names := harness.ItemValues(&st.SlotTable)
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
