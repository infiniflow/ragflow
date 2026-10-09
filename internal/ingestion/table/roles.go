package table

import (
	"fmt"
	"strings"
)

// Column modes. auto indexes every column into both body text and chunk_data
// without document-level column metadata; manual routes columns by their
// configured role.
const (
	ModeAuto   = "auto"
	ModeManual = "manual"
)

// Column roles for manual mode. indexing puts the column in body text only;
// metadata puts it in chunk_data and document aggregation; both does all
// three.
const (
	RoleIndexing = "indexing"
	RoleMetadata = "metadata"
	RoleBoth     = "both"
)

// ValidRole reports whether role is one of the supported column roles.
func ValidRole(role string) bool {
	switch role {
	case RoleIndexing, RoleMetadata, RoleBoth:
		return true
	}
	return false
}

// ValidateMode checks a raw column_mode value from configuration. Wrong types
// and unknown values are errors; nothing is silently dropped.
func ValidateMode(v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("column_mode must be a string, got %T", v)
	}
	switch s {
	case ModeAuto, ModeManual:
		return s, nil
	default:
		return "", fmt.Errorf("column_mode %q is invalid: only %q and %q are allowed", s, ModeAuto, ModeManual)
	}
}

// ValidateRoles checks a raw column_roles value and returns the typed map.
// Keys must be non-empty normalized column keys; values must be supported
// role strings.
func ValidateRoles(v any) (map[string]string, error) {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("column_roles must be a JSON object, got %T", v)
	}
	roles := make(map[string]string, len(raw))
	for key, val := range raw {
		if key == "" {
			return nil, fmt.Errorf("column_roles contains an empty column key")
		}
		role, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("column_roles[%q] must be a string, got %T", key, val)
		}
		if !ValidRole(role) {
			return nil, fmt.Errorf("column_roles[%q] has an invalid role %q: allowed roles are %s",
				key, role, strings.Join([]string{RoleIndexing, RoleMetadata, RoleBoth}, ", "))
		}
		roles[key] = role
	}
	return roles, nil
}
