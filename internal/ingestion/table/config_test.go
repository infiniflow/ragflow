package table

import (
	"strings"
	"testing"
)

func TestValidateNodeConfig(t *testing.T) {
	mode, roles, err := ValidateNodeConfig(map[string]any{
		"column_mode":  ModeManual,
		"column_roles": map[string]any{"金额": "metadata"},
	})
	if err != nil {
		t.Fatalf("valid config refused: %v", err)
	}
	if mode != ModeManual || roles["金额"] != RoleMetadata {
		t.Errorf("mode=%q roles=%v", mode, roles)
	}

	// An absent field is the component default, not an error.
	mode, roles, err = ValidateNodeConfig(map[string]any{})
	if err != nil || mode != ModeAuto || len(roles) != 0 {
		t.Errorf("empty config: mode=%q roles=%v err=%v", mode, roles, err)
	}
}

func TestValidateNodeConfigRefusesBadValues(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{"unknown mode", map[string]any{"column_mode": "assist"}, "column_mode"},
		{"non-string mode", map[string]any{"column_mode": 7}, "must be a string"},
		{"unknown role", map[string]any{"column_roles": map[string]any{"金额": "keyword"}}, "invalid role"},
		{"roles not an object", map[string]any{"column_roles": "金额"}, "must be a JSON object"},
		// A misspelled field must not be accepted-and-ignored: the client would
		// believe the role was set.
		{"unknown field", map[string]any{"column_rules": map[string]any{"金额": "metadata"}}, "does not accept the parameter"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := ValidateNodeConfig(c.params)
			if err == nil {
				t.Fatalf("accepted %v", c.params)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

// TestMergeNodeParamsKeepsTheRestOfTheNode: an upload override is about columns,
// so every other parameter of the node has to survive it.
func TestMergeNodeParamsKeepsTheRestOfTheNode(t *testing.T) {
	base := map[string]any{
		"column_mode":     ModeAuto,
		"column_roles":    map[string]string{"旧列": RoleMetadata},
		"enable_children": true,
		"outputs":         map[string]any{"chunks": map[string]any{"type": "Array<Object>"}},
	}
	merged, err := MergeNodeParams(base, map[string]any{"column_mode": ModeManual})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if merged["column_mode"] != ModeManual {
		t.Errorf("column_mode = %v", merged["column_mode"])
	}
	if merged["enable_children"] != true {
		t.Errorf("an unrelated node parameter was lost: %v", merged)
	}
	if _, ok := merged["outputs"]; !ok {
		t.Error("node outputs lost")
	}
	// A role map left out of the override keeps the dataset's roles.
	roles, ok := merged["column_roles"].(map[string]string)
	if !ok || roles["旧列"] != RoleMetadata {
		t.Errorf("column_roles = %v, want the base map kept", merged["column_roles"])
	}
}

// TestMergeNodeParamsReplacesRolesWholesale: removing a role means submitting
// the map without that column, not omitting the field.
func TestMergeNodeParamsReplacesRolesWholesale(t *testing.T) {
	base := map[string]any{
		"column_mode":  ModeManual,
		"column_roles": map[string]any{"金额": "metadata", "编号": "both"},
	}
	merged, err := MergeNodeParams(base, map[string]any{"column_roles": map[string]any{"金额": "indexing"}})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	roles, ok := merged["column_roles"].(map[string]any)
	if !ok {
		t.Fatalf("column_roles type %T", merged["column_roles"])
	}
	if _, stale := roles["编号"]; stale {
		t.Errorf("a dropped role survived: %v", roles)
	}
	if roles["金额"] != "indexing" {
		t.Errorf("column_roles = %v", roles)
	}

	// An empty map clears every role rather than being treated as "no change".
	cleared, err := MergeNodeParams(base, map[string]any{"column_roles": map[string]any{}})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if len(cleared["column_roles"].(map[string]any)) != 0 {
		t.Errorf("empty roles map did not clear: %v", cleared["column_roles"])
	}
}

func TestMergeNodeParamsRefusesBadOverride(t *testing.T) {
	if _, err := MergeNodeParams(map[string]any{}, map[string]any{"column_mode": "nope"}); err == nil {
		t.Fatal("an invalid override was accepted")
	}
}

func TestIsNodeKey(t *testing.T) {
	cases := map[string]bool{
		"TableChunker:FastFoxesJump": true,
		"Parser:FastFoxesJump":       false,
		"TableChunker":               false,
		"tablechunker:X":             false,
	}
	for key, want := range cases {
		if got := IsNodeKey(key); got != want {
			t.Errorf("IsNodeKey(%q) = %v, want %v", key, got, want)
		}
	}
}

func TestCheckLegacyFlatKeys(t *testing.T) {
	found := CheckLegacyFlatKeys(map[string]any{
		"table_column_roles": map[string]any{},
		"table_column_names": []any{},
		"unrelated":          1,
	})
	if strings.Join(found, ",") != "table_column_names,table_column_roles" {
		t.Errorf("found = %v", found)
	}
	if got := CheckLegacyFlatKeys(map[string]any{"TableChunker:X": map[string]any{}}); len(got) != 0 {
		t.Errorf("a component-scoped config reported legacy keys: %v", got)
	}
}
