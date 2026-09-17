// Package slots is the VALUE ALGEBRA of a slot table: what a candidate value IS,
// how two values MERGE, and how a table's count is DERIVED.
//
// It exists because the alternative fails. When a candidate is free text, the structure is
// lost at the boundary — a count arrives as a sentence — and every consumer downstream has
// to GUESS it back: is this a count? a list? are these pieces names? Each guess is a rule,
// the rules interact, and the result is a count slot holding the words around the number
// (digits rejected as "not members", qualifiers accepted as "members") while the real names
// sit in another slot.
//
// So the structure travels WITH the value: a members list carries its items (each
// with the chunk that proves it), a count carries a number, a range carries two, and
// anything the model did not type is Text — stored, shown, never interpreted, never
// split. Nothing in this package inspects text, so there is no rule here to misfire.
//
// Four invariants hold, and they are what the rest of the pipeline may rely on:
//
//	I1  Text never merges by union: the caller's strength rule decides. This is what
//	    keeps a value question (a date, a name, a phrase) behaving exactly as before.
//	I2  members ∪ members = members, deduped by name, each keeping its first evidence.
//	I3  members BEAT a number: a count is a claim ABOUT the list, so the number comes
//	    back as the dropped value (kept as an alternate) rather than overwriting it.
//	I4  two numbers keep the larger: a set that shrinks when a second source agrees
//	    with it is a set that loses members.
package slots

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is what a value IS. There is no "unknown": anything the model did not type is
// Text, which is opaque by construction.
type Kind uint8

const (
	// KindEmpty is a slot with no value at all.
	KindEmpty Kind = iota
	// KindText is an opaque value: a date, a name, a phrase, a sentence. It is shown
	// and merged by strength, and NEVER parsed, split, counted, or compared
	// arithmetically.
	KindText
	// KindItems is a list of ITEMS the corpus states, each optionally carrying the chunk that
	// proves it. The element kind is the answer's (names for one question, dates for the next),
	// so the type says item and nothing here claims to know which kind this list holds.
	KindItems
	// KindCount is a claimed number ("13").
	KindCount
	// KindRange is a claimed interval ("about 17-19"), kept as two numbers rather than as a
	// string that someone later tries to parse: parsed digit-by-digit it is 1719.
	KindRange
)

func (k Kind) String() string {
	switch k {
	case KindText:
		return "text"
	case KindItems:
		return "members"
	case KindCount:
		return "count"
	case KindRange:
		return "range"
	}
	return "empty"
}

// Item is one element of an enumerated answer and the passage that states it: a name for one
// question, a date or a number for the next. The element kind belongs to the answer, not to the
// type, and an empty ChunkID is allowed and visible — an item without one is not counted.
type Item struct {
	Value   string
	ChunkID string
	Quote   string
}

// Value is what one slot holds. Exactly one of the fields matching Kind is set.
type Value struct {
	Kind  Kind
	Text  string // KindText (the raw candidate, verbatim)
	Items []Item // KindItems
	Count int    // KindCount
	Lo    int    // KindRange (inclusive)
	Hi    int    // KindRange (inclusive)
}

// Text wraps an opaque string.
func Text(s string) Value { return Value{Kind: KindText, Text: s} }

// Members wraps a member list (evidence may be empty per item).
func Items(items ...Item) Value { return Value{Kind: KindItems, Items: items} }

// Number wraps a claimed count.
func Number(n int) Value { return Value{Kind: KindCount, Count: n} }

// Interval wraps a claimed range; lo > hi is normalised.
func Interval(lo, hi int) Value {
	if hi < lo {
		lo, hi = hi, lo
	}
	return Value{Kind: KindRange, Lo: lo, Hi: hi}
}

// IsZero reports "no value at all".
func (v Value) IsZero() bool {
	switch v.Kind {
	case KindText:
		return strings.TrimSpace(v.Text) == ""
	case KindItems:
		return len(v.Items) == 0
	}
	return v.Kind == KindEmpty
}

// ItemValues is the items' values in order, deduped case-insensitively.
func (v Value) ItemValues() []string {
	if v.Kind != KindItems {
		return nil
	}
	seen := make(map[string]bool, len(v.Items))
	out := make([]string, 0, len(v.Items))
	for _, m := range v.Items {
		name := strings.TrimSpace(m.Value)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	return out
}

// Anchored is the items that carry the chunk stating them: what a count may be derived from and
// what an answer may cite. The test reads ChunkID, not Quote, because a quotation with no passage
// to look it up in cannot be checked.
func (v Value) Anchored() []Item {
	return v.itemsWhere(func(it Item) bool { return strings.TrimSpace(it.ChunkID) != "" })
}

// Unanchored is the complement of Anchored: the items asserted with no passage in hand. They are
// not deleted — the record lists them beside the number — but they are not counted.
func (v Value) Unanchored() []Item {
	return v.itemsWhere(func(it Item) bool { return strings.TrimSpace(it.ChunkID) == "" })
}

// itemsWhere filters the items by a property of the item. A value that is not a list has no
// items, so the filters answer nothing rather than inventing a list out of text.
func (v Value) itemsWhere(keep func(Item) bool) []Item {
	if v.Kind != KindItems {
		return nil
	}
	out := make([]Item, 0, len(v.Items))
	for _, it := range v.Items {
		if keep(it) {
			out = append(out, it)
		}
	}
	return out
}

// Number is the number a value claims: a count's number, a range's UPPER bound (the
// claim it makes about how many there could be). Text and members claim no number —
// that is the point of typing them.
func (v Value) Number() (int, bool) {
	switch v.Kind {
	case KindCount:
		return v.Count, true
	case KindRange:
		return v.Hi, true
	}
	return 0, false
}

// Render is the value's display text: what a prompt, a record line or a patch log
// shows. It is derived FROM the typed value, so nothing downstream ever has to
// re-derive the type from it.
func Render(v Value) string {
	switch v.Kind {
	case KindText:
		return v.Text
	case KindItems:
		names := v.ItemValues()
		return strings.Join(names, "、")
	case KindCount:
		return strconv.Itoa(v.Count)
	case KindRange:
		if v.Lo == v.Hi {
			return strconv.Itoa(v.Lo)
		}
		return fmt.Sprintf("%d-%d", v.Lo, v.Hi)
	}
	return ""
}

// Union merges two values and reports what changed.
//
// ok is false when the pair is not the union's business — either side being Text is
// exactly that case (I1), so the caller keeps its strength rule and a value question
// merges as it always did. dropped is the claim that lost the merge, as TEXT, so the
// caller can keep it as an alternate; the framework stops discarding claims, it does
// not decide which one is true.
func Union(a, b Value) (union, dropped Value, ok bool) {
	if a.IsZero() || b.IsZero() {
		return Value{}, Value{}, false
	}
	switch {
	case a.Kind == KindItems && b.Kind == KindItems:
		// Two member lists never produce a LOSER: the union is the resolution, and a
		// subset union (this side added nothing) is a resolution too — the base is
		// already the answer, so nothing is reported as dropped.
		merged, _ := mergeItems(a.Items, b.Items)
		return Value{Kind: KindItems, Items: merged}, Value{}, true
	case a.Kind == KindItems && isNumeric(b.Kind):
		return a, b, true
	case b.Kind == KindItems && isNumeric(a.Kind):
		return b, a, true
	case isNumeric(a.Kind) && isNumeric(b.Kind):
		an, _ := a.Number()
		bn, _ := b.Number()
		if bn > an {
			return b, a, true
		}
		if bn < an {
			return a, b, true
		}
		// Equal claims: there is no loser to keep. Recording one put "alternate
		// (claimed by another session, not adopted): 13" beside a slot saying 13 —
		// a dropped claim identical to the one that won.
		return a, Value{}, true
	}
	return Value{}, Value{}, false
}

func isNumeric(k Kind) bool { return k == KindCount || k == KindRange }

// mergeItems unions two member lists by name, first evidence wins.
func mergeItems(base, branch []Item) ([]Item, bool) {
	out := make([]Item, 0, len(base)+len(branch))
	seen := make(map[string]int, len(base)+len(branch))
	for _, m := range base {
		name := strings.TrimSpace(m.Value)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = len(out)
		out = append(out, Item{Value: name, ChunkID: m.ChunkID, Quote: m.Quote})
	}
	added := false
	for _, m := range branch {
		name := strings.TrimSpace(m.Value)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if i, dup := seen[key]; dup {
			// The member is already known; a second passage is worth keeping if the
			// first came without one (evidence is what makes a member accountable).
			if out[i].ChunkID == "" && m.ChunkID != "" {
				out[i].ChunkID, out[i].Quote = m.ChunkID, m.Quote
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, Item{Value: name, ChunkID: m.ChunkID, Quote: m.Quote})
		added = true
	}
	return out, added
}

// Parse reads ONE patch entry — the JSON object a session emits for a slot — as a
// typed value. Anything it cannot read as a type is Text: fail closed, never guess.
//
// The accepted shapes (see prompts/action_run.md, which teaches them):
//
//	{"kind":"members","items":[{"name":"华雄","chunk_id":"c1","quote":"提华雄之头"}]}
//	{"kind":"count","count":13}
//	{"kind":"range","lo":17,"hi":19}
//	{"kind":"text","text":"…"}            or  {"candidate":"…"}   (legacy/other)
func Parse(patch map[string]any) Value {
	if patch == nil {
		return Value{}
	}
	switch strings.ToLower(strings.TrimSpace(asString(patch["kind"]))) {
	case "members":
		items := parseItems(patch["items"])
		if len(items) == 0 {
			// Declared a list and gave none: the text (if any) is opaque, not a list.
			return Text(candidateText(patch))
		}
		return Value{Kind: KindItems, Items: items}
	case "count":
		if n, ok := asInt(patch["count"]); ok {
			return Number(n)
		}
		return Text(candidateText(patch))
	case "range":
		lo, okLo := asInt(patch["lo"])
		hi, okHi := asInt(patch["hi"])
		if okLo && okHi {
			return Interval(lo, hi)
		}
		if okLo {
			return Number(lo)
		}
		if okHi {
			return Number(hi)
		}
		return Text(candidateText(patch))
	}
	return Text(candidateText(patch))
}

// candidateText reads the legacy free-text field. A null candidate is an empty value
// (a null candidate — an eliminated claim).
func candidateText(patch map[string]any) string {
	raw, ok := patch["candidate"]
	if !ok || raw == nil {
		if t, has := patch["text"]; has && t != nil {
			return strings.TrimSpace(asString(t))
		}
		return ""
	}
	return strings.TrimSpace(asString(raw))
}

func parseItems(raw any) []Item {
	list, _ := raw.([]any)
	out := make([]Item, 0, len(list))
	for _, item := range list {
		switch it := item.(type) {
		case string:
			if name := strings.TrimSpace(it); name != "" {
				out = append(out, Item{Value: name})
			}
		case map[string]any:
			name := strings.TrimSpace(asString(it["name"]))
			if name == "" {
				// Tolerate {"candidate": …} / {"value": …} spellings, but a missing
				// value is not an item: skip rather than invent one.
				name = strings.TrimSpace(asString(it["candidate"]))
			}
			if name == "" {
				continue
			}
			out = append(out, Item{
				Value:   name,
				ChunkID: strings.TrimSpace(asString(it["chunk_id"])),
				Quote:   strings.TrimSpace(asString(it["quote"])),
			})
		}
	}
	return out
}

func asString(v any) string {
	switch s := v.(type) {
	case nil:
		return ""
	case string:
		return s
	default:
		return fmt.Sprint(s)
	}
}

// asInt reads a JSON number (float64), a numeric string, or a bool-free anything.
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case string:
		s := strings.TrimSpace(n)
		if s == "" {
			return 0, false
		}
		if i, err := strconv.Atoi(s); err == nil {
			return i, true
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return int(f), true
		}
	}
	return 0, false
}
