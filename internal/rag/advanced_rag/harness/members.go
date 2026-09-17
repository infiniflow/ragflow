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
	"strings"

	"ragflow/internal/rag/advanced_rag/slots"
)

// The table's members, defined ONCE.
//
// The count the answer reports, the list the model reads in the record line, and the
// members a merge unions are three readers of the same fact; while each collected them
// itself they could disagree (measured 2026-09-16: the graph's union deduped ACROSS slots
// case-sensitively while the record line lowercased, so one entity written two ways by two
// sessions was one member to the record and two to the count).

// memberNames is the set of member NAMES the table declares: every slot whose value IS
// members (slots.KindMembers), deduped case-insensitively, minus the pieces that merely
// CONTAIN a member.
//
// Two clauses, each with its reason:
//
//   - DECLARED members only. Nothing is parsed to find them: a slot holds members
//     because the model said so, each with its evidence, and a slot holding text — a
//     phrase, a sentence, a count, a date — contributes none. That is what makes the
//     count derived and the members accountable: measured (2026-09-16, 三国/关羽) a count
//     slot holding "约 17-19 人" was split into "约、人" and counted as two members while
//     fifteen real names sat in another slot.
//   - Case-insensitive, because every other member list in the build is (slots.Names,
//     slots.mergeMembers, the record line): the union used to dedupe ACROSS slots
//     case-sensitively, so one entity written two ways by two sessions counted as two —
//     a silent +1 in the number the answer reports.
//
// The last clause is what keeps a phrase out of the count. A session writing a place
// beside its owner (`洛阳关孟坦`) or an event beside its object (`温酒斩华雄`) writes a
// token that is short, digit-free and unpunctuated — it passes every shape test a name
// passes — yet it is not a second member: the member it names is already in the table.
// Measured (2026-09-16): a record whose slots enumerated 21 names counted 25, the four
// extra being `洛阳关孟坦`, `汜水关卞喜`, `荥阳王植`, `黄河渡口秦琪`. It stays until the
// last node is the only writer of a member list: then a name the corpus does not state
// cannot enter one at all.
func MemberNames(table *State) []string {
	if table == nil {
		return nil
	}
	var items []string
	seen := map[string]bool{}
	for _, v := range table.State {
		if v.Typed().Kind != slots.KindMembers {
			continue
		}
		for _, name := range v.Typed().Names() {
			name = strings.TrimSpace(name)
			key := strings.ToLower(name)
			if key == "" || seen[key] {
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
