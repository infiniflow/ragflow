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

package common

import (
	"os"
	"reflect"
	"testing"
)

// setOrUnset toggles a key for the duration of a subtest, restoring afterwards.
func setOrUnset(t *testing.T, key, val string, set bool) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if set {
		t.Setenv(key, val)
	} else {
		_ = os.Unsetenv(key)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestGetEnvBool(t *testing.T) {
	cases := []struct {
		name string
		key  string
		val  string
		set  bool
		def  bool
		want bool
	}{
		{"true lower", "GETENVBOOL_T", "true", true, false, true},
		{"false lower with default-on", "GETENVBOOL_F", "false", true, true, false},
		{"1 is true", "GETENVBOOL_1", "1", true, false, true},
		{"0 is false", "GETENVBOOL_0", "0", true, true, false},
		{"t upper", "GETENVBOOL_TU", "T", true, false, true},
		{"TRUE upper", "GETENVBOOL_TF", "TRUE", true, false, true},
		{"True mixed", "GETENVBOOL_TM", "True", true, false, true},
		{"F upper is false", "GETENVBOOL_FU", "F", true, true, false},
		{"FALSE upper", "GETENVBOOL_FF", "FALSE", true, true, false},
		{"False mixed", "GETENVBOOL_FM", "False", true, true, false},
		{"padded true", "GETENVBOOL_PAD", "  true  ", true, false, true},
		// malformed value falls back to def (adversarial: must honour def)
		{"garbage def-false", "GETENVBOOL_G0", "yes", true, false, false},
		{"garbage def-true", "GETENVBOOL_G1", "maybe", true, true, true},
		{"whitespace-only def-false", "GETENVBOOL_WS0", "   ", true, false, false},
		// unset falls back to def in BOTH directions
		{"unset defaults true", "GETENVBOOL_UNSET_T", "", false, true, true},
		{"unset defaults false", "GETENVBOOL_UNSET_F", "", false, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setOrUnset(t, c.key, c.val, c.set)
			got := GetEnvBool(c.key, c.def)
			if got != c.want {
				t.Fatalf("GetEnvBool(%q, %v) = %v, want %v (val=%q set=%v)",
					c.key, c.def, got, c.want, c.val, c.set)
			}
		})
	}
}

func TestSplitCommaList(t *testing.T) {
	cases := []struct {
		name string
		val  string
		set  bool
		want []string
		nil  bool
	}{
		{"three", "a,b,c", true, []string{"a", "b", "c"}, false},
		{"spaces trimmed", "a, b ,c", true, []string{"a", "b", "c"}, false},
		{"consecutive empties dropped", "a,,b", true, []string{"a", "b"}, false},
		{"trailing comma", "a,", true, []string{"a"}, false},
		{"single", "x", true, []string{"x"}, false},
		// whitespace-only entry dropped, not kept as " "
		{"entry is whitespace only", "a, ,b", true, []string{"a", "b"}, false},
		// empty / all-whitespace => nil (adversarial: must be nil, not empty slice)
		{"empty unset", "", false, nil, true},
		{"only separators and spaces", "  , , ", true, nil, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setOrUnset(t, "SPLITCOMMALIST_KEY", c.val, c.set)
			got := SplitCommaList("SPLITCOMMALIST_KEY")
			if c.nil {
				if got != nil {
					t.Fatalf("SplitCommaList(%q) = %v, want nil", c.val, got)
				}
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("SplitCommaList(%q) = %v, want %v", c.val, got, c.want)
			}
		})
	}
}
