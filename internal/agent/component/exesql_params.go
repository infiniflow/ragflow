// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package component

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// exeSQLDBTypes is the whitelist enforced at save time for an ExeSQL
// component's db_type. It mirrors Python ExeSQLParam.check
// (agent/tools/exesql.py), where ExeSQLParam defaults db_type to "mysql"
// when the DSL omits it.
var exeSQLDBTypes = []string{"mysql", "postgres", "mariadb", "mssql", "IBM DB2", "trino", "oceanbase"}

// validateExeSQLParams enforces the required connection settings of an
// ExeSQL component at save time. The Go agent PUT path validates DSL
// component parameters through validateDynamicParams; Python performs the
// same checks via Canvas.validate_component_parameters ->
// ExeSQLParam.check. Without them a canvas can be persisted with a broken
// DB tool that only fails at runtime.
func validateExeSQLParams(params map[string]any) error {
	dbType, explicit := params["db_type"]
	if !explicit {
		// Python default when the DSL omits db_type.
		dbType = "mysql"
	}
	dbTypeString, ok := dbType.(string)
	if !ok || !exeSQLDBTypeAllowed(dbTypeString) {
		return fmt.Errorf("Choose DB type %v is not supported, it should be in %v", dbType, exeSQLDBTypes)
	}

	// Connection identity fields have no Python defaults and must be present.
	for _, field := range []struct {
		key  string
		desc string
	}{
		{"database", "Database name"},
		{"username", "database username"},
		{"host", "IP Address"},
	} {
		if !isNonBlankString(params[field.key]) {
			return fmt.Errorf("%s does not support empty value", field.desc)
		}
	}

	if port, present := params["port"]; present {
		if !isPositiveInteger(port) {
			return fmt.Errorf("IP Port %v not supported, should be positive integer", port)
		}
	}

	// Trino connects without credentials; every other supported DB type
	// requires a password.
	if dbTypeString != "trino" && !isNonBlankString(params["password"]) {
		return fmt.Errorf("Database password does not support empty value")
	}

	if maxRecords, present := params["max_records"]; present {
		if !isPositiveInteger(maxRecords) {
			return fmt.Errorf("Maximum number of records %v not supported, should be positive integer", maxRecords)
		}
	}

	// Refuse configs aimed at RAGFlow's own metadata database, mirroring
	// Python ExeSQLParam.check's security guard.
	if trimmedDBString(params["database"]) == "rag_flow" &&
		(trimmedDBString(params["host"]) == "ragflow-mysql" || trimmedDBString(params["password"]) == "infini_rag_flow") {
		return fmt.Errorf("For the security reason, it does not support database named rag_flow.")
	}
	return nil
}

// exeSQLDBTypeAllowed reports whether dbType is one of exeSQLDBTypes.
func exeSQLDBTypeAllowed(dbType string) bool {
	for _, candidate := range exeSQLDBTypes {
		if candidate == dbType {
			return true
		}
	}
	return false
}

// trimmedDBString returns the whitespace-trimmed value of a string-ish
// parameter, or "" when it is absent or not a string.
func trimmedDBString(value any) string {
	s, _ := value.(string)
	return strings.TrimSpace(s)
}

// isPositiveInteger reports whether value is a whole number greater than
// zero. json.Number lexemes survive the JSON decode and whole-valued
// float64/float32 are accepted for consistency with isIntegerNumber.
func isPositiveInteger(value any) bool {
	if !isIntegerNumber(value) {
		return false
	}
	switch n := value.(type) {
	case json.Number:
		i, err := n.Int64()
		return err == nil && i > 0
	case float64:
		return n > 0
	case float32:
		return n > 0
	default:
		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return v.Int() > 0
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return v.Uint() > 0
		default:
			return false
		}
	}
}
