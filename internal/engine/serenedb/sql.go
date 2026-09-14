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
	"fmt"
	"regexp"
	"strings"
)

const runSQLDefaultLimit = 1024

var (
	selectPrefixRe = regexp.MustCompile(`(?i)^(select|with)\b`)
	hasLimitRe     = regexp.MustCompile(`(?i)\blimit\b`)
)

// boundRunSQL trims a statement and appends a default LIMIT to unbounded
// SELECT/WITH queries so the text-to-SQL path cannot stream an entire table.
// Pure for tests.
func boundRunSQL(sqlText string, limit int) string {
	txt := strings.TrimSpace(sqlText)
	txt = strings.TrimRight(txt, ";")
	if limit > 0 && selectPrefixRe.MatchString(txt) && !hasLimitRe.MatchString(txt) {
		txt = fmt.Sprintf("%s LIMIT %d", txt, limit)
	}
	return txt
}

// RunSQL executes a text-to-SQL query directly against SereneDB and returns the
// rows as maps. SereneDB speaks SQL natively, so unlike the Infinity engine no
// psql subprocess or field-alias rewrite is needed.
func (e *serenedbEngine) RunSQL(ctx context.Context, tableName string, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	query := boundRunSQL(sqlText, runSQLDefaultLimit)
	rows, err := e.queryMaps(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("serenedb: run sql: %w", err)
	}
	if rows == nil {
		rows = []map[string]interface{}{}
	}
	return rows, nil
}
