package esoscommon_test

import (
	"testing"

	"ragflow/internal/engine/internal/esoscommon"
)

func TestChunkID(t *testing.T) {
	cases := []struct {
		name   string
		chunk  map[string]interface{}
		wantID string
		wantOK bool
	}{
		{"id takes priority over _id", map[string]interface{}{"id": "a", "_id": "b"}, "a", true},
		{"_id fallback", map[string]interface{}{"_id": "b"}, "b", true},
		{"empty id falls through to _id", map[string]interface{}{"id": "", "_id": "b"}, "b", true},
		{"non-string id falls through to _id", map[string]interface{}{"id": 42, "_id": "b"}, "b", true},
		{"both empty rejected", map[string]interface{}{"id": "", "_id": ""}, "", false},
		{"no id rejected", map[string]interface{}{"content": "x"}, "", false},
		{"non-string _id rejected", map[string]interface{}{"_id": 42}, "", false},
		{"nil map rejected", nil, "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, ok := esoscommon.ChunkID(c.chunk)
			if id != c.wantID || ok != c.wantOK {
				t.Fatalf("ChunkID(%v) = (%q,%v), want (%q,%v)", c.chunk, id, ok, c.wantID, c.wantOK)
			}
		})
	}
}

func TestCountAggregationTag(t *testing.T) {
	counts := map[string]int{}
	esoscommon.CountAggregationTag(counts, "  tag  ")
	esoscommon.CountAggregationTag(counts, "tag")
	esoscommon.CountAggregationTag(counts, "   ")
	esoscommon.CountAggregationTag(counts, "")
	if counts["tag"] != 2 || len(counts) != 1 {
		t.Fatalf("counts = %v, want {tag:2} with no blank-tag entries", counts)
	}
}

func TestCompileHighlightPattern(t *testing.T) {
	if re := esoscommon.CompileHighlightPattern(nil); re != nil {
		t.Fatalf("nil keywords -> nil, got %v", re)
	}
	if re := esoscommon.CompileHighlightPattern([]string{"", ""}); re != nil {
		t.Fatalf("all-empty keywords -> nil, got %v", re)
	}

	// Regex metacharacters must be quoted: literal dot matches, wildcard does not.
	re := esoscommon.CompileHighlightPattern([]string{"a.b"})
	if re == nil || !re.MatchString("A.B") || re.MatchString("axb") {
		t.Fatalf("metacharacter keyword must be quoted, pattern = %v", re)
	}

	// A Latin keyword gets a trailing \p{Latin}* so stems expand to inflections.
	re = esoscommon.CompileHighlightPattern([]string{"requir"})
	if re == nil || !re.MatchString("Required") {
		t.Fatalf("Latin keyword must expand to inflections, pattern = %v", re)
	}

	// A non-Latin keyword must NOT get the Latin suffix: match must be exactly the keyword.
	re = esoscommon.CompileHighlightPattern([]string{"汉"})
	if re == nil || re.FindString("汉abc") != "汉" {
		t.Fatalf("non-Latin keyword must not gain a Latin suffix, pattern = %v", re)
	}

	// Longest rune-length-first ordering decides which alternation wins.
	re = esoscommon.CompileHighlightPattern([]string{"汉", "汉字"})
	if re == nil || re.FindString("汉字") != "汉字" {
		t.Fatalf("keywords must be ordered by descending rune length, pattern = %v", re)
	}

	re = esoscommon.CompileHighlightPattern([]string{"ab", "abc"})
	if re == nil || !re.MatchString("ABC") {
		t.Fatalf("pattern must match case-insensitively, pattern = %v", re)
	}
}
