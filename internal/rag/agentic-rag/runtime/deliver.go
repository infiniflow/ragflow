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
	"sort"
	"strings"
)

// Deliverable is the model-visible view of a chunk's text inside charBudget: the text a
// delivery point is allowed to show, chosen by LINE rather than cut at an arbitrary offset.
//
// Why lines and not a prefix: a chunk's fact is often not at its beginning. Measured
// 2026-09-22 on a ten-nominee question, the bio infoboxes went into the pool while the
// preview lines were the first 300 characters of each chunk — and the field the question
// asked for sat well past that cut, so the answer came back "not in any passage I read"
// with the passage in the pool. The unit that carries a fact is a line ("Children: 3",
// "1: Alice | 90"), and it is also the unit a reader cites, so the budget is spent on
// lines instead of on the head of a chunk.
//
// The selection is content-driven, never field-driven:
//   - a line naming the direction (the question's own words, tokenized) ranks highest;
//   - a line shaped like a field ("key: value", or a table row) ranks next, because such a
//     line states ONE fact and is the cheapest thing to hand a reader;
//   - a very long line ranks last: it is prose whose fact may sit anywhere inside it.
//
// ORDER IS PRESERVED on the way out: the lines kept are re-emitted in the chunk's own
// order, so what the model reads is the passage (minus the lines that say nothing about
// the question), not a relevance-sorted bag of fragments.
//
// Tables are rendered first (see RenderTables), so a table's rows compete as lines like
// any other text and a table chunk is no longer a special case at the call site.
//
// A text with no tokens to rank by (an empty direction) degrades to the old behaviour:
// the opening lines, in order, capped by the same budget.
func Deliverable(text, direction string, charBudget int) string {
	if charBudget <= 0 {
		return ""
	}
	text = TableViewOrRaw(text)
	if strings.TrimSpace(text) == "" {
		return ""
	}
	type line struct {
		idx   int
		text  string
		score int
	}
	raw := strings.Split(text, "\n")
	lines := make([]line, 0, len(raw))
	for i, l := range raw {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		lines = append(lines, line{idx: i, text: l})
	}
	if len(lines) == 0 {
		return ""
	}
	tokens := tokenizeDirection(direction)
	for i := range lines {
		lines[i].score = deliverLineScore(lines[i].text, tokens)
	}

	// Selection order: relevance first, document order to break ties. Output order is the
	// document's own (see the doc comment).
	order := make([]int, len(lines))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		x, y := lines[order[a]], lines[order[b]]
		if x.score != y.score {
			return x.score > y.score
		}
		return x.idx < y.idx
	})
	picked := make([]bool, len(lines))
	used := 0
	// Pass 1 — the FIELD lines (a rendered table row, see isFieldLine) are taken first: a line
	// that states one field of one entity is what a question matches against, while a prose line
	// can outscore it merely by repeating more of the direction's words. Measured 2026-09-22: with
	// the direction's words landing only in the prose, every field line of an infobox lost the
	// budget and the answer said the corpus carried no field for it.
	for _, i := range order {
		if !isFieldLine(lines[i].text) {
			continue
		}
		need := len([]rune(lines[i].text)) + 1
		if used+need > charBudget && used > 0 {
			continue
		}
		picked[i] = true
		used += need
	}
	// Pass 2 — everything else, with what the fields left over.
	for _, i := range order {
		if picked[i] || isFieldLine(lines[i].text) {
			continue
		}
		need := len([]rune(lines[i].text)) + 1
		if used+need > charBudget && used > 0 {
			continue
		}
		picked[i] = true
		used += need
	}

	kept := make([]string, 0, len(lines))
	for i, l := range lines {
		if picked[i] {
			kept = append(kept, l.text)
		}
	}
	return TruncateRunes(strings.Join(kept, "\n"), charBudget)
}

// FlattenLine folds a text into ONE line: runs of whitespace become a single space and the ends are
// trimmed.
//
// It is the shape every call site uses when it prints a PASSAGE as one line — an opening preview, a
// scan window, a nav prefix, a tool view's own text — and it lives here because the alternative is
// what the code had: the same expression written out by hand in six places, each free to drift.
func FlattenLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// deliverLineScore ranks one line for one direction. See Deliverable for what the signals
// mean; the numbers only order the lines against each other, they are not comparable
// across runs.
func deliverLineScore(line string, tokens []string) int {
	score := 0
	lower := strings.ToLower(line)
	for _, t := range tokens {
		if t != "" && strings.Contains(lower, t) {
			score += 2
		}
	}
	if isFieldLine(line) {
		// A field line already outranks prose by being taken first (see Deliverable); the bonus
		// only orders fields among themselves, so the ones whose name or value names the
		// direction come first inside that pass.
		score += 3
	} else if strings.Contains(line, ": ") || strings.Contains(line, "|") {
		score++
	}
	if len([]rune(line)) > deliverLongLineChars {
		score--
	}
	return score
}

// isFieldLine reports whether a line is one row rendered as a JSON object (see RenderTables) —
// {"Children": "3"}, {"Rank": "19", "Rider": "Danilo"}. Such a line states ONE field of one entity
// and carries its FIELD NAME, which is what a question matches against; a prose line carries only
// the direction's words, and any number of them.
func isFieldLine(line string) bool {
	return strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") && strings.Contains(line, ": ")
}

// deliverLongLineChars is where a line stops looking like a field and starts looking like
// prose: past it a line is deprioritised, because the fact inside it is not what the line
// advertises.
const deliverLongLineChars = 240

// Stage names the point of the run a delivery happens at. It exists so that what a delivery is
// allowed to be — how many items, how much of one item, how much of the block — is DATA (see
// stageBudgets) rather than a constant declared next to whichever function happened to need one:
// the session's seed alone used to carry four of them (the metadata catalog, the opening previews,
// the already-retrieved digest, the nav prefix) and they were kept in step by hand.
type Stage string

const (
	// StageOpening is the ranked preview list the session is handed every round.
	StageOpening Stage = "opening"
	// StageDigest is the "ALREADY RETRIEVED" block, ranked by the round's own direction.
	StageDigest Stage = "digest"
	// StagePrefix is the numbered block a nav-prefix read is replayed as.
	StagePrefix Stage = "prefix"
	// StageScan is the scan windows' block.
	StageScan Stage = "scan"
	// StageCatalog is the metadata-field catalog block.
	StageCatalog Stage = "catalog"
	// StageDraft is the composed draft.
	StageDraft Stage = "draft"
	// StageAnswer is the final answer's evidence render.
	StageAnswer Stage = "answer"
)

// Budget is what one stage is allowed to deliver.
//
// A delivery has THREE half-bounds and they used to live in three different places: how MANY items
// reach the model (MaxItems — the opening's 8, the digest's 4, the catalog's 20), how much of ONE
// item (MaxCharsPerItem), and how much of the whole BLOCK (MaxChars — a scan page, a catalog, a
// draft). A zero field means that kind of bound does not apply to the stage.
type Budget struct {
	// MaxItems bounds the delivery's item count. Zero: unbounded.
	MaxItems int
	// MaxCharsPerItem is the allowance of ONE item, spent by line (see Deliverable).
	// Zero: unbounded.
	MaxCharsPerItem int
	// MaxChars bounds the block as a whole. Zero: unbounded.
	MaxChars int
}

// stageBudgets is the ONE place a delivery's size is decided.
//
// The per-item value is an allowance, not a target: an item shorter than its allowance is
// delivered whole (see Deliverable), and only a longer one is cut — at a LINE boundary, never
// mid-line. Opening is the largest because its items are the members' own documents and the fact a
// question asks for can sit anywhere inside one (measured 2026-09-22: at 300 characters every
// member's preview stopped before its infobox field row and ten documents in the pool produced an
// answer that said no passage carried the fact).
var stageBudgets = map[Stage]Budget{
	StageOpening: {MaxItems: 8, MaxCharsPerItem: 2500},
	StageDigest:  {MaxItems: 4, MaxCharsPerItem: 1200},
	StagePrefix:  {MaxCharsPerItem: 200},
	StageScan:    {MaxChars: 40000},
	StageCatalog: {MaxItems: 20, MaxChars: 2000},
	StageDraft:   {MaxChars: 6000},
	StageAnswer:  {MaxChars: 12000},
}

// StageBudget is the budget of one stage. An unknown stage has none (the all-zero value), which
// makes every helper below deliver nothing — a new call site must NAME its stage, which is the
// point: the size is not the caller's private decision.
func StageBudget(stage Stage) Budget { return stageBudgets[stage] }

// StageMaxItems is a stage's item bound, 0 when it has none. A call site that loops over
// candidates takes its cap here, instead of keeping a private constant in step with the table by
// hand (the opening's 8 used to be exactly that constant).
func StageMaxItems(stage Stage) int { return stageBudgets[stage].MaxItems }

// StageChars is a stage's block bound, 0 when it has none.
func StageChars(stage Stage) int { return stageBudgets[stage].MaxChars }

// StageMaxItemsFor is a stage's ITEM bound raised to carry `declared` items, for a stage whose items
// are a set the plan NAMED rather than a sample of the pool.
//
// Why it exists: the opening is the list of things the session looks at first, and on an enumerated
// question it has to show every member the plan declared — a member the bound leaves out of the
// preview is a member the answer cannot name, however complete the pool is. Measured 2026-09-22 on
// the ten-nominee Oscar question: the entity channel read all TEN biographies into the pool, the
// opening delivered the first EIGHT, and the answer carried no child count for the two members that
// fell past the bound (Michelle Williams, Andrea Riseborough) — their documents were in the pool the
// whole time, and nothing said they had been dropped.
//
// The declared count is bounded by the caller (DeclaredProbes is capped at DeclaredProbesMax), so
// this raises the bound but never unbounds it; a value below the stage's own bound does NOT lower it,
// because that number is the floor the stage's other callers rely on.
func StageMaxItemsFor(stage Stage, declared int) int {
	if floor := stageBudgets[stage].MaxItems; declared < floor {
		return floor
	}
	return declared
}

// DeliverItemText is the model-visible text of ONE item at a stage: the stage's allowance, the
// round's direction as the ranking signal, and the line-level cutter they share (see Deliverable).
func DeliverItemText(stage Stage, text, direction string) string {
	return Deliverable(text, direction, stageBudgets[stage].MaxCharsPerItem)
}

// DeliverBlock is the model-visible text of a whole BLOCK at a stage: one bound, spent on the
// block rather than on one item of it, and cut at a character boundary.
//
// It deliberately does NOT re-select lines the way DeliverItemText does: a block is a PAGE the
// session reads top to bottom until the bound (the scan windows, the draft), so re-ordering or
// dropping lines inside it would be a DIFFERENT delivery rather than a smaller one. A caller that
// wants line selection calls DeliverItemText per item.
func DeliverBlock(stage Stage, text string) string {
	b := stageBudgets[stage]
	if b.MaxChars <= 0 {
		return text
	}
	return TruncateRunes(text, b.MaxChars)
}
