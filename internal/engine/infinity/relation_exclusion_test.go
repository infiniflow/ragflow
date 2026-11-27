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

package infinity

import (
	"fmt"
	"strings"
	"testing"
)

// TestEquivalentConditionToStrExcludesRelationWindowIDs drives the production
// condition wrapper (so CompilationFilter runs as it does on the search path)
// with the writer's relation filter plus the read-id exclusion. The rendered
// expression must evaluate to false for every id already read and true for an
// id that was not read, for exactly one result window and past it. Reading the
// expression back, rather than counting substrings, is what pins the polarity:
// a filter that ORs the exclusions inverts exactly this outcome.
func TestEquivalentConditionToStrExcludesRelationWindowIDs(t *testing.T) {
	const chunk = 512
	for _, tc := range []struct {
		name  string
		total int
	}{
		{"exactly one result window", 10000},
		{"past the result window", 10400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ids := make([]string, tc.total)
			for i := range ids {
				ids[i] = fmt.Sprintf("id-%d", i)
			}
			var and []map[string]interface{}
			for start := 0; start < len(ids); start += chunk {
				end := min(start+chunk, len(ids))
				and = append(and, map[string]interface{}{"must_not": map[string]interface{}{"id": ids[start:end]}})
			}
			got := equivalentConditionToStr(map[string]interface{}{
				"knowledge_graph_kwd": "relation",
				"from_entity_kwd":     []string{"Alice"},
				"to_entity_kwd":       []string{"Bob"},
				"and":                 and,
			}, nil)

			if want := (tc.total + chunk - 1) / chunk; strings.Count(got, "NOT (id IN (") != want {
				t.Errorf("NOT clauses = %d, want %d", strings.Count(got, "NOT (id IN ("), want)
			}
			if !strings.Contains(got, " AND ") {
				t.Errorf("exclusions are not conjoined: %q", got[:min(len(got), 200)])
			}
			for _, id := range ids {
				if n := strings.Count(got, "'"+id+"'"); n != 1 {
					t.Fatalf("id %q excluded %d times, want 1", id, n)
				}
			}
			if strings.Contains(got, "'unread-id'") {
				t.Error("an id that was not read is excluded")
			}

			row := func(id string) map[string]string {
				return map[string]string{
					"id":                  id,
					"knowledge_graph_kwd": "relation",
					"type_kwd":            "relation",
					"from_entity_kwd":     "Alice",
					"to_entity_kwd":       "Bob",
				}
			}
			if !evalInfinityFilter(t, got, row("unread-id")) {
				t.Error("an id that was not read no longer matches the expression")
			}
			for _, id := range []string{ids[0], ids[tc.total/2], ids[tc.total-1]} {
				if evalInfinityFilter(t, got, row(id)) {
					t.Fatalf("read id %q still matches the expression", id)
				}
			}
		})
	}
}

// evalInfinityFilter evaluates the subset of Infinity's filter grammar the
// relation sweep renders: AND/OR/NOT (" and "/" or " as well as the uppercase
// forms), parentheses, `field IN (...)`, `filter_fulltext('needle', 'field')`,
// and `field='value'` comparisons.
func evalInfinityFilter(t *testing.T, expr string, doc map[string]string) bool {
	t.Helper()
	p := &infinityFilterParser{src: expr, doc: doc, t: t}
	match := p.parseOr()
	p.skipSpace()
	if p.pos != len(p.src) {
		t.Fatalf("unparsed filter at %d: %q", p.pos, p.src[p.pos:])
	}
	return match
}

type infinityFilterParser struct {
	src string
	pos int
	doc map[string]string
	t   *testing.T
}

func (p *infinityFilterParser) skipSpace() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\n') {
		p.pos++
	}
}

func (p *infinityFilterParser) hasOp(ops ...string) bool {
	p.skipSpace()
	for _, op := range ops {
		if strings.HasPrefix(p.src[p.pos:], op) {
			p.pos += len(op)
			return true
		}
	}
	return false
}

func (p *infinityFilterParser) parseOr() bool {
	match := p.parseAnd()
	for p.hasOp("OR ", "or ") {
		right := p.parseAnd()
		match = match || right
	}
	return match
}

func (p *infinityFilterParser) parseAnd() bool {
	match := p.parseUnary()
	for p.hasOp("AND ", "and ") {
		right := p.parseUnary()
		match = match && right
	}
	return match
}

func (p *infinityFilterParser) parseUnary() bool {
	if p.hasOp("NOT (") {
		match := p.parseOr()
		if !p.hasOp(")") {
			p.t.Fatalf("unterminated NOT at %d: %q", p.pos, p.src)
		}
		return !match
	}
	if p.hasOp("(") {
		match := p.parseOr()
		if !p.hasOp(")") {
			p.t.Fatalf("unterminated group at %d: %q", p.pos, p.src)
		}
		return match
	}
	return p.parseAtom()
}

func (p *infinityFilterParser) parseAtom() bool {
	start := p.pos
	depth := 0
	quoted := false
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		switch {
		case quoted:
			if c == '\'' {
				quoted = false
			}
		case c == '\'':
			quoted = true
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				return p.evalAtom(strings.TrimSpace(p.src[start:p.pos]))
			}
			depth--
		}
		if !quoted && depth == 0 {
			if rest := p.src[p.pos:]; strings.HasPrefix(rest, " AND ") || strings.HasPrefix(rest, " OR ") || strings.HasPrefix(rest, " or ") || strings.HasPrefix(rest, " and ") {
				return p.evalAtom(strings.TrimSpace(p.src[start:p.pos]))
			}
		}
		p.pos++
	}
	return p.evalAtom(strings.TrimSpace(p.src[start:p.pos]))
}

func (p *infinityFilterParser) evalAtom(atom string) bool {
	switch {
	case strings.HasPrefix(atom, "id IN ("):
		list := strings.TrimSuffix(strings.TrimPrefix(atom, "id IN ("), ")")
		for _, item := range strings.Split(list, ",") {
			if strings.Trim(strings.TrimSpace(item), "'") == p.doc["id"] {
				return true
			}
		}
		return false
	case strings.HasPrefix(atom, "filter_fulltext("):
		args := strings.Split(strings.TrimSuffix(strings.TrimPrefix(atom, "filter_fulltext("), ")"), ",")
		field := strings.Trim(strings.TrimSpace(args[0]), "'")
		needle := strings.Trim(strings.TrimSpace(args[1]), "'")
		for _, token := range strings.Fields(p.doc[field]) {
			if strings.EqualFold(token, needle) {
				return true
			}
		}
		return false
	}
	idx := strings.Index(atom, "=")
	if idx < 0 {
		p.t.Fatalf("cannot evaluate atom %q", atom)
	}
	field := strings.TrimSpace(atom[:idx])
	value := strings.Trim(strings.TrimSpace(atom[idx+1:]), "'")
	return p.doc[field] == value
}
