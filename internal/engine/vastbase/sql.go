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
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

var sqlLimitPattern = regexp.MustCompile(`(?i)\blimit\b`)

// RunSQL executes the read-only SQL produced by the chat SQL-retrieval flow.
// The service layer injects the kb_id filter before the SQL reaches the
// engine (shared tables are not auto-scoped here). The SELECT/WITH prefix
// check is a guardrail, not the read-only control: a data-modifying CTE
// (WITH d AS (DELETE FROM ... RETURNING *) SELECT * FROM d) also starts with
// WITH, so the statement runs inside a READ ONLY transaction the engine
// always rolls back — the server itself rejects writes on that path.
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
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("vastbase: begin read-only sql transaction: %w", err)
	}
	defer tx.Rollback()
	return queryRowsTx(ctx, tx, normalized)
}
