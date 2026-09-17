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
// The single call it replaces is measured (2026-09-16, 三国/关羽): 28 candidates in one
// prompt, a 30s clock, `asked=28 answered=0` — a whole enumeration's members lost to one
// timeout.
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
	if stats.Batches != 2 || stats.Asked != 9 {
		t.Fatalf("stats = %+v, want 9 windows asked about in 2 batches", stats)
	}
	// 4 of the 9 windows got a verdict (3 members + 1 not-a-member); the failed batch's
	// window AND the four lines the model skipped are UNKNOWN, not absent.
	if stats.Failed != 1 || stats.Answered != 4 || stats.Unknown != 5 {
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
