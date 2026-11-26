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

package opengauss

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"ragflow/internal/engine/types"
	"ragflow/internal/tokenizer"
)

// Each score operator needs its own ordered index scan. Combine the weighted
// hits by primary key before projecting document fields or limiting candidates.
func (e *Engine) bm25Scan(ctx context.Context, table, kind string, output []string, filter string, filterArgs []interface{}, text *types.MatchTextExpr, args *sqlArgs) (string, error) {
	fields, weights := parseFullTextFields(text)
	if len(fields) == 0 {
		switch kind {
		case "skill":
			fields = slices.Clone(skillFullTextFields)
		case "memory":
			fields = bm25Fields(kind)
		default:
			fields = []string{"content_ltks"}
		}
		weights = make([]string, len(fields))
		for i := range weights {
			weights[i] = "1"
		}
	}
	keys := identifierColumn(kind)
	if kind == "chunk" {
		keys = "kb_id, id"
	}
	for i, field := range fields {
		if kind == "skill" && !strings.HasSuffix(field, "_tks") {
			field += "_tks"
		}
		if kind == "memory" && field == "content" {
			field = "tokenized_content_ltks"
		}
		if !slices.Contains(bm25Fields(kind), field) {
			return "", fmt.Errorf("opengauss: no BM25 index for %s field %q", kind, field)
		}
		fields[i] = field
	}
	if err := e.ensureBM25Indexes(ctx, table, fields); err != nil {
		return "", err
	}
	query := fullTextQuery(text)
	if kind == "skill" {
		// Skill APIs pass the original query; skill token columns store RAG tokens.
		if tokens, err := tokenizer.Tokenize(query); err == nil {
			query = tokens
		}
	}
	var branches []string
	for i, field := range fields {
		predicate := args.embedFilter(filter, filterArgs)
		score := fmt.Sprintf("%s::text <&> $%d::text", quoteIdent(field), args.add(query))
		if len(fields) == 1 {
			return fmt.Sprintf("SELECT %s, (%s) * %s AS _score FROM %s WHERE %s ORDER BY %s DESC OFFSET 0", selectFieldsSQL(output), score, weights[i], quoteIdent(table), predicate, score), nil
		}
		branch := fmt.Sprintf("SELECT %s, raw_score * %s AS score FROM (SELECT %s, %s AS raw_score FROM %s WHERE %s ORDER BY %s DESC OFFSET 0) AS hits", keys, weights[i], keys, score, quoteIdent(table), predicate, score)
		branches = append(branches, branch)
	}
	fieldsSQL := make([]string, len(output))
	for i, field := range output {
		fieldsSQL[i] = "d." + quoteIdent(field)
		if field == "*" {
			fieldsSQL[i] = "d.*"
		}
	}
	return fmt.Sprintf("SELECT %s, hits._score FROM %s d JOIN (SELECT %s, sum(score)::float8 AS _score FROM (%s) AS terms GROUP BY %s) hits USING (%s) ORDER BY hits._score DESC, %s",
		strings.Join(fieldsSQL, ", "), quoteIdent(table), keys, strings.Join(branches, " UNION ALL "), keys, keys, keys), nil
}
