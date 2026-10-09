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

package oceanbase

import (
	"context"
	"fmt"
	"strings"

	"ragflow/internal/sqlscan"
)

func prepareSQL(sqlText string) (string, error) {
	tokens, err := sqlscan.Scan(strings.TrimSuffix(strings.TrimSpace(sqlText), ";"))
	if err != nil {
		return "", err
	}
	shape, err := sqlscan.SplitSelect(tokens)
	if err != nil {
		return "", err
	}
	if _, err := sqlscan.TableReference(shape.Clauses.From); err != nil {
		return "", err
	}
	// Work from the end so nested calls are translated before their parents.
	for i := len(tokens) - 1; i >= 0; i-- {
		if !tokens[i].IsWord("json_extract_string") && !tokens[i].IsWord("json_extract_isnull") {
			continue
		}
		args, next, err := sqlscan.CallArguments(tokens, i)
		if err != nil || len(args) != 2 {
			return "", fmt.Errorf("%s requires two arguments", tokens[i].Text)
		}
		call := "JSON_EXTRACT ( " + sqlscan.Render(args[0], '`') + ", " + sqlscan.Render(args[1], '`') + " )"
		if tokens[i].IsWord("json_extract_string") {
			call = "JSON_UNQUOTE ( " + call + " )"
		} else {
			call = "( " + call + " IS NULL )"
		}
		replacement, err := sqlscan.Scan(call)
		if err != nil {
			return "", err
		}
		tokens = append(append(append([]sqlscan.Token(nil), tokens[:i]...), replacement...), tokens[next:]...)
	}
	normalized := sqlscan.Render(tokens, '`')
	if len(shape.Clauses.Limit) == 0 {
		normalized += " LIMIT 1024"
	}
	return normalized, nil
}

// RunSQL executes the read-only SQL produced by the chat SQL-retrieval flow.
func (e *Engine) RunSQL(ctx context.Context, tableName, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	if tableName != "" {
		if err := validateIdentifier(tableName); err != nil {
			return nil, err
		}
	}
	normalized, err := prepareSQL(sqlText)
	if err != nil {
		return nil, err
	}
	return e.queryRows(ctx, normalized)
}
