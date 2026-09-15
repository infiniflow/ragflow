package component

import (
	"strings"
	"testing"
)

func TestValidateDynamicEntriesExeSQL(t *testing.T) {
	valid := dslWithComponents(
		componentDSL("ExeSQL", map[string]any{
			"db_type":     "mysql",
			"database":    "orders",
			"username":    "report",
			"host":        "10.0.0.5",
			"port":        3306,
			"password":    "secret",
			"max_records": 100,
		}),
	)
	if err := ValidateDynamicEntries(valid); err != nil {
		t.Fatalf("valid ExeSQL DSL rejected: %v", err)
	}

	// db_type is optional: ExeSQLParam defaults it to "mysql".
	noDBType := dslWithComponents(componentDSL("ExeSQL", map[string]any{
		"database": "orders",
		"username": "report",
		"host":     "10.0.0.5",
		"password": "secret",
	}))
	if err := ValidateDynamicEntries(noDBType); err != nil {
		t.Fatalf("ExeSQL DSL without db_type rejected: %v", err)
	}

	// Trino connects without credentials.
	trino := dslWithComponents(componentDSL("ExeSQL", map[string]any{
		"db_type":  "trino",
		"database": "catalog",
		"username": "report",
		"host":     "10.0.0.5",
		"password": "",
	}))
	if err := ValidateDynamicEntries(trino); err != nil {
		t.Fatalf("valid trino ExeSQL DSL rejected: %v", err)
	}

	tests := []struct {
		name      string
		component map[string]any
		want      string
	}{
		{
			"unsupported db_type",
			componentDSL("ExeSQL", map[string]any{"db_type": "oracle", "database": "d", "username": "u", "host": "h", "password": "p"}),
			"Choose DB type oracle is not supported, it should be in",
		},
		{
			"non-string db_type",
			componentDSL("ExeSQL", map[string]any{"db_type": 1, "database": "d", "username": "u", "host": "h", "password": "p"}),
			"Choose DB type 1 is not supported, it should be in",
		},
		{
			"empty database",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "", "username": "u", "host": "h", "password": "p"}),
			"[ExeSQL] Database name does not support empty value",
		},
		{
			"missing database",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "username": "u", "host": "h", "password": "p"}),
			"[ExeSQL] Database name does not support empty value",
		},
		{
			"empty username",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "d", "username": "", "host": "h", "password": "p"}),
			"[ExeSQL] database username does not support empty value",
		},
		{
			"empty host",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "d", "username": "u", "host": " ", "password": "p"}),
			"[ExeSQL] IP Address does not support empty value",
		},
		{
			"missing password",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "d", "username": "u", "host": "h"}),
			"[ExeSQL] Database password does not support empty value",
		},
		{
			"zero port",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "d", "username": "u", "host": "h", "password": "p", "port": 0}),
			"[ExeSQL] IP Port 0 not supported, should be positive integer",
		},
		{
			"fractional port",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "d", "username": "u", "host": "h", "password": "p", "port": 3306.5}),
			"[ExeSQL] IP Port 3306.5 not supported, should be positive integer",
		},
		{
			"negative max_records",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "d", "username": "u", "host": "h", "password": "p", "max_records": -1}),
			"[ExeSQL] Maximum number of records -1 not supported, should be positive integer",
		},
		{
			"rag_flow database with default mysql host",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "rag_flow", "username": "u", "host": "ragflow-mysql", "password": "secret"}),
			"[ExeSQL] For the security reason, it does not support database named rag_flow.",
		},
		{
			"rag_flow database with default mysql password",
			componentDSL("ExeSQL", map[string]any{"db_type": "mysql", "database": "rag_flow", "username": "u", "host": "10.0.0.5", "password": "infini_rag_flow"}),
			"[ExeSQL] For the security reason, it does not support database named rag_flow.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDynamicEntries(dslWithComponents(tt.component))
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.want)
			}
		})
	}
}
