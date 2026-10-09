package sqlscan

import (
	"errors"
	"fmt"
	"strings"
)

// Clauses are the top-level parts of a single-table SELECT, split at depth
// zero so a caller can reason about each one on its own. A part the statement
// did not contain is empty.
type Clauses struct {
	Select  []Token
	From    []Token
	Where   []Token
	GroupBy []Token
	Having  []Token
	OrderBy []Token
	Limit   []Token
	Offset  []Token
}

// StatementShape is a parsed single-table SELECT.
type StatementShape struct {
	Clauses *Clauses
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

// structuralWords cannot appear at the top level of the statement this package
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
func clauseKeyword(tokens []Token, i int) (boundary, bool) {
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

// SplitSelect reads one single-table SELECT. It rejects anything that would
// have to be read as two queries, a query inside a query, or a write, and it
// does not guess at a clause boundary: a clause keyword is only honoured at
// depth zero and only in the order SQL allows.
func SplitSelect(tokens []Token) (*StatementShape, error) {
	if len(tokens) == 0 || !tokens[0].IsWord("select") {
		return nil, errors.New("sqlscan: statement is not a SELECT")
	}

	// The select list has no keyword of its own to point at; it starts after
	// the SELECT token that SplitSelect already checked.
	bounds := []boundary{{rank: rankSelect, keyword: 0, bodyStart: 1}}
	depth := 0
	for i := 1; i < len(tokens); i++ {
		token := tokens[i]
		if token.IsPunct("(") {
			depth++
			if next, ok := at(tokens, i+1); ok && depth == 1 && (next.IsWord("select") || next.IsWord("with")) {
				return nil, fmt.Errorf("sqlscan: a query in parentheses at token %d is a subquery", i)
			}
			continue
		}
		if token.IsPunct(")") {
			if depth--; depth < 0 {
				return nil, fmt.Errorf("sqlscan: unbalanced parentheses at token %d", i)
			}
			continue
		}
		if depth != 0 || token.Kind != Word {
			continue
		}
		if structuralWords[token.Lower] {
			return nil, fmt.Errorf("sqlscan: %q is not allowed in a table query", token.Lower)
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

	clauses := &Clauses{}
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

	shape := &StatementShape{Clauses: clauses}
	for _, body := range [][]Token{clauses.Select, clauses.Having, clauses.OrderBy} {
		for i, token := range body {
			if token.Kind == Word && aggregateFunctions[token.Lower] && nextIsPunct(body, i+1, "(") {
				shape.Aggregating = true
			}
		}
	}
	return shape, nil
}

// TableReference reads a FROM clause as the one table it names, returning the
// dotted name lowercased. An alias, a comma, or any other word after the name
// is refused: the caller scopes a query to a table it chose itself, and a
// second table or a renamed one is how a check stops matching what runs.
func TableReference(clause []Token) (string, error) {
	if len(clause) == 0 {
		return "", errors.New("sqlscan: FROM has no table")
	}
	name := make([]string, 0, 3)
	isName := func(t Token) bool { return t.Kind == Word || t.Kind == Quoted }
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

func tableReferenceError(clause []Token) error {
	return fmt.Errorf("sqlscan: FROM reads as %q, want one table name", Render(clause, 0))
}

func nextIsPunct(tokens []Token, i int, punct string) bool {
	t, ok := at(tokens, i)
	return ok && t.IsPunct(punct)
}

func lastRank(bounds []boundary) int {
	if len(bounds) == 0 {
		return rankSelect
	}
	return bounds[len(bounds)-1].rank
}

func at(tokens []Token, i int) (Token, bool) {
	if i < 0 || i >= len(tokens) {
		return Token{}, false
	}
	return tokens[i], true
}

func nextIsWord(tokens []Token, i int, word string) bool {
	t, ok := at(tokens, i)
	return ok && t.IsWord(word)
}

// MatchingParen returns the index of the ")" that closes the "(" at open.
func MatchingParen(tokens []Token, open int) (int, error) {
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

// CallArguments splits the argument list of the call whose name sits at index
// name. It returns the comma-separated groups at the top level of the call and
// the index just past its closing parenthesis.
func CallArguments(tokens []Token, name int) ([][]Token, int, error) {
	open := name + 1
	if open >= len(tokens) || !tokens[open].IsPunct("(") {
		return nil, 0, fmt.Errorf("sqlscan: %q is not called", tokens[name].Text)
	}
	end, err := MatchingParen(tokens, open)
	if err != nil {
		return nil, 0, err
	}
	var args [][]Token
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
