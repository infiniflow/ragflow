//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use the file except in compliance with the License.
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

import "testing"

// TestParseIngestorTier verifies the ingestor-tier CLI value is mapped to a
// queue Tier. The community/OSS build has a single queue, so only low/default
// (and an empty/absent value, which defaults to low) are valid; medium/high are
// rejected because they require the multi-lane topology that only the
// enterprise build provides.
func TestParseIngestorTier(t *testing.T) {
	cases := []struct {
		in   string
		want Tier
		err  bool
	}{
		{"", TierDefault, false},
		{"low", TierDefault, false},
		{"LOW", TierDefault, false},
		{"Low", TierDefault, false},
		{"default", TierDefault, false},
		{"DEFAULT", TierDefault, false},
		{"medium", TierDefault, true}, // rejected in community
		{"high", TierDefault, true},   // rejected in community
		{"bogus", TierDefault, true},  // rejected in community
	}
	for _, c := range cases {
		got, err := ParseIngestorTier(c.in)
		if c.err {
			if err == nil {
				t.Errorf("ParseIngestorTier(%q): expected error, got nil", c.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseIngestorTier(%q): unexpected error: %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseIngestorTier(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
