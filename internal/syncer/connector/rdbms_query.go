//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package connector

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"ragflow/internal/utility/sqllex"
)

type rdbmsSyncMode int

const (
	rdbmsVerbatim rdbmsSyncMode = iota
	rdbmsOrderByID
	rdbmsTimeWindow
)

func (r SyncRequest) rdbmsMode(idColumn, timestampColumn string) rdbmsSyncMode {
	switch {
	case r.FromBeginning && idColumn != "":
		return rdbmsOrderByID
	case !r.FromBeginning && timestampColumn != "":
		return rdbmsTimeWindow
	default:
		return rdbmsVerbatim
	}
}

type rdbmsSessionDB interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	Close() error
}

type rdbmsSessionConn struct {
	*sql.Conn
	pool *sql.DB
}

func (c *rdbmsSessionConn) Close() error {
	connErr := c.Conn.Close()
	if errors.Is(connErr, sql.ErrConnDone) {
		connErr = nil
	}
	return errors.Join(connErr, c.pool.Close())
}

// openRDBMSQueries prepares the source SQL once for the sync or prune session.
func openRDBMSQueries(
	ctx context.Context, open func() (*sql.DB, error),
	tables func(context.Context, *sql.DB) ([]rdbmsQuery, error),
	custom string, dialect sqllex.Dialect, wrapped bool,
) (rdbmsSessionDB, []rdbmsQuery, error) {
	pool, err := open()
	if err != nil {
		return nil, nil, err
	}
	if custom == "" {
		bases, err := tables(ctx, pool)
		if err != nil {
			pool.Close()
			return nil, nil, err
		}
		return pool, bases, nil
	}
	var db rdbmsSessionDB = pool
	if wrapped {
		mode := ""
		// Read the string escape rules on the same connection that executes
		// the query when a backslash can affect delimiter recognition.
		if strings.Contains(custom, ";") && strings.Contains(custom, "\\") {
			conn, err := pool.Conn(ctx)
			if err != nil {
				pool.Close()
				return nil, nil, err
			}
			db = &rdbmsSessionConn{Conn: conn, pool: pool}
			setting := "SELECT @@SESSION.sql_mode"
			if dialect == sqllex.PostgreSQL {
				setting = "SHOW standard_conforming_strings"
			}
			if err := conn.QueryRowContext(ctx, setting).Scan(&mode); err != nil {
				db.Close()
				return nil, nil, fmt.Errorf("read SQL string quoting mode: %w", err)
			}
		}
		custom = dialect.TrimTerminator(strings.TrimSpace(custom), mode)
	}
	return db, []rdbmsQuery{{sql: custom}}, nil
}

// wrapRDBMSQuery preserves the source query's row selection inside the derived
// table. The newline terminates a trailing SQL line comment.
func wrapRDBMSQuery(base, projection string) string {
	return fmt.Sprintf("SELECT %s FROM (%s\n) AS ragflow_src", projection, base)
}
