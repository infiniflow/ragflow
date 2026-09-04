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

package pipeline

import (
	"strings"
	"testing"
)

func TestExtractPayload(t *testing.T) {
	tests := []struct {
		name    string
		dsl     string
		output  map[string]any
		wantOK  bool
		wantErr string
		want    map[string]any
	}{
		{
			name:   "output_format key present → return output as-is",
			dsl:    `{}`,
			output: map[string]any{"output_format": "x", "data": 1},
			wantOK: true,
			want:   map[string]any{"output_format": "x", "data": 1},
		},
		{
			name:   "nil output → nil, nil",
			dsl:    `{}`,
			wantOK: true,
		},
		{
			name:   "single terminal → extract its payload",
			dsl:    `{"components":{"c1":{"downstream":null},"begin":{"downstream":["c1"]}}}`,
			output: map[string]any{"c1": map[string]any{"text": "hi"}, "begin": map[string]any{}},
			wantOK: true,
			want:   map[string]any{"text": "hi"},
		},
		{
			name:    "zero terminals → error",
			dsl:     `{"components":{}}`,
			output:  map[string]any{},
			wantErr: "requires exactly 1 terminal, got 0",
		},
		{
			name:    "multiple terminals → error",
			dsl:     `{"components":{"c1":{"downstream":null},"c2":{"downstream":null}}}`,
			output:  map[string]any{"c1": map[string]any{}, "c2": map[string]any{}},
			wantErr: "requires exactly 1 terminal, got 2",
		},
		{
			name:   "nested dsl template → unwrap and extract",
			dsl:    `{"id":"t1","dsl":{"components":{"c1":{"downstream":null}},"path":["c1"],"graph":{"nodes":[]}}}`,
			output: map[string]any{"c1": map[string]any{"text": "nested"}},
			wantOK: true,
			want:   map[string]any{"text": "nested"},
		},
		{
			name:    "DSL missing components key",
			dsl:     `{}`,
			output:  map[string]any{},
			wantErr: "missing components map",
		},
		{
			name:    "terminal payload is not a map",
			dsl:     `{"components":{"c1":{"downstream":null}}}`,
			output:  map[string]any{"c1": "not-a-map"},
			wantErr: "missing terminal payload",
		},
		{
			name:   "output_format takes priority over terminal extraction",
			dsl:    `{"components":{"c1":{"downstream":null}}}`,
			output: map[string]any{"output_format": "markdown", "text": "content"},
			wantOK: true,
			want:   map[string]any{"output_format": "markdown", "text": "content"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractPayload(tt.dsl, tt.output)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantOK {
				return
			}
			if tt.want == nil && got != nil {
				t.Fatalf("got = %v, want nil", got)
			}
			if tt.want != nil {
				for k, v := range tt.want {
					if got[k] != v {
						t.Fatalf("got[%q] = %v, want %v", k, got[k], v)
					}
				}
			}
		})
	}
}

func TestTerminalComponentIDs(t *testing.T) {
	tests := []struct {
		name    string
		raw     []byte
		want    []string
		wantErr string
	}{
		{
			name: "single terminal",
			raw:  []byte(`{"components":{"c1":{"downstream":null}}}`),
			want: []string{"c1"},
		},
		{
			name: "multiple terminals sorted",
			raw:  []byte(`{"components":{"c2":{"downstream":null},"c1":{"downstream":null}}}`),
			want: []string{"c1", "c2"},
		},
		{
			name: "non-terminal with downstream connections excluded",
			raw:  []byte(`{"components":{"c1":{"downstream":["c2"]},"c2":{"downstream":null}}}`),
			want: []string{"c2"},
		},
		{
			name: "empty downstream array counts as terminal",
			raw:  []byte(`{"components":{"c1":{"downstream":[]}}}`),
			want: []string{"c1"},
		},
		{
			name: "nested dsl template unwrapped",
			raw:  []byte(`{"dsl":{"components":{"c1":{"downstream":null}}}}`),
			want: []string{"c1"},
		},
		{
			name:    "no components key",
			raw:     []byte(`{}`),
			wantErr: "missing components map",
		},
		{
			name: "no terminals",
			raw:  []byte(`{"components":{}}`),
			want: []string{},
		},
		{
			name:    "invalid JSON",
			raw:     []byte(`{invalid`),
			wantErr: "unmarshal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := TerminalComponentIDs(tt.raw)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v (%d), want %v (%d)", got, len(got), tt.want, len(tt.want))
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
