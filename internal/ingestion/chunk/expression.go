//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package chunk

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"unicode"
)

// ---------------------------------------------------------------------------
// Token types
// ---------------------------------------------------------------------------

type tokenType int

const (
	tokenEOF tokenType = iota
	tokenIdentifier
	tokenString
	tokenNumber
	tokenTrue
	tokenFalse
	tokenEq
	tokenNeq
	tokenGt
	tokenLt
	tokenGte
	tokenLte
	tokenAnd
	tokenOr
	tokenNot
	tokenLParen
	tokenRParen
)

var keywords = map[string]tokenType{
	"AND":   tokenAnd,
	"OR":    tokenOr,
	"NOT":   tokenNot,
	"true":  tokenTrue,
	"false": tokenFalse,
	"TRUE":  tokenTrue,
	"FALSE": tokenFalse,
}

type token struct {
	typ tokenType
	raw string
}

// ---------------------------------------------------------------------------
// Lexer
// ---------------------------------------------------------------------------

type lexer struct {
	input []rune
	pos   int
}

func newLexer(input string) *lexer {
	return &lexer{input: []rune(input)}
}

func (l *lexer) skipWhitespace() {
	for l.pos < len(l.input) && unicode.IsSpace(l.input[l.pos]) {
		l.pos++
	}
}

func (l *lexer) next() token {
	l.skipWhitespace()
	if l.pos >= len(l.input) {
		return token{typ: tokenEOF, raw: ""}
	}

	ch := l.input[l.pos]

	// Single-quoted string
	if ch == '\'' {
		l.pos++ // skip opening '
		start := l.pos
		for l.pos < len(l.input) && l.input[l.pos] != '\'' {
			l.pos++
		}
		raw := string(l.input[start:l.pos])
		if l.pos < len(l.input) {
			l.pos++ // skip closing '
		}
		return token{typ: tokenString, raw: raw}
	}

	// Operators
	if l.pos+1 < len(l.input) {
		next := l.input[l.pos+1]
		switch string([]rune{ch, next}) {
		case ">=":
			l.pos += 2
			return token{typ: tokenGte, raw: ">="}
		case "<=":
			l.pos += 2
			return token{typ: tokenLte, raw: "<="}
		case "!=":
			l.pos += 2
			return token{typ: tokenNeq, raw: "!="}
		}
	}
	switch ch {
	case '=':
		l.pos++
		return token{typ: tokenEq, raw: "="}
	case '>':
		l.pos++
		return token{typ: tokenGt, raw: ">"}
	case '<':
		l.pos++
		return token{typ: tokenLt, raw: "<"}
	case '(':
		l.pos++
		return token{typ: tokenLParen, raw: "("}
	case ')':
		l.pos++
		return token{typ: tokenRParen, raw: ")"}
	}

	// Number
	if unicode.IsDigit(ch) || (ch == '-' && l.pos+1 < len(l.input) && unicode.IsDigit(l.input[l.pos+1])) {
		start := l.pos
		if l.input[l.pos] == '-' {
			l.pos++
		}
		for l.pos < len(l.input) && (unicode.IsDigit(l.input[l.pos]) || l.input[l.pos] == '.') {
			l.pos++
		}
		return token{typ: tokenNumber, raw: string(l.input[start:l.pos])}
	}

	// Identifier / keyword
	if unicode.IsLetter(ch) || ch == '_' {
		start := l.pos
		for l.pos < len(l.input) && (unicode.IsLetter(l.input[l.pos]) || unicode.IsDigit(l.input[l.pos]) || l.input[l.pos] == '_') {
			l.pos++
		}
		raw := string(l.input[start:l.pos])
		if kw, ok := keywords[raw]; ok {
			return token{typ: kw, raw: raw}
		}
		return token{typ: tokenIdentifier, raw: raw}
	}

	// Unknown
	l.pos++
	return token{typ: tokenIdentifier, raw: string(ch)}
}

func (l *lexer) peek() token {
	pos := l.pos
	tok := l.next()
	l.pos = pos
	return tok
}

// ---------------------------------------------------------------------------
// AST nodes
// ---------------------------------------------------------------------------

type Expr interface {
	String() string
}

type binaryExpr struct {
	left  Expr
	op    tokenType
	right Expr
}

func (e binaryExpr) String() string {
	ops := map[tokenType]string{
		tokenEq:  "=",
		tokenNeq: "!=",
		tokenGt:  ">",
		tokenLt:  "<",
		tokenGte: ">=",
		tokenLte: "<=",
		tokenAnd: "AND",
		tokenOr:  "OR",
	}
	return fmt.Sprintf("(%s %s %s)", e.left, ops[e.op], e.right)
}

type unaryExpr struct {
	op    tokenType
	right Expr
}

func (e unaryExpr) String() string {
	return fmt.Sprintf("(NOT %s)", e.right)
}

type identifierExpr struct {
	name string
}

func (e identifierExpr) String() string {
	return e.name
}

type stringExpr struct {
	value string
}

func (e stringExpr) String() string {
	return "'" + e.value + "'"
}

type numberExpr struct {
	value float64
}

func (e numberExpr) String() string {
	return strconv.FormatFloat(e.value, 'f', -1, 64)
}

type boolExpr struct {
	value bool
}

func (e boolExpr) String() string {
	return strconv.FormatBool(e.value)
}

// ---------------------------------------------------------------------------
// Recursive-descent parser
// ---------------------------------------------------------------------------

type parser struct {
	lex    *lexer
	cur    token
	peeked bool
}

func newParser(input string) *parser {
	p := &parser{lex: newLexer(input)}
	p.advance()
	return p
}

func (p *parser) advance() {
	if p.peeked {
		p.peeked = false
		return
	}
	p.cur = p.lex.next()
}

func (p *parser) peek() token {
	if !p.peeked {
		p.peeked = true
		p.cur = p.lex.next()
	}
	return p.cur
}

func (p *parser) expect(typ tokenType) token {
	tok := p.cur
	if tok.typ != typ {
		panic(fmt.Sprintf("expected token %d but got %d (%q)", typ, tok.typ, tok.raw))
	}
	p.advance()
	return tok
}

func (p *parser) parse() Expr {
	return p.parseOr()
}

// or_expr → and_expr ("OR" and_expr)*
func (p *parser) parseOr() Expr {
	e := p.parseAnd()
	for p.cur.typ == tokenOr {
		op := p.cur.typ
		p.advance()
		right := p.parseAnd()
		e = binaryExpr{left: e, op: op, right: right}
	}
	return e
}

// and_expr → not_expr ("AND" not_expr)*
func (p *parser) parseAnd() Expr {
	e := p.parseNot()
	for p.cur.typ == tokenAnd {
		op := p.cur.typ
		p.advance()
		right := p.parseNot()
		e = binaryExpr{left: e, op: op, right: right}
	}
	return e
}

// not_expr → "NOT" not_expr | primary
func (p *parser) parseNot() Expr {
	if p.cur.typ == tokenNot {
		op := p.cur.typ
		p.advance()
		right := p.parseNot()
		return unaryExpr{op: op, right: right}
	}
	return p.parsePrimary()
}

// primary → comparison | "(" expression ")"
func (p *parser) parsePrimary() Expr {
	if p.cur.typ == tokenLParen {
		p.advance()
		e := p.parseOr()
		p.expect(tokenRParen)
		return e
	}
	return p.parseComparison()
}

// comparison → IDENTIFIER OP value | value
// comparison → IDENTIFIER OP value
func (p *parser) parseComparison() Expr {
	if p.cur.typ == tokenIdentifier {
		id := p.cur.raw
		p.advance()
		switch p.cur.typ {
		case tokenEq, tokenNeq, tokenGt, tokenLt, tokenGte, tokenLte:
			op := p.cur.typ
			p.advance()
			right := p.parseValue()
			return binaryExpr{left: identifierExpr{name: id}, op: op, right: right}
		default:
			// identifier alone – treat as boolean check
			return binaryExpr{
				left:  identifierExpr{name: id},
				op:    tokenEq,
				right: boolExpr{value: true},
			}
		}
	}
	return p.parseValue()
}

// value → STRING | NUMBER | BOOLEAN
func (p *parser) parseValue() Expr {
	switch p.cur.typ {
	case tokenString:
		v := stringExpr{value: p.cur.raw}
		p.advance()
		return v
	case tokenNumber:
		f, _ := strconv.ParseFloat(p.cur.raw, 64)
		p.advance()
		return numberExpr{value: f}
	case tokenTrue:
		p.advance()
		return boolExpr{value: true}
	case tokenFalse:
		p.advance()
		return boolExpr{value: false}
	default:
		// treat as identifier (e.g. bare variable reference)
		id := identifierExpr{name: p.cur.raw}
		p.advance()
		return id
	}
}

// ---------------------------------------------------------------------------
// Evaluator
// ---------------------------------------------------------------------------

var reMediaURL = regexp.MustCompile(`(?i)https?://[^\s]*\.(jpg|jpeg|png|gif|bmp|webp|svg|mp4|avi|mov|wmv|flv|mkv|m4v|mp3|wav|ogg|aac)`)
var reImageURL = regexp.MustCompile(`(?i)https?://[^\s]*\.(jpg|jpeg|png|gif|bmp|webp|svg)`)
var reVideoURL = regexp.MustCompile(`(?i)https?://[^\s]*\.(mp4|avi|mov|wmv|flv|mkv|m4v)`)
var reAnyURL = regexp.MustCompile(`(?i)https?://[^\s]+`)

// buildExprContext builds a variable context from a chunk's content and metadata.
// It auto-detects media/image/video URLs and language hints.
func buildExprContext(chunk ContentProvider, metadata map[string]interface{}) map[string]interface{} {
	vars := make(map[string]interface{})
	content := chunk.GetContent()

	// Pre-populate from metadata
	for k, v := range metadata {
		vars[k] = v
	}

	// Auto-detect URL presence
	vars["has_media_url"] = reMediaURL.MatchString(content)
	vars["has_image_url"] = reImageURL.MatchString(content)
	vars["has_video_url"] = reVideoURL.MatchString(content)
	vars["has_url"] = reAnyURL.MatchString(content)
	vars["length"] = len([]rune(content))

	return vars
}

// ContentProvider allows evaluating expressions against any type that has content.
type ContentProvider interface {
	GetContent() string
}

// Evaluate parses and evaluates a boolean expression against a variable map.
func Evaluate(exprStr string, vars map[string]interface{}) (bool, error) {
	p := newParser(exprStr)
	ast := p.parse()
	res, err := eval(ast, vars)
	if err != nil {
		return false, fmt.Errorf("evaluate %q: %w", exprStr, err)
	}
	b, ok := toBool(res)
	if !ok {
		return false, fmt.Errorf("evaluate %q: result %v (%T) is not a boolean", exprStr, res, res)
	}
	return b, nil
}

// CompileExpression parses an expression string into a reusable AST.
func CompileExpression(exprStr string) (Expr, error) {
	defer func() {
		if r := recover(); r != nil {
			panic(fmt.Sprintf("compile expression %q: %v", exprStr, r))
		}
	}()
	p := newParser(exprStr)
	return p.parse(), nil
}

// EvalCompiled evaluates a pre-compiled expression AST against variables.
func EvalCompiled(ast interface{}, vars map[string]interface{}) (bool, error) {
	e, ok := ast.(Expr)
	if !ok {
		return false, fmt.Errorf("invalid AST type: %T", ast)
	}
	res, err := eval(e, vars)
	if err != nil {
		return false, err
	}
	b, ok := toBool(res)
	if !ok {
		return false, fmt.Errorf("result %v (%T) is not boolean", res, res)
	}
	return b, nil
}

func eval(e Expr, vars map[string]interface{}) (interface{}, error) {
	switch n := e.(type) {
	case binaryExpr:
		return evalBinary(n, vars)
	case unaryExpr:
		return evalUnary(n, vars)
	case identifierExpr:
		v, ok := vars[n.name]
		if !ok {
			return nil, fmt.Errorf("undefined variable: %s", n.name)
		}
		return v, nil
	case stringExpr:
		return n.value, nil
	case numberExpr:
		return n.value, nil
	case boolExpr:
		return n.value, nil
	default:
		return nil, fmt.Errorf("unknown expression type: %T", e)
	}
}

func evalBinary(e binaryExpr, vars map[string]interface{}) (interface{}, error) {
	left, err := eval(e.left, vars)
	if err != nil {
		return nil, err
	}
	right, err := eval(e.right, vars)
	if err != nil {
		return nil, err
	}

	switch e.op {
	case tokenAnd:
		l, ok := toBool(left)
		if !ok {
			return false, fmt.Errorf("AND requires boolean left operand")
		}
		if !l {
			return false, nil
		}
		r, ok := toBool(right)
		if !ok {
			return false, fmt.Errorf("AND requires boolean right operand")
		}
		return r, nil

	case tokenOr:
		l, ok := toBool(left)
		if !ok {
			return false, fmt.Errorf("OR requires boolean left operand")
		}
		if l {
			return true, nil
		}
		r, ok := toBool(right)
		if !ok {
			return false, fmt.Errorf("OR requires boolean right operand")
		}
		return r, nil

	case tokenEq:
		return compareEq(left, right), nil
	case tokenNeq:
		return !compareEq(left, right), nil
	case tokenGt, tokenLt, tokenGte, tokenLte:
		return compareOrder(left, right, e.op)
	default:
		return false, fmt.Errorf("unknown binary op %d", e.op)
	}
}

func evalUnary(e unaryExpr, vars map[string]interface{}) (interface{}, error) {
	right, err := eval(e.right, vars)
	if err != nil {
		return nil, err
	}
	b, ok := toBool(right)
	if !ok {
		return false, fmt.Errorf("NOT requires boolean operand")
	}
	return !b, nil
}

func toBool(v interface{}) (bool, bool) {
	switch vv := v.(type) {
	case bool:
		return vv, true
	case string:
		return vv == "true" || vv == "TRUE" || vv == "1", true
	case float64:
		return vv != 0, true
	case int:
		return vv != 0, true
	}
	return false, false
}

func compareEq(a, b interface{}) bool {
	// Normalise numeric types
	af, aIsNum := toFloat(a)
	bf, bIsNum := toFloat(b)
	if aIsNum && bIsNum {
		return af == bf
	}
	// Fall back to string comparison
	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}

func toFloat(v interface{}) (float64, bool) {
	switch vv := v.(type) {
	case float64:
		return vv, true
	case int:
		return float64(vv), true
	case string:
		f, err := strconv.ParseFloat(vv, 64)
		return f, err == nil
	}
	return 0, false
}

func compareOrder(a, b interface{}, op tokenType) (bool, error) {
	af, aOK := toFloat(a)
	bf, bOK := toFloat(b)
	if aOK && bOK {
		switch op {
		case tokenGt:
			return af > bf, nil
		case tokenLt:
			return af < bf, nil
		case tokenGte:
			return af >= bf, nil
		case tokenLte:
			return af <= bf, nil
		}
	}
	// String fallback
	sa := fmt.Sprintf("%v", a)
	sb := fmt.Sprintf("%v", b)
	switch op {
	case tokenGt:
		return sa > sb, nil
	case tokenLt:
		return sa < sb, nil
	case tokenGte:
		return sa >= sb, nil
	case tokenLte:
		return sa <= sb, nil
	}
	return false, fmt.Errorf("unsupported comparison op %d between %T and %T", op, a, b)
}

// ---------------------------------------------------------------------------
// Language heuristics
// ---------------------------------------------------------------------------

// DetectLanguage returns a best-effort language code ('zh', 'en', etc.)
// based on the proportion of CJK characters.
func DetectLanguage(text string) string {
	cjk := 0
	total := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			cjk++
		}
		if unicode.IsLetter(r) {
			total++
		}
	}
	if total > 0 && float64(cjk)/float64(total) > 0.3 {
		return "zh"
	}
	return "en"
}

// RuneCount returns the number of runes in text.
func RuneCount(text string) int {
	return len([]rune(text))
}

// Ensure math is used (for NaN etc.)
var _ = math.NaN
