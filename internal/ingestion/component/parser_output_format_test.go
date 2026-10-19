//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package component

import (
	"testing"
)

// TestDefaultSetupsNormalizedToJSON pins that the raw defaults already carry
// the single JSON output protocol. defaultSetups() must not ship a per-family
// output_format literal (normalizeParserOutputFormats owns that key), yet
// every family it returns must end up on JSON — otherwise a caller building a
// ParserComponent straight from defaultSetups() would silently get an
// un-normalized setup (the fragility flagged in the PR #21084 review).
func TestDefaultSetupsNormalizedToJSON(t *testing.T) {
	for family, setup := range defaultSetups() {
		if got := setup["output_format"]; got != "json" {
			t.Errorf("defaultSetups()[%q].output_format = %v, want json", family, got)
		}
	}
}

// TestParserOutputFormatAlwaysJSON pins the observable contract that survives
// the literal cleanup: however the component is constructed — defaults, an
// empty param map, or a legacy per-family override — every family ends up on
// the single JSON output protocol.
func TestParserOutputFormatAlwaysJSON(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]any
	}{
		{"defaults", nil},
		{"empty params", map[string]any{}},
		{"legacy markdown override", map[string]any{"pdf": map[string]any{"output_format": "markdown"}}},
		{"legacy text override", map[string]any{"text&code": map[string]any{"output_format": "text"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			comp, err := NewParserComponent(c.params)
			if err != nil {
				t.Fatalf("NewParserComponent: %v", err)
			}
			pc, ok := comp.(*ParserComponent)
			if !ok {
				t.Fatalf("NewParserComponent returned %T, want *ParserComponent", comp)
			}
			if len(pc.setups) == 0 {
				t.Fatal("expected non-empty setups")
			}
			for family, setup := range pc.setups {
				if got := setup["output_format"]; got != "json" {
					t.Errorf("family %q output_format = %v, want json", family, got)
				}
			}
		})
	}
}
