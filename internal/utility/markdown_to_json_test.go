package utility

import (
	"strings"
	"testing"
)

func TestDictify_HeadingsNest(t *testing.T) {
	md := "# Top\n\n## Mid A\n\n## Mid B\n\n### Deep\n"
	got := Dictify(md)
	if len(got) != 1 || got[0].k != "Top" {
		t.Fatalf("top-level = %+v, want single key Top", got)
	}
	mid, ok := got[0].v.(OMap)
	if !ok || len(mid) != 2 {
		t.Fatalf("Top value = %+v, want {Mid A, Mid B}", got[0].v)
	}
	if mid[0].k != "Mid A" || mid[0].v != "" {
		t.Errorf("Mid A = %+v, want empty string value (no children)", mid[0])
	}
	deep, ok := mid[1].v.(OMap)
	if !ok || len(deep) != 1 || deep[0].k != "Deep" {
		t.Errorf("Mid B value = %+v, want {Deep}", mid[1].v)
	}
}

func TestDictify_BulletPairsViaTodict(t *testing.T) {
	// "- term\n  - def" pairs become {term: def}; the unpaired bullet
	// ("term2") is dropped — faithful to _list_to_kv.
	md := "# T\n\n## S\n- term1\n  - def1\n- term2\n"
	got := Todict(Dictify(md))
	if len(got) != 1 || got[0].k != "T" {
		t.Fatalf("dictify = %+v", got)
	}
	s, ok := got[0].v.(OMap)
	if !ok || len(s) != 1 || s[0].k != "S" {
		t.Fatalf("T value = %+v", got[0].v)
	}
	pairs, ok := s[0].v.(OMap)
	if !ok || len(pairs) != 1 || pairs[0].k != "term1" || pairs[0].v != "def1" {
		t.Fatalf("S value = %+v, want {term1: def1} (term2 dropped)", s[0].v)
	}
}

func TestDictify_PlainListVanishes(t *testing.T) {
	// Bullets without definition sub-lists collapse to an empty dict
	// (Python _list_to_kv parity).
	md := "# T\n\n- a\n- b\n"
	got := Todict(Dictify(md))
	if len(got) != 1 {
		t.Fatalf("dictify = %+v", got)
	}
	if v, ok := got[0].v.(OMap); !ok || len(v) != 0 {
		t.Fatalf("plain bullets must collapse to empty dict, got %+v", got[0].v)
	}
}

func TestDictify_PlainProse(t *testing.T) {
	got := Todict(Dictify("hello world, no headings here"))
	if len(got) != 1 || got[0].k != "root" {
		t.Fatalf("prose dictify = %+v, want single root key", got)
	}
	// The root list has no definition pairs → collapses to empty dict.
	if v, ok := got[0].v.(OMap); !ok || len(v) != 0 {
		t.Fatalf("root value = %+v, want empty dict", got[0].v)
	}
}

func TestDictify_JumpedHeadingBecomesText(t *testing.T) {
	// H1 → H3 (no H2): the jumped heading is not treated as a key.
	md := "# H1\n\n### H3\n\ntext\n"
	got := Dictify(md)
	if len(got) != 1 || got[0].k != "H1" {
		t.Fatalf("dictify = %+v", got)
	}
	if got[0].v != "H3\n\ntext" {
		t.Fatalf("H1 value = %q, want jumped heading rendered as text", got[0].v)
	}
}

func TestDictify_LeadingContentDropped(t *testing.T) {
	// Content before the first heading at the minimum level is discarded
	// (dictify_list_by parity).
	md := "preamble\n\n# H\n\nbody\n"
	got := Dictify(md)
	if len(got) != 1 || got[0].k != "H" || got[0].v != "body" {
		t.Fatalf("leading content must be dropped: %+v", got)
	}
}

func TestDictify_ClosedHeadingStripsTrailingHashes(t *testing.T) {
	// Deviation #1 fix: "## Title ##" must yield "Title" like Python's
	// CommonMark (leading AND trailing '#' runs removed).
	md := "# Root ##\n\n## Title ##\n\nbody\n"
	got := Dictify(md)
	if len(got) != 1 || got[0].k != "Root" {
		t.Fatalf("closed root heading = %+v, want key \"Root\"", got)
	}
	children, ok := got[0].v.(OMap)
	if !ok || len(children) != 1 || children[0].k != "Title" {
		t.Fatalf("closed child heading = %+v, want key \"Title\"", got[0].v)
	}
}

func TestMergeDicts_Order(t *testing.T) {
	d1 := OMap{{"A", []any{"x"}}}
	d2 := OMap{{"A", []any{"y"}}, {"B", "b"}}
	got := MergeDicts(d1, d2)
	if len(got) != 2 || got[0].k != "A" || got[1].k != "B" {
		t.Fatalf("merge = %+v", got)
	}
	list, _ := got[0].v.([]any)
	if len(list) != 2 || list[0] != "y" || list[1] != "x" {
		t.Fatalf("list merge = %v, want [y x] (d2 items before d1)", list)
	}
}

func TestShapeTree_MultiTopKeys(t *testing.T) {
	merged := OMap{
		{"A", OMap{{"shared", "x"}}},
		{"B", OMap{{"shared", "y"}}},
	}
	root := ShapeTree(merged)
	if root.ID != "root" || len(root.Children) != 2 {
		t.Fatalf("shape = %+v, want root with A,B", root)
	}
	// The second "shared" key must survive (keyset dedups within a subtree
	// walk, and A's subtree consumes "shared" first — Python parity: B's
	// "shared" is dropped because the keyset is shared across children).
	seen := map[string]int{}
	var walk func(n *Node)
	walk = func(n *Node) {
		seen[n.ID]++
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	if seen["shared"] != 1 {
		t.Fatalf("shared key count = %d, want 1 (global keyset dedup)", seen["shared"])
	}
}

func TestShapeTree_SingleKey(t *testing.T) {
	merged := OMap{{"Only", OMap{{"child", "text"}}}}
	root := ShapeTree(merged)
	if root.ID != "Only" {
		t.Fatalf("single-key root id = %q, want Only", root.ID)
	}
	if len(root.Children) != 1 || root.Children[0].ID != "child" {
		t.Fatalf("root children = %+v", root.Children)
	}
}

func TestBeChildren_StringLeaf(t *testing.T) {
	keyset := map[string]bool{}
	out := BeChildren("plain string", keyset)
	if len(out) != 1 || out[0].ID != "plain string" {
		t.Fatalf("string leaf = %+v", out)
	}
	if !keyset["plain string"] {
		t.Fatalf("raw string must enter the keyset")
	}
}

func TestStripStars(t *testing.T) {
	if got := StripStars("**Bold** title"); got != "Bold title" {
		t.Fatalf("StripStars = %q", got)
	}
}

func TestStripFences(t *testing.T) {
	in := "```json\n{\"a\": 1}\n```\n"
	if got := StripFences(in); strings.Contains(got, "```") {
		t.Fatalf("fences not stripped: %q", got)
	}
}
