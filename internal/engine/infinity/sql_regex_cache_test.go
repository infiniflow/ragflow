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

package infinity

import "testing"

const d5SQL = "select d5_alias_a, d5_alias_b from ragflow_t1 where d5_alias_a = 1 order by d5_alias_b group by d5_alias_c having d5_alias_c > 0"

var d5Aliases = map[string]string{
	"d5_alias_a": "actual_a",
	"d5_alias_b": "actual_b",
	"d5_alias_c": "actual_c",
}

// TestCachedSQLRegexesAreReused is a deterministic guard against reintroducing
// per-call compilation: each cache lookup must return the same compiled object,
// while distinct aliases/keywords must not share an entry.
func TestCachedSQLRegexesAreReused(t *testing.T) {
	for _, tc := range []struct {
		name                string
		first, again, other interface{}
	}{
		{"boundary alias", cachedAliasBoundaryRegex("d5_alias_a"), cachedAliasBoundaryRegex("d5_alias_a"), cachedAliasBoundaryRegex("d5_alias_b")},
		{"word alias", cachedAliasWordRegex("d5_alias_a"), cachedAliasWordRegex("d5_alias_a"), cachedAliasWordRegex("d5_alias_b")},
		{"keyword", cachedKeywordRegex("where"), cachedKeywordRegex("where"), cachedKeywordRegex("having")},
	} {
		if tc.first != tc.again {
			t.Errorf("%s regex was rebuilt across calls", tc.name)
		}
		if tc.first == tc.other {
			t.Errorf("%s regex cache did not separate distinct keys", tc.name)
		}
	}
}

// TestRewriteFieldAliasesStableAcrossCalls pins that repeated rewrites with
// cached regexes stay byte-for-byte identical.
func TestRewriteFieldAliasesStableAcrossCalls(t *testing.T) {
	want := rewriteFieldAliases(d5SQL, d5Aliases)
	for range 5 {
		if got := rewriteFieldAliases(d5SQL, d5Aliases); got != want {
			t.Fatalf("rewrite changed across calls: %q vs %q", got, want)
		}
	}
}

// TestRewriteFieldAliasesCacheKeepsOutput pins that introducing the regex
// cache does not alter a single byte of the rewritten SQL.
func TestRewriteFieldAliasesCacheKeepsOutput(t *testing.T) {
	got := rewriteFieldAliases(d5SQL, d5Aliases)
	want := "select actual_a, actual_b from ragflow_t1 where actual_a = 1 order by actual_b group by actual_c having actual_c > 0"
	if got != want {
		t.Fatalf("rewriteFieldAliases = %q, want %q", got, want)
	}
}

// BenchmarkRewriteFieldAliasesCached guards the SQL rewrite path against
// reintroducing per-call regex compilation.
func BenchmarkRewriteFieldAliasesCached(b *testing.B) {
	rewriteFieldAliases(d5SQL, d5Aliases)
	b.ResetTimer()
	for range b.N {
		rewriteFieldAliases(d5SQL, d5Aliases)
	}
}

// TestRewriteFieldAliasesCacheHandlesMetaCharacters is the adversarial case
// for caching by literal alias: a dot/bracket in the alias must stay quoted.
func TestRewriteFieldAliasesCacheHandlesMetaCharacters(t *testing.T) {
	aliases := map[string]string{"a.b1": "actual"}
	in := "select a.b1 from ragflow_t1 where a.b1 = 1"
	want := "select actual from ragflow_t1 where actual = 1"
	for range 3 {
		if got := rewriteFieldAliases(in, aliases); got != want {
			t.Fatalf("rewriteFieldAliases = %q, want %q", got, want)
		}
	}
}

// TestRewriteFieldAliasesCacheKeepsWordBoundary pins that an alias embedded
// in a longer identifier is still left untouched after the regex is cached.
func TestRewriteFieldAliasesCacheKeepsWordBoundary(t *testing.T) {
	aliases := map[string]string{"d5_alias_a": "actual_a"}
	in := "select d5_alias_a_extra from ragflow_t1 where d5_alias_a = 1"
	want := "select d5_alias_a_extra from ragflow_t1 where actual_a = 1"
	for range 3 {
		if got := rewriteFieldAliases(in, aliases); got != want {
			t.Fatalf("rewriteFieldAliases = %q, want %q", got, want)
		}
	}
}
