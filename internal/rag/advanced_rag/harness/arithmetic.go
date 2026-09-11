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
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/chat"
)

// Deterministic arithmetic over retrieved evidence.
//
// Mirrors Python harness/arithmetic.py.
//
// Some questions ask for a number no single source states — the combined population of
// three counties, how many listed films won an award, the years between two dates. Every
// input is in the evidence by then and only the arithmetic is missing, which an LLM does
// by writing digits one at a time and gets wrong often enough to matter. So the LLM
// writes ONE expression and we evaluate it.
//
// The expression is model-written and NOT trusted: Go has no eval and the model writes
// PYTHON syntax (`**`, `x if y else z`, list literals) that go/parser cannot read, so
// this file ships a small parser for a deliberately tiny Python subset and rejects
// everything outside the whitelist BEFORE evaluation. No reflection, no name lookup, no
// property access: an unlisted construct is a parse error, not a sandbox escape.
//
// Public interface
//
//	Compute(expression) -> (rendered, error)     evaluate one expression safely.
//	ComputeFromFacts(question, facts, fitBudget) -> *ComputedFact | nil
//	    ask the model whether the question asks for a derivable number; if so,
//	    write the expression, evaluate it, return a structured result.

const (
	// computeMaxChars caps the whole expression; every figure is inline, none
	// is long (Python: _COMPUTE_MAX_CHARS).
	computeMaxChars = 400
	// maxStringLiteral caps a string literal argument (Python: 256).
	maxStringLiteral = 256
	// maxPowExponent caps `**` (Python: abs(exponent) > 64 is refused).
	maxPowExponent = 64
)

// Compute evaluates an LLM-written arithmetic expression.
// Returns (rendered, error); exactly one of the two is non-empty. Every
// rejection is a normal outcome — the caller simply carries on without the
// computed evidence.
func Compute(expression string) (string, string) {
	expr := strings.TrimSpace(expression)
	if expr == "" {
		return "", "empty expression"
	}
	if len(expr) > computeMaxChars {
		return "", fmt.Sprintf("expression is longer than %d characters", computeMaxChars)
	}
	p := &parser{src: expr}
	node, err := p.parseExpression()
	if err != nil {
		return "", err.Error()
	}
	if rest := p.skipSpace(); rest < len(p.src) {
		return "", fmt.Sprintf("unexpected trailing input at %d", rest)
	}
	if problem := checkExpression(node); problem != "" {
		return "", problem
	}
	value, err := evalSafe(node)
	if err != nil {
		return "", fmt.Sprintf("failed to evaluate (%v)", err)
	}
	switch v := value.(type) {
	case bool:
		return "", "result is bool, not a number"
	case int64:
		return formatNumber(float64(v)), ""
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return "", "result is not a finite number"
		}
		return formatNumber(v), ""
	}
	return "", fmt.Sprintf("result is %T, not a number", value)
}

// formatNumber mirrors Python _format_number: render a computed number without
// float noise ("3.0" -> "3", 0.1+0.2 -> "0.3").
func formatNumber(value float64) string {
	if value == math.Trunc(value) && math.Abs(value) < 1e15 {
		return strconv.FormatInt(int64(value), 10)
	}
	s := strconv.FormatFloat(value, 'f', 6, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimRight(s, ".")
}

// ---------------------------------------------------------------------------
// AST
// ---------------------------------------------------------------------------

type node interface{ pos() int }

type constNode struct {
	p    int
	kind string // "int" | "float" | "str" | "bool"
	num  float64
	str  string
}

func (n *constNode) pos() int { return n.p }

type nameNode struct {
	p    int
	name string
}

func (n *nameNode) pos() int { return n.p }

type unaryNode struct {
	p       int
	op      string
	operand node
}

func (n *unaryNode) pos() int { return n.p }

type binaryNode struct {
	p           int
	op          string
	left, right node
}

func (n *binaryNode) pos() int { return n.p }

// chainNode mirrors Python ast.Compare, which is an N-ary node: `a < b < c` is
// `(a < b) and (b < c)`, with each operand evaluated exactly ONCE and the chain
// short-circuiting on the first false comparison.
//
// Building it as left-associative binary nodes instead — `((a < b) < c)` —
// compares the previous comparison's BOOLEAN result against the next operand,
// so `3 > 2 > 1` becomes `True > 1` = `1 > 1` = false, and `2 < 1 < 3` becomes
// `False < 3` = `0 < 3` = true. Both are wrong.
type chainNode struct {
	p        int
	ops      []string // len(operands) - 1
	operands []node
}

func (n *chainNode) pos() int { return n.p }

type ifExpNode struct {
	p                  int
	cond, body, orelse node
}

func (n *ifExpNode) pos() int { return n.p }

type callNode struct {
	p    int
	fn   string
	args []node
}

func (n *callNode) pos() int { return n.p }

type seqNode struct {
	p    int
	kind string // "list" | "tuple" | "set"
	elts []node
}

func (n *seqNode) pos() int { return n.p }

// ---------------------------------------------------------------------------
// Lexer + Pratt parser for a tiny Python subset
// ---------------------------------------------------------------------------

// Supported: numeric and string literals, True/False/None, list/tuple/set
// literals, unary + - not, binary + - * / // % ** and comparison chains, `and`
// / `or`, the ternary `a if b else c`, and calls to the whitelisted functions.
// Everything else — attributes, subscripts, comprehensions, lambdas, f-strings,
// assignments, imports — is a parse error.
type parser struct {
	src string
	i   int
}

func (p *parser) skipSpace() int {
	for p.i < len(p.src) && (p.src[p.i] == ' ' || p.src[p.i] == '\t') {
		p.i++
	}
	return p.i
}

func (p *parser) errf(format string, args ...any) error {
	return fmt.Errorf("does not parse (%s)", fmt.Sprintf(format, args...))
}

func (p *parser) parseExpression() (node, error) { return p.parseTernary() }

func (p *parser) parseTernary() (node, error) {
	body, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if !p.matchWord("if") {
		return body, nil
	}
	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if !p.matchWord("else") {
		return nil, p.errf("ternary needs 'else'")
	}
	orelse, err := p.parseExpression()
	if err != nil {
		return nil, err
	}
	return &ifExpNode{p: 0, cond: cond, body: body, orelse: orelse}, nil
}

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if !p.matchWord("or") {
			return left, nil
		}
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: "or", left: left, right: right}
	}
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if !p.matchWord("and") {
			return left, nil
		}
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: "and", left: left, right: right}
	}
}

func (p *parser) parseNot() (node, error) {
	p.skipSpace()
	if p.matchWord("not") {
		operand, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return &unaryNode{op: "not", operand: operand}, nil
	}
	return p.parseComparison()
}

var cmpOps = []string{"==", "!=", "<=", ">=", "<", ">"}

// parseComparison collects the WHOLE comparison chain into one chainNode,
// mirroring Python's ast.Compare. Two or more operators are what make Python's
// chaining observable; a single comparison is just a one-op chain.
func (p *parser) parseComparison() (node, error) {
	start := p.i
	first, err := p.parseSum()
	if err != nil {
		return nil, err
	}
	operands := []node{first}
	var ops []string
	for {
		p.skipSpace()
		matched := ""
		for _, op := range cmpOps {
			if strings.HasPrefix(p.src[p.i:], op) {
				matched = op
				break
			}
		}
		if matched == "" {
			break
		}
		p.i += len(matched)
		right, err := p.parseSum()
		if err != nil {
			return nil, err
		}
		ops = append(ops, matched)
		operands = append(operands, right)
	}
	if len(ops) == 0 {
		return first, nil
	}
	return &chainNode{p: start, ops: ops, operands: operands}, nil
}

func (p *parser) parseSum() (node, error) {
	left, err := p.parseProduct()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if p.i >= len(p.src) {
			return left, nil
		}
		var op string
		switch p.src[p.i] {
		case '+':
			op = "+"
		case '-':
			op = "-"
		default:
			return left, nil
		}
		p.i++
		right, err := p.parseProduct()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseProduct() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		p.skipSpace()
		if p.i >= len(p.src) {
			return left, nil
		}
		var op string
		switch {
		case strings.HasPrefix(p.src[p.i:], "**"):
			op = "**"
		case strings.HasPrefix(p.src[p.i:], "//"):
			op = "//"
		case p.src[p.i] == '*':
			op = "*"
		case p.src[p.i] == '/':
			op = "/"
		case p.src[p.i] == '%':
			op = "%"
		default:
			return left, nil
		}
		p.i += len(op)
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &binaryNode{op: op, left: left, right: right}
	}
}

func (p *parser) parseUnary() (node, error) {
	p.skipSpace()
	if p.i < len(p.src) {
		switch p.src[p.i] {
		case '-':
			p.i++
			operand, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &unaryNode{op: "-", operand: operand}, nil
		case '+':
			p.i++
			operand, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			return &unaryNode{op: "+", operand: operand}, nil
		}
	}
	return p.parsePower()
}

// parsePower binds tighter than unary on the right and is right-associative,
// matching Python: `2 ** 3 ** 2` is 512 and `-2 ** 2` is -4.
func (p *parser) parsePower() (node, error) {
	base, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	p.skipSpace()
	if strings.HasPrefix(p.src[p.i:], "**") {
		p.i += 2
		exponent, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return &binaryNode{op: "**", left: base, right: exponent}, nil
	}
	return base, nil
}

func (p *parser) parseAtom() (node, error) {
	p.skipSpace()
	if p.i >= len(p.src) {
		return nil, p.errf("unexpected end of expression")
	}
	start := p.i
	c := p.src[p.i]

	switch {
	case c == '(':
		p.i++
		inner, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.i >= len(p.src) {
			return nil, p.errf("unclosed '('")
		}
		// A trailing (or interior) comma turns this into a TUPLE literal, which
		// Python whitelists as ast.Tuple and uses as the default sequence in
		// sum((1,2,3)) / len((1,2,3)) / min((1,2,3)) / letters((...)). A single
		// element with no comma is a plain parenthesised expression (not a
		// one-tuple) — Python's `(1+2)` == 3 vs `(1,)` == (1,).
		if p.src[p.i] == ',' {
			items := []node{inner}
			for {
				p.i++ // consume ','
				p.skipSpace()
				if p.i >= len(p.src) {
					return nil, p.errf("unclosed '('")
				}
				if p.src[p.i] == ')' {
					p.i++
					return &seqNode{kind: "tuple", elts: items}, nil
				}
				elem, err := p.parseExpression()
				if err != nil {
					return nil, err
				}
				items = append(items, elem)
				p.skipSpace()
				if p.i >= len(p.src) {
					return nil, p.errf("unclosed '('")
				}
				if p.src[p.i] == ')' {
					p.i++
					return &seqNode{kind: "tuple", elts: items}, nil
				}
				if p.src[p.i] != ',' {
					return nil, p.errf("unclosed '('")
				}
			}
		}
		if p.src[p.i] != ')' {
			return nil, p.errf("unclosed '('")
		}
		p.i++
		return inner, nil
	case c == '[':
		return p.parseSeq("[", "]", "list")
	case c == '{':
		return p.parseSeq("{", "}", "set")
	case c == '"' || c == '\'':
		return p.parseString()
	case c >= '0' && c <= '9':
		return p.parseNumber()
	case isIdentStart(c):
		return p.parseNameOrCall()
	}
	return nil, p.errf("unexpected character %q at %d", string(c), start)
}

func (p *parser) parseSeq(open, close, kind string) (node, error) {
	p.i++ // consume open
	seq := &seqNode{kind: kind}
	p.skipSpace()
	if p.i < len(p.src) && p.src[p.i] == close[0] {
		p.i++
		return seq, nil
	}
	for {
		elt, err := p.parseExpression()
		if err != nil {
			return nil, err
		}
		seq.elts = append(seq.elts, elt)
		p.skipSpace()
		if p.i >= len(p.src) {
			return nil, p.errf("unclosed %q", open)
		}
		if p.src[p.i] == close[0] {
			p.i++
			return seq, nil
		}
		if p.src[p.i] != ',' {
			return nil, p.errf("expected ',' or %q", close)
		}
		p.i++
	}
}

func (p *parser) parseString() (node, error) {
	quote := p.src[p.i]
	p.i++
	var b strings.Builder
	for p.i < len(p.src) && p.src[p.i] != quote {
		if p.src[p.i] == '\\' && p.i+1 < len(p.src) {
			b.WriteByte(p.src[p.i+1])
			p.i += 2
			continue
		}
		b.WriteByte(p.src[p.i])
		p.i++
	}
	if p.i >= len(p.src) {
		return nil, p.errf("unterminated string")
	}
	p.i++ // consume closing quote
	return &constNode{kind: "str", str: b.String()}, nil
}

func (p *parser) parseNumber() (node, error) {
	start := p.i
	for p.i < len(p.src) && (p.src[p.i] >= '0' && p.src[p.i] <= '9') {
		p.i++
	}
	isFloat := false
	if p.i < len(p.src) && p.src[p.i] == '.' && p.i+1 < len(p.src) &&
		p.src[p.i+1] >= '0' && p.src[p.i+1] <= '9' {
		isFloat = true
		p.i++
		for p.i < len(p.src) && p.src[p.i] >= '0' && p.src[p.i] <= '9' {
			p.i++
		}
	}
	lit := p.src[start:p.i]
	if isFloat {
		v, err := strconv.ParseFloat(lit, 64)
		if err != nil {
			return nil, p.errf("bad float %q", lit)
		}
		return &constNode{kind: "float", num: v}, nil
	}
	v, err := strconv.ParseFloat(lit, 64)
	if err != nil {
		return nil, p.errf("bad int %q", lit)
	}
	return &constNode{kind: "int", num: v}, nil
}

func (p *parser) parseNameOrCall() (node, error) {
	start := p.i
	for p.i < len(p.src) && isIdentChar(p.src[p.i]) {
		p.i++
	}
	name := p.src[start:p.i]
	switch name {
	case "True":
		return &constNode{kind: "bool", str: "true"}, nil
	case "False":
		return &constNode{kind: "bool", str: "false"}, nil
	case "None":
		return nil, p.errf("None is not allowed")
	}
	p.skipSpace()
	if p.i < len(p.src) && p.src[p.i] == '(' {
		p.i++
		call := &callNode{fn: name}
		p.skipSpace()
		if p.i < len(p.src) && p.src[p.i] == ')' {
			p.i++
			return call, nil
		}
		for {
			arg, err := p.parseExpression()
			if err != nil {
				return nil, err
			}
			call.args = append(call.args, arg)
			p.skipSpace()
			if p.i >= len(p.src) {
				return nil, p.errf("unclosed '('")
			}
			if p.src[p.i] == ')' {
				p.i++
				return call, nil
			}
			if p.src[p.i] != ',' {
				return nil, p.errf("expected ',' or ')'")
			}
			p.i++
		}
	}
	return &nameNode{name: name}, nil
}

func (p *parser) matchWord(word string) bool {
	p.skipSpace()
	if !strings.HasPrefix(p.src[p.i:], word) {
		return false
	}
	end := p.i + len(word)
	if end < len(p.src) && isIdentChar(p.src[end]) {
		return false
	}
	p.i = end
	return true
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

// ---------------------------------------------------------------------------
// Whitelist check — runs BEFORE evaluation
// ---------------------------------------------------------------------------

// computeFunctions mirrors Python _COMPUTE_FUNCTIONS.
var computeFunctions = map[string]bool{
	"abs": true, "round": true, "min": true, "max": true, "sum": true,
	"len": true, "int": true, "float": true, "sorted": true,
	"letters": true, "digit_sum": true, "date_diff": true,
}

// computeAlwaysNumeric mirrors Python _COMPUTE_ALWAYS_NUMERIC: functions whose
// result is a number whatever they are handed. `min`/`max`/`sum` are absent on
// purpose — min("b","a") is a string — and `sorted` returns a list, so neither
// may stand where a number is required.
var computeAlwaysNumeric = map[string]bool{
	"abs": true, "round": true, "int": true, "float": true, "len": true,
	"letters": true, "digit_sum": true, "date_diff": true,
}

// checkExpression mirrors Python _check_expression: reject anything outside the
// arithmetic whitelist. Returns "" when clean.
func checkExpression(n node) string {
	switch t := n.(type) {
	case *constNode:
		return ""
	case *nameNode:
		if !computeFunctions[t.name] {
			return fmt.Sprintf("unknown name %q", t.name)
		}
		return ""
	case *unaryNode:
		return checkExpression(t.operand)
	case *binaryNode:
		if problem := checkExpression(t.left); problem != "" {
			return problem
		}
		if problem := checkExpression(t.right); problem != "" {
			return problem
		}
		// Multiplication and exponentiation can turn a short expression into an
		// arbitrarily large object ("a" * 10**9, [1] * 10**9), so both operands
		// must be provably numeric.
		if t.op == "*" || t.op == "**" {
			if !isNumeric(t.left) || !isNumeric(t.right) {
				if t.op == "*" {
					return "multiplication is only allowed on numbers"
				}
				return "exponentiation is only allowed on numbers"
			}
			if t.op == "**" {
				if c, ok := t.right.(*constNode); ok &&
					(c.kind == "int" || c.kind == "float") && math.Abs(c.num) > maxPowExponent {
					return "exponent is too large"
				}
			}
		}
		return ""
	case *chainNode:
		for _, sub := range t.operands {
			if problem := checkExpression(sub); problem != "" {
				return problem
			}
		}
		return ""
	case *ifExpNode:
		for _, sub := range []node{t.cond, t.body, t.orelse} {
			if problem := checkExpression(sub); problem != "" {
				return problem
			}
		}
		return ""
	case *seqNode:
		for _, elt := range t.elts {
			if problem := checkExpression(elt); problem != "" {
				return problem
			}
		}
		return ""
	case *callNode:
		if !computeFunctions[t.fn] {
			return "only the listed functions may be called"
		}
		// len("Ada Lovelace") is 12 and the answer is 11 — the gap is silent, so
		// the expression is refused rather than counted.
		if t.fn == "len" && len(t.args) == 1 {
			if c, ok := t.args[0].(*constNode); ok && c.kind == "str" {
				return "len() on a string literal is ambiguous; use letters()"
			}
		}
		for _, arg := range t.args {
			if c, ok := arg.(*constNode); ok && c.kind == "str" && len(c.str) > maxStringLiteral {
				return "string literal is too long"
			}
			if problem := checkExpression(arg); problem != "" {
				return problem
			}
		}
		return ""
	}
	return "unsupported expression"
}

// isNumeric mirrors Python _is_numeric: true when the node can ONLY evaluate to
// a number.
func isNumeric(n node) bool {
	switch t := n.(type) {
	case *constNode:
		return t.kind == "int" || t.kind == "float" || t.kind == "bool"
	case *unaryNode:
		return isNumeric(t.operand)
	case *binaryNode:
		switch t.op {
		case "+", "-", "*", "/", "//", "%", "**":
			return isNumeric(t.left) && isNumeric(t.right)
		case "==", "!=", "<", "<=", ">", ">=", "and", "or":
			return true // a comparison is a bool, and a bool is an int
		}
		return false
	case *chainNode:
		return true // Python _is_numeric: a comparison is a bool, and a bool is an int
	case *ifExpNode:
		return isNumeric(t.body) && isNumeric(t.orelse)
	case *callNode:
		if computeAlwaysNumeric[t.fn] {
			return true
		}
		if t.fn == "sum" || t.fn == "min" || t.fn == "max" {
			for _, arg := range t.args {
				if !(isNumeric(arg) || isNumericSequence(arg)) {
					return false
				}
			}
			return true
		}
		return false
	}
	return false
}

// isNumericSequence mirrors Python _is_numeric_sequence: a literal sequence whose
// every element is provably numeric.
func isNumericSequence(n node) bool {
	seq, ok := n.(*seqNode)
	if !ok {
		return false
	}
	for _, elt := range seq.elts {
		if !isNumeric(elt) {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Evaluation — no builtins, no reflection
// ---------------------------------------------------------------------------

// evalSafe evaluates n, converting a panic into an error.
//
// The helper functions (letters / digit_sum) reject bad argument types by
// panicking, mirroring Python's TypeError. Python catches those at the top of
// `compute` (`except Exception`); without a recover here the same input would
// crash the whole request instead of being a normal, logged refusal.
func evalSafe(n node) (value any, err error) {
	defer func() {
		if r := recover(); r != nil {
			value = nil
			err = fmt.Errorf("%v", r)
		}
	}()
	return evalNode(n)
}

func evalNode(n node) (any, error) {
	switch t := n.(type) {
	case *constNode:
		switch t.kind {
		case "int":
			return int64(t.num), nil
		case "float":
			return t.num, nil
		case "bool":
			return t.str == "true", nil
		case "str":
			return t.str, nil
		}
	case *nameNode:
		return nil, fmt.Errorf("bare name %q is not a value", t.name)
	case *unaryNode:
		v, err := evalNode(t.operand)
		if err != nil {
			return nil, err
		}
		switch t.op {
		case "-":
			f, err := asNumber(v)
			if err != nil {
				return nil, err
			}
			if i, ok := v.(int64); ok {
				return -i, nil
			}
			return -f, nil
		case "+":
			f, err := asNumber(v)
			if err != nil {
				return nil, err
			}
			if i, ok := v.(int64); ok {
				return i, nil
			}
			return f, nil
		case "not":
			return !truthy(v), nil
		}
	case *ifExpNode:
		cond, err := evalNode(t.cond)
		if err != nil {
			return nil, err
		}
		if truthy(cond) {
			return evalNode(t.body)
		}
		return evalNode(t.orelse)
	case *seqNode:
		out := make([]any, 0, len(t.elts))
		for _, elt := range t.elts {
			v, err := evalNode(elt)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
		if t.kind == "set" {
			// Python set literals deduplicate members: len({1,1,2}) == 2.
			// Without this, len/min/max/sorted on a set literal would count
			// duplicates, diverging from Python.
			out = dedupSequence(out)
		}
		return out, nil
	case *binaryNode:
		return evalBinary(t)
	case *chainNode:
		return evalChain(t)
	case *callNode:
		return evalCall(t)
	}
	return nil, fmt.Errorf("unsupported node")
}

// evalChain evaluates a comparison chain the way Python's ast.Compare does:
// each comparison is applied to ADJACENT operands, results are combined with
// `and`, and each operand is evaluated exactly once. It short-circuits on the
// first false comparison.
func evalChain(t *chainNode) (any, error) {
	left, err := evalNode(t.operands[0])
	if err != nil {
		return nil, err
	}
	for i, op := range t.ops {
		right, err := evalNode(t.operands[i+1])
		if err != nil {
			return nil, err
		}
		ok, err := compareValues(op, left, right)
		if err != nil {
			return nil, err
		}
		if !ok {
			return false, nil
		}
		// The next comparison starts from this operand, not from `true`.
		left = right
	}
	return true, nil
}

func evalBinary(t *binaryNode) (any, error) {
	// `and` / `or` short-circuit, as in Python.
	if t.op == "and" || t.op == "or" {
		left, err := evalNode(t.left)
		if err != nil {
			return nil, err
		}
		if t.op == "and" && !truthy(left) {
			return left, nil
		}
		if t.op == "or" && truthy(left) {
			return left, nil
		}
		return evalNode(t.right)
	}
	left, err := evalNode(t.left)
	if err != nil {
		return nil, err
	}
	right, err := evalNode(t.right)
	if err != nil {
		return nil, err
	}
	switch t.op {
	case "+":
		// Numeric addition, or sequence concatenation (both bounded by literal
		// length, so no amplification risk).
		if li, ok := left.(int64); ok {
			if ri, ok := right.(int64); ok {
				return li + ri, nil
			}
		}
		lf, err1 := asNumber(left)
		rf, err2 := asNumber(right)
		if err1 == nil && err2 == nil {
			return lf + rf, nil
		}
		return concatSequences(left, right)
	case "-", "*", "/", "//", "%", "**":
		lf, err1 := asNumber(left)
		if err1 != nil {
			return nil, err1
		}
		rf, err2 := asNumber(right)
		if err2 != nil {
			return nil, err2
		}
		switch t.op {
		case "-":
			if li, ok := left.(int64); ok {
				if ri, ok := right.(int64); ok {
					return li - ri, nil
				}
			}
			return lf - rf, nil
		case "*":
			if li, ok := left.(int64); ok {
				if ri, ok := right.(int64); ok {
					return li * ri, nil
				}
			}
			return lf * rf, nil
		case "/":
			if rf == 0 {
				return nil, fmt.Errorf("division by zero")
			}
			return lf / rf, nil
		case "//":
			if rf == 0 {
				return nil, fmt.Errorf("floor division by zero")
			}
			return math.Floor(lf / rf), nil
		case "%":
			if rf == 0 {
				return nil, fmt.Errorf("modulo by zero")
			}
			// Python's % returns a value with the SIGN OF THE DIVISOR
			// (a - (b * floor(a / b))): -3 % 2 == 1, 3 % -2 == -1.
			// math.Mod returns a value with the sign of the dividend
			// (math.Mod(-3, 2) == -1), so adjust when signs disagree.
			r := math.Mod(lf, rf)
			if r != 0 && (r < 0) != (rf < 0) {
				r += rf
			}
			return r, nil
		case "**":
			return math.Pow(lf, rf), nil
		}
	case "==", "!=", "<", "<=", ">", ">=":
		return compareValues(t.op, left, right)
	}
	return nil, fmt.Errorf("unsupported operator %q", t.op)
}

func evalCall(t *callNode) (any, error) {
	args := make([]any, 0, len(t.args))
	for _, arg := range t.args {
		v, err := evalNode(arg)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	}
	switch t.fn {
	case "abs":
		f, err := asNumber(single(args))
		if err != nil {
			return nil, err
		}
		if v, ok := single(args).(int64); ok && v < 0 {
			return -v, nil
		}
		return math.Abs(f), nil
	case "round":
		f, err := asNumber(single(args))
		if err != nil {
			return nil, err
		}
		if len(args) > 1 {
			nd, err := asNumber(args[1])
			if err != nil {
				return nil, err
			}
			scale := math.Pow(10, nd)
			return math.RoundToEven(f*scale) / scale, nil
		}
		// Python's builtin round() is round-half-to-even (banker's rounding):
		// round(2.5) == 2, round(3.5) == 4. math.Round rounds half away from
		// zero (round(2.5) == 3), so RoundToEven mirrors Python.
		return math.RoundToEven(f), nil
	case "int":
		// Python's int() accepts a number (truncating toward zero) or a
		// whole-number string; int("1.5") is a ValueError, so reject decimals.
		v := single(args)
		switch n := v.(type) {
		case int64, float64, bool:
			f, err := asNumber(v)
			if err != nil {
				return nil, err
			}
			return int64(f), nil
		case string:
			i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("int(%q) is not a whole number", n)
			}
			return i, nil
		default:
			return nil, fmt.Errorf("int() expects a number or numeric string, got %T", v)
		}
	case "float":
		// Python's float() accepts a number or any numeric string.
		v := single(args)
		switch n := v.(type) {
		case int64, float64, bool:
			f, err := asNumber(v)
			if err != nil {
				return nil, err
			}
			return f, nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if err != nil {
				return nil, fmt.Errorf("float(%q) is not a number", n)
			}
			return f, nil
		default:
			return nil, fmt.Errorf("float() expects a number or numeric string, got %T", v)
		}
	case "len":
		return int64(sequenceLen(single(args))), nil
	case "min", "max":
		flat := flattenArgs(args)
		if len(flat) == 0 {
			return nil, fmt.Errorf("%s() needs at least one argument", t.fn)
		}
		return extrema(t.fn, flat)
	case "sum":
		flat := flattenArgs(args)
		total := 0.0
		allInt := true
		for _, v := range flat {
			f, err := asNumber(v)
			if err != nil {
				return nil, err
			}
			if _, ok := v.(int64); !ok {
				allInt = false
			}
			total += f
		}
		if allInt {
			return int64(total), nil
		}
		return total, nil
	case "sorted":
		return sortValues(flattenArgs(args))
	case "letters":
		return int64(letters(flattenArgs(args))), nil
	case "digit_sum":
		return int64(digitSum(flattenArgs(args))), nil
	case "date_diff":
		if len(args) != 2 {
			return nil, fmt.Errorf("date_diff() takes exactly two ISO dates")
		}
		return dateDiff(args[0], args[1])
	}
	return nil, fmt.Errorf("unknown function %q", t.fn)
}

func single(args []any) any {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

// flattenArgs mirrors Python's "any number of args, or a single list of them".
func flattenArgs(args []any) []any {
	if len(args) == 1 {
		if list, ok := args[0].([]any); ok {
			return list
		}
	}
	return args
}

func asNumber(v any) (float64, error) {
	switch n := v.(type) {
	case int64:
		return float64(n), nil
	case float64:
		return n, nil
	case bool:
		if n {
			return 1, nil
		}
		return 0, nil
	}
	return 0, fmt.Errorf("%v is not a number", v)
}

func truthy(v any) bool {
	switch n := v.(type) {
	case bool:
		return n
	case int64:
		return n != 0
	case float64:
		return n != 0
	case string:
		return n != ""
	case []any:
		return len(n) > 0
	case nil:
		return false
	}
	return true
}

func sequenceLen(v any) int {
	if list, ok := v.([]any); ok {
		return len(list)
	}
	if s, ok := v.(string); ok {
		return len([]rune(s))
	}
	return 0
}

// dedupSequence mirrors Python set semantics for {a, b, c} literals: repeated
// members collapse to one. The key is a stable, panic-free rendering so
// unhashable elements (nested lists) never crash the evaluator; numeric
// int64/float64 compare by value, so {1, 1.0} dedups to one element, matching
// Python.
func dedupSequence(in []any) []any {
	seen := make(map[string]bool, len(in))
	out := make([]any, 0, len(in))
	for _, v := range in {
		key := fmt.Sprintf("%#v", v)
		if !seen[key] {
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
}

func concatSequences(a, b any) (any, error) {
	la, aOk := a.([]any)
	lb, bOk := b.([]any)
	if aOk && bOk {
		return append(append([]any{}, la...), lb...), nil
	}
	sa, aStr := a.(string)
	sb, bStr := b.(string)
	if aStr && bStr {
		return sa + sb, nil
	}
	return nil, fmt.Errorf("unsupported operand types for +")
}

func compareValues(op string, a, b any) (bool, error) {
	if af, err := asNumber(a); err == nil {
		if bf, err := asNumber(b); err == nil {
			switch op {
			case "==":
				return af == bf, nil
			case "!=":
				return af != bf, nil
			case "<":
				return af < bf, nil
			case "<=":
				return af <= bf, nil
			case ">":
				return af > bf, nil
			case ">=":
				return af >= bf, nil
			}
		}
	}
	as, aStr := a.(string)
	bs, bStr := b.(string)
	if aStr && bStr {
		switch op {
		case "==":
			return as == bs, nil
		case "!=":
			return as != bs, nil
		case "<":
			return as < bs, nil
		case "<=":
			return as <= bs, nil
		case ">":
			return as > bs, nil
		case ">=":
			return as >= bs, nil
		}
	}
	return false, fmt.Errorf("unsupported comparison")
}

func extrema(op string, values []any) (any, error) {
	// Mirrors Python min()/max(): elements are ordered with the same comparison
	// semantics as the rest of the evaluator (compareValues), so strings compare
	// lexicographically and the result keeps its original type. The final numeric
	// gate then rejects a non-numeric result, exactly as Python's
	// isinstance(value, (int, float)) check does.
	best := values[0]
	for _, v := range values[1:] {
		// Keep v when it is the smaller (min) or larger (max) element.
		smaller, err := compareValues("<", v, best)
		if err != nil {
			return nil, err
		}
		if (op == "min" && smaller) || (op == "max" && !smaller) {
			best = v
		}
	}
	return best, nil
}

func sortValues(values []any) ([]any, error) {
	// Mirrors Python sorted(): order elements with the evaluator's comparison
	// semantics (compareValues), so strings sort lexicographically. The result is
	// a list, which the final numeric gate rejects — exactly as Python's
	// isinstance(value, (int, float)) check refuses a list result.
	out := append([]any{}, values...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0; {
			lt, err := compareValues("<", out[j-1], out[j])
			if err != nil {
				return nil, err
			}
			if !lt {
				break
			}
			out[j-1], out[j] = out[j], out[j-1]
			j--
		}
	}
	return out, nil
}

// letters mirrors Python _letters: count alphabetic characters across the given
// names. Spaces, hyphens, apostrophes, digits and punctuation do NOT count;
// letters carrying diacritics DO ("José" is 4).
//
// "How many letters are in these names" is a real question, and every plain way
// to answer it needs machinery this evaluator refuses — an attribute call or a
// comprehension — so it is a function instead.
func letters(items []any) int {
	total := 0
	for _, item := range items {
		s, ok := item.(string)
		if !ok {
			panic(fmt.Sprintf("letters() takes names, not %T", item))
		}
		for _, ch := range s {
			if unicode.IsLetter(ch) {
				total++
			}
		}
	}
	return total
}

// digitSum mirrors Python _digit_sum: add up the decimal digits inside the
// given values. Every digit is added SEPARATELY: digit_sum("L7 7BN") is 14 and
// digit_sum("2020") is 4. Only ASCII digits count.
func digitSum(items []any) int {
	total := 0
	for _, item := range items {
		switch n := item.(type) {
		case bool:
			panic("digit_sum() takes text or whole numbers, not bool")
		case string:
			for i := 0; i < len(n); i++ {
				if n[i] >= '0' && n[i] <= '9' {
					total += int(n[i] - '0')
				}
			}
		case int64:
			for _, ch := range strconv.FormatInt(n, 10) {
				total += int(ch - '0')
			}
		default:
			panic(fmt.Sprintf("digit_sum() takes text or whole numbers, not %T", item))
		}
	}
	return total
}

// dateDiff mirrors Python _date_diff: days between two ISO dates (inclusive of
// the earlier, exclusive of the later — a calendar span).
func dateDiff(a, b any) (int64, error) {
	as, okA := a.(string)
	bs, okB := b.(string)
	if !okA || !okB {
		return 0, fmt.Errorf("date_diff() takes ISO date strings")
	}
	da, err := parseISODate(as)
	if err != nil {
		return 0, err
	}
	db, err := parseISODate(bs)
	if err != nil {
		return 0, err
	}
	days := db.Sub(da).Hours() / 24
	if days < 0 {
		days = -days
	}
	return int64(days), nil
}

func parseISODate(s string) (time.Time, error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("not an ISO date: %q", s)
	}
	y, err1 := strconv.Atoi(parts[0])
	mo, err2 := strconv.Atoi(parts[1])
	d, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, fmt.Errorf("not an ISO date: %q", s)
	}
	if mo < 1 || mo > 12 {
		return time.Time{}, fmt.Errorf("month %d out of range in %q", mo, s)
	}
	t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
	// time.Date silently normalizes out-of-range days (Feb 30 -> Mar 1, month 13
	// -> next Jan). Python date() raises ValueError, so reject any date that does
	// not survive the round trip unchanged.
	if t.Year() != y || int(t.Month()) != mo || t.Day() != d {
		return time.Time{}, fmt.Errorf("day %d out of range for %q", d, s)
	}
	return t, nil
}

// ---------------------------------------------------------------------------
// ComputeFromFacts: decide whether the question asks for a derivable number and,
// if so, compute it. Mirrors Python arithmetic.compute_from_facts (line 356) and
// the _COMPUTE_SYSTEM prompt (line 267). The evaluator side of this lives above
// (Compute / the parser); the two together form the Go port of arithmetic.py.
// ---------------------------------------------------------------------------

// ComputeSystem mirrors arithmetic.py::_COMPUTE_SYSTEM verbatim in intent. It is
// inlined (rather than loaded from rag/prompts) because the Go harness has no
// Jinja env and the template carries no variables.
//
// IMPORTANT: the prompt asks for a PYTHON expression, and the parser in
// arithmetic.go implements exactly the Python subset the prompt promises
// (`**`, `x if y else z`, list literals, the listed functions). Changing one
// without the other will silently refuse every model-written expression.
const ComputeSystem = `You are given the ORIGINAL question and every fact discovered so far. Decide whether that question asks for a NUMBER that NO fact states outright but that FOLLOWS ARITHMETICALLY from figures the facts DO state — a sum, a difference, a count, an average, a percentage, a unit conversion, an elapsed span.

If it does, compute it by writing ONE Python expression with every figure substituted as a literal. The expression is evaluated on its own: no variables, no assignments, no imports, no attributes, no subscripts. The only functions available are abs, round, min, max, sum, len, int, float, sorted, letters, digit_sum and date_diff.
  combined population of three  -> 12345 + 6789 + 101112
  how many of the listed items  -> len(["Alpha", "Beta", "Gamma"])
  what percentage one figure is -> 100 * 4523 / 18092
  years between two dates       -> 1998 - 1954
  days between two dates        -> date_diff("1941-07-28", "1959-07-17")
  letters in a set of names     -> letters("Ada Lovelace", "Alan Turing")
  digits of a postcode added up -> digit_sum("L7 7BN")

ADDING UP THE DIGITS of a postcode, a house number, a serial number, a year or an address: use digit_sum(...), and never read the digits out by hand. It adds each digit separately, which is what such a question means — digit_sum("L7 7BN") is 7+7 = 14, digit_sum("2020") is 2+0+2+0 = 4. Pass the identifier EXACTLY as the facts write it, letters and spaces included; they are ignored. It is the WRONG tool for whole numbers the facts state separately — two populations, two prices, two years are added as plain literals (12345 + 6789), not fed to digit_sum.

COUNTING LETTERS: use letters(...), NEVER len(...) on a name. len counts spaces, hyphens and apostrophes as though they were letters, so it is wrong by exactly the amount nobody notices (len("Ada Lovelace") is 12; the name has 11 letters). letters(...) takes any number of names, or one list of them, and counts alphabetic characters only. Spell each name EXACTLY as the facts give it, including any middle name or accent — and if the facts do not show a name in full, that figure is missing, so return "needed": false rather than counting a partial name.

DAYS BETWEEN TWO DATES: when the question asks "how many days after X did Y happen" / "how many days between two dates", use date_diff("YYYY-MM-DD", "YYYY-MM-DD") with the two dates EXACTLY as the facts write them. Do NOT subtract the years (1959 - 1941) — that is the wrong quantity for a days question (18 is years, not days). If either date is not a full YYYY-MM-DD in the facts, the figure is missing, so return "needed": false rather than approximating.

AGE (an age, or an age difference, at some event): the facts almost always give a birth YEAR and an event YEAR; the age is event_year - birth_year (or birth_year - event_year, taken as the positive difference). You do NOT need the birth month or day — the year is enough. If the facts give FULL dates (YYYY-MM-DD), prefer date_diff(...) which handles the day correctly; otherwise subtract the years. Example: "elected in 2010, born 1971" -> 2010 - 1971. If the event year is BEFORE the birth year, the difference is birth_year - event_year (use abs(...)). Never refuse because the birthday is not a full date — the YEAR is sufficient.

PERCENTAGE (what percent / what share / what proportion / what fraction): 100 * part / whole, where part and whole are the exact figures from the facts. Example:
  "2.7 million Tamazight speakers out of 556 million total" -> 100 * 2.7 / 556
Do not round to an integer unless the question asks for that; keep the source figures exact.

UNIT CONVERSION (a speed, rate, or span in mixed units): convert inside the expression. A speed in km/h becomes m/s by dividing by 3.6. Example for a difference in m/s between a fish and a swimmer:
  fish_kmh / 3.6 - 50 / swimmer_seconds      -> e.g. 132 / 3.6 - 50 / 21.07
Use the EXACT figures the facts state (do not round 21.07 to 21); if the facts give the speed already in m/s, use it directly without dividing.

MULTIPLICATION (a rate times a count, e.g. dollars per day times days): multiply the RATE by the COUNT exactly as the facts state them. Read the rate's NUMBER from the facts. Example: "a suggested donation of $25 per day, kept up for 49 days" -> 25 * 49. If the facts state the rate as 1 but the question calls it "a suggested donation", still use the exact figure the facts give — never substitute a made-up base amount.

Prefer computing over giving up: when the question asks for a derivable number and the facts provide the figures (even if in different units or spread across several facts), WRITE the expression and compute it. In particular, questions asking for an AGE DIFFERENCE, a PERCENTAGE, a SPEED DIFFERENCE (with unit conversion), or a MULTIPLICATION (a rate times a count) are exactly what this tool is for.

Return "needed": false, with an empty expression, ONLY when:
- the ORIGINAL question does not ask for a number;
- a fact already states that number outright — a value you would only be restating is not a calculation;
- a figure the calculation needs is genuinely absent from the facts, or a list the count depends on is not shown to be complete. NEVER invent, estimate, recall or infer a figure. When input is missing, say so and return "needed": false — a wrong number is worse than none — but first check that the figure really is absent (e.g. the age's birth YEAR is enough; you do not need the month).

"label" names what the number IS, as a short noun phrase ("combined population of the three counties"), so a later step can use the result without re-deriving it.
"uses" lists the INDEX NUMBERS of the facts whose figures you substituted.
Output ONLY JSON, no prose, no code fences:
{"needed": true/false, "expression": "<one Python expression, or empty>", "label": "<short noun phrase>", "uses": [<index number>, ...]}`

// ComputedFact mirrors Python compute_from_facts' return dict.
type ComputedFact struct {
	Needed     bool
	Label      string
	Value      string
	Expression string
	Uses       []int
}

// ComputeFromFacts asks the model whether `question` asks for a derivable
// number and, if so, writes + safely evaluates the expression over `facts`.
//
// Returns nil when no derivation is needed or possible — including when the
// model's expression is refused by the whitelist. A refusal is logged, never
// propagated: the caller simply carries on without the computed evidence.
func ComputeFromFacts(ctx context.Context, model SessionModel, question string, facts []string, fitBudget int) *ComputedFact {
	if model == nil || strings.TrimSpace(question) == "" || len(facts) == 0 {
		return nil
	}
	ctx, done := Phase(ctx, PhaseCompute)
	defer done()

	user := fmt.Sprintf("Facts discovered so far:\n%s\n\nOriginal question:\n%s\n\nOutput JSON:",
		renderFacts(facts), question)

	// Mirror Python compute_from_facts: budget = fit_budget or llm.max_length,
	// then message_fit_in(form_message(system, user), budget). The chat seam now
	// exposes llm.max_length via ContextLengthModel; when neither an explicit
	// fitBudget nor a model context window is available the 8192 default (chat
	// EffectiveContextLength) applies, exactly as Python's LLM.max_length
	// defaults when the model config omits max_tokens.
	budget := fitBudget
	if budget <= 0 {
		if cl, ok := model.(ContextLengthModel); ok {
			budget = cl.ContextLength()
		}
		if budget <= 0 {
			budget = chat.EffectiveContextLength(0)
		}
	}
	fitted, fitErr := chat.FitMessages(ComputeSystem, []schema.Message{
		*schema.UserMessage(user),
	}, budget)
	if fitErr != "" {
		_LOG.Printf("[Compute] prompt fitting failed: %s", fitErr)
		return nil
	}
	// FitMessages may prepend/trim a system message; re-extract it so the model
	// call is exactly [system, user...] as Python sends msg[0] then msg[1:].
	systemPrompt := ComputeSystem
	history := fitted
	if len(fitted) > 0 && fitted[0].Role == schema.System {
		systemPrompt = fitted[0].Content
		history = fitted[1:]
	}
	msgs := make([]schema.Message, 0, 1+len(history))
	msgs = append(msgs, *schema.SystemMessage(systemPrompt))
	msgs = append(msgs, history...)
	// Python hardcodes {"temperature": 0.0} for this node — a mechanical rewrite,
	// so it must be deterministic. When the model supports per-call temperature we
	// pass 0.0 exactly; otherwise we fall back to the plain Complete (it then runs
	// at the model's default), the same documented seam as ExtractWeightedKeywords
	// (which uses 0.1).
	var reply *ModelReply
	var err error
	if tm, ok := model.(TemperatureModel); ok {
		reply, err = tm.CompleteWithTemperature(ctx, msgs, nil, 0.0)
	} else {
		reply, err = model.Complete(ctx, msgs, nil)
	}
	if err != nil {
		_LOG.Printf("[Compute] LLM call failed: %v", err)
		return nil
	}
	data, _ := ExtractJSON(StripThinkAndFences(reply.Content)).(map[string]any)
	if data == nil {
		_LOG.Printf("[Compute] could not parse LLM JSON")
		return nil
	}
	// Python evaluates `not data.get("needed")`, i.e. builtin bool(): a truthy
	// non-bool (1, "true", a non-empty list) counts as needed. A strict bool
	// assertion would reject those and wrongly report "nothing derivable".
	if !truthy(data["needed"]) {
		return nil
	}
	expression := ""
	if v, ok := data["expression"].(string); ok {
		expression = strings.TrimSpace(v)
	}
	if expression == "" {
		return nil
	}
	label := ""
	if v, ok := data["label"].(string); ok {
		label = strings.TrimSpace(v)
	}
	if label == "" {
		label = "Value calculated from the facts found"
	}
	uses := make([]int, 0, 4)
	if raw, ok := data["uses"].([]any); ok {
		for _, n := range raw {
			if v, ok := toIntStrict(n); ok {
				uses = append(uses, v)
			}
		}
	}

	value, problem := Compute(expression)
	if problem != "" {
		_LOG.Printf("[Compute] refused %q — %s", trunc(expression, 120), problem)
		return nil
	}
	return &ComputedFact{
		Needed:     true,
		Label:      label,
		Value:      value,
		Expression: expression,
		Uses:       uses,
	}
}

// renderFacts mirrors Python _render_facts: one fact per line, prefixed with its
// index — the index is what "uses" refers back to.
func renderFacts(facts []string) string {
	lines := make([]string, 0, len(facts))
	for i, f := range facts {
		lines = append(lines, fmt.Sprintf("[%d] %s", i, f))
	}
	return strings.Join(lines, "\n")
}
