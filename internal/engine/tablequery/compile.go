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

package tablequery

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"ragflow/internal/tokenizer"
)

// Dialect identifies the target SQL engine dialect for compilation.
type Dialect string

const (
	DialectElasticsearch Dialect = "elasticsearch"
	DialectOpenSearch    Dialect = "opensearch"
	DialectOceanBase     Dialect = "oceanbase"
	DialectInfinity      Dialect = "infinity"
	DialectVastbase      Dialect = "vastbase"
	DialectSereneDB      Dialect = "serenedb"
)

// CompiledSQL holds the deterministic SQL text and optional positional bind
// arguments produced by Compile.
type CompiledSQL struct {
	SQL  string
	Args []any
}

// CompileOptions allows tests or callers to override tokenization hooks without
// changing production behavior.
type CompileOptions struct {
	// TokenizeForMatch converts a raw search string into ES/OpenSearch MATCH()
	// fine-grained tokens. Nil uses ragflow/internal/tokenizer.
	TokenizeForMatch func(string) (string, error)
}

// Compile translates a ValidatedQuery into a target engine's native read-only
// SQL statement with authoritative table and dataset isolation baked in.
func Compile(vq *ValidatedQuery, dialect Dialect) (CompiledSQL, error) {
	return CompileWithOptions(vq, dialect, CompileOptions{})
}

// CompileWithOptions behaves like Compile with optional compilation hooks.
func CompileWithOptions(vq *ValidatedQuery, dialect Dialect, opts CompileOptions) (CompiledSQL, error) {
	if vq == nil {
		return CompiledSQL{}, fmt.Errorf("tablequery: nil validated query")
	}
	if err := ValidateScope(vq.scope); err != nil {
		return CompiledSQL{}, err
	}
	c := &compiler{
		vq:      vq,
		dialect: dialect,
		opts:    opts,
	}
	return c.compile()
}

type compiler struct {
	vq      *ValidatedQuery
	dialect Dialect
	opts    CompileOptions
	args    []any
}

func (c *compiler) compile() (CompiledSQL, error) {
	if c.dialect == DialectInfinity && len(c.vq.scope.KBIDs) != 1 {
		return CompiledSQL{}, fmt.Errorf("tablequery: infinity requires exactly one authorized kb_id, got %d", len(c.vq.scope.KBIDs))
	}

	tableExpr, err := c.quoteTable(c.vq.scope.TableName)
	if err != nil {
		return CompiledSQL{}, err
	}

	selectParts, err := c.compileSelectList()
	if err != nil {
		return CompiledSQL{}, err
	}

	var b strings.Builder
	b.WriteString("SELECT ")
	if c.vq.query.Distinct && !c.vq.citationOnly {
		b.WriteString("DISTINCT ")
	}
	b.WriteString(strings.Join(selectParts, ", "))
	b.WriteString(" FROM ")
	b.WriteString(tableExpr)

	whereSQL, err := c.compileScopedWhere()
	if err != nil {
		return CompiledSQL{}, err
	}
	if whereSQL != "" {
		b.WriteString(" WHERE ")
		b.WriteString(whereSQL)
	}

	if len(c.vq.query.GroupBy) > 0 && !c.vq.citationOnly {
		groupParts := make([]string, 0, len(c.vq.query.GroupBy))
		for _, field := range c.vq.query.GroupBy {
			expr, err := c.fieldExpr(field, false)
			if err != nil {
				return CompiledSQL{}, err
			}
			groupParts = append(groupParts, expr)
		}
		b.WriteString(" GROUP BY ")
		b.WriteString(strings.Join(groupParts, ", "))
	}

	if c.vq.query.Having != nil && !c.vq.citationOnly {
		havingSQL, err := c.compileFilter(c.vq.query.Having, true)
		if err != nil {
			return CompiledSQL{}, err
		}
		b.WriteString(" HAVING ")
		b.WriteString(havingSQL)
	}

	if len(c.vq.query.OrderBy) > 0 && !c.vq.citationOnly {
		orderParts := make([]string, 0, len(c.vq.query.OrderBy))
		for _, o := range c.vq.query.OrderBy {
			expr, err := c.orderExpr(o.Field)
			if err != nil {
				return CompiledSQL{}, err
			}
			if o.Desc {
				expr += " DESC"
			} else {
				expr += " ASC"
			}
			orderParts = append(orderParts, expr)
		}
		b.WriteString(" ORDER BY ")
		b.WriteString(strings.Join(orderParts, ", "))
	}

	limit := c.vq.query.Limit
	if limit <= 0 || limit > c.vq.scope.MaxLimit {
		limit = c.vq.scope.MaxLimit
	}
	b.WriteString(" LIMIT ")
	b.WriteString(strconv.Itoa(limit))
	if c.vq.query.Offset > 0 && !c.vq.citationOnly {
		b.WriteString(" OFFSET ")
		b.WriteString(strconv.Itoa(c.vq.query.Offset))
	}

	return CompiledSQL{SQL: b.String(), Args: c.args}, nil
}

func (c *compiler) compileSelectList() ([]string, error) {
	parts := make([]string, 0, len(c.vq.query.Select)+len(c.vq.query.Aggregates)+3)

	if c.vq.includeCitations {
		docIDCol, err := c.systemColumn("doc_id")
		if err != nil {
			return nil, err
		}
		docNameCol, err := c.systemColumn(c.docNameColumn())
		if err != nil {
			return nil, err
		}
		parts = append(parts, docIDCol, docNameCol)
		if len(c.vq.scope.KBIDs) > 1 && c.dialect != DialectInfinity {
			kbCol, err := c.systemColumn("kb_id")
			if err != nil {
				return nil, err
			}
			parts = append(parts, kbCol)
		}
	}

	if c.vq.citationOnly {
		return parts, nil
	}

	for _, field := range c.vq.query.Select {
		expr, err := c.fieldExpr(field, false)
		if err != nil {
			return nil, err
		}
		if c.needsExplicitAlias(field) {
			parts = append(parts, expr+" AS "+c.quoteAlias(field))
		} else {
			parts = append(parts, expr)
		}
	}

	for _, agg := range c.vq.query.Aggregates {
		expr, err := c.aggregateExpr(agg)
		if err != nil {
			return nil, err
		}
		parts = append(parts, expr+" AS "+c.quoteAlias(agg.As))
	}

	return parts, nil
}

func (c *compiler) compileScopedWhere() (string, error) {
	var userWhere string
	if c.vq.query.Where != nil {
		w, err := c.compileFilter(c.vq.query.Where, false)
		if err != nil {
			return "", err
		}
		userWhere = w
	}

	// Infinity isolates datasets by per-dataset table name (verified to hold
	// exactly one authorized kb_id). All shared-table engines conjoin the
	// authorized kb_id predicate unconditionally.
	if c.dialect == DialectInfinity {
		return userWhere, nil
	}

	scopePred, err := c.compileKBScopePredicate()
	if err != nil {
		return "", err
	}
	if userWhere == "" {
		return scopePred, nil
	}
	return "(" + userWhere + ") AND (" + scopePred + ")", nil
}

func (c *compiler) compileKBScopePredicate() (string, error) {
	kbCol, err := c.systemColumn("kb_id")
	if err != nil {
		return "", err
	}
	ids := c.vq.scope.KBIDs
	if len(ids) == 1 {
		return kbCol + " = " + c.bindValue(ids[0]), nil
	}
	switch c.dialect {
	case DialectElasticsearch, DialectOpenSearch:
		parts := make([]string, len(ids))
		for i, id := range ids {
			parts[i] = kbCol + " = " + c.bindValue(id)
		}
		return strings.Join(parts, " OR "), nil
	default:
		placeholders := make([]string, len(ids))
		for i, id := range ids {
			placeholders[i] = c.bindValue(id)
		}
		return kbCol + " IN (" + strings.Join(placeholders, ", ") + ")", nil
	}
}

func (c *compiler) compileFilter(node *FilterNode, inHaving bool) (string, error) {
	if len(node.And) > 0 {
		parts := make([]string, len(node.And))
		for i := range node.And {
			s, err := c.compileFilter(&node.And[i], inHaving)
			if err != nil {
				return "", err
			}
			parts[i] = "(" + s + ")"
		}
		return strings.Join(parts, " AND "), nil
	}
	if len(node.Or) > 0 {
		parts := make([]string, len(node.Or))
		for i := range node.Or {
			s, err := c.compileFilter(&node.Or[i], inHaving)
			if err != nil {
				return "", err
			}
			parts[i] = "(" + s + ")"
		}
		return strings.Join(parts, " OR "), nil
	}
	if node.Not != nil {
		s, err := c.compileFilter(node.Not, inHaving)
		if err != nil {
			return "", err
		}
		return "NOT (" + s + ")", nil
	}

	return c.compileLeafFilter(node, inHaving)
}

func (c *compiler) compileLeafFilter(node *FilterNode, inHaving bool) (string, error) {
	// Resolve left-hand side expression (aggregate alias in HAVING, or field).
	var lhs string
	var fieldMeta Field
	isAggRef := false
	if inHaving {
		if agg, ok := c.vq.aggAliases[node.Field]; ok {
			expr, err := c.aggregateExpr(agg)
			if err != nil {
				return "", err
			}
			lhs = expr
			isAggRef = true
		}
	}
	if !isAggRef {
		fieldMeta = c.vq.scope.Fields[node.Field]
	}

	// Handle IS NULL / IS NOT NULL.
	if node.Op == OpIsNull || node.Op == OpIsNotNull {
		if !isAggRef && c.dialect == DialectInfinity {
			jsonPath := c.infinityJSONPath(node.Field)
			if node.Op == OpIsNull {
				return fmt.Sprintf("json_extract_isnull(chunk_data, %s) = true", jsonPath), nil
			}
			return fmt.Sprintf("json_extract_isnull(chunk_data, %s) = false", jsonPath), nil
		}
		if !isAggRef {
			var err error
			lhs, err = c.fieldExpr(node.Field, false)
			if err != nil {
				return "", err
			}
		}
		if node.Op == OpIsNull {
			return lhs + " IS NULL", nil
		}
		return lhs + " IS NOT NULL", nil
	}

	// ES/OpenSearch tokenized text fields (*_tks / *_ltks) rewrite eq/contains
	// into RAGFlow's tokenized MATCH(...) expression cleanly at compile time.
	if !isAggRef && (c.dialect == DialectElasticsearch || c.dialect == DialectOpenSearch) &&
		fieldMeta.Type == FieldText && (node.Op == OpEq || node.Op == OpContains) {
		if s, ok := node.Value.(string); ok {
			matchExpr, okMatch, err := c.compileESMatch(node.Field, s)
			if err != nil {
				return "", err
			}
			if okMatch {
				return matchExpr, nil
			}
		}
	}

	needNumeric := !isAggRef && c.needsNumericCast(fieldMeta, node.Op, node.Value)
	if !isAggRef {
		var err error
		lhs, err = c.fieldExpr(node.Field, needNumeric)
		if err != nil {
			return "", err
		}
	}

	switch node.Op {
	case OpEq:
		return lhs + " = " + c.bindValue(node.Value), nil
	case OpNeq:
		return lhs + " <> " + c.bindValue(node.Value), nil
	case OpGt:
		return lhs + " > " + c.bindValue(node.Value), nil
	case OpGte:
		return lhs + " >= " + c.bindValue(node.Value), nil
	case OpLt:
		return lhs + " < " + c.bindValue(node.Value), nil
	case OpLte:
		return lhs + " <= " + c.bindValue(node.Value), nil
	case OpIn, OpNotIn:
		vals, ok := node.Value.([]any)
		if !ok || len(vals) == 0 {
			return "", fmt.Errorf("tablequery: %s requires non-empty slice", node.Op)
		}
		placeholders := make([]string, len(vals))
		for i, v := range vals {
			placeholders[i] = c.bindValue(v)
		}
		kw := " IN ("
		if node.Op == OpNotIn {
			kw = " NOT IN ("
		}
		return lhs + kw + strings.Join(placeholders, ", ") + ")", nil
	case OpContains, OpStartsWith, OpEndsWith:
		s, ok := node.Value.(string)
		if !ok {
			return "", fmt.Errorf("tablequery: %s requires string value", node.Op)
		}
		pattern := c.likePattern(node.Op, s)
		return lhs + " LIKE " + c.bindValue(pattern), nil
	default:
		return "", fmt.Errorf("tablequery: unsupported operator %q", node.Op)
	}
}

func (c *compiler) compileESMatch(field, raw string) (string, bool, error) {
	cleaned := strings.ReplaceAll(raw, "%", "")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return "", false, nil
	}
	tokenizeFn := c.opts.TokenizeForMatch
	if tokenizeFn == nil {
		tokenizeFn = defaultTokenizeForMatch
	}
	fine, err := tokenizeFn(cleaned)
	if err != nil {
		return "", false, nil
	}
	fine = strings.TrimSpace(fine)
	if fine == "" {
		return "", false, nil
	}
	col, err := c.fieldExpr(field, false)
	if err != nil {
		return "", false, err
	}
	return fmt.Sprintf("MATCH(%s, %s, 'operator=OR;minimum_should_match=30%%')", col, quoteSQLLiteral(fine)), true, nil
}

func defaultTokenizeForMatch(val string) (string, error) {
	tokenized, err := tokenizer.Tokenize(val)
	if err != nil {
		return "", err
	}
	return tokenizer.FineGrainedTokenize(tokenized)
}

func (c *compiler) aggregateExpr(agg Aggregate) (string, error) {
	switch agg.Fn {
	case AggCount:
		if agg.Field == "" {
			return "COUNT(*)", nil
		}
		expr, err := c.fieldExpr(agg.Field, false)
		if err != nil {
			return "", err
		}
		return "COUNT(" + expr + ")", nil
	case AggCountDistinct:
		expr, err := c.fieldExpr(agg.Field, false)
		if err != nil {
			return "", err
		}
		return "COUNT(DISTINCT " + expr + ")", nil
	case AggSum, AggAvg:
		expr, err := c.fieldExpr(agg.Field, true)
		if err != nil {
			return "", err
		}
		return strings.ToUpper(string(agg.Fn)) + "(" + expr + ")", nil
	case AggMin, AggMax:
		fieldMeta := c.vq.scope.Fields[agg.Field]
		expr, err := c.fieldExpr(agg.Field, fieldMeta.Type == FieldNumber)
		if err != nil {
			return "", err
		}
		return strings.ToUpper(string(agg.Fn)) + "(" + expr + ")", nil
	default:
		return "", fmt.Errorf("tablequery: unsupported aggregate %q", agg.Fn)
	}
}

func (c *compiler) orderExpr(field string) (string, error) {
	if agg, ok := c.vq.aggAliases[field]; ok {
		if c.dialect == DialectElasticsearch || c.dialect == DialectOpenSearch {
			return c.quoteAlias(agg.As), nil
		}
		return c.quoteAlias(agg.As), nil
	}
	return c.fieldExpr(field, false)
}

func (c *compiler) needsNumericCast(field Field, op FilterOp, val any) bool {
	if !c.usesJSONColumn() {
		return false
	}
	switch op {
	case OpGt, OpGte, OpLt, OpLte:
		if _, isNum := val.(float64); isNum {
			return true
		}
		return field.Type == FieldNumber
	case OpEq, OpNeq:
		_, isNum := val.(float64)
		return isNum
	case OpIn, OpNotIn:
		if slice, ok := val.([]any); ok && len(slice) > 0 {
			_, isNum := slice[0].(float64)
			return isNum
		}
	}
	return false
}

func (c *compiler) usesJSONColumn() bool {
	return c.dialect == DialectOceanBase || c.dialect == DialectInfinity
}

func (c *compiler) needsExplicitAlias(field string) bool {
	return c.usesJSONColumn()
}

func (c *compiler) docNameColumn() string {
	if c.dialect == DialectInfinity {
		return "docnm"
	}
	return "docnm_kwd"
}

func (c *compiler) systemColumn(col string) (string, error) {
	switch c.dialect {
	case DialectOceanBase:
		return "`" + col + "`", nil
	case DialectVastbase, DialectSereneDB:
		return `"` + col + `"`, nil
	default:
		return col, nil
	}
}

func (c *compiler) quoteTable(name string) (string, error) {
	if !IsValidTableIdentifier(name) {
		return "", fmt.Errorf("tablequery: invalid table identifier %q", name)
	}
	switch c.dialect {
	case DialectOceanBase:
		return "`" + name + "`", nil
	case DialectVastbase, DialectSereneDB:
		return `"` + name + `"`, nil
	default:
		return name, nil
	}
}

func (c *compiler) quoteAlias(alias string) string {
	switch c.dialect {
	case DialectOceanBase:
		return "`" + strings.ReplaceAll(alias, "`", "``") + "`"
	default:
		return `"` + strings.ReplaceAll(alias, `"`, `""`) + `"`
	}
}

func (c *compiler) fieldExpr(field string, numeric bool) (string, error) {
	switch c.dialect {
	case DialectElasticsearch, DialectOpenSearch:
		return quoteESColumn(field)
	case DialectOceanBase:
		path := mysqlJSONPath(field)
		expr := fmt.Sprintf("JSON_UNQUOTE(JSON_EXTRACT(`chunk_data`, %s))", quoteSQLLiteral(path))
		if numeric {
			return "CAST(" + expr + " AS DOUBLE)", nil
		}
		return expr, nil
	case DialectInfinity:
		expr := fmt.Sprintf("json_extract_string(chunk_data, %s)", c.infinityJSONPath(field))
		if numeric {
			return "CAST(" + expr + " AS DOUBLE)", nil
		}
		return expr, nil
	case DialectVastbase, DialectSereneDB:
		return quotePGColumn(field)
	default:
		return "", fmt.Errorf("tablequery: unsupported dialect %q", c.dialect)
	}
}

func (c *compiler) infinityJSONPath(field string) string {
	// Escape backslashes and double quotes in JSONPath keys; quote when needed.
	if isSimpleJSONPathKey(field) {
		return quoteSQLLiteral("$." + field)
	}
	escaped := strings.ReplaceAll(field, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return quoteSQLLiteral(`$."` + escaped + `"`)
}

func mysqlJSONPath(field string) string {
	if isSimpleJSONPathKey(field) {
		return "$." + field
	}
	escaped := strings.ReplaceAll(field, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `$."` + escaped + `"`
}

func isSimpleJSONPathKey(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		isAlpha := (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
		isDigit := ch >= '0' && ch <= '9'
		if i == 0 && !isAlpha {
			return false
		}
		if !isAlpha && !isDigit {
			return false
		}
	}
	return true
}

func quoteESColumn(col string) (string, error) {
	if col == "" || strings.ContainsRune(col, '\x00') {
		return "", fmt.Errorf("tablequery: invalid column name %q", col)
	}
	if isSimpleJSONPathKey(col) {
		return col, nil
	}
	return `"` + strings.ReplaceAll(col, `"`, `""`) + `"`, nil
}

func quotePGColumn(col string) (string, error) {
	if col == "" || strings.ContainsRune(col, '\x00') {
		return "", fmt.Errorf("tablequery: invalid column name %q", col)
	}
	return `"` + strings.ReplaceAll(col, `"`, `""`) + `"`, nil
}

func (c *compiler) likePattern(op FilterOp, raw string) string {
	escaped := raw
	if c.dialect == DialectOceanBase || c.dialect == DialectVastbase || c.dialect == DialectSereneDB {
		escaped = strings.ReplaceAll(escaped, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `%`, `\%`)
		escaped = strings.ReplaceAll(escaped, `_`, `\_`)
	}
	switch op {
	case OpContains:
		return "%" + escaped + "%"
	case OpStartsWith:
		return escaped + "%"
	case OpEndsWith:
		return "%" + escaped
	default:
		return escaped
	}
}

func (c *compiler) bindValue(v any) string {
	switch c.dialect {
	case DialectOceanBase:
		c.args = append(c.args, v)
		return "?"
	case DialectVastbase, DialectSereneDB:
		c.args = append(c.args, v)
		return fmt.Sprintf("$%d", len(c.args))
	default:
		// Elasticsearch, OpenSearch, and Infinity accept literal SQL text only.
		return formatSQLLiteral(v)
	}
}

func formatSQLLiteral(v any) string {
	switch x := v.(type) {
	case string:
		return quoteSQLLiteral(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if math.Trunc(x) == x && x >= math.MinInt64 && x <= math.MaxInt64 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return quoteSQLLiteral(fmt.Sprintf("%v", v))
	}
}

func quoteSQLLiteral(s string) string {
	// Strip NUL bytes defensively and double single quotes per SQL standard.
	s = strings.ReplaceAll(s, "\x00", "")
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
