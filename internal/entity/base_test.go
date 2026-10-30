package entity

import (
	"testing"
)

func TestJSONMapScan(t *testing.T) {
	for _, tt := range []struct {
		name  string
		input any
		want  map[string]any
	}{
		{"nil", nil, nil},
		{"empty string", "", nil},
		{"object bytes", []byte(`{"k":"v"}`), map[string]any{"k": "v"}},
		{"object string", `{"k":"v"}`, map[string]any{"k": "v"}},
		{"quoted object string", `"{\"k\":\"v\"}"`, map[string]any{"k": "v"}},
		{"quoted object bytes", []byte(`"{\"k\":\"v\"}"`), map[string]any{"k": "v"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var j JSONMap
			if err := j.Scan(tt.input); err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if len(j) != len(tt.want) {
				t.Fatalf("got %v, want %v", j, tt.want)
			}
			for k, v := range tt.want {
				if j[k] != v {
					t.Fatalf("key %q got %v want %v", k, j[k], v)
				}
			}
		})
	}
}
