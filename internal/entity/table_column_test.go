package entity

import (
	"regexp"
	"testing"
)

func TestValidateTableRolesReservesProfileKey(t *testing.T) {
	for _, role := range []string{TableRoleMetadata, TableRoleBoth} {
		if _, err := ValidateTableRoles(map[string]any{"_table_profile": role}); err == nil {
			t.Errorf("role %q accepted a column that overwrites the system profile", role)
		}
	}
	if _, err := ValidateTableRoles(map[string]any{"_table_profile": TableRoleIndexing}); err != nil {
		t.Fatalf("body-only column must remain usable: %v", err)
	}
}

func TestDeriveColumnsKeys(t *testing.T) {
	// Raw "名称#2" must not collide with the second "名称", and an empty
	// header must not collide with the raw header "#column:1".
	cols := DeriveTableColumns([]string{"名称", "名称", `名称#2`, "", "#column:1", "名称"})
	wantKeys := []string{"名称", "名称#2", `名称\#2`, "#column:4", `\#column:1`, "名称#3"}
	wantDisplay := []string{"名称", "名称 (2)", "名称#2", "列 4", "#column:1", "名称 (3)"}
	if len(cols) != len(wantKeys) {
		t.Fatalf("got %d columns, want %d", len(cols), len(wantKeys))
	}
	for i, c := range cols {
		if c.Key != wantKeys[i] {
			t.Errorf("column %d: key = %q, want %q", i+1, c.Key, wantKeys[i])
		}
		if c.DisplayName != wantDisplay[i] {
			t.Errorf("column %d: display = %q, want %q", i+1, c.DisplayName, wantDisplay[i])
		}
		if c.Index != i+1 {
			t.Errorf("column %d: index = %d", i+1, c.Index)
		}
	}
}

func TestDeriveColumnsWhitespaceHeaderUsesPosition(t *testing.T) {
	cols := DeriveTableColumns([]string{"\t ", "a"})
	if cols[0].Key != "#column:1" || cols[0].DisplayName != "列 1" {
		t.Errorf("whitespace-only header not position-named: %+v", cols[0])
	}
}

func TestDeriveColumnsPositionKeyStableAcrossSheets(t *testing.T) {
	// Column identity must not change when another sheet has more or fewer
	// columns: keys depend only on this header row.
	a := DeriveTableColumns([]string{"", "x", ""})
	b := DeriveTableColumns([]string{"y", "", "x", ""})
	if a[0].Key != "#column:1" || a[2].Key != "#column:3" {
		t.Errorf("unexpected position keys: %+v", a)
	}
	if b[1].Key != "#column:2" || b[3].Key != "#column:4" {
		t.Errorf("unexpected position keys: %+v", b)
	}
}

func TestDataKey(t *testing.T) {
	k1 := TableDataKey("金额")
	k2 := TableDataKey("金额")
	if k1 != k2 {
		t.Errorf("DataKey not deterministic: %q vs %q", k1, k2)
	}
	if k1 == TableDataKey("金额#2") {
		t.Error("different keys share a data key")
	}
	if !regexp.MustCompile(`^c_[a-f0-9]{64}$`).MatchString(k1) {
		t.Errorf("data key %q is not c_+64 hex", k1)
	}
	cols := DeriveTableColumns([]string{"a", "b"})
	for _, c := range cols {
		if c.DataKey != TableDataKey(c.Key) {
			t.Errorf("column %q data key not derived from its key", c.Key)
		}
	}
}

func TestDeriveColumnsNFCDuplicate(t *testing.T) {
	// A decomposed and a precomposed form of the same header are duplicates.
	cols := DeriveTableColumns([]string{"e\u0301", "\u00e9"})
	if cols[0].Key != "\u00e9" || cols[1].Key != "\u00e9#2" {
		t.Errorf("NFC duplicates not suffixed: %q, %q", cols[0].Key, cols[1].Key)
	}
}

func TestValidateMode(t *testing.T) {
	for _, ok := range []string{TableModeAuto, TableModeManual} {
		got, err := ValidateTableMode(ok)
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
		if _, err := ValidateTableMode(c.v); err == nil {
			t.Errorf("ValidateMode(%v) [%s]: expected error, got nil", c.v, c.name)
		}
	}
}

func TestValidateRoles(t *testing.T) {
	raw := map[string]any{"名称": "indexing", "金额": "metadata", "编号": "both"}
	roles, err := ValidateTableRoles(raw)
	if err != nil {
		t.Fatalf("ValidateRoles: %v", err)
	}
	if roles["名称"] != TableRoleIndexing || roles["金额"] != TableRoleMetadata || roles["编号"] != TableRoleBoth {
		t.Errorf("unexpected roles: %+v", roles)
	}
	if r, err := ValidateTableRoles(map[string]any{}); err != nil || len(r) != 0 {
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
		if _, err := ValidateTableRoles(c.v); err == nil {
			t.Errorf("ValidateRoles(%v) [%s]: expected error, got nil", c.v, c.name)
		}
	}
}
