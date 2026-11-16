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

//go:build !ee

package config

import (
	"testing"

	"ragflow/internal/common"
)

func tierArg(s string) *string { return &s }

// TestGetIngestorTier pins the resolution contract of the new tier knob:
// CLI > environment > default, with empty/whitespace meaning "unset" at both
// layers and any unsupported name being a fatal startup error. It mirrors the
// coverage its sibling resolvers already have, and adds adversarial fixtures
// that distinguish which layer rejected a value.
func TestGetIngestorTier(t *testing.T) {
	const envKey = common.EnvIngestorTier

	cases := []struct {
		name     string
		env      string // "" means leave unset
		argValue *string
		want     common.Tier
		wantErr  bool
	}{
		{"default", "", nil, common.TierDefault, false},
		{"env low", "low", nil, common.TierDefault, false},
		{"env case insensitive", "LOW", nil, common.TierDefault, false},
		{"env trims whitespace", "  low  ", nil, common.TierDefault, false},
		{"env blank is unset", "   ", nil, common.TierDefault, false},
		{"env high rejected", "high", nil, "", true},
		{"env default spelling rejected", "default", nil, "", true},
		{"arg low", "", tierArg("low"), common.TierDefault, false},
		{"arg case insensitive", "", tierArg("LoW"), common.TierDefault, false},
		{"arg trims whitespace", "", tierArg(" low "), common.TierDefault, false},
		{"arg blank is unset", "", tierArg("  "), common.TierDefault, false},
		{"arg high rejected", "", tierArg("high"), "", true},
		{"arg default spelling rejected", "", tierArg("DEFAULT"), "", true},
		// Empty CLI value must fall back to the environment, not mask it.
		{"empty arg keeps env", "low", tierArg(""), common.TierDefault, false},
		// Adversarial: an invalid env is rejected even when a valid CLI value
		// is present — the same fail-fast rule the sibling resolvers apply.
		{"invalid env wins over valid arg", "bogus", tierArg("low"), "", true},
		// Adversarial: an invalid CLI value is rejected even when the env is
		// valid, proving both layers validate rather than only the first hit.
		{"invalid arg wins over valid env", "low", tierArg("bogus"), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envKey, tc.env)
			c := &Config{}
			got, err := c.GetIngestorTier(tc.argValue)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("GetIngestorTier(env=%q arg=%v) error = nil, want rejection", tc.env, tc.argValue)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetIngestorTier(env=%q arg=%v) unexpected error: %v", tc.env, tc.argValue, err)
			}
			if got != tc.want {
				t.Fatalf("GetIngestorTier(env=%q arg=%v) = %q, want %q", tc.env, tc.argValue, got, tc.want)
			}
		})
	}
}
