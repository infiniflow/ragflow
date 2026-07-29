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

package tool

import (
	"context"
	"errors"
	"testing"
)

// TestExeSQL_TrinoDriverMissing verifies Trino is now routed through the
// Trino DSN path. In this workspace we do not register a real "trino"
// database/sql driver, so InvokableRun should fail at sql.Open with an
// unknown-driver error rather than the old unsupported-db sentinel.
func TestExeSQL_TrinoDriverMissing(t *testing.T) {
	conn := exesqlConnParams{DBType: "trino", Host: "1.1.1.1", Port: 8080, Database: "d", Username: "u"}
	tool := NewExeSQLTool(conn)
	_, err := tool.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err == nil {
		t.Fatal("expected driver error for trino")
	}
	if errors.Is(err, ErrExeSQLUnsupportedDB) {
		t.Fatalf("err=%v, did not want ErrExeSQLUnsupportedDB after trino wiring", err)
	}
}

// TestExeSQL_IBMDB2Unsupported: same as above for IBM DB2.
func TestExeSQL_IBMDB2Unsupported(t *testing.T) {
	conn := exesqlConnParams{DBType: "ibm db2", Host: "1.1.1.1", Port: 50000, Database: "d", Username: "u"}
	tool := NewExeSQLTool(conn)
	_, err := tool.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err == nil {
		t.Fatal("expected ErrExeSQLUnsupportedDB for ibm db2")
	}
	if !errors.Is(err, ErrExeSQLUnsupportedDB) {
		t.Errorf("err=%v, want ErrExeSQLUnsupportedDB", err)
	}
}

// TestExeSQL_UnknownDB: an unrecognised db_type returns a clear error
// (not a panic). Today this surfaces a plain "unknown db_type"
// error string rather than wrapping ErrExeSQLUnsupportedDB; a
// follow-up should normalize the error. The regression guard here
// is "doesn't panic, returns a non-nil error".
func TestExeSQL_UnknownDB(t *testing.T) {
	conn := exesqlConnParams{DBType: "fake-db", Host: "1.1.1.1", Port: 1234, Database: "d", Username: "u"}
	tool := NewExeSQLTool(conn)
	_, err := tool.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err == nil {
		t.Fatal("expected error for unknown db_type")
	}
	// Note: today this returns "ExeSQL: unknown db_type ..." rather
	// than ErrExeSQLUnsupportedDB. A follow-up should wrap; the
	// regression guard just ensures the call returns an error and
	// doesn't panic.
}
