package table

import (
	"regexp"
	"testing"
)

func TestNormalizeHeader(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"plain", "名称", "名称"},
		{"trim spaces", "  金额\t", "金额"},
		{"trim full-width space", "　金额　", "金额"},
		{"strip bom", "\ufeff金额", "金额"},
		{"nfc compose", "e\u0301", "\u00e9"},
		{"escape backslash", `a\b`, `a\\b`},
		{"escape hash", "名称#2", `名称\#2`},
		{"escape both", `#\`, `\#\\`},
		{"empty stays empty", "   ", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeHeader(c.raw); got != c.want {
				t.Errorf("NormalizeHeader(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

func TestDeriveColumnsKeys(t *testing.T) {
	// Raw "名称#2" must not collide with the second "名称", and an empty
	// header must not collide with the raw header "#column:1".
	cols := DeriveColumns([]string{"名称", "名称", `名称#2`, "", "#column:1", "名称"})
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
	cols := DeriveColumns([]string{"\t ", "a"})
	if cols[0].Key != "#column:1" || cols[0].DisplayName != "列 1" {
		t.Errorf("whitespace-only header not position-named: %+v", cols[0])
	}
}

func TestDeriveColumnsPositionKeyStableAcrossSheets(t *testing.T) {
	// Column identity must not change when another sheet has more or fewer
	// columns: keys depend only on this header row.
	a := DeriveColumns([]string{"", "x", ""})
	b := DeriveColumns([]string{"y", "", "x", ""})
	if a[0].Key != "#column:1" || a[2].Key != "#column:3" {
		t.Errorf("unexpected position keys: %+v", a)
	}
	if b[1].Key != "#column:2" || b[3].Key != "#column:4" {
		t.Errorf("unexpected position keys: %+v", b)
	}
}

func TestDataKey(t *testing.T) {
	k1 := DataKey("金额")
	k2 := DataKey("金额")
	if k1 != k2 {
		t.Errorf("DataKey not deterministic: %q vs %q", k1, k2)
	}
	if k1 == DataKey("金额#2") {
		t.Error("different keys share a data key")
	}
	if !regexp.MustCompile(`^c_[a-f0-9]{64}$`).MatchString(k1) {
		t.Errorf("data key %q is not c_+64 hex", k1)
	}
	cols := DeriveColumns([]string{"a", "b"})
	for _, c := range cols {
		if c.DataKey != DataKey(c.Key) {
			t.Errorf("column %q data key not derived from its key", c.Key)
		}
	}
}

func TestDeriveColumnsNFCDuplicate(t *testing.T) {
	// A decomposed and a precomposed form of the same header are duplicates.
	cols := DeriveColumns([]string{"e\u0301", "\u00e9"})
	if cols[0].Key != "\u00e9" || cols[1].Key != "\u00e9#2" {
		t.Errorf("NFC duplicates not suffixed: %q, %q", cols[0].Key, cols[1].Key)
	}
}
