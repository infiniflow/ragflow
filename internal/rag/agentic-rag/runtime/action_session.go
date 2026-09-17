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
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/kaptinlin/jsonrepair"
	"gorm.io/gorm"

	"ragflow/internal/agent/chat"
	"ragflow/internal/rag/agentic-rag/slots"
	"ragflow/internal/rag/prompts"

	"ragflow/internal/common"
)

// action_session.go consolidates what was previously split across sessiontypes.go,
// extract.go, toolschema.go, run.go, navchain.go, and session.go. The split is preserved
// as clearly marked section banners below; every symbol keeps its original name and
// behavior so tool_executor.go and the tests are unaffected.
//
// The section ORDER follows the stages of a session (helpers → classes → tool specs →
// executors → model seam → session state → nodes → nav prefix → entry points at the end),
// not a Go dependency order.

// ===================== sessiontypes.go =====================
// Slot-table models and the unified tool-result
//
// The slot-table models and the unified tool result:
//   - Variable / State / Result
//   - tool status constants
//   - ToolOutcome
//   - ApplyPatch

// Variable is one unknown entity to resolve. ID is immutable across patches.
type Variable struct {
	ID              int
	Type            string
	QuestionClues   []string
	DiscoveredClues []string
	// Alternates are claims this slot LOST a comparison to (see MergeSlotPatch) and
	// keeps so they are not silently discarded: one slot holds one candidate, and the
	// framework does not decide which claim is true — it stops throwing the other away.
	// They are a FIELD and not a prefix inside DiscoveredClues because the record, the
	// ledger and the merge all read them, and a fact three readers parse out of a
	// string is a fact three parsers can disagree about.
	Alternates        []string
	Candidate         *string
	CandidateStrength *float64
	// Value is the TYPED candidate (see package slots): what this slot holds, as
	// data. It is set when the model DECLARED a kind, and nil otherwise.
	//
	// nil means "untyped", not "unknown": Candidate still carries the text, and that
	// text is OPAQUE — nothing may split it, count it, or compare it numerically.
	// Typing is what lets the runtime merge and count without guessing, so an
	// untyped slot is simply not merged by union (see slots.Union I1) rather than
	// parsed to find out what it might have been.
	Value *slots.Value
	// Terms are the ACT WORDS a direction declared for this slot — how its source
	// words the deed being enumerated (斩 / 杀 / 诛 / 劈 / 砍). They are declared,
	// never inferred from a slot's text or from the question.
	//
	// They exist because an enumeration cannot be bounded by what the model can
	// recall: the corpus holds the truth, and the words the source uses are what
	// reaches it. The runtime asks the corpus with them and RUNS the enumeration
	// (see Coverage / EnumerateCoverage), and the sessions read what came back, so the
	// tail of the list is a property of the corpus rather than of the model's memory.
	Terms []string
	// Subject is WHO the act is about, declared next to the act words. The pattern asks for
	// subject AND act together, because an act word on its own has a poor candidate pool — a
	// rare verb can come back empty for a text that does state the deed. Empty means the
	// pattern searches the term alone.
	Subject string
}

// Typed is the value this slot holds, for merging and counting. An untyped slot is
// Text by definition — its candidate verbatim, never interpreted.
func (v Variable) Typed() slots.Value {
	if v.Value != nil {
		return *v.Value
	}
	if v.Candidate != nil {
		return slots.Text(*v.Candidate)
	}
	return slots.Value{}
}

// Brief: one-line rendering for prompts.
func (v Variable) Brief() string {
	if v.Candidate != nil && *v.Candidate != "" {
		cs := "?"
		if v.CandidateStrength != nil {
			cs = fmt.Sprintf("%.2f", *v.CandidateStrength)
		}
		return fmt.Sprintf("[%d] %s: %s (%s)", v.ID, v.Type, *v.Candidate, cs)
	}
	return fmt.Sprintf("[%d] %s: EMPTY", v.ID, v.Type)
}

// Filled
func (v Variable) Filled() bool { return v.Candidate != nil && *v.Candidate != "" }

// State is the slot table carried through one action session.
type State struct {
	State                []Variable
	Depth                int
	ID                   string
	RetrievedEvidenceIDs []string
}

// NewState builds a State, generating its ID as
// "<depth:03x>_<millis%1e8:08x><1 random byte:02x>".
func NewState(vars []Variable, depth int, evidenceIDs []string) State {
	s := State{
		State:                vars,
		Depth:                depth,
		RetrievedEvidenceIDs: evidenceIDs,
	}
	if s.ID == "" {
		s.ID = newStateID(depth)
	}
	return s
}

// newStateID: id scheme.
func newStateID(depth int) string {
	ms := time.Now().UnixMilli() % 100000000
	var b [1]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%03x_%08x%02x", depth, ms, b[0])
}

// Unresolved: slots still missing a candidate.
func (s State) Unresolved() []Variable {
	out := make([]Variable, 0, len(s.State))
	for _, v := range s.State {
		if !v.Filled() {
			out = append(out, v)
		}
	}
	return out
}

// ByID: Returns nil when no slot carries vid.
func (s *State) ByID(vid int) *Variable {
	for i := range s.State {
		if s.State[i].ID == vid {
			return &s.State[i]
		}
	}
	return nil
}

// Brief: e.g. "d0(++.)".
func (s State) Brief() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("d%d(", s.Depth))
	for _, v := range s.State {
		if v.Filled() {
			b.WriteByte('+')
		} else {
			b.WriteByte('.')
		}
	}
	b.WriteByte(')')
	return b.String()
}

// RenderSlots: the multi-line prompt block
// listing every slot, its clues, and any candidate found so far.
func (s State) RenderSlots() string {
	var lines []string
	for _, v := range s.State {
		line := fmt.Sprintf("- id=%d type=%s", v.ID, v.Type)
		if len(v.QuestionClues) > 0 {
			line += "\n  question_clues: " + strings.Join(v.QuestionClues, "; ")
		}
		if len(v.DiscoveredClues) > 0 {
			tail := v.DiscoveredClues
			if len(tail) > 4 {
				tail = tail[len(tail)-4:]
			}
			line += "\n  discovered_clues: " + strings.Join(tail, "; ")
		}
		if v.Candidate != nil && *v.Candidate != "" {
			cs := "?"
			if v.CandidateStrength != nil {
				cs = fmt.Sprintf("%.2f", *v.CandidateStrength)
			}
			line += fmt.Sprintf("\n  CANDIDATE: %s (strength=%s)", *v.Candidate, cs)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Result is the outcome of ONE run_action session (one of NewStates /
// FoundAnswer).
//
// Messages is typed as []schema.Message: the session builds them through the eino
// constructors, and a typed slice keeps the conversation inspectable (and testable)
// without type assertions.
type Result struct {
	Messages             []schema.Message
	NewStates            []State
	FoundAnswer          *string
	RetrievedEvidenceIDs []string
	TerminalType         *string
	TerminalPayload      map[string]any
}

// ApplyPatch: ONLY existing
// ids are patchable; the mutable fields are candidate / candidate_strength /
// discovered_clues. ID is immutable, so patches may not add variables.
//
// Returns nil when a patch entry is malformed (not a map, or missing "id") or when
// nothing actually changed.
func ApplyPatch(base State, branchPatches []map[string]any) *State {
	newVars := make([]Variable, 0, len(base.State))
	for _, v := range base.State {
		nv := Variable{
			ID:                v.ID,
			Type:              v.Type,
			QuestionClues:     append([]string(nil), v.QuestionClues...),
			DiscoveredClues:   append([]string(nil), v.DiscoveredClues...),
			Alternates:        append([]string(nil), v.Alternates...),
			Candidate:         v.Candidate,
			CandidateStrength: v.CandidateStrength,
			Value:             v.Value,
		}
		newVars = append(newVars, nv)
	}

	changed := false
	for _, pv := range branchPatches {
		if pv == nil {
			return nil
		}
		rawID, ok := pv["id"]
		if !ok {
			return nil
		}
		// The comparison is integer-only, so a JSON string id ("1") simply matches no slot
		// and is skipped. Only JSON numbers match, which decode as float64 — accept those,
		// and reject anything else so the behaviour is strict rather than silently more
		// permissive.
		want, ok := toIntStrict(rawID)
		if !ok {
			continue
		}
		idx := -1
		for i := range newVars {
			if newVars[i].ID == want {
				idx = i
				break
			}
		}
		if idx < 0 {
			continue
		}
		nv := &newVars[idx]

		if rawKind, has := pv["kind"]; has && rawKind != nil && strings.TrimSpace(fmt.Sprint(rawKind)) != "" {
			// A patch that DECLARES what it holds is read as data (package slots) and
			// its rendered text is kept in Candidate, so every renderer, prompt and
			// log keeps working unchanged. This branch is the whole point of the
			// typed contract: the structure the model states is carried, and nothing
			// downstream has to infer it back from a string.
			value := slots.Parse(pv)
			nv.Value = &value
			rendered := slots.Render(value)
			nv.Candidate = &rendered
			changed = true
		} else if raw, has := pv["candidate"]; has {
			// Any falsy value (0, 0.0, False, "", [], {}, None) becomes None, NOT its string
			// form. fmt.Sprint would otherwise turn 0/False into "0"/"false".
			if !isTruthy(raw) {
				nv.Candidate = nil
				nv.Value = nil
			} else {
				s := fmt.Sprint(raw)
				nv.Candidate = &s
				// Opaque by construction: the model did not say what this is, so
				// nothing here will guess (see Variable.Value).
				text := slots.Text(s)
				nv.Value = &text
			}
			changed = true
		}
		if raw, has := pv["candidate_strength"]; has && raw != nil {
			if f, ok := toFloat(raw); ok {
				v := math.Min(math.Max(f, 0.0), 1.0)
				nv.CandidateStrength = &v
				changed = true
			}
			// Unparseable strengths are ignored.
		}
		if raw, has := pv["discovered_clues"]; has {
			if list, ok := raw.([]any); ok {
				tail := list
				if len(tail) > 4 {
					tail = tail[len(tail)-4:]
				}
				for _, c := range tail {
					nv.DiscoveredClues = append(nv.DiscoveredClues, truncateRunes(fmt.Sprint(c), 160))
				}
				changed = true
			}
		}
	}
	if !changed {
		return nil
	}
	out := State{
		State:                newVars,
		Depth:                base.Depth + 1,
		RetrievedEvidenceIDs: append([]string(nil), base.RetrievedEvidenceIDs...),
	}
	out.ID = newStateID(out.Depth)
	return &out
}

// truncateRunes cuts s to at most n characters (runes, not bytes).
func truncateRunes(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// toInt coerces a JSON-decoded value to int. Handles float64 (the default for
// JSON numbers), the integer types, and numeric strings.
func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil {
			return i, true
		}
	}
	return 0, false
}

// toIntStrict converts JSON NUMBER types only. Unlike toInt it does NOT parse strings: a
// patch whose id is "1" must match nothing.
func toIntStrict(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// toFloat coerces a JSON-decoded value to float64.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case string:
		return ParseFloat(n)
	}
	return 0, false
}

// ParseFloat parses a float64 from a string, tolerating surrounding
// whitespace. Exported for the orchestrator package, whose SCA verdict
// coercion needs the same lenient parsing.
func ParseFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// isTruthy applies bool() truthiness to the JSON-decoded values that reach ApplyPatch:
// None/nil, empty string/collection, zero number, and False are falsy; everything else
// (including non-empty objects and any other type) is truthy.
func isTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int32:
		return x != 0
	case int64:
		return x != 0
	case float32:
		return x != 0
	case float64:
		return x != 0
	case []any:
		return len(x) > 0
	case []string:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

// Slot-table value coercion (text / int / literal)
//
// initialize_state reads the model's slot-table reply strictly: an id that is not an
// integer, a type that is not text, or clues that are not a list are FAILURES, not
// guesses — the caller discards the whole decomposition and falls back to the planner's
// fan-outs (see buildSlotTableFrom). So these helpers report failure instead of inventing
// a value: `int(s.get("id", i))`, `str(s.get("type") or "entity")` and
// `[str(c) for c in (s.get("clues") or [])]` are the three questions they answer.
//
// quoteLiteral/formatLiteral render the LITERAL SYNTAX this protocol is written in —
// single-quoted strings, None/True/False, [...] and {...} — which is the format the
// prompts teach the model and the format its replies come back in.

// quoteLiteral renders s as the protocol's quoted string literal: single quotes unless
// the text contains ' but no ", with the standard escapes (\\, \n, \r, \t, \xNN for other
// control characters). Printable non-ASCII is kept verbatim.
func quoteLiteral(s string) string {
	quote := '\''
	if strings.ContainsRune(s, '\'') && !strings.ContainsRune(s, '"') {
		quote = '"'
	}
	var b strings.Builder
	b.WriteRune(quote)
	for _, r := range s {
		switch {
		case r == quote:
			b.WriteByte('\\')
			b.WriteRune(r)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			b.WriteString(fmt.Sprintf(`\x%02x`, r))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteRune(quote)
	return b.String()
}

// formatLiteral renders a JSON-decoded value in the protocol's literal syntax:
// None/True/False, quoted strings, numbers, [...] and {...}. Map keys come out SORTED —
// Go's maps do not preserve the JSON text's insertion order, which is the one thing this
// cannot reproduce.
func formatLiteral(v any) string {
	switch t := v.(type) {
	case nil:
		return "None"
	case bool:
		if t {
			return "True"
		}
		return "False"
	case string:
		return quoteLiteral(t)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(t), 'g', -1, 32)
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			parts = append(parts, formatLiteral(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, quoteLiteral(k)+": "+formatLiteral(t[k]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return quoteLiteral(fmt.Sprint(v))
	}
}

// displayText is the text of a value: a string is itself, every other value comes back as
// its literal (containers and scalars included).
func displayText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return formatLiteral(v)
}

// asInt converts the values a slot table can carry, and reports ok=false where the
// conversion is impossible: a string that is not a plain integer ("5.5" fails), None, or a
// container.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
			return 0, false
		}
		return int(n), true // truncates toward zero
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return 0, false
		}
		return int(n), true
	case bool:
		// A bool IS an integer here, the way the protocol's literals treat it: true == 1.
		if n {
			return 1, true
		}
		return 0, true
	case string:
		// A quoted integer may carry surrounding whitespace and "_" separators.
		s := strings.ReplaceAll(strings.TrimSpace(n), "_", "")
		if s == "" {
			return 0, false
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			return 0, false
		}
		return i, true
	}
	return 0, false
}

// asStringList reads a list-ish value as text: a falsy value yields nothing, a list yields
// its items rendered as text, a string yields its characters, and a map yields its keys
// (sorted). ok=false when the value cannot be read as a list at all.
func asStringList(v any) ([]string, bool) {
	if !isTruthy(v) {
		return nil, true
	}
	switch t := v.(type) {
	case []any:
		out := make([]string, 0, len(t))
		for _, item := range t {
			out = append(out, displayText(item))
		}
		return out, true
	case []string:
		return append([]string(nil), t...), true
	case string:
		out := make([]string, 0, len(t))
		for _, r := range t {
			out = append(out, string(r))
		}
		return out, true
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys, true
	default:
		return nil, false
	}
}

// ===================== extract.go =====================
// JSON / terminal-block extraction from model output.
//
// Models wrap JSON in thinking preamble and Markdown fences, and often emit a second
// object after the first — a greedy regex over-captures there and the parse then fails
// with "Extra data", so the extractor brace-matches to isolate ONE complete object and
// walks to the next "{" when that one is invalid.

// ExtractJSON returns the first parseable JSON value found in text, or nil.
func ExtractJSON(text string) any {
	if text == "" {
		return nil
	}
	n := len(text)
	for i := 0; i < n; {
		start := strings.IndexByte(text[i:], '{')
		if start < 0 {
			break
		}
		start += i
		depth := 0
		for j := start; j < n; j++ {
			switch text[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := text[start : j+1]
					var out any
					if err := json.Unmarshal([]byte(candidate), &out); err == nil {
						return out
					}
					// First try a lenient parse, then the repair pass; only if BOTH fail
					// does the scan move on to the next opening brace.
					if v := repairJSONObject(candidate); v != nil {
						return v
					}
					if v := repairJSONWhole(candidate); v != nil {
						return v
					}
					// Invalid object; try the next "{".
				}
			}
		}
		i = start + 1
	}
	// All brace-matched candidates failed strict and shallow repair. The last resort is a
	// whole-text repair, which can salvage deformities (unquoted keys, dropped separators, a
	// lost outer brace) that no single brace-matched candidate can. Mirror it.
	return repairJSONWhole(text)
}

// ExtractJSONObject parses the first JSON object/array found in text, but ONLY
// from a brace/bracket-matched region. It deliberately stops short of
// repairJSONWhole's whole-text prose fallback: prose with no JSON shape (e.g.
// "I cannot produce JSON today.") returns nil so the caller can treat it as a
// parse failure. This is the right gate for gen_json-style loops, where the
// model is expected to emit JSON (possibly fenced or trailing-comma damaged)
// and a non-JSON reply must trigger a corrective retry rather than be silently
// coerced into a value. Keywords-style salvage of prose-wrapped JSON should use
// ExtractJSON instead.
func ExtractJSONObject(text string) any {
	if text == "" {
		return nil
	}
	n := len(text)
	for i := 0; i < n; {
		start := strings.IndexByte(text[i:], '{')
		if start < 0 {
			break
		}
		start += i
		depth := 0
		for j := start; j < n; j++ {
			switch text[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := text[start : j+1]
					var out any
					if err := json.Unmarshal([]byte(candidate), &out); err == nil {
						return out
					}
					if v := repairJSONObject(candidate); v != nil {
						return v
					}
					// Invalid object; try the next "{".
				}
			}
		}
		i = start + 1
	}
	return nil
}

// repairJSONObject is a conservative repair pass applied only after a candidate fails
// strict json.Unmarshal. It walks cheap, idempotent normalizations that models commonly
// emit and re-validates after each: trailing commas, single-quoted strings, NaN/Infinity
// numeric literals, and illegal control characters inside string literals. It never
// rewrites already-valid JSON and returns nil when no transform yields a parse.
func repairJSONObject(candidate string) any {
	steps := []struct {
		name  string
		apply func(string) string
	}{
		{"dropTrailingCommas", dropTrailingCommas},
		{"normalizeSingleQuotes", normalizeSingleQuotes},
		{"nullNonFinite", nullNonFiniteLiterals},
		{"stripRawControlChars", stripRawControlChars},
	}
	fixed := candidate
	for _, s := range steps {
		fixed = s.apply(fixed) // accumulate so multi-step repairs compose
		var out any
		if err := json.Unmarshal([]byte(fixed), &out); err == nil {
			return out
		}
	}
	return nil
}

// dropTrailingCommas removes a comma immediately before a closing brace or
// bracket (e.g. {"a": 1,} or [1, 2,]), repeated until stable.
func dropTrailingCommas(s string) string {
	for {
		before := s
		s = strings.ReplaceAll(s, ",}", "}")
		s = strings.ReplaceAll(s, ",]", "]")
		if s == before {
			return s
		}
	}
}

// normalizeSingleQuotes rewrites single-quoted string literals to double-quoted
// ones, mirroring json_repair. It only touches quotes outside of double-quoted
// strings so it never corrupts existing valid JSON.
func normalizeSingleQuotes(s string) string {
	var b strings.Builder
	inDouble := false
	inSingle := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			b.WriteByte(ch)
			escaped = false
			continue
		}
		switch ch {
		case '\\':
			escaped = true
			b.WriteByte(ch)
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
			b.WriteByte(ch)
		case '\'':
			if !inDouble {
				inSingle = !inSingle
				b.WriteByte('"') // replace single quote with double quote
			} else {
				b.WriteByte(ch)
			}
		default:
			b.WriteByte(ch)
		}
	}
	return b.String()
}

// nullNonFiniteLiterals rewrites bare NaN / Infinity / -Infinity tokens into JSON null so
// the object can parse.
func nullNonFiniteLiterals(s string) string {
	s = strings.ReplaceAll(s, ": NaN", ": null")
	s = strings.ReplaceAll(s, ": Infinity", ": null")
	s = strings.ReplaceAll(s, ": -Infinity", ": null")
	return s
}

// stripRawControlChars removes raw ASCII control characters (other than tab, newline and
// carriage-return) that would otherwise make Go's json.Unmarshal reject a string literal.
func stripRawControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return -1
		}
		return r
	}, s)
}

// repairJSONWhole is the deep fallback applied to the WHOLE text (not just one
// brace-matched candidate). ExtractJSON first tries the brace-matched object and the
// shallow repairJSONObject steps; only when BOTH fail does it reach here. This salvages
// structural deformities the single-object path cannot — unquoted object keys ({a: 1}),
// missing separators between adjacent values ({"a":1} {"b":2} or "x" "y"), bare string
// values ({name: foo}), and any combination the model emits across the full output. It
// delegates to github.com/kaptinlin/jsonrepair. It returns nil when nothing parses.
func repairJSONWhole(text string) any {
	cleaned := reThinkWrap.ReplaceAllString(text, "")
	cleaned = reFencedJSON.ReplaceAllString(cleaned, "$1")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return nil
	}
	// Pre-normalize unquoted object keys (e.g. {entity: "x"} -> {"entity": "x"}).
	// The repair library handles unquoted keys with unquoted values well but mangles a key
	// that precedes an already-quoted value; quoting keys first sidesteps that.
	// Already-valid JSON is left untouched.
	normalized := quoteUnquotedKeys(cleaned)
	repaired, err := jsonrepair.Repair(normalized)
	if err != nil {
		// Fall back to the un-normalized input in case the heuristic mis-fired.
		repaired, err = jsonrepair.Repair(cleaned)
		if err != nil {
			return nil
		}
	}
	return tryUnmarshal(repaired)
}

// quoteUnquotedKeys wraps bare object keys in double quotes: {a: 1} -> {"a": 1}.
// It only touches a key token (identifier directly followed by a colon) that is
// not already quoted, so already-valid JSON is left untouched. The optional
// brace/bracket/comma prefix is preserved verbatim.
func quoteUnquotedKeys(s string) string {
	return reUnquotedKey.ReplaceAllStringFunc(s, func(m string) string {
		groups := reUnquotedKey.FindStringSubmatch(m)
		return groups[1] + `"` + groups[2] + `":`
	})
}

// reUnquotedKey matches a structural prefix ({ [ ,) plus optional whitespace,
// then a bare identifier immediately followed by a colon — the shape of an
// unquoted JSON key. Group 1 is the prefix, group 2 is the key name.
var reUnquotedKey = regexp.MustCompile(`([{\[,]\s*)([A-Za-z_][A-Za-z0-9_]*)\s*:`)

// tryUnmarshal attempts json.Unmarshal and returns the value, or nil on failure.
func tryUnmarshal(s string) any {
	if s == "" {
		return nil
	}
	var out any
	if err := json.Unmarshal([]byte(s), &out); err == nil {
		return out
	}
	return nil
}

var reFencedJSON = regexp.MustCompile("(?s)```(?:json)?\\s*(\\{.*?\\}|\\[.*?\\])\\s*```")

// ExtractTag: exact-tag extraction first, then
// lenient fallbacks for models that wrap the JSON in code fences or emit bare
// objects (observed with DeepSeek-class models ignoring the XML protocol).
//
// Returns "" when the tag is absent.
func ExtractTag(text, tag string) string {
	if text == "" {
		return ""
	}
	open, closeTag := "<"+tag+">", "</"+tag+">"
	s := strings.LastIndex(text, open)
	e := strings.LastIndex(text, closeTag)
	if s >= 0 && e > s {
		return strings.TrimSpace(text[s+len(open) : e])
	}
	// Fenced ```json ... ``` blocks mentioning the tag name.
	fenced := reFencedJSON.FindAllStringSubmatch(text, -1)
	if len(fenced) == 0 {
		if m, _ := regexp.MatchString(`<`+regexp.QuoteMeta(tag)+`\b`, text); !m {
			return ""
		}
		return ""
	}
	want := map[string]bool{"new_states": true}
	if tag != "state" {
		want = map[string]bool{"answer": true, "new_state": true}
	}
	// Prefer the LAST block (most recent decision).
	for i := len(fenced) - 1; i >= 0; i-- {
		var obj map[string]any
		if err := json.Unmarshal([]byte(fenced[i][1]), &obj); err != nil {
			continue
		}
		for k := range obj {
			if want[k] {
				return strings.TrimSpace(fenced[i][1])
			}
		}
	}
	return ""
}

// ===================== toolschema.go =====================
// Tool schemas, the per-session tool object, and the two seams the action
// session depends on.
//
// What the session depends on:
//   - tool specs
//   - ToolMap (the dispatch registry)
//   - the active tool surface
//   - tool disabling
//   - reason → status mapping
//   - execute_tool      (line 1003)

func arrayParam(desc string, minItems, maxItems int) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":     "array",
				"items":    map[string]any{"type": "string"},
				"minItems": minItems,
				"maxItems": maxItems,
			},
		},
		"required": []string{"query"},
	}
}

// Tool schemas. The descriptions are the model's only guide for when to pick which tool,
// and rephrasing them changes routing behaviour.
var (
	retrieveToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "retrieve",
			Description: `WHEN TO CALL: you know or suspect exact surface terms in the corpus (names, titles, codes, phrases) — the first recall pass; cover different facets.` +
				`HOW IT WORKS: keyword recall FIRST, then the pattern is applied to what came back — ` + "`A.*B`" + ` (A then B, anything between) matches only inside the passages its operands recalled, and the RAREST operand bounds it: put the rare word first. A FULL recall page was truncated.` +
				`ONE STRING, MANY TERMS: ` + "`A|B|C`" + ` recalls each term in ONE call — synonyms belong in one string, not one call each.` +
				`ENUMERATING A SET: probe the NAMES themselves, alternated with |, 4-6 per query. Hits are members; guess the next batch yourself rather than stopping at what you hold.` +
				`DO NOT CALL: for a whole document (list_chunks); when no surface word matches (search_chunks).` +
				`ARGUMENTS: query — array of 1-3 strings; only a query's first ~10 snippets survive; doc_scope is NOT declared.` +
				`OUTPUT: Exact-term snippets with doc_id and chunk id. ok = new evidence; redundant = seen.` +
				`IF IT FAILS: a miss on an exact-term probe means the corpus lacks that term — in an enumeration that is a RESULT (record it as not a member, probe the next). redundant = stop and emit a state patch.`,
			Parameters: arrayParam("", 1, 3),
		},
	}

	listChunksToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "list_chunks",
			Description: `WHEN TO CALL: You need the FULL text of one document (enumeration, counts, arithmetic over many passages) and you already have its doc_id from a prior tool result.` +
				`DO NOT CALL: When you only need a single passage (use search_chunks or retrieve first); when you have no doc_id yet (locate it via navigate_tree or search_chunks first).` +
				`ARGUMENTS: doc_id — string, the document id seen in a retrieve / search_chunks / navigate result. ONLY doc_id is accepted; there is no chunk_ids argument, and the tool returns the whole document (capped at 30 chunks).` +
				`OUTPUT: All chunks of the document in reading order. ok = new evidence; redundant = already in pool.` +
				`IF IT FAILS: An unknown or blank doc_id yields an empty result (query-level miss, not a dataset fact) — pick a different doc_id or locate one first. Do not treat this as a reason to disable the tool.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"doc_id": map[string]any{
						"type":        "string",
						"description": "document id seen in a retrieve snippet",
					},
				},
				"required": []string{"doc_id"},
			},
		},
	}

	searchChunksToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "search_chunks",
			Description: `WHEN TO CALL: Primary semantic recall. Use when exact retrieve returns nothing useful, when the corpus is large and you are unsure which document holds the answer, or when the answer passage shares no surface words with your query. Send 1-2 queries; compiled-structure expansion is automatic (a no-op without compiled structure). ` +
				`DO NOT CALL: When you already have a doc_id and want to read that document (use list_chunks); when a single exact passage would be found faster by grep-style retrieve.` +
				`ARGUMENTS: query — array of 1-2 strings.` +
				`OUTPUT: Relevance-ranked snippet chunks, possibly with structural neighbours appended. ` +
				`Results may LEAD with [claim score=...] entries — the dataset's compiled atomic facts carrying VERBATIM source quotes. If a claim directly answers the query, cite it and answer WITHOUT further searching; deep-read its listed chunk only for missing context or numbers. ` +
				`ok = new evidence; redundant = already seen.` +
				`IF IT FAILS: miss means this query matched nothing — change the angle or fall back to retrieve or navigate_tree. Re-issuing a near-duplicate query is skipped as redundant, so vary the query instead of paraphrasing it.`,
			Parameters: arrayParam("", 1, 2),
		},
	}

	webSearchToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "web_search",
			Description: `WHEN TO CALL: The needed fact is world knowledge, a recent event, or newer than the corpus (a current event, a person's alive-now status, a fresh statistic). This tool only appears when a web provider is configured.` +
				`DO NOT CALL: When the fact plausibly lives in the fixed corpus — prefer retrieve or search_chunks first. For corpus-only questions this tool is unavailable.` +
				`ARGUMENTS: query — array of 1-2 strings.` +
				`OUTPUT: Web results shaped like corpus chunks, merged into the same evidence pool.` +
				`IF IT FAILS: error (no provider) — it will not appear at all this session; if it does appear and fails, switch to corpus tools permanently and do not retry it.`,
			Parameters: arrayParam("", 1, 2),
		},
	}

	// wikiQueryToolSpec is deliberately absent from ToolMap: wiki_query has no caller and is
	// not part of the action session's dispatch registry, so it stays unregistered. The
	// handler still lives in tool_exploration.go as an unplugged extension seam.

	navigateTreeToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "navigate_tree",
			Description: `WHEN TO CALL: The question names a topic, entity, or alias but you do NOT know which document discusses it, especially on a large corpus. Routes by topic or cluster similarity over the compiled navigation tree.` +
				`DO NOT CALL: When you already hold a doc_id (go straight to navigate_structure); when the answer is likely a single exact passage (use retrieve or search_chunks).` +
				`ARGUMENTS: query — string, the topic / entity / alias whose document(s) to locate. Note: keywords is read by the executor but is NOT a declared parameter; do not pass it.` +
				`OUTPUT: Candidate doc_ids plus a first-chunk summary of each; these become your known-docs set for the next step.` +
				`IF IT FAILS: empty (no_structure) means the dataset has no compiled navigation tree — immediately switch to search_chunks. A second such empty disables this tool for the rest of the session, so do not retry it.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "topic / entity / alias whose document(s) to locate",
					},
				},
				"required": []string{"query"},
			},
		},
	}

	navigateStructureToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "navigate_structure",
			Description: `WHEN TO CALL: You know the doc_id and need to PINPOINT where the answer lives inside that one document, without reading every chunk. The in-document counterpart of navigate_tree.` +
				`DO NOT CALL: When you have no doc_id yet; when the document has no compiled structure (use list_chunks to read the full document).` +
				`ARGUMENTS: doc_id — string, required. query — string, what to locate within the document. kind — enum catalog / mindmap / graph, default catalog (compiled-structure kind).` +
				`OUTPUT: The structure outline annotated with matching chunk_ids, reading-order aware. [claim] lines carry VERBATIM quotes from the document — cite them and answer WITHOUT calling list_chunks when they directly answer the query (deep-read the claimed chunk ids only for surrounding context or numbers the quotes lack). ` +
				`ok = useful hits; poor (chunk_ptrs = 0) means it drilled to nothing usable.` +
				`IF IT FAILS: empty (no_structure) — try another doc_id or kind, or fall back to list_chunks / search_chunks. poor — read the full document via list_chunks(doc_id). A second empty disables the tool for the session.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"doc_id": paramString("document id seen from a prior tool result"),
					"query":  paramString("what to locate within the document"),
					"kind":   paramEnum("compiled structure kind, default catalog", "catalog", "mindmap", "graph"),
				},
				"required": []string{"doc_id"},
			},
		},
	}

	calculateToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "calculate",
			Description: `WHEN TO CALL: The question asks you to DERIVE a number by combining facts you found (sum / difference / percentage / ratio / sort / compare / length / age / price / area / growth). NEVER do arithmetic mentally.` +
				`DO NOT CALL: When the answer IS one of the stated numbers (no combination needed) — answer directly. When a needed number is still missing — retrieve it first; do not estimate.` +
				`ARGUMENTS: question — string, the user question verbatim. facts — array of strings, the numbers or facts found in evidence, verbatim (keep the original language; pass them exactly as written).` +
				`OUTPUT: an object with expression and result — report the computed result verbatim.` +
				`IF IT FAILS: poor (no numeric answer derivable) — retrieve more numbers, or answer directly if the answer is already stated. Never fabricate a computation.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": paramString("the user's question, verbatim"),
					"facts": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "numbers/facts found in the evidence, verbatim",
					},
				},
				"required": []string{"question", "facts"},
			},
		},
	}

	graphExploreToolSpec = ToolSpec{
		Type: "function",
		Function: ToolFunction{
			Name: "graph_explore",
			Description: ("EXPLORE the compiled KNOWLEDGE GRAPH (entities + relations) for a " +
				"RELATIONAL/multi-hop answer. Different from navigate_*: instead of " +
				"locating a document or passage, it seeds entities for the query, " +
				"hops along their RELATIONS, and returns either a direct answer or " +
				"the source passages behind the relevant entities/relations. " +
				"Use when the answer requires connecting several entities through " +
				"their relations (e.g. who-was-related-to-whom, cause-effect chains, " +
				"membership/ownership) and you already have a starting entity from a " +
				"search result, a navigation outline, or a list_chunks reading. " +
				"If the dataset has NO compiled knowledge graph, it returns empty — " +
				"fall back to search_chunks / navigate_structure."),
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": paramString("the relational question / starting entity"),
					"doc_scope": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "optional doc_ids to restrict the graph to (from prior navigation/list_chunks)",
					},
				},
				"required": []string{"query"},
			},
		},
	}
)

func paramString(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func paramEnum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "enum": values, "description": desc}
}

// ToolMap is the multi-tool registry.
// executeTool dispatches by name; add a tool by registering its schema here.
var ToolMap = map[string]ToolSpec{
	"retrieve":           retrieveToolSpec,
	"search_chunks":      searchChunksToolSpec,
	"list_chunks":        listChunksToolSpec,
	"navigate_tree":      navigateTreeToolSpec,
	"navigate_structure": navigateStructureToolSpec,
	"calculate":          calculateToolSpec,
	"graph_explore":      graphExploreToolSpec,
	"web_search":         webSearchToolSpec,
}

// ToolMapNames are the registered tool names, sorted for deterministic output.
func ToolMapNames() []string {
	names := make([]string, 0, len(ToolMap))
	for n := range ToolMap {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The per-session tool object

// ToolExecutor runs one tool call by name and reports what happened.
//
// Every executor returns a ToolOutcome so the tool node can act on the KIND of
// result (empty / miss / poor / redundant / error) instead of measuring payload
// size.
type ToolExecutor interface {
	Execute(ctx context.Context, name string, args map[string]any) (ToolOutcome, error)
}

// Toolset is the per-session tool object: thinking mode, web provider availability, the
// runtime-disabled tool set, and the executor.
//
// The disabled-tool state lives here rather than on the request object, so a session
// cannot leak it into another request.
type Toolset struct {
	// ThinkingMode selects the ModeSpec (see config.go).
	ThinkingMode string
	// HasWebSearch reports whether a web provider is configured. When false,
	// web_search is hidden from the surface rather than merely discouraged.
	HasWebSearch bool
	// DisabledTools holds tools proven unavailable this session (no compiled
	// structure of their kind).
	DisabledTools map[string]bool
	// mu guards DisabledTools: one Toolset is shared by a session's concurrent
	// rag calls (models.appendToolResults runs a round's tool calls in parallel),
	// so the map must not be written and read unsynchronized.
	mu sync.Mutex
	// Exec runs the tools.
	Exec ToolExecutor
}

// GetThinkingMode implements ThinkingModeCarrier so ResolveMode works on *Toolset.
func (t *Toolset) GetThinkingMode() string {
	if t == nil {
		return ""
	}
	return t.ThinkingMode
}

// ActiveToolSpecs: the tool schemas exposed
// to the model for THIS mode.
//
// Visibility rules:
//   - the per-mode tool set comes from config.THINKING_MODES (only ultra sees
//     the relational graph_explore);
//   - web_search is hidden when no web provider is configured — the model
//     otherwise retries it several times per session burning turns;
//   - compile-only tools stay visible even on datasets without compiled
//     structure: their executors return an explicit empty-with-hint so the
//     model falls back to plain search;
//   - RUNTIME: a compile-only tool is removed after it proves the dataset has
//     no such compiled structure (DisableTool).
func (t *Toolset) ActiveToolSpecs() []ToolSpec {
	spec := ResolveMode(t)
	var out []ToolSpec
	for _, name := range spec.ToolNames() {
		if name == "web_search" && !t.HasWebSearch {
			continue
		}
		if t.IsDisabled(name) {
			continue
		}
		if s, ok := ToolMap[name]; ok {
			out = append(out, s)
		}
	}
	return out
}

// DisableTool: mark a compile-only tool
// unavailable for the REST of this session, so ActiveToolSpecs stops
// advertising it and ExecuteTool short-circuits it.
func (t *Toolset) DisableTool(name string) {
	if _, ok := ToolMap[name]; !ok {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.DisabledTools == nil {
		t.DisabledTools = map[string]bool{}
	}
	t.DisabledTools[name] = true
}

// IsDisabled reports whether name has been disabled this session.
func (t *Toolset) IsDisabled(name string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.DisabledTools[name]
}

// ReasonStatus: single source of truth for the cause→status mapping, so a tool cannot
// disagree with itself about what its own reason means.
// The unified tool-result contract shared by the session and the tool implementations
// (ToolOutcome plus the OK/EMPTY/MISS/POOR/REDUNDANT/ERROR statuses).

// Tool-result statuses: the signal the session's tool node acts on.
const (
	StatusOK        = "ok"        // normal hit
	StatusEmpty     = "empty"     // dataset-level: no such compiled structure exists here
	StatusMiss      = "miss"      // query-level: nothing matched THIS query; tool still valid
	StatusPoor      = "poor"      // produced output, but too weak to be useful
	StatusRedundant = "redundant" // ran fine, but added no NEW evidence
	StatusError     = "error"     // infra / provider failure
)

// Machine-readable ToolOutcome reasons. Only ReasonNoStructure is
// DATASET-level and may disable a tool; ReasonNoDoc is QUERY-level and must not.
// ReasonUnwired labels a call to a tool NAME this deployment has no binding for
// (the model invented it): nothing ran, so it is reported as MISS and never
// counts toward the strikes that disable a real tool.
const (
	ReasonNone        = ""
	ReasonNoStructure = "no_structure"
	ReasonNoDoc       = "no_doc"
	ReasonInfra       = "infra"
	ReasonBadArgs     = "bad_args"
	ReasonUnwired     = "unwired"
)

// ToolOutcome is the result of ONE tool call. Every executor returns one so the
// tool node can inspect WHAT happened instead of counting characters in a JSON
// string — timeout / budget / routing / disable policies hook into these fields.
type ToolOutcome struct {
	Payload     []any          // what the model sees (passage dicts)
	EvidenceIDs []string       // doc/chunk ids worth tracking
	Status      string         // one of the Status* constants
	Reason      string         // one of the Reason* constants
	Metrics     map[string]any // numeric signals: hits, new_evidence, top_score, ...
	// Note is a line ABOUT this call, rendered into the tool result next to the
	// passages (see toolNode). It carries what the passages cannot say: which of
	// the terms the call asked about were reached and which were not, so a batch
	// probe ("华雄|颜良|蔡阳") is actionable rather than four passages whose
	// missing member looks exactly like a member nobody asked for.
	Note string
	// Diagnostic is the tool's own explanation of a failure (Go-only: Python's
	// ToolOutcome carries no such field). A bare Reason — "infra" — tells a
	// reader nothing about what went wrong, so the underlying message rides
	// along and is reported with the step. It is NEVER put in Payload: the model
	// sees the status note, not the infrastructure detail.
	Diagnostic string
}

// NewToolOutcome builds an OK outcome with an empty metric map.
func NewToolOutcome(payload []any, evidenceIDs []string) ToolOutcome {
	return ToolOutcome{
		Payload:     payload,
		EvidenceIDs: evidenceIDs,
		Status:      StatusOK,
		Reason:      ReasonNone,
		Metrics:     map[string]any{},
	}
}

// ReasonStatus maps a ToolOutcome reason to the Status it implies.
func ReasonStatus(reason string) string {
	switch reason {
	case ReasonNoStructure:
		return StatusEmpty
	case ReasonBadArgs, ReasonInfra:
		return StatusError
	}
	// ReasonNoDoc: this query reached nothing; the tool itself is fine.
	return StatusMiss
}

// ===================== session.go (model seam, _SessionState, nodes, routing) =====================
// The graph-edge action session: one bounded ReAct loop pursuing ONE direction.
// THIS FILE USES AN EXPLICIT LOOP, NOT A COMPILED GRAPH: the graph is small and fixed
// (see sessionLoop), so three functions and a switch buy more than compiling one. The node
// bodies, routing predicates and their ordering follow the same design; only the driver
// differs.
//
// The provider seam calls the native tool-calling API (`tools=[...]`,
// `tool_choice="auto"`) and parses `message.tool_calls`: the chat seam exposes that as
// chat.Request.Tools / Response.ToolCalls, and the SessionModel implementation
// (InvokerSessionModel) forwards the runtime ToolSpec surface through it. There is no
// prompt-based fallback: downstream session logic is identical either way.

const (
	// initTimeoutS bounds the slot-table decomposition call.
	initTimeoutS = 45.0
	// actionTimeoutS is the default per-session wall-clock budget.
	actionTimeoutS = 75.0
	// setActionTimeoutS is the wall clock an ENUMERATION session gets, i.e. a
	// session whose direction declared a set (see SessionWallS). The extra time is
	// not a bigger turn budget — the turns are what the enumeration spends, and one
	// of them is a batch of names whose results have to be read and patched: a session can be
	// cancelled at its wall clock a millisecond before the patch that would have recorded a
	// member it had already reached — its passages admitted to the shared pool by its own
	// batch. A value question has nothing to spend the extra time on, so it keeps the tighter
	// clock.
	setActionTimeoutS = 150.0
	// snippetsPerQuery is the FALLBACK per-query snippet cap, used when the mode
	// leaves ModeSpec.SnippetsPerQuery unset. The live value is
	// snippetsPerQueryFor(RunRequest.ThinkingMode).
	snippetsPerQuery = 4
	// evidenceDigestChars bounds ONE chunk in the session seed's ALREADY RETRIEVED
	// digest. It matches what an admitted passage carries (passageFromChunk: 1200), so
	// the digest never shows a session LESS of a chunk than the same chunk would carry
	// as a tool result.
	//
	// The previous 300-code-point cut ended mid-sentence on narrative passages, and the
	// model — told to answer only from what it was shown — excluded the parties whose clause
	// fell outside the window: a cap smaller than one sentence truncates exactly the evidence
	// the answer is built from, even when the passage is in the pool.
	evidenceDigestChars = 1200
	// maxToolResponseChars bounds ONE tool payload.
	maxToolResponseChars = 12000
	// emptyStrikes is how many dataset-level empties (reason=no_structure) a
	// compiled-structure tool must accumulate before being disabled for the rest
	// of the session. Kept above 1: a single empty can be SCOPED — graph_explore
	// over a doc_scope that has no knowledge graph says nothing about the other
	// documents, and disabling on the first empty is exactly the over-eager
	// behaviour this replaced.
	emptyStrikes = 2
	// nearDupJaccard is the token-overlap threshold at which a retrieval query
	// counts as a paraphrase of an already-run query. The model re-issues the
	// SAME intent with many paraphrases (observed: 20+ variants in ONE session),
	// each hitting the index even though the evidence is unchanged.
	nearDupJaccard = 0.8
	// skippedDupLimit: once this many near-duplicate retrievals were skipped,
	// further turns are unlikely to surface new evidence, so the session
	// converges to finalize instead of burning the remaining budget on
	// paraphrases (Q30/Q759 timeout root cause).
	skippedDupLimit = 2
	// turnRunExtra is how many turns the MODEL may add beyond the mode's floor
	// (see offerContinuation): medium/high run 8 → 12, ultra 10 → 14. It is the
	// hard bound the runtime keeps while the decision itself is the model's, so an
	// eager model cannot turn one session into a whole research programme.
	turnRunExtra = 4
	// valueTurnFloor is the floor a session runs on when it is NOT assembling a set
	// (see actionMaxTurns). It is the floor every mode had before the enumeration
	// path asked for a larger one (medium/high 4 → 8, ultra 6 → 10): the extra turns
	// exist to spend a batch of NAMES turn after turn, and a session that never
	// writes one has no batch to spend them on — while the mode's number is billed
	// to every turn of every session on the question. A raised floor gives a batch-writing
	// session the turns it needs, and a question that writes almost no batches pays it on
	// every session.
	valueTurnFloor = 4
	// turnAskFloorS is the session clock below which no further turn is offered:
	// the finalize/salvage step must still fit, or the extra turn buys evidence
	// that never reaches the slot table.
	turnAskFloorS = 20.0
	// finalizeTimeout bounds the salvage call in the finalize node.
	finalizeTimeoutS = 150.0
	// minFinalizeTimeout is the floor for the salvage call.
	minFinalizeTimeoutS = 15.0
)

// snippetsPerQueryFor resolves the per-query snippet cap for a thinking mode.
//
// The cap decides how much of ONE query's candidate list the session reads, and
// that list is where a themed query's fact-bearing passage sits: the engine
// returns 30-60 candidates per leg, and on a 64-candidate leg the passage that
// carried the answer ranked 20th and 38th — both discarded by a cap of four,
// while handing the model those same passages produced the complete list.
//
// It therefore rises with the mode (a deeper mode issues more queries and owns a
// larger budget) and falls back to the flat snippetsPerQuery when the mode leaves
// it unset, so a mode that never declared one behaves exactly as before.
func snippetsPerQueryFor(mode string) int {
	if spec := GetMode(mode); spec.SnippetsPerQuery > 0 {
		return spec.SnippetsPerQuery
	}
	return snippetsPerQuery
}

// retrievalTools: near-duplicate suppression applies to these. grep_chunks / grep_search
// are legacy names kept so a model emitting them is still deduped rather than executed.
var retrievalTools = map[string]bool{
	"search_chunks": true,
	"grep_chunks":   true,
	"grep_search":   true,
}

var _LOG = common.StdLogger()

var reSearchToken = regexp.MustCompile(`[a-z0-9]{2,}`)

// searchTokens: lowercased alphanumeric tokens
// of a query, for near-duplicate detection.
func searchTokens(q string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, t := range reSearchToken.FindAllString(strings.ToLower(q), -1) {
		out[t] = struct{}{}
	}
	return out
}

// argQueryString coerces a tool argument to a trimmed query string: a string passes
// through, a nil/empty value yields "", and any other value — typically the []any query
// list retrieve accepts — is stringified so it still participates in near-dup detection
// and the seen_queries ledger.
func argQueryString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		if t == "" {
			return ""
		}
		return strings.TrimSpace(t)
	case []any:
		if len(t) == 0 {
			return ""
		}
		parts := make([]string, 0, len(t))
		for _, e := range t {
			parts = append(parts, fmt.Sprintf("%v", e))
		}
		return strings.TrimSpace("[" + strings.Join(parts, ", ") + "]")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

// IsNearDup: true when q shares >=
// nearDupJaccard of its tokens with any query in seen.
func IsNearDup(q string, seen []string) bool {
	if strings.TrimSpace(q) == "" || len(seen) == 0 {
		return false
	}
	toks := searchTokens(q)
	if len(toks) < 2 {
		return false
	}
	for _, s := range seen {
		other := searchTokens(s)
		inter := 0
		for t := range toks {
			if _, ok := other[t]; ok {
				inter++
			}
		}
		union := len(toks)
		for t := range other {
			if _, ok := toks[t]; !ok {
				union++
			}
		}
		if union > 0 && float64(inter)/float64(union) >= nearDupJaccard {
			return true
		}
	}
	return false
}

// The model seam

// The production carrier always supports per-call temperatures: the pinned per-node
// temperatures (keywords 0.1 / structure_qa 0.2 / compute 0.0) must never silently fall
// back to a model default.
var _ TemperatureModel = (*InvokerSessionModel)(nil)

// CompleteWithTemperature implements TemperatureModel.
func (m *InvokerSessionModel) CompleteWithTemperature(ctx context.Context, messages []schema.Message, tools []ToolSpec, temp float64) (*ModelReply, error) {
	if m == nil || m.Invoker == nil {
		return nil, fmt.Errorf("runtime: no chat invoker configured")
	}
	// The tools are bound natively (tool_choice="auto") and the result is read out of
	// msg.tool_calls. There is no prompt-based fallback: a provider that cannot express
	// native tools simply has no tool capability.
	req := chat.Request{Messages: messages, Temperature: &temp}
	if chatTools := toChatTools(tools); chatTools != nil {
		req.Tools = chatTools
		req.ToolChoice = chat.ToolChoiceAuto
	}
	resp, err := m.Invoker.Invoke(ctx, m.DB, req)
	if err != nil {
		return nil, err
	}
	reply := &ModelReply{Content: resp.Content}
	if resp != nil {
		reply.Usage = resp.Usage
		reply.Tokens = totalOf(resp)
		reply.ToolCalls = nativeToHarnessCalls(resp.ToolCalls)
	}
	return reply, nil
}

// InvokerSessionModel adapts chat.Invoker to SessionModel using the provider's NATIVE
// tool calling (native tools with tool_choice="auto", reading back msg.tool_calls).
//
// There is no prompt-based fallback: a provider that cannot express native tools simply
// has no tool capability. A reply with no tool_calls is a no-call turn, which the session
// nudges.
type InvokerSessionModel struct {
	Invoker chat.Invoker
	DB      *gorm.DB
	// MaxLength is the resolved chat model's context window in tokens. Used by
	// ContextLength to size the prompt fit. <= 0 means "unknown", in which case
	// ContextLength reports chat.EffectiveContextLength's 8192 fallback.
	MaxLength int
}

// ContextLength implements ContextLengthModel. It returns the model's context
// window in tokens when known, otherwise the 8192 default used when the config omits a
// context length.
func (m *InvokerSessionModel) ContextLength() int {
	if m == nil {
		return chat.EffectiveContextLength(0)
	}
	return chat.EffectiveContextLength(m.MaxLength)
}

// Complete implements SessionModel.
func (m *InvokerSessionModel) Complete(ctx context.Context, messages []schema.Message, tools []ToolSpec) (*ModelReply, error) {
	if m == nil || m.Invoker == nil {
		return nil, fmt.Errorf("runtime: no chat invoker configured")
	}
	temp := 0.3
	req := chat.Request{Messages: messages, Temperature: &temp}
	if chatTools := toChatTools(tools); chatTools != nil {
		req.Tools = chatTools
		req.ToolChoice = chat.ToolChoiceAuto
	}
	resp, err := m.Invoker.Invoke(ctx, m.DB, req)
	if err != nil {
		return nil, err
	}
	reply := &ModelReply{Content: resp.Content}
	if resp != nil {
		reply.Usage = resp.Usage
		reply.Tokens = totalOf(resp)
		reply.ToolCalls = nativeToHarnessCalls(resp.ToolCalls)
	}
	return reply, nil
}

// StreamComplete implements StreamingSessionModel by delegating to the
// underlying invoker when it can stream. A non-streaming invoker yields an
// error, which the caller treats as "fall back to the one-shot call" — the
// prompt is identical either way, so the answer is not affected.
func (m *InvokerSessionModel) StreamComplete(ctx context.Context, messages []schema.Message, tools []ToolSpec, onDelta func(delta string, isThink bool) error) (*ModelReply, error) {
	if m == nil || m.Invoker == nil {
		return nil, fmt.Errorf("runtime: no chat invoker configured")
	}
	streamer, ok := m.Invoker.(chat.StreamingInvoker)
	if !ok {
		return nil, fmt.Errorf("runtime: chat invoker %T does not support streaming", m.Invoker)
	}
	temp := 0.3
	req := chat.Request{Messages: messages, Temperature: &temp}
	if chatTools := toChatTools(tools); chatTools != nil {
		req.Tools = chatTools
		req.ToolChoice = chat.ToolChoiceAuto
	}
	resp, err := streamer.Stream(ctx, m.DB, req, onDelta)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("runtime: streaming invoker returned no response")
	}
	reply := &ModelReply{
		Content: resp.Content,
		Usage:   resp.Usage,
		Tokens:  totalOf(resp),
	}
	reply.ToolCalls = nativeToHarnessCalls(resp.ToolCalls)
	return reply, nil
}

// totalOf returns the total token count for a chat.Response, preferring the
// Usage split and falling back to the legacy single counter.
func totalOf(resp *chat.Response) int {
	if resp == nil {
		return 0
	}
	if resp.Usage != nil {
		return resp.Usage.TotalTokens
	}
	return resp.Tokens
}

// toChatTools converts the runtime ToolSpec surface into the chat seam's native Tool
// declarations. The chat seam forwards them to the provider as native tools; there is no
// prompt-based fallback.
func toChatTools(tools []ToolSpec) []chat.Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]chat.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Function.Name == "" {
			continue
		}
		out = append(out, chat.Tool{
			Type: "function",
			Function: chat.ToolFunction{
				Name:        t.Function.Name,
				Description: t.Function.Description,
				Parameters:  t.Function.Parameters,
			},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// knownTool reports whether name is a real tool, judged against the STATIC full set
// (ToolMap).
//
// It deliberately does NOT consult the active surface. Disabled or web-hidden
// tools are real tools that this session has stopped advertising; a model that
// still calls one must get the "unavailable, use this instead" note from
// ExecuteTool, not an "unknown tool" correction. Judging against the active
// surface would turn every disabled call into Unknown and override that
// deliberately gentler degradation path.
func knownTool(name string) bool {
	_, ok := ToolMap[name]
	return ok
}

// nativeToHarnessCalls maps chat-seam native tool calls into the runtime ToolCall
// shape, preserving the Unknown flag for names outside the static tool map.
//
// A missing id is synthesized as `call_{i}` — without it the assistant message and its
// tool responses cannot be paired and the next request is rejected with
// "tool call result does not follow tool call".
func nativeToHarnessCalls(calls []chat.ToolCall) []ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(calls))
	for i, c := range calls {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("call_%d", i)
		}
		out = append(out, ToolCall{
			ID:      id,
			Name:    c.Name,
			Args:    c.Arguments,
			Unknown: !knownTool(c.Name),
		})
	}
	return out
}

// ensureToolCallIDs backfills synthesized ids for calls produced by a
// SessionModel that did not assign one, so the assistant message and every tool
// response reference the same id (see nativeToHarnessCalls).
func ensureToolCallIDs(calls []ToolCall) {
	for i := range calls {
		if calls[i].ID == "" {
			calls[i].ID = fmt.Sprintf("call_%d", i)
		}
	}
}

// assistantToolCalls renders runtime tool calls as the schema assistant message's
// tool_calls: the model's native tool_calls are passed through verbatim so they pair with
// the tool
// responses. Passing nil here leaves a dangling tool_call and the next request
// is rejected.
func assistantToolCalls(calls []ToolCall) []schema.ToolCall {
	if len(calls) == 0 {
		return nil
	}
	out := make([]schema.ToolCall, 0, len(calls))
	for _, c := range calls {
		raw, err := json.Marshal(c.Args)
		if err != nil || c.Args == nil {
			raw = []byte("{}")
		}
		out = append(out, schema.ToolCall{
			ID:       c.ID,
			Type:     "function",
			Function: schema.FunctionCall{Name: c.Name, Arguments: string(raw)},
		})
	}
	return out
}

// Session state

// SessionState: (the LangGraph TypedDict).
type SessionState struct {
	// Messages is the running conversation (system + user + assistant + tool).
	Messages []schema.Message
	// ParentState is the slot table being patched by this session.
	ParentState State
	Tools       *Toolset
	Model       SessionModel

	// PendingCalls are tool calls awaiting execution this turn.
	PendingCalls []ToolCall
	Done         bool

	NewStates   []State
	FoundAnswer *string

	RetrievedEvidenceIDs []string
	Attempts             int
	DeadlineLeft         float64

	// CtxBudget is the cumulative tool-payload char ceiling for the session.
	CtxBudget int
	// ToolChars is the running total of tool-payload chars emitted so far (O(1)
	// accounting).
	ToolChars int
	// ToolCache avoids re-executing an identical (name, args) call. It is the
	// per-round cache shared with the round's other sessions, so it is a guarded
	// *ToolCache rather than a bare map.
	ToolCache *ToolCache
	// SearchQueries accumulates retrieval queries for near-dup detection.
	SearchQueries []string
	// SkippedDup counts near-duplicate retrievals suppressed so far.
	SkippedDup int
	// ToolStrikes counts dataset-level empties per tool.
	ToolStrikes map[string]int
	// ToolOutcomes is the audit trail of (name, status, reason, metrics).
	ToolOutcomes []map[string]any

	// KB is the shared evidence pool. The session reads it for the per-turn
	// RECORD line (see SessionRecord) and for the names the evidence offers;
	// the tools write it. Nil skips both.
	KB *Kbinfos
	// Record is the last turn's record — the facts the continuation decision is
	// made from (see offerContinuation and SessionRecord).
	Record SessionRecord
	// PoolWalk is how many pool chunks this session has already looked at while
	// choosing an excerpt to show (see unreadPoolExcerpt). The pool only ever
	// appends, so the watermark is what keeps a session from being shown the same
	// passage twice.
	PoolWalk int
	// PoolRead is whether this session has already been given its one pool excerpt
	// (see unreadPoolExcerpt): the read is a last resort, not a routine.
	PoolRead bool
	// ContinuationAsked is the Attempts value the continuation offer was appended
	// for, so one turn never carries the offer twice.
	ContinuationAsked int
	// EnumerationProtocol is the SET-direction method this session MAY be handed:
	// seeded when the table declared a set (see setProtocolFor), and appended once
	// mid-session when the caller writes its first batch (see appendBatchProtocol).
	// Resolved once at seed time so nothing mid-session needs the prompt loader.
	EnumerationProtocol string
	// BatchProtocolShown is whether the mid-session append has happened. Once per
	// session: the method repeated is prompt noise.
	BatchProtocolShown bool
	// floorGateLogged records that the turn floor was reported (see actionMaxTurns).
	// The floor is resolved on every turn, so only the first resolution is worth a
	// log line — it is what says whether the shape gate fired for this session.
	floorGateLogged bool

	Direction  string
	RoutedDocs []string
	// NavHint is the joined routed-doc summaries carried by the navigation
	// ladder; it soft-boosts later retrieval ("nav is a hint, not a constraint")
	// without restricting the corpus.
	NavHint string
	// NavRuleID is where the navigation ladder currently rests ("" = the ladder
	// finished or was never armed).
	NavRuleID string

	TerminalType    *string
	TerminalPayload map[string]any
}

// appendMessages is the single place a session grows its message list.
//
// The message list only ever grows by appending: no message id is set (a fresh uuid per
// message means ids never collide) and nothing here reads one — the `.id` fields in play
// are tool-call and slot/nav-rule ids.
//
// Deliberately NOT a tool_call_id dedupe: fallback ids are per-turn
// (`ensureToolCallIDs` assigns call_0, call_1… to unnamed calls), so two turns can both
// carry call_0 and dropping the earlier response would orphan that turn's assistant
// tool_calls. Assistant turns are new by construction.
//
// A real message id would be needed to mirror add_messages if we ever stream partial
// assistant messages into the state or restore a session from a checkpoint; eino's
// schema.Message has no id field.
func appendMessages(dst []schema.Message, msgs ...schema.Message) []schema.Message {
	return append(dst, msgs...)
}

// Tool dispatch

// ExecuteTool: dispatch ONE tool call by name.
//
// Returns a ToolOutcome whose status/reason let the caller act on WHAT happened
// empty payload, query miss, infra failure, or a run that added no new
// evidence — instead of only counting characters.
func ExecuteTool(ctx context.Context, tools *Toolset, name string, args map[string]any) ToolOutcome {
	// Short-circuit a tool already proven unavailable this session (no compiled
	// structure of its kind). We still return a note, not an error, so the model
	// learns to switch to the corpus tools rather than loop.
	if tools != nil && tools.IsDisabled(name) {
		_LOG.Printf("[Action Session] tool %q disabled (no compiled structure); returning note", name)
		return ToolOutcome{
			Payload: []any{map[string]any{
				"kind": name,
				"note": fmt.Sprintf("%s is unavailable in this dataset (no compiled structure of its kind). Use search_chunks / retrieve / list_chunks instead.", name),
			}},
			Status: StatusEmpty,
			Reason: ReasonNoStructure,
		}
	}
	if tools == nil || tools.Exec == nil {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonInfra}
	}
	out, err := tools.Exec.Execute(ctx, name, args)
	if err != nil {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: ReasonInfra}
	}
	return out
}

// Nodes

// runActionNode: ONE model turn with tools.
// It appends the assistant message and records any tool calls as pending.
func (s *SessionState) runActionNode(ctx context.Context) error {
	s.Attempts++
	// Per-turn wall budget: max(15, min(75, deadline_left)). Without this a single turn can
	// eat the entire session budget, starving the salvage/finalize steps.
	wall := max(15.0, min(75.0, s.DeadlineLeft))
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(wall*float64(time.Second)))
	defer cancel()
	reply, err := s.Model.Complete(callCtx, s.Messages, s.Tools.ActiveToolSpecs())
	if err != nil {
		// A timed-out or failed turn CONVERGES the session empty instead of aborting it: the
		// node marks itself done with no new states and no answer, routing ends the graph,
		// and the entry point hands back the messages/evidence gathered so far. Returning
		// the error here aborts the whole eino run, which is reported as a failed session and
		// answered with an empty Result — losing the nav prefix's evidence ids and every tool
		// outcome.
		if callCtx.Err() == context.DeadlineExceeded {
			_LOG.Printf("[Action Session] turn timed out after %.0fs", wall)
		} else {
			_LOG.Printf("[Action Session] LLM call failed (%T); converging session empty", err)
		}
		s.Done = true
		s.NewStates = nil
		s.FoundAnswer = nil
		return nil
	}
	// The model's tool_calls are passed through verbatim on the assistant message so the
	// tool responses that follow can be paired by id. Passing nil leaves a dangling
	// tool_call and the next request is rejected with "tool call result does not follow
	// tool call".
	ensureToolCallIDs(reply.ToolCalls)
	s.Messages = append(s.Messages,
		*schema.AssistantMessage(reply.Content, assistantToolCalls(reply.ToolCalls)))

	if len(reply.ToolCalls) > 0 {
		s.PendingCalls = reply.ToolCalls
		return nil
	}
	s.PendingCalls = nil

	// No call: try the terminal protocol (<state> / <answer>).
	newStates, foundAnswer, terminalType, payload := ParseTerminal(reply.Content, s.ParentState)
	if foundAnswer != nil || len(newStates) > 0 {
		s.NewStates = newStates
		s.FoundAnswer = foundAnswer
		s.TerminalType = terminalType
		s.TerminalPayload = payload
		s.Done = true
		return nil
	}
	// Neither a call nor a terminal block: nudge once per turn.
	s.Messages = append(s.Messages, *schema.UserMessage("Call the retrieve tool, or output a <state> patch, or emit <answer>."))
	return nil
}

// toolNode: execute pending tool calls, append tool
// responses, and apply the per-outcome policies.
func (s *SessionState) toolNode(ctx context.Context) error {
	if s.Tools == nil {
		return nil
	}
	// ranAny: whether this node executed at least one call, i.e. whether a tool
	// result exists to annotate with the turn's record line (appendRecordLine).
	ranAny := len(s.PendingCalls) > 0
	evidenceIDs := append([]string(nil), s.RetrievedEvidenceIDs...)
	budgetChars := s.CtxBudget
	if budgetChars <= 0 {
		budgetChars = maxToolResponseChars * 4
	}
	used := s.ToolChars
	if s.ToolCache == nil {
		s.ToolCache = NewToolCache()
	}
	seenQueries := append([]string(nil), s.SearchQueries...)
	skipped := 0
	strikes := map[string]int{}
	for k, v := range s.ToolStrikes {
		strikes[k] = v
	}
	// Where the nav ladder currently rests. Bound BEFORE the loop so it survives
	// the empty-pending case and is the input for every tool_call that comes in —
	// a call only advances its own rung, and a later ladder continuation must
	// continue from the SAME resting point.
	//
	// The continuation is ported but currently UNREACHABLE:
	// it only runs once a rung leaves the chain in ModeLLM, and every rule in
	// NavRules is ModeAuto, so the chain always runs to completion in the prefix
	// and PendingRule is "" by the time control reaches here. Adding an LLM rung
	// makes it live with no further change.
	pendingRule := s.NavRuleID

	for _, c := range s.PendingCalls {
		// Near-duplicate retrieval suppression: if the model re-issues the same
		// intent as an earlier search (paraphrase), do NOT re-run the index —
		// return a nudge so it patches / reframes instead of burning turns.
		// q = str(args.get("query") or "").strip: the query may
		// be a LIST (retrieve takes up to 3), and its string form still counts
		// for near-dup detection and the seen_queries ledger. A type assertion
		// here degrades every array query to "" — seen_queries stays empty, and
		// the skipped-dup convergence can never trigger.
		q := argQueryString(c.Args["query"])
		if retrievalTools[c.Name] && q != "" && IsNearDup(q, seenQueries) {
			skipped++
			_LOG.Printf("[Action Session] skipping near-duplicate retrieval %q (already searched)", trunc(q, 80))
			s.Messages = appendMessages(s.Messages, toolMessage(c.ID, []any{map[string]any{
				"kind": c.Name,
				"note": "This query is a near-duplicate of an earlier retrieval and was skipped to avoid redundant searching. Patch the slot with what you have, or issue a genuinely NEW retrieval angle.",
			}}))
			continue
		}
		// Unknown tool name — never execute it; answer with a correction so the
		// model can recover. Models usually emit the XML protocol tags (state /
		// answer) as tool names; they belong in the reply body as plain text.
		if c.Unknown {
			hint := fmt.Sprintf(
				"'%s' is not a tool. State patches and final answers are plain TEXT in your reply body, wrapped in <state>...</state> or <answer>...</answer> XML tags — do not emit them as tool calls. Available tools: %s.",
				c.Name, strings.Join(ToolMapNames(), ", "))
			s.Messages = appendMessages(s.Messages, toolMessage(c.ID, []any{map[string]any{"kind": "error", "note": hint}}))
			continue
		}

		// Same-round cache: avoid re-running an identical (name, args) call.
		// The cache only avoids RE-EXECUTING; a response is still returned for
		// every declared call, because the assistant message declared it.
		// The cache is shared by a round's concurrent sessions, hence the guarded
		// Get/Put.
		cacheKey := callCacheKey(c)
		oc, cached := s.ToolCache.Get(cacheKey)
		if !cached {
			oc = ExecuteTool(ctx, s.Tools, c.Name, c.Args)
			s.ToolCache.Put(cacheKey, oc)
		}
		if q != "" {
			seenQueries = append(seenQueries, q)
		}
		evidenceIDs = append(evidenceIDs, oc.EvidenceIDs...)
		chunks := append([]any(nil), oc.Payload...)

		// ── Policy: act on WHAT happened, not just on payload size ──────────
		switch {
		case oc.Status == StatusOK:
			// A real hit clears the tool's strike record: it demonstrably works.
			delete(strikes, c.Name)
		case oc.Status == StatusEmpty && oc.Reason == ReasonNoStructure:
			// Dataset-level dead end. Strike it; disable once the strikes pile up
			// so one unlucky scope cannot kill the tool.
			n := strikes[c.Name] + 1
			strikes[c.Name] = n
			if n >= emptyStrikes {
				s.Tools.DisableTool(c.Name)
				_LOG.Printf("[Action Session] %s disabled after %d dataset-level empty results", c.Name, n)
			}
		case oc.Status == StatusRedundant:
			// Ran fine, but every hit was already in the shared evidence pool.
			// Say so explicitly — otherwise the model sees a normal passage list
			// and concludes the search succeeded, then re-searches the same ground.
			chunks = append(chunks, map[string]any{
				"kind": c.Name,
				"note": "All hits from this call were ALREADY in your evidence. Re-reading them will not add anything — issue a genuinely new angle, or output a <state> patch with what you have.",
			})
		}

		s.ToolOutcomes = append(s.ToolOutcomes, map[string]any{
			"name": c.Name, "status": oc.Status, "reason": oc.Reason, "metrics": oc.Metrics,
		})

		// The payload stays the JSON document it always was; the call's own note
		// rides BEHIND it, exactly like the turn's record line (appendRecordLine),
		// so nothing that parses the payload has to learn about either.
		payload := marshalPassages(chunks)
		if oc.Note != "" {
			payload += "\n" + oc.Note
			_LOG.Printf("[Action Session] result note (%s): %s", c.Name, trunc(oc.Note, 280))
		}
		// If the session is already heavy, cut this payload proportionally.
		// -1514 counts len of a str — CODE POINTS, not bytes — so
		// the budget and the cut must be rune-based too; a byte cap would hit
		// CJK payloads ~3x early and shrink the evidence the model sees.
		if used+utf8.RuneCountInString(payload) > budgetChars {
			keep := budgetChars - used
			if keep < 800 {
				keep = 800
			}
			payload = truncateRunes(payload, keep)
		}
		used += utf8.RuneCountInString(payload)
		s.Messages = appendMessages(s.Messages, *schema.ToolMessage(payload, c.ID))

		// Ladder continuation: when the model's rung came back weak, keep advancing the
		// ladder IN CODE so one
		// weak step cascades through the remaining (cheaper, wider) rungs instead
		// of leaving the model to rediscover the fallback one turn at a time.
		//
		// Unreachable with today's NavRules (every rung is ModeAuto, so the chain
		// always runs to completion in the prefix and PendingRule is ""). It is
		// ported so an LLM rung resumes from the right place without another
		// change here.
		if pendingRule != "" {
			rule, ok := navRuleByID[pendingRule]
			if ok {
				nxt := rule.Next[oc.Status]
				if nxt != "" {
					// The nav context is rebuilt (direction, known docs) from the live state —
					// the ladder runs against the CURRENT routed scope, not the one captured at
					// prefix time.
					ladderNav := &NavContext{
						Direction: s.Direction,
						KnownDocs: append([]string(nil), s.RoutedDocs...),
					}
					ladderBudget := s.DeadlineLeft * navPrefixBudgetRatio
					if ladderBudget < navPrefixMinBudgetS {
						ladderBudget = navPrefixMinBudgetS
					}
					// Respect the session's remaining context budget: the ladder
					// pairs are appended outside this node's own accounting
					// (max_chars = max(800, budget_chars - used).)
					ex := RunNavChain(ctx, s.Tools, ladderNav, nxt, ladderBudget, "ladder",
						max(800, budgetChars-used))
					// The ladder keeps advancing: a later tool_call in the SAME
					// batch (or the next turn) resumes from where this call left
					// it, not from the original resting point. "" means the
					// ladder finished — also correct to record.
					pendingRule = ex.PendingRule
					for _, m := range ex.Messages {
						if m.Role == schema.Tool {
							// counts len of a str — code points.
							used += utf8.RuneCountInString(m.Content)
						}
						s.Messages = appendMessages(s.Messages, m)
					}
					evidenceIDs = append(evidenceIDs, ex.EvidenceIDs...)
					s.ToolOutcomes = append(s.ToolOutcomes, ex.Outcomes...)
					if oc.Status != StatusOK {
						_LOG.Printf("[Action Session] ladder %q -> %q (reason=%s)",
							rule.ID, nxt, oc.Reason)
					}
				}
			}
		}
	}

	// Where the ladder now rests ("" = finished).
	s.NavRuleID = pendingRule

	s.PendingCalls = nil
	s.RetrievedEvidenceIDs = evidenceIDs
	s.ToolChars = used
	s.SearchQueries = seenQueries
	s.SkippedDup += skipped
	s.ToolStrikes = strikes
	s.appendBatchProtocol(ranAny)
	s.appendRecordLine(ranAny)
	return nil
}

// appendBatchProtocol hands the set-direction method to a session that has SHOWN it
// is enumerating — the second, and the only zero-false-positive, delivery.
//
// A session seeded by setProtocolFor already has the text; this path exists for the set
// direction whose planner typed its count slot with a type that does not read as a count.
// The signal it rides is the caller's own writing: the model writes a batch of items
// exactly when the direction is a set.
//
// It rides the tool result the model is about to read, like the record line, and is
// appended at most once per session.
func (s *SessionState) appendBatchProtocol(ranAny bool) {
	if s.BatchProtocolShown || !ranAny || s.EnumerationProtocol == "" || len(s.Messages) == 0 {
		return
	}
	if !s.wroteBatch() {
		return
	}
	last := &s.Messages[len(s.Messages)-1]
	if last.Role != schema.Tool {
		return
	}
	s.BatchProtocolShown = true
	last.Content = strings.TrimRight(last.Content, "\n") + "\n\n" + s.EnumerationProtocol
	// The same signal widens the retrieval budget for the rest of the run: on a
	// set direction a dropped query is a member nobody searched, and the batch
	// the caller just wrote is the proof that this direction is assembling a set
	// (see Kbinfos.MarkSetDirection).
	s.KB.MarkSetDirection()
	_LOG.Printf("[Action Session] the caller wrote a batch — set-direction method appended to the turn (%d char(s))",
		len(s.EnumerationProtocol))
}

// appendRecordLine appends the per-turn RECORD line to the last tool result.
//
// This is the loop's feedback channel: the model's next query was otherwise aimed
// with no knowledge of what this session had already probed (and what came back
// empty), which names the record already holds, which confirmed names are still
// undecided, or which names the passages themselves offer. All of that is
// computed here anyway — it was simply never shown.
//
// It rides on the last tool message instead of a message of its own: one line,
// same turn, no extra role in the conversation.
//
// The line is appended ONLY when this node ran at least one tool call, so a turn
// with no calls cannot accumulate duplicates of the previous line.
func (s *SessionState) appendRecordLine(ranAny bool) {
	if !ranAny || len(s.Messages) == 0 {
		return
	}
	rec := s.sessionRecordNow()
	grew := rec.grewFrom(s.Record)
	s.Record = rec
	// A session that is not ENUMERATING gets no line at all (see enumerating).
	//
	// The line reports members, probed names and reached names — the set's to-do
	// list. On a single-value question every field of it is empty or meaningless
	// (`members=0 | probed-reached=0`), it is recomputed and appended on every turn,
	// and the model then spends turns on it. The record itself is still kept on the session
	// (the continuation ask reads it), only the line the model sees is skipped.
	if !s.enumerating() {
		return
	}
	// The line the model steers by is message content, so the log could not show
	// whether a mechanism fired at all: three rounds of analysis here ended up
	// inferring it from side effects. One truncated line per turn ends that.
	_LOG.Printf("[Action Session] record line: %s", trunc(rec.Line(), 320))
	last := &s.Messages[len(s.Messages)-1]
	if last.Role != schema.Tool || rec.Pool == 0 {
		// No pool bound (or no tool result to annotate): keep the record, skip the
		// line rather than inventing a message for it.
		return
	}
	line := rec.Line()
	last.Content = strings.TrimRight(last.Content, "\n") + "\n" + line
	// An ENUMERATION session whose record has gone FLAT also gets one excerpt from a
	// pool passage it has never been shown (see unreadPoolExcerpt): the round has
	// already paid for that text, unread text is where unnoticed members live, and a
	// session that is still finding things does not need rescuing. Gated on the
	// shape so no other question pays for it at all, and one excerpt per turn.
	if s.enumerating() && !grew {
		if excerpt := s.unreadPoolExcerpt(); excerpt != "" {
			// Logged as well as delivered: whether the mechanism fired is otherwise
			// only visible inside the message content.
			_LOG.Printf("[Action Session] pool excerpt: %s", trunc(excerpt, 240))
			last.Content += "\n" + excerpt
		}
	}
}

// turnRunCap is the hard ceiling on a session's turns: the mode's floor plus the
// turns the model may add on its own decision (see offerContinuation).
func (s *SessionState) turnRunCap() int { return s.actionMaxTurns() + turnRunExtra }

// offerContinuation asks the MODEL whether the session takes another turn.
//
// The mode's turn count used to end the session outright (ActionMaxTurns was 4 on
// medium/high at the time), which finalized sessions mid-enumeration with their own clock
// unspent and a tool result the model never read. Replacing that with a runtime heuristic
// ("did the record grow?") fixed the cut-off but bought the wrong thing: the ledger grows
// whenever the model PROBES, so the heuristic extended sessions that were piling up findings
// without recording any.
//
// So the floor is the floor and the decision is the model's: it is told the
// budget state, handed the record line it just received, and asked to continue
// only for something that line shows is missing — otherwise to emit the state
// patch now. The runtime keeps two hard bounds it does not delegate: the run cap
// above (so an eager model cannot run away) and the session clock (below which no
// further turn is offered, because the finalize/salvage step must still fit).
//
// The offer is gated on this session ENUMERATING (see enumerating): an extra turn is
// an extra model call plus up to turnRunExtra more turns, and a question that is not
// assembling a set has nothing for those turns to find.
//
// While the gate read the shape of the planner's table, past runs extended questions that
// were doing arithmetic over two values, whose caller's own batch — the thing this reads —
// was never written at all.
func (s *SessionState) offerContinuation() bool {
	if !s.enumerating() {
		return false
	}
	if s.Attempts >= s.turnRunCap() || s.DeadlineLeft <= turnAskFloorS {
		return false
	}
	if s.ContinuationAsked == s.Attempts {
		return true
	}
	s.ContinuationAsked = s.Attempts
	ask := continuationAsk(s.Attempts, s.turnRunCap(), s.Record.Brief())
	s.Messages = appendMessages(s.Messages, *schema.UserMessage(ask))
	_LOG.Printf("[Action Session] turn %d/%d — the floor is spent; offered the model one more turn while the record says something is missing (%s, %.0fs left).\noffer=%q",
		s.Attempts, s.turnRunCap(), s.Record.Brief(), s.DeadlineLeft, trunc(ask, 700))
	return true
}

// parentSet reports whether the direction this session was sent on ASKS FOR A SET —
// what the planner DECLARED, never what a candidate looks like (see CoverageOf).
func (s *SessionState) parentSet() bool { return CoverageOf(s.ParentState).Set }

// wroteBatch reports whether the CALLER wrote a batch of terms in one query — the
// signal that it is enumerating rather than asking a question.
//
// The gate is the caller's own writing, not a guess about the question, because the guess
// is wrong: a slot TYPE cannot tell "how many people did X kill" from "how many times larger
// is A than B", and a single phrase's commas make it look like two members.
func (s *SessionState) wroteBatch() bool {
	for _, q := range s.SearchQueries {
		if callerBatch(q) {
			return true
		}
	}
	return false
}

// enumerating reports whether this session is assembling a SET: the caller wrote a
// batch (the tell that survives contact), or the direction's own table asks for a
// count/list.
//
// The table stays as the second half on purpose: a session may enumerate without ever
// writing a two-name batch (one probe per member), and then the record line is exactly what
// it needs. That half's false positives are bounded and rare (a sentence written into a
// count-typed slot); the batch half has none.
func (s *SessionState) enumerating() bool {
	return s.wroteBatch() || s.parentSet()
}

// SessionWallS is the wall clock a session on this direction is given: the
// enumeration clock when the table declared a set, the default one otherwise.
//
// It is a FUNCTION of the shape rather than a constant because the shape decides
// what the time is spent on: an enumeration session is mid-batch when its clock runs
// out, and a batch that is cut loses the members it had already reached (see
// setActionTimeoutS for the measurement). A value session has no such work in
// flight, so it keeps the tighter clock and the shorter end-to-end latency.
//
// The caller owns the context: this number is only the budget a session is told it
// has, and a pass whose own timeout is shorter still bounds it (see
// RunSlotResearchPass, which hangs the session clock off the round's PARENT because
// the round's own clock is fixed before the table — and therefore the shape — is
// known).
func SessionWallS(parent State) float64 {
	// The enumeration clock is for a direction that IS one — a set of named members whose
	// deed the planner wrote the words for (Coverage.Ok) — not for every direction that
	// happens to contain a count: an enumeration session is mid-batch when its clock runs
	// out and a batch that is cut loses the members it had reached (see setActionTimeoutS
	// for the measurement), while a value session has no such work in flight and keeps the
	// tighter clock with the shorter latency.
	if CoverageOf(parent).Ok() {
		return setActionTimeoutS
	}
	return actionTimeoutS
}

// setMethodFor returns the enumeration METHOD, or "" when the direction is not an
// enumeration.
//
// The method is `action_set`, and both halves of when it is delivered are deliberate: its
// FIRST instruction — propose more candidates than you expect — is a decision taken before
// the first query, so a direction that has declared itself an enumeration is seeded before
// its first turn; and a direction that has not is handed the method after the first batch it
// writes (see appendBatchProtocol), which is the signal with no observed false positives.
//
// The gate is the enumeration itself (see Coverage): the method tells a session to
// enumerate NAMED members, and a single-value question that merely contains a count must
// not be told to assemble anything. A permissive version seeds most sessions with set
// strategy on questions that assemble nothing, and a shape-only version still seeds the value
// questions that merely contain a count — where it can only add cost and timeouts.
func setMethodFor(table State, prompts PromptLoader) string {
	if !CoverageOf(table).Ok() {
		return ""
	}
	return loadOptionalPrompt(prompts, "action_set")
}

// enumerationSeed builds the text a session sent on this direction is seeded with: the
// method, and — when the runtime has already run the enumeration — the windows it found
// (see CoverageSet.Render).
//
// A list of queries in a prompt is ADVICE, and advice may simply not be taken. A window is
// evidence — it carries the chunk id a member is cited by — so the session's job becomes
// reading what came back. When no enumeration ran (no clock, no executor), the method
// travels alone: the seed never carries queries to make.
func enumerationSeed(table State, prompts PromptLoader, seed string) string {
	if !CoverageOf(table).Ok() {
		// Not an enumeration: nothing to assemble, so neither the method nor a window has
		// any business in this session's seed.
		return ""
	}
	method := setMethodFor(table, prompts)
	if s := strings.TrimSpace(seed); s != "" {
		if method != "" {
			return method + "\n\n" + s
		}
		return s
	}
	return method
}

// continuationAsk is the offer the model decides on: it names the hard bound, the
// remaining turns, and the ONLY grounds on which another turn is granted — what
// the record line shows is still missing. The record itself was appended to the
// model's last tool result, so the decision is made on facts, not on appetite.
func continuationAsk(taken, cap int, record string) string {
	return fmt.Sprintf(
		"TURN BUDGET: %d turn(s) taken, up to %d available. DECIDE NOW — your next reply decides it:\n"+
			"- If the [record] line still shows something MISSING — a name you proved reachable and never recorded, a slot with no candidate, an angle you have not tried — call ONE more tool aimed at exactly that. %d turn(s) left.\n"+
			"- Otherwise emit the state patch NOW with everything you have.\n"+
			"An extra turn is only for something the record line shows is missing; re-running a search you already ran is not.\n"+
			"Current record: %s",
		taken, cap, cap-taken, record)
}

// finalizeNode: tool budget spent — ONE last call
// WITHOUT tools, demanding the terminal JSON to salvage whatever was learned.
func (s *SessionState) finalizeNode(ctx context.Context) error {
	budgetPrompt := ("TOOL BUDGET EXHAUSTED. Based ONLY on the passages retrieved above, output now — no prose outside the block:\n" +
		"<state>{\"new_states\": [{\"state\": [{\"id\": <slot_id>, \"kind\": \"members\", " +
		"\"items\": [{\"name\": \"<name>\", \"chunk_id\": \"<its passage>\"}], " +
		"\"candidate_strength\": <0..1>}]}]}</state>\n" +
		"Use \"kind\": \"count\" with \"count\": <n> for a number, and \"kind\": \"text\" with " +
		"\"candidate\": \"<value>\" for one value that is neither a member list nor a number.\n" +
		"If NOTHING was learned use: <state>{\"new_states\": []}</state>")

	// The patch written HERE is the one that lands in the record, so the record is
	// part of the order.
	//
	// A session can probe a name, get its passages back, and never mention it in the terminal
	// patch — the run's own evidence, found and paid for, dropped at the last step. The record
	// is the only place that fact exists, and this is the last call that can act on it, so the
	// salvage order names the names it has to account for instead of asking for "whatever you
	// have".
	if rec := s.sessionRecordNow(); rec.Pool > 0 {
		budgetPrompt += "\n\nAccount for the record below in the patch you write. Every name listed as " +
			"FOUND BUT NOT RECORDED has a passage in the pool, so it is either a member this patch includes or " +
			"a name this patch REJECTS with its clue. More searching is no longer possible, so an unmentioned " +
			"name is simply lost.\n" + rec.Line()
	}

	// Defensive: strip any assistant.tool_calls that never got a tool response,
	// else the provider rejects the history.
	msgs := append([]schema.Message(nil), s.Messages...)
	msgs = stripUnpairedToolCalls(msgs)
	msgs = append(msgs, *schema.UserMessage(budgetPrompt))

	tmo := s.DeadlineLeft
	if tmo <= 0 {
		tmo = finalizeTimeoutS
	}
	tmo = min(max(tmo, minFinalizeTimeoutS), finalizeTimeoutS)

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(tmo*float64(time.Second)))
	defer cancel()

	reply, err := s.Model.Complete(callCtx, msgs, nil)
	if err != nil {
		// A failed salvage call is logged and the node CONTINUES to the deterministic
		// loose-clue harvest below. Returning early here drops the last-narration
		// breadcrumb exactly when the salvage model is unavailable, which is the case the
		// harvest exists for.
		_LOG.Printf("[Action Session] salvage call failed: %v", err)
	} else {
		newStates, foundAnswer, terminalType, payload := ParseTerminal(reply.Content, s.ParentState)
		s.NewStates = newStates
		s.FoundAnswer = foundAnswer
		s.TerminalType = terminalType
		s.TerminalPayload = payload
		if foundAnswer != nil {
			_LOG.Printf("[Action Session] answer salvaged from exhausted session")
		} else if len(newStates) > 0 {
			_LOG.Printf("[Action Session] %d branch(es) salvaged from exhausted session", len(newStates))
		}
	}

	// Loose-clue harvest (deterministic, zero-LLM): when the salvage produced no branches
	// AND no answer, the last AI narration often still carries a fact
	// worth keeping as a breadcrumb in the first unresolved slot.
	if len(s.NewStates) == 0 && s.FoundAnswer == nil {
		if txt := lastNarration(s.Messages); len(txt) >= 24 {
			unresolved := s.ParentState.Unresolved()
			var targetID int
			if len(unresolved) > 0 {
				targetID = unresolved[0].ID
			} else if len(s.ParentState.State) > 0 {
				targetID = s.ParentState.State[0].ID
			} else {
				targetID = -1
			}
			if targetID >= 0 {
				// discovered_clues must be []any: ApplyPatch type-switches on the
				// JSON-decoded shape, so a []string is silently ignored and the whole patch
				// no-ops.
				if patched := ApplyPatch(s.ParentState, []map[string]any{
					{"id": targetID, "discovered_clues": []any{"narrative: " + txt[:min(len(txt), 220)]}},
				}); patched != nil {
					s.NewStates = []State{*patched}
				}
			}
		}
	}

	s.Done = true
	return nil
}

// lastNarration returns the content of the last assistant message that carries non-empty
// text, scanning newest-first.
func lastNarration(msgs []schema.Message) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != schema.Assistant {
			continue
		}
		// Skip assistant messages that only hold tool_calls (no free text).
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		return strings.TrimSpace(m.Content)
	}
	return ""
}

// Routing

type routeTarget int

const (
	routeEnd routeTarget = iota
	routeTool
	routeFinalize
	routeRunAction
)

// actionMaxTurns: the mode's turn budget.
//
// The mode's number is a FLOOR, and it is billed to every turn of every session on
// the question, so it is paid only by the shape it was raised for: the deeper modes
// lifted it (medium/high 4 → 8, ultra 6 → 10) for the enumeration path, whose turns
// each spend a batch of names. A session that is not assembling a set has no such
// batch and runs on the value floor instead (see valueTurnFloor for the measurement).
// A shape that declares itself LATER is not penalised: the floor is resolved per
// turn, so the first batch — or a parent table that declares a count/list — raises
// it for the turns that follow.
func (s *SessionState) actionMaxTurns() int {
	floor := ResolveMode(s.Tools).ActionMaxTurns
	if floor <= 0 {
		floor = valueTurnFloor
	}
	if floor > valueTurnFloor && !s.enumerating() {
		if !s.floorGateLogged {
			s.floorGateLogged = true
			_LOG.Printf("[Action Session] turn floor %d → %d: this session is not assembling a set (%q, mode %s)",
				floor, valueTurnFloor, trunc(s.Direction, 60), ResolveMode(s.Tools).Label)
		}
		return valueTurnFloor
	}
	return floor
}

// route
func (s *SessionState) route() routeTarget {
	if s.Done {
		return routeEnd
	}
	// Run pending tool_calls FIRST, even at the turn budget: leaving an
	// assistant.tool_calls message without its tool response makes the provider
	// reject the next call. The tool node clears PendingCalls, then the route
	// re-checks the budget.
	if len(s.PendingCalls) > 0 {
		return routeTool
	}
	if s.Attempts >= s.actionMaxTurns() {
		// The mode's turn count is a FLOOR: past it the MODEL decides whether the
		// session takes another turn, up to the run cap (see offerContinuation).
		if s.offerContinuation() {
			return routeRunAction
		}
		return routeFinalize
	}
	// No-progress convergence: the model keeps re-issuing near-duplicate
	// retrievals, so further turns are unlikely to surface new evidence.
	if s.SkippedDup >= skippedDupLimit {
		_LOG.Printf("[Action Session] %d near-duplicate retrieval(s) skipped; converging session early", s.SkippedDup)
		return routeFinalize
	}
	return routeRunAction
}

// routeAfterTool: the turn budget is checked
// AFTER the tool responses are appended. A pending tool_call must always
// receive its matching tool response, but once the budget is spent we must NOT
// go back to run_action — that was the Q86 infinite-loop: the model kept
// emitting tool_calls, PendingCalls stayed non-empty, so the attempts check in
// route was never reached and the session burned the whole timeout.
func (s *SessionState) routeAfterTool() routeTarget {
	if s.Attempts >= s.actionMaxTurns() {
		// Same rule as route: the tool result that just arrived is what the continuation offer
		// is about, so the model decides with it in hand — otherwise the last probe's result is
		// never read by a turn.
		if s.offerContinuation() {
			return routeRunAction
		}
		return routeFinalize
	}
	return routeRunAction
}

// Session-node adapters: unlike method values (s.runActionNode) these take the
// state from the graph payload instead of capturing an instance, which is what lets ONE
// compiled graph serve every session: the session's tools/model arrive in the state
// (SessionState.Tools / .Model) rather than in a closure.
func runActionNodeFn(ctx context.Context, st *SessionState) (*SessionState, error) {
	return st, st.runActionNode(ctx)
}

func toolNodeFn(ctx context.Context, st *SessionState) (*SessionState, error) {
	return st, st.toolNode(ctx)
}

func finalizeNodeFn(ctx context.Context, st *SessionState) (*SessionState, error) {
	return st, st.finalizeNode(ctx)
}

var (
	sessionGraphOnce sync.Once
	sessionGraphRun  compose.Runnable[*SessionState, *SessionState]
	sessionGraphErr  error
)

// sessionGraph declares the action-session graph and compiles it ONCE per process: every
// session invokes the same runnable with its own state, so the compile (and any structural
// error) happens once instead of once per session.
//
// RunSlotResearchPass runs sessions in parallel, so the runnable is invoked
// concurrently by design. That is safe under three preconditions — break any and the
// sharing must be revisited:
//
//  1. Nodes touch only their own *SessionState; no package-level mutable state inside
//     a node.
//  2. No checkpoint store is configured (compose.WithCheckPointStore) — the
//     checkPointer lives on the shared runner. Guarded by
//     TestSessionGraphHasNoCheckpointStore.
//  3. Eino v0.9.14's runner keeps no per-run state: it holds only the static topology
//     and every Invoke builds its own channels and task manager
//     (compose/graph_run.go:129-130, :933, :948). Re-check on upgrade.
//
// See TestSessionGraphIsSharedAcrossConcurrentSessions.
func sessionGraph() (compose.Runnable[*SessionState, *SessionState], error) {
	sessionGraphOnce.Do(func() {
		g := compose.NewGraph[*SessionState, *SessionState]()

		var buildErr error
		addNode := func(name string, fn func(context.Context, *SessionState) (*SessionState, error)) {
			if buildErr != nil {
				return
			}
			buildErr = g.AddLambdaNode(name, compose.InvokableLambda(fn))
		}
		addEdge := func(from, to string) {
			if buildErr != nil {
				return
			}
			buildErr = g.AddEdge(from, to)
		}
		addBranch := func(from string, ends map[string]bool, cond func(*SessionState) routeTarget) {
			if buildErr != nil {
				return
			}
			buildErr = g.AddBranch(from, compose.NewGraphBranch(func(_ context.Context, st *SessionState) (string, error) {
				return routeNodeName(cond(st)), nil
			}, ends))
		}

		addNode("run_action", runActionNodeFn)
		addNode("tool", toolNodeFn)
		addNode("finalize", finalizeNodeFn)

		addEdge(compose.START, "run_action")
		addEdge("finalize", compose.END)

		addBranch("run_action", map[string]bool{
			"tool": true, "finalize": true, "run_action": true, compose.END: true,
		}, (*SessionState).route)
		addBranch("tool", map[string]bool{"run_action": true, "finalize": true}, (*SessionState).routeAfterTool)

		if buildErr != nil {
			sessionGraphErr = buildErr
			return
		}
		sessionGraphRun, sessionGraphErr = g.Compile(context.Background(),
			compose.WithGraphName("action_session"),
			// The session is bounded by the turn budget (route / routeAfterTool)
			// and the caller's deadline; this is only a backstop so a mis-wired
			// cycle cannot spin forever.
			compose.WithMaxRunSteps(maxSessionRunSteps),
		)
	})
	return sessionGraphRun, sessionGraphErr
}

// maxSessionRunSteps bounds one session's Eino steps: a safety net sized well above what
// the turn budget can reach.
const maxSessionRunSteps = 256

// sessionLoop invokes the compiled action-session graph, edge for edge:
//
//	START → run_action
//	run_action → route → {END | tool | finalize | run_action}
//	tool → route_after_tool → {run_action | finalize}
//	finalize → END
func (s *SessionState) sessionLoop(ctx context.Context) error {
	runnable, err := sessionGraph()
	if err != nil {
		return err
	}
	_, err = runnable.Invoke(ctx, s)
	return err
}

// routeNodeName maps the routing predicates onto the Eino graph's node keys.
func routeNodeName(t routeTarget) string {
	switch t {
	case routeTool:
		return "tool"
	case routeFinalize:
		return "finalize"
	case routeRunAction:
		return "run_action"
	default:
		return compose.END
	}
}

// Terminal parsing

// ParseTerminal: parse the two terminal blocks
// (<state> patches → new-state branches; <answer> → final answer).
//
// Returns (newStates, foundAnswer, terminalType, payload) — exactly one of
// newStates / foundAnswer may be non-empty.
func ParseTerminal(content string, parent State) ([]State, *string, *string, map[string]any) {
	if strings.Contains(content, "<state>") {
		block := ExtractTag(content, "state")
		if block == "" {
			block = "{}"
		}
		data, _ := ExtractJSON(block).(map[string]any)
		if data == nil {
			data = map[string]any{}
		}
		rawBranches, _ := data["new_states"].([]any)
		// Tolerate a bare list of variable patches (no {"state": [...]} wrapper):
		// models emit both shapes.
		if len(rawBranches) > 0 {
			allBare := true
			for _, b := range rawBranches {
				if m, ok := b.(map[string]any); ok {
					if _, has := m["state"]; has {
						allBare = false
						break
					}
				}
			}
			if allBare {
				wrapped := make([]any, 0, len(rawBranches))
				for _, b := range rawBranches {
					wrapped = append(wrapped, map[string]any{"state": []any{b}})
				}
				rawBranches = wrapped
			}
		}
		var branches []State
		for _, rb := range rawBranches {
			br, ok := rb.(map[string]any)
			if !ok {
				continue
			}
			patches := toPatchList(br["state"])
			if ns := ApplyPatch(parent, patches); ns != nil {
				branches = append(branches, *ns)
			}
		}
		tt := "state"
		return branches, nil, &tt, data
	}
	if strings.Contains(content, "<answer>") {
		block := ExtractTag(content, "answer")
		if block == "" {
			block = "{}"
		}
		data, _ := ExtractJSON(block).(map[string]any)
		if data == nil {
			data = map[string]any{}
		}
		// An empty/whitespace answer is NOT a found answer. Returning a non-nil empty
		// string here would terminate the session with FoundAnswer="" — the slot research
		// pass then reported
		// collected_answer=false for a session that never answered, and the
		// model never got the chance to re-emit a proper patch/answer.
		ans := ""
		if raw, ok := data["answer"]; ok && raw != nil {
			ans = strings.TrimSpace(fmt.Sprint(raw))
		}
		var found *string
		if ans != "" {
			found = &ans
		}
		patches := toPatchList(data["new_state"])
		var branches []State
		if ns := ApplyPatch(parent, patches); ns != nil {
			branches = append(branches, *ns)
		}
		tt := "answer"
		return branches, found, &tt, data
	}
	return nil, nil, nil, nil
}

func toPatchList(v any) []map[string]any {
	list, _ := v.([]any)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// Helpers

func toolMessage(id string, passages []any) schema.Message {
	return *schema.ToolMessage(marshalPassages(passages), id)
}

func marshalPassages(passages []any) string {
	if passages == nil {
		passages = []any{}
	}
	raw, err := json.Marshal(map[string]any{"passages": passages})
	if err != nil {
		return `{"passages": []}`
	}
	return string(raw)
}

// callCacheKey keys a call by name plus a canonical JSON encoding of its arguments.
func callCacheKey(c ToolCall) string {
	raw, err := json.Marshal(c.Args)
	if err != nil {
		raw = []byte("{}")
	}
	return c.Name + "|" + string(raw)
}

// ToolCache is the per-round tool-outcome cache. One instance is shared by a round's
// CONCURRENT sessions, and Go's sessions are goroutines: concurrent map access is a FATAL
// error, so the map is guarded.
type ToolCache struct {
	mu sync.Mutex
	m  map[string]ToolOutcome
}

// NewToolCache returns an empty, concurrency-safe tool cache.
func NewToolCache() *ToolCache { return &ToolCache{m: map[string]ToolOutcome{}} }

// Get returns the cached outcome for key. A nil cache never holds anything, so a
// session constructed without one simply re-executes (the pre-existing behavior).
func (c *ToolCache) Get(key string) (ToolOutcome, bool) {
	if c == nil {
		return ToolOutcome{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	oc, ok := c.m[key]
	return oc, ok
}

// Put stores the outcome for key. Two identical calls that lose the race both compute and
// both store: the cache avoids re-executing DELIBERATE repeats, not simultaneous ones.
func (c *ToolCache) Put(key string, oc ToolOutcome) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m == nil {
		c.m = map[string]ToolOutcome{}
	}
	c.m[key] = oc
}

// stripUnpairedToolCalls removes assistant tool_calls that never received a
// tool response — a malformed history makes providers reject the next request
// ("tool call result does not follow tool call").
func stripUnpairedToolCalls(msgs []schema.Message) []schema.Message {
	answered := map[string]bool{}
	for _, m := range msgs {
		if m.Role == schema.Tool && m.ToolCallID != "" {
			answered[m.ToolCallID] = true
		}
	}
	out := make([]schema.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == schema.Assistant && len(m.ToolCalls) > 0 {
			kept := make([]schema.ToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				if answered[tc.ID] {
					kept = append(kept, tc)
				}
			}
			if len(kept) != len(m.ToolCalls) {
				m.ToolCalls = kept
			}
		}
		out = append(out, m)
	}
	return out
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// sortKeys is a small helper for deterministic map-key iteration in logs.
func sortKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ===================== navchain.go =====================
// Deterministic navigation prefix: a code-driven tool chain.
//
// WHY THIS EXISTS: the ReAct loop lets the MODEL decide which tool to call
// next, so ordering rules ("route with navigate_tree first, then drill with
// navigate_structure, fall back to retrieve when the drill is weak") can only
// be *suggested* in a prompt and are routinely ignored. This prefix runs those
// rules IN CODE before the loop starts and injects the resulting exchange as
// completed assistant/tool messages: the model then explores on top of a
// guaranteed-correct opening and cannot skip a step. The loop keeps its freedom
// for everything after the chain.

// Step modes.
const (
	// ModeAuto means the orchestrator runs this step itself (no model
	// round-trip).
	ModeAuto = "auto"
	// ModeLLM means the step needs a decision only the model can make (e.g.
	// WHICH routed document to drill into), so the chain stops here and hands
	// control back.
	ModeLLM = "llm"
)

// Navigation-chain constants.
const (
	// navPrefixBudgetRatio is the share of the session deadline the prefix may
	// spend; the ReAct loop keeps the rest.
	navPrefixBudgetRatio = 0.35
	// navPrefixCallTimeoutS is the per-call ceiling, so one slow navigation
	// cannot swallow the whole prefix.
	navPrefixCallTimeoutS = 25.0
	// navHintChars caps the nav-hint text fed back into retrieval as keywords.
	navHintChars = 600
	// navPrefixMinBudgetS floors the prefix budget.
	navPrefixMinBudgetS = 5.0
	// navMinStepBudgetS floors a single step's budget.
	navMinStepBudgetS = 5.0
)

// NavRulesEnabled
const NavRulesEnabled = true

// NavContext is the mutable state threaded through the navigation chain.
//
// KnownDocs is the routed scope (doc_ids). RoutedDocs carries each routed doc's
// OVERALL SUMMARY — the hint that feeds retrieval as a soft boost instead of a
// hard filter. NavHint is the joined summaries used as retrieval keywords so
// routed docs rank up WITHOUT excluding the rest of the corpus.
type NavContext struct {
	Direction  string
	KnownDocs  []string
	RoutedDocs [][2]string // (doc_id, summary)
	NavHint    string
}

// NavRule is one step of the navigation chain.
type NavRule struct {
	ID   string
	Tool string
	// Mode is ModeAuto or ModeLLM.
	Mode string
	// Run replaces the single-tool path for a step that composes several tools
	// (drill: retrieve + navigate_structure + merge).
	Run func(ctx context.Context, ts *Toolset, nav *NavContext, available map[string]bool, budgetS float64) ToolOutcome
	// Args builds the tool arguments from the running context (ModeAuto only;
	// also used for display when Run composes internally).
	Args func(nav *NavContext) map[string]any
	// When is an optional guard; the step is skipped when it returns false.
	When func(nav *NavContext) bool
	// Next maps status -> next rule id. "" (or a missing key) ends the chain.
	// Routing on STATUS is what makes "quality poor -> fall back" a code
	// decision rather than a prompt suggestion.
	Next map[string]string
}

// NavRules is the per-slot strategy ladder: "nav is a hint, not a constraint" — no
// rung filters the corpus down to the routed docs. The tree ROUTES (locate), then
// drill retrieves over the WHOLE corpus while softly boosting the routed-doc chunks,
// so an answer living outside them survives (ranked lower) instead of being dropped.
//
//	locate  navigate_tree    route to top-n documents, exposing their summaries
//	drill   retrieve+merge   whole-corpus retrieve (nav summaries as BM25 soft
//	                         hint) + navigate_structure paths merged on chunk_id
//	                         + routed-doc chunks re-ranked to the top
//	global  retrieve         safety net only if drill itself came back empty
//
// There is deliberately NO LLM verdict on the drill evidence: retrieval is not
// scope-locked, so there is no "did the scoped search miss?" signal to grade.
var NavRules = []NavRule{
	{
		ID:   "locate",
		Tool: "navigate_tree",
		Mode: ModeAuto,
		Args: func(nav *NavContext) map[string]any { return map[string]any{"query": nav.Direction} },
		// Tree missed => no routed hints to merge against; the only useful step
		// is an unscoped search. — {OK, MISS, EMPTY, POOR, ERROR};
		// REDUNDANT is deliberately absent: a redundant locate changed nothing,
		// and widening it to global would re-run the same corpus search that
		// produced the duplicates.
		Next: map[string]string{
			StatusOK:    "drill",
			StatusMiss:  "global",
			StatusEmpty: "global",
			StatusPoor:  "global",
			StatusError: "global",
		},
	},
	{
		ID:   "drill",
		Tool: "navigate_structure",
		Mode: ModeAuto,
		Run:  runDrillMerge,
		Args: func(nav *NavContext) map[string]any { return map[string]any{"query": nav.Direction} },
		// drill returns OK with the merged, re-ranked evidence; only an empty
		// whole-corpus result (MISS) or an infra failure falls through to global.
		// {OK, MISS, EMPTY, ERROR}; no POOR and no REDUNDANT.
		Next: map[string]string{
			StatusOK:    "",
			StatusMiss:  "global",
			StatusEmpty: "global",
			StatusError: "global",
		},
	},
	{
		ID:   "global",
		Tool: "retrieve",
		Mode: ModeAuto,
		Args: func(nav *NavContext) map[string]any { return map[string]any{"query": []string{nav.Direction}} },
		Next: map[string]string{},
	},
}

var navRuleByID = func() map[string]*NavRule {
	m := make(map[string]*NavRule, len(NavRules))
	for i := range NavRules {
		m[NavRules[i].ID] = &NavRules[i]
	}
	return m
}()

// NavStartRule is the ladder's entry point.
const NavStartRule = "locate"

// runDrillMerge is the `drill` rung (AUTO): corpus retrieve for REAL chunks +
// the root->chunk structure paths from navigate_structure, MERGED on chunk_id
// so every retrieved chunk carries its hierarchy context.
//
// Nav is a hint, not a constraint: retrieval runs over the WHOLE corpus (no
// doc_scope filter), but the nav-routed docs' summaries are fed back as
// retrieval keywords (soft boost), and the returned chunks are re-ranked so
// routed-doc chunks float to the top while non-routed chunks stay below. A
// multi-hop / enumeration answer living OUTSIDE the routed docs therefore
// survives — it just ranks lower — instead of being filtered out entirely.
func runDrillMerge(ctx context.Context, ts *Toolset, nav *NavContext, available map[string]bool, budgetS float64) ToolOutcome {
	var merged []any
	var evidenceIDs []string
	seenIDs := map[string]bool{}

	// 1. Skeleton A: whole-corpus retrieval, softly boosted by the nav hints.
	// The hint is an explicit PARAMETER, passed explicitly here instead of being parked on
	// the shared executor (which would make the model's own retrieve calls carry a hint they
	// were never given).
	retOC := ExecuteTool(ctx, ts, "retrieve", map[string]any{
		"query":    []string{nav.Direction},
		"nav_hint": nav.NavHint,
	})
	for _, cid := range retOC.EvidenceIDs {
		if !seenIDs[cid] {
			seenIDs[cid] = true
			evidenceIDs = append(evidenceIDs, cid)
		}
	}
	routedIDs := map[string]bool{}
	for _, d := range nav.KnownDocs {
		routedIDs[d] = true
	}

	// 2. Supplement B: root->chunk paths from EVERY routed doc's structure.
	paths := map[string]string{}
	if available["navigate_structure"] && len(nav.KnownDocs) > 0 {
		stepCtx, cancel := context.WithTimeout(ctx, stepBudget(budgetS))
		defer cancel()
		for _, docID := range nav.KnownDocs {
			select {
			case <-stepCtx.Done():
				break
			default:
			}
			sOC := ExecuteTool(stepCtx, ts, "navigate_structure", map[string]any{
				"doc_id": docID, "query": nav.Direction, "kind": "catalog",
			})
			for _, d := range sOC.EvidenceIDs {
				if !seenIDs[d] {
					seenIDs[d] = true
					evidenceIDs = append(evidenceIDs, d)
				}
			}
			for cid, path := range chunkPathsFromMetrics(sOC.Metrics) {
				if cid != "" {
					if _, ok := paths[cid]; !ok {
						paths[cid] = path
					}
				}
			}
		}
	}

	// 3. Merge: skeleton A is the base; attach structure_path where ids align.
	//    Re-rank so routed-doc chunks float to the top WITHOUT deleting the rest.
	for _, p := range retOC.Payload {
		e, ok := p.(map[string]any)
		if !ok {
			merged = append(merged, map[string]any{"content": fmt.Sprint(p)})
			continue
		}
		entry := make(map[string]any, len(e)+2)
		for k, v := range e {
			entry[k] = v
		}
		if cid, ok := entry["id"].(string); ok && cid != "" {
			if sp, ok := paths[cid]; ok {
				entry["structure_path"] = sp
			}
		}
		doc := ""
		if v, ok := entry["doc_id"].(string); ok {
			doc = v
		} else if v, ok := entry["document_id"].(string); ok {
			doc = v
		}
		rank := 1
		if routedIDs[doc] {
			rank = 0 // 0 = routed (top)
		}
		entry["nav_rank"] = rank
		merged = append(merged, entry)
	}
	// Stable sort: routed first, original order preserved within each group
	// (Go's sort.Slice is not stable).
	sort.SliceStable(merged, func(i, j int) bool {
		return navRankOf(merged[i]) < navRankOf(merged[j])
	})

	if len(merged) == 0 {
		return ToolOutcome{
			Payload:     []any{},
			EvidenceIDs: evidenceIDs,
			Status:      StatusMiss,
			Reason:      ReasonNoDoc,
			Metrics:     map[string]any{"hits": 0},
		}
	}
	return ToolOutcome{
		Payload:     merged,
		EvidenceIDs: evidenceIDs,
		Status:      StatusOK,
		Metrics:     map[string]any{"hits": len(merged), "routed": len(routedIDs)},
	}
}

func navRankOf(v any) int {
	m, ok := v.(map[string]any)
	if !ok {
		return 1
	}
	if n, ok := m["nav_rank"].(int); ok {
		return n
	}
	return 1
}

// stepBudget applies the per-step floor to the remaining budget.
func stepBudget(remaining float64) time.Duration {
	if remaining < navMinStepBudgetS {
		remaining = navMinStepBudgetS
	}
	return time.Duration(remaining * float64(time.Second))
}

// chunkPathsFromMetrics reads metrics["chunk_paths"], tolerating the shape drift
// of model/executor-produced metrics.
func chunkPathsFromMetrics(metrics map[string]any) map[string]string {
	out := map[string]string{}
	raw, ok := metrics["chunk_paths"]
	if !ok || raw == nil {
		return out
	}
	switch v := raw.(type) {
	case map[string]string:
		return v
	case map[string]any:
		for k, val := range v {
			out[k] = fmt.Sprint(val)
		}
	}
	return out
}

// navToolSurface is the set of tools this session may call.
func (t *Toolset) navToolSurface() map[string]bool {
	out := map[string]bool{}
	for _, s := range t.ActiveToolSpecs() {
		out[s.Function.Name] = true
	}
	return out
}

// NavExchange is one completed assistant/tool pair produced by the chain.
type NavExchange struct {
	Messages    []schema.Message
	EvidenceIDs []string
	Outcomes    []map[string]any
	// PendingRule is the rule control now rests on ("" when the ladder finished
	// or was abandoned).
	PendingRule string
	// BudgetLeft is the unspent prefix budget in seconds.
	BudgetLeft float64
}

// RunNavChain: run NavRules from startID, stopping
// at the first LLM step.
//
// AUTO steps execute here; an LLM step is returned without being run, because
// only the model can supply its arguments (e.g. which routed document to drill).
// Shared by the pre-session prefix and the in-session fallback, so both consume
// exactly the same ladder.
// idPrefix tags every tool_call id produced by a chain run so two chains sharing the same
// rule ids never collide: the prefix case emits "nav_<rule_id>" and the in-session ladder
// fallback emits "ladder_<rule_id>", and the provider rejects a history where two
// tool_calls share an id.
func RunNavChain(ctx context.Context, ts *Toolset, nav *NavContext, startID string, budgetS float64, idPrefix string, maxChars int) NavExchange {
	var out NavExchange
	if !NavRulesEnabled {
		return out
	}
	available := ts.navToolSurface()

	started := time.Now()
	ruleID := startID
	for ruleID != "" {
		rule, ok := navRuleByID[ruleID]
		if !ok || !available[rule.Tool] {
			break
		}
		if rule.Mode == ModeLLM {
			// Hand control back: the model must decide.
			out.PendingRule = ruleID
			out.BudgetLeft = budgetS - time.Since(started).Seconds()
			return out
		}
		if rule.When != nil && !rule.When(nav) {
			break
		}
		remaining := budgetS - time.Since(started).Seconds()
		if remaining <= 1.0 {
			_LOG.Printf("[Action Session] nav chain out of budget before %q", ruleID)
			break
		}

		var oc ToolOutcome
		if rule.Run != nil {
			// A composed step (e.g. drill) owns its own tool calls.
			oc = rule.Run(ctx, ts, nav, available, remaining)
		} else {
			stepCtx, cancel := context.WithTimeout(ctx, minDuration(stepBudget(remaining),
				time.Duration(navPrefixCallTimeoutS*float64(time.Second))))
			oc = ExecuteTool(stepCtx, ts, rule.Tool, rule.Args(nav))
			cancel()
		}

		out.Outcomes = append(out.Outcomes, map[string]any{
			"name": rule.Tool, "status": oc.Status, "reason": oc.Reason, "metrics": oc.Metrics,
		})
		if rule.Tool == "navigate_tree" && oc.Status == StatusOK {
			// Routed docs become the validated scope for every later step.
			nav.KnownDocs = appendUnique(nav.KnownDocs, docIDsFromPayload(oc.Payload)...)
			nav.RoutedDocs = routedDocsFromMetrics(oc.Metrics)
			var hints []string
			for _, rd := range nav.RoutedDocs {
				if rd[1] != "" {
					hints = append(hints, rd[1])
				}
			}
			if len(hints) > 0 {
				nav.NavHint = truncateRunes(strings.Join(hints, " "), navHintChars)
			}
		}
		out.EvidenceIDs = append(out.EvidenceIDs, oc.EvidenceIDs...)
		out.consumeExchange(ruleID, rule.Tool, rule.Args(nav), oc, idPrefix, maxChars)
		ruleID = rule.Next[oc.Status]
	}
	out.PendingRule = ruleID
	out.BudgetLeft = budgetS - time.Since(started).Seconds()
	return out
}

// consumeExchange appends one completed assistant/tool exchange.
//
// The pair MUST be well formed — every tool_call needs its tool response, or
// the provider rejects the history (the reason stripUnpairedToolCalls exists).
//
// maxChars caps the serialized payload: the in-session ladder continuation appends these
// pairs OUTSIDE the tool node's own accounting, so it passes the session's remaining
// context budget down. 0 means uncapped (the prefix path).
func (e *NavExchange) consumeExchange(ruleID, tool string, args map[string]any, oc ToolOutcome, idPrefix string, maxChars int) {
	callID := fmt.Sprintf("%s_%s", idPrefix, ruleID)
	rawArgs, err := json.Marshal(args)
	if err != nil {
		rawArgs = []byte("{}")
	}
	payload := marshalPassages(oc.Payload)
	// slices a str — CODE POINTS, not bytes — so the cap must be
	// rune-based too; a byte cut would hit CJK payloads ~3x early AND could
	// split a UTF-8 sequence, corrupting the JSON tool response.
	if maxChars > 0 && utf8.RuneCountInString(payload) > maxChars {
		payload = truncateRunes(payload, max(maxChars, 800))
	}
	e.Messages = append(e.Messages,
		*schema.AssistantMessage("", []schema.ToolCall{{
			ID:   callID,
			Type: "function",
			Function: schema.FunctionCall{
				Name:      tool,
				Arguments: string(rawArgs),
			},
		}}),
		*schema.ToolMessage(payload, callID),
	)
}

// RunNavPrefix: run the navigation ladder up to
// the first model-driven step.
//
// The returned messages are completed assistant/tool pairs ready to seed the
// session history, so the model sees the chain as work already done and cannot
// skip a step.
//
// Returns an empty exchange when navigation is unavailable — the ladder is an
// optimisation, never a precondition for the session to run.
func RunNavPrefix(ctx context.Context, ts *Toolset, direction string, deadlineLeft float64, nav *NavContext) NavExchange {
	if !NavRulesEnabled || ts == nil {
		return NavExchange{}
	}
	available := ts.navToolSurface()
	if len(available) == 0 || !available["navigate_tree"] {
		return NavExchange{}
	}
	if nav == nil {
		nav = &NavContext{Direction: direction}
	}
	budget := deadlineLeft * navPrefixBudgetRatio
	if budget < navPrefixMinBudgetS {
		budget = navPrefixMinBudgetS
	}
	// maxChars 0: the prefix runs before the session's context accounting starts.
	return RunNavChain(ctx, ts, nav, NavStartRule, budget, "nav", 0)
}

// Payload helpers

// docIDsFromPayload reads payload[0]["doc_ids"] from a navigate_tree result.
func docIDsFromPayload(payload []any) []string {
	if len(payload) == 0 {
		return nil
	}
	first, ok := payload[0].(map[string]any)
	if !ok {
		return nil
	}
	return toStringList(first["doc_ids"])
}

// routedDocsFromMetrics reads metrics["routed_docs"] — a list of (doc_id,
// summary) pairs, which models/executors emit either as [doc, summary] lists or
// as {"doc_id":..., "summary":...} objects.
func routedDocsFromMetrics(metrics map[string]any) [][2]string {
	raw, ok := metrics["routed_docs"]
	if !ok || raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([][2]string, 0, len(list))
	for _, item := range list {
		switch v := item.(type) {
		case []any:
			if len(v) >= 2 {
				out = append(out, [2]string{fmt.Sprint(v[0]), fmt.Sprint(v[1])})
			} else if len(v) == 1 {
				out = append(out, [2]string{fmt.Sprint(v[0]), ""})
			}
		case []string:
			if len(v) >= 2 {
				out = append(out, [2]string{v[0], v[1]})
			} else if len(v) == 1 {
				out = append(out, [2]string{v[0], ""})
			}
		case map[string]any:
			id := fmt.Sprint(v["doc_id"])
			summary := ""
			if s, ok := v["summary"].(string); ok {
				summary = s
			}
			if id != "" {
				out = append(out, [2]string{id, summary})
			}
		}
	}
	return out
}

func toStringList(v any) []string {
	list, ok := v.([]any)
	if !ok {
		if ss, ok := v.([]string); ok {
			return ss
		}
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if item == nil {
			continue
		}
		if s := fmt.Sprint(item); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func appendUnique(existing []string, add ...string) []string {
	seen := make(map[string]bool, len(existing))
	for _, v := range existing {
		seen[v] = true
	}
	for _, v := range add {
		if !seen[v] {
			seen[v] = true
			existing = append(existing, v)
		}
	}
	return existing
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// ===================== run.go (entry points) =====================
// The two public entry points of the action session, placed at the END of the file:
//   - RunActionSession
//   - InitializeState

// PromptLoader loads a named prompt template (e.g. "action_run",
// "action_initialize_state"). The runtime takes this as an interface so the templates can
// come from the repository's prompt files, from embedded assets, or from a test double.
type PromptLoader interface {
	Load(name string) (string, error)
}

// StringPromptLoader serves templates from an in-memory map. Used by tests and
// as the fallback when no filesystem loader is wired.
type StringPromptLoader map[string]string

// Load implements PromptLoader.
func (p StringPromptLoader) Load(name string) (string, error) {
	if t, ok := p[name]; ok {
		return t, nil
	}
	return "", fmt.Errorf("runtime: prompt %q not found", name)
}

// SessionDeps are the dependencies of one action session.
type SessionDeps struct {
	Tools *Toolset
	Model SessionModel
	// Prompts supplies the "action_run" / "action_initialize_state" templates.
	// When nil, built-in fallback instructions are used.
	Prompts PromptLoader
	// KB is the shared evidence pool accumulated across the research so far.
	// When non-nil, its chunks are injected into the seed user prompt as
	// "ALREADY RETRIEVED" so the model does not re-retrieve evidence it already
	// has. Nil skips the injection.
	KB *Kbinfos
	// CoverageSeed is what this question's enumeration already brought back (see
	// EnumerateCoverage / CoverageSet.Render): the windows where the deed is stated, each
	// with the chunk id a member is cited by. When non-empty it travels in this
	// session's seed — a window is evidence, where a list of queries to make is advice.
	CoverageSeed string
}

// RunActionSession: a bounded session
// pursuing ONE direction.
//
// Returns an empty Result (no states) when no model is configured or the session fails:
// the failure is logged and an empty Result returned rather than propagating.
func RunActionSession(ctx context.Context, deps SessionDeps, direction string, parent State, deadlineLeft float64, baseSummary string, sharedToolCache *ToolCache, sharedSearchQueries []string) Result {
	system := loadPrompt(deps.Prompts, "action_run")
	seedUser := fmt.Sprintf("Direction: %s\n\nState:\n%s", direction, parent.RenderSlots())

	// A direction that IS an enumeration is seeded WITH THE METHOD, before its first turn.
	//
	// The method's first instruction is to propose more candidates than you expect, which is a
	// decision taken BEFORE the first query: seeded a turn later it is already too late to
	// change the candidates the session proposed. The gate is the enumeration itself (see
	// Coverage) and not a reading of the candidates (see setMethodFor). A set direction that is
	// not an enumeration gets the method from appendBatchProtocol, on the first batch it
	// writes.
	seededMethod := enumerationSeed(parent, deps.Prompts, deps.CoverageSeed)
	if seededMethod != "" {
		seedUser += "\n\n" + seededMethod
		// The seeded direction also widens the retrieval budget (see
		// Kbinfos.MarkSetDirection): on a set direction the caller's queries are
		// facets of one list, so the executor must not drop them silently.
		deps.KB.MarkSetDirection()
		// Logged because a seeded method is text inside a prompt: without this line
		// nothing distinguishes "the gate opened for the direction that needed it" from
		// "the gate opened for every direction", which is the failure mode to watch.
		_LOG.Printf("[Action Session] enumeration direction — method (and any evidence it brought back) added to the seed (%d char(s))", len(seededMethod))
		if seed := strings.TrimSpace(deps.CoverageSeed); seed != "" {
			// Said separately from the line above, because the difference is the whole
			// point: the seed carries what the enumeration FOUND, not queries to make
			// (see enumerationSeed).
			_LOG.Printf("[Action Session] enumeration windows in the seed (%d char(s)) — the corpus was asked, not the model.", len(seed))
		}
	}

	// ALREADY RETRIEVED: surface the evidence already in the shared pool so the model fills
	// slots from it instead of re-retrieving the same ground. Without this the ReAct loop
	// repeatedly searches evidence it already holds.
	if existing := extractRelevantEvidence(deps.KB, direction, 4); existing != "" {
		seedUser += "\n\nALREADY RETRIEVED (do NOT re-retrieve these — use them to fill slots or identify gaps):\n" + existing
	}

	if len(baseSummary) > 0 {
		seedUser += fmt.Sprintf("\n\nPrior round summary:\n%s", baseSummary)
	}

	if deps.Model == nil {
		_LOG.Printf("[Action Session] no usable model resolved for action session")
		return Result{Messages: nil, NewStates: nil}
	}

	budgetLeft := deadlineLeft
	if budgetLeft <= 0 {
		// A caller that omits the budget gets the shape's own clock (see
		// SessionWallS): the direction is known here even when the caller is not.
		budgetLeft = SessionWallS(parent)
	}

	st := &SessionState{
		Messages: []schema.Message{
			*schema.SystemMessage(system),
			*schema.UserMessage(seedUser),
		},
		ParentState:          parent,
		Tools:                deps.Tools,
		Model:                deps.Model,
		KB:                   deps.KB,
		PendingCalls:         nil,
		Done:                 false,
		NewStates:            nil,
		FoundAnswer:          nil,
		RetrievedEvidenceIDs: nil,
		Attempts:             0,
		DeadlineLeft:         budgetLeft,
		CtxBudget:            maxToolResponseChars * 4,
		ToolChars:            0,
		ToolCache:            sharedToolCache,
		SearchQueries:        sharedSearchQueries,
		SkippedDup:           0,
		ToolStrikes:          map[string]int{},
		ToolOutcomes:         nil,
		Direction:            direction,
		// The method this session may be handed, resolved here so nothing mid-session
		// needs the prompt loader. It arrives in the seed when the table declared a
		// set (see setProtocolFor), or on the first batch the caller writes
		// (appendBatchProtocol) — so it is loaded for every session, and shown at
		// most ONCE: a seeded session is marked as already carrying it, or it would receive the
		// whole text twice — once from the seed and once from the batch append.
		EnumerationProtocol: loadOptionalPrompt(deps.Prompts, "action_set"),
		BatchProtocolShown:  seededMethod != "",
	}
	if st.ToolCache == nil {
		st.ToolCache = NewToolCache()
	}

	// The graph loop is bounded by the turn budget and the deadline; the context
	// carries the wall-clock so a stalled provider cannot outlive the request.
	runCtx, cancel := context.WithTimeout(ctx, deadlineToDuration(budgetLeft))
	defer cancel()

	// Deterministic navigation prefix: run the ladder IN CODE and seed the
	// conversation with the completed exchanges, so the model starts on top of a
	// guaranteed-correct opening instead of being merely *told* the routing rule.
	// The prefix shares the ladder with the in-session fallback, and its pending
	// rule becomes the session's resting point (see SessionState.NavRuleID).
	nav := &NavContext{Direction: direction}
	prefixStarted := time.Now()
	prefix := RunNavPrefix(runCtx, deps.Tools, direction, budgetLeft, nav)
	// The prefix's elapsed time is charged to the session with a 10s floor, so prefix + loop
	// stay inside the caller's deadline. Leaving DeadlineLeft at budgetLeft let one action
	// session run a whole prefix too long.
	st.DeadlineLeft = max(10.0, budgetLeft-time.Since(prefixStarted).Seconds())
	if len(prefix.Messages) > 0 {
		st.Messages = append(st.Messages, prefix.Messages...)
		st.RetrievedEvidenceIDs = append(st.RetrievedEvidenceIDs, prefix.EvidenceIDs...)
		st.ToolOutcomes = append(st.ToolOutcomes, prefix.Outcomes...)
		st.NavRuleID = prefix.PendingRule
		st.RoutedDocs = nav.KnownDocs
		st.NavHint = nav.NavHint
		_LOG.Printf("[Action Session] nav prefix: %d exchange(s), %d evidence id(s), resting on %q",
			len(prefix.Messages)/2, len(prefix.EvidenceIDs), prefix.PendingRule)
	}

	if err := st.sessionLoop(runCtx); err != nil {
		_LOG.Printf("[Action Session] session failed: %v", err)
		return Result{Messages: nil, NewStates: nil}
	}
	return Result{
		Messages:             st.Messages,
		NewStates:            st.NewStates,
		FoundAnswer:          st.FoundAnswer,
		RetrievedEvidenceIDs: st.RetrievedEvidenceIDs,
		TerminalType:         st.TerminalType,
		TerminalPayload:      st.TerminalPayload,
	}
}

// InitResult: tuple return: the root slot
// table plus the queries for the first round.
type InitResult struct {
	Root         State
	FirstQueries []string
}

// InitializeState: ask the model to decompose
// the question into a slot table, with a deterministic fallback when the call
// fails.
//
// deadlineLeft bounds the decomposition call.
func InitializeState(ctx context.Context, deps SessionDeps, question string, fanoutHint []string, deadlineLeft float64) InitResult {
	system := loadPrompt(deps.Prompts, "action_initialize_state")
	user := "Question: " + question
	if len(fanoutHint) > 0 {
		lines := make([]string, 0, len(fanoutHint))
		for _, h := range fanoutHint {
			lines = append(lines, "- "+h)
		}
		// "\n".join(...), so the block has NO trailing newline.
		user += "\n\nCandidate aspects already identified:\n" + strings.Join(lines, "\n")
	}
	// `min(_INIT_TIMEOUT_S, deadline_left or _INIT_TIMEOUT_S)`:
	// 0 means "unset" (falls back to the full budget) while a NEGATIVE deadline —
	// an already-exhausted round — is used as-is and times the call out at once.
	tmo := initTimeoutS
	if deadlineLeft != 0 && deadlineLeft < tmo {
		tmo = deadlineLeft
	}

	raw := initChat(ctx, deps, system, user, tmo)
	data, _ := ExtractJSON(raw).(map[string]any)
	if len(data) == 0 {
		// One quick retry — transient provider stalls were observed (45s with zero bytes); a
		// second attempt succeeded in production logs. The retry gets a LONGER budget: on slow
		// models a 45s bound times out both times and the table degrades to
		// a single answer slot, losing the second hop of a multi-hop question.
		raw = initChat(ctx, deps, system, user, initRetryTimeout(tmo, deadlineLeft))
		data, _ = ExtractJSON(raw).(map[string]any)
	}

	// A value the reply cannot convert is a hard failure: the whole decomposition is
	// discarded and the planner fan-outs are used instead. An empty root reproduces that
	// here.
	var slots []Variable
	if rawList, ok := data["slots"].([]any); ok {
		for i, s := range rawList {
			m, ok := s.(map[string]any)
			if !ok {
				// Only dict entries become Variables.
				continue
			}
			rawID, hasID := m["id"]
			if !hasID {
				rawID = i
			}
			id, ok := asInt(rawID)
			if !ok {
				_LOG.Printf("[Action Session:init] slot %d has a non-integer id (%v); discarding the decomposition", i, rawID)
				return InitResult{}
			}
			// `str(s.get("type") or "entity")`: a falsy type is replaced, any
			// other value is STRINGIFIED (never rejected).
			vType := "entity"
			if rawType, hasType := m["type"]; hasType && isTruthy(rawType) {
				vType = displayText(rawType)
			}
			clues, ok := asStringList(m["clues"])
			if !ok {
				_LOG.Printf("[Action Session:init] slot %d has non-iterable clues (%v); discarding the decomposition", i, m["clues"])
				return InitResult{}
			}
			if len(clues) > 4 {
				clues = clues[:4]
			}
			// The slot may DECLARE the act words its enumeration must cover (see
			// Variable.Terms): a missing or malformed list is simply no declaration,
			// never a reason to discard the decomposition.
			terms, _ := asStringList(m["scan"])
			if len(terms) > CoverageActWordsMax {
				terms = terms[:CoverageActWordsMax]
			}
			kept := make([]string, 0, len(terms))
			for _, t := range terms {
				if t = strings.TrimSpace(t); t != "" {
					kept = append(kept, t)
				}
			}
			subject := strings.TrimSpace(displayText(m["subject"]))
			slots = append(slots, Variable{ID: id, Type: vType, QuestionClues: clues, Terms: kept, Subject: subject})
		}
	}
	// `[str(q).strip for q in (data.get("first_queries") or [])][:3]`:
	// the first three entries are stripped and KEPT even when they end up empty
	// (the filter that used to live here made Go pick later entries instead), and
	// a non-iterable value raises exactly like the slot parsing above.
	rawFirst, ok := asStringList(data["first_queries"])
	if !ok {
		_LOG.Printf("[Action Session:init] first_queries is not iterable (%v); discarding the decomposition", data["first_queries"])
		return InitResult{}
	}
	var firstQueries []string
	for j, q := range rawFirst {
		if j >= 3 {
			break
		}
		firstQueries = append(firstQueries, strings.TrimSpace(q))
	}

	if len(slots) == 0 {
		// Decomposition failed (timeout/parse): build the table from planner
		// fanouts so the first round still targets DISTINCT aspects instead of
		// one oversized query (observed: single-slot trees answered directly and
		// died, or grepped the raw question and missed).
		if len(fanoutHint) > 0 {
			limit := min(len(fanoutHint), 4)
			for i := 0; i < limit; i++ {
				h := fanoutHint[i]
				if len([]rune(h)) > 120 {
					h = string([]rune(h)[:120])
				}
				slots = append(slots, Variable{ID: i, Type: "aspect", QuestionClues: []string{h}})
			}
			if len(firstQueries) == 0 {
				limit = min(len(fanoutHint), 3)
				for i := 0; i < limit; i++ {
					firstQueries = append(firstQueries, fanoutHint[i])
				}
			}
		} else {
			slots = []Variable{{ID: 0, Type: "answer", QuestionClues: []string{question}}}
			if len(firstQueries) == 0 {
				firstQueries = []string{question}
			}
		}
	} else if len(firstQueries) == 0 {
		firstQueries = []string{question}
	}

	root := NewState(slots, 0, nil)
	_LOG.Printf("[Action Session:init] %s\n%s", root.Brief(), root.RenderSlots())
	return InitResult{Root: root, FirstQueries: firstQueries}
}

// initRetryTimeout: the
// slot-table decomposition retry gets a longer window than the first attempt,
// because on slow models the 45s bound times out both times and the table
// degrades to a single answer slot.
//
//   - floor: the first attempt's budget (never shrink below what already failed)
//   - ceiling: 2x the first attempt, capped at a 90s absolute upper bound
//   - deadline: leave at least 5s of the round budget for the rest of the session
func initRetryTimeout(firstTmo, deadlineLeft float64) float64 {
	if deadlineLeft <= 0 {
		return min(2*firstTmo, 90.0)
	}
	return max(firstTmo, min(min(2*firstTmo, 90.0), deadlineLeft-5.0))
}

// initChat: ONE bounded LLM turn for the slot-table
// decomposition. Returns "" on timeout or failure — the caller falls back.
func initChat(ctx context.Context, deps SessionDeps, system, user string, tmo float64) string {
	if deps.Model == nil {
		return ""
	}
	if tmo <= 0 {
		// `asyncio.timeout(tmo)` with a spent budget fires on
		// the next tick, so the turn is abandoned before it starts. Going through
		// deadlineToDuration would silently hand it the full ACTION_TIMEOUT
		// instead, spending a budget the round no longer has.
		_LOG.Printf("[Action Session:init] timed out (%ds)", int(tmo))
		return ""
	}
	callCtx, cancel := context.WithTimeout(ctx, deadlineToDuration(tmo))
	defer cancel()
	reply, err := deps.Model.Complete(callCtx, []schema.Message{
		*schema.SystemMessage(system),
		*schema.UserMessage(user),
	}, nil)
	if err != nil {
		_LOG.Printf("[Action Session:init] failed: %v", err)
		return ""
	}
	return reply.Content
}

// Model / tool-calling seam: tool specs, parsed tool calls, and the completion call.

// ToolFunction is the OpenAI-style function descriptor.
type ToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolSpec is an OpenAI-style tool schema.
type ToolSpec struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// ToolCall is one tool invocation requested by the model.
type ToolCall struct {
	ID      string
	Name    string
	Args    map[string]any
	Unknown bool // name is not in ToolMap — answer with a correction, never execute
}

// ModelReply is one model turn: either free text, or tool calls, or both.
type ModelReply struct {
	Content   string
	ToolCalls []ToolCall
	Tokens    int // total tokens, for convenience (== Usage.TotalTokens when Usage != nil)
	// Usage is the provider-reported prompt/completion/total split. Nil when the
	// provider reported nothing.
	Usage *chat.Usage
}

// TemperatureModel is an OPTIONAL extension of SessionModel: models that can
// vary the sampling temperature per call implement it.
//
// Temperatures are set per node (keyword extraction uses 0.1, the answer composition uses
// the default). Without this seam every node would run at whatever the invoker defaulted
// to. Callers that need a specific temperature type-assert to this interface and fall back
// to Complete when it is not implemented.
type TemperatureModel interface {
	CompleteWithTemperature(ctx context.Context, messages []schema.Message, tools []ToolSpec, temp float64) (*ModelReply, error)
}

// ContextLengthModel is implemented by models that can report their context window in
// tokens, which message-fitting nodes use as the budget for chat.FitMessages. Callers
// type-assert for it and fall back to chat.EffectiveContextLength's 8192 default when it
// is absent, i.e. when the model config omits a context length. This is the fitting
// counterpart to the TemperatureModel seam.
type ContextLengthModel interface {
	ContextLength() int
}

// SessionModel is ONE model turn with THIS mode's tool surface.
//
// The production implementation (InvokerSessionModel) binds the tool schemas natively
// through the chat seam (chat.Request.Tools + ToolChoiceAuto) and reads the calls back from
// Response.ToolCalls.
type SessionModel interface {
	Complete(ctx context.Context, messages []schema.Message, tools []ToolSpec) (*ModelReply, error)
}

// StreamingSessionModel is implemented by models that can emit the answer
// incrementally. Callers type-assert for it and fall back to the one-shot
// Complete when it is absent, so streaming is strictly an enhancement (fed by the graph's
// token stream).
type StreamingSessionModel interface {
	SessionModel
	// StreamComplete sends the reply in pieces. onDelta receives each piece and
	// isThink tells whether it belongs to a hidden reasoning block; both are
	// forwarded to the sink.
	StreamComplete(ctx context.Context, messages []schema.Message, tools []ToolSpec, onDelta func(delta string, isThink bool) error) (*ModelReply, error)
}

// Parsing of model output.
//
// Every LLM call in the runtime asks for JSON, and models wrap that JSON in
// thinking preamble and Markdown fences. The approach here is to strip the wrappers first,
// then repair, and let encoding/json handle the rest.

var reFence = regexp.MustCompile("```(?:json)?\\s*|\\s*```")

// UnmarshalModelJSON strips any thinking preamble and Markdown fences, then parses JSON.
// An empty result parses as an empty object so callers can index into `out` without a nil
// check.
func UnmarshalModelJSON(text string, out any) error {
	text = common.StripThinkTrailing(text)
	text = reFence.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	if text == "" {
		return json.Unmarshal([]byte("{}"), out)
	}
	return json.Unmarshal([]byte(text), out)
}

// resolveLoader returns the configured PromptLoader, or the embedded
// Markdown-backed loader when none is wired. The runtime defaults to the repository's
// authoritative templates instead of the terse Go fallback constants.
func resolveLoader(p PromptLoader) PromptLoader {
	if p != nil {
		return p
	}
	return prompts.EmbeddedPromptLoader{}
}

// loadPrompt resolves the prompt loader (defaulting to prompts.EmbeddedPromptLoader
// when none is wired on SessionDeps) and loads name. The canonical templates are
// the Markdown files action_run.md and action_initialize_state.md under
// internal/rag/prompts, embedded into the binary via //go:embed so they are never absent
// at runtime. A missing template is an error, not a silent fallback to a stale string.
func loadPrompt(p PromptLoader, name string) string {
	t, err := resolveLoader(p).Load(name)
	if err != nil {
		panic(fmt.Sprintf("loadPrompt(%q): %v", name, err))
	}
	return t
}

// loadOptionalPrompt loads a template a given loader may not carry, and returns ""
// instead of panicking.
//
// action_set is optional by design: a loader that predates it (or a test's
// in-memory loader) simply gets no enumeration protocol, and its session runs
// exactly as it did before the protocol existed.
func loadOptionalPrompt(p PromptLoader, name string) string {
	t, err := resolveLoader(p).Load(name)
	if err != nil {
		return ""
	}
	return t
}

// deadlineToDuration converts a seconds budget to a context deadline.
// A non-positive budget falls back to the default session timeout so a caller
// that omits it does not produce an already-expired context.
func deadlineToDuration(seconds float64) time.Duration {
	if seconds <= 0 {
		seconds = actionTimeoutS
	}
	return time.Duration(seconds * float64(time.Second))
}

// extractRelevantEvidence flattens the shared evidence pool into a compact, line-delimited
// digest the model can read without re-retrieving. Chunks are ranked by the number of
// direction tokens they contain, then the top maxChunks are surfaced so the seed prompt
// stays bounded.
func extractRelevantEvidence(kb *Kbinfos, direction string, maxChunks int) string {
	if kb == nil || maxChunks <= 0 {
		return ""
	}
	chunks := kb.Chunks
	if len(chunks) == 0 {
		return ""
	}

	// Build the set of direction tokens (>=2 chars, alnum or CJK), then rank chunks by how
	// many tokens appear in their text.
	dirTokens := tokenizeDirection(direction)
	var ranked []map[string]any
	if len(dirTokens) == 0 {
		// No usable direction: fall back to the tail of the pool.
		if len(chunks) > maxChunks {
			ranked = chunks[len(chunks)-maxChunks:]
		} else {
			ranked = chunks
		}
	} else {
		type scored struct {
			chunk map[string]any
			score int
		}
		scoredChunks := make([]scored, 0, len(chunks))
		for _, c := range chunks {
			scoredChunks = append(scoredChunks, scored{chunk: c, score: directionRelevance(c, dirTokens)})
		}
		sort.SliceStable(scoredChunks, func(i, j int) bool {
			return scoredChunks[i].score > scoredChunks[j].score
		})
		if len(scoredChunks) > maxChunks {
			scoredChunks = scoredChunks[:maxChunks]
		}
		for _, s := range scoredChunks {
			ranked = append(ranked, s.chunk)
		}
	}

	var b strings.Builder
	for _, c := range ranked {
		content := strings.TrimSpace(ChunkTextOf(c))
		if content == "" {
			continue
		}
		// Cap each chunk at evidenceDigestChars and flatten newlines so the digest
		// stays single-line per chunk. The cap is evidenceDigestChars, which is sized to hold a
		// whole sentence (see the constant).
		content = truncateRunes(content, evidenceDigestChars)
		content = strings.ReplaceAll(content, "\n", " ")
		// "[cid] text" so the model can cite the chunk id.
		b.WriteString("[")
		b.WriteString(ChunkIDOf(c))
		b.WriteString("] ")
		b.WriteString(content)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// tokenizeDirection splits a direction string into >=2-char alnum/CJK tokens.
func tokenizeDirection(direction string) []string {
	if direction == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var tokens []string
	var buf strings.Builder
	flush := func() {
		if buf.Len() >= 2 {
			tok := buf.String()
			if _, dup := seen[tok]; !dup {
				seen[tok] = struct{}{}
				tokens = append(tokens, tok)
			}
		}
		buf.Reset()
	}
	for _, r := range strings.ToLower(direction) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || (r >= '一' && r <= '鿿'):
			buf.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

// directionRelevance counts how many direction tokens appear in a chunk's text.
func directionRelevance(chunk map[string]any, dirTokens []string) int {
	text := strings.ToLower(ChunkTextOf(chunk))
	if text == "" {
		return 0
	}
	score := 0
	for _, t := range dirTokens {
		if strings.Contains(text, t) {
			score++
		}
	}
	return score
}
