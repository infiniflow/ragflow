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

package runtime

import (
	"strings"
	"testing"

	"ragflow/internal/rag/agentic-rag/slots"
)

// TestAnchoredItemsSplitWhatTheTableCanQuote pins the split the count reads: a member carrying
// the chunk that states it is countable, a member without one is a claim. The split reads the
// ChunkID — the anchor the answer cites — not the Quote, because a quotation with no passage to
// look it up in is words nobody can check.
//
// Neither reading deletes a name: MemberNames still holds all three, which is what lets the record
// list the claims beside the number instead of dropping them.
func TestAnchoredItemsSplitWhatTheTableCanQuote(t *testing.T) {
	value := slots.Items(
		slots.Item{Value: "荀正", ChunkID: "c1", Quote: "关公大怒，直取荀正 交马一合，砍荀正于马下"},
		slots.Item{Value: "于禁"},
		slots.Item{Value: "刘延", Quote: "quoted words with no passage to look them up in"},
	)
	table := NewState([]Variable{{ID: 0, Type: "person", Value: &value}}, 0, nil)

	anchored := AnchoredItems(&table)
	if len(anchored) != 1 || anchored[0] != "荀正" {
		t.Fatalf("anchored = %v, want only the member carrying a passage", anchored)
	}
	unanchored := UnanchoredItems(&table)
	if len(unanchored) != 2 {
		t.Fatalf("unanchored = %v, want the two claims", unanchored)
	}
	joined := strings.Join(unanchored, "、")
	if !strings.Contains(joined, "于禁") || !strings.Contains(joined, "刘延") {
		t.Fatalf("unanchored = %v, want both claimed names", unanchored)
	}
	if all := ItemValues(&table); len(all) != 3 {
		t.Fatalf("all members = %v, want the three names (the split deletes none)", all)
	}
}

// TestItemValuesDropsTheActorOfTheDeed pins the one identity rule the run has data for: the
// actor's own declared forms are not elements of the answer.
//
// A question that asks what someone did lists what he did it TO, so 关羽 cannot be one of the
// people he killed — yet the run probes his name as a term and a session can write it into a list
// (measured 2026-09-17: the probe ledger offered 关羽 among the "names" beside the victims). The
// forms come from the planner (Coverage.Actors, "关羽|云长"), which is also why the match is an
// IDENTITY rule and not a string rule: `关云长` carries the declared `云长` and is one item dropped;
// `关公` carries neither and is left alone, because guessing a form nobody declared is how an
// overloaded word starts dropping elements that are not the actor.
func TestItemValuesDropsTheActorOfTheDeed(t *testing.T) {
	value := slots.Items(
		slots.Item{Value: "关羽", ChunkID: "c1"},
		slots.Item{Value: "关云长", ChunkID: "c2"},
		slots.Item{Value: "关公"},
		slots.Item{Value: "华雄", ChunkID: "c3"},
	)
	table := NewState([]Variable{
		{ID: 0, Type: "count", Terms: []string{"斩"}, Subject: "关羽|云长"},
		{ID: 1, Type: "person", Value: &value},
	}, 0, nil)

	got := ItemValues(&table)
	if len(got) != 2 {
		t.Fatalf("items = %v, want the actor's two declared forms gone and the rest kept", got)
	}
	for _, want := range []string{"关公", "华雄"} {
		if !strings.Contains(strings.Join(got, "、"), want) {
			t.Errorf("items = %v, want %q kept", got, want)
		}
	}
	if a := AnchoredItems(&table); len(a) != 1 || a[0] != "华雄" {
		t.Fatalf("anchored = %v, want only the anchored item that is not the actor", a)
	}
}

// TestAnchoredItemsIsEmptyWithoutItems keeps the split safe on the shapes every other reader
// guards: a nil table, a table with no member slot, and a member slot whose items carry no name.
func TestAnchoredItemsIsEmptyWithoutItems(t *testing.T) {
	if got := AnchoredItems(nil); got != nil {
		t.Errorf("anchored(nil) = %v, want nothing", got)
	}
	text := slots.Text("华雄、颜良、文丑")
	textOnly := NewState([]Variable{{ID: 0, Type: "person", Value: &text}}, 0, nil)
	if got := AnchoredItems(&textOnly); len(got) != 0 {
		t.Errorf("anchored(text slot) = %v, want nothing: text declares no members", got)
	}
	if got := UnanchoredItems(&textOnly); len(got) != 0 {
		t.Errorf("unanchored(text slot) = %v, want nothing", got)
	}
	empty := slots.Items(slots.Item{Value: "  "})
	namedless := NewState([]Variable{{ID: 0, Type: "person", Value: &empty}}, 0, nil)
	if got := AnchoredItems(&namedless); len(got) != 0 {
		t.Errorf("anchored(blank name) = %v, want nothing", got)
	}
}

// TestCiteAnchoredMembersFillsWhatTheModelLeftUncited pins the step's whole reason: which passage
// a member rests on is the naming node's finding, so the runtime writes that member's citation
// itself. A line that already carries a marker is kept verbatim, a member the answer never states
// is left alone, and a member whose passage holds no published position cannot be pointed at.
func TestCiteAnchoredMembersFillsWhatTheModelLeftUncited(t *testing.T) {
	refs := []AnchoredRef{
		{Name: "华雄", ChunkID: "c-hua"},
		{Name: "管亥", ChunkID: "c-guan"},
		{Name: "蔡阳", ChunkID: "c-cai"},
		{Name: "不在答案里", ChunkID: "c-absent"},
		{Name: "庞德", ChunkID: "c-unpublished"},
	}
	ids := []string{"c-hua", "c-guan", "c-cai"}
	answer := strings.Join([]string{
		"关羽所斩：",
		"华雄——\"温酒斩华雄\"",
		"管亥——\"劈管亥于马下\" [ID:1]",
		"蔡阳——\"蔡阳头已落地\"",
		"庞德——\"德引颈受刑\"",
		"除上述数人外，另斩华雄、蔡阳等",
	}, "\n")
	got := CiteAnchoredMembers(answer, refs, ids)
	want := strings.Join([]string{
		"关羽所斩：",
		"华雄——\"温酒斩华雄\" [ID:0]",
		"管亥——\"劈管亥于马下\" [ID:1]",
		"蔡阳——\"蔡阳头已落地\" [ID:2]",
		"庞德——\"德引颈受刑\"",
		"除上述数人外，另斩华雄、蔡阳等 [ID:0][ID:2]",
	}, "\n")
	if got != want {
		t.Errorf("CiteAnchoredMembers =\n%s\nwant\n%s", got, want)
	}
	if got := CiteAnchoredMembers(answer, nil, ids); got != answer {
		t.Errorf("no refs must change nothing, got %q", got)
	}
	if got := CiteAnchoredMembers(answer, refs, nil); got != answer {
		t.Errorf("no published positions must change nothing, got %q", got)
	}
}
