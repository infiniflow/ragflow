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

package dataset

import "testing"

func TestModularMetadataConfigLegacyFlatShape(t *testing.T) {
	builtIn := []any{map[string]any{"key": "update_time", "type": "time"}}
	userFields := []any{map[string]any{"key": "author", "type": "string"}}

	tests := []struct {
		name        string
		config      map[string]any
		wantPresent bool
		wantEnabled bool
		wantMeta    int
		wantBuiltIn int
	}{
		{
			name:        "legacy flat keys are read",
			config:      map[string]any{"enable_metadata": true, "metadata": userFields, "built_in_metadata": builtIn},
			wantPresent: true, wantEnabled: true, wantMeta: 1, wantBuiltIn: 1,
		},
		{
			name:        "legacy disabled flag is preserved",
			config:      map[string]any{"enable_metadata": false, "built_in_metadata": builtIn},
			wantPresent: true, wantEnabled: false, wantMeta: 0, wantBuiltIn: 1,
		},
		{
			name:        "legacy fields without flag imply enabled",
			config:      map[string]any{"built_in_metadata": builtIn},
			wantPresent: true, wantEnabled: true, wantMeta: 0, wantBuiltIn: 1,
		},
		{
			name: "modular shape wins over stray legacy keys",
			config: map[string]any{
				"metadata":          map[string]any{"enabled": false, "metadata": []any{}, "built_in_metadata": []any{}},
				"built_in_metadata": builtIn,
			},
			wantPresent: true, wantEnabled: false, wantMeta: 0, wantBuiltIn: 0,
		},
		{
			name:        "no metadata config at all",
			config:      map[string]any{"chunk_token_num": 512},
			wantPresent: false, wantEnabled: false, wantMeta: 0, wantBuiltIn: 0,
		},
		{
			name:        "malformed metadata value is ignored",
			config:      map[string]any{"metadata": "oops"},
			wantPresent: false, wantEnabled: false, wantMeta: 0, wantBuiltIn: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			present, enabled, meta, bi := modularMetadataConfig(tt.config)
			if present != tt.wantPresent || enabled != tt.wantEnabled || len(meta) != tt.wantMeta || len(bi) != tt.wantBuiltIn {
				t.Fatalf("got present=%v enabled=%v meta=%d builtIn=%d, want present=%v enabled=%v meta=%d builtIn=%d",
					present, enabled, len(meta), len(bi), tt.wantPresent, tt.wantEnabled, tt.wantMeta, tt.wantBuiltIn)
			}
		})
	}
}
