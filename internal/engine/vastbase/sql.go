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

package vastbase

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

var sqlLimitPattern = regexp.MustCompile(`(?i)\blimit\b`)

// RunSQL executes the read-only SQL produced by the chat SQL-retrieval flow.
// The service layer injects the kb_id filter before the SQL reaches the
// engine (shared tables are not auto-scoped here).
func (e *Engine) RunSQL(ctx context.Context, tableName, sqlText string, kbIDs []string, format string) ([]map[string]interface{}, error) {
	if tableName != "" {
		if err := validateIdentifier(tableName); err != nil {
			return nil, err
		}
	}
	normalized := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(sqlText), ";"))
	if strings.Contains(normalized, ";") {
		return nil, fmt.Errorf("multiple statements are not allowed")
	}
	lower := strings.ToLower(normalized)
	if !strings.HasPrefix(lower, "select ") && !strings.HasPrefix(lower, "with ") {
		return nil, fmt.Errorf("only SELECT and WITH statements are allowed")
	}
	if !sqlLimitPattern.MatchString(normalized) {
		normalized += " LIMIT 1024"
	}
	return e.queryRows(ctx, normalized)
}
