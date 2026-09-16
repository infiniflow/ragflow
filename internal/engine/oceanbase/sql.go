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
	"regexp"
	"strings"
)

var (
	jsonExtractStringPattern = regexp.MustCompile(`(?i)json_extract_string\s*\(\s*([^,]+?)\s*,\s*([^)]+?)\s*\)`)
	jsonExtractNullPattern   = regexp.MustCompile(`(?i)json_extract_isnull\s*\(\s*([^,]+?)\s*,\s*([^)]+?)\s*\)`)
	limitPattern             = regexp.MustCompile(`(?i)\blimit\b`)
)

// RunSQL executes the read-only SQL produced by the chat SQL-retrieval flow.
func (e *Engine) RunSQL(ctx context.Context, tableName, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	if tableName != "" {
		if err := validateIdentifier(tableName); err != nil {
			return nil, err
		}
	}
	normalized := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sqlText), ";"))
	normalized = strings.ReplaceAll(normalized, "`", "")
	normalized = jsonExtractStringPattern.ReplaceAllString(normalized, "JSON_UNQUOTE(JSON_EXTRACT($1, $2))")
	normalized = jsonExtractNullPattern.ReplaceAllString(normalized, "(JSON_EXTRACT($1, $2) IS NULL)")
	lower := strings.ToLower(strings.TrimSpace(normalized))
	if !strings.HasPrefix(lower, "select ") && !strings.HasPrefix(lower, "with ") {
		return nil, fmt.Errorf("only SELECT and WITH statements are allowed")
	}
	if !limitPattern.MatchString(normalized) {
		normalized += " LIMIT 1024"
	}
	return e.queryRows(ctx, normalized)
}
