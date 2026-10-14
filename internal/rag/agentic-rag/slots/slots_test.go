package slots

import "testing"

// TestUnionKeepsTheInvariants pins the four rules the rest of the pipeline relies on.
//
// They exist because the alternative was measured to fail: when a candidate is free
// text, "is this a count or a list" has to be guessed back, and the guesses interact —
// measured (2026-09-16, 三国/关羽) a count slot holding "约 17-19 人" ended up as
// "约、人" (the digits rejected as members, the qualifiers accepted as members) while
// fifteen real names sat in the list slot and the answer reported ten.
func TestUnionKeepsTheInvariants(t *testing.T) {
	// I2 — members union by name, first evidence wins.
	a := Items(Item{Value: "华雄", ChunkID: "c1"}, Item{Value: "颜良"})
	b := Items(Item{Value: "颜良", ChunkID: "c2"}, Item{Value: "管亥", ChunkID: "c3"})
	got, dropped, ok := Union(a, b)
	if !ok || len(got.ItemValues()) != 3 {
		t.Fatalf("members ∪ members = %v (ok=%v), want three names", got, ok)
	}
	if !dropped.IsZero() {
		t.Errorf("dropped = %v, want nothing: a member list does not lose members to another list", dropped)
	}
	for _, m := range got.Items {
		switch m.Value {
		case "华雄":
			if m.ChunkID != "c1" {
				t.Errorf("华雄 = %v, want its own evidence kept", m)
			}
		case "颜良":
			// The base listed 颜良 without evidence; the branch had one. A member
			// without a passage is a member nobody can point at, so the evidence is
			// filled in rather than dropped with the duplicate.
			if m.ChunkID != "c2" {
				t.Errorf("颜良 = %v, want the evidence the other list supplied", m)
			}
		}
	}
	// A member that arrived without evidence takes the one a later list supplies.
	withEvidence, _, _ := Union(Items(Item{Value: "庞德"}), Items(Item{Value: "庞德", ChunkID: "c9"}))
	if withEvidence.Items[0].ChunkID != "c9" {
		t.Errorf("庞德 = %v, want the evidence a later list supplied", withEvidence.Items[0])
	}

	// I3 — members beat a number, and the number is kept as the dropped claim.
	union, dropped, ok := Union(a, Number(2))
	if !ok || union.Kind != KindItems {
		t.Fatalf("members ∪ count = %v (ok=%v), want the members", union, ok)
	}
	if dropped.Kind != KindCount || dropped.Count != 2 {
		t.Errorf("dropped = %v, want the losing count 2", dropped)
	}
	// …in either order.
	union, dropped, ok = Union(Number(2), a)
	if !ok || union.Kind != KindItems || dropped.Count != 2 {
		t.Errorf("count ∪ members = %v / %v (ok=%v), want the members with the count dropped", union, dropped, ok)
	}

	// I4 — two numbers keep the larger, in both orders.
	for _, pair := range [][2]Value{{Number(10), Number(13)}, {Number(13), Number(10)}} {
		union, dropped, ok := Union(pair[0], pair[1])
		if !ok || union.Count != 13 {
			t.Errorf("counts %v ∪ %v = %v (ok=%v), want 13", pair[0], pair[1], union, ok)
		}
		if dropped.Count != 10 {
			t.Errorf("dropped = %v, want 10", dropped)
		}
	}
	// A range counts as the number it claims at its upper bound.
	if union, dropped, ok := Union(Interval(17, 19), Number(16)); !ok || union.Kind != KindRange || dropped.Count != 16 {
		t.Errorf("range ∪ count = %v / %v (ok=%v), want the range with 16 dropped", union, dropped, ok)
	}

	// I1 — Text never merges by union: the caller's strength rule decides, which is
	// what keeps a value question (a date, a name, a phrase) behaving as it always did.
	if _, _, ok := Union(Text("1858"), Text("1849")); ok {
		t.Error("text ∪ text must not resolve: a value question merges by strength")
	}
	if _, _, ok := Union(Text("13"), Number(11)); ok {
		t.Error("text ∪ count must not resolve: text is opaque and claims nothing")
	}
	if _, _, ok := Union(Items(Item{Value: "华雄"}), Text("华雄")); ok {
		t.Error("a member list and a text value must not resolve by union")
	}
}

// TestParseIsFailClosed pins the boundary contract: a patch that declares a kind is
// read as data, and anything else is opaque text. Nothing is inferred from the string,
// which is what removes the rules that used to misfire.
func TestParseIsFailClosed(t *testing.T) {
	members := Parse(map[string]any{
		"kind": "members",
		"items": []any{
			map[string]any{"name": "华雄", "chunk_id": "c1", "quote": "提华雄之头"},
			map[string]any{"name": "颜良"},
			map[string]any{"quote": "no name here"}, // skipped, not invented
		},
	})
	if members.Kind != KindItems || len(members.ItemValues()) != 2 {
		t.Fatalf("members patch = %v, want two declared members", members)
	}
	if members.Items[0].ChunkID != "c1" {
		t.Errorf("华雄 = %v, want its evidence", members.Items[0])
	}

	// A count phrase the model did not type is TEXT: it is neither split nor parsed.
	// This is the measured failure the typing exists for.
	opaque := Parse(map[string]any{"candidate": "约 17-19 人"})
	if opaque.Kind != KindText || opaque.Text != "约 17-19 人" {
		t.Fatalf("untyped candidate = %v, want it opaque", opaque)
	}
	if _, ok := opaque.Number(); ok {
		t.Error("text must claim no number: that is what stops 17-19 from being read as 1719")
	}
	if names := opaque.ItemValues(); names != nil {
		t.Errorf("text names = %v, want none", names)
	}

	// Declared kinds read their own fields, and a malformed one falls back to text
	// rather than being half-read.
	if v := Parse(map[string]any{"kind": "count", "count": float64(13)}); v.Kind != KindCount || v.Count != 13 {
		t.Errorf("count patch = %v, want 13", v)
	}
	if v := Parse(map[string]any{"kind": "range", "lo": float64(17), "hi": float64(19)}); v.Kind != KindRange || v.Lo != 17 || v.Hi != 19 {
		t.Errorf("range patch = %v, want 17-19", v)
	}
	if v := Parse(map[string]any{"kind": "count", "candidate": "about ten"}); v.Kind != KindText {
		t.Errorf("a count without a number = %v, want text (fail closed)", v)
	}
	if v := Parse(map[string]any{"kind": "members"}); v.Kind != KindText {
		t.Errorf("members without items = %v, want text (fail closed)", v)
	}
	if v := Parse(nil); !v.IsZero() {
		t.Errorf("nil patch = %v, want empty", v)
	}
}

// TestRenderIsDerivedFromTheType pins the other half of the contract: what a prompt
// or a record line shows is derived from the typed value, so nothing downstream has
// to re-derive the type from the string.
func TestRenderIsDerivedFromTheType(t *testing.T) {
	cases := []struct {
		in   Value
		want string
	}{
		{Items(Item{Value: "华雄"}, Item{Value: "颜良"}), "华雄、颜良"},
		{Number(13), "13"},
		{Interval(17, 19), "17-19"},
		{Interval(5, 5), "5"},
		{Text("1858"), "1858"},
	}
	for _, c := range cases {
		if got := Render(c.in); got != c.want {
			t.Errorf("Render(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
