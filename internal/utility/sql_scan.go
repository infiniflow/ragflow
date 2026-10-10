package utility

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SQLKind is the syntactic class of a token.
type SQLKind uint8

const (
	// Word is a bare identifier or keyword: select, doc_id, count.
	SQLWord SQLKind = iota
	// Quoted is a quoted identifier: "region" or `region`.
	SQLQuoted
	// Number is a numeric literal.
	SQLNumber
	// String is a single-quoted literal.
	SQLString
	// Punct is an operator or separator.
	SQLPunct
)

// SQLToken is one syntactic unit of a statement.
type SQLToken struct {
	Kind SQLKind
	// Text is the source bytes, quotes included.
	Text string
	// Name is the identifier a Word or Quoted token refers to, unquoted.
	Name string
	// Value is the contents of a String literal, unquoted and unescaped.
	Value string
	// Lower is Text lowercased, for a Word or Punct token.
	Lower string
}

// IsWord reports a keyword or identifier by lowercased name.
func (t SQLToken) IsWord(name string) bool { return t.Kind == SQLWord && t.Lower == name }

// IsPunct reports an operator or separator.
func (t SQLToken) IsPunct(text string) bool { return t.Kind == SQLPunct && t.Lower == text }

// String renders the token for error messages. A literal's contents are left
// out: they are caller data and can be arbitrarily large.
func (t SQLToken) String() string {
	switch t.Kind {
	case SQLString:
		return "'...'"
	case SQLQuoted:
		return t.Name
	default:
		return t.Text
	}
}

// multiCharPuncts are matched longest-first so "<=>" is not read as "<=>"
// through a "<=" prefix.
var multiCharPuncts = []string{"<=>", "<=", ">=", "<>", "!=", "==", "||", "::"}

const singleCharPuncts = "(),.;+-*/%=<>."

var errComments = errors.New("sqlscan: comments are not allowed")

// SQLScan splits one statement into tokens. Comments, backslash escapes,
// placeholders and unterminated literals are rejected rather than interpreted.
func SQLScan(sql string) ([]SQLToken, error) {
	if !utf8.ValidString(sql) {
		return nil, errors.New("sqlscan: invalid UTF-8")
	}
	src := []rune(sql)
	tokens := make([]SQLToken, 0, len(src)/4+8)

	for i := 0; i < len(src); {
		r := src[i]
		switch {
		case r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '\v' || r == '\f':
			i++
		case r == '-' && i+1 < len(src) && src[i+1] == '-':
			return nil, errComments
		case r == '/' && i+1 < len(src) && src[i+1] == '*':
			return nil, errComments
		case r == '#':
			return nil, errComments
		case r == '\'':
			token, next, err := scanString(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			i = next
		case r == '"' || r == '`':
			token, next, err := scanQuoted(src, i, r)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			i = next
		case unicode.IsDigit(r), r == '.' && i+1 < len(src) && unicode.IsDigit(src[i+1]):
			token, next, err := scanNumber(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			i = next
		case r == '_', unicode.IsLetter(r):
			start := i
			for i < len(src) && (unicode.IsLetter(src[i]) || unicode.IsDigit(src[i]) || src[i] == '_') {
				i++
			}
			text := string(src[start:i])
			tokens = append(tokens, SQLToken{Kind: SQLWord, Text: text, Name: text, Lower: strings.ToLower(text)})
		default:
			token, next, err := scanPunct(src, i)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, token)
			i = next
		}
	}

	if len(tokens) == 0 {
		return nil, errors.New("sqlscan: statement is empty")
	}
	return tokens, nil
}

func scanString(src []rune, start int) (SQLToken, int, error) {
	var value strings.Builder
	i := start + 1
	for i < len(src) {
		switch r := src[i]; r {
		case '\\':
			return SQLToken{}, 0, fmt.Errorf("sqlscan: backslash escapes are not supported")
		case '\'':
			if i+1 < len(src) && src[i+1] == '\'' {
				value.WriteRune('\'')
				i += 2
				continue
			}
			i++
			text := string(src[start:i])
			return SQLToken{Kind: SQLString, Text: text, Value: value.String()}, i, nil
		default:
			if r < 0x20 || r == 0x7f {
				return SQLToken{}, 0, fmt.Errorf("sqlscan: string literal contains a control character")
			}
			value.WriteRune(r)
			i++
		}
	}
	return SQLToken{}, 0, fmt.Errorf("sqlscan: unterminated string literal")
}

func scanQuoted(src []rune, start int, quote rune) (SQLToken, int, error) {
	var name strings.Builder
	i := start + 1
	for i < len(src) {
		switch src[i] {
		case quote:
			if i+1 < len(src) && src[i+1] == quote {
				name.WriteRune(quote)
				i += 2
				continue
			}
			i++
			nameStr := name.String()
			if nameStr == "" {
				return SQLToken{}, 0, fmt.Errorf("sqlscan: empty quoted identifier")
			}
			return SQLToken{
				Kind:  SQLQuoted,
				Text:  string(src[start:i]),
				Name:  nameStr,
				Lower: strings.ToLower(nameStr),
			}, i, nil
		default:
			if src[i] < 0x20 || src[i] == 0x7f {
				return SQLToken{}, 0, fmt.Errorf("sqlscan: quoted identifier contains a control character")
			}
			name.WriteRune(src[i])
			i++
		}
	}
	return SQLToken{}, 0, fmt.Errorf("sqlscan: unterminated quoted identifier")
}

func scanNumber(src []rune, start int) (SQLToken, int, error) {
	i := start
	for i < len(src) && unicode.IsDigit(src[i]) {
		i++
	}
	if i < len(src) && src[i] == '.' {
		i++
		for i < len(src) && unicode.IsDigit(src[i]) {
			i++
		}
	}
	if i < len(src) && (src[i] == 'e' || src[i] == 'E') {
		j := i + 1
		if j < len(src) && (src[j] == '+' || src[j] == '-') {
			j++
		}
		if j < len(src) && unicode.IsDigit(src[j]) {
			i = j
			for i < len(src) && unicode.IsDigit(src[i]) {
				i++
			}
		}
	}
	if i < len(src) && (unicode.IsLetter(src[i]) || src[i] == '_') {
		return SQLToken{}, 0, fmt.Errorf("sqlscan: number is followed by an identifier")
	}
	text := string(src[start:i])
	return SQLToken{Kind: SQLNumber, Text: text, Lower: text}, i, nil
}

func scanPunct(src []rune, i int) (SQLToken, int, error) {
	for _, op := range multiCharPuncts {
		if hasPrefixRunes(src[i:], op) {
			return SQLToken{Kind: SQLPunct, Text: op, Lower: op}, i + len([]rune(op)), nil
		}
	}
	r := src[i]
	if strings.ContainsRune(singleCharPuncts, r) {
		return SQLToken{Kind: SQLPunct, Text: string(r), Lower: string(r)}, i + 1, nil
	}
	if r < 0x20 || r == 0x7f {
		return SQLToken{}, 0, fmt.Errorf("sqlscan: control character %#x is not allowed", r)
	}
	return SQLToken{}, 0, fmt.Errorf("sqlscan: %q is not a supported operator", string(r))
}

func hasPrefixRunes(src []rune, prefix string) bool {
	runes := []rune(prefix)
	if len(src) < len(runes) {
		return false
	}
	for i, r := range runes {
		if src[i] != r {
			return false
		}
	}
	return true
}

// SQLRender normalizes == to = and writes tokens separated by a
// single space. Both engines read that as the same syntax, and it means a
// checked statement cannot be re-split by whatever whitespace a caller typed.
//
// quote is the character a Quoted identifier is re-quoted with: '"' for
// Infinity and PostgreSQL, '`' for OceanBase and SeekDB. Zero keeps the
// source text, which is what a caller that only moves tokens needs.
func SQLRender(tokens []SQLToken, quote rune) string {
	var b strings.Builder
	for i, token := range tokens {
		// A separator only binds a name to its qualifier: "t.col", "a, b".
		// Everything else keeps its space, so no two tokens can merge into a
		// comment marker or an operator the caller did not write.
		if i > 0 && !token.IsPunct(",") && !token.IsPunct(".") && !tokens[i-1].IsPunct(".") {
			b.WriteByte(' ')
		}
		switch {
		case token.IsPunct("=="):
			b.WriteByte('=')
		case token.Kind == SQLQuoted && quote != 0:
			escaped := strings.ReplaceAll(token.Name, string(quote), string(quote)+string(quote))
			b.WriteString(string(quote) + escaped + string(quote))
		default:
			b.WriteString(token.Text)
		}
	}
	return b.String()
}

// SQLQuoteLiteral writes a value as a single-quoted literal. Callers use it for
// the identifiers the backend itself injects, so the statement stays one the
// scanner can read back.
func SQLQuoteLiteral(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("sqlscan: %q is not valid UTF-8", value)
	}
	for _, r := range value {
		if r == '\\' || r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("sqlscan: literal contains an unsupported character")
		}
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}

// Clauses are the top-level parts of a single-table SELECT, split at depth
// zero so a caller can reason about each one on its own. A part the statement
// did not contain is empty.
type SQLClauses struct {
	Select  []SQLToken
	From    []SQLToken
	Where   []SQLToken
	GroupBy []SQLToken
	Having  []SQLToken
	OrderBy []SQLToken
	Limit   []SQLToken
	Offset  []SQLToken
}

// SQLStatementShape is a parsed single-table SELECT.
type SQLStatementShape struct {
	Clauses *SQLClauses
	// Aggregating is true when an aggregate function is called in the select
	// list, which is what decides whether the result rows name source
	// documents.
	Aggregating bool
}

// aggregateFunctions are the calls that collapse rows into one answer.
var aggregateFunctions = map[string]bool{
	"count": true, "sum": true, "avg": true, "min": true, "max": true,
	"total": true, "group_concat": true, "stddev": true, "variance": true,
	"var_pop": true, "stddev_pop": true,
}

// structuralWords cannot appear at any depth of the statement this package
// reads: each one starts a compound query, a common table expression, a join
// or a write.
var structuralWords = map[string]bool{
	"with": true, "union": true, "intersect": true, "except": true,
	"join": true, "using": true, "window": true, "into": true,
	"tablesample": true, "values": true, "call": true, "insert": true,
	"update": true, "delete": true, "drop": true, "alter": true,
	"create": true, "truncate": true, "grant": true, "revoke": true,
	"set": true, "show": true, "describe": true, "explain": true,
	"merge": true, "upsert": true, "load": true, "handler": true,
	"straight_join": true,
}

// clause ranks, in the order SQL allows the parts of a SELECT.
const (
	rankSelect = iota
	rankFrom
	rankWhere
	rankGroupBy
	rankHaving
	rankOrderBy
	rankLimit
	rankOffset
)

var clauseNames = []string{"SELECT", "FROM", "WHERE", "GROUP BY", "HAVING", "ORDER BY", "LIMIT", "OFFSET"}

// boundary is where one clause of the statement begins.
type boundary struct {
	rank      int
	keyword   int // index of the keyword that introduced the clause
	bodyStart int // index of the clause's first token
}

// clauseKeyword maps a top-level keyword to the rank it belongs at and how
// many tokens it takes. A bare "group" or "order" is not a clause: it is only
// one when followed by "by".
func clauseKeyword(tokens []SQLToken, i int) (boundary, bool) {
	switch tokens[i].Lower {
	case "from":
		return boundary{rankFrom, i, i + 1}, true
	case "where":
		return boundary{rankWhere, i, i + 1}, true
	case "group":
		if nextIsWord(tokens, i+1, "by") {
			return boundary{rankGroupBy, i, i + 2}, true
		}
	case "having":
		return boundary{rankHaving, i, i + 1}, true
	case "order":
		if nextIsWord(tokens, i+1, "by") {
			return boundary{rankOrderBy, i, i + 2}, true
		}
	case "limit":
		return boundary{rankLimit, i, i + 1}, true
	case "offset":
		return boundary{rankOffset, i, i + 1}, true
	}
	return boundary{}, false
}

// SQLSplitSelect reads one single-table SELECT. It rejects anything that would
// have to be read as two queries, a query inside a query, or a write, and it
// does not guess at a clause boundary: a clause keyword is only honoured at
// depth zero and only in the order SQL allows.
func SQLSplitSelect(tokens []SQLToken) (*SQLStatementShape, error) {
	if len(tokens) == 0 || !tokens[0].IsWord("select") {
		return nil, errors.New("sqlscan: statement is not a SELECT")
	}

	// The select list has no keyword of its own to point at; it starts after
	// the SELECT token that SplitSelect already checked.
	bounds := []boundary{{rank: rankSelect, keyword: 0, bodyStart: 1}}
	depth := 0
	for i := 1; i < len(tokens); i++ {
		token := tokens[i]
		if token.IsPunct(";") {
			return nil, errors.New("sqlscan: semicolons are not allowed")
		}
		if token.IsPunct("(") {
			depth++
			continue
		}
		if token.IsPunct(")") {
			if depth--; depth < 0 {
				return nil, fmt.Errorf("sqlscan: unbalanced parentheses at token %d", i)
			}
			continue
		}
		if token.Kind != SQLWord {
			continue
		}
		if token.IsWord("select") || structuralWords[token.Lower] {
			return nil, fmt.Errorf("sqlscan: %q is not allowed in a table query", token.Lower)
		}
		if depth != 0 {
			continue
		}
		found, ok := clauseKeyword(tokens, i)
		if !ok {
			continue
		}
		if lastRank(bounds) >= found.rank {
			return nil, fmt.Errorf("sqlscan: %s appears twice or out of order", clauseNames[found.rank])
		}
		if found.rank > rankFrom && lastRank(bounds) < rankFrom {
			return nil, fmt.Errorf("sqlscan: %s appears before FROM", clauseNames[found.rank])
		}
		if found.rank == rankOffset && lastRank(bounds) != rankLimit {
			return nil, errors.New("sqlscan: OFFSET is only read after LIMIT")
		}
		bounds = append(bounds, found)
	}
	if depth != 0 {
		return nil, errors.New("sqlscan: unbalanced parentheses")
	}
	if lastRank(bounds) < rankFrom {
		return nil, errors.New("sqlscan: SELECT without FROM")
	}

	clauses := &SQLClauses{}
	for i, b := range bounds {
		end := len(tokens)
		if i+1 < len(bounds) {
			end = bounds[i+1].keyword
		}
		body := tokens[b.bodyStart:end]
		switch b.rank {
		case rankSelect:
			clauses.Select = body
		case rankFrom:
			clauses.From = body
		case rankWhere:
			clauses.Where = body
		case rankGroupBy:
			clauses.GroupBy = body
		case rankHaving:
			clauses.Having = body
		case rankOrderBy:
			clauses.OrderBy = body
		case rankLimit:
			clauses.Limit = body
		case rankOffset:
			clauses.Offset = body
		}
	}
	if len(clauses.Select) == 0 {
		return nil, errors.New("sqlscan: SELECT has no columns")
	}

	shape := &SQLStatementShape{Clauses: clauses}
	for _, body := range [][]SQLToken{clauses.Select, clauses.Having, clauses.OrderBy} {
		for i, token := range body {
			if token.Kind == SQLWord && aggregateFunctions[token.Lower] && nextIsPunct(body, i+1, "(") {
				shape.Aggregating = true
			}
		}
	}
	return shape, nil
}

// SQLTableReference reads a FROM clause as the one table it names, returning the
// dotted name lowercased. An alias, a comma, or any other word after the name
// is refused: the caller scopes a query to a table it chose itself, and a
// second table or a renamed one is how a check stops matching what runs.
func SQLTableReference(clause []SQLToken) (string, error) {
	if len(clause) == 0 {
		return "", errors.New("sqlscan: FROM has no table")
	}
	name := make([]string, 0, 3)
	isName := func(t SQLToken) bool { return t.Kind == SQLWord || t.Kind == SQLQuoted }
	if !isName(clause[0]) {
		return "", tableReferenceError(clause)
	}
	name = append(name, clause[0].Lower)
	for i := 1; i < len(clause); {
		if !clause[i].IsPunct(".") || i+1 >= len(clause) || !isName(clause[i+1]) {
			return "", tableReferenceError(clause)
		}
		name = append(name, clause[i+1].Lower)
		i += 2
	}
	if len(name) > 3 {
		return "", tableReferenceError(clause)
	}
	return strings.Join(name, "."), nil
}

func tableReferenceError(clause []SQLToken) error {
	return fmt.Errorf("sqlscan: FROM reads as %q, want one table name", SQLRender(clause, 0))
}

func nextIsPunct(tokens []SQLToken, i int, punct string) bool {
	t, ok := at(tokens, i)
	return ok && t.IsPunct(punct)
}

func lastRank(bounds []boundary) int {
	if len(bounds) == 0 {
		return rankSelect
	}
	return bounds[len(bounds)-1].rank
}

func at(tokens []SQLToken, i int) (SQLToken, bool) {
	if i < 0 || i >= len(tokens) {
		return SQLToken{}, false
	}
	return tokens[i], true
}

func nextIsWord(tokens []SQLToken, i int, word string) bool {
	t, ok := at(tokens, i)
	return ok && t.IsWord(word)
}

// SQLMatchingParen returns the index of the ")" that closes the "(" at open.
func SQLMatchingParen(tokens []SQLToken, open int) (int, error) {
	if open < 0 || open >= len(tokens) || !tokens[open].IsPunct("(") {
		return 0, fmt.Errorf("sqlscan: token %d is not an open parenthesis", open)
	}
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch {
		case tokens[i].IsPunct("("):
			depth++
		case tokens[i].IsPunct(")"):
			if depth--; depth == 0 {
				return i, nil
			}
		}
	}
	return 0, errors.New("sqlscan: unclosed parenthesis")
}

// SQLCallArguments splits the argument list of the call whose name sits at index
// name. It returns the comma-separated groups at the top level of the call and
// the index just past its closing parenthesis.
func SQLCallArguments(tokens []SQLToken, name int) ([][]SQLToken, int, error) {
	if name < 0 || name >= len(tokens) {
		return nil, 0, fmt.Errorf("sqlscan: invalid function index %d", name)
	}
	open := name + 1
	if open >= len(tokens) || !tokens[open].IsPunct("(") {
		return nil, 0, fmt.Errorf("sqlscan: %q is not called", tokens[name].Text)
	}
	end, err := SQLMatchingParen(tokens, open)
	if err != nil {
		return nil, 0, err
	}
	var args [][]SQLToken
	start, depth := open+1, 0
	for i := start; i < end; i++ {
		switch {
		case tokens[i].IsPunct("("):
			depth++
		case tokens[i].IsPunct(")"):
			depth--
		case tokens[i].IsPunct(",") && depth == 0:
			args = append(args, tokens[start:i])
			start = i + 1
		}
	}
	if start == end && len(args) == 0 {
		return args, end + 1, nil
	}
	return append(args, tokens[start:end]), end + 1, nil
}
