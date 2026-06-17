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

// TestExeSQL_TrinoUnsupported verifies the Trino dialect
// (currently via the trinoDSN stub; see exesql_trino_stub.go) is
// recognized by the unsupported path. The actual driver lands
// when a use-case surfaces.
func TestExeSQL_TrinoUnsupported(t *testing.T) {
	conn := exesqlConnParams{DBType: "trino", Host: "h", Port: 8080, Database: "d", Username: "u"}
	tool := NewExeSQLTool(conn)
	_, err := tool.InvokableRun(context.Background(), `{"sql":"SELECT 1"}`)
	if err == nil {
		t.Fatal("expected ErrExeSQLUnsupportedDB for trino")
	}
	if !errors.Is(err, ErrExeSQLUnsupportedDB) {
		t.Errorf("err=%v, want ErrExeSQLUnsupportedDB", err)
	}
}

// TestExeSQL_IBMDB2Unsupported: same as above for IBM DB2.
func TestExeSQL_IBMDB2Unsupported(t *testing.T) {
	conn := exesqlConnParams{DBType: "ibm db2", Host: "h", Port: 50000, Database: "d", Username: "u"}
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
	conn := exesqlConnParams{DBType: "fake-db", Host: "h", Port: 1234, Database: "d", Username: "u"}
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
