package table

import (
	"testing"
)

func TestProfileEncodeDecodeRoundTrip(t *testing.T) {
	cols := DeriveColumns([]string{"金额", "编号"})
	profile := &Profile{
		Engine:        "infinity",
		Columns:       []Column{cols[1], cols[0]},
		OwnedMetadata: []string{"编号", "金额"},
	}
	raw, err := profile.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	decoded, ok, err := DecodeProfile(raw)
	if err != nil || !ok {
		t.Fatalf("DecodeProfile: ok=%v err=%v", ok, err)
	}
	if decoded.Engine != "infinity" {
		t.Errorf("engine = %q", decoded.Engine)
	}
	// Columns and owned keys are stored in key order, so two runs that indexed
	// the same fields write byte-identical records regardless of the order the
	// sheets were read in.
	for i := 1; i < len(decoded.Columns); i++ {
		if decoded.Columns[i-1].Key > decoded.Columns[i].Key {
			t.Errorf("columns not sorted by key: %#v", decoded.Columns)
		}
	}
	for i := 1; i < len(decoded.OwnedMetadata); i++ {
		if decoded.OwnedMetadata[i-1] > decoded.OwnedMetadata[i] {
			t.Errorf("owned metadata not sorted: %v", decoded.OwnedMetadata)
		}
	}
	if len(decoded.Columns) != 2 || len(decoded.OwnedMetadata) != 2 {
		t.Fatalf("round trip lost entries: %#v", decoded)
	}
}

func TestProfileFieldMapUsesReadableNames(t *testing.T) {
	cols := DeriveColumns([]string{"金额", "金额"})
	profile := &Profile{Engine: "infinity", Columns: cols}
	fields := profile.FieldMap()
	if len(fields) != 2 {
		t.Fatalf("field map = %v, want one entry per column", fields)
	}
	if fields[cols[0].DataKey] != "金额" {
		t.Errorf("first column maps to %q", fields[cols[0].DataKey])
	}
	if fields[cols[1].DataKey] != "金额 (2)" {
		t.Errorf("duplicate column maps to %q, want its distinct display name", fields[cols[1].DataKey])
	}
}

func TestDecodeProfileRejectsUnusableRecords(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		wantOK  bool
		wantErr bool
	}{
		{"absent", nil, false, false},
		{"empty string", "", false, false},
		{"not a string", 7, false, false},
		{"broken json", "{", false, true},
		{"no engine", `{"columns":[{"key":"金额","data_key":"c_1"}]}`, false, false},
		{"no columns", `{"engine":"infinity","columns":[]}`, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, ok, err := DecodeProfile(c.value)
			if ok != c.wantOK {
				t.Errorf("ok = %v, want %v", ok, c.wantOK)
			}
			if (err != nil) != c.wantErr {
				t.Errorf("err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}

func TestWithoutProfileFieldKeepsCallerMapIntact(t *testing.T) {
	fields := map[string]any{"作者": "张三", ProfileMetadataField: "{}"}
	stripped := WithoutProfileField(fields)
	if _, ok := stripped[ProfileMetadataField]; ok {
		t.Error("reserved key survived the strip")
	}
	if len(stripped) != 1 {
		t.Errorf("stripped map = %v", stripped)
	}
	// The caller's map keeps the record: a reader that needs it must not have
	// lost it because another reader printed the map.
	if _, ok := fields[ProfileMetadataField]; !ok {
		t.Error("strip mutated the caller's map")
	}

	plain := map[string]any{"作者": "张三"}
	if got := WithoutProfileField(plain); len(got) != 1 {
		t.Errorf("ordinary metadata changed: %v", got)
	}
}
