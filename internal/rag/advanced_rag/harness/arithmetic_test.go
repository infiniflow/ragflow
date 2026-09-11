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
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// ---------------------------------------------------------------------------
// The happy path: what the prompt promises the model
// ---------------------------------------------------------------------------

func TestComputeBasicArithmetic(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"12345 + 6789 + 101112", "120246"}, // combined population
		{"1998 - 1954", "44"},               // years between
		{"100 * 4523 / 18092", "25"},        // percentage
		{"25 * 49", "1225"},                 // rate x count
		{"(132 / 3.6) * 1", "36.666667"},    // unit conversion (float rendering)
		{"2 ** 10", "1024"},                 // exponent
		{"7 // 2", "3"},                     // floor division
		{"7 % 3", "1"},                      // modulo
		{"abs(1954 - 1998)", "44"},          // abs
		{"round(3.14159, 2)", "3.14"},       // round with digits
		{"round(2.5)", "2"},                 // Python banker's rounding (half-to-even)
		{"round(3.5)", "4"},                 // half-to-even, away from even
		{"round(0.5)", "0"},                 // half-to-even
		{"-3 % 2", "1"},                     // Python %: sign of the divisor
		{"3 % -2", "-1"},                    // sign of the divisor
		{"min(3, 1, 2)", "1"},
		{"max(3, 1, 2)", "3"},
		{"sum([1, 2, 3, 4])", "10"},
		{"len([\"Alpha\", \"Beta\", \"Gamma\"])", "3"},
		{"0.1 + 0.2", "0.3"},       // float noise suppressed
		{"(2 + 3) * 4", "20"},      // grouping
		{"2 ** 3 ** 2", "512"},     // right-associative
		{"-2 ** 2", "-4"},          // Python precedence
		{"1 if 2 > 1 else 0", "1"}, // ternary
	}
	for _, c := range cases {
		got, err := Compute(c.expr)
		if err != "" {
			t.Errorf("Compute(%q) refused: %s", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("Compute(%q) = %q, want %q", c.expr, got, c.want)
		}
	}
}

func TestComputeHelperFunctions(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`letters("Ada Lovelace")`, "11"},                 // 12 with len — the whole point
		{`letters("Ada Lovelace", "Alan Turing")`, "21"},  // multiple names
		{`letters(["José"])`, "4"},                        // diacritics count
		{`digit_sum("L7 7BN")`, "14"},                     // 7 + 7
		{`digit_sum("2020")`, "4"},                        // each digit separately
		{`date_diff("1941-07-28", "1959-07-17")`, "6563"}, // calendar span (Python docstring: Q317 = 6563)
		{`date_diff("1959-07-17", "1941-07-28")`, "6563"}, // order-independent
		{`sorted([3, 1, 2])[0]`, ""},                      // subscripts refused (see below)
	}
	for _, c := range cases {
		got, err := Compute(c.expr)
		if c.want == "" {
			continue // asserted separately in the refusal tests
		}
		if err != "" {
			t.Errorf("Compute(%q) refused: %s", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("Compute(%q) = %q, want %q", c.expr, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Security: the AST whitelist
// ---------------------------------------------------------------------------

func TestComputeRefusesUnsafeExpressions(t *testing.T) {
	cases := []struct{ expr, wantSubstr string }{
		// Attribute access — the classic sandbox escape.
		{`"".__class__`, ""},
		{`(1).__class__`, ""},
		// Subscripts / indexing.
		{"[1,2,3][0]", ""},
		// Comprehensions and lambdas.
		{"[x for x in [1,2]]", ""},
		{"(lambda: 1)()", ""},
		// Names outside the whitelist.
		{"__import__(\"os\").system(\"ls\")", ""},
		{"open(\"f\").read()", ""},
		{"eval(\"1\")", ""},
		{"exec(\"x\")", ""},
		// Assignment / statements.
		{"x = 1", ""},
		{"import os", ""},
		{"1;2", ""},
		// String amplification via multiplication.
		{`"a" * 100000000`, "multiplication is only allowed on numbers"},
		{"[1] * 100000000", "multiplication is only allowed on numbers"},
		// Exponentiation abuse.
		{"2 ** 1000", "exponent is too large"},
		{`"a" ** 2`, "exponentiation is only allowed on numbers"},
		// len() on a string literal is ambiguous by design.
		{`len("Ada Lovelace")`, "len() on a string literal is ambiguous"},
		// Keyword arguments are refused.
		{"round(3.14159, ndigits=2)", "does not parse"},
		// None is not a value here.
		{"None", "does not parse"},
	}
	for _, c := range cases {
		got, err := Compute(c.expr)
		if err == "" {
			t.Errorf("Compute(%q) = %q, want a refusal", c.expr, got)
			continue
		}
		// The exact wording differs by rejection path (a parse failure vs. a
		// whitelist refusal), but every case here MUST be refused. Assert the
		// specific reason only where it is the point of the case.
		if c.wantSubstr == "" {
			continue
		}
		if !strings.Contains(err, c.wantSubstr) {
			t.Errorf("Compute(%q) error = %q, want it to contain %q", c.expr, err, c.wantSubstr)
		}
	}
}

// TestComputeRecoversFromHelperTypeErrors guards the panic→error conversion:
// letters()/digit_sum() reject bad argument types by panicking (mirroring
// Python's TypeError), and Compute must turn that into a normal refusal rather
// than crashing the request.
func TestComputeRecoversFromHelperTypeErrors(t *testing.T) {
	for _, expr := range []string{`letters(123)`, `digit_sum(1.5)`} {
		got, err := Compute(expr)
		if err == "" {
			t.Errorf("Compute(%q) = %q, want a refusal", expr, got)
		} else if !strings.Contains(err, "failed to evaluate") {
			t.Errorf("Compute(%q) err = %q, want an evaluation failure", expr, err)
		}
	}
}

// no unsafe construct may ever produce a rendered value, whatever the wording.
func TestComputeRejectionsAreNeverValues(t *testing.T) {
	unsafe := []string{
		`"".__class__`, `(1).__class__`, `[1,2,3][0]`,
		`[x for x in [1,2]]`, `(lambda: 1)()`,
		`__import__("os").system("ls")`, `open("f").read()`,
		`eval("1")`, `exec("x")`, `globals()`, `getattr(1, "x")`,
		`x = 1`, `import os`, `None`, `1;2`,
	}
	for _, expr := range unsafe {
		got, err := Compute(expr)
		if err == "" {
			t.Errorf("SECURITY: Compute(%q) = %q — must be refused", expr, got)
		}
	}
}

func TestComputeRefusesEmptyAndOversized(t *testing.T) {
	if _, err := Compute(""); err != "empty expression" {
		t.Errorf("empty: err = %q", err)
	}
	if _, err := Compute("   "); err != "empty expression" {
		t.Errorf("blank: err = %q", err)
	}
	long := strings.Repeat("1+", computeMaxChars)
	if _, err := Compute(long); !strings.Contains(err, "longer than") {
		t.Errorf("oversized: err = %q", err)
	}
}

func TestComputeRefusesNonNumericResult(t *testing.T) {
	// A call whose result is not a number must be refused, not rendered.
	for _, expr := range []string{`sorted([3, 1, 2])`, `min("b", "a")`, `max("b", "a")`} {
		if got, err := Compute(expr); err == "" {
			t.Errorf("Compute(%q) = %q, want a refusal (result is not a number)", expr, got)
		}
	}
}

// TestComputeBuiltinStringArgs pins Python's real builtin semantics: int()/float()
// accept numeric strings (mirroring the genuine Python builtins injected via
// _COMPUTE_FUNCTIONS), and min/max/sorted accept and order strings the same way
// Python does. The final result gate then rejects non-numeric results, matching
// Python's isinstance(value, (int, float)) check.
func TestComputeBuiltinStringArgs(t *testing.T) {
	// int()/float() accept strings -> these produce a number and are ACCEPTED,
	// exactly like Python (previously Go rejected them — a true divergence).
	accept := []struct{ expr, want string }{
		{`int("12")`, "12"},
		{`int("  12  ")`, "12"}, // Python strips surrounding whitespace
		{`int(1.5)`, "1"},       // truncate toward zero
		{`float("1.5")`, "1.5"},
		{`float("  1e3  ")`, "1000"},
	}
	for _, c := range accept {
		if got, err := Compute(c.expr); err != "" || got != c.want {
			t.Errorf("Compute(%q) = %q, err=%q; want %q", c.expr, got, err, c.want)
		}
	}
	// These raise in Python (ValueError / TypeError) and must be refused.
	for _, expr := range []string{`int("1.5")`, `float("abc")`, `int("0x10")`} {
		if got, err := Compute(expr); err == "" {
			t.Errorf("Compute(%q) = %q, want a refusal (Python raises)", expr, got)
		}
	}
}

func TestComputeRefusesDivisionByZero(t *testing.T) {
	for _, expr := range []string{"1 / 0", "1 // 0", "1 % 0"} {
		if _, err := Compute(expr); err == "" || !strings.Contains(err, "zero") {
			t.Errorf("Compute(%q) err = %q, want a division-by-zero refusal", expr, err)
		}
	}
}

func TestComputeComparisonChains(t *testing.T) {
	// Python's ast.Compare is an N-ary chain: `a < b < c` means
	// (a < b) and (b < c), with each operand evaluated exactly once and
	// short-circuiting on the first false comparison. A left-associative
	// binary rewrite would instead compare the previous comparison's BOOLEAN
	// result against the next operand, which is wrong.
	//
	// At the top level a comparison yields a bool, and compute() rejects
	// non-numeric final results ("result is bool, not a number"), exactly as
	// in Python. But a bool used numerically inside a call (int/abs/round…)
	// must evaluate with the chained semantics.
	rejected := []string{
		"3 > 2 > 1", "2 < 1 < 3", "1 < 2 < 3 < 4", "1 < 2 > 3",
		"1 <= 1 <= 2", "3 == 3 == 3", "3 == 3 == 4", "5 > 4 > 3 > 2 > 1", "3 > 2",
	}
	for _, expr := range rejected {
		if got, err := Compute(expr); err == "" {
			t.Errorf("Compute(%q) = %q, want rejection (bool is not a number)", expr, got)
		}
	}
	numeric := []struct {
		expr string
		want string
	}{
		{"int(3 > 2 > 1)", "1"},
		{"int(2 < 1 < 3)", "0"},
		{"abs(1 < 2 < 3)", "1"},
		{"int(3 == 3 == 4)", "0"},
		{"round(3 > 2 > 1)", "1"},
		{"int(1 < 2 < 3 and 4 < 5)", "1"},
	}
	for _, c := range numeric {
		if got, err := Compute(c.expr); err != "" || got != c.want {
			t.Errorf("Compute(%q) = %q, err=%q; want %q", c.expr, got, err, c.want)
		}
	}
}

func TestComputeRejectsTrailingInput(t *testing.T) {
	// Everything after the expression must be consumed, or `1 + 1; rm -rf` style
	// smuggling would parse.
	if _, err := Compute("1 + 1 2"); err == "" {
		t.Error("trailing input must be refused")
	}
	if _, err := Compute("1 + 1)"); err == "" {
		t.Error("unbalanced paren must be refused")
	}
}

// ---------------------------------------------------------------------------
// Formatting
// ---------------------------------------------------------------------------

func TestFormatNumber(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{3.0, "3"},
		{0.1 + 0.2, "0.3"},
		{-2.5, "-2.5"},
		{0, "0"},
		{1e20, "100000000000000000000"},
		{3.14159265358979, "3.141593"},
	}
	for _, c := range cases {
		if got := formatNumber(c.in); got != c.want {
			t.Errorf("formatNumber(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// The helper functions directly
// ---------------------------------------------------------------------------

func TestLettersAndDigitSum(t *testing.T) {
	// "José" is 4 letters — diacritics count, spaces/punctuation do not.
	if got := letters([]any{"José"}); got != 4 {
		t.Errorf("letters(José) = %d, want 4", got)
	}
	if got := letters([]any{"Ada Lovelace"}); got != 11 {
		t.Errorf("letters = %d, want 11", got)
	}
	// "Any number of names, or one list of them" (Python _letters).
	if got := letters([]any{"ab", "cde"}); got != 5 {
		t.Errorf("letters multi = %d, want 5", got)
	}
	if got := digitSum([]any{"L7 7BN"}); got != 14 {
		t.Errorf("digit_sum = %d, want 14", got)
	}
	if got := digitSum([]any{"2020"}); got != 4 {
		t.Errorf("digit_sum(2020) = %d, want 4", got)
	}
	// Whole numbers are also accepted.
	if got := digitSum([]any{int64(2020)}); got != 4 {
		t.Errorf("digit_sum(2020 int) = %d, want 4", got)
	}
}

func TestParseISODate(t *testing.T) {
	if _, err := parseISODate("1941-07-28"); err != nil {
		t.Errorf("valid date rejected: %v", err)
	}
	for _, bad := range []string{
		"1941-07", "not-a-date", "1941-07-28-01", // malformed
		"2024-13-01", "2024-00-15", // month out of range (Python ValueError)
		"2024-02-30", "2023-02-29", // day out of range for month
	} {
		if _, err := parseISODate(bad); err == nil {
			t.Errorf("parseISODate(%q) accepted, want a refusal", bad)
		}
	}
}

// TestComputeSetLiteralDedups pins Python set semantics: {a, b, c} literals
// collapse duplicates (len({1,1,2}) == 2), and min/max operate on the unique
// members. Before the dedup fix, Go counted every member, diverging from Python.
func TestComputeSetLiteralDedups(t *testing.T) {
	cases := []struct{ expr, want string }{
		{`len({1,1,2})`, "2"},   // three members, two unique
		{`len({5,5,5,5})`, "1"}, // all duplicate
		{`min({3,3,1})`, "1"},   // dedup before extremum
		{`max({2,2,9})`, "9"},
	}
	for _, c := range cases {
		if got, err := Compute(c.expr); err != "" || got != c.want {
			t.Errorf("Compute(%q) = %q, err=%q; want %q", c.expr, got, err, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Tool-outcome mapping
// ---------------------------------------------------------------------------

func TestExecutorCalculate(t *testing.T) {
	// A model that writes a derivable expression.
	mdl := &fakeModel{replies: []*ModelReply{{
		Content: `{"needed": true, "expression": "1998 - 1954", "label": "years between", "uses": [0]}`,
	}}}
	ex := &searchExecutor{deps: SearchDeps{Model: mdl}}
	oc, _ := ex.Execute(context.Background(), "calculate", map[string]any{
		"question": "How many years between them?",
		"facts":    []any{"born 1954", "died 1998"},
	})
	if oc.Status != StatusOK {
		t.Fatalf("status = %s (%v), want ok", oc.Status, oc.Metrics)
	}
	entry := oc.Payload[0].(map[string]any)
	// Python :961 — the success payload is exactly {"kind","expression","result"};
	// label/uses are deliberately NOT echoed.
	if entry["result"] != "44" {
		t.Errorf("result = %v, want 44", entry["result"])
	}
	if entry["expression"] != "1998 - 1954" {
		t.Errorf("expression = %v", entry["expression"])
	}
	if _, ok := entry["label"]; ok {
		t.Errorf("payload must not echo label: %v", entry)
	}

	// A model that says no derivation is needed → POOR (not an error), so the
	// model then answers from the facts it already has (mirrors Python
	// action_session._exec_calculate: nothing derivable → status=POOR/no_doc).
	mdl = &fakeModel{replies: []*ModelReply{{Content: `{"needed": false}`}}}
	ex = &searchExecutor{deps: SearchDeps{Model: mdl}}
	oc, _ = ex.Execute(context.Background(), "calculate", map[string]any{
		"question": "q", "facts": []any{"a"},
	})
	if oc.Status != StatusPoor {
		t.Errorf("not needed: status = %s, want poor", oc.Status)
	}

	// An unsafe expression → POOR (refused, logged), never a panic or a breach.
	mdl = &fakeModel{replies: []*ModelReply{{
		Content: `{"needed": true, "expression": "__import__(\"os\").system(\"ls\")", "label": "x"}`,
	}}}
	ex = &searchExecutor{deps: SearchDeps{Model: mdl}}
	if oc, _ = ex.Execute(context.Background(), "calculate", map[string]any{
		"question": "q", "facts": []any{"a"},
	}); oc.Status != StatusPoor {
		t.Errorf("unsafe expr: status = %s, want poor (refused)", oc.Status)
	}

	// No facts → POOR/no_doc, NOT bad_args: Python validates nothing and
	// compute_from_facts' own `not facts` guard returns None (action_session.py:_exec_calculate).
	ex = &searchExecutor{deps: SearchDeps{Model: &fakeModel{}}}
	if oc, _ := ex.Execute(context.Background(), "calculate", map[string]any{"question": "q"}); oc.Status != StatusPoor || oc.Reason != ReasonNoDoc {
		t.Errorf("no facts: got (%s,%s), want (poor,no_doc)", oc.Status, oc.Reason)
	}
}

// TestComputeFromFactsAcceptsNonBoolNeeded pins Python's `not data.get("needed")`
// guard: builtin bool() treats a NON-EMPTY STRING as truthy (even the literal
// "false"), so a model that emits needed as a string must still compute. A strict
// bool assertion would silently report "nothing derivable".
func TestComputeFromFactsAcceptsNonBoolNeeded(t *testing.T) {
	mdl := &fakeModel{replies: []*ModelReply{{
		Content: `{"needed": "true", "expression": "2 + 2", "label": "sum"}`,
	}}}
	got := ComputeFromFacts(context.Background(), mdl, "q", []string{"two things"}, 0)
	if got == nil {
		t.Fatal("ComputeFromFacts = nil, want a result for a truthy non-bool needed")
	}
	if got.Value != "4" {
		t.Errorf("value = %q, want 4", got.Value)
	}
}

func TestToolStringListAcceptsAllShapes(t *testing.T) {
	// Models emit []any, []string, or a bare string.
	if got := toolStringList(map[string]any{"facts": []any{"a", "b"}}, "facts"); len(got) != 2 {
		t.Errorf("[]any = %v", got)
	}
	if got := toolStringList(map[string]any{"facts": []string{"a"}}, "facts"); len(got) != 1 {
		t.Errorf("[]string = %v", got)
	}
	if got := toolStringList(map[string]any{"facts": "a"}, "facts"); len(got) != 1 {
		t.Errorf("string = %v", got)
	}
	if got := toolStringList(map[string]any{}, "facts"); got != nil {
		t.Errorf("absent = %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// Tuple literals: Python whitelists ast.Tuple, used as the default sequence in
// sum((1,2,3)) / len((1,2,3)) / min / max / letters.
// ---------------------------------------------------------------------------

func TestComputeTupleLiterals(t *testing.T) {
	cases := []struct{ expr, want string }{
		{"sum((1, 2, 3))", "6"},
		{"len((1, 2, 3))", "3"},
		{"min((3, 1, 2))", "1"},
		{"max((3, 1, 2))", "3"},
		{"100 + sum((10, 20))", "130"},
		{`letters(("Ada", "Lovelace"))`, "11"},
	}
	for _, c := range cases {
		got, err := Compute(c.expr)
		if err != "" {
			t.Errorf("Compute(%q) refused: %s", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("Compute(%q) = %q, want %q", c.expr, got, c.want)
		}
	}
	// A bare top-level tuple is not a number and must be refused (mirrors
	// Python: a tuple result fails the isinstance(value, (int, float)) check).
	if _, err := Compute("(1, 2, 3)"); err == "" {
		t.Error("Compute(\"(1, 2, 3)\") returned a value, want a refusal")
	}
	// Parenthesised expressions (no comma) stay scalar, not a one-tuple.
	if got, err := Compute("(1 + 2) * 4"); err != "" || got != "12" {
		t.Errorf("Compute(\"(1 + 2) * 4\") = %q, %q; want \"12\"", got, err)
	}
}

// tempRecordingModel records the temperature handed to CompleteWithTemperature
// so the compute_from_facts 0.0 requirement can be asserted.
type tempRecordingModel struct {
	replies       []*ModelReply
	temperature   *float64
	contextLength int
	calls         int
}

// ContextLength implements ContextLengthModel; 0 reports "unknown".
func (m *tempRecordingModel) ContextLength() int {
	return m.contextLength
}

func (m *tempRecordingModel) Complete(_ context.Context, _ []schema.Message, _ []ToolSpec) (*ModelReply, error) {
	if m.calls >= len(m.replies) {
		return &ModelReply{Content: "{}"}, nil
	}
	r := m.replies[m.calls]
	m.calls++
	return r, nil
}

func (m *tempRecordingModel) CompleteWithTemperature(_ context.Context, _ []schema.Message, _ []ToolSpec, temp float64) (*ModelReply, error) {
	t := temp
	m.temperature = &t
	if m.calls >= len(m.replies) {
		return &ModelReply{Content: "{}"}, nil
	}
	r := m.replies[m.calls]
	m.calls++
	return r, nil
}

func TestComputeFromFactsUsesTemperatureZero(t *testing.T) {
	mdl := &tempRecordingModel{replies: []*ModelReply{{
		Content: `{"needed": true, "expression": "1998 - 1954", "label": "years", "uses": [0]}`,
	}}}
	cf := ComputeFromFacts(context.Background(), mdl, "How many years?", []string{"born 1954", "died 1998"}, 0)
	if cf == nil {
		t.Fatal("expected a ComputedFact")
	}
	if cf.Value != "44" {
		t.Errorf("value = %q, want 44", cf.Value)
	}
	if mdl.temperature == nil {
		t.Fatal("CompleteWithTemperature was not called")
	}
	if *mdl.temperature != 0.0 {
		t.Errorf("temperature = %v, want 0.0", *mdl.temperature)
	}
}

func TestComputeFromFactsFitsToContextBudget(t *testing.T) {
	mdl := &tempRecordingModel{contextLength: 0, replies: []*ModelReply{{
		Content: `{"needed": true, "expression": "1998 - 1954", "label": "years", "uses": [0]}`,
	}}}
	cf := ComputeFromFacts(context.Background(), mdl, "How many years?", []string{"born 1954", "died 1998"}, 0)
	if cf == nil {
		t.Fatal("expected a ComputedFact")
	}
	if cf.Value != "44" {
		t.Errorf("value = %q, want 44", cf.Value)
	}
}
