// Package slots is the VALUE ALGEBRA of a slot table: what a candidate value IS,
// how two values MERGE, and how a table's count is DERIVED.
//
// It exists because the alternative was measured to fail. When a candidate is free
// text, the structure is lost at the boundary — the model writes "约 17-19 人" — and
// every consumer downstream has to GUESS it back: is this a count? a list? are these
// pieces names? Each guess is a rule, the rules interact, and measured (2026-09-16,
// 三国/关羽) the result was a count slot holding "约、人" (the digits rejected as "not
// members", the qualifiers accepted as "members") while fifteen real names sat in
// another slot and the answer reported ten.
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
	// KindMembers is a list of named members, each optionally carrying the chunk that
	// proves it.
	KindMembers
	// KindCount is a claimed number ("13").
	KindCount
	// KindRange is a claimed interval ("约 17-19"), kept as two numbers rather than as
	// a string that someone later tries to parse (measured: "17-19" parsed
	// digit-by-digit is 1719).
	KindRange
)

func (k Kind) String() string {
	switch k {
	case KindText:
		return "text"
	case KindMembers:
		return "members"
	case KindCount:
		return "count"
	case KindRange:
		return "range"
	}
	return "empty"
}

// Member is one named member and the passage that proves it. A member without
// evidence is a member nobody can point at, so the empty ChunkID is allowed but
// visible (renderers show it).
type Member struct {
	Name    string
	ChunkID string
	Quote   string
}

// Value is what one slot holds. Exactly one of the fields matching Kind is set.
type Value struct {
	Kind  Kind
	Text  string   // KindText (the raw candidate, verbatim)
	Items []Member // KindMembers
	Count int      // KindCount
	Lo    int      // KindRange (inclusive)
	Hi    int      // KindRange (inclusive)
}

// Text wraps an opaque string.
func Text(s string) Value { return Value{Kind: KindText, Text: s} }

// Members wraps a member list (evidence may be empty per item).
func Members(items ...Member) Value { return Value{Kind: KindMembers, Items: items} }

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
	case KindMembers:
		return len(v.Items) == 0
	}
	return v.Kind == KindEmpty
}

// Names is the member names in order, deduped case-insensitively.
func (v Value) Names() []string {
	if v.Kind != KindMembers {
		return nil
	}
	seen := make(map[string]bool, len(v.Items))
	out := make([]string, 0, len(v.Items))
	for _, m := range v.Items {
		name := strings.TrimSpace(m.Name)
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
	case KindMembers:
		names := v.Names()
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
	case a.Kind == KindMembers && b.Kind == KindMembers:
		merged, added := mergeMembers(a.Items, b.Items)
		if !added {
			// A subset union is still a resolution: the union IS the base.
			return Value{Kind: KindMembers, Items: merged}, Value{}, true
		}
		return Value{Kind: KindMembers, Items: merged}, Value{}, true
	case a.Kind == KindMembers && isNumeric(b.Kind):
		return a, b, true
	case b.Kind == KindMembers && isNumeric(a.Kind):
		return b, a, true
	case isNumeric(a.Kind) && isNumeric(b.Kind):
		an, _ := a.Number()
		bn, _ := b.Number()
		if bn > an {
			return b, a, true
		}
		return a, b, true
	}
	return Value{}, Value{}, false
}

func isNumeric(k Kind) bool { return k == KindCount || k == KindRange }

// mergeMembers unions two member lists by name, first evidence wins.
func mergeMembers(base, branch []Member) ([]Member, bool) {
	out := make([]Member, 0, len(base)+len(branch))
	seen := make(map[string]int, len(base)+len(branch))
	for _, m := range base {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = len(out)
		out = append(out, Member{Name: name, ChunkID: m.ChunkID, Quote: m.Quote})
	}
	added := false
	for _, m := range branch {
		name := strings.TrimSpace(m.Name)
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
		out = append(out, Member{Name: name, ChunkID: m.ChunkID, Quote: m.Quote})
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
		return Value{Kind: KindMembers, Items: items}
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
// (Python's "set candidate null" — an eliminated claim).
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

func parseItems(raw any) []Member {
	list, _ := raw.([]any)
	out := make([]Member, 0, len(list))
	for _, item := range list {
		switch it := item.(type) {
		case string:
			if name := strings.TrimSpace(it); name != "" {
				out = append(out, Member{Name: name})
			}
		case map[string]any:
			name := strings.TrimSpace(asString(it["name"]))
			if name == "" {
				// Tolerate {"candidate": …} / {"value": …} spellings, but a missing
				// name is not a member: skip rather than invent one.
				name = strings.TrimSpace(asString(it["candidate"]))
			}
			if name == "" {
				continue
			}
			out = append(out, Member{
				Name:    name,
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
