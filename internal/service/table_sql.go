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

package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
)

// A structured table answer reads the index directly, so the statement a model
// writes is the only thing between a question and every row of the tenant's
// table. tableSQL turns one generated statement into a statement whose range
// the backend wrote, and refuses anything it cannot read that way.

// tableSQLBudgetBytes caps the statement that reaches an engine. A knowledge
// base with more rows than fit the budget is not truncated into a partial
// answer: the caller falls back to ordinary retrieval instead.
const tableSQLBudgetBytes = 64 << 10

const (
	tableSQLDefaultRows = 100
	tableSQLMaxRows     = 1000
)

// jsonPathRe matches a JSONPath that addresses one indexed table column. The
// key is a hash of a header (entity.TableDataKey), so a path either names a column a
// document published or was invented.
var jsonPathRe = regexp.MustCompile(`^\$\.(c_[0-9a-f]{64})$`)

// tableIdentifierRe is the shape of a document, knowledge base or table name
// this process generated. Anything else is refused rather than written into a
// statement.
var tableIdentifierRe = regexp.MustCompile(`^[0-9A-Za-z_-]{1,64}$`)

// tableColumns are the physical columns a table query may name. It lists both
// the Elasticsearch-shaped names and the Infinity names that
// conf/infinity_mapping.json rewrites them to, because the engine rewrites
// after this check runs. A name outside the list reaches no data: the engine
// answers "unknown column" and the caller falls back to retrieval.
var tableColumns = map[string]bool{
	"id": true, "doc_id": true, "kb_id": true,
	"docnm": true, "docnm_kwd": true, "doc_type_kwd": true,
	"title_tks": true, "title_sm_tks": true,
	"content": true, "content_with_weight": true, "content_ltks": true, "content_sm_ltks": true,
	"important_kwd": true, "important_tks": true, "question_kwd": true, "question_tks": true,
	"tag_kwd": true, "img_id": true, "create_time": true, "create_timestamp_flt": true,
	"pagerank_fea": true, "weight_int": true, "weight_flt": true, "rank_flt": true,
	"available_int": true, "position_int": true, "page_num_int": true, "top_int": true,
	"chunk_data": true, "table_row_int": true,
}

// tableFunctions are the calls a table query may make. json_extract_string and
// json_extract_isnull are the forms the prompt asks for; the OceanBase
// executor rewrites them into that engine's native pair.
var tableFunctions = map[string]bool{
	"count": true, "sum": true, "avg": true, "min": true, "max": true, "total": true,
	"group_concat": true, "stddev": true, "variance": true,
	"abs": true, "round": true, "floor": true, "ceil": true, "ceiling": true,
	"coalesce": true, "if": true, "ifnull": true, "nullif": true, "cast": true, "try_cast": true,
	"concat": true, "concat_ws": true, "lower": true, "upper": true, "trim": true,
	"length": true, "char_length": true, "substr": true, "substring": true,
	"replace": true, "instr": true,
	"json_extract_string": true, "json_extract": true, "json_value": true,
	"json_extract_isnull": true,
}

// jsonFunctions read a published table column and so have their arguments
// checked, not only their name.
var jsonFunctions = map[string]bool{
	"json_extract_string": true, "json_extract": true, "json_value": true, "json_extract_isnull": true,
}

// tableKeywords are bare words that name neither a column nor a function. CAST
// type names appear unquoted, so they are here too.
var tableKeywords = map[string]bool{
	"and": true, "or": true, "not": true, "is": true, "null": true, "true": true, "false": true,
	"like": true, "ilike": true, "rlike": true, "regexp": true, "glob": true, "match": true,
	"in": true, "between": true, "case": true, "when": true, "then": true, "else": true,
	"end": true, "as": true, "distinct": true, "all": true, "any": true, "some": true,
	"exists": true, "asc": true, "desc": true, "nulls": true, "first": true, "last": true,
	"escape": true, "interval": true,
	"integer": true, "int": true, "bigint": true, "smallint": true, "tinyint": true,
	"float": true, "double": true, "real": true, "decimal": true, "numeric": true,
	"char": true, "varchar": true, "text": true, "boolean": true, "bool": true,
	"date": true, "datetime": true, "time": true, "timestamp": true, "json": true,
	"unsigned": true, "signed": true,
}

// tableOperators are the separators and operators a statement may keep. A dot
// is not among them: it only appears inside a qualified name, which is consumed
// as one unit.
var tableOperators = map[string]bool{
	",": true, "(": true, ")": true, "*": true, "+": true, "-": true, "/": true, "%": true,
	"=": true, "==": true, "<": true, ">": true, "<=": true, ">=": true, "<>": true,
	"!=": true, "<=>": true,
}

// tableSQLPolicy is the range one answer may read.
type tableSQLPolicy struct {
	tableName string
	// quote is how this engine writes a quoted name, and so how a checked
	// statement is emitted.
	quote rune
	// kbCondition restricts a table shared by several knowledge bases. It is
	// empty for Infinity, whose table name already names the knowledge base.
	kbCondition string
	// docIDs is the non-empty set of publishing documents the answer may read.
	docIDs   []string
	dataKeys map[string]bool
}

// tableSQLStatement is a checked statement, rewritten to read only its range.
type tableSQLStatement struct {
	// text is what goes to the engine.
	text string
	// aggregating is true when the answer collapses rows, which decides
	// whether the result can name its own sources.
	aggregating bool
	// where is the caller's own condition as written. A follow-up query asks
	// the same question with it, instead of reading back a statement that is
	// already restricted and restricting it twice.
	where string
}

// tableSQL runs every statement of one structured table answer. Each call
// re-reads its input: a statement that came back from a repair model is no
// better known than the first one.
type tableSQL struct {
	docEngine engine.DocEngine
	kbIDs     []string
	fieldMap  map[string]interface{}
	policy    tableSQLPolicy
}

// newTableSQL builds the range for one structured table answer. A nil result
// means SQL does not apply to this question, which is not a failure: the caller
// answers from vector retrieval.
//
// allowedDocIDs is the intersection of enabled publishing documents and any
// requested document restriction. A missing or empty set cannot execute SQL.
func newTableSQL(docEngine engine.DocEngine, chat *entity.Chat, kbs []*entity.Knowledgebase, allowedDocIDs []string, fieldMap map[string]interface{}) (*tableSQL, error) {
	if docEngine == nil || chat == nil || len(kbs) != 1 || len(fieldMap) == 0 {
		return nil, nil
	}
	engineName := docEngine.GetType()
	if !SupportsStructuredTableSQL(engineName) {
		return nil, nil
	}
	if len(kbs[0].ID) == 0 || !tableIdentifierRe.MatchString(kbs[0].ID) {
		return nil, fmt.Errorf("knowledge base id %q is not readable", kbs[0].ID)
	}
	if len(allowedDocIDs) == 0 {
		return nil, errors.New("no document in range publishes table columns")
	}

	policy := tableSQLPolicy{
		tableName: ragflowTableName(chat.TenantID, kbs, docEngine),
		quote:     '`',
		docIDs:    append([]string(nil), allowedDocIDs...),
		dataKeys:  make(map[string]bool, len(fieldMap)),
	}
	if engineName == "infinity" {
		policy.quote = '"'
		// The knowledge base is only in range because the table names it.
		// ragflowTableName falls back to the tenant's shared table when the
		// identifier does not read as one, and a shared table is not a range.
		if want := fmt.Sprintf("ragflow_%s_%s", chat.TenantID, kbs[0].ID); policy.tableName != want {
			return nil, fmt.Errorf("table %q does not name knowledge base %s", policy.tableName, kbs[0].ID)
		}
	} else {
		kbCondition, err := tableComparison("kb_id", kbs[0].ID)
		if err != nil {
			return nil, err
		}
		policy.kbCondition = kbCondition
	}
	if !tableIdentifierRe.MatchString(policy.tableName) {
		return nil, fmt.Errorf("table %q is not a readable identifier", policy.tableName)
	}
	for dataKey := range fieldMap {
		policy.dataKeys[dataKey] = true
	}

	return &tableSQL{
		docEngine: docEngine,
		kbIDs:     kbIDStrings(kbs),
		fieldMap:  fieldMap,
		policy:    policy,
	}, nil
}

// run reads one generated statement and executes what the range allows. It
// returns the statement that ran so a follow-up query can be built from the
// same reading.
func (q *tableSQL) run(ctx context.Context, sqlText string) ([]map[string]interface{}, *tableSQLStatement, error) {
	if q == nil {
		return nil, nil, errors.New("no table query range")
	}
	statement, err := q.policy.check(sqlText)
	if err != nil {
		return nil, nil, err
	}
	rows, err := q.docEngine.RunSQL(ctx, q.policy.tableName, statement.text, q.kbIDs, "json")
	if err != nil {
		return nil, statement, err
	}
	return rows, statement, nil
}

// check reads a statement, then rewrites it so the only rows it can read are the
// ones in range. Anything it cannot fully account for is refused: an unreadable
// statement is a skipped SQL answer, never a partially checked one.
func (p *tableSQLPolicy) check(sqlText string) (*tableSQLStatement, error) {
	tokens, err := utility.SQLScan(sqlText)
	if err != nil {
		return nil, err
	}
	shape, err := utility.SQLSplitSelect(tokens)
	if err != nil {
		return nil, err
	}
	clauses := shape.Clauses

	table, err := utility.SQLTableReference(clauses.From)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(table, p.tableName) {
		return nil, fmt.Errorf("the query reads %q, not %q", table, p.tableName)
	}

	for _, clause := range []struct {
		label  string
		tokens []utility.SQLToken
	}{
		{"select", clauses.Select}, {"where", clauses.Where}, {"group by", clauses.GroupBy},
		{"having", clauses.Having}, {"order by", clauses.OrderBy},
		{"limit", clauses.Limit}, {"offset", clauses.Offset},
	} {
		if err := p.validate(clause.tokens, clause.label); err != nil {
			return nil, err
		}
	}
	return p.rewrite(shape)
}

// rewrite puts the range in front of whatever the caller asked for. The
// caller's own condition keeps its parentheses, so an OR inside it cannot open
// out into the injected conditions.
func (p *tableSQLPolicy) rewrite(shape *utility.SQLStatementShape) (*tableSQLStatement, error) {
	clauses := shape.Clauses
	limit := tableSQLDefaultRows
	if len(clauses.Limit) > 0 {
		if len(clauses.Limit) != 1 || clauses.Limit[0].Kind != utility.SQLNumber {
			return nil, errors.New("LIMIT must be a non-negative integer")
		}
		n, err := strconv.ParseUint(clauses.Limit[0].Text, 10, 64)
		if err != nil {
			return nil, errors.New("LIMIT must be a non-negative integer")
		}
		limit = int(min(n, uint64(tableSQLMaxRows)))
	}
	conditions := make([]string, 0, 5)
	if p.kbCondition != "" {
		conditions = append(conditions, p.kbCondition)
	}
	docCondition, err := p.docIDCondition()
	if err != nil {
		return nil, err
	}
	conditions = append(conditions, docCondition)
	conditions = append(conditions, "available_int = 1", "table_row_int = 1")

	parts := []string{"select " + utility.SQLRender(clauses.Select, p.quote), "from " + p.tableName}
	where := utility.SQLRender(clauses.Where, p.quote)
	if where != "" {
		conditions = append(conditions, "("+where+")")
	}
	parts = append(parts, "where "+strings.Join(conditions, " and "))
	for _, clause := range []struct {
		keyword string
		tokens  []utility.SQLToken
	}{
		{"group by", clauses.GroupBy}, {"having", clauses.Having}, {"order by", clauses.OrderBy},
	} {
		if rendered := utility.SQLRender(clause.tokens, p.quote); rendered != "" {
			parts = append(parts, clause.keyword+" "+rendered)
		}
	}
	// LIMIT applies after aggregation, so counts and sums still read the full range.
	parts = append(parts, "limit "+strconv.Itoa(limit))
	if offset := utility.SQLRender(clauses.Offset, p.quote); offset != "" {
		parts = append(parts, "offset "+offset)
	}

	statement := &tableSQLStatement{
		text:        strings.Join(parts, " "),
		aggregating: shape.Aggregating,
		where:       where,
	}
	if len(statement.text) > tableSQLBudgetBytes {
		return nil, fmt.Errorf("the scoped statement needs %d bytes, over the %d budget",
			len(statement.text), tableSQLBudgetBytes)
	}
	return statement, nil
}

func (p *tableSQLPolicy) docIDCondition() (string, error) {
	if len(p.docIDs) == 0 {
		return "", errors.New("no publishing documents in query range")
	}
	literals := make([]string, 0, len(p.docIDs))
	for _, docID := range p.docIDs {
		comparison, err := tableComparison("", docID)
		if err != nil {
			return "", err
		}
		literals = append(literals, comparison)
	}
	if len(literals) == 1 {
		return "doc_id = " + literals[0], nil
	}
	return "doc_id IN (" + strings.Join(literals, ", ") + ")", nil
}

// tableComparison writes one value as a quoted literal, or as a name compared
// against it when a column is given.
func tableComparison(column, value string) (string, error) {
	if !tableIdentifierRe.MatchString(value) {
		return "", fmt.Errorf("value %q is not readable", value)
	}
	quoted, err := utility.SQLQuoteLiteral(value)
	if err != nil {
		return "", err
	}
	if column == "" {
		return quoted, nil
	}
	return column + " = " + quoted, nil
}

// validate reads one clause against what a table answer may ask for. It walks
// tokens rather than matching text, so a keyword inside a value stays a value
// and an unknown word is a refusal.
func (p *tableSQLPolicy) validate(clause []utility.SQLToken, label string) error {
	for i := 0; i < len(clause); i++ {
		token := clause[i]
		switch token.Kind {
		case utility.SQLNumber, utility.SQLString:
			continue
		case utility.SQLQuoted:
			// A quoted name means nothing here except as the label a caller put
			// on a result column.
			if i == 0 || !clause[i-1].IsWord("as") {
				return fmt.Errorf("%s: %q is not a column this query may read", label, token.Name)
			}
		case utility.SQLPunct:
			if !tableOperators[token.Lower] {
				return fmt.Errorf("%s: %q is not a supported operator", label, token.Text)
			}
		case utility.SQLWord:
			used, err := p.validateWord(clause, i, label)
			if err != nil {
				return err
			}
			i = used - 1
		}
	}
	return nil
}

// validateWord reads one word together with however far its name reaches: a
// function call, a qualified column, a result label, or a plain name. It
// returns the index after everything it consumed.
func (p *tableSQLPolicy) validateWord(clause []utility.SQLToken, i int, label string) (int, error) {
	token := clause[i]
	if isPunctAt(clause, i+1, "(") {
		if !tableFunctions[token.Lower] {
			return 0, fmt.Errorf("%s: %s() is not a supported function", label, token.Text)
		}
		if jsonFunctions[token.Lower] {
			if err := p.validateJSONPath(clause, i, label); err != nil {
				return 0, err
			}
		}
		return i + 1, nil
	}
	if isPunctAt(clause, i+1, ".") {
		return p.validateQualifiedName(clause, i, label)
	}
	if i > 0 && clause[i-1].IsWord("as") {
		return i + 1, nil // whatever the caller chose to call a result column
	}
	if tableColumns[token.Lower] || tableKeywords[token.Lower] {
		return i + 1, nil
	}
	return 0, fmt.Errorf("%s: %q is not a column this query may read", label, token.Name)
}

// validateQualifiedName reads table.column. Only the table the query is
// restricted to may qualify a column, so a name cannot point at another table
// while the range check is looking at this one.
func (p *tableSQLPolicy) validateQualifiedName(clause []utility.SQLToken, i int, label string) (int, error) {
	segments := []utility.SQLToken{clause[i]}
	next := i + 1
	for isPunctAt(clause, next, ".") {
		if next+1 >= len(clause) || !isName(clause[next+1]) {
			return 0, fmt.Errorf("%s: %q is not a complete name", label, segments[0].Name)
		}
		segments = append(segments, clause[next+1])
		next += 2
	}
	if len(segments) > 3 || !strings.EqualFold(segments[0].Lower, p.tableName) {
		return 0, fmt.Errorf("%s: %q does not name a column of %s", label, joinNames(segments), p.tableName)
	}
	for _, segment := range segments[1:] {
		if !tableColumns[segment.Lower] {
			return 0, fmt.Errorf("%s: %q is not a column this query may read", label, segment.Name)
		}
	}
	return next, nil
}

// validateJSONPath checks that a JSON extraction reads a published column by
// its stored key. The readable name is never the address; it is what the answer
// is labelled with afterwards.
func (p *tableSQLPolicy) validateJSONPath(clause []utility.SQLToken, name int, label string) error {
	args, _, err := utility.SQLCallArguments(clause, name)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", label, clause[name].Text, err)
	}
	if len(args) != 2 {
		return fmt.Errorf("%s: %s needs a column and a path, got %d arguments", label, clause[name].Text, len(args))
	}
	if len(args[0]) != 1 || !args[0][0].IsWord("chunk_data") {
		return fmt.Errorf("%s: %s reads %q, want chunk_data", label, clause[name].Text, utility.SQLRender(args[0], 0))
	}
	if len(args[1]) != 1 || args[1][0].Kind != utility.SQLString {
		return fmt.Errorf("%s: %s needs a quoted path", label, clause[name].Text)
	}
	match := jsonPathRe.FindStringSubmatch(args[1][0].Value)
	if match == nil {
		return fmt.Errorf("%s: %q is not a published column path", label, args[1][0].Value)
	}
	if !p.dataKeys[match[1]] {
		return fmt.Errorf("%s: column %q is not published by these documents", label, match[1])
	}
	return nil
}

func isPunctAt(tokens []utility.SQLToken, i int, punct string) bool {
	return i >= 0 && i < len(tokens) && tokens[i].IsPunct(punct)
}

func isName(token utility.SQLToken) bool {
	return token.Kind == utility.SQLWord || token.Kind == utility.SQLQuoted
}

func joinNames(tokens []utility.SQLToken) string {
	names := make([]string, 0, len(tokens))
	for _, token := range tokens {
		names = append(names, token.Name)
	}
	return strings.Join(names, ".")
}

// restrictToRequestedDocs keeps the documents a request named inside the
// documents that publish table columns. A nil request restriction uses all
// publishers; an explicit empty restriction or no overlap permits no documents.
func restrictToRequestedDocs(publishing, requested []string) []string {
	if requested == nil {
		return publishing
	}
	allowed := make(map[string]bool, len(publishing))
	for _, docID := range publishing {
		allowed[docID] = true
	}
	restricted := make([]string, 0, len(requested))
	for _, docID := range requested {
		if allowed[docID] {
			restricted = append(restricted, docID)
		}
	}
	return restricted
}

// SupportsStructuredTableSQL reports whether the active engine can answer a
// query against the columns a derived profile describes. The profile names
// columns by the JSON key they are stored under, which only an engine whose SQL
// prompt extracts from a JSON column can read back; Elasticsearch, OpenSearch
// and SereneDB address physical fields, so handing them a JSON field map would
// generate a query nothing can answer.
func SupportsStructuredTableSQL(engineName string) bool {
	return engineName == string(engine.EngineInfinity) || engine.IsOceanBaseFamily(engineName)
}
