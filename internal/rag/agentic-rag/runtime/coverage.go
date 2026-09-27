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

	"ragflow/internal/rag/agentic-rag/slots"
)

// CoverageActWordsMax bounds the act words ONE slot may declare.
const CoverageActWordsMax = 10

// Coverage is what a question ASKS FOR when its answer is a SET OF NAMED MEMBERS: an
// actor, the words the source uses for the deed, and the two declarations that say the
// answer is a set of names at all.
//
// It is the WHOLE of the enumeration strategy's interface — two fields carried from the
// planner's own declaration (Actor / Acts) and two readings of the same declaration
// (Set / Members). Everything the enumeration needs is answered by "is this three things
// true", and everything it does is answered by "one recall per operand, one window per
// place the deed is stated, one verdict per window" — no pattern language, no per-round
// bookkeeping, no reach ledger.
//
// Where the fields come from: the planner is asked to declare act words for "a SET or a
// COUNT of things that someone DID" and names the actor beside them (a slot's Terms and
// Subject). Those two declarations are the plan. A table that does not make them is not
// enumerated, and pays nothing (Coverage.Ok).
type Coverage struct {
	// Actor is the actor's declared forms, alternatives joined by '|' (a name and its
	// aliases).
	// Empty is allowed: the deed's own words still enumerate ("who was killed" questions
	// state the act without naming the actor).
	Actor string
	// Acts are the words the planner declared for the deed — the verbs the source uses for
	// it. A phrasing no act word covers is a passage no query names, which is why the
	// planner is asked for several.
	Acts []string
	// Set reports that the table asked for a count, a set or a list — the answer is a SET
	// rather than one value.
	Set bool
	// ItemKind is what the answer's elements are, when the table declares it: "person", "entity",
	// "dataset", or the bare "list"/"set". Empty when the answer is not a list of elements.
	ItemKind string
	// itemsInValue: a slot's value IS a list of items — the direction declared by the run's own
	// output rather than by the planner's word.
	//
	// Read by the point-of-naming node ALONE (see RunCoverageResolve), which runs last: a plan-time
	// type word must not be the only way a set can be completed. Deliberately NOT folded into Set —
	// the two readers of Set ask what the planner DECLARED (RenderSlotRecord's set block and
	// SessionState.parentSet), and read from the value they demote a value question's own draft
	// answer: measured (2026-09-17, FRAMES q759) a one-person question's record lost its
	// "Candidate answer:" line and gained "enumerated members across the slots above: 1".
	itemsInValue bool
}

// CoverageItemKindItems is the element kind a slot declares BY ITS OWN VALUE: a slot holding a
// list of items says the answer is a set, and no planner word names the kind any further.
const CoverageItemKindItems = "items"

// CoverageItemKinds reads the planner's slot vocabulary as what the answer enumerates. A kind not
// in this table is not a list of elements (a count, a range, a date, a phrase), and the gate below
// pays nothing for it.
var CoverageItemKinds = map[string]string{
	"entity":  "entity",
	"person":  "person",
	"dataset": "dataset",
	"list":    "list",
	"set":     "set",
}

// CoverageOf reads the declaration out of the planner's own table.
//
// Nothing here reads a slot's text, the question, or a candidate: the shape and the deed
// are what the PLANNER said about the answer, so the gate cannot be fooled by how a
// passage happens to be worded. A permissive reading instead spends set strategy on
// questions that assemble nothing.
func CoverageOf(table State) Coverage {
	var c Coverage
	for _, v := range table.State {
		// The two readings are independent: "list" and "set" are BOTH set-shaped and
		// element-carrying, so a slot may satisfy either or both (they were two predicates
		// before, and this keeps their union exact).
		kind := strings.ToLower(strings.TrimSpace(v.Type))
		switch kind {
		case "count", "set", "list":
			c.Set = true
		}
		if k, ok := CoverageItemKinds[kind]; ok && c.ItemKind == "" {
			c.ItemKind = k
		}
		// The VALUE declares the direction too, but to a NARROWER audience than Set: a slot holding
		// a list of items says the answer is a set, and that is what lets the point-of-naming node
		// complete a set the planner never typed (measured 2026-09-17, 三国/关羽: the members were
		// patched into a slot typed "count" and the run judged only the ten names a session wrote).
		// It is NOT Set: see the field's note for the question that read it too widely.
		if v.Typed().Kind == slots.KindItems {
			c.itemsInValue = true
			if c.ItemKind == "" {
				c.ItemKind = CoverageItemKindItems
			}
		}
		if c.Actor == "" {
			c.Actor = strings.TrimSpace(v.Subject)
		}
		for _, term := range v.Terms {
			if term = strings.TrimSpace(term); term != "" {
				c.Acts = appendUnique(c.Acts, term)
			}
		}
	}
	return c
}

// Ok reports whether this table asks for an enumeration: a SET of ELEMENTS whose deed the
// planner wrote the words for.
//
// The conjunction is the whole gate, and it is what separates a set of elements from a
// count of EVENTS — the planner declares act words for the latter too ("how many times did
// it happen" → the verb for it), and no element an enumeration could return changes a count
// of events. Held only by the element-carrying half, the enumeration runs on single-value
// questions, where it can only add cost and timeouts.
func (c Coverage) Ok() bool {
	return c.Set && c.ItemKind != "" && len(c.Acts) > 0
}

// ActsAll is the act vocabulary the enumeration filters by: the planner's declaration, and
// nothing else. It is a named extension point rather than a constant (see coverage_enumerate.go,
// which filters with it).
func (c Coverage) ActsAll() []string {
	return append([]string(nil), c.Acts...)
}

// ItemsInValue reports whether the table's own slots HOLD items — the direction the run's output
// declares where the planner's word did not. The point-of-naming node reads it and nothing else
// does (see Coverage.itemsInValue for why the record and a session must not).
func (c Coverage) ItemsInValue() bool { return c.itemsInValue }

// Actors splits the declared actor into its alternatives, which are what the corpus has
// to be read with: one source words one person several ways.
func (c Coverage) Actors() []string {
	var out []string
	for _, part := range strings.FieldsFunc(c.Actor, func(r rune) bool {
		return r == '|' || r == '｜' || r == '/' || r == '、' || r == ',' || r == '，'
	}) {
		if part = strings.TrimSpace(part); part != "" {
			out = appendUnique(out, part)
		}
	}
	return out
}

// Operands is the recall list: the actor's alternatives and the act words, deduped.
//
// ONE entry per operand is the point. Asking once per (actor, act) PAIR recalls the same
// operand over and over, so a common word fills its recall bound before the rarer ones are
// ever reached.
func (c Coverage) Operands() []string {
	var out []string
	for _, a := range c.Actors() {
		out = appendUnique(out, a)
	}
	for _, act := range c.Acts {
		if act = strings.TrimSpace(act); act != "" {
			out = appendUnique(out, act)
		}
	}
	return out
}
