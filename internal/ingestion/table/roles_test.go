package table

import (
	"strings"
	"testing"
)

func TestValidateMode(t *testing.T) {
	for _, ok := range []string{ModeAuto, ModeManual} {
		got, err := ValidateMode(ok)
		if err != nil || got != ok {
			t.Errorf("ValidateMode(%q) = %q, %v", ok, got, err)
		}
	}
	bad := []struct {
		name string
		v    any
	}{
		{"unknown", "assist"},
		{"empty", ""},
		{"number", 100},
		{"null", nil},
		{"bool", true},
	}
	for _, c := range bad {
		if _, err := ValidateMode(c.v); err == nil {
			t.Errorf("ValidateMode(%v) [%s]: expected error, got nil", c.v, c.name)
		}
	}
}

func TestValidateRoles(t *testing.T) {
	raw := map[string]any{"名称": "indexing", "金额": "metadata", "编号": "both"}
	roles, err := ValidateRoles(raw)
	if err != nil {
		t.Fatalf("ValidateRoles: %v", err)
	}
	if roles["名称"] != RoleIndexing || roles["金额"] != RoleMetadata || roles["编号"] != RoleBoth {
		t.Errorf("unexpected roles: %+v", roles)
	}
	if r, err := ValidateRoles(map[string]any{}); err != nil || len(r) != 0 {
		t.Errorf("empty object: %v, %v", r, err)
	}
	bad := []struct {
		name string
		v    any
	}{
		{"unknown role", map[string]any{"名称": "keyword"}},
		{"non-string value", map[string]any{"名称": 1}},
		{"null value", map[string]any{"名称": nil}},
		{"empty key", map[string]any{"": "both"}},
		{"not an object", "manual"},
		{"null", nil},
		{"array", []any{"名称"}},
	}
	for _, c := range bad {
		if _, err := ValidateRoles(c.v); err == nil {
			t.Errorf("ValidateRoles(%v) [%s]: expected error, got nil", c.v, c.name)
		}
	}
}

func TestCanonicalProfile(t *testing.T) {
	// auto ignores saved roles.
	if CanonicalProfile(ModeAuto, map[string]string{"金额": RoleMetadata}) !=
		CanonicalProfile(ModeAuto, nil) {
		t.Error("auto profile should not keep roles")
	}
	// manual keeps only explicit roles and is order-stable.
	a := CanonicalProfile(ModeManual, map[string]string{"b": RoleBoth, "a": RoleIndexing})
	b := CanonicalProfile(ModeManual, map[string]string{"a": RoleIndexing, "b": RoleBoth})
	if a != b {
		t.Errorf("profile not stable: %q vs %q", a, b)
	}
	if !strings.Contains(a, `"a":"indexing","b":"both"`) {
		t.Errorf("unexpected canonical JSON: %q", a)
	}
	// Explicit both must differ from unset (which also behaves as both).
	if CanonicalProfile(ModeManual, map[string]string{"a": RoleBoth}) ==
		CanonicalProfile(ModeManual, nil) {
		t.Error("explicit both collapsed into default both")
	}
	// Keys containing quotes/backslashes survive escaping without breaking
	// the JSON shape.
	esc := CanonicalProfile(ModeManual, map[string]string{`a"b\c`: RoleMetadata})
	if !strings.Contains(esc, `a\"b\\c`) {
		t.Errorf("key not escaped: %q", esc)
	}
}
