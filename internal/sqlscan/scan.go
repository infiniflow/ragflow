// Package sqlscan reads a SQL statement as tokens instead of as text.
//
// The chat SQL path has to decide what a model-generated statement will read
// before any backend runs it. String matching cannot answer that: the first
// "where" in a statement may be inside a literal, and a transform that
// collapses spaces or backticks globally rewrites the caller's data along with
// its syntax. Tokenizing makes the two separable, so a statement can be
// restricted to one table and a literal can keep its bytes.
//
// The scanner is deliberately strict. Anything it would have to guess at, an
// unterminated literal or a backslash escape whose dialect it cannot tell, is
// an error: a scanner that guesses where a literal ends is a scanner a
// generated statement can move the boundary of.
package sqlscan

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind is the syntactic class of a token.
type Kind uint8

const (
	// Word is a bare identifier or keyword: select, doc_id, count.
	Word Kind = iota
	// Quoted is a quoted identifier: "region" or `region`.
	Quoted
	// Number is a numeric literal.
	Number
	// String is a single-quoted literal.
	String
	// Punct is an operator or separator.
	Punct
)

// Token is one syntactic unit of a statement.
type Token struct {
	Kind Kind
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
func (t Token) IsWord(name string) bool { return t.Kind == Word && t.Lower == name }

// IsPunct reports an operator or separator.
func (t Token) IsPunct(text string) bool { return t.Kind == Punct && t.Lower == text }

// String renders the token for error messages. A literal's contents are left
// out: they are caller data and can be arbitrarily large.
func (t Token) String() string {
	switch t.Kind {
	case String:
		return "'...'"
	case Quoted:
		return t.Name
	default:
		return t.Text
	}
}

// multiCharPuncts are matched longest-first so "<=>" is not read as "<=>"
// through a "<=" prefix.
var multiCharPuncts = []string{"<=>", "<=", ">=", "<>", "!=", "||", "::"}

const singleCharPuncts = "(),.;+-*/%=<>."

var errComments = errors.New("sqlscan: comments are not allowed")

// Scan splits one statement into tokens. Comments, backslash escapes,
// placeholders and unterminated literals are rejected rather than interpreted.
func Scan(sql string) ([]Token, error) {
	src := []rune(sql)
	tokens := make([]Token, 0, len(src)/4+8)

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
			tokens = append(tokens, Token{Kind: Word, Text: text, Name: text, Lower: strings.ToLower(text)})
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

func scanString(src []rune, start int) (Token, int, error) {
	var value strings.Builder
	i := start + 1
	for i < len(src) {
		switch r := src[i]; r {
		case '\\':
			return Token{}, 0, fmt.Errorf("sqlscan: backslash escapes are not supported")
		case '\'':
			if i+1 < len(src) && src[i+1] == '\'' {
				value.WriteRune('\'')
				i += 2
				continue
			}
			i++
			text := string(src[start:i])
			return Token{Kind: String, Text: text, Value: value.String()}, i, nil
		default:
			if r < 0x20 || r == 0x7f {
				return Token{}, 0, fmt.Errorf("sqlscan: string literal contains a control character")
			}
			value.WriteRune(r)
			i++
		}
	}
	return Token{}, 0, fmt.Errorf("sqlscan: unterminated string literal")
}

func scanQuoted(src []rune, start int, quote rune) (Token, int, error) {
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
				return Token{}, 0, fmt.Errorf("sqlscan: empty quoted identifier")
			}
			return Token{
				Kind:  Quoted,
				Text:  string(src[start:i]),
				Name:  nameStr,
				Lower: strings.ToLower(nameStr),
			}, i, nil
		default:
			if src[i] < 0x20 || src[i] == 0x7f {
				return Token{}, 0, fmt.Errorf("sqlscan: quoted identifier contains a control character")
			}
			name.WriteRune(src[i])
			i++
		}
	}
	return Token{}, 0, fmt.Errorf("sqlscan: unterminated quoted identifier")
}

func scanNumber(src []rune, start int) (Token, int, error) {
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
		return Token{}, 0, fmt.Errorf("sqlscan: number is followed by an identifier")
	}
	text := string(src[start:i])
	return Token{Kind: Number, Text: text, Lower: text}, i, nil
}

func scanPunct(src []rune, i int) (Token, int, error) {
	for _, op := range multiCharPuncts {
		if hasPrefixRunes(src[i:], op) {
			return Token{Kind: Punct, Text: op, Lower: op}, i + len([]rune(op)), nil
		}
	}
	r := src[i]
	if strings.ContainsRune(singleCharPuncts, r) {
		return Token{Kind: Punct, Text: string(r), Lower: string(r)}, i + 1, nil
	}
	if r < 0x20 || r == 0x7f {
		return Token{}, 0, fmt.Errorf("sqlscan: control character %#x is not allowed", r)
	}
	return Token{}, 0, fmt.Errorf("sqlscan: %q is not a supported operator", string(r))
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

// Render writes tokens back as a statement whose tokens are separated by a
// single space. Both engines read that as the same syntax, and it means a
// checked statement cannot be re-split by whatever whitespace a caller typed.
//
// quote is the character a Quoted identifier is re-quoted with: '"' for
// Infinity and PostgreSQL, '`' for OceanBase and SeekDB. Zero keeps the
// source text, which is what a caller that only moves tokens needs.
func Render(tokens []Token, quote rune) string {
	var b strings.Builder
	for i, token := range tokens {
		// A separator only binds a name to its qualifier: "t.col", "a, b".
		// Everything else keeps its space, so no two tokens can merge into a
		// comment marker or an operator the caller did not write.
		if i > 0 && !token.IsPunct(",") && !token.IsPunct(".") && !tokens[i-1].IsPunct(".") {
			b.WriteByte(' ')
		}
		switch {
		case token.Kind == Quoted && quote != 0:
			escaped := strings.ReplaceAll(token.Name, string(quote), string(quote)+string(quote))
			b.WriteString(string(quote) + escaped + string(quote))
		default:
			b.WriteString(token.Text)
		}
	}
	return b.String()
}

// QuoteLiteral writes a value as a single-quoted literal. Callers use it for
// the identifiers the backend itself injects, so the statement stays one the
// scanner can read back.
func QuoteLiteral(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("sqlscan: %q is not valid UTF-8", value)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("sqlscan: literal contains a control character")
		}
	}
	return "'" + strings.ReplaceAll(value, "'", "''") + "'", nil
}
