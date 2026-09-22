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
)

// TestDeliverablePrefersTheLinesThatNameTheDirection pins the selection rule: with a budget
// too small for the whole chunk, the lines that say something about the question are the ones
// kept — and they come back in the chunk's own order, not in relevance order.
func TestDeliverablePrefersTheLinesThatNameTheDirection(t *testing.T) {
	text := "Born: 1961\nCitizenship: American\nChildren: 3\nSpouse: Jane Doe\nAwards: Full list"
	got := Deliverable(text, "how many children did the nominees have", 40)
	if !strings.Contains(got, "Children: 3") {
		t.Fatalf("Deliverable = %q, want the line naming the direction", got)
	}
	if strings.Index(got, "Born:") > strings.Index(got, "Children:") {
		t.Errorf("Deliverable = %q, want the chunk's own order preserved", got)
	}
	if len([]rune(got)) > 40 {
		t.Errorf("Deliverable = %q (%d runes), want at most the budget", got, len([]rune(got)))
	}
}

// TestDeliverableRendersTablesFirst pins that a table chunk is no longer a special case at the
// call site: its rows are rendered as field objects (see RenderTables) and then compete as lines
// like any other text — so the fact the question asks for is reachable without a table-specific
// branch.
func TestDeliverableRendersTablesFirst(t *testing.T) {
	raw := "<table><tr><th>Born</th><td>1961</td></tr><tr><th>Children</th><td>3</td></tr></table>"
	got := Deliverable(raw, "children", 200)
	if strings.Contains(got, "<td>") {
		t.Fatalf("Deliverable = %q, want the table rendered", got)
	}
	if !strings.Contains(got, `{"Children": "3"}`) {
		t.Fatalf("Deliverable = %q, want the rendered field object", got)
	}
}

// TestDeliverableKeepsFieldLinesWhenOnlyProseMatches pins the failure this pass exists for: with a
// direction whose words land ONLY in the prose, the field objects of an infobox used to lose the
// whole budget to the prose (measured 2026-09-22: an answer said the corpus carried no field for
// the question while the infobox was in the budget). Fields are taken first, prose gets what is
// left.
func TestDeliverableKeepsFieldLinesWhenOnlyProseMatches(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"Born": "1968-12-03"}` + "\n")
	b.WriteString(`{"Children": "3"}` + "\n")
	b.WriteString(`{"Spouse": "Afton Smith"}` + "\n")
	b.WriteString("Fraser is an American and Canadian actor whose film career spans comedy and drama. ")
	for i := 0; i < 6; i++ {
		b.WriteString("He is an American and Canadian actor with a long film career. ")
	}
	got := Deliverable(b.String(), "American Canadian actor film career", 400)
	if !strings.Contains(got, `{"Children": "3"}`) {
		t.Fatalf("Deliverable = %q, want the field objects kept when only the prose matches", got)
	}
}

// TestDeliverableWithoutDirectionKeepsTheOpeningLines pins the degrade path: with nothing to
// rank by, the behaviour is the old one — the chunk's opening lines, in order, same budget.
func TestDeliverableWithoutDirectionKeepsTheOpeningLines(t *testing.T) {
	text := "line one\nline two\nline three"
	if got := Deliverable(text, "", 100); got != text {
		t.Errorf("Deliverable = %q, want the text unchanged", got)
	}
	short := Deliverable(text, "", 9)
	if !strings.HasPrefix(short, "line one") {
		t.Errorf("Deliverable = %q, want the opening kept", short)
	}
	if len([]rune(short)) > 9 {
		t.Errorf("Deliverable = %q (%d runes), want at most the budget", short, len([]rune(short)))
	}
}

// TestDeliverableRespectsTheBudget pins the bound on every path, including the one where the
// first pick alone is longer than the budget.
func TestDeliverableRespectsTheBudget(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 50; i++ {
		b.WriteString("field ")
		b.WriteString(strings.Repeat("x", 20))
		b.WriteString(": value\n")
	}
	for _, budget := range []int{1, 30, 120, 500} {
		if got := Deliverable(b.String(), "value", budget); len([]rune(got)) > budget {
			t.Errorf("Deliverable(budget=%d) returned %d runes", budget, len([]rune(got)))
		}
	}
}

// TestDeliverItemTextUsesTheStageAllowance pins the table: one text, three stages — the opening
// delivers more of it than the digest, and the digest more than the prefix. The point of the table
// is that a call site cannot pick its own length any more.
func TestDeliverItemTextUsesTheStageAllowance(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 300; i++ {
		b.WriteString("Born: 1961\n")
	}
	text := b.String()
	opening := DeliverItemText(StageOpening, text, "")
	digest := DeliverItemText(StageDigest, text, "")
	prefix := DeliverItemText(StagePrefix, text, "")
	if !(len([]rune(opening)) > len([]rune(digest)) && len([]rune(digest)) > len([]rune(prefix))) {
		t.Fatalf("allowances are not ordered: opening=%d digest=%d prefix=%d",
			len([]rune(opening)), len([]rune(digest)), len([]rune(prefix)))
	}
	if got := len([]rune(opening)); got > stageBudgets[StageOpening].MaxCharsPerItem {
		t.Errorf("opening delivered %d runes, want at most the stage allowance %d",
			got, stageBudgets[StageOpening].MaxCharsPerItem)
	}
}

// TestDeliverItemTextUnknownStageDeliversNothing pins the other half of the table's contract: a
// stage with no allowance delivers nothing, so a new call site has to NAME its stage rather than
// inherit whichever length a default happened to have.
func TestDeliverItemTextUnknownStageDeliversNothing(t *testing.T) {
	if got := DeliverItemText(Stage("not-a-stage"), "Born: 1961", ""); got != "" {
		t.Errorf("DeliverItemText(unknown stage) = %q, want nothing", got)
	}
}

// TestStageAllowancesCoverEveryStage pins that the table carries every stage the code names: a
// stage added without an allowance would deliver nothing, silently. Each stage needs its kind of
// bound — the per-item ones an item allowance, the block ones a block bound.
func TestStageAllowancesCoverEveryStage(t *testing.T) {
	for _, s := range []Stage{StageOpening, StageDigest, StagePrefix, StageScan, StageCatalog, StageDraft, StageAnswer} {
		b := StageBudget(s)
		if b.MaxCharsPerItem <= 0 && b.MaxChars <= 0 {
			t.Errorf("stage %q has neither an item allowance nor a block bound", s)
		}
	}
}

// TestStageMaxItemsIsTheItemBound pins the second half of a budget: the ITEM COUNT, which used to
// be a private constant at the call site (the opening's 8) kept in step with the length by hand.
// The opening is the one stage that both shows several items and bounds how many.
func TestStageMaxItemsIsTheItemBound(t *testing.T) {
	if got := StageMaxItems(StageOpening); got != 8 {
		t.Errorf("StageMaxItems(opening) = %d, want 8", got)
	}
	if got := StageMaxItems(StagePrefix); got != 0 {
		t.Errorf("StageMaxItems(prefix) = %d, want unbounded", got)
	}
}

// TestDeliverBlockSpendsOneBoundOnTheWholeBlock pins the block path: a block bound cuts the text,
// and — unlike the per-item path — it does NOT re-select lines, because a page the session reads
// top to bottom is not made smaller by reordering it.
func TestDeliverBlockSpendsOneBoundOnTheWholeBlock(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 400; i++ {
		b.WriteString("Born: 1961\n")
	}
	got := DeliverBlock(StageDraft, b.String())
	if len([]rune(got)) > StageChars(StageDraft) {
		t.Errorf("DeliverBlock delivered %d runes, want at most the block bound %d",
			len([]rune(got)), StageChars(StageDraft))
	}
	if !strings.HasPrefix(got, "Born: 1961") {
		t.Errorf("DeliverBlock = %q…, want the block's own opening", truncateRunes(got, 20))
	}
	if same := DeliverBlock(StageDraft, "short"); same != "short" {
		t.Errorf("DeliverBlock(short) = %q, want it unchanged", same)
	}
}

// TestStageMaxItemsForRaisesToTheDeclaredSet pins the two-sided rule: the declared set RAISES a
// stage's item bound — an enumerated question must show every member it named (measured 2026-09-22,
// the ten Oscar nominees: the entity channel read all ten biographies into the pool, the opening
// showed the first eight, and the answer carried no child count for the two members past the bound)
// — and it never LOWERS it, because the stage's own number is the floor its other callers rely on.
func TestStageMaxItemsForRaisesToTheDeclaredSet(t *testing.T) {
	if got := StageMaxItemsFor(StageOpening, 10); got != 10 {
		t.Errorf("StageMaxItemsFor(opening, 10) = %d, want the declared set", got)
	}
	if got := StageMaxItemsFor(StageOpening, 3); got != StageMaxItems(StageOpening) {
		t.Errorf("StageMaxItemsFor(opening, 3) = %d, want the stage's own bound %d",
			got, StageMaxItems(StageOpening))
	}
	if got := StageMaxItemsFor(StagePrefix, 5); got != 5 {
		t.Errorf("StageMaxItemsFor(prefix, 5) = %d, want 5 (that stage keeps no floor)", got)
	}
}
