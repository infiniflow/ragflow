package table

import (
	"fmt"
	"sort"
	"strings"
)

// NodePrefix is the component-domain key prefix a column configuration lives
// under, e.g. "TableChunker:FastFoxesJump".
const NodePrefix = "TableChunker"

// IsNodeKey reports whether a parser_config key names a TableChunker node.
func IsNodeKey(key string) bool {
	component, _, found := strings.Cut(key, ":")
	return found && component == NodePrefix
}

// ValidateColumnFields checks the column fields wherever they appear. Other
// parameters of the node are left for their own owners: a saved node carries
// outputs, labels and everything else a canvas holds, and refusing those here
// would break unrelated configuration.
func ValidateColumnFields(params map[string]any) (mode string, roles map[string]string, err error) {
	mode = ModeAuto
	roles = map[string]string{}
	if params == nil {
		return mode, roles, nil
	}
	if v, ok := params["column_mode"]; ok {
		mode, err = ValidateMode(v)
		if err != nil {
			return "", nil, err
		}
	}
	if v, ok := params["column_roles"]; ok {
		roles, err = ValidateRoles(v)
		if err != nil {
			return "", nil, err
		}
	}
	return mode, roles, nil
}

// ValidateNodeConfig checks an override that must consist of column fields only:
// unknown keys are refused rather than accepted and ignored, so a client that
// misspells a field hears about it instead of watching the setting do nothing.
func ValidateNodeConfig(params map[string]any) (mode string, roles map[string]string, err error) {
	mode, roles, err = ValidateColumnFields(params)
	if err != nil {
		return "", nil, err
	}
	for key := range params {
		if key != "column_mode" && key != "column_roles" {
			return "", nil, fmt.Errorf("TableChunker node does not accept the parameter %q", key)
		}
	}
	return mode, roles, nil
}

// MergeNodeParams applies a column override onto the parameters a node would
// otherwise run with. Fields the override does not mention keep their previous
// value; column_roles replaces the whole map, so dropping a role is done by
// submitting the map without that column rather than by omitting the field.
//
// Every other parameter of the node — outputs, delimiters, anything a canvas
// carries — survives untouched, because an upload override is about columns,
// not about re-declaring the node.
func MergeNodeParams(base, override map[string]any) (map[string]any, error) {
	if _, _, err := ValidateNodeConfig(override); err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged, nil
}

// IsColumnOnlyConfig reports whether a parser_config carries nothing but table
// column settings. A client that only wants to change roles should not have to
// resend — and therefore not risk rebuilding — the rest of the document's
// component configuration.
func IsColumnOnlyConfig(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	for key, value := range raw {
		if !IsNodeKey(key) {
			return false
		}
		params, ok := value.(map[string]any)
		if !ok {
			return false
		}
		for field := range params {
			if field != "column_mode" && field != "column_roles" {
				return false
			}
		}
	}
	return true
}

// LegacyFlatKeys are the pre-component-scoping upload keys. They are refused
// with an explicit error rather than ignored: a request that carried them used
// to report success while the roles silently did nothing.
var LegacyFlatKeys = []string{"table_column_mode", "table_column_roles", "table_column_names"}

// CheckLegacyFlatKeys reports which legacy keys a request carried.
func CheckLegacyFlatKeys(raw map[string]any) []string {
	found := make([]string, 0, len(LegacyFlatKeys))
	for _, key := range LegacyFlatKeys {
		if _, ok := raw[key]; ok {
			found = append(found, key)
		}
	}
	sort.Strings(found)
	return found
}
