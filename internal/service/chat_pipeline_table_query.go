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
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/tablequery"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"

	"go.uber.org/zap"
)

const tableQuerySysPrompt = `You are a structured-table query planner. Convert the user's question into a single JSON query object for ONE table.

Output ONLY valid JSON matching this schema (no markdown fences, no commentary):
{
  "unsupported": false,
  "select": ["field_name"],
  "distinct": false,
  "aggregates": [{"fn": "count|count_distinct|sum|avg|min|max", "field": "field_name_or_empty_for_count_star", "as": "alias"}],
  "where": <FilterNode>,
  "group_by": ["field_name"],
  "having": <FilterNode>,
  "order_by": [{"field": "field_or_aggregate_alias", "desc": true}],
  "limit": 50
}

FilterNode is one of:
- {"and": [<FilterNode>, ...]}
- {"or": [<FilterNode>, ...]}
- {"not": <FilterNode>}
- {"field": "exact_field_name", "op": "eq|neq|gt|gte|lt|lte|in|not_in|contains|starts_with|ends_with|is_null|is_not_null", "value": ...}

RULES:
1. Use ONLY exact field names from the provided field list. Never invent table names, kb_id, doc_id, or SQL text.
2. Do NOT include doc_id or docnm columns in select; citations are added automatically.
3. For is_null / is_not_null, omit "value". For in / not_in, "value" must be a non-empty JSON array.
4. For COUNT(*), use {"fn": "count", "as": "total_rows"} with no "field".
5. When the question asks to display specific columns, add {"field": "<col>", "op": "is_not_null"} if appropriate.
6. If the question cannot be answered with this single-table JSON schema (e.g., unstructured narrative question or unsupported calculation), return {"unsupported": true}.

Examples:
- Question: "Show top 5 products in Phones ordered by price descending"
  {"select": ["product_tks", "price_flt"], "where": {"field": "category", "op": "eq", "value": "Phones"}, "order_by": [{"field": "price_flt", "desc": true}], "limit": 5}
- Question: "Average price per category where average price > 100"
  {"select": ["category"], "aggregates": [{"fn": "avg", "field": "price_flt", "as": "avg_price"}], "group_by": ["category"], "having": {"field": "avg_price", "op": "gt", "value": 100}, "order_by": [{"field": "avg_price", "desc": true}]}
`

const tableQueryUserPromptTemplate = `Available fields (exact name -> description):
%s

Question: %s
Return ONLY the JSON query object.`

const tableQueryRepairPromptTemplate = `Available fields (exact name -> description):
%s

Question: %s

Your previous JSON query failed validation/execution with error:
%s

Fix the error and return ONLY a valid JSON query object matching the schema (or {"unsupported": true} if not representable).`

// useSQL executes structured table retrieval using the closed TableQuery IR.
// The authorization scope (physical table name, validated kb_ids, whitelisted
// fields) is enforced before calling the LLM and compiled natively by the
// active document engine. Returns (nil, nil) to fall back to vector search.
func (s *ChatPipelineService) useSQL(
	ctx context.Context,
	chat *entity.Chat,
	kbs []*entity.Knowledgebase,
	question string,
	chatModel *modelModule.ChatModel,
	fieldMap map[string]interface{},
	quote bool,
) (ans map[string]interface{}, err error) {
	if chat == nil || chatModel == nil || len(kbs) == 0 || len(fieldMap) == 0 {
		return nil, nil
	}

	docEngine := engine.Get()
	if docEngine == nil {
		return nil, nil
	}
	return s.useSQLWithEngine(ctx, docEngine, chat, kbs, question, chatModel, fieldMap, quote)
}

func (s *ChatPipelineService) useSQLWithEngine(
	ctx context.Context,
	docEngine engine.DocEngine,
	chat *entity.Chat,
	kbs []*entity.Knowledgebase,
	question string,
	chatModel *modelModule.ChatModel,
	fieldMap map[string]interface{},
	quote bool,
) (ans map[string]interface{}, err error) {
	if docEngine == nil || chat == nil || chatModel == nil || len(kbs) == 0 || len(fieldMap) == 0 {
		return nil, nil
	}

	scope, ok := buildTableQueryScope(chat.TenantID, kbs, docEngine, fieldMap)
	if !ok {
		return nil, nil
	}

	common.Debug("Table retrieval: question", zap.String("question", question))

	userPrompt := buildTableQueryUserPrompt(question, fieldMap)

	var vq *tablequery.ValidatedQuery
	if isRowCountQuestion(question) {
		var buildErr error
		vq, buildErr = tablequery.BuildRowCountQuery(scope)
		if buildErr != nil {
			common.Warn("Table retrieval: failed to build row-count query", zap.Error(buildErr))
			return nil, nil
		}
		common.Debug("Table retrieval: using deterministic row-count query")
	} else {
		var planErr error
		vq, planErr = generateAndValidateTableQuery(ctx, chatModel, scope, question, fieldMap, quote)
		if planErr != nil {
			if errors.Is(planErr, tablequery.ErrUnsupported) {
				common.Debug("Table retrieval: question marked unsupported; falling back to vector search")
			} else {
				common.Warn("Table retrieval: query planning failed; falling back", zap.Error(planErr))
			}
			return nil, nil
		}
	}

	rows, execErr := docEngine.ExecuteTableQuery(ctx, vq)
	if execErr != nil {
		common.Debug("Table retrieval: initial execution failed, attempting repair", zap.Error(execErr))
		repairedVQ, repairErr := repairTableQuery(ctx, chatModel, scope, question, execErr.Error(), fieldMap, quote)
		if repairErr != nil {
			common.Warn("Table retrieval: repair failed; falling back", zap.Error(repairErr))
			return nil, nil
		}
		rows, execErr = docEngine.ExecuteTableQuery(ctx, repairedVQ)
		if execErr != nil {
			common.Warn("Table retrieval: repaired query also failed; falling back", zap.Error(execErr))
			return nil, nil
		}
		vq = repairedVQ
	}

	if len(rows) == 0 {
		common.Debug("Table retrieval: execution succeeded but returned 0 rows")
		return nil, nil
	}

	answerStr, ref := s.buildSQLReference(ctx, docEngine, vq, rows, fieldMap, quote)
	return map[string]interface{}{
		"answer":    answerStr,
		"reference": ref,
		"prompt":    tableQuerySysPrompt + "\n\n" + userPrompt,
	}, nil
}

// buildTableQueryScope validates tenant/dataset isolation invariants up front,
// before spending an LLM call.
func buildTableQueryScope(
	tenantID string,
	kbs []*entity.Knowledgebase,
	docEngine engine.DocEngine,
	fieldMap map[string]interface{},
) (tablequery.Scope, bool) {
	if docEngine == nil || len(kbs) == 0 || len(fieldMap) == 0 {
		return tablequery.Scope{}, false
	}
	kbIDs := kbIDStrings(kbs)
	if len(kbIDs) != len(kbs) {
		common.Warn("Table retrieval: missing kb_id in authorized dataset list; falling back")
		return tablequery.Scope{}, false
	}
	engineName := docEngine.GetType()
	if engineName == string(engine.EngineInfinity) && len(kbIDs) != 1 {
		common.Debug("Table retrieval: Infinity supports single-dataset table queries only; falling back",
			zap.Int("kb_count", len(kbIDs)))
		return tablequery.Scope{}, false
	}
	tableName, ok := ragflowTableName(tenantID, kbs, docEngine)
	if !ok {
		return tablequery.Scope{}, false
	}
	scope := tablequery.Scope{
		TenantID:  tenantID,
		TableName: tableName,
		KBIDs:     kbIDs,
		Fields:    tablequery.FieldsFromFieldMap(fieldMap),
		MaxLimit:  tablequery.DefaultMaxLimit,
	}
	if err := tablequery.ValidateScope(scope); err != nil {
		common.Warn("Table retrieval: invalid authorization scope; falling back", zap.Error(err))
		return tablequery.Scope{}, false
	}
	return scope, true
}

// ragflowTableName resolves the authoritative physical table/index name for the
// active engine. Returns ("", false) when the tenant ID or Infinity kb_id is
// not a safe identifier.
func ragflowTableName(tenantID string, kbs []*entity.Knowledgebase, docEngine engine.DocEngine) (string, bool) {
	if tenantID == "" {
		return "", false
	}
	var name string
	if docEngine != nil && docEngine.GetType() == string(engine.EngineInfinity) {
		if len(kbs) != 1 || kbs[0] == nil || !tablequery.IsValidUUID(kbs[0].ID) {
			common.Warn("ragflowTableName: Infinity requires exactly one valid kb_id UUID")
			return "", false
		}
		name = fmt.Sprintf("ragflow_%s_%s", tenantID, kbs[0].ID)
	} else {
		name = "ragflow_" + tenantID
	}
	if !tablequery.IsValidTableIdentifier(name) {
		common.Warn("ragflowTableName: resolved table name is not a valid identifier", zap.String("table", name))
		return "", false
	}
	return name, true
}

func formatFieldBullets(fieldMap map[string]interface{}) string {
	names := sortedFieldNames(fieldMap)
	var b strings.Builder
	for _, n := range names {
		fmt.Fprintf(&b, "  - %s: %v\n", n, fieldMap[n])
	}
	return strings.TrimRight(b.String(), "\n")
}

func buildTableQueryUserPrompt(question string, fieldMap map[string]interface{}) string {
	return fmt.Sprintf(tableQueryUserPromptTemplate, formatFieldBullets(fieldMap), question)
}

func buildTableQueryRepairPrompt(question, errMsg string, fieldMap map[string]interface{}) string {
	return fmt.Sprintf(tableQueryRepairPromptTemplate, formatFieldBullets(fieldMap), question, errMsg)
}

func chatForTableQueryJSON(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	sysPrompt, userPrompt, errPrefix string,
) (string, error) {
	if chatModel == nil || chatModel.ModelDriver == nil {
		return "", fmt.Errorf("nil chat model")
	}
	tempLow := 0.06
	cfg := &modelModule.ChatConfig{
		Temperature: &tempLow,
	}
	msgs := []modelModule.Message{
		{Role: "system", Content: sysPrompt},
		{Role: "user", Content: userPrompt},
	}
	resp, err := chatModel.ChatWithMessages(ctx, msgs, cfg, nil)
	if err != nil {
		return "", err
	}
	if resp == nil || resp.Answer == nil || strings.TrimSpace(*resp.Answer) == "" {
		return "", fmt.Errorf("%s: empty response", errPrefix)
	}
	return *resp.Answer, nil
}

func generateAndValidateTableQuery(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	scope tablequery.Scope,
	question string,
	fieldMap map[string]interface{},
	includeCitations bool,
) (*tablequery.ValidatedQuery, error) {
	userPrompt := buildTableQueryUserPrompt(question, fieldMap)
	raw, err := chatForTableQueryJSON(ctx, chatModel, tableQuerySysPrompt, userPrompt, "table query generation")
	if err != nil {
		return nil, err
	}
	vq, valErr := tablequery.ParseAndValidate(raw, scope, includeCitations)
	if valErr == nil || errors.Is(valErr, tablequery.ErrUnsupported) {
		return vq, valErr
	}
	// One deterministic schema-validation repair attempt.
	common.Debug("Table retrieval: initial JSON failed validation, attempting repair", zap.Error(valErr))
	return repairTableQuery(ctx, chatModel, scope, question, valErr.Error(), fieldMap, includeCitations)
}

func repairTableQuery(
	ctx context.Context,
	chatModel *modelModule.ChatModel,
	scope tablequery.Scope,
	question, errMsg string,
	fieldMap map[string]interface{},
	includeCitations bool,
) (*tablequery.ValidatedQuery, error) {
	repairPrompt := buildTableQueryRepairPrompt(question, errMsg, fieldMap)
	raw, err := chatForTableQueryJSON(ctx, chatModel, tableQuerySysPrompt, repairPrompt, "table query repair")
	if err != nil {
		return nil, err
	}
	return tablequery.ParseAndValidate(raw, scope, includeCitations)
}

var rowCountPhraseRe = regexp.MustCompile(`(?i)\b(how many rows|number of rows|row count)\b`)
var rowCountSubjectRe = regexp.MustCompile(`(?i)\b(dataset|table|spreadsheet|excel)\b`)

func isRowCountQuestion(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	return rowCountPhraseRe.MatchString(q) && rowCountSubjectRe.MatchString(q)
}

func sortedFieldNames(fieldMap map[string]interface{}) []string {
	names := make([]string, 0, len(fieldMap))
	for k := range fieldMap {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// -----------------------------------------------------------------------
// Result formatting & citation assembly
// -----------------------------------------------------------------------

var isoTimestampCellRe = regexp.MustCompile(`T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+Z)?\|`)

func stripISOTimestamps(rows string) string {
	return isoTimestampCellRe.ReplaceAllString(rows, "|")
}

var asAliasRe = regexp.MustCompile(`(?i)\s+AS\s+([^\s,)]+)`)
var parenSuffixRe = regexp.MustCompile(`(/.*|（[^（）]+）)`)

func cleanDisplay(s string) string {
	return parenSuffixRe.ReplaceAllString(s, "")
}

func mapColumnName(colName string, fieldMap map[string]interface{}) string {
	if strings.EqualFold(colName, "count(star)") {
		return "COUNT(*)"
	}
	if m := asAliasRe.FindStringSubmatch(colName); len(m) >= 2 {
		alias := strings.Trim(m[1], `"'`)
		if disp, ok := fieldMap[alias]; ok {
			return cleanDisplay(fmt.Sprintf("%v", disp))
		}
		for k, v := range fieldMap {
			if strings.EqualFold(k, alias) {
				return cleanDisplay(fmt.Sprintf("%v", v))
			}
		}
		return alias
	}
	if disp, ok := fieldMap[colName]; ok {
		return cleanDisplay(fmt.Sprintf("%v", disp))
	}
	colLower := strings.ToLower(colName)
	for k, v := range fieldMap {
		if strings.ToLower(k) == colLower {
			return cleanDisplay(fmt.Sprintf("%v", v))
		}
	}
	result := colName
	for k, v := range fieldMap {
		result = strings.ReplaceAll(result, k, fmt.Sprintf("%v", v))
	}
	return cleanDisplay(result)
}

func chunkKBIDForDoc(rowDict map[string]interface{}, kbIDs []string, docID interface{}) string {
	if len(kbIDs) == 1 {
		return kbIDs[0]
	}
	if v, ok := rowDict["kb_id"]; ok && v != nil && v != "" {
		return fmt.Sprintf("%v", v)
	}
	if v, ok := rowDict["kb_id_kwd"]; ok && v != nil && v != "" {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func cleanCellValue(v interface{}) string {
	s := fmt.Sprintf("%v", v)
	s = strings.ReplaceAll(s, "None", " ")
	return common.RemoveRedundantSpaces(s)
}

func extractSourceColumnIndexes(rows []map[string]interface{}) (docIDIdx, docNameIdx, kbIDIdx []int, columns []string) {
	if len(rows) == 0 {
		return
	}
	for k := range rows[0] {
		columns = append(columns, k)
	}
	sort.Strings(columns)
	for i, c := range columns {
		switch strings.ToLower(c) {
		case "doc_id":
			docIDIdx = append(docIDIdx, i)
		case "docnm_kwd", "docnm":
			docNameIdx = append(docNameIdx, i)
		case "kb_id", "kb_id_kwd":
			kbIDIdx = append(kbIDIdx, i)
		}
	}
	return
}

func toIfaceSlice(maps []map[string]interface{}) []interface{} {
	out := make([]interface{}, len(maps))
	for i, m := range maps {
		out[i] = m
	}
	return out
}

// fetchAggregateChunks executes a bounded citation-column query derived directly
// from the validated aggregate query's Scope and Where predicate.
func (s *ChatPipelineService) fetchAggregateChunks(
	ctx context.Context,
	docEngine engine.DocEngine,
	vq *tablequery.ValidatedQuery,
) (chunks []map[string]interface{}, docAggs []map[string]interface{}) {
	if docEngine == nil || vq == nil {
		return nil, nil
	}
	scope := vq.Scope()
	if len(scope.KBIDs) > 1 && docEngine.GetType() == string(engine.EngineInfinity) {
		common.Debug("Table retrieval: skipping aggregate secondary fetch on Infinity multi-KB",
			zap.Strings("kb_ids", scope.KBIDs))
		return nil, nil
	}

	citeQuery, ok := vq.DeriveCitationQuery(tablequery.DefaultCitationLimit)
	if !ok {
		common.Debug("Table retrieval: aggregate secondary fetch skipped (no WHERE predicate)")
		return nil, nil
	}

	rows, err := docEngine.ExecuteTableQuery(ctx, citeQuery)
	if err != nil {
		common.Warn("Table retrieval: aggregate secondary fetch failed", zap.Error(err))
		return nil, nil
	}
	if len(rows) == 0 {
		return nil, nil
	}

	docIDIdx, docNameIdx, kbIDIdx, columns := extractSourceColumnIndexes(rows)
	if len(docIDIdx) == 0 || len(docNameIdx) == 0 {
		common.Warn("Table retrieval: aggregate secondary fetch missing source columns",
			zap.Any("columns", columns))
		return nil, nil
	}

	kbIDs := scope.KBIDs
	chunks = make([]map[string]interface{}, 0, len(rows))
	docAggMap := map[string]map[string]interface{}{}
	for _, r := range rows {
		docID := r[columns[docIDIdx[0]]]
		docName := r[columns[docNameIdx[0]]]
		chunk := map[string]interface{}{"doc_id": docID, "docnm_kwd": docName}
		kid := chunkKBIDForDoc(r, kbIDs, docID)
		if kid == "" && len(kbIDIdx) > 0 {
			if v := r[columns[kbIDIdx[0]]]; v != nil && v != "" {
				kid = fmt.Sprintf("%v", v)
			}
		}
		if kid != "" {
			chunk["kb_id"] = kid
		}
		chunks = append(chunks, chunk)

		if entry, exists := docAggMap[fmt.Sprintf("%v", docID)]; exists {
			entry["count"] = entry["count"].(int) + 1
		} else {
			docAggMap[fmt.Sprintf("%v", docID)] = map[string]interface{}{
				"doc_name": docName,
				"count":    1,
			}
		}
	}

	docAggs = make([]map[string]interface{}, 0, len(docAggMap))
	for did, d := range docAggMap {
		docAggs = append(docAggs, map[string]interface{}{
			"doc_id":   did,
			"doc_name": d["doc_name"],
			"count":    d["count"],
		})
	}
	common.Debug("Table retrieval: aggregate secondary fetch produced chunks",
		zap.Int("chunks", len(chunks)),
		zap.Int("doc_aggs", len(docAggs)))
	return chunks, docAggs
}

// buildSQLReference renders the Markdown table answer and assembles citation
// references (chunks + doc_aggs) for a table query result.
func (s *ChatPipelineService) buildSQLReference(
	ctx context.Context,
	docEngine engine.DocEngine,
	vq *tablequery.ValidatedQuery,
	rows []map[string]interface{},
	fieldMap map[string]interface{},
	quote bool,
) (string, map[string]interface{}) {
	if len(rows) == 0 {
		if !quote {
			return "No results.", map[string]interface{}{}
		}
		return "No results.", map[string]interface{}{
			"chunks":   []map[string]interface{}{},
			"doc_aggs": []interface{}{},
			"total":    0,
		}
	}

	// Scalar shortcut: single-cell result renders directly without a table.
	if len(rows) == 1 && len(rows[0]) == 1 {
		for _, v := range rows[0] {
			if !quote {
				return cleanCellValue(v), map[string]interface{}{}
			}
			return cleanCellValue(v), map[string]interface{}{
				"chunks":   []map[string]interface{}{},
				"doc_aggs": []interface{}{},
				"total":    1,
			}
		}
	}

	var kbIDs []string
	if vq != nil {
		kbIDs = vq.Scope().KBIDs
	}

	docIDIdx, docNameIdx, kbIDIdx, columns := extractSourceColumnIndexes(rows)
	hasSrc := len(docIDIdx) > 0 && len(docNameIdx) > 0
	showSource := quote && hasSrc

	exclude := map[int]bool{}
	for _, i := range docIDIdx {
		exclude[i] = true
	}
	for _, i := range docNameIdx {
		exclude[i] = true
	}
	for _, i := range kbIDIdx {
		exclude[i] = true
	}
	displayCols := make([]int, 0, len(columns))
	for i := range columns {
		if !exclude[i] {
			displayCols = append(displayCols, i)
		}
	}

	var header strings.Builder
	header.WriteString("|")
	for _, i := range displayCols {
		header.WriteString(mapColumnName(columns[i], fieldMap))
		header.WriteString("|")
	}
	if showSource {
		header.WriteString("Source|")
	}

	sep := strings.Repeat("|------", len(displayCols)) + "|"
	if showSource {
		sep += "------|"
	}

	bodyRows := make([]string, 0, len(rows))
	for rowIdx, r := range rows {
		var cells strings.Builder
		cells.WriteString("|")
		for _, i := range displayCols {
			cells.WriteString(cleanCellValue(r[columns[i]]))
			cells.WriteString("|")
		}
		if showSource {
			cells.WriteString(fmt.Sprintf(" ##%d$$|", rowIdx))
		}
		rowStr := cells.String()
		if strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(rowStr, "|", ""), " ", "")) != "" {
			bodyRows = append(bodyRows, rowStr)
		}
	}
	rowsJoined := stripISOTimestamps(strings.Join(bodyRows, "\n"))

	answer := strings.Join([]string{header.String(), sep, rowsJoined}, "\n")
	if !quote {
		return answer, map[string]interface{}{}
	}

	ref := map[string]interface{}{
		"chunks":   []map[string]interface{}{},
		"doc_aggs": []interface{}{},
		"total":    len(rows),
	}

	if hasSrc {
		chunks := make([]map[string]interface{}, 0, len(rows))
		docAggMap := map[string]map[string]interface{}{}
		for _, r := range rows {
			did := r[columns[docIDIdx[0]]]
			dn := r[columns[docNameIdx[0]]]
			entry := map[string]interface{}{"doc_id": did, "docnm_kwd": dn}
			if kid := chunkKBIDForDoc(r, kbIDs, did); kid != "" {
				entry["kb_id"] = kid
			} else if len(kbIDIdx) > 0 {
				if v := r[columns[kbIDIdx[0]]]; v != nil && v != "" {
					entry["kb_id"] = fmt.Sprintf("%v", v)
				}
			}
			chunks = append(chunks, entry)

			docIDKey := fmt.Sprintf("%v", did)
			if e, ok := docAggMap[docIDKey]; ok {
				e["count"] = e["count"].(int) + 1
			} else {
				docAggMap[docIDKey] = map[string]interface{}{
					"doc_name": dn,
					"count":    1,
				}
			}
		}
		docAggs := make([]map[string]interface{}, 0, len(docAggMap))
		for did, d := range docAggMap {
			docAggs = append(docAggs, map[string]interface{}{
				"doc_id":   did,
				"doc_name": d["doc_name"],
				"count":    d["count"],
			})
		}
		ref["chunks"] = chunksFormat(chunks)
		ref["doc_aggs"] = docAggs
		return answer, ref
	}

	if vq != nil && vq.IsAggregate() {
		chunks, docAggs := s.fetchAggregateChunks(ctx, docEngine, vq)
		if len(chunks) > 0 {
			ref["chunks"] = chunksFormat(chunks)
			ref["doc_aggs"] = docAggs
		}
		return answer, ref
	}

	common.Debug("Table retrieval: non-aggregate result missing source columns; returning best-effort answer")
	return answer, ref
}
