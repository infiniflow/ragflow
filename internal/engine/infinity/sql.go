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

package infinity

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/utility"
)

const (
	sqlTimeout     = 10 * time.Second
	defaultSQLHost = "infinity"
	defaultSQLPort = "5432"
)

// fieldMappingEntry is one entry in infinity_mapping.json.
type fieldMappingEntry struct {
	Type    string `json:"type"`
	Comment string `json:"comment"`
}

// loadFieldMapping reads field aliases from infinity_mapping.json.
// The configured mapping file must exist.
func loadFieldMapping(mappingFileName string) (aliasToActual map[string]string, err error) {
	if mappingFileName == "" {
		mappingFileName = "infinity_mapping.json"
	}

	filePath, err := utility.FindConfFileInProject(mappingFileName)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(*filePath)
	if err != nil {
		return nil, fmt.Errorf("load field mapping %q: %w", *filePath, err)
	}

	fields := map[string]fieldMappingEntry{}
	if err = json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("parse field mapping %q: %w", *filePath, err)
	}

	aliasToActual = make(map[string]string, len(fields)*2)
	for actual, info := range fields {
		if info.Comment == "" {
			continue
		}
		for raw := range strings.SplitSeq(info.Comment, ",") {
			alias := strings.TrimSpace(raw)
			if alias == "" {
				continue
			}
			aliasToActual[alias] = actual
		}
	}
	return aliasToActual, nil
}

// prepareSQL rewrites field identifiers while preserving string literals and
// result labels. It only transforms the expressions of one single-table SELECT.
func prepareSQL(sqlText string, aliasToActual map[string]string) (string, error) {
	tokens, err := utility.SQLScan(strings.TrimSuffix(strings.TrimSpace(sqlText), ";"))
	if err != nil {
		return "", err
	}
	shape, err := utility.SQLSplitSelect(tokens)
	if err != nil {
		return "", err
	}
	return rewriteSQL(tokens, shape.Clauses, aliasToActual)
}

// rewriteSQL uses the parsed clauses shared by the result and numeric checks.
func rewriteSQL(tokens []utility.SQLToken, clauses *utility.SQLClauses, aliasToActual map[string]string) (string, error) {
	// Infinity cannot safely bind CASE expressions in the table SQL path.
	for _, token := range tokens {
		if token.IsWord("case") {
			return "", fmt.Errorf("Infinity table SQL does not support CASE expressions")
		}
	}
	if _, err := utility.SQLTableReference(clauses.From); err != nil {
		return "", err
	}
	for _, expression := range [][]utility.SQLToken{clauses.Select, clauses.Where, clauses.GroupBy, clauses.Having, clauses.OrderBy} {
		for i, token := range expression {
			if token.Kind != utility.SQLWord && token.Kind != utility.SQLQuoted {
				continue
			}
			if i > 0 && expression[i-1].IsWord("as") {
				continue
			}
			if i+1 < len(expression) && (expression[i+1].IsPunct("(") || expression[i+1].IsPunct(".")) {
				continue
			}
			actual, ok := aliasToActual[token.Lower]
			if !ok {
				continue
			}
			expression[i].Text = actual
			expression[i].Name = actual
			expression[i].Lower = strings.ToLower(actual)
		}
	}
	if len(clauses.GroupBy) == 0 && len(clauses.OrderBy) == 0 {
		return utility.SQLRender(tokens, '"'), nil
	}
	needsProjection := false
	for _, body := range [][]utility.SQLToken{clauses.Select, clauses.GroupBy, clauses.Having, clauses.OrderBy} {
		for i, token := range body {
			if i+1 < len(body) && body[i+1].IsPunct("(") && (token.IsWord("cast") || token.IsWord("json_extract_string") || token.IsWord("json_extract") || token.IsWord("json_value")) {
				needsProjection = true
			}
		}
	}
	if !needsProjection {
		return utility.SQLRender(tokens, '"'), nil
	}
	return projectSQLExpressions(clauses)
}

// projectSQLExpressions makes JSON and cast expressions physical input columns
// for Infinity's grouping and ordering binders. The original WHERE stays inside
// the projection, so the query's document range is unchanged.
func projectSQLExpressions(c *utility.SQLClauses) (string, error) {
	var projections []string
	names := map[string]string{}
	derivedNames := map[string]bool{}
	occupied := map[string]bool{}
	for _, body := range [][]utility.SQLToken{c.Select, c.Where, c.GroupBy, c.Having, c.OrderBy} {
		for _, token := range body {
			occupied[token.Lower] = true
		}
	}
	newName := func() string {
		for i := len(projections); ; i++ {
			name := fmt.Sprintf("table_expr_%d", i)
			if !occupied[name] {
				occupied[name] = true
				return name
			}
		}
	}
	countColumn := ""
	for _, clause := range []*[]utility.SQLToken{&c.Select, &c.GroupBy, &c.Having, &c.OrderBy} {
		var rewritten []utility.SQLToken
		for i := 0; i < len(*clause); {
			token := (*clause)[i]
			// Native columns are now read from the derived table. Remove only
			// the original, already checked table qualifier from outer references.
			if i+len(c.From) < len(*clause) && (*clause)[i+len(c.From)].IsPunct(".") {
				matches := true
				for j, part := range c.From {
					if (*clause)[i+j].Lower != part.Lower {
						matches = false
						break
					}
				}
				if matches {
					i += len(c.From) + 1
					continue
				}
			}
			// Infinity's COUNT(*) on a derived table can crash the server.
			// Counting a non-null constant column has the same row semantics.
			if token.IsWord("count") && i+3 < len(*clause) && (*clause)[i+1].IsPunct("(") && (*clause)[i+2].IsPunct("*") && (*clause)[i+3].IsPunct(")") {
				if countColumn == "" {
					countColumn = newName()
					projections = append(projections, "1 AS "+countColumn)
					derivedNames[countColumn] = true
				}
				replacement, err := utility.SQLScan("COUNT(" + countColumn + ")")
				if err != nil {
					return "", err
				}
				rewritten = append(rewritten, replacement...)
				i += 4
				continue
			}
			scalar := token.IsWord("cast") || token.IsWord("json_extract_string") || token.IsWord("json_extract") || token.IsWord("json_value")
			if !scalar || i+1 >= len(*clause) || !(*clause)[i+1].IsPunct("(") {
				rewritten = append(rewritten, token)
				i++
				continue
			}
			_, end, err := utility.SQLCallArguments(*clause, i)
			if err != nil {
				return "", err
			}
			expression := utility.SQLRender((*clause)[i:end], '"')
			name, ok := names[expression]
			if !ok {
				name = newName()
				if clause == &c.Select && end+1 < len(*clause) && (*clause)[end].IsWord("as") {
					name = utility.SQLRender((*clause)[end+1:end+2], '"')
				}
				names[expression] = name
				projections = append(projections, expression+" AS "+name)
			}
			word, err := utility.SQLScan(name)
			if err != nil {
				return "", err
			}
			derivedNames[word[0].Lower] = true
			rewritten = append(rewritten, word...)
			i = end
			if clause == &c.Select && end+1 < len(*clause) && (*clause)[end].IsWord("as") && utility.SQLRender((*clause)[end+1:end+2], '"') == name {
				i += 2
			}
		}
		*clause = rewritten
	}
	from := utility.SQLRender(c.From, '"')
	where := utility.SQLRender(c.Where, '"')
	// Include only native columns still used by the outer expressions. Infinity
	// cannot reliably expand a star alongside computed fields in a derived table.
	aliases := map[string]bool{}
	depth := 0
	for i, token := range c.Select {
		if token.IsPunct("(") {
			depth++
		} else if token.IsPunct(")") {
			depth--
		} else if depth == 0 && token.IsWord("as") && i+1 < len(c.Select) {
			aliases[c.Select[i+1].Lower] = true
		}
	}
	inputs := map[string]bool{}
	for clauseIndex, body := range [][]utility.SQLToken{c.Select, c.GroupBy, c.Having, c.OrderBy} {
		for i, token := range body {
			if token.Kind != utility.SQLWord && token.Kind != utility.SQLQuoted {
				continue
			}
			// HAVING and ORDER BY may refer to output aliases. SELECT inputs
			// remain physical columns even when an alias has the same name.
			if clauseIndex >= 2 && aliases[token.Lower] {
				continue
			}
			if derivedNames[token.Lower] || (i > 0 && body[i-1].IsWord("as")) || (i+1 < len(body) && body[i+1].IsPunct("(")) {
				continue
			}
			switch token.Lower {
			case "as", "distinct", "all", "asc", "desc", "nulls", "first", "last", "and", "or", "not", "is", "null", "true", "false", "in", "like", "between":
				continue
			}
			if !inputs[token.Text] {
				projections = append(projections, utility.SQLRender([]utility.SQLToken{token}, '"'))
				inputs[token.Text] = true
			}
		}
	}
	parts := []string{"SELECT " + utility.SQLRender(c.Select, '"')}
	if len(projections) > 0 {
		inner := "SELECT " + strings.Join(projections, ", ") + " FROM " + from
		if where != "" {
			inner += " WHERE " + where
		}
		parts = append(parts, "FROM ("+inner+") AS table_projection")
	} else {
		parts = append(parts, "FROM "+from)
		if where != "" {
			parts = append(parts, "WHERE "+where)
		}
	}
	for _, clause := range []struct {
		name string
		body []utility.SQLToken
	}{
		{"GROUP BY", c.GroupBy}, {"HAVING", c.Having}, {"ORDER BY", c.OrderBy}, {"LIMIT", c.Limit}, {"OFFSET", c.Offset},
	} {
		if len(clause.body) > 0 {
			parts = append(parts, clause.name+" "+utility.SQLRender(clause.body, '"'))
		}
	}
	return strings.Join(parts, " "), nil
}

// runSQL reads PostgreSQL wire results without converting them to display text.
func runSQL(ctx context.Context, host, port, database, sql string, jsonColumns map[int]bool, checks []numericSQLCheck) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(ctx, sqlTimeout)
	defer cancel()
	cfg, err := pgconn.ParseConfig("postgres://postgres@localhost/default_db?sslmode=disable")
	if err != nil {
		return nil, err
	}
	cfg.Database = database
	cfg.Host = host
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return nil, err
	}
	cfg.Port = uint16(n)
	cfg.RuntimeParams = map[string]string{}
	conn, err := pgconn.ConnectConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("infinity SQL connection: %w", err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		conn.Close(cleanup)
	}()
	// Infinity does not consume the database field in PostgreSQL startup.
	// Select the configured database explicitly on this connection.
	databaseSQL := utility.SQLRender([]utility.SQLToken{{Kind: utility.SQLQuoted, Name: database}}, '"')
	if _, err := conn.Exec(ctx, "USE "+databaseSQL).ReadAll(); err != nil {
		return nil, fmt.Errorf("infinity SQL database: %w", err)
	}
	for _, check := range checks {
		reader := conn.Exec(ctx, check.query)
		if !reader.NextResult() {
			if err := reader.Close(); err != nil {
				return nil, err
			}
			continue
		}
		result := reader.ResultReader()
		for result.NextRow() {
			cells := result.Values()
			if cells[0] == nil || (len(cells) > 1 && (cells[1] == nil || string(cells[1]) == "null")) {
				continue
			}
			value := string(cells[0])
			var err error
			if check.bits != 0 {
				_, err = strconv.ParseInt(value, 10, check.bits)
			} else {
				var number float64
				number, err = strconv.ParseFloat(value, check.floatBits)
				if math.IsNaN(number) || math.IsInf(number, 0) {
					err = fmt.Errorf("non-finite number")
				}
			}
			if err != nil {
				return nil, fmt.Errorf("infinity SQL: invalid numeric value %q", value)
			}
		}
		if err := reader.Close(); err != nil {
			return nil, fmt.Errorf("infinity numeric check: %w", err)
		}
	}
	results, err := conn.Exec(ctx, sql).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("infinity SQL: %w", err)
	}
	if len(results) != 1 {
		return nil, fmt.Errorf("infinity SQL: expected one result, got %d", len(results))
	}
	rows := toRowMaps(results[0])
	// Infinity omits the row description when a SELECT has no matches.
	if len(rows) == 0 {
		return rows, nil
	}
	for i := range jsonColumns {
		if i >= len(results[0].FieldDescriptions) {
			return nil, fmt.Errorf("infinity SQL: missing result column %d", i)
		}
		name := results[0].FieldDescriptions[i].Name
		for _, row := range rows {
			if text, ok := row[name].(string); ok {
				var value interface{}
				if err := json.Unmarshal([]byte(text), &value); err != nil {
					return nil, fmt.Errorf("infinity JSON result: %w", err)
				}
				row[name] = value
			}
		}
	}
	return rows, nil
}

type numericSQLCheck struct {
	query           string
	bits, floatBits int
}

// numericSQLChecks validates original values before Infinity's permissive
// casts can turn malformed input into zero. Checks read the same WHERE range,
// stream distinct values, and share the statement's timeout and connection.
func numericSQLChecks(tokens []utility.SQLToken, clauses *utility.SQLClauses, aliases map[string]string) ([]numericSQLCheck, error) {
	var checks []numericSQLCheck
	seen := map[string]bool{}
	for i, token := range tokens {
		if token.IsWord("try_cast") {
			return nil, fmt.Errorf("Infinity table SQL does not support TRY_CAST")
		}
		if !token.IsWord("cast") || i+1 >= len(tokens) || !tokens[i+1].IsPunct("(") {
			continue
		}
		args, _, err := utility.SQLCallArguments(tokens, i)
		if err != nil {
			return nil, err
		}
		if len(args) != 1 {
			return nil, fmt.Errorf("invalid CAST")
		}
		body := args[0]
		depth, at := 0, -1
		for j, token := range body {
			if token.IsPunct("(") {
				depth++
			}
			if token.IsPunct(")") {
				depth--
			}
			if depth == 0 && token.IsWord("as") {
				at = j
				break
			}
		}
		if at < 1 || at+2 != len(body) {
			return nil, fmt.Errorf("Infinity table SQL requires a supported scalar CAST type")
		}
		check := numericSQLCheck{floatBits: 64}
		switch body[at+1].Lower {
		case "integer", "int":
			check.bits = 32
		case "bigint":
			check.bits = 64
		case "smallint":
			check.bits = 16
		case "tinyint":
			check.bits = 8
		case "float", "real":
			check.floatBits = 32
		case "double":
		default:
			return nil, fmt.Errorf("Infinity table SQL does not support CAST to %s", body[at+1].Text)
		}
		operand := body[:at]
		projection := utility.SQLRender(operand, '"') + " AS cast_value"
		if operand[0].IsWord("json_extract_string") {
			raw := append([]utility.SQLToken(nil), operand...)
			raw[0].Text, raw[0].Lower = "json_extract", "json_extract"
			projection += ", " + utility.SQLRender(raw, '"') + " AS raw_value"
		}
		query := "SELECT DISTINCT " + projection + " FROM " + utility.SQLRender(clauses.From, '"')
		if len(clauses.Where) > 0 {
			query += " WHERE " + utility.SQLRender(clauses.Where, '"')
		}
		check.query, err = prepareSQL(query, aliases)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%d:%d:%s", check.bits, check.floatBits, check.query)
		if !seen[key] {
			checks = append(checks, check)
			seen[key] = true
		}
	}
	return checks, nil
}

// prepareJSONResults reads raw JSON for standalone string projections. This
// preserves the distinction between JSON null and the ordinary string "null".
func prepareJSONResults(tokens []utility.SQLToken) ([]utility.SQLToken, *utility.SQLClauses, map[int]bool, error) {
	// A missing path makes Infinity's isnull return SQL NULL. Both a missing
	// path and an explicit JSON null must satisfy the caller's null check.
	var normalized []utility.SQLToken
	for i := 0; i < len(tokens); {
		if tokens[i].IsWord("json_extract_isnull") && i+1 < len(tokens) && tokens[i+1].IsPunct("(") {
			_, end, err := utility.SQLCallArguments(tokens, i)
			if err != nil {
				return nil, nil, nil, err
			}
			call := utility.SQLRender(tokens[i:end], '"')
			replacement, err := utility.SQLScan("(" + call + " IS NULL OR " + call + ")")
			if err != nil {
				return nil, nil, nil, err
			}
			normalized = append(normalized, replacement...)
			i = end
		} else {
			normalized = append(normalized, tokens[i])
			i++
		}
	}
	shape, err := utility.SQLSplitSelect(normalized)
	if err != nil {
		return nil, nil, nil, err
	}
	columns := map[int]bool{}
	expressions := map[string]bool{}
	selectList := shape.Clauses.Select
	if len(selectList) > 0 && (selectList[0].IsWord("distinct") || selectList[0].IsWord("all")) {
		selectList = selectList[1:]
	}
	start, depth, column := 0, 0, 0
	for i := 0; i <= len(selectList); i++ {
		if i < len(selectList) {
			if selectList[i].IsPunct("(") {
				depth++
			}
			if selectList[i].IsPunct(")") {
				depth--
			}
			if !selectList[i].IsPunct(",") || depth != 0 {
				continue
			}
		}
		item := selectList[start:i]
		if len(item) > 1 && item[0].IsWord("json_extract_string") {
			_, end, err := utility.SQLCallArguments(item, 0)
			if err != nil {
				return nil, nil, nil, err
			}
			if end == len(item) || (end+2 == len(item) && item[end].IsWord("as")) {
				expressions[utility.SQLRender(item[:end], '"')] = true
				item[0].Text, item[0].Lower = "json_extract", "json_extract"
				columns[column] = true
			}
		}
		start, column = i+1, column+1
	}
	// Keep bare grouping and ordering expressions aligned with their raw JSON
	// projections, including standalone ORDER BY. Nested numeric casts must keep
	// extracting strings: casting a quoted JSON value changes the number.
	for _, clause := range [][]utility.SQLToken{shape.Clauses.GroupBy, shape.Clauses.OrderBy} {
		for i := 0; i < len(clause); i++ {
			if !clause[i].IsWord("json_extract_string") || (i > 0 && !clause[i-1].IsPunct(",")) {
				continue
			}
			_, end, err := utility.SQLCallArguments(clause, i)
			if err != nil {
				return nil, nil, nil, err
			}
			if expressions[utility.SQLRender(clause[i:end], '"')] {
				clause[i].Text, clause[i].Lower = "json_extract", "json_extract"
			}
			i = end - 1
		}
	}
	return normalized, shape.Clauses, columns, nil
}

func toRowMaps(res *pgconn.Result) []map[string]interface{} {
	if res == nil {
		return nil
	}
	var rows []map[string]interface{}
	for _, cells := range res.Rows {
		row := make(map[string]interface{}, len(res.FieldDescriptions))
		for i, field := range res.FieldDescriptions {
			if cells[i] == nil {
				row[field.Name] = nil
			} else {
				row[field.Name] = string(cells[i])
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func resolveSQLHostPort(hostURI string, postgresPort int) (host, port string) {
	host = defaultSQLHost
	port = defaultSQLPort
	if postgresPort > 0 {
		port = strconv.Itoa(postgresPort)
	}
	if hostURI != "" {
		if h, _, ok := strings.Cut(hostURI, ":"); ok && h != "" {
			host = h
		}
	}
	return host, port
}

// RunSQL implements the SQL retrieval path: rewrite identifiers,
// read structured PostgreSQL wire results.
func (e *Engine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, _ string) ([]map[string]interface{}, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("infinity RunSQL: client not initialized")
	}
	sqlText = strings.TrimSpace(sqlText)
	if sqlText == "" {
		return nil, fmt.Errorf("infinity RunSQL: empty SQL")
	}

	common.Debug("InfinityConnection.sql get sql", zap.String("sql", sqlText))

	aliasMap, err := loadFieldMapping(e.client.mappingFileName)
	if err != nil {
		return nil, fmt.Errorf("infinity RunSQL: %w", err)
	}
	tokens, err := utility.SQLScan(sqlText)
	if err != nil {
		return nil, fmt.Errorf("infinity RunSQL: %w", err)
	}
	tokens, clauses, jsonColumns, err := prepareJSONResults(tokens)
	if err != nil {
		return nil, fmt.Errorf("infinity RunSQL: %w", err)
	}
	checks, err := numericSQLChecks(tokens, clauses, aliasMap)
	if err != nil {
		return nil, fmt.Errorf("infinity RunSQL: %w", err)
	}
	sqlText, err = rewriteSQL(tokens, clauses, aliasMap)
	if err != nil {
		return nil, fmt.Errorf("infinity RunSQL: %w", err)
	}

	common.Debug("InfinityConnection.sql to execute", zap.String("sql", sqlText))

	host, port := resolveSQLHostPort(e.client.hostURI, e.client.postgresPort)
	rows, err := runSQL(ctx, host, port, e.client.dbName, sqlText, jsonColumns, checks)
	if err != nil {
		return nil, err
	}

	return rows, nil
}
