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

	"ragflow/internal/engine/tablequery"
)

// ExecuteTableQuery compiles a validated TableQuery into parameterized
// PostgreSQL-dialect SQL and executes it inside a READ ONLY transaction that is
// always rolled back.
func (e *Engine) ExecuteTableQuery(ctx context.Context, query *tablequery.ValidatedQuery) ([]map[string]interface{}, error) {
	if e == nil || e.db == nil {
		return nil, fmt.Errorf("vastbase: execute table query: database not initialized")
	}
	compiled, err := tablequery.Compile(query, tablequery.DialectVastbase)
	if err != nil {
		return nil, fmt.Errorf("vastbase: execute table query: %w", err)
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("vastbase: begin read-only sql transaction: %w", err)
	}
	defer tx.Rollback()
	return queryRowsTx(ctx, tx, compiled.SQL, compiled.Args...)
}
