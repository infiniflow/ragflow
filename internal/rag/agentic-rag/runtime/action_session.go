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
//   - applyPatch

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
	// Subjects is WHO the act is about: one entry per spelling the SOURCE uses for them, declared
	// next to the act words. It is a LIST and not a delimited string, because a runtime that splits
	// a string is a runtime guessing the writer's punctuation — the same reason Variable.Value is
	// opaque (see its note). The plan writes JSON; an array is what a list looks like in JSON.
	//
	// Empty means the slot is not about one actor, and the scan asks nothing for it.
	Subjects []string
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
	// Unresolved is what the session itself said it could NOT establish (the text of its
	// <unresolved> block, see ParseUnresolved). It travels with FoundAnswer because the two
	// together say whether the answer is finished: an answer that names an open part is not a
	// reason to stop researching (see routeResearch).
	Unresolved string
	// EvidenceRefs is the session's evidence registry in first-seen order: the chunk ids the
	// model was shown as [ID:0], [ID:1], … (see stampEvidenceRefs). A caller that lets the
	// session's own answer stand uses it as the citation list — the numbers the model wrote are
	// indices into THIS slice.
	EvidenceRefs []string
	TerminalType *string
	// TerminalPayload carries the terminal's structured payload (see TerminalType).
	TerminalPayload map[string]any
}

// applyPatch: ONLY existing
// ids are patchable; the mutable fields are candidate / candidate_strength /
// discovered_clues. ID is immutable, so patches may not add variables.
//
// Returns nil when a patch entry is malformed (not a map, or missing "id") or when
// nothing actually changed.
func applyPatch(base State, branchPatches []map[string]any) *State {
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
			if f, ok := ToFloat(raw); ok {
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
					nv.DiscoveredClues = append(nv.DiscoveredClues, TruncateRunes(fmt.Sprint(c), 160))
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

// TruncateRunes cuts s to at most n characters (runes, not bytes). A non-positive n yields the
// empty string rather than a panic: the two copies this replaces disagreed on that case, and the
// stricter one won.
func TruncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
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
func ToFloat(v any) (float64, bool) {
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
		return parseFloat(n)
	}
	return 0, false
}

// parseFloat parses a float64 from a string, tolerating surrounding whitespace. It is the lenient arm of toFloat (numeric strings pass) and stays package-private: nothing outside this package coerces strings to floats.
func parseFloat(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// isTruthy applies bool() truthiness to the JSON-decoded values that reach applyPatch:
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

// extractTag: exact-tag extraction first, then
// lenient fallbacks for models that wrap the JSON in code fences or emit bare
// objects (observed with DeepSeek-class models ignoring the XML protocol).
//
// Returns "" when the tag is absent.
func extractTag(text, tag string) string {
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
//   - toolMap (the dispatch registry)
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

// arrayParamWithReason is arrayParam plus `reason`: one short statement of the SPECIFIC new clue
// or gap this call is pursuing.
//
// The field is the paper's (its search tool requires a <100-word reason naming the new clue),
// and what it buys is the failure mode our own logs showed — a call that is a paraphrase of the
// last one. A reason that names a new clue cannot be written for a call that has none, so the
// model has to say which one it is chasing; the near-duplicate check (SkippedDup) still catches
// the case where it says one and searches another, but by then the call is already on record.
//
// It is optional in the schema on purpose: a REQUIRED field turns a retrieval the model needs
// into a malformed call, which is a worse trade than an occasionally missing reason line.
func arrayParamWithReason(minItems, maxItems int) map[string]any {
	p := arrayParam("", minItems, maxItems)
	p["properties"].(map[string]any)["reason"] = map[string]any{
		"type": "string",
		"description": "one sentence, under 100 words: the SPECIFIC new clue or gap this call " +
			"pursues (a name, a date, a quoted phrase you have not searched yet). Not a " +
			"restatement of the question, and not 'try another phrasing'.",
	}
	return p
}

// arrayParamWithReasonAndDocScope is arrayParamWithReason plus `doc_scope`: the retrieve family honours
// a scope that pins the search INSIDE documents the model already has (see resolveDocScope), and this is
// the only tool that does. Decoding is ceiling-ed by the session's own DocScope and verified against the
// bound datasets, so a stale or out-of-dataset doc_id cannot widen the search — the property exists so
// the model can ASK for it, which it could not when the schema omitted it while the tool accepted it.
func arrayParamWithReasonAndDocScope(minItems, maxItems int) map[string]any {
	p := arrayParamWithReason(minItems, maxItems)
	p["properties"].(map[string]any)["doc_scope"] = map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": "optional doc_ids from a prior tool result (metadata_search / navigate_tree / list_chunks), to search inside those documents only",
	}
	return p
}

// Tool schemas. The descriptions are the model's only guide for when to pick which tool,
// and rephrasing them changes routing behaviour.
var (
	retrieveToolSpec = ToolSpec{
		Type: "function",
		Function: toolFunction{
			Name: "retrieve",
			Description: `WHEN TO CALL: you know or suspect exact surface terms in the corpus (names, titles, codes, phrases) — the first recall pass; cover different facets.` +
				`HOW IT WORKS: keyword recall FIRST, then the pattern applies to what came back — ` + "`A.*B`" + ` (A then B, anything between) matches only inside the passages its operands recalled, and the RAREST operand bounds it: put the rare word first. A FULL recall page was truncated.` +
				`ONE STRING, MANY TERMS: ` + "`A|B|C`" + ` recalls each in ONE call — synonyms go in one string.` +
				`ENUMERATING A SET: probe the NAMES themselves, alternated with |, 4-6 per query; hits are members — guess the next batch yourself.` +
				`DO NOT CALL: for a whole document (list_chunks); when no surface word matches (search_chunks).` +
				`ARGUMENTS: query — array of 1-3 strings (only the first ~10 snippets survive); doc_scope — optional doc_ids from an earlier tool result, to search INSIDE them only.` +
				`OUTPUT: Exact-term snippets with doc_id and chunk id. ok = new evidence; redundant = seen.` +
				`IF IT FAILS: a miss on an exact-term probe means the corpus lacks that term — in an enumeration that is a RESULT (record it as not a member, probe the next). redundant = stop and emit a state patch.`,
			Parameters: arrayParamWithReasonAndDocScope(1, 3),
		},
	}

	listChunksToolSpec = ToolSpec{
		Type: "function",
		Function: toolFunction{
			Name: "list_chunks",
			Description: `WHEN TO CALL: You need the FULL text of one document (enumeration, counts, arithmetic over many passages) and you already have its doc_id from a prior tool result.` +
				`DO NOT CALL: When you only need a single passage (use search_chunks or retrieve first); when you have no doc_id yet (locate it via navigate_tree or search_chunks first).` +
				`ARGUMENTS: doc_id — string, the document id seen in a retrieve / search_chunks / navigate result; offset — integer, the chunk to start this page at (default 0). There is no chunk_ids argument.` +
				`OUTPUT: At most 30 chunks of the document in reading order, starting at offset. ok = new evidence; redundant = already in pool. The result's note says where this page sits and whether the document CONTINUES — a document you have not paged to the end is not fully read, so call again with the offset advanced past the chunks you were shown.` +
				`IF IT FAILS: An unknown or blank doc_id yields an empty result (query-level miss, not a dataset fact) — pick a different doc_id or locate one first. Do not treat this as a reason to disable the tool.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"doc_id": map[string]any{
						"type":        "string",
						"description": "document id seen in a retrieve snippet",
					},
					"offset": map[string]any{
						"type":        "integer",
						"description": "chunk to start this page at (default 0); advance it to read on",
					},
				},
				"required": []string{"doc_id"},
			},
		},
	}

	searchChunksToolSpec = ToolSpec{
		Type: "function",
		Function: toolFunction{
			Name: "search_chunks",
			Description: `WHEN TO CALL: Primary semantic recall. Use when exact retrieve returns nothing useful, when the corpus is large and you are unsure which document holds the answer, or when the answer passage shares no surface words with your query. Send 1-2 queries; compiled-structure expansion is automatic (a no-op without compiled structure). ` +
				`DO NOT CALL: When you already have a doc_id and want to read that document (use list_chunks); when a single exact passage would be found faster by grep-style retrieve.` +
				`ARGUMENTS: query — array of 1-2 strings.` +
				`OUTPUT: Relevance-ranked snippet chunks, possibly with structural neighbours appended. ` +
				`Results may LEAD with [claim score=...] entries — the dataset's compiled atomic facts carrying VERBATIM source quotes. If a claim directly answers the query, cite it and answer WITHOUT further searching; deep-read its listed chunk only for missing context or numbers. ` +
				`ok = new evidence; redundant = already seen.` +
				`IF IT FAILS: miss means this query matched nothing — change the angle or fall back to retrieve or navigate_tree. Re-issuing a near-duplicate query is skipped as redundant, so vary the query instead of paraphrasing it.`,
			Parameters: arrayParamWithReason(1, 2),
		},
	}

	metadataSearchToolSpec = ToolSpec{
		Type: "function",
		Function: toolFunction{
			Name: "metadata_search",
			Description: `WHEN TO CALL: SELECT the document set by METADATA before searching — call it when the question names explicit entities (a person, a time, a place) or any concrete name a metadata field would carry (a title, a file name, an author, a date), or needs a named subset. Use ONLY the AVAILABLE METADATA fields; prefer 'contains' with a distinctive substring. ` +
				`ONE FILTER PER CALL (two days = two calls), then spend the returned doc_ids: list_chunks(doc_id), navigate_structure(doc_id, query), or retrieve(query, doc_scope=[ids]). ` +
				`DO NOT CALL: nothing names a document/subset; you already hold a doc_id; counting or enumerating. ` +
				`ARGUMENTS: filters — [{key, value, op}] over the AVAILABLE METADATA fields; op — see enum; logic 'and'|'or'. String ops take ONE keyword, one call per keyword; 'in' takes a list; 'empty' none. Example (one day): [{key: 'update_time', op: 'start with', value: '2026-09-20'}]. ` +
				`OUTPUT: doc_ids — the handle other tools take — plus each matched document's metadata as CONTEXT ONLY, never an argument to pass on. No passages. ok = selected; miss = nothing matched. ` +
				`IF IT FAILS: 'no documents match' — shorten the substring or use search_chunks; do not retry.`,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filters": map[string]any{
						"type":     "array",
						"minItems": 1,
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								// The field enum is rendered PER SESSION from the dataset's
								// catalog (see metadataSearchSpecForCatalog); the shipped
								// parameter deliberately carries no enum, so no field is
								// advertised as available before the dataset says it is.
								"key": paramString("a metadata field of this dataset — one of those listed under AVAILABLE METADATA"),
								"value": map[string]any{
									"type": []any{"string", "array", "null"},
									"description": "the keyword/value to match: ONE string keyword for contains / = / start with / " +
										"end with / not contains (call once per keyword); a list for in / not in; nothing for empty / not empty. " +
										"Copy a value exactly as the dataset stores it — never re-normalise it. " +
										"A 'time' field stores 'YYYY-MM-DD HH:MM:SS', so filter ONE day with op 'start with' and the bare " +
										"'YYYY-MM-DD' — '=' never matches a stored time.",
								},
								"op": paramEnum("the comparison", "=", "contains", "not contains", "start with", "end with", "in", "empty", "not empty"),
							},
							"required": []string{"key", "op"},
						},
					},
					"logic": paramEnum("how several filters combine, default and", "and", "or"),
				},
				"required": []string{"filters"},
			},
		},
	}

	webSearchToolSpec = ToolSpec{
		Type: "function",
		Function: toolFunction{
			Name: "web_search",
			Description: `WHEN TO CALL: The needed fact is world knowledge, a recent event, or newer than the corpus (a current event, a person's alive-now status, a fresh statistic). This tool only appears when a web provider is configured.` +
				`DO NOT CALL: When the fact plausibly lives in the fixed corpus — prefer retrieve or search_chunks first. For corpus-only questions this tool is unavailable.` +
				`ARGUMENTS: query — array of 1-2 strings.` +
				`OUTPUT: Web results shaped like corpus chunks, merged into the same evidence pool.` +
				`IF IT FAILS: error (no provider) — it will not appear at all this session; if it does appear and fails, switch to corpus tools permanently and do not retry it.`,
			Parameters: arrayParam("", 1, 2),
		},
	}

	// wikiQueryToolSpec is deliberately absent from toolMap: wiki_query has no caller and is
	// not part of the action session's dispatch registry, so it stays unregistered. The
	// handler still lives in tool_exploration.go as an unplugged extension seam.

	navigateTreeToolSpec = ToolSpec{
		Type: "function",
		Function: toolFunction{
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
		Function: toolFunction{
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
		Function: toolFunction{
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
		Function: toolFunction{
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

const (
	// maxToolDescriptionRunes is the per-tool description budget TestToolSpecsHavePlaybookSections
	// pins.
	maxToolDescriptionRunes = 1200
)

// metadataSearchSpecForCatalog renders the metadata_search schema for a session's catalog: the
// `key` enum becomes the fields the dataset offers, so a model can name any of them. The
// description is dataset-independent (it points at AVAILABLE METADATA and never names a field),
// so it needs no per-session rewriting and cannot outgrow its budget.
//
// An empty (or nil) catalog returns the base spec UNCHANGED: no field is advertised at all —
// the `key` stays the free-form string the shipped spec carries, which is also how an empty
// enum is avoided (some providers reject `"enum": []`).
//
// The base spec is the shared package-level toolMap entry, so every map this touches is
// COPIED — mutating them in place would leak one session's dataset fields into every other
// session (and into a session's concurrent rag calls, see Toolset.mu).
func metadataSearchSpecForCatalog(base ToolSpec, cat *MetadataCatalog) ToolSpec {
	if cat == nil || cat.Empty() {
		return base
	}

	baseProps, _ := base.Function.Parameters["properties"].(map[string]any)
	filters, _ := baseProps["filters"].(map[string]any)
	items, _ := filters["items"].(map[string]any)
	itemProps, _ := items["properties"].(map[string]any)
	if baseProps == nil || filters == nil || items == nil || itemProps == nil {
		// The schema shape changed under us; the unpatched spec is still valid.
		return base
	}

	patchedItemProps := copyAnyMap(itemProps)
	patchedItemProps["key"] = paramEnum("one of this dataset's metadata fields (see AVAILABLE METADATA)", cat.Keys...)

	patchedItems := copyAnyMap(items)
	patchedItems["properties"] = patchedItemProps

	patchedFilters := copyAnyMap(filters)
	patchedFilters["items"] = patchedItems

	patchedProps := copyAnyMap(baseProps)
	patchedProps["filters"] = patchedFilters

	patchedParams := copyAnyMap(base.Function.Parameters)
	patchedParams["properties"] = patchedProps

	out := base
	out.Function.Parameters = patchedParams
	return out
}

// copyAnyMap shallow-copies a JSON-shaped map, so a patched tool spec shares no map with
// toolMap.
func copyAnyMap(m map[string]any) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// toolMap is the multi-tool registry.
// executeTool dispatches by name; add a tool by registering its schema here.
var toolMap = map[string]ToolSpec{
	"retrieve":           retrieveToolSpec,
	"search_chunks":      searchChunksToolSpec,
	"metadata_search":    metadataSearchToolSpec,
	"list_chunks":        listChunksToolSpec,
	"navigate_tree":      navigateTreeToolSpec,
	"navigate_structure": navigateStructureToolSpec,
	"calculate":          calculateToolSpec,
	"graph_explore":      graphExploreToolSpec,
	"web_search":         webSearchToolSpec,
}

// toolMapNames are the registered tool names, sorted for deterministic output.
func toolMapNames() []string {
	names := make([]string, 0, len(toolMap))
	for n := range toolMap {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// The per-session tool object

// toolExecutor runs one tool call by name and reports what happened.
//
// Every executor returns a ToolOutcome so the tool node can act on the KIND of
// result (empty / miss / poor / redundant / error) instead of measuring payload
// size.
type toolExecutor interface {
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
	// MetadataFields is the session's metadata catalog (see metadataCatalogFor). It supplies
	// the metadata_search key enum and rides the session seed, so a filter can name the fields
	// the dataset really carries. Nil or empty advertises no field at all — there is no field
	// name baked into the shipped schema to fall back on.
	MetadataFields *MetadataCatalog
	// DisabledTools holds tools proven unavailable this session (no compiled
	// structure of their kind).
	DisabledTools map[string]bool
	// mu guards DisabledTools: one Toolset is shared by a session's concurrent
	// rag calls (models.appendToolResults runs a round's tool calls in parallel),
	// so the map must not be written and read unsynchronized.
	mu sync.Mutex
	// Exec runs the tools.
	Exec toolExecutor
}

// GetThinkingMode implements thinkingModeCarrier so ResolveMode works on *Toolset.
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
		if s, ok := toolMap[name]; ok {
			if name == "metadata_search" {
				// Per-session rewrite: the catalog names the dataset's real fields. It
				// returns the shared spec untouched when there is no catalog.
				s = metadataSearchSpecForCatalog(s, t.MetadataFields)
			}
			out = append(out, s)
		}
	}
	return out
}

// DisableTool: mark a compile-only tool
// unavailable for the REST of this session, so ActiveToolSpecs stops
// advertising it and executeTool short-circuits it.
func (t *Toolset) DisableTool(name string) {
	if _, ok := toolMap[name]; !ok {
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

// reasonStatus: single source of truth for the cause→status mapping, so a tool cannot
// disagree with itself about what its own reason means.
// The unified tool-result contract shared by the session and the tool implementations
// (ToolOutcome plus the OK/EMPTY/MISS/POOR/REDUNDANT/ERROR statuses).

// Tool-result statuses: the signal the session's tool node acts on.
const (
	StatusOK        = "ok"        // normal hit
	StatusEmpty     = "empty"     // dataset-level: no such compiled structure exists here
	StatusMiss      = "miss"      // query-level: nothing matched THIS query; tool still valid
	statusPoor      = "poor"      // produced output, but too weak to be useful
	statusRedundant = "redundant" // ran fine, but added no NEW evidence
	StatusError     = "error"     // infra / provider failure
)

// Machine-readable ToolOutcome reasons. Only reasonNoStructure is
// DATASET-level and may disable a tool; ReasonNoDoc is QUERY-level and must not.
// ReasonUnwired labels a call to a tool NAME this deployment has no binding for
// (the model invented it): nothing ran, so it is reported as MISS and never
// counts toward the strikes that disable a real tool.
const (
	reasonNone        = ""
	reasonNoStructure = "no_structure"
	ReasonNoDoc       = "no_doc"
	reasonInfra       = "infra"
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

// reasonStatus maps a ToolOutcome reason to the Status it implies.
func reasonStatus(reason string) string {
	switch reason {
	case reasonNoStructure:
		return StatusEmpty
	case ReasonBadArgs, reasonInfra:
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
	// The digest's per-chunk length is the ALREADY RETRIEVED stage's allowance (see
	// deliverItemText). It matches what an admitted passage carries (passageFromChunk: 1200), so
	// the digest never shows a session LESS of a chunk than the same chunk would carry as a tool
	// result.
	//
	// The previous 300-code-point cut ended mid-sentence on narrative passages, and the model —
	// told to answer only from what it was shown — excluded the parties whose clause fell outside
	// the window: a cap smaller than one sentence truncates exactly the evidence the answer is
	// built from, even when the passage is in the pool.
	// maxToolResponseChars bounds ONE tool payload.
	maxToolResponseChars = 12000
	// foldedToolResultChars is the size above which an EARLIER turn's tool result is folded into
	// a digest. Below it the fold costs more than it saves, and a short result is cheap to keep.
	foldedToolResultChars = 1200
	// verbatimSessionTurns is how many of the most recent turns keep their tool results VERBATIM
	// (see compactEarlierToolResults).
	verbatimSessionTurns = 3
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
	// (see offerContinuation): medium/high run 8 → 16, ultra 10 → 18.
	//
	// It is a RUNAWAY GUARD, not the loop's bound. The bound is the clock: a turn may not spend the
	// answer's reserve (see runActionNode), and at the reserve the session goes to its answer turn.
	// When this was 4 the cap (12) was what actually stopped the longest sessions — so what ended
	// the research was a turn count, which is the thing the reference loops do not have at all
	// (the paper runs on a wall-clock budget T with no turn cap; WeKnora's MaxIterations is paired
	// with a finalize call, not with a cut).
	turnRunExtra = 8
	// valueTurnFloor is the fallback a session runs on when its mode declares no turn count at
	// all (see actionMaxTurns). The per-shape reduction that used to read this — a session that
	// was not assembling a set was cut to this floor whatever its mode said — is gone: it was
	// cutting multi-hop VALUE questions at four turns with the clock unspent.
	valueTurnFloor = 4
	// turnAskFloorS is the session clock below which no further turn is offered:
	// the finalize/salvage step must still fit, or the extra turn buys evidence
	// that never reaches the slot table.
	turnAskFloorS = 20.0
	// sessionClockGuardS separates the session's OWN clock from the context deadline it runs
	// under.
	//
	// They used to be the same number, which means every budget computed from the clock (a turn's
	// wall, the answer call's timeout, a tool call) could consume exactly up to the context
	// deadline and lose the race: the call returns at the boundary, the context fires, the session
	// graph errors out and the round is reported as cut — with the answer it had just written
	// never parsed (measured 2026-09-20, 20:39: 4 of 20 rounds, one of them right after a
	// `turn timed out after 62s; … asking for the answer` had already routed the session to its
	// answer turn). The clock is a PLAN; the context is the WALL. The plan must end first.
	sessionClockGuardS = 10.0
	// finalizeMarginS is what the answer call leaves unspent inside the session clock, so its own
	// reply can be received and parsed before either the clock or the context runs out.
	finalizeMarginS = 5.0
	// minSessionClockS is the floor for the session clock after the guard is subtracted: a session
	// handed almost no budget still gets a moment to answer rather than a negative clock.
	minSessionClockS = 5.0
	// minSalvageRetryS is what has to be left, after the answer's own margin, before the answer call
	// is retried: below it a second attempt would only race the wall (see the retry in finalizeNode).
	minSalvageRetryS = 8.0
	// answerReserveS is the slice of the session clock that belongs to the ANSWER turn: below it
	// the session stops searching and spends what is left writing the answer.
	//
	// 40s, and the reason it went back up is the shape of a session that is allowed to work: with
	// the turn floor in the mode's hands (see actionMaxTurns) a session legitimately spends its
	// clock instead of being cut at four turns, and the answer turn it needs is a full one — read
	// the record, write prose, cite every fact. Measured 2026-09-20 (FRAMES, 20:15): 5 of 19
	// sessions were cut by the clock mid-turn, and those rounds produced no answer at all. A reserve
	// that cannot fit the answer is not a reserve; the research it costs comes out of the same
	// 180s either way (TotalBudgetS).
	answerReserveS = 40.0
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

// isNearDup: true when q shares >=
// nearDupJaccard of its tokens with any query in seen.
func isNearDup(q string, seen []string) bool {
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

// ContextLength implements contextLengthModel. It returns the model's context
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
// (toolMap).
//
// It deliberately does NOT consult the active surface. Disabled or web-hidden
// tools are real tools that this session has stopped advertising; a model that
// still calls one must get the "unavailable, use this instead" note from
// executeTool, not an "unknown tool" correction. Judging against the active
// surface would turn every disabled call into Unknown and override that
// deliberately gentler degradation path.
func knownTool(name string) bool {
	_, ok := toolMap[name]
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

// sessionState: (the LangGraph TypedDict).
type sessionState struct {
	// Messages is the running conversation (system + user + assistant + tool).
	Messages []schema.Message
	// ParentState is the slot table being patched by this session.
	ParentState State
	Tools       *Toolset
	Model       SessionModel

	// PendingCalls are tool calls awaiting execution this turn.
	PendingCalls []ToolCall
	Done         bool
	// ForceAnswer stops the SEARCHING without ending the session: the next node is the answer
	// turn, which runs with tools OFF. It is how a failed turn, a timed-out call or a stuck reply
	// is handled now — the session keeps its record and still produces an answer, where it used to
	// end (discarding the record) and leave the round to the composition path.
	ForceAnswer bool

	NewStates   []State
	FoundAnswer *string
	// Unresolved is the session's own statement of what it could not establish, taken from its
	// <unresolved> block (see ParseUnresolved). Kept on the state because the round's routing
	// reads it: an answer beside an open question means the research has one more hop to try.
	Unresolved string

	RetrievedEvidenceIDs []string
	// EvidenceRefs is the session's evidence registry in FIRST-SEEN order:
	// EvidenceRefs[n] is the chunk the model was shown as [ID:n] (see stampEvidenceRefs).
	//
	// It is what makes the model's own citations checkable. The final-answer call used to be
	// the one that numbered the evidence (kb.CiteChunkIDs, written by the compose renderer); in
	// a session that answers from what it read, the numbers the model saw are the registry, and
	// the resolver is handed exactly this list.
	EvidenceRefs []string
	// evidenceRefOf maps a chunk id to its number in EvidenceRefs.
	evidenceRefOf map[string]int
	// lastReply is the previous turn's assistant text, so an exactly repeated reply can END the
	// session (see runActionNode).
	lastReply string
	Attempts  int
	// DeadlineLeft is what is left of the session's own clock (see sessionClockFor). It is DERIVED
	// from SessionDeadline at the top of every turn and route (see refreshClock).
	DeadlineLeft float64
	// SessionDeadline is when the session's clock runs out, as an absolute time.
	//
	// The clock used to be a number written ONCE (after the navigation prefix) and never read
	// again, so nothing inside the loop knew how much time was left: turns were bounded only by
	// the CALLER's context, which expires mid-call — which is how a session ends with no answer
	// and no patch (measured 2026-09-20, 三国: three turns of 36s and 74s inside a 110s clock,
	// `session cut` inside the third call, 0 patches, no answer, and an answer composed for it
	// that named six of the seventeen members).
	SessionDeadline time.Time
	// expectedTurnS is the longest turn this session has completed: the pace to plan the answer
	// turn around (see affordableSearchTurns). Zero until the first turn returns.
	expectedTurnS float64
	// BudgetS is the session's WHOLE clock, kept so the patch checkpoint can say how much of it is
	// gone. Only the remaining half moves.
	BudgetS float64
	// PatchChecked says the record checkpoint has already been delivered: it fires once per
	// session, because a session that ignored it once is not helped by hearing it again.
	PatchChecked bool
	// LastTurnNoticed says the "this is your last search turn" notice has been sent (see
	// runActionNode).
	LastTurnNoticed bool

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
	// RECORD line (see sessionRecord) and for the names the evidence offers;
	// the tools write it. Nil skips both.
	KB *Kbinfos
	// Record is the last turn's record — the facts the continuation decision is
	// made from (see offerContinuation and sessionRecord).
	Record sessionRecord
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

	// notesAtEvidence is how many passages had been shown when the model last wrote a note (see
	// sessionRecord.ShownSinceNote). It is the one mechanical fact behind "you have read more than you
	// have written down" — a COUNT about the session's own behaviour, not a judgement about what the
	// passages mean.
	notesAtEvidence int
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

// executeTool: dispatch ONE tool call by name.
//
// Returns a ToolOutcome whose status/reason let the caller act on WHAT happened
// empty payload, query miss, infra failure, or a run that added no new
// evidence — instead of only counting characters.
func executeTool(ctx context.Context, tools *Toolset, name string, args map[string]any) ToolOutcome {
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
			Reason: reasonNoStructure,
		}
	}
	if tools == nil || tools.Exec == nil {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: reasonInfra}
	}
	out, err := tools.Exec.Execute(ctx, name, args)
	if err != nil {
		return ToolOutcome{Payload: []any{}, Status: StatusError, Reason: reasonInfra}
	}
	return out
}

// Nodes

// runActionNode: ONE model turn with tools.
// It appends the assistant message and records any tool calls as pending.
func (s *sessionState) runActionNode(ctx context.Context) error {
	s.Attempts++
	s.refreshClock()
	// On the LAST search turn this session can afford, say so (see lastSearchTurnNotice): the
	// session is one turn's notice ahead of the wall, which is where a patch and an answer still
	// fit. The paper says this at 70% of T; here it is said at the point the MEASURED pace says no
	// further searching turn fits.
	if !s.LastTurnNoticed && s.Attempts >= s.effectiveTurnFloor() {
		s.LastTurnNoticed = true
		s.Messages = append(s.Messages, *schema.UserMessage(lastSearchTurnNotice))
		_LOG.Printf("[Action Session] last search turn (turn %d; the clock affords %d): told the session to close out with a patch and an answer.",
			s.Attempts, s.effectiveTurnFloor())
	}
	// Per-turn wall budget: max(15, min(75, deadline_left - answerReserveS)).
	//
	// The reserve is subtracted HERE and not only at routing time. The route checks the clock
	// BEFORE a turn, so a turn that started with just over the reserve could still run up to 75s,
	// overrun the reserve, and be cut by the session clock mid-call — the shape behind 5 of 19
	// sessions ending as "session failed: context has been canceled" in one FRAMES run
	// (2026-09-20). A turn may spend everything except the answer's own clock.
	wall := max(15.0, min(75.0, s.DeadlineLeft-answerReserveS))
	callCtx, cancel := context.WithTimeout(ctx, time.Duration(wall*float64(time.Second)))
	defer cancel()
	turnStarted := time.Now()
	reply, err := s.Model.Complete(callCtx, s.Messages, s.Tools.ActiveToolSpecs())
	// The session's own pace: what ONE call to this provider on this question costs. It is what the
	// search/answer split is planned around (see affordableSearchTurns), and it is measured rather
	// than assumed because it varies by an order of magnitude between providers.
	if turnWall := time.Since(turnStarted).Seconds(); turnWall > s.expectedTurnS {
		s.expectedTurnS = turnWall
	}
	if err != nil {
		// A failed or timed-out turn does NOT end the session and does NOT clear its record.
		//
		// It used to set Done and wipe NewStates — which threw away every patch the session had
		// written on earlier turns, so a single provider hiccup or a slow call erased the round's
		// whole record (the round then reported no work and the composition answered instead).
		// What a failed turn means is only "no more searching": the session still has its reserve
		// and the answer turn that the reserve exists for, so it goes there (see route).
		if callCtx.Err() == context.DeadlineExceeded {
			_LOG.Printf("[Action Session] turn timed out after %.0fs; keeping the %d branch(es) it had and asking for the answer",
				wall, len(s.NewStates))
		} else {
			_LOG.Printf("[Action Session] LLM call failed (%T); keeping the %d branch(es) it had and asking for the answer",
				err, len(s.NewStates))
		}
		s.ForceAnswer = true
		return nil
	}
	// The model's tool_calls are passed through verbatim on the assistant message so the
	// tool responses that follow can be paired by id. Passing nil leaves a dangling
	// tool_call and the next request is rejected with "tool call result does not follow
	// tool call".
	ensureToolCallIDs(reply.ToolCalls)
	s.Messages = append(s.Messages,
		*schema.AssistantMessage(reply.Content, assistantToolCalls(reply.ToolCalls)))

	// A model that repeats itself EXACTLY is not going to make progress, and every further turn
	// costs a full round trip (the WeKnora loop stops on the same signal, maxRepeatedResponse
	// Rounds = 2). Two identical replies in a row therefore END the session, converging with
	// whatever the record already holds.
	//
	// The comparison is the whole reply — words AND tool calls — because that is what "the same
	// turn again" means. It is a hard stop rather than a nudged retry: a nudge is another turn
	// with the same prompt, and the point is that this prompt already produced this reply.
	if replyText := strings.TrimSpace(reply.Content); replyText != "" && replyText == s.lastReply {
		// Same signal, same treatment as a failed turn: stop SEARCHING, keep the record, and let
		// the answer turn ask with a different prompt (tools off) instead of ending the session
		// with no answer at all.
		_LOG.Printf("[Action Session] two consecutive identical replies (%d rune(s)); stopping the search and asking for the answer.",
			utf8.RuneCountInString(replyText))
		s.ForceAnswer = true
		return nil
	} else {
		s.lastReply = replyText
	}

	if len(reply.ToolCalls) > 0 {
		s.PendingCalls = reply.ToolCalls
		return nil
	}
	s.PendingCalls = nil

	// No call: try the terminal protocol (<state> / <answer>).
	//
	// An ANSWER ends the session. A state patch does NOT — it is bookkeeping.
	//
	// A patch used to end it, and that single line is why this engine never answered: the prompt
	// tells the model to end every action with a patch ("ALWAYS end this action with a state patch"),
	// so the session stopped at the model's first checkpoint and the round's `FoundAnswer` stayed
	// nil. The answer then always came from the closing composition, which never read the passages —
	// measured 2026-09-20: 63 session turns over 18 questions (3.5/题 against 5.9/题 before), 0.2
	// rewrites per question (against 0.9), 8 answers that read "the evidence does not contain it",
	// and every citation decided by the six blocks that composition happens to render.
	//
	// A checkpoint is not an ending. The session keeps working — the model patched what it found and
	// can go on looking — and what actually stops it is an answer, the turn budget, the clock, or a
	// round that stops learning (see route/routeAfterTool).
	newStates, foundAnswer, terminalType, payload := parseTerminal(reply.Content, s.ParentState)
	s.TerminalType = terminalType
	s.TerminalPayload = payload
	// Recorded on every turn, answer or not: the last statement is the one the round routes on,
	// and a turn that answers while naming an open part must not overwrite it with "".
	if u := ParseUnresolved(reply.Content); u != "" {
		s.Unresolved = u
	}
	if len(newStates) > 0 {
		// APPEND, never replace: a later patch is a further finding, and the round folds every
		// branch this session wrote (see RunSlotResearchPass).
		s.NewStates = append(s.NewStates, newStates...)
		// Writing a note resets the "shown since your last note" counter: the fact it reports is about
		// how much evidence has arrived since the model last spoke, so the model's own writing is what
		// moves it (see sessionRecord).
		s.notesAtEvidence = len(s.RetrievedEvidenceIDs)
	}
	if foundAnswer != nil {
		s.FoundAnswer = foundAnswer
		s.Done = true
		// Whether the model used the open-parts protocol is otherwise invisible until the routing
		// line says a round was reopened for it: log the statement itself.
		if u := strings.TrimSpace(s.Unresolved); u != "" {
			_LOG.Printf("[Action Session] answered with an open part (%s)", TruncateRunes(u, 200))
		}
		if len(newStates) > 0 {
			// Both blocks in one reply is the finalize shape, so it is worth a line: the answer
			// ends the session AND the patches beside it are kept (see parseTerminal).
			_LOG.Printf("[Action Session] answered with %d branch(es) recorded from the same reply (turn %d/%d).",
				len(newStates), s.Attempts, s.turnRunCap())
		}
		return nil
	}
	if len(newStates) > 0 {
		_LOG.Printf("[Action Session] %d branch(es) checkpointed (turn %d/%d); the session keeps working until it can answer.",
			len(newStates), s.Attempts, s.turnRunCap())
		return nil
	}
	// Neither a call nor a terminal block: nudge once per turn.
	s.Messages = append(s.Messages, *schema.UserMessage("Call the retrieve tool, or output a <state> patch, or emit <answer>."))
	return nil
}

// toolNode: execute pending tool calls, append tool
// responses, and apply the per-outcome policies.
func (s *sessionState) toolNode(ctx context.Context) error {
	if s.Tools == nil {
		return nil
	}
	// ranAny: whether this node executed at least one call, i.e. whether a tool
	// result exists to annotate with the turn's record line (appendRecordLine).
	ranAny := len(s.PendingCalls) > 0
	evidenceIDs := append([]string(nil), s.RetrievedEvidenceIDs...)
	budgetChars := s.CtxBudget
	if budgetChars <= 0 {
		budgetChars = ctxBudgetFor(ResolveMode(s.Tools))
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
	// it only runs once a rung leaves the chain in modeLLM, and every rule in
	// navRules is modeAuto, so the chain always runs to completion in the prefix
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
		if retrievalTools[c.Name] && q != "" && isNearDup(q, seenQueries) {
			skipped++
			_LOG.Printf("[Action Session] skipping near-duplicate retrieval %q (already searched)", TruncateRunes(q, 80))
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
				c.Name, strings.Join(toolMapNames(), ", "))
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
			// Each tool call is bounded by the session clock MINUS the answer's reserve: a slow
			// backend must not consume the clock the answer turn exists for. The cut sessions of one
			// FRAMES run happened exactly here — a tool result logged, then "session cut: context
			// deadline exceeded" (measured 2026-09-20, 20:39).
			toolCtx, cancelTool := context.WithTimeout(ctx, toolWallS(s.DeadlineLeft))
			oc = executeTool(toolCtx, s.Tools, c.Name, c.Args)
			cancelTool()
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
		case oc.Status == StatusEmpty && oc.Reason == reasonNoStructure:
			// Dataset-level dead end. Strike it; disable once the strikes pile up
			// so one unlucky scope cannot kill the tool.
			n := strikes[c.Name] + 1
			strikes[c.Name] = n
			if n >= emptyStrikes {
				s.Tools.DisableTool(c.Name)
				_LOG.Printf("[Action Session] %s disabled after %d dataset-level empty results", c.Name, n)
			}
		case oc.Status == statusRedundant:
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
		//
		// The passages are numbered BEFORE they are serialized: the model can only cite what it
		// was shown, so a citation it writes has to be checkable against a number it saw.
		s.stampEvidenceRefs(chunks)
		markReadState(s.KB, chunks)
		payload := marshalPassages(chunks)
		if oc.Note != "" {
			// The note carries the tool's own continuation data (list_chunks' hasMore / offset,
			// a nav hint). It rides at the FRONT: the cut below removes the TAIL, and a
			// continuation hint that got cut is a session that concludes the corpus is short
			// instead of asking for the next page (measured 2026-09-22, c1 bowling question).
			payload = oc.Note + "\n" + payload
			_LOG.Printf("[Action Session] result note (%s): %s", c.Name, TruncateRunes(oc.Note, 280))
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
			// The cut is ANNOUNCED. A silent cut is what makes a session report "the corpus
			// does not say" about a passage it was never shown (measured 2026-09-22: an
			// 8275-code-point standings table reached the model as its first ~800, and the
			// answer then said the table "only displayed the top four finishers").
			cut := utf8.RuneCountInString(payload) - keep
			if cut < 0 {
				cut = 0
			}
			payload = TruncateRunes(payload, keep) + fmt.Sprintf(
				"\n[… TRUNCATED: %d more code point(s) of this tool result were NOT shown. The tool "+
					"returned MORE than you see — ask it for the next page with its own argument "+
					"(list_chunks takes doc_id + offset), or narrow the query. Do not conclude the "+
					"corpus lacks what was cut.]", cut)
		}
		used += utf8.RuneCountInString(payload)
		s.Messages = appendMessages(s.Messages, *schema.ToolMessage(payload, c.ID))

		// Ladder continuation: when the model's rung came back weak, keep advancing the
		// ladder IN CODE so one
		// weak step cascades through the remaining (cheaper, wider) rungs instead
		// of leaving the model to rediscover the fallback one turn at a time.
		//
		// Unreachable with today's navRules (every rung is modeAuto, so the chain
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
					ladderNav := &navContext{
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
					ex := runNavChain(ctx, s.Tools, ladderNav, nxt, ladderBudget, "ladder",
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
	// Everything the model was shown EARLIER is folded: only the turn just produced stays
	// verbatim (see compactEarlierToolResults).
	s.compactEarlierToolResults()
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
// compactEarlierToolResults folds the tool results of EARLIER turns into one line each, keeping
// the turn just produced verbatim.
//
// Why: a tool result is appended to the conversation and re-sent on EVERY later turn, so its size
// is paid once per turn rather than once per call. A round that reads ~48k characters over eight
// turns re-sends them about eight times — measured 2026-09-20: 10,033 prompt tokens per turn on
// average, 18,205 on the question that read the most, and the session's own turns were 37% of the
// run's wall clock.
//
// The paper bounds this at the source (a search cache with a five-result window, plus a spill file
// for oversized results). This does the equivalent to our conversation: what the model was JUST
// shown stays verbatim, and everything older becomes a line stating what was read, how much of it,
// and that the words are unchanged in the pool.
//
// Nothing the run depends on is lost: the passages stay in the pool, the evidence registry keeps
// their [ID:n] numbers (so an answer written later still cites what it read), and a model that
// wants to re-read something asks again — which is allowed whenever the query is not a paraphrase
// of an earlier one (see nearDupJaccard).
func (s *sessionState) compactEarlierToolResults() {
	// Keep the last `verbatimSessionTurns` turns verbatim: they are what the model is reasoning over.
	//
	// One was too few. A multi-hop question holds the hop it just read while it searches the next
	// one, and folding hop-1 the moment hop-2 starts is how an answer comes back "the evidence does
	// not contain it" with the evidence two turns behind it (measured 2026-09-20: 8 of 20 answers,
	// every one of them a chain). Three turns is the depth those chains run at; what a long
	// enumeration accumulates beyond that is what the model has already written into its checkpoint.
	keepFrom := 0
	seen := 0
	for i := len(s.Messages) - 1; i >= 0; i-- {
		if s.Messages[i].Role == schema.Assistant {
			seen++
			if seen >= verbatimSessionTurns {
				keepFrom = i
				break
			}
		}
	}
	folded := 0
	for i := 0; i < keepFrom; i++ {
		m := &s.Messages[i]
		if m.Role != schema.Tool || len(m.Content) < foldedToolResultChars {
			continue
		}
		m.Content = foldedToolResult(m.Content)
		folded++
	}
	if folded > 0 {
		_LOG.Printf("[Action Session] folded %d earlier tool result(s) into digests", folded)
	}
}

// foldedToolResult is the digest an earlier turn's tool result becomes: what was read, how much of
// it, and the two facts a model needs to decide whether it must go back for the words.
func foldedToolResult(content string) string {
	n := strings.Count(content, `"ref":`)
	if n == 0 {
		n = strings.Count(content, `"chunk_id"`)
	}
	if n == 0 {
		return `{"folded":true,"note":"This result was read earlier in the session and has been folded away; ask again if you need it."}`
	}
	return fmt.Sprintf(
		`{"folded":true,"passages":%d,"note":"These %d passage(s) were read earlier in this session. Their words are unchanged in the evidence pool and their [ID:n] numbers still resolve, so you can cite them — ask for them again only if you need the exact text."}`,
		n, n)
}

// stampEvidenceRefs numbers the passages of ONE tool result and records them in the session's
// registry.
//
// The number is stamped INTO the passage the model reads ("ref": n). That is the whole point:
// the model can only cite what it was shown, so a citation has to be checkable against a number
// it actually saw — a number the runtime computes later, in a renderer the session never sees
// (which is how the final-answer call used to work), cannot be written down by the model at all.
//
// A chunk already in the registry KEEPS its number: the same passage reached twice is one
// place, cited the same way, and the registry stays stable across turns.
func (s *sessionState) stampEvidenceRefs(chunks []any) {
	if s.evidenceRefOf == nil {
		s.evidenceRefOf = map[string]int{}
	}
	for _, raw := range chunks {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := payloadChunkID(m)
		if id == "" {
			continue
		}
		n, seen := s.evidenceRefOf[id]
		if !seen {
			n = len(s.EvidenceRefs)
			s.evidenceRefOf[id] = n
			s.EvidenceRefs = append(s.EvidenceRefs, id)
		}
		m["ref"] = n
	}
}

// seedEvidenceRefs registers passages the SEED showed the model — the opening's ranked previews, the
// nav prefix's tool payloads — in the run's citation registry, in the order they were shown, and
// returns one line per passage carrying the handle the model can cite.
//
// The registry is what the answer's [ID:n] markers resolve against, and until this existed ONLY the
// tool loop wrote to it (see stampEvidenceRefs). The opening's previews and the ladder's payloads
// were shown with a raw chunk id or with nothing at all, so the material an answer was WRITTEN FROM
// was absent from the registry the answer was resolved against. Measured 2026-09-20 (三国/关羽): the
// ladder's own retrieve brought the ten passages holding every name, the session answered from them
// on its first turn, and the final answer — sixteen members, all correct — shipped with 0 passages in
// the citation registry, so not one member could be cited.
//
// The numbers come from the POOL (see Kbinfos.PublishEvidence), so this session's numbering continues
// the run's rather than restarting: a marker written in round 1 keeps pointing at the passage it was
// written for after round 2 has shown its own.
//
// Each line shows as much of its passage as the PREFIX stage allows (see deliverItemText): enough
// to tell the passages apart and to see which part of each document was read, not a second copy of
// the payload the ladder already put in the history.
func (s *sessionState) seedEvidenceRefs(ids []string) []string {
	if s == nil || s.KB == nil || len(ids) == 0 {
		return nil
	}
	var lines []string
	for _, id := range ids {
		n, c, ok := s.publishEvidenceID(id)
		if !ok {
			continue
		}
		text := FlattenLine(deliverItemText(stagePrefix, ChunkTextOf(c), ""))
		lines = append(lines, fmt.Sprintf("[ID:%d] %s", n, text))
	}
	return lines
}

// publishEvidenceID publishes one passage and mirrors the number into the session's own registry,
// which is what makes the model's [ID:n] and the run's list the same numbering.
//
// An id the pool holds no chunk for is refused: navigate_tree reports DOC ids, and the client opens
// chunk ids, so an id with no chunk behind it must not take a number.
func (s *sessionState) publishEvidenceID(id string) (int, map[string]any, bool) {
	id = strings.TrimSpace(id)
	if s == nil || id == "" || s.KB == nil {
		return 0, nil, false
	}
	c := s.KB.ChunkByID(id)
	if c == nil {
		return 0, nil, false
	}
	nums := s.KB.PublishEvidence([]string{id})
	if len(nums) == 0 {
		return 0, nil, false
	}
	n := nums[0]
	if s.evidenceRefOf == nil {
		s.evidenceRefOf = map[string]int{}
	}
	if _, seen := s.evidenceRefOf[id]; !seen {
		s.evidenceRefOf[id] = n
		s.EvidenceRefs = append(s.EvidenceRefs, id)
	}
	return n, c, true
}

// loadEvidenceRefs starts this session's registry from the run's, so its numbering continues where the
// last round stopped instead of restarting at zero (see Kbinfos.PublishEvidence).
func (s *sessionState) loadEvidenceRefs(kb *Kbinfos) {
	if s == nil || kb == nil || len(kb.SessionEvidenceRefs) == 0 {
		return
	}
	if s.evidenceRefOf == nil {
		s.evidenceRefOf = map[string]int{}
	}
	for i, id := range kb.SessionEvidenceRefs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, seen := s.evidenceRefOf[id]; seen {
			continue
		}
		s.evidenceRefOf[id] = i
		s.EvidenceRefs = append(s.EvidenceRefs, id)
	}
}

// payloadChunkID is the chunk identity inside a tool payload entry. The search tools name it
// "chunk_id"; list_chunks' passages are {"id","content"} (see listChunks), so both are read.
func payloadChunkID(m map[string]any) string {
	for _, key := range []string{"chunk_id", "id"} {
		if v, ok := m[key].(string); ok {
			if v = strings.TrimSpace(v); v != "" {
				return v
			}
		}
	}
	return ""
}

// markReadState labels every passage of ONE tool result with what the run has done with it: "read"
// when list_chunks delivered the chunk, "preview" when it came back from a search.
//
// The two arrive in the same shape, and only the run knows which is which — a search result is a
// ranked guess about WHERE the answer might be, a document page is what the document SAYS. The
// label is attached here, where the result is rendered, because it is a fact about the run rather
// than about this call: a chunk previewed earlier and read later is "read" in every result after
// that. The seed reports the same distinction per document (see Kbinfos.ReadProgress).
func markReadState(kb *Kbinfos, chunks []any) {
	for _, c := range chunks {
		m, ok := c.(map[string]any)
		if !ok {
			continue
		}
		state := "preview"
		if kb.WasRead(payloadChunkID(m)) {
			state = "read"
		}
		m["seen"] = state
	}
}

func (s *sessionState) appendBatchProtocol(ranAny bool) {
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
func (s *sessionState) appendRecordLine(ranAny bool) {
	if !ranAny || len(s.Messages) == 0 {
		return
	}
	rec := s.sessionRecord()
	s.Record = rec
	// EVERY session gets the line. It used to be gated on enumeration, on the argument that on a
	// single-value question every field of it is empty or meaningless — but the line also carries
	// `asked-nothing-back` (the probes this session already ran and got nothing for) and
	// `FOUND BUT NOT RECORDED` (names a passage offered that no patch accounts for), which is
	// exactly the feedback a multi-hop VALUE session needs to take its next hop instead of
	// re-asking what it already knows. Those sessions were also the ones stopped at four turns
	// (see actionMaxTurns), so they were the sessions with the most to lose from the missing line.
	//
	// The pool excerpt below stays gated: it exists to rescue a SET that has stopped growing.
	// The line the model steers by is message content, so the log could not show
	// whether a mechanism fired at all: three rounds of analysis here ended up
	// inferring it from side effects. One truncated line per turn ends that.
	_LOG.Printf("[Action Session] record line: %s", TruncateRunes(rec.Line(), 320))
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
	if s.enumerating() {
		if excerpt := s.unreadPoolExcerpt(); excerpt != "" {
			// Logged as well as delivered: whether the mechanism fired is otherwise
			// only visible inside the message content.
			_LOG.Printf("[Action Session] pool excerpt: %s", TruncateRunes(excerpt, 240))
			last.Content += "\n" + excerpt
		}
	}
	// What the model did NOT write down is not the runtime's to reconstruct: the one mechanical signal
	// that says "you have read more than you have written down" is the count in the line above
	// (ShownSinceNote). Whether that means anything is the model's judgement, made on its own record
	// (see sessionRecord).
}

// turnRunCap is the hard ceiling on a session's turns: the mode's floor plus the
// turns the model may add on its own decision (see offerContinuation).
func (s *sessionState) turnRunCap() int { return s.actionMaxTurns() + turnRunExtra }

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
// The offer used to be gated on this session ENUMERATING (a caller-written batch of terms),
// on the argument that a question which is not assembling a set has nothing for those turns to
// find. Both halves of that gate were wrong for the questions that come back empty: a multi-hop
// VALUE question has hops to find, and its session is exactly the one stopped at the reduced
// turn floor with its clock unspent (see actionMaxTurns). The runtime keeps the two hard bounds
// it does not delegate — the run cap and the session clock — and the ask hands the model the
// record's own brief, so "continue" has to be justified by something the record shows is missing.
func (s *sessionState) offerContinuation() bool {
	if s.Attempts >= s.turnRunCap() || s.DeadlineLeft <= turnAskFloorS {
		return false
	}
	if s.ContinuationAsked == s.Attempts {
		return true
	}
	s.ContinuationAsked = s.Attempts
	ask := continuationAsk(s.Attempts, s.turnRunCap(), s.Record.Brief(), s.Record.Verbose())
	s.Messages = appendMessages(s.Messages, *schema.UserMessage(ask))
	_LOG.Printf("[Action Session] turn %d/%d — the floor is spent; offered the model one more turn while the record says something is missing (%s, %.0fs left).\noffer=%q",
		s.Attempts, s.turnRunCap(), s.Record.Brief(), s.DeadlineLeft, TruncateRunes(ask, 700))
	return true
}

// wroteBatch reports whether the CALLER wrote a batch of terms in one query — the
// signal that it is enumerating rather than asking a question.
//
// The gate is the caller's own writing, not a guess about the question, because the guess
// is wrong: a slot TYPE cannot tell "how many people did X kill" from "how many times larger
// is A than B", and a single phrase's commas make it look like two members.
func (s *sessionState) wroteBatch() bool {
	for _, q := range s.SearchQueries {
		if callerBatch(q) {
			return true
		}
	}
	return false
}

// enumerating reports whether this session is assembling a SET. The tell is the one with no
// observed false positives: the CALLER wrote a batch of terms in one query (see wroteBatch).
//
// The other half used to be the direction's own table — a count/list-typed slot the planner
// had written — and it is gone with the coverage engine: a slot type cannot tell an
// enumeration from a count of events, and being wrong about it moved the session's turn floor
// for a question that had no members to assemble.
func (s *sessionState) enumerating() bool {
	return s.wroteBatch()
}

// The three shape-gated helpers that used to live here are gone with the coverage engine:
// SessionWallS (a per-shape session clock), setMethodFor (the SET method, gated on the
// planner's typed slots) and enumerationSeed (the method plus the windows the runtime's own
// enumeration had found).
//
// What replaces them is smaller and makes no claim about the QUESTION: ONE session clock
// (actionTimeoutS, or the budget the caller passes), and the SET method delivered by
// appendBatchProtocol on the first batch the CALLER writes. Both halves of the old gate were
// decisions taken before the session had done anything, and a wrong decision about a value
// question could only add cost.
// continuationAsk is the offer the model decides on: it names the hard bound, the
// remaining turns, and the ONLY grounds on which another turn is granted — what
// the record line shows is still missing. The record itself was appended to the
// model's last tool result, so the decision is made on facts, not on appetite.
func continuationAsk(taken, cap int, record, notes string) string {
	ask := fmt.Sprintf(
		"TURN BUDGET: %d turn(s) taken, up to %d available. DECIDE NOW — your next reply decides it:\n"+
			"- If the [record] line still shows something MISSING — a name you proved reachable and never recorded, a slot with no candidate, an angle you have not tried — call ONE more tool aimed at exactly that. %d turn(s) left.\n"+
			"- Otherwise emit the state patch NOW with everything you have.\n"+
			"An extra turn is only for something the record line shows is missing; re-running a search you already ran is not.\n"+
			"Current record: %s",
		taken, cap, cap-taken, record)
	if notes = strings.TrimSpace(notes); notes != "" {
		// The model's OWN notes ride the DECISION, verbatim. The runtime neither summarizes them nor
		// adds a list of its own: "what is still missing" is read off what the model wrote, which is
		// the only place that fact exists (see sessionRecord).
		ask += "\n" + notes
	}
	return ask
}

// finalizeNode: tool budget spent — ONE last call
// WITHOUT tools, demanding the terminal JSON to salvage whatever was learned.
func (s *sessionState) finalizeNode(ctx context.Context) error {
	// The last turn ASKS FOR THE ANSWER.
	//
	// It used to ask for a state patch only ("output now: <state>{...}"), which is why the session
	// never answered: the protocol it was given had no <answer> in it, so the round's findings
	// always had to be re-composed by a separate call that had never read the passages (measured
	// 2026-09-20: 三国's record held twelve members with their quoted lines, and the answer carried
	// citations for a third of them because the composer could only cite the blocks it rendered).
	//
	// The paper's loop ends when the AGENT answers, so the answer is what this turn asks for. The
	// patch stays available — a session that resolved slots must still record them — but it is the
	// secondary output, and the answer is the one that reaches the user.
	budgetPrompt := ("TOOL BUDGET EXHAUSTED. Based ONLY on the passages you have read, write your final answer now.\n" +
		"Format: <state>{...}</state> (only if you have slot findings to record) followed by " +
		"<answer>your answer</answer>.\n" +
		"The answer is what the user receives: answer the question that was asked, in prose, and cite " +
		"the passage behind EVERY fact with its [ID:n]. Do not include an [ID:n] you were not shown. " +
		"If the question counts members of a set (how many named people X killed), the answer LISTS " +
		"them — one per line with the passage behind it — and gives the total last; the number alone " +
		"is not an answer. " +
		"If part of the question cannot be supported by a passage you read, say which part and what " +
		"you could not find — do not leave it out and do not guess.\n" +
		"Also write <unresolved>…</unresolved> in the same reply, naming each part you could not " +
		"establish (a few words each, specific enough to probe); use <unresolved></unresolved> only " +
		"when nothing is left open. That block decides whether the run stops here.\n" +
		"For the state block: <state>{\"new_states\": [{\"state\": [{\"id\": <slot_id>, \"kind\": \"members\", " +
		"\"items\": [{\"name\": \"<name>\", \"chunk_id\": \"<its passage>\"}], " +
		"\"candidate_strength\": <0..1>}]}]}</state>; use \"kind\": \"count\" with \"count\": <n> for a number, " +
		"and \"kind\": \"text\" with \"candidate\": \"<value>\" for one value that is neither a member list nor a " +
		"number; if NOTHING was learned use <state>{\"new_states\": []}</state>.\n" +
		"An answer is REQUIRED even when the state block is empty.")

	// The patch written HERE is the one that lands in the record, so the record is
	// part of the order.
	//
	// A session can probe a name, get its passages back, and never mention it in the terminal
	// patch — the run's own evidence, found and paid for, dropped at the last step. The record
	// is the only place that fact exists, and this is the last call that can act on it, so the
	// salvage order names the names it has to account for instead of asking for "whatever you
	// have".
	if rec := s.sessionRecord(); rec.Pool > 0 {
		budgetPrompt += "\n\n" + rec.Line()
		// The notes the model wrote are rendered WHOLE here, verbatim: this is the last call that can
		// answer from them, and the runtime adds no list of its own — what the run holds is what the
		// model wrote down, plus the passages it was shown (see sessionRecord).
		if notes := rec.Verbose(); notes != "" {
			budgetPrompt += "\n" + notes
		}
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
	// The answer call leaves finalizeMarginS unspent: it used to be handed the whole remaining
	// clock, so a call that used it returned exactly when the context fired and the reply was lost
	// (see sessionClockGuardS).
	tmo = min(max(tmo-finalizeMarginS, minFinalizeTimeoutS), finalizeTimeoutS)

	callCtx, cancel := context.WithTimeout(ctx, time.Duration(tmo*float64(time.Second)))
	defer cancel()

	reply, err := s.Model.Complete(callCtx, msgs, nil)
	if err != nil {
		// ONE retry, because a transient provider stall is not a reason to lose an answer the session
		// already HAS. Measured 2026-09-20 (三国/关羽): `salvage call failed: … Post
		// "https://api.minimaxi.com/v1/text/chat…": failed to send request` cost the run the
		// sixteen-member answer whose evidence the session had already read, and the fallback
		// composition answered from the few passages its own budget admitted (six members). The
		// init call retries once for the same reason (see initRetryTimeout).
		_LOG.Printf("[Action Session] salvage call failed: %v — retrying once.", err)
		s.refreshClock()
		if left := s.DeadlineLeft - finalizeMarginS; left > minSalvageRetryS {
			retryCtx, cancelRetry := context.WithTimeout(ctx,
				time.Duration(min(left, tmo)*float64(time.Second)))
			reply, err = s.Model.Complete(retryCtx, msgs, nil)
			cancelRetry()
		}
	}
	if err != nil {
		// A failed salvage call is logged and the node CONTINUES to the deterministic
		// loose-clue harvest below. Returning early here drops the last-narration
		// breadcrumb exactly when the salvage model is unavailable, which is the case the
		// harvest exists for.
		_LOG.Printf("[Action Session] salvage call failed twice: %v", err)
	} else {
		newStates, foundAnswer, terminalType, payload := parseTerminal(reply.Content, s.ParentState)
		// APPEND, like every other checkpoint: the answer turn is the last word, not the only
		// word. Assigning here wiped the record the session had written turn by turn whenever this
		// reply carried an empty patch — which is exactly what the order above invites ("if NOTHING
		// was learned use <state>{"new_states": []}</state>"), and how a round that had recorded
		// slots arrived at the round with none (measured 2026-09-20: the graph test
		// TestRunAgenticSeedsSlotsAndRunsSession lost its patched slot this way, and the FRAMES runs
		// showed rounds reporting unresolved=0 for questions whose sessions had filled slots).
		if len(newStates) > 0 {
			s.NewStates = append(s.NewStates, newStates...)
			// Writing a note resets the "shown since your last note" counter: the fact it reports is about
			// how much evidence has arrived since the model last spoke, so the model's own writing is what
			// moves it (see sessionRecord).
			s.notesAtEvidence = len(s.RetrievedEvidenceIDs)
		}
		s.FoundAnswer = foundAnswer
		s.TerminalType = terminalType
		s.TerminalPayload = payload
		if u := ParseUnresolved(reply.Content); u != "" {
			s.Unresolved = u
		}
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
				// discovered_clues must be []any: applyPatch type-switches on the
				// JSON-decoded shape, so a []string is silently ignored and the whole patch
				// no-ops.
				if patched := applyPatch(s.ParentState, []map[string]any{
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
// actionMaxTurns is this session's turn floor: the mode's own count, for EVERY session.
//
// It used to be halved for a session that was not "enumerating" (a caller-written batch of terms
// in one query), on the argument that a question which is not assembling a set has nothing to
// spend those turns on. The measurement says the opposite: the questions that come back empty are
// multi-hop VALUE questions, and their sessions were stopped by their TURN count while their clock
// was still full — the log of a stopped session reads "167 seconds of research budget left" in the
// same breath as "TOOL BUDGET EXHAUSTED" (2026-09-20, FRAMES: "turn floor 8 → 4" 15 times in one
// 20-question run, 8 rounds finalized by the salvage prompt, ~150s of the 180s budget unspent; the
// four questions that stayed at zero — Quincy's mayors, Gifu's population, AP's law, Lahore in
// 1858 — each hit that wall).
//
// The constraint that should bind is the CLOCK (the paper's loop has no turn cap at all), so the
// mode's count is returned as it stands and the run cap (turnRunCap) remains the only hard turn
// ceiling. The enumeration-specific extras — the SET method appended on the caller's first batch,
// the pool excerpt for a set that stopped growing (see appendBatchProtocol, appendRecordLine) —
// stay gated on the shape, because those are about enumeration, not about how long a session may
// work.
func (s *sessionState) actionMaxTurns() int {
	floor := ResolveMode(s.Tools).ActionMaxTurns
	if floor <= 0 {
		floor = valueTurnFloor
	}
	return floor
}

// route
func (s *sessionState) route() routeTarget {
	if s.Done {
		return routeEnd
	}
	s.refreshClock()
	// The answer turn owns the last slice of the clock — the paper's "submit-now" applied to a
	// session: at a fraction of the budget the retrieval stops and the agent answers in its own
	// words.
	//
	// This check comes BEFORE the pending tool calls, and that order is the whole point. Pending
	// calls used to run first ("so the provider never sees a dangling tool_calls"), and the answer
	// turn strips them anyway (see finalizeNode's stripUnpairedToolCalls). Running them cost the
	// session its answer: measured 2026-09-20 (三国), the last reply declared three probes, the
	// tool node ran them into the wall, and the round ended with
	// "session cut … returning the 44 passage(s) and 0 patch(es)" — the answer turn never ran, so
	// the count and every citation were lost. At this boundary the answer is worth more than one
	// more search, which is exactly what the paper does at 70% of T.
	//
	// ForceAnswer is the same destination reached the other way: a turn that failed, timed out or
	// got stuck has no more searching to do either, and it keeps the record it has.
	if s.ForceAnswer {
		_LOG.Printf("[Action Session] no more searching this session; asking for the answer with the record it has.")
		return routeFinalize
	}
	if s.DeadlineLeft <= answerReserveS {
		_LOG.Printf("[Action Session] %.0fs left: stopping the search so the answer has its own clock", s.DeadlineLeft)
		return routeFinalize
	}
	// Run pending tool_calls now that the answer's clock is safe: leaving an assistant.tool_calls
	// message without its tool response makes the provider reject the next call, so the tool node
	// clears PendingCalls and the route re-checks the clock.
	if len(s.PendingCalls) > 0 {
		return routeTool
	}
	if s.Attempts >= s.effectiveTurnFloor() {
		// The mode's turn count is a FLOOR, capped by what the clock can afford at this session's
		// own pace (see affordableSearchTurns): past it the MODEL decides whether the session takes
		// another turn — but only one that still leaves the answer its clock.
		if s.canAffordAnotherTurn() && s.offerContinuation() {
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
func (s *sessionState) routeAfterTool() routeTarget {
	s.refreshClock()
	// Same reserve as route(): the answer turn needs the clock more than another probe does.
	if s.ForceAnswer {
		return routeFinalize
	}
	if s.DeadlineLeft <= answerReserveS {
		_LOG.Printf("[Action Session] %.0fs left: stopping the search so the answer has its own clock", s.DeadlineLeft)
		return routeFinalize
	}
	if s.Attempts >= s.effectiveTurnFloor() {
		// Same rule as route: the tool result that just arrived is what the continuation offer
		// is about, so the model decides with it in hand — otherwise the last probe's result is
		// never read by a turn. The offer is still bounded by the clock (see canAffordAnotherTurn).
		if s.canAffordAnotherTurn() && s.offerContinuation() {
			return routeRunAction
		}
		return routeFinalize
	}
	return routeRunAction
}

// Session-node adapters: unlike method values (s.runActionNode) these take the
// state from the graph payload instead of capturing an instance, which is what lets ONE
// compiled graph serve every session: the session's tools/model arrive in the state
// (sessionState.Tools / .Model) rather than in a closure.
func runActionNodeFn(ctx context.Context, st *sessionState) (*sessionState, error) {
	return st, st.runActionNode(ctx)
}

func toolNodeFn(ctx context.Context, st *sessionState) (*sessionState, error) {
	return st, st.toolNode(ctx)
}

func finalizeNodeFn(ctx context.Context, st *sessionState) (*sessionState, error) {
	return st, st.finalizeNode(ctx)
}

var (
	sessionGraphOnce sync.Once
	sessionGraphRun  compose.Runnable[*sessionState, *sessionState]
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
//  1. Nodes touch only their own *sessionState; no package-level mutable state inside
//     a node.
//  2. No checkpoint store is configured (compose.WithCheckPointStore) — the
//     checkPointer lives on the shared runner. Guarded by
//     TestSessionGraphHasNoCheckpointStore.
//  3. Eino v0.9.14's runner keeps no per-run state: it holds only the static topology
//     and every Invoke builds its own channels and task manager
//     (compose/graph_run.go:129-130, :933, :948). Re-check on upgrade.
//
// See TestSessionGraphIsSharedAcrossConcurrentSessions.
func sessionGraph() (compose.Runnable[*sessionState, *sessionState], error) {
	sessionGraphOnce.Do(func() {
		g := compose.NewGraph[*sessionState, *sessionState]()

		var buildErr error
		addNode := func(name string, fn func(context.Context, *sessionState) (*sessionState, error)) {
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
		addBranch := func(from string, ends map[string]bool, cond func(*sessionState) routeTarget) {
			if buildErr != nil {
				return
			}
			buildErr = g.AddBranch(from, compose.NewGraphBranch(func(_ context.Context, st *sessionState) (string, error) {
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
		}, (*sessionState).route)
		addBranch("tool", map[string]bool{"run_action": true, "finalize": true}, (*sessionState).routeAfterTool)

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
func (s *sessionState) sessionLoop(ctx context.Context) error {
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

// parseTerminal: parse the two terminal blocks
// (<state> patches → new-state branches; <answer> → final answer).
//
// An ANSWER wins, and BOTH blocks are read.
//
// The order used to be "state first, then return", which discarded an answer written in the
// same reply — and the finalized turn asks for exactly that shape ("<state>{...}</state>
// followed by <answer>your answer</answer>", see finalizeNode). A session that obeyed the
// protocol was therefore recorded as never having answered, and its findings had to be
// re-composed by a call that had not read the passages (measured 2026-09-20: 7 of 21 rounds
// reported collected_answer=false, 6 of them through the salvage call, which uses this parser
// too — the log line was "1 branch(es) salvaged from exhausted session").
//
// A state block is bookkeeping either way, so it is still applied whenever it is present; it
// simply cannot suppress the answer beside it.
//
// Returns (newStates, foundAnswer, terminalType, payload): newStates carries the patches from
// both blocks, foundAnswer is non-nil only when a NON-EMPTY answer was written, terminalType
// names the block that decided the outcome, and payload is that block's parsed data.
func parseTerminal(content string, parent State) ([]State, *string, *string, map[string]any) {
	stateBranches, statePayload := parseStateBlock(content, parent)
	answerPresent := strings.Contains(content, "<answer>")
	found, answerBranches, answerPayload := parseAnswerBlock(content, parent)

	branches := append(stateBranches, answerBranches...)
	if len(branches) == 0 {
		// Every caller distinguishes "nothing was written" from "something was", so an empty
		// list is reported as nil and the fall-through below decides the rest.
		branches = nil
	}

	if found != nil {
		if answerPayload == nil {
			answerPayload = map[string]any{}
		}
		tt := "answer"
		return branches, found, &tt, answerPayload
	}
	// An <answer> block with an empty payload is still an ANSWER terminal (the answer itself is
	// not a found answer — see parseAnswerBlock — so the session keeps working, but the round
	// must not be reported as a state patch).
	if answerPresent {
		if answerPayload == nil {
			answerPayload = map[string]any{}
		}
		tt := "answer"
		return branches, nil, &tt, answerPayload
	}
	if statePayload != nil {
		tt := "state"
		return branches, nil, &tt, statePayload
	}
	return nil, nil, nil, nil
}

// parseStateBlock reads the <state> block, if any, into a list of new-state branches. A nil
// payload means "no <state> block in this reply".
func parseStateBlock(content string, parent State) ([]State, map[string]any) {
	if !strings.Contains(content, "<state>") {
		return nil, nil
	}
	block := extractTag(content, "state")
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
		if ns := applyPatch(parent, patches); ns != nil {
			branches = append(branches, *ns)
		}
	}
	return branches, data
}

// parseAnswerBlock reads the <answer> block, if any, into the answer plus the state patches
// that block may carry.
//
// The block is JSON when the model wrote a payload (that shape may also carry a state
// patch), and PLAIN TEXT when it simply answered.
//
// Accepting only the JSON shape is what kept the session protocol patch-only: an answer
// written as prose — "<answer>关羽杀了十二人 [ID:0]</answer>", which is exactly what the
// turn now asks for — parsed to FoundAnswer="" (an empty answer is not a found answer, by
// the rule below), so the round reported "no answer" with the answer sitting in the reply
// (measured 2026-09-20: every session of the run, including the ones whose record held
// twelve members and their quoted lines).
func parseAnswerBlock(content string, parent State) (*string, []State, map[string]any) {
	if !strings.Contains(content, "<answer>") {
		return nil, nil, nil
	}
	block := extractTag(content, "answer")
	data, _ := ExtractJSON(block).(map[string]any)
	ans := ""
	if data != nil {
		if raw, ok := data["answer"]; ok && raw != nil {
			ans = strings.TrimSpace(fmt.Sprint(raw))
		}
	}
	if ans == "" && data == nil {
		ans = strings.TrimSpace(block)
	}
	// An empty/whitespace answer is NOT a found answer. Returning a non-nil empty
	// string here would terminate the session with FoundAnswer="" — the slot research
	// pass then reported
	// collected_answer=false for a session that never answered, and the
	// model never got the chance to re-emit a proper patch/answer.
	var found *string
	if ans != "" {
		found = &ans
	}
	if data == nil {
		data = map[string]any{}
	}
	patches := toPatchList(data["new_state"])
	var branches []State
	if ns := applyPatch(parent, patches); ns != nil {
		branches = append(branches, *ns)
	}
	return found, branches, data
}

// ParseUnresolved reads the session's <unresolved> block: what it could NOT establish, in its own
// words. "" means it did not write one (or wrote an empty one).
//
// The block exists because "did the session answer?" and "is the answer finished?" are two
// different facts, and the loop only had the first. A multi-hop question's first answer is very
// often "I read X, and X does not carry Y" — an answer, and simultaneously a statement of work
// remaining — so a round whose session wrote that was treated exactly like a round whose session
// had finished, and the run closed with the hop unsearched (measured 2026-09-20: four questions
// stayed at zero this way — Quincy's mayors, Gifu's population, AP's law, Lahore in 1858 — while
// their sessions each held the bridge and said so, and the round's clock had ~150s of its 180s
// unspent; see routeResearch).
//
// It is parsed as its own block rather than read out of the answer text because prose cannot be
// tested for meaning: the model states the open part once, in a shape the router can rely on, and
// the same words stay in the answer for the reader.
func ParseUnresolved(content string) string {
	if !strings.Contains(content, "<unresolved>") {
		return ""
	}
	return strings.TrimSpace(extractTag(content, "unresolved"))
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
	// modeAuto means the orchestrator runs this step itself (no model
	// round-trip).
	modeAuto = "auto"
	// modeLLM means the step needs a decision only the model can make (e.g.
	// WHICH routed document to drill into), so the chain stops here and hands
	// control back.
	modeLLM = "llm"
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

// navRulesEnabled
const navRulesEnabled = true

// navContext is the mutable state threaded through the navigation chain.
//
// KnownDocs is the routed scope (doc_ids). RoutedDocs carries each routed doc's
// OVERALL SUMMARY — the hint that feeds retrieval as a soft boost instead of a
// hard filter. NavHint is the joined summaries used as retrieval keywords so
// routed docs rank up WITHOUT excluding the rest of the corpus.
type navContext struct {
	Direction  string
	KnownDocs  []string
	RoutedDocs [][2]string // (doc_id, summary)
	NavHint    string
}

// navRule is one step of the navigation chain.
type navRule struct {
	ID   string
	Tool string
	// Mode is modeAuto or modeLLM.
	Mode string
	// Run replaces the single-tool path for a step that composes several tools
	// (drill: retrieve + navigate_structure + merge).
	Run func(ctx context.Context, ts *Toolset, nav *navContext, available map[string]bool, budgetS float64) ToolOutcome
	// Args builds the tool arguments from the running context (modeAuto only;
	// also used for display when Run composes internally).
	Args func(nav *navContext) map[string]any
	// When is an optional guard; the step is skipped when it returns false.
	When func(nav *navContext) bool
	// Next maps status -> next rule id. "" (or a missing key) ends the chain.
	// Routing on STATUS is what makes "quality poor -> fall back" a code
	// decision rather than a prompt suggestion.
	Next map[string]string
}

// navRules is the per-slot strategy ladder: "nav is a hint, not a constraint" — no
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
// navQuery is what the LADDER searches with: the direction's question, not the direction block.
//
// Every rung used to hand `nav.Direction` verbatim to its tool, so the retrieval the ladder runs on
// the session's behalf searched the seed's own scaffolding. Measured 2026-09-20 (三国/关羽): one
// prefix leg issued keyword searches for "Clues", "to" and "cover" beside the question — the exact
// tokenization the tool path has guarded against since sanitizeRetrievalQuery exists, unapplied here
// because this call is CODE's and sanitizing was only wired into the model's calls.
//
// The direction's first content line IS the question (see graph_slots.go where it is assembled), so
// the sanitizer's answer is the right probe rather than a heuristic of ours.
func navQuery(nav *navContext) string {
	if nav == nil {
		return ""
	}
	if q := sanitizeRetrievalQuery(nav.Direction); q != "" {
		return q
	}
	return strings.TrimSpace(nav.Direction)
}

var navRules = []navRule{
	{
		ID:   "locate",
		Tool: "navigate_tree",
		Mode: modeAuto,
		Args: func(nav *navContext) map[string]any { return map[string]any{"query": navQuery(nav)} },
		// Tree missed => no routed hints to merge against; the only useful step
		// is an unscoped search. — {OK, MISS, EMPTY, POOR, ERROR};
		// REDUNDANT is deliberately absent: a redundant locate changed nothing,
		// and widening it to global would re-run the same corpus search that
		// produced the duplicates.
		Next: map[string]string{
			StatusOK:    "drill",
			StatusMiss:  "global",
			StatusEmpty: "global",
			statusPoor:  "global",
			StatusError: "global",
		},
	},
	{
		ID:   "drill",
		Tool: "navigate_structure",
		Mode: modeAuto,
		Run:  runDrillMerge,
		Args: func(nav *navContext) map[string]any { return map[string]any{"query": navQuery(nav)} },
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
		Mode: modeAuto,
		Args: func(nav *navContext) map[string]any { return map[string]any{"query": []string{navQuery(nav)}} },
		Next: map[string]string{},
	},
}

var navRuleByID = func() map[string]*navRule {
	m := make(map[string]*navRule, len(navRules))
	for i := range navRules {
		m[navRules[i].ID] = &navRules[i]
	}
	return m
}()

// navStartRule is the ladder's entry point.
const navStartRule = "locate"

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
func runDrillMerge(ctx context.Context, ts *Toolset, nav *navContext, available map[string]bool, budgetS float64) ToolOutcome {
	var merged []any
	var evidenceIDs []string
	seenIDs := map[string]bool{}

	// 1. Skeleton A: whole-corpus retrieval, softly boosted by the nav hints.
	// The hint is an explicit PARAMETER, passed explicitly here instead of being parked on
	// the shared executor (which would make the model's own retrieve calls carry a hint they
	// were never given).
	retOC := executeTool(ctx, ts, "retrieve", map[string]any{
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
			sOC := executeTool(stepCtx, ts, "navigate_structure", map[string]any{
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

// sessionClockFor is the session's OWN clock for a budget: the budget minus the guard, floored so a
// tiny budget still leaves a moment to answer (see sessionClockGuardS).
func sessionClockFor(budgetLeft float64) float64 {
	return max(minSessionClockS, budgetLeft-sessionClockGuardS)
}

// clockLeftS is what is left of the session's own clock, never negative.
func (s *sessionState) clockLeftS() float64 {
	if s == nil || s.SessionDeadline.IsZero() {
		return s.DeadlineLeft
	}
	if left := time.Until(s.SessionDeadline).Seconds(); left > 0 {
		return left
	}
	return 0
}

// refreshClock re-reads the session's clock. Called at the top of every turn and every route: the
// clock is a deadline, and a value read once at the start is not a clock at all.
func (s *sessionState) refreshClock() {
	if s == nil || s.SessionDeadline.IsZero() {
		return
	}
	s.DeadlineLeft = s.clockLeftS()
}

// paceS is the pace the session's own calls actually take: the longest turn it has completed, or
// zero before the first one returns. minExpectedTurnS floors it so a session that has not run a
// turn yet is not planned around a 3-second call.
func (s *sessionState) paceS() float64 {
	if s.expectedTurnS > minExpectedTurnS {
		return s.expectedTurnS
	}
	return minExpectedTurnS
}

// affordableSearchTurns is how many SEARCH turns still fit at this pace, with the answer's own
// reserve out of the clock. One is the floor: a session always gets a turn to work with.
//
// It is the answer to "how many calls does this question's clock actually buy", measured rather
// than assumed — the turn budget used to be the mode's constant (8 on high) whatever the clock or
// the provider's latency, so a session whose calls take 40s ran until the wall and never reached an
// answer turn (measured 2026-09-20, 三国: 3 turns of 36s/74s in a 110s clock).
func (s *sessionState) affordableSearchTurns() int {
	room := s.DeadlineLeft - answerReserveS
	if room <= 0 {
		return 0
	}
	n := int(room / s.paceS())
	if n < 1 {
		n = 1
	}
	return n
}

// effectiveTurnFloor is the mode's turn floor, capped by what the clock can afford: it is the point
// where the session stops SEARCHING and the answer turn takes over.
func (s *sessionState) effectiveTurnFloor() int {
	affordable := s.affordableSearchTurns()
	if floor := s.actionMaxTurns(); affordable < floor {
		if affordable < 1 {
			return 1
		}
		return affordable
	}
	return s.actionMaxTurns()
}

// canAffordAnotherTurn reports whether ONE more turn at this session's pace still leaves the answer
// its share. The answer turn is the one call that must not be raced (see FinaleMinS).
func (s *sessionState) canAffordAnotherTurn() bool {
	return s.DeadlineLeft-s.paceS() > answerReserveS
}

// minExpectedTurnS floors the measured pace: below it a turn is not a turn.
const minExpectedTurnS = 20.0

// lastSearchTurnNotice is what the session is told on the last search turn it can afford.
const lastSearchTurnNotice = "LAST SEARCH TURN: this is the final turn of this session's clock that " +
	"can search — after it, only the answer fits. Use it to CLOSE OUT: write a <state> patch with " +
	"everything you have established (members with the passage behind each), then <answer> with what " +
	"the evidence supports and <unresolved> naming what it does not. Do not start a new line of " +
	"searching you cannot finish."

// toolWallS is how long ONE tool call may take: the session clock minus the answer's reserve, so
// the tools spend the searching budget and never the answering one (see answerReserveS).
func toolWallS(deadlineLeft float64) time.Duration {
	wall := max(5.0, min(75.0, deadlineLeft-answerReserveS))
	return time.Duration(wall * float64(time.Second))
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

// navExchange is one completed assistant/tool pair produced by the chain.
type navExchange struct {
	Messages    []schema.Message
	EvidenceIDs []string
	Outcomes    []map[string]any
	// PendingRule is the rule control now rests on ("" when the ladder finished
	// or was abandoned).
	PendingRule string
	// BudgetLeft is the unspent prefix budget in seconds.
	BudgetLeft float64
}

// runNavChain: run navRules from startID, stopping
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
func runNavChain(ctx context.Context, ts *Toolset, nav *navContext, startID string, budgetS float64, idPrefix string, maxChars int) navExchange {
	var out navExchange
	if !navRulesEnabled {
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
		if rule.Mode == modeLLM {
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
			oc = executeTool(stepCtx, ts, rule.Tool, rule.Args(nav))
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
				nav.NavHint = TruncateRunes(strings.Join(hints, " "), navHintChars)
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
func (e *navExchange) consumeExchange(ruleID, tool string, args map[string]any, oc ToolOutcome, idPrefix string, maxChars int) {
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
		payload = TruncateRunes(payload, max(maxChars, 800))
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

// runNavPrefix: run the navigation ladder up to
// the first model-driven step.
//
// The returned messages are completed assistant/tool pairs ready to seed the
// session history, so the model sees the chain as work already done and cannot
// skip a step.
//
// Returns an empty exchange when navigation is unavailable — the ladder is an
// optimisation, never a precondition for the session to run.
func runNavPrefix(ctx context.Context, ts *Toolset, direction string, deadlineLeft float64, nav *navContext) navExchange {
	if !navRulesEnabled || ts == nil {
		return navExchange{}
	}
	available := ts.navToolSurface()
	if len(available) == 0 || !available["navigate_tree"] {
		return navExchange{}
	}
	if nav == nil {
		nav = &navContext{Direction: direction}
	}
	budget := deadlineLeft * navPrefixBudgetRatio
	if budget < navPrefixMinBudgetS {
		budget = navPrefixMinBudgetS
	}
	// maxChars 0: the prefix runs before the session's context accounting starts.
	return runNavChain(ctx, ts, nav, navStartRule, budget, "nav", 0)
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

// stringPromptLoader serves templates from an in-memory map. Used by tests and
// as the fallback when no filesystem loader is wired.
type stringPromptLoader map[string]string

// Load implements PromptLoader.
func (p stringPromptLoader) Load(name string) (string, error) {
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
}

// answerContract is the one thing a session must be told beyond its research instructions: the
// answer IT writes is the answer, and it may only cite passages it was shown.
//
// It is appended in Go rather than living in one prompt template because it is a RUNTIME
// contract, not a strategy: the numbers come from the session's own evidence registry
// (stampEvidenceRefs), which is what the citation resolver is handed afterwards. So a number the
// model did not see cannot resolve, and a passage it did see can always be cited — which is what
// stops an unsupported claim from being dressed up as a supported one.
const answerContract = `

FINAL ANSWER: when you have read enough, write the answer as <answer>…</answer> and stop
searching. That text is what the user receives — not a draft, not a summary of what you did.
CITE AS YOU WRITE: put the handle of the passage behind a fact right after it — the [ID:n] a seed
block prints beside a passage, or the ref number a tool result prints beside one, and only handles
you were actually shown. The reader's numbers are renumbered from what you cite, so cite the handle
you saw rather than renumbering it yourself. If part of the question cannot be supported
by passages you have read, say so in the answer (which part, and what you could not find) rather
than leaving it out or guessing.

OPEN PARTS: whenever you could not establish something the question asks for, ALSO write it as
<unresolved>…</unresolved> in the same reply — the specific missing fact, in a few words, named so
it can be probed ("the population figure for Gifu Prefecture", not "the last step"). That block is
how the run knows your answer is not the end: with it, another round is aimed at what you named,
and without it your answer is final. Write an empty block — <unresolved></unresolved> — only when
nothing in the question is left open. Do not use it as a hedge: name a part only if you actually
tried it and the corpus did not answer.
`

// RunActionSession: a bounded session
// pursuing ONE direction.
//
// Returns an empty Result (no states) when no model is configured or the session fails:
// the failure is logged and an empty Result returned rather than propagating.
// ctxBudgetFor is the session's cumulative tool-payload ceiling in CODE POINTS — how much tool
// output ONE direction may ever show its model.
//
// It used to be maxToolResponseChars*4 (48000) for every mode, which is smaller than one page of
// a Wikipedia document: a session read its way to the table it needed and was cut off before the
// rows arrived (measured 2026-09-22, c1 bowling question: an 8275-code-point standings table
// reached the model as its first ~800, and the answer said the table "only displayed the top four
// finishers"). The ceiling now scales with how much the mode is allowed to read — turns and
// snippets-per-query are already mode data (see ModeSpec); this is the same dial, in characters.
func ctxBudgetFor(mode ModeSpec) int {
	turns := mode.ActionMaxTurns
	if turns <= 0 {
		turns = 8
	}
	snips := mode.SnippetsPerQuery
	if snips <= 0 {
		snips = 6
	}
	// medium 8×6 → 6×, high 8×8 → 8×, ultra 10×10 → 12× (clamped); low/naive floor 4× = the old 48000.
	factor := max(4, min(12, turns*snips/8))
	return maxToolResponseChars * factor
}

// sessionDateLine is the one piece of clock context a session cannot infer from the corpus.
// Without it the model falls back on its own priors: measured 2026-09-22, a session answered
// "2026-09-21 is in the future — the current system date is 2026-05-22" and refused a question
// the dataset could answer. Every session seed carries the line.
func sessionDateLine() string {
	now := time.Now()
	return "Today: " + now.Format("2006-01-02") + " (UTC" + now.Format("-07:00") + ")"
}

func RunActionSession(ctx context.Context, deps SessionDeps, direction string, parent State, deadlineLeft float64, baseSummary string, sharedToolCache *ToolCache, sharedSearchQueries []string) Result {
	system := loadPrompt(deps.Prompts, "action_run") + answerContract
	seedUser := sessionDateLine() + "\n\n" + fmt.Sprintf("Direction: %s\n\nState:\n%s", direction, parent.RenderSlots())

	// The SET method is NOT seeded here. It is handed to a session mid-run, on the first batch
	// the caller writes (see appendBatchProtocol): the signal is the session's OWN writing, and
	// it is the one with no observed false positives.
	//
	// What this replaced: seeding the method up front for directions the RUNTIME had decided
	// were enumerations (a typed slot in the planner's table). That decision could not be made
	// from a slot type — "how many people did X kill" and "how many times larger is A than B"
	// are the same shape — and being wrong about it meant a value question was handed set
	// strategy it could only pay for. The batch tell needs no such guess.

	// AVAILABLE METADATA: the dataset's real metadata fields, so a metadata_search filter
	// can name one the dataset actually carries. Omitted entirely when the catalog is
	// empty (no metadata, an unreadable index, or no resolver wired), which keeps a
	// metadata-free corpus's prompt exactly as it shipped.
	if deps.Tools != nil && deps.Tools.MetadataFields != nil {
		if block := deps.Tools.MetadataFields.Render(); block != "" {
			seedUser += "\n\n" + block
			_LOG.Printf("[Action Session] metadata catalog in the seed (%d field(s))", len(deps.Tools.MetadataFields.Keys))
		}
	}

	// ALREADY RETRIEVED: surface the evidence already in the shared pool so the model fills
	// slots from it instead of re-retrieving the same ground. Without this the ReAct loop
	// repeatedly searches evidence it already holds. How MANY of them show is the digest stage's
	// item bound, and how much of each shows is its item allowance (see stageMaxItems /
	// deliverItemText) — neither is a count chosen at this call site.
	if existing := extractRelevantEvidence(deps.KB, direction, stageMaxItems(stageDigest)); existing != "" {
		seedUser += "\n\nALREADY RETRIEVED (do NOT re-retrieve these — use them to fill slots or identify gaps):\n" + existing
	}

	if len(baseSummary) > 0 {
		seedUser += fmt.Sprintf("\n\nPrior round summary:\n%s", baseSummary)
	}

	// The opening's RANKED union, best first: the session's first observation is a ranked candidate
	// list (the paper's o_1), not the pool's insertion order and not a search of its own. Measured
	// 2026-09-20 (三国/关羽): the session spent its only two calls searching a sixty-passage pool and
	// never answered, while the passages its answer needed were reachable but unnamed.
	// The opening shows every member the plan declared (see stageMaxItemsFor), so how many previews it
	// carries comes from the slot table's subjects — the same list the entity-title channel reads.
	if opening := renderOpening(deps.KB, direction, len(DeclaredProbes(parent))); opening != "" {
		seedUser += "\n\nOPENING (candidates for this question in RANKED order, best first — these are PREVIEWS: read a passage before you cite or answer from it):\n" + opening
	}

	// The scan channel: its coverage line AND its delivered windows. Both ride the seed because the
	// number that matters to an enumeration is the one the model cannot see for itself — how much of the
	// corpus's matching material it has NOT been shown — and because windows left in the pool unrendered
	// are windows no answer can enumerate (measured 2026-09-21: 99 windows matched and delivered, and the
	// answer listed twelve of the sixteen members).
	if deps.KB != nil {
		if line := deps.KB.ScanLine(); line != "" {
			seedUser += "\n\n" + line
		}
		// The delivered material, when the channel's seed block is on (see scanSeedBlock).
		if block := scanSeedBlock(deps.KB); block != "" {
			seedUser += "\n\n" + block
		}
	}

	// Documents the run has READ (as opposed to searched), and how far into each it got. The
	// distinction is the difference between the snippets above and the pages behind them: a
	// document read to page 1 of 9 with the answer plausibly further in is the cheapest thing to
	// finish, and nothing else in the session says so (see Kbinfos.ReadProgress).
	if progress := deps.KB.ReadProgress(); len(progress) > 0 {
		seedUser += "\n\nALREADY READ (these documents were read, page by page — a document whose last page still continues is only partly read):\n- " +
			strings.Join(progress, "\n- ")
	}

	if deps.Model == nil {
		_LOG.Printf("[Action Session] no usable model resolved for action session")
		return Result{Messages: nil, NewStates: nil}
	}

	budgetLeft := deadlineLeft
	if budgetLeft <= 0 {
		// A caller that omits the budget gets the session's own clock. There is ONE clock: what
		// a session's time is spent on is decided by what the session does, not by a reading of
		// the direction's shape (see the note on setActionTimeoutS).
		budgetLeft = actionTimeoutS
	}
	sessionClock := sessionClockFor(budgetLeft)

	st := &sessionState{
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
		// The session's clock, INSIDE the context deadline it runs under (see sessionClockGuardS):
		// every budget derived from it — turn wall, tool budget, answer timeout — has to end before
		// the wall does, or the last call is lost at the boundary.
		DeadlineLeft:  sessionClock,
		BudgetS:       sessionClock,
		CtxBudget:     ctxBudgetFor(ResolveMode(deps.Tools)),
		ToolChars:     0,
		ToolCache:     sharedToolCache,
		SearchQueries: sharedSearchQueries,
		SkippedDup:    0,
		ToolStrikes:   map[string]int{},
		ToolOutcomes:  nil,
		Direction:     direction,
		// The method this session MAY be handed, resolved here so nothing mid-session needs
		// the prompt loader. It is shown ONCE, on the first batch the caller writes
		// (appendBatchProtocol) — never in the seed — so nothing has been shown yet.
		EnumerationProtocol: loadOptionalPrompt(deps.Prompts, "action_set"),
	}
	if st.ToolCache == nil {
		st.ToolCache = NewToolCache()
	}
	// This session's numbering CONTINUES the run's: the opening's previews were already published
	// while the seed was built, and the earlier rounds' passages keep the numbers their answers'
	// markers were written against (see Kbinfos.PublishEvidence).
	st.loadEvidenceRefs(deps.KB)

	// The graph loop is bounded by the turn budget and the deadline; the context
	// carries the wall-clock so a stalled provider cannot outlive the request.
	runCtx, cancel := context.WithTimeout(ctx, DeadlineToDuration(budgetLeft))
	defer cancel()

	// Deterministic navigation prefix: run the ladder IN CODE and seed the
	// conversation with the completed exchanges, so the model starts on top of a
	// guaranteed-correct opening instead of being merely *told* the routing rule.
	// The prefix shares the ladder with the in-session fallback, and its pending
	// rule becomes the session's resting point (see sessionState.NavRuleID).
	nav := &navContext{Direction: direction}
	prefixStarted := time.Now()
	prefix := runNavPrefix(runCtx, deps.Tools, direction, budgetLeft, nav)
	// The prefix's elapsed time is charged to the session through the CLOCK, which is an absolute
	// deadline from here on (see refreshClock): the assignment that used to live here subtracted
	// the prefix from a value nothing ever decremented again, so the loop ran on a stale number.
	st.SessionDeadline = time.Now().Add(time.Duration(max(10.0, budgetLeft-time.Since(prefixStarted).Seconds()) * float64(time.Second)))
	st.refreshClock()
	if len(prefix.Messages) > 0 {
		st.Messages = append(st.Messages, prefix.Messages...)
		st.RetrievedEvidenceIDs = append(st.RetrievedEvidenceIDs, prefix.EvidenceIDs...)
		st.ToolOutcomes = append(st.ToolOutcomes, prefix.Outcomes...)
		st.NavRuleID = prefix.PendingRule
		st.RoutedDocs = nav.KnownDocs
		st.NavHint = nav.NavHint
		_LOG.Printf("[Action Session] nav prefix: %d exchange(s), %d evidence id(s), resting on %q",
			len(prefix.Messages)/2, len(prefix.EvidenceIDs), prefix.PendingRule)
		// The ladder's payloads print no handle, and nothing registered them: they are the FIRST
		// passages the model reads, so they are the first it may answer from (see seedEvidenceRefs).
		if lines := st.seedEvidenceRefs(prefix.EvidenceIDs); len(lines) > 0 {
			st.Messages = append(st.Messages, *schema.UserMessage(
				"NAV PREFIX EVIDENCE — the ladder already read these passages; [ID:n] is the handle to cite each of them:\n- " +
					strings.Join(lines, "\n- ")))
			_LOG.Printf("[Action Session] nav prefix: %d passage(s) numbered for citation.", len(lines))
		}
	}

	if err := st.sessionLoop(runCtx); err != nil {
		// A session that was CUT still did work, and the work is what the round needs: its state
		// patches (the record), the passages it read (the citation registry) and, when it got that
		// far, its answer. This used to return an empty Result, which threw all of it away —
		// measured 2026-09-20 (FRAMES, 20:15): 5 of 19 sessions were cut by the clock and each lost
		// every patch and every evidence id, so the round had no answer AND a thin registry, and the
		// composition that replaced it answered from whatever ranked first. A cut is the normal end
		// of a session that ran to its clock; it is not a reason to discard the research.
		_LOG.Printf("[Action Session] session cut: %v — returning the %d passage(s) and %d patch(es) it had.",
			err, len(st.EvidenceRefs), len(st.NewStates))
	}
	return sessionResult(st)
}

// sessionResult is what leaves a session, cut or finished: the record it wrote, the passages it
// read (the registry its own [ID:n] markers index into), the answer when it got that far, and the
// parts it said were still open. One mapping for both exits, so a cut cannot silently produce
// nothing while a completed session produces everything.
func sessionResult(st *sessionState) Result {
	return Result{
		Messages:             st.Messages,
		NewStates:            st.NewStates,
		FoundAnswer:          st.FoundAnswer,
		RetrievedEvidenceIDs: st.RetrievedEvidenceIDs,
		// The registry travels OUT of the session: a caller that lets the session's own answer
		// stand hands THIS list to the citation resolver, so the [ID:n] the model wrote (against
		// the numbers it was shown) resolve against the same numbering.
		EvidenceRefs: append([]string(nil), st.EvidenceRefs...),
		// What the session said it could not establish travels out with the answer: the round's
		// routing decides on BOTH (see routeResearch).
		Unresolved:      strings.TrimSpace(st.Unresolved),
		TerminalType:    st.TerminalType,
		TerminalPayload: st.TerminalPayload,
	}
}

// initResult: tuple return: the root slot
// table plus the queries for the first round.
type initResult struct {
	Root         State
	FirstQueries []string
}

// InitializeState: ask the model to decompose
// the question into a slot table, with a deterministic fallback when the call
// fails.
//
// deadlineLeft bounds the decomposition call.
func InitializeState(ctx context.Context, deps SessionDeps, question string, fanoutHint []string, deadlineLeft float64) initResult {
	system := loadPrompt(deps.Prompts, "action_initialize_state")
	user := sessionDateLine() + "\nQuestion: " + question
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
				return initResult{}
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
				return initResult{}
			}
			if len(clues) > 4 {
				clues = clues[:4]
			}
			// The slot may DECLARE the act words of its direction (see Variable.Terms): a
			// missing or malformed list is simply no declaration, never a reason to discard
			// the decomposition. The cap is a prompt-budget bound on a list the model wrote,
			// like every other one here.
			const actWordsMax = 10
			terms, _ := asStringList(m["scan"])
			if len(terms) > actWordsMax {
				terms = terms[:actWordsMax]
			}
			kept := make([]string, 0, len(terms))
			for _, t := range terms {
				if t = strings.TrimSpace(t); t != "" {
					kept = append(kept, t)
				}
			}
			// The spellings arrive as a list, like `scan`: a field the model writes is read as the
			// model wrote it, never parsed out of a string (see Variable.Subjects).
			rawSubjects, _ := asStringList(m["subjects"])
			subjects := make([]string, 0, len(rawSubjects))
			for _, sp := range rawSubjects {
				if sp = strings.TrimSpace(sp); sp != "" {
					subjects = append(subjects, sp)
				}
			}
			slots = append(slots, Variable{ID: id, Type: vType, QuestionClues: clues, Terms: kept, Subjects: subjects})
		}
	}
	// `[str(q).strip for q in (data.get("first_queries") or [])][:3]`:
	// the first three entries are stripped and KEPT even when they end up empty
	// (the filter that used to live here made Go pick later entries instead), and
	// a non-iterable value raises exactly like the slot parsing above.
	rawFirst, ok := asStringList(data["first_queries"])
	if !ok {
		_LOG.Printf("[Action Session:init] first_queries is not iterable (%v); discarding the decomposition", data["first_queries"])
		return initResult{}
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
	return initResult{Root: root, FirstQueries: firstQueries}
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
	callCtx, cancel := context.WithTimeout(ctx, DeadlineToDuration(tmo))
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

// toolFunction is the OpenAI-style function descriptor.
type toolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolSpec is an OpenAI-style tool schema.
type ToolSpec struct {
	Type     string       `json:"type"`
	Function toolFunction `json:"function"`
}

// ToolCall is one tool invocation requested by the model.
type ToolCall struct {
	ID      string
	Name    string
	Args    map[string]any
	Unknown bool // name is not in toolMap — answer with a correction, never execute
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

// contextLengthModel is implemented by models that can report their context window in
// tokens, which message-fitting nodes use as the budget for chat.FitMessages. Callers
// type-assert for it and fall back to chat.EffectiveContextLength's 8192 default when it
// is absent, i.e. when the model config omits a context length. This is the fitting
// counterpart to the TemperatureModel seam.
type contextLengthModel interface {
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

// unmarshalModelJSON strips any thinking preamble and Markdown fences, then parses JSON.
// An empty result parses as an empty object so callers can index into `out` without a nil
// check.
func unmarshalModelJSON(text string, out any) error {
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
func DeadlineToDuration(seconds float64) time.Duration {
	if seconds <= 0 {
		seconds = actionTimeoutS
	}
	return time.Duration(seconds * float64(time.Second))
}

// The seed's scan material is bounded by CHARACTERS — the one bound, rather than a line cap, because
// the point of the block is that it is the whole delivered material (see renderScanWindows) — and the
// bound itself is the SCAN stage's block bound (see StageChars(stageScan)), not a constant here.
//
// 40000 is about 20-30k tokens of Chinese, the same order as what a WeKnora-style pipeline hands its
// answerer in one pass. The block is a PAGE, not the pool: a bigger one does not buy members — measured
// 2026-09-21 00:52, an 80000-character page with the actor channel in front of it produced a FIVE-member
// answer, because the round costing nothing but reading is the same round that has nothing left to
// search with. The actor channel's coverage lives in the pool (every window it reached is citable), and
// the page shows the head of the act-word ranking, which is where the deed's sentences are.

// scanSeedBlockEnabled gates the SCAN WINDOWS block in the session's seed.
//
// ON, and that is a MEASURED decision, not a default: switching it OFF for one FRAMES run
// (2026-09-21 15:31, everything else at the 14:17 configuration) took the session from 19 retrieval
// calls to 32 and from turn ≤ 7 to turn 13 — the hypothesis was right — and the score from 0.850 to
// 0.725: the sessions searched more and found less, because the block is the only place the run hands
// over the material it ALREADY retrieved. More searching is not the goal; the goal is that the material
// reaches the answer. The measured cost of the block on a single-answer question (692 — the answer
// read "the corpus does not hold the continental-US tallest waterfall") is real but is a GATING
// problem: gate it on the plan's declaration (a member-set slot), do not switch it off wholesale.
const scanSeedBlockEnabled = true

// scanSeedBlock returns the SCAN WINDOWS block for the seed, or "" while the block is switched off.
func scanSeedBlock(kb *Kbinfos) string {
	if !scanSeedBlockEnabled {
		return ""
	}
	return renderScanWindows(kb, 0)
}

// scanWindowRunes bounds one rendered scan window: the sentence around the match.
const scanWindowRunes = 240

// renderScanWindows renders the scan's delivered windows as the enumeration material the session starts
// from: one line per window, each with the citation handle of the passage it came from, bounded by
// CHARACTERS (the scan stage's block bound, see StageChars) and by max lines when a caller asks for one (max <= 0: no line cap, which
// is what production passes). Every window it shows is PUBLISHED, so the answer can cite what it read.
//
// COMPLETE by construction, and that is the point. The block used to render thirty lines and say "…and
// 103 more window(s)" — a sample, so the model enumerated from a sample. Measured 2026-09-21 (三国/关羽):
// the scan matched 162 windows and delivered 133, the seed showed 30, and the answer listed eleven of
// the sixteen members, while the round before — 141 matched — listed fourteen. A counted answer needs
// the whole set in front of it ONCE, the way a WeKnora-style pipeline hands its answerer every retained
// chunk in a single pass and asks it to cite; more retrieval and a better sample of it do not help.
//
// A window is a PREVIEW of the deed's wording (the sentence the probe matched, with its neighbours), not
// proof: the playbook's rule stands — read the passage before you cite it — and the line names the
// passage to read. An identical window is shown once: overlapping windows repeat a sentence, and a
// repeat is not a second member.
func renderScanWindows(kb *Kbinfos, max int) string {
	if kb == nil {
		return ""
	}
	windows := kb.ScanWindows()
	if len(windows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("SCAN WINDOWS (the run's own declared probes matched these passages; each line is the " +
		"sentence around the match — the material to enumerate from):\n")
	budget := StageChars(stageScan)
	shown, chars, repeated := 0, 0, 0
	seen := map[string]bool{}
	for _, w := range windows {
		if max > 0 && shown >= max {
			break
		}
		text := FlattenLine(w.Text)
		if text == "" {
			continue
		}
		if seen[text] {
			repeated++
			continue
		}
		nums := kb.PublishEvidence([]string{w.ChunkID})
		if len(nums) == 0 {
			continue
		}
		line := fmt.Sprintf("[ID:%d] %s | doc %s\n", nums[0], TruncateRunes(text, scanWindowRunes), w.DocID)
		if chars+len([]rune(line)) > budget {
			break
		}
		seen[text] = true
		b.WriteString(line)
		chars += len([]rune(line))
		shown++
	}
	if shown == 0 {
		return ""
	}
	if rest := len(windows) - shown - repeated; rest > 0 {
		fmt.Fprintf(&b, "…and %d more window(s) in the evidence pool (this block is bounded by %d "+
			"characters); read on with list_chunks on the documents above.\n", rest, budget)
	}
	return b.String()
}

// extractRelevantEvidence flattens the shared evidence pool into a compact, line-delimited
// digest the model can read without re-retrieving. Chunks are ranked by the number of
// direction tokens they contain, then the top maxChunks are surfaced so the seed prompt
// stays bounded.
//
// How much of the opening reaches the model has TWO halves and neither is decided here: the LENGTH
// of a preview is the stage's item allowance (see deliverItemText) and the NUMBER of previews is the
// stage's item bound (see stageMaxItems). They used to be a constant pair — an item allowance and a
// separate count of 8 — kept in step by hand at this call site.

// renderOpening renders the opening's ranked union as a bounded, ranked preview list, publishing each
// preview it shows into the run's citation registry (see Kbinfos.PublishEvidence).
//
// The rank order IS the product (see the fan-out's rankOpening): with a clock that affords two
// calls, the order decides what the session reads first. Nothing is filtered — the rest of the
// opening's passages stay in the pool and reachable through the tools.
//
// How MANY it shows is the opening stage's item bound raised to the number of entities the plan
// declared (see stageMaxItemsFor and declaredSubjects): on an enumerated question the preview list is
// the members themselves, and a member past the bound is a member no answer can name.
//
// Each line carries the citation handle of its passage, taken from the registry the answer's markers
// are resolved against: a preview the model may answer from must be citable, and the handle it prints
// must be the number the registry holds — the seed is rendered before the session exists, so the pool
// is the only place that can hand out both.
func renderOpening(kb *Kbinfos, direction string, declaredSubjects int) string {
	if kb == nil {
		return ""
	}
	ids := kb.Opening()
	if len(ids) == 0 {
		return ""
	}
	var b strings.Builder
	shown := 0
	limit := stageMaxItemsFor(stageOpening, declaredSubjects)
	for _, id := range ids {
		if shown >= limit {
			break
		}
		c := kb.ChunkByID(id)
		if c == nil {
			continue
		}
		// The preview is one line per candidate, so the chosen lines are folded to a single
		// line: what the stage's allowance buys is WHICH lines (see deliverable), not how they
		// wrap.
		text := FlattenLine(deliverItemText(stageOpening, ChunkTextOf(c), direction))
		if text == "" {
			continue
		}
		nums := kb.PublishEvidence([]string{id})
		if len(nums) == 0 {
			continue
		}
		fmt.Fprintf(&b, "[ID:%d] %s | doc %s\n", nums[0], text, DocIDOf(c))
		_LOG.Printf("[Action Session] opening preview %d: %s", shown, TruncateRunes(text, 200))
		shown++
	}
	if shown == 0 {
		return ""
	}
	return b.String()
}

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
		// One line per chunk, showing the same kind of view the opening preview shows (see
		// deliverItemText): the LINES that answer the direction, in the chunk's own order,
		// rather than its first N characters.
		content := FlattenLine(deliverItemText(stageDigest, ChunkTextOf(c), direction))
		if content == "" {
			continue
		}
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
