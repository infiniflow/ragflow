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

package deepdoc

import "testing"

// TestResolveDeviceSpec pins the precedence and normalization of the DeepDoc
// device spec: a non-blank CLI value wins over the env value, a blank CLI value
// falls back to env, and an all-blank resolution defaults to "cpu". Whitespace
// is trimmed at every layer.
func TestResolveDeviceSpec(t *testing.T) {
	cases := []struct {
		name string
		cli  string
		env  string
		want string
	}{
		{"both blank -> cpu", "", "", "cpu"},
		{"env only", "", "gpu:1", "gpu:1"},
		{"cli only", "gpu:2", "", "gpu:2"},
		{"cli overrides env", "gpu:2", "cpu", "gpu:2"},
		{"blank cli falls back to env", "   ", "gpu:3", "gpu:3"},
		{"blank env with cli", "cpu", "   ", "cpu"},
		{"whitespace trimmed on cli", "  gpu:4  ", "cpu", "gpu:4"},
		{"whitespace trimmed on env", "", "  gpu:5  ", "gpu:5"},
		{"blank cli and blank env -> cpu", "  ", "  ", "cpu"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resolveDeviceSpec(c.cli, c.env)
			if got != c.want {
				t.Errorf("resolveDeviceSpec(cli=%q, env=%q) = %q, want %q", c.cli, c.env, got, c.want)
			}
		})
	}
}
