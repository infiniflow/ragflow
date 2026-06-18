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
	"testing"
)

// TestNewExeSQLConnParams_RequiredFields: missing any required field
// returns an error.
func TestNewExeSQLConnParams_RequiredFields(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
	}{
		{"empty", map[string]any{}},
		{"only db_type", map[string]any{"db_type": "mysql"}},
		{"missing host", map[string]any{"db_type": "mysql", "database": "d", "username": "u"}},
		{"missing database", map[string]any{"db_type": "mysql", "host": "h", "username": "u"}},
		{"missing username", map[string]any{"db_type": "mysql", "host": "h", "database": "d"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewExeSQLConnParams(tc.params)
			if err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

// TestNewExeSQLConnParams_AllFields: full params map decodes correctly.
func TestNewExeSQLConnParams_AllFields(t *testing.T) {
	conn, err := NewExeSQLConnParams(map[string]any{
		"db_type":     "postgres",
		"host":        "db.example.com",
		"port":        5432,
		"database":    "ragflow",
		"username":    "rag",
		"password":    "secret",
		"max_records": 100,
	})
	if err != nil {
		t.Fatalf("NewExeSQLConnParams: %v", err)
	}
	if conn.DBType != "postgres" {
		t.Errorf("DBType=%q, want postgres", conn.DBType)
	}
	if conn.Host != "db.example.com" {
		t.Errorf("Host=%q, want db.example.com", conn.Host)
	}
	if conn.Port != 5432 {
		t.Errorf("Port=%d, want 5432", conn.Port)
	}
	if conn.Database != "ragflow" {
		t.Errorf("Database=%q, want ragflow", conn.Database)
	}
	if conn.Username != "rag" {
		t.Errorf("Username=%q, want rag", conn.Username)
	}
	if conn.Password != "secret" {
		t.Errorf("Password=%q, want secret", conn.Password)
	}
	if conn.MaxRecords != 100 {
		t.Errorf("MaxRecords=%d, want 100", conn.MaxRecords)
	}
}

// TestExeSQLConnParams_Alias: the public type alias ExeSQLConnParams
// refers to the same underlying type as the lowercase exesqlConnParams.
// The factory returns the public name, and existing in-package
// callers can keep using the lowercase name (Go type alias
// compatibility).
func TestExeSQLConnParams_Alias(t *testing.T) {
	var a ExeSQLConnParams
	var b exesqlConnParams
	// Both are assignable to each other via the alias.
	a = ExeSQLConnParams(b)
	b = exesqlConnParams(a)
	_ = a
	_ = b
}
