//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

// Package sqllex recognizes comments and quoted text in SQL.
package sqllex

import (
	"slices"
	"strings"
)

// Dialect selects the quoting and comment rules of a database.
type Dialect int

const (
	other Dialect = iota
	MySQL
	PostgreSQL
	sqlServer
)

// TrimTerminator removes a semicolon only when it is the final SQL token,
// leaving trailing comments and semicolons inside quoted text intact.
// sessionMode is MySQL's sql_mode or PostgreSQL's standard_conforming_strings.
func (d Dialect) TrimTerminator(s, sessionMode string) string {
	if !strings.Contains(s, ";") {
		return s
	}
	rules := d.quoteRules(sessionMode)
	last := -1
	for i := 0; i < len(s); {
		if end, executable := d.commentEnd(s, i); end > i {
			if executable {
				last = i
			}
			i = end
			continue
		}
		if strings.IndexByte(" \t\n\r\f\v", s[i]) < 0 {
			last = i
		}
		if end := d.quotedEnd(s, i, rules); end > i {
			i = end
			continue
		}
		i++
	}
	if last >= 0 && s[last] == ';' {
		return s[:last] + s[last+1:]
	}
	return s
}

// MaskLiteralsAndComments replaces quoted text and comments with spaces for
// ExeSQL statement checks. Byte offsets are preserved. The flag reports a
// MySQL or MariaDB executable comment.
func MaskLiteralsAndComments(s, dbType string) (string, bool) {
	d := other
	switch strings.ToLower(dbType) {
	case "mysql", "mariadb", "oceanbase":
		d = MySQL
	case "postgres", "postgresql":
		d = PostgreSQL
	case "mssql", "sqlserver":
		d = sqlServer
	}
	rules := d.quoteRules("")
	masked := []byte(s)
	executableComment := false
	for i := 0; i < len(s); {
		end, executable := d.commentEnd(s, i)
		if end == i {
			switch {
			case s[i] == '`':
				// ExeSQL masks backtick-quoted text for every driver.
				end = scanSQLQuotedText(s, i, '`', d == MySQL)
			case d == sqlServer && s[i] == '[':
				end = scanSQLQuotedText(s, i, ']', false)
			default:
				end = d.quotedEnd(s, i, rules)
			}
		}
		if end == i {
			i++
			continue
		}
		executableComment = executableComment || executable
		for ; i < end; i++ {
			masked[i] = ' '
		}
	}
	return string(masked), executableComment
}

// commentEnd returns the byte after a comment, or start for ordinary SQL.
// The flag distinguishes MySQL executable comments from ordinary comments.
func (d Dialect) commentEnd(s string, start int) (int, bool) {
	rest := s[start:]
	if strings.HasPrefix(rest, "/*") {
		executable := d == MySQL && (strings.HasPrefix(rest, "/*!") || strings.HasPrefix(rest, "/*M!"))
		return blockCommentEnd(s, start, d == PostgreSQL), executable
	}
	dashComment := strings.HasPrefix(rest, "--") && (d != MySQL || len(rest) == 2 || rest[2] <= ' ')
	if dashComment || d == MySQL && rest[0] == '#' {
		if end := strings.IndexAny(rest, "\r\n"); end >= 0 {
			return start + end, false
		}
		return len(s), false
	}
	return start, false
}

type quoteRules struct {
	singleEscapes bool
	doubleEscapes bool
}

func (d Dialect) quoteRules(mode string) quoteRules {
	if d == MySQL {
		modes := strings.Split(mode, ",")
		backslash := !slices.Contains(modes, "NO_BACKSLASH_ESCAPES")
		return quoteRules{singleEscapes: backslash, doubleEscapes: backslash && !slices.Contains(modes, "ANSI_QUOTES")}
	}
	return quoteRules{singleEscapes: d == PostgreSQL && mode == "off"}
}

// quotedEnd returns the byte after a string or quoted identifier, or start.
func (d Dialect) quotedEnd(s string, start int, rules quoteRules) int {
	switch s[start] {
	case '\'':
		return scanSQLQuotedText(s, start, '\'', rules.singleEscapes || d == PostgreSQL && isPostgresEscapeString(s, start))
	case '"':
		return scanSQLQuotedText(s, start, '"', rules.doubleEscapes)
	case '`':
		if d == MySQL {
			return scanSQLQuotedText(s, start, '`', false)
		}
	case '$':
		if d != PostgreSQL {
			return start
		}
		if delimiter := postgresDollarQuoteDelimiter(s, start); delimiter != "" {
			body := start + len(delimiter)
			if n := strings.Index(s[body:], delimiter); n >= 0 {
				return body + n + len(delimiter)
			}
			return len(s)
		}
	}
	return start
}

func blockCommentEnd(s string, start int, nested bool) int {
	depth := 1
	for i := start + 2; i < len(s); {
		if nested && i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			depth++
			i += 2
			continue
		}
		if i+1 < len(s) && s[i] == '*' && s[i+1] == '/' {
			depth--
			i += 2
			if depth == 0 {
				return i
			}
			continue
		}
		i++
	}
	return len(s)
}

func scanSQLQuotedText(s string, start int, quote byte, backslashEscapes bool) int {
	for i := start + 1; i < len(s); {
		if backslashEscapes && s[i] == '\\' && i+1 < len(s) {
			i += 2
			continue
		}
		if s[i] != quote {
			i++
			continue
		}
		if i+1 < len(s) && s[i+1] == quote {
			i += 2
			continue
		}
		return i + 1
	}
	return len(s)
}

func isPostgresEscapeString(s string, quoteAt int) bool {
	if quoteAt == 0 || s[quoteAt-1] != 'E' && s[quoteAt-1] != 'e' {
		return false
	}
	return quoteAt == 1 || !isSQLIdentifierByte(s[quoteAt-2])
}

func postgresDollarQuoteDelimiter(s string, start int) string {
	if start > 0 && isSQLIdentifierByte(s[start-1]) {
		return ""
	}
	for i := start + 1; i < len(s); i++ {
		if s[i] == '$' {
			return s[start : i+1]
		}
		if i == start+1 {
			if !isSQLIdentifierStartByte(s[i]) {
				return ""
			}
			continue
		}
		if !isSQLIdentifierByte(s[i]) {
			return ""
		}
	}
	return ""
}

func isSQLIdentifierStartByte(b byte) bool {
	return b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= 0x80
}

func isSQLIdentifierByte(b byte) bool {
	return isSQLIdentifierStartByte(b) || b >= '0' && b <= '9' || b == '$'
}
