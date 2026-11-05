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

package serenedb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const runSQLDefaultLimit = 1024

type runSQLToken struct {
	text  string
	word  bool
	start int
}

var runSQLMutationWords = map[string]struct{}{
	"ALTER": {}, "ANALYZE": {}, "BEGIN": {}, "CALL": {}, "CHECKPOINT": {}, "CLUSTER": {}, "COMMENT": {},
	"COMMIT": {}, "COPY": {}, "CREATE": {}, "DEALLOCATE": {}, "DELETE": {}, "DISCARD": {}, "DO": {},
	"DROP": {}, "EXECUTE": {}, "EXPORT": {}, "GRANT": {}, "IMPORT": {}, "INSERT": {}, "INTO": {}, "LISTEN": {},
	"LOCK": {}, "MERGE": {}, "NOTIFY": {}, "PREPARE": {}, "REASSIGN": {}, "REINDEX": {}, "RELEASE": {},
	"REFRESH": {}, "RESET": {}, "REVOKE": {}, "ROLLBACK": {}, "SAVEPOINT": {}, "SET": {}, "START": {},
	"TRUNCATE": {}, "UNLISTEN": {}, "UPDATE": {}, "VACUUM": {},
}

// scanRunSQLTokens separates SQL words from quoted strings, quoted identifiers,
// and comments so command checks cannot be hidden in comments or string values.
func scanRunSQLTokens(sqlText string) ([]runSQLToken, error) {
	tokens := make([]runSQLToken, 0, len(sqlText)/4)
	for i := 0; i < len(sqlText); {
		c := sqlText[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' {
			i++
			continue
		}
		if c == '-' && i+1 < len(sqlText) && sqlText[i+1] == '-' {
			i += 2
			for i < len(sqlText) && sqlText[i] != '\n' && sqlText[i] != '\r' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sqlText) && sqlText[i+1] == '*' {
			commentStart, depth := i, 1
			i += 2
			for i < len(sqlText) && depth > 0 {
				switch {
				case i+1 < len(sqlText) && sqlText[i] == '/' && sqlText[i+1] == '*':
					depth++
					i += 2
				case i+1 < len(sqlText) && sqlText[i] == '*' && sqlText[i+1] == '/':
					depth--
					i += 2
				default:
					i++
				}
			}
			if depth != 0 {
				return nil, fmt.Errorf("unterminated SQL comment at byte %d", commentStart)
			}
			continue
		}
		if c == '\'' || c == '"' {
			quote, start := c, i
			backslashEscapes := quote == '\'' && i > 0 && (sqlText[i-1] == 'e' || sqlText[i-1] == 'E') &&
				(i == 1 || !isRunSQLWordByte(sqlText[i-2]))
			i++
			closed := false
			for i < len(sqlText) {
				if sqlText[i] == '\\' && i+1 < len(sqlText) {
					if backslashEscapes {
						i += 2
						continue
					}
					// Plain-string escaping depends on standard_conforming_strings; reject
					// ambiguous quote escapes rather than guessing the server setting.
					if quote == '\'' && sqlText[i+1] == '\'' {
						return nil, fmt.Errorf("ambiguous backslash escape in SQL string at byte %d", i)
					}
				}
				if sqlText[i] != quote {
					i++
					continue
				}
				if i+1 < len(sqlText) && sqlText[i+1] == quote {
					i += 2
					continue
				}
				i++
				closed = true
				break
			}
			if !closed {
				return nil, fmt.Errorf("unterminated SQL quote at byte %d", start)
			}
			tokens = append(tokens, runSQLToken{start: start})
			continue
		}
		if c == '$' {
			if delimiter, ok := runSQLDollarQuoteDelimiter(sqlText, i); ok {
				bodyStart := i + len(delimiter)
				closeOffset := strings.Index(sqlText[bodyStart:], delimiter)
				if closeOffset < 0 {
					return nil, fmt.Errorf("unterminated SQL dollar quote at byte %d", i)
				}
				i = bodyStart + closeOffset + len(delimiter)
				tokens = append(tokens, runSQLToken{start: bodyStart - len(delimiter)})
				continue
			}
		}
		if isRunSQLWordStart(c) {
			start := i
			i++
			for i < len(sqlText) && isRunSQLWordByte(sqlText[i]) {
				i++
			}
			tokens = append(tokens, runSQLToken{text: strings.ToUpper(sqlText[start:i]), word: true, start: start})
			continue
		}
		tokens = append(tokens, runSQLToken{text: string(c), start: i})
		i++
	}
	return tokens, nil
}

func runSQLDollarQuoteDelimiter(sqlText string, start int) (string, bool) {
	i := start + 1
	if i < len(sqlText) && sqlText[i] == '$' {
		return "$$", true
	}
	if i >= len(sqlText) || !isRunSQLWordStart(sqlText[i]) {
		return "", false
	}
	for i < len(sqlText) && (isRunSQLWordStart(sqlText[i]) || sqlText[i] >= '0' && sqlText[i] <= '9') {
		i++
	}
	if i >= len(sqlText) || sqlText[i] != '$' {
		return "", false
	}
	return sqlText[start : i+1], true
}

func isRunSQLWordStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isRunSQLWordByte(c byte) bool {
	return isRunSQLWordStart(c) || c >= '0' && c <= '9' || c == '$'
}

// boundRunSQL accepts one read-only SELECT statement, including SELECT CTEs,
// and appends a default row limit when the query has none. The lexical checks
// deliberately reject command keywords anywhere outside quoted data, which
// also blocks data-modifying CTEs.
func boundRunSQL(sqlText string, limit int) (string, error) {
	txt := strings.TrimSpace(sqlText)
	tokens, err := scanRunSQLTokens(txt)
	if err != nil {
		return "", err
	}
	if len(tokens) == 0 {
		return "", fmt.Errorf("only read-only SELECT statements are allowed")
	}
	if last := tokens[len(tokens)-1]; last.text == ";" {
		txt = strings.TrimSpace(txt[:last.start])
		tokens = tokens[:len(tokens)-1]
	}
	for _, token := range tokens {
		if token.text == ";" {
			return "", fmt.Errorf("multiple SQL statements are not allowed")
		}
	}
	if len(tokens) == 0 || !tokens[0].word || (tokens[0].text != "SELECT" && tokens[0].text != "WITH") {
		return "", fmt.Errorf("only read-only SELECT statements are allowed")
	}
	for _, token := range tokens {
		if token.word {
			if _, mutating := runSQLMutationWords[token.text]; mutating {
				return "", fmt.Errorf("only read-only SELECT statements are allowed")
			}
		}
	}
	if limit > 0 && !runSQLHasLimit(tokens) {
		txt = fmt.Sprintf("%s\nLIMIT %d", txt, limit)
	}
	return txt, nil
}

func runSQLHasLimit(tokens []runSQLToken) bool {
	for _, token := range tokens {
		if token.word && token.text == "LIMIT" {
			return true
		}
	}
	return false
}

// RunSQL executes a text-to-SQL query directly against SereneDB and returns the
// rows as maps. SereneDB speaks SQL natively, so unlike the Infinity engine no
// psql subprocess or field-alias rewrite is needed.
func (e *serenedbEngine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	query, err := boundRunSQL(sqlText, runSQLDefaultLimit)
	if err != nil {
		return nil, fmt.Errorf("serenedb: run sql: %w", err)
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("serenedb: run sql: begin read-only transaction: %w", err)
	}
	defer tx.Rollback()
	rows, err := queryMapsWith(ctx, tx, query)
	if err != nil {
		return nil, fmt.Errorf("serenedb: run sql: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("serenedb: run sql: commit read-only transaction: %w", err)
	}
	if rows == nil {
		rows = []map[string]interface{}{}
	}
	return rows, nil
}
