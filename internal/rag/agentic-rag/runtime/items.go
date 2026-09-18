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
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"ragflow/internal/rag/agentic-rag/slots"
)

// The table's items, read in one place: the count the answer reports, the size the record line
// states, and the elements a merge unions are three readings of the same fact.

// ItemValues is the values the table's item slots declare: every slot whose value IS items
// (slots.KindItems), deduped case-insensitively, minus the pieces that merely contain another
// item and minus the actor's own forms. Nothing is parsed out of a slot's text: a slot holds items
// because the model said so.
func ItemValues(table *State) []string {
	if table == nil {
		return nil
	}
	// The actor of the deed is not one of its elements, so his declared forms (Coverage.Actors)
	// are dropped here. Two spellings of a victim's name still count as two: nothing declares them
	// as one yet.
	actorForms := CoverageOf(*table).Actors()
	var items []string
	seen := map[string]bool{}
	for _, v := range table.State {
		if v.Typed().Kind != slots.KindItems {
			continue
		}
		for _, name := range v.Typed().ItemValues() {
			name = strings.TrimSpace(name)
			key := strings.ToLower(name)
			if key == "" || seen[key] || IsActorForm(name, actorForms) {
				continue
			}
			seen[key] = true
			items = append(items, name)
		}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		contained := false
		for _, other := range items {
			if other == item || len([]rune(other)) >= len([]rune(item)) {
				continue
			}
			if strings.Contains(strings.ToLower(item), strings.ToLower(other)) {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, item)
		}
	}
	return out
}

// IsActorForm reports whether a value is the actor under one of the declared forms: an exact
// match, or a form of two runes or more carried inside a value no longer than the form plus one
// (关云长 carries 云长). A form nobody declared is not guessed: 关公 stays a member until something
// declares it as one of the actor's names.
func IsActorForm(value string, forms []string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return false
	}
	for _, form := range forms {
		f := strings.ToLower(strings.TrimSpace(form))
		if f == "" {
			continue
		}
		if v == f {
			return true
		}
		if utf8.RuneCountInString(f) >= 2 && strings.Contains(v, f) &&
			utf8.RuneCountInString(v) <= utf8.RuneCountInString(f)+1 {
			return true
		}
	}
	return false
}

// AnchoredItems is ItemValues restricted to the items that carry the passage stating them: the set
// a count may be derived from, since an item nobody can point at cannot be cited either. The
// predicate lives on the value (slots.Value.Anchored).
func AnchoredItems(table *State) []string {
	return itemValuesWhere(table, slots.Value.Anchored)
}

// UnanchoredItems is the complement of AnchoredItems: the values asserted with no passage in hand.
// They are not deleted — the record lists them beside the number as claims the answer has to
// account for — but they are not counted.
func UnanchoredItems(table *State) []string {
	return itemValuesWhere(table, slots.Value.Unanchored)
}

// AnchoredItemChunks is the passage behind each anchored item, deduped and in table order: what an
// answer that must cite one passage per element needs in front of it (see withCitedChunks).
func AnchoredItemChunks(table *State) []string {
	if table == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, v := range table.State {
		for _, it := range v.Typed().Anchored() {
			id := strings.TrimSpace(it.ChunkID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// AnchoredItemRefs is AnchoredItemChunks' pairing: every anchored item's value with the passage it
// rests on, in table order and deduped by value. The naming node matched each member to the
// passage that states its deed, so this is the member→passage table the answer's citations are
// written from (see CiteAnchoredMembers).
func AnchoredItemRefs(table *State) []AnchoredRef {
	if table == nil {
		return nil
	}
	var out []AnchoredRef
	seen := map[string]bool{}
	for _, v := range table.State {
		for _, it := range v.Typed().Anchored() {
			name := strings.TrimSpace(it.Value)
			id := strings.TrimSpace(it.ChunkID)
			if name == "" || id == "" {
				continue
			}
			key := strings.ToLower(name)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, AnchoredRef{Name: name, ChunkID: id, Quote: strings.TrimSpace(it.Quote)})
		}
	}
	return out
}

// citedMarkerPattern matches a citation marker the answer may already carry: the canonical
// "[ID:n]" the rules prescribe, and the bare "[n]" a model writes on its own.
var citedMarkerPattern = regexp.MustCompile(`\[(?:ID:\s*)?[0-9]+\]`)

// CiteAnchoredMembers attaches the citation of every anchored member the answer states without
// one: the line naming the member gets the marker of the passage that member rests on, taken from
// citeIDs — the published evidence list, whose positions are what the client opens.
//
// The step is the RUNTIME's, not the model's: the naming node already matched each member to the
// passage that names it, while a model can only cite the blocks it was shown — the evidence budget
// admits the first few whole chunks, so a member past them has no block number it could write. A
// member the answer never states is left alone, and a line that already carries a marker is kept
// as written.
func CiteAnchoredMembers(answer string, refs []AnchoredRef, citeIDs []string) string {
	if strings.TrimSpace(answer) == "" || len(refs) == 0 || len(citeIDs) == 0 {
		return answer
	}
	pos := map[string]int{}
	for i, id := range citeIDs {
		if id = strings.TrimSpace(id); id == "" {
			continue
		}
		if _, dup := pos[id]; !dup {
			pos[id] = i
		}
	}
	if len(pos) == 0 {
		return answer
	}
	lines := strings.Split(answer, "\n")
	for i, line := range lines {
		// A line naming members is given the marker of EVERY member it names, and any marker the
		// model wrote there is replaced by them: the model cites the blocks it read, so a line it
		// grouped ("颜良…文丑" both [ID:5]) points at a passage that is not that member's, and a
		// sentence naming nine members carries one marker for nine. Which member rests on which
		// passage is the naming node's finding (refs × citeIDs), so the runtime's answer replaces
		// the model's guess on exactly those lines; a line naming no member is left untouched.
		var add []string
		for _, r := range refs {
			if !strings.Contains(line, r.Name) {
				continue
			}
			if idx, ok := pos[strings.TrimSpace(r.ChunkID)]; ok {
				add = append(add, "[ID:"+strconv.Itoa(idx)+"]")
			}
		}
		if len(add) == 0 {
			continue
		}
		stripped := strings.TrimRight(strings.TrimSpace(citedMarkerPattern.ReplaceAllString(line, "")), " \t")
		lines[i] = stripped + " " + strings.Join(add, "")
	}
	return strings.Join(lines, "\n")
}

// itemValuesWhere joins the items a picker selects ACROSS the table's slots, in ItemValues order,
// so every reading of the table agrees on WHICH values exist and in what order; only the picker
// differs.
func itemValuesWhere(table *State, pick func(slots.Value) []slots.Item) []string {
	if table == nil {
		return nil
	}
	kept := map[string]bool{}
	for _, v := range table.State {
		for _, it := range pick(v.Typed()) {
			if val := strings.TrimSpace(it.Value); val != "" {
				kept[strings.ToLower(val)] = true
			}
		}
	}
	out := make([]string, 0, len(kept))
	for _, name := range ItemValues(table) {
		if kept[strings.ToLower(strings.TrimSpace(name))] {
			out = append(out, name)
		}
	}
	return out
}
