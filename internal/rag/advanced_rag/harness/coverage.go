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

import "strings"

// CoverageActWordsMax bounds the act words ONE slot may declare. Ten because a source
// words one deed many ways (斩 / 杀 / 劈 / 挥为两段 …), and a phrasing no act word covers
// is a passage no query ever names — which is why the planner is asked for several.
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
	// Members reports that a slot of this table carries NAMES (entity / person / dataset),
	// as opposed to numbers, dates and phrases.
	Members bool
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
		// name-carrying, so a slot may satisfy either or both (they were two predicates
		// before, and this keeps their union exact).
		switch strings.ToLower(strings.TrimSpace(v.Type)) {
		case "count", "set", "list":
			c.Set = true
		}
		switch strings.ToLower(strings.TrimSpace(v.Type)) {
		case "entity", "person", "dataset", "list", "set":
			c.Members = true
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

// Ok reports whether this table asks for an enumeration: a SET of NAMED members whose
// deed the planner wrote the words for.
//
// The conjunction is the whole gate, and it is what separates a set of named members
// from a count of EVENTS — the planner declares act words for the latter too ("how many
// times did it happen" → the verb for it), and no name an enumeration could return changes
// a count of events. Held only by the NAME-carrying half, the enumeration runs on
// single-value questions, where it can only add cost and timeouts.
func (c Coverage) Ok() bool {
	return c.Set && c.Members && len(c.Acts) > 0
}

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
