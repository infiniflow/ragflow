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

package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// TestExeSQL_TrinoDSN_ParsesCatalog covers splitTrinoCatalogSchema's
// branch table. Mirrors Python exesql.py:_parse_catalog_schema: the
// first "." or "/" is the catalog/schema boundary, schema defaults to
// "default" when absent, and "a.b.c" splits to ("a", "b.c").
func TestExeSQL_TrinoDSN_ParsesCatalog(t *testing.T) {
	cases := []struct {
		name    string
		db      string
		wantCat string
		wantSch string
	}{
		{"plain", "tpch", "tpch", "default"},
		{"dot", "tpch.tiny", "tpch", "tiny"},
		{"slash", "tpch/tiny", "tpch", "tiny"},
		{"empty", "", "", ""},
		{"multi_dot_first_wins", "a.b.c", "a", "b.c"},
		{"multi_slash_first_wins", "a/b/c", "a", "b/c"},
		{"dot_in_schema", "catalog.my.schema", "catalog", "my.schema"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotCat, gotSch := splitTrinoCatalogSchema(tc.db)
			if gotCat != tc.wantCat || gotSch != tc.wantSch {
				t.Errorf("splitTrinoCatalogSchema(%q) = (%q, %q), want (%q, %q)",
					tc.db, gotCat, gotSch, tc.wantCat, tc.wantSch)
			}
		})
	}
}

// TestExeSQL_TrinoDSN_HappyPath_BasicShape asserts the DSN is well-formed
// for the simplest case: HTTP, no password, port 8080, single-segment
// database name → catalog=tiny, schema=default.
func TestExeSQL_TrinoDSN_HappyPath_BasicShape(t *testing.T) {
	t.Setenv("TRINO_USE_TLS", "")
	dsn := trinoDSN(exesqlConnParams{
		DBType:   "trino",
		Host:     "trino.example.com",
		Port:     8080,
		Username: "alice",
		Database: "tiny",
	})
	if !strings.HasPrefix(dsn, "http://alice@trino.example.com:8080?") {
		t.Errorf("DSN prefix mismatch: %q", dsn)
	}
	if !strings.Contains(dsn, "catalog=tiny") {
		t.Errorf("DSN missing catalog=tiny: %q", dsn)
	}
	if !strings.Contains(dsn, "schema=default") {
		t.Errorf("DSN missing schema=default: %q", dsn)
	}
	if strings.Contains(dsn, "password") || strings.Contains(dsn, ":@") {
		t.Errorf("DSN should not contain a password over plain HTTP: %q", dsn)
	}
}

// TestExeSQL_TrinoDSN_TLSEnv asserts the env-var toggle flips the
// scheme to https. Matches Python exesql.py:167 (TRINO_USE_TLS).
func TestExeSQL_TrinoDSN_TLSEnv(t *testing.T) {
	t.Setenv("TRINO_USE_TLS", "1")
	dsn := trinoDSN(exesqlConnParams{
		DBType:   "trino",
		Host:     "h",
		Port:     443,
		Username: "u",
		Database: "c.s",
	})
	if !strings.HasPrefix(dsn, "https://") {
		t.Errorf("TRINO_USE_TLS=1 should yield https, got: %q", dsn)
	}
}

// TestExeSQL_TrinoDSN_BasicAuthEncoded: over HTTPS, a non-empty password
// is URL-encoded into the userinfo. The trino driver's DSN parser
// accepts the percent-encoded form; the raw form would also parse but
// the percent-encoded contract is safer for special characters.
func TestExeSQL_TrinoDSN_BasicAuthEncoded(t *testing.T) {
	t.Setenv("TRINO_USE_TLS", "1")
	dsn := trinoDSN(exesqlConnParams{
		DBType:   "trino",
		Host:     "h",
		Port:     443,
		Username: "alice",
		Password: "p@ss:word/1",
		Database: "c",
	})
	// Expect https://alice:p%40ss%3Aword%2F1@h:443?...
	if !strings.Contains(dsn, "https://alice:p%40ss%3Aword%2F1@h:443") {
		t.Errorf("encoded userinfo not in DSN: %q", dsn)
	}
}

// TestExeSQL_TrinoDSN_NoPasswordOverHTTP: a password over plain HTTP
// must NOT be in the DSN (Basic auth over http = cleartext leakage).
// The trino driver itself warns against this; Python gates BasicAuth
// on https at exesql.py:169-170. We do the same.
func TestExeSQL_TrinoDSN_NoPasswordOverHTTP(t *testing.T) {
	t.Setenv("TRINO_USE_TLS", "")
	dsn := trinoDSN(exesqlConnParams{
		DBType:   "trino",
		Host:     "h",
		Port:     8080,
		Username: "u",
		Password: "supersecret",
		Database: "c",
	})
	if strings.Contains(dsn, "supersecret") {
		t.Errorf("password must not be encoded over plain HTTP: %q", dsn)
	}
	if !strings.HasPrefix(dsn, "http://u@h:8080") {
		t.Errorf("expected http://u@h:8080, got: %q", dsn)
	}
}

// TestExeSQL_TrinoDSN_DefaultUser covers the "empty username" Python
// quirk: exesql.py:176 defaults the user to "ragflow" if blank.
func TestExeSQL_TrinoDSN_DefaultUser(t *testing.T) {
	t.Setenv("TRINO_USE_TLS", "")
	dsn := trinoDSN(exesqlConnParams{
		DBType:   "trino",
		Host:     "h",
		Port:     8080,
		Username: "",
		Database: "c",
	})
	if !strings.Contains(dsn, "ragflow@") {
		t.Errorf("empty username should default to ragflow, got: %q", dsn)
	}
}

// TestExeSQL_TrinoDSN_DefaultPort asserts the port defaults to 8080
// when unset (Python parity — exesql.py:175 `int(self._param.port or 8080)`).
// This is exercised via exesqlDriverAndDSN's trino case, not the
// trinoDSN helper directly.
func TestExeSQL_TrinoDSN_DefaultPort(t *testing.T) {
	t.Setenv("TRINO_USE_TLS", "")
	driver, dsn, err := exesqlDriverAndDSN(exesqlConnParams{
		DBType:   "trino",
		Host:     "h",
		Port:     0,
		Username: "u",
		Database: "c",
	})
	if err != nil {
		t.Fatalf("exesqlDriverAndDSN: %v", err)
	}
	if driver != "trino" {
		t.Errorf("driver = %q, want %q", driver, "trino")
	}
	if !strings.Contains(dsn, "h:8080") {
		t.Errorf("port default 8080 not applied in DSN: %q", dsn)
	}
}

// TestExeSQL_Trino_HappyPath is the end-to-end smoke test using the
// sqlmock dialer. It verifies the full InvokableRun path against a
// database/sql backend — i.e. it confirms the trino driver name, the
// parsed DSN, and the SELECT-only + row-mapping machinery all chain
// together without a live Trino cluster.
//
// (sqlmockDialer is defined in exesql_test.go; same package, no
// separate import needed.)
func TestExeSQL_Trino_HappyPath(t *testing.T) {
	t.Setenv("TRINO_USE_TLS", "")

	dialer, mock, cleanup := sqlmockDialer(t)
	defer cleanup()

	cols := []string{"id", "name"}
	rows := sqlmock.NewRows(cols).
		AddRow(int64(1), "alpha").
		AddRow(int64(2), "beta")
	mock.ExpectQuery("SELECT id, name FROM catalog.tiny.users").
		WillReturnRows(rows)

	tool := NewExeSQLTool(exesqlConnParams{
		DBType:     "trino",
		Host:       "1.1.1.1",
		Port:       8080,
		Username:   "u",
		Database:   "catalog.tiny",
		MaxRecords: 100,
	}).WithExeSQLDialer(dialer)

	out, err := tool.InvokableRun(context.Background(),
		`{"sql":"SELECT id, name FROM catalog.tiny.users"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, `"alpha"`) || !strings.Contains(out, `"beta"`) {
		t.Errorf("output missing row data: %s", out)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet sqlmock expectations: %v", err)
	}
}
