// Package dsl — v2 -> v1 converter (Phase 5.5).
//
// The reverse direction of converter_v1_to_v2.go. The motivation is to keep
// the v2 -> v1 round-trip closed: a canvas loaded from v1, upgraded to v2,
// and then serialized back to v1 must be load-equivalent through the v1
// reader (i.e. it must parse into the same v2 in-memory model).
//
// v1 format reference (see agent/canvas.py:43-95):
//
//	{
//	  "components": {
//	    "<ComponentName>:<UUID>": {
//	      "downstream": ["<ComponentName>:<UUID>", ...],
//	      "upstream":   ["<ComponentName>:<UUID>", ...],   // optional
//	      "obj": {
//	        "component_name": "ComponentName",
//	        "params":         {...},
//	        "downstream":     [...],   // duplicate of the outer field
//	        "upstream":       [...]    // optional
//	      }
//	    }
//	  }
//	}
//
// v2 -> v1 algorithm:
//   - For each v2 Component (in deterministic order — sorted by v2 ID, with
//     the "begin_" prefix naturally first):
//     1. Reverse the v2 ID "<name>_<UUID>" back to a v1 key "<Name>:<UUID>".
//     The split happens on the FIRST "_" from the left. The left half is
//     the lowercased Name, the right half is the (lowercased) UUID. We
//     apply a best-effort PascalCase restore to the name by upper-casing
//     its first rune: "begin_abc" -> "Begin:abc", "agent_abc" -> "Agent:abc".
//     The UUID is emitted as-is. The reverse is LOSSY when the original
//     name was already lowercase (e.g. "begin" -> "begin_" -> "Begin:")
//     or when the original name was multi-segment PascalCase (e.g.
//     "LLM" -> "llm_xxx" -> "Llm:xxx" not "LLM:xxx"). Round-trip
//     structural equivalence through v1ToV2 is preserved (v1ToV2 always
//     lowercases on the way in), but the v1 key string itself is not
//     byte-for-byte restorable.
//     2. Build a v1 entry: { "downstream": [v1 keys], "obj":
//     { "component_name": name, "params": params,
//     "downstream": [same list, duplicated per v1 format] } }.
//   - Return JSON bytes of { "components": { ... } } with 2-space indent.
//
// Edge cases handled:
//   - Component with no Downstream: emit "downstream": [] (NOT null).
//   - Component with nil Params:    emit "params":     {} (NOT null).
//   - v2 ID with no "_" (e.g. legacy fixture): emit as "<Name>:" (left-side
//     handling — see convertKey for the forward direction).
//   - v2 ID with multiple "_" (e.g. "switch_abc_def"): split on the FIRST
//     "_" from the left ("switch" + "abc_def") — see plan §5 Phase 5.5.
//   - Legacy v1 fields (_deprecated_params, _feeded_deprecated_params,
//     _user_feeded_params) are NEVER emitted; v2 does not carry them.
//
// Upstream is intentionally omitted from the v1 emit. The Python v1 reader
// computes upstream by inverting downstream (it is the dual of the graph
// edge relation) and tolerates its absence. This keeps the converter
// minimal and aligned with the plan.
package dsl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// v2ToV1 serializes a v2 Canvas back into the v1 JSON envelope.
//
// The output is deterministic: components are emitted in lexicographic
// order of their v2 ID (with the "begin_" prefix naturally first).
//
// Returns an error if the canvas is nil, empty, or fails Validate.
// Use json.MarshalIndent with 2-space indent to keep the output
// human-readable, matching the rest of the dsl package's conventions.
func v2ToV1(c *Canvas) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("dsl: v2ToV1: nil canvas")
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("dsl: v2ToV1: validate: %w", err)
	}
	if len(c.Components) == 0 {
		return nil, fmt.Errorf("dsl: v2ToV1: empty components")
	}

	// Deterministic iteration order: the "begin" component (v2 id
	// "begin_...") sorts first to match the v1 reader's convention;
	// remaining ids are sorted lexicographically.
	ids := make([]string, 0, len(c.Components))
	for id := range c.Components {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		iBegin := strings.HasPrefix(ids[i], "begin_") || ids[i] == "begin_"
		jBegin := strings.HasPrefix(ids[j], "begin_") || ids[j] == "begin_"
		if iBegin != jBegin {
			return iBegin
		}
		return ids[i] < ids[j]
	})

	out := v1Envelope{Components: make(map[string]v1Emit, len(c.Components))}

	for _, v2ID := range ids {
		cpn := c.Components[v2ID]

		// Reverse the v2 id to a v1 key. Lossy case-restore is acceptable;
		// the structural invariant (v1ToV2 closure) is preserved.
		v1Key := reverseIDToV1Key(v2ID)

		// Remap each downstream v2 id to its v1 key form.
		ds := cpn.Downstream
		if ds == nil {
			ds = []string{}
		}
		dsOut := make([]string, 0, len(ds))
		for _, d := range ds {
			dsOut = append(dsOut, reverseIDToV1Key(d))
		}

		// Params: ensure non-nil so the v1 emit is "{}" not "null".
		params := cpn.Params
		if params == nil {
			params = map[string]any{}
		}

		out.Components[v1Key] = v1Emit{
			Downstream: dsOut,
			Obj: &v1ObjEmit{
				ComponentName: cpn.Name,
				Params:        params,
				Downstream:    dsOut,
			},
		}
	}

	return json.MarshalIndent(out, "", "  ")
}

// v1Envelope is the on-the-wire v1 shape we emit. Only the fields the
// Python reader actually consumes are present.
//
// We override MarshalJSON to guarantee deterministic key order (Begin
// first, then lexicographic). The default Go map encoder sorts by
// key text, which would place "Alpha" before "Begin" — not what
// canonical v1 emits do.
type v1Envelope struct {
	Components map[string]v1Emit `json:"components"`
}

// MarshalJSON renders the v1 envelope with deterministic component order.
// The "Begin" key sorts first; the remaining keys sort lexicographically.
// This is the ordering convention used by the Python v1 writer, and
// tests like TestV2ToV1_BeginFirst depend on it.
//
// We hand-build the indented output rather than relying on the default
// map encoder, which sorts by key text and would place "Alpha" before
// "Begin" — not what canonical v1 emits do.
func (e v1Envelope) MarshalJSON() ([]byte, error) {
	keys := make([]string, 0, len(e.Components))
	for k := range e.Components {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		iBegin := isBeginV1Key(keys[i])
		jBegin := isBeginV1Key(keys[j])
		if iBegin != jBegin {
			return iBegin
		}
		return keys[i] < keys[j]
	})

	// Re-marshal each entry at indent level 2 and stitch them into
	// the outer envelope. json.Indent rejects a pre-indented body, so
	// we keep the per-value marshaling compact and use Indent on the
	// final blob to apply 2-space layout to the top-level braces and
	// "components" key as well.

	// Build a compact but ordered JSON manually.
	var buf bytes.Buffer
	buf.WriteString(`{"components": {`)
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(e.Components[k])
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteString(`: `)
		buf.Write(vb)
	}
	buf.WriteString(`}}`)

	// Apply 2-space indent across the whole envelope.
	var out bytes.Buffer
	if err := json.Indent(&out, buf.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// isBeginV1Key reports whether a v1 key corresponds to the Begin
// component. Forms covered:
//   - "begin"             (legacy no-colon form, after v1ToV2 round-trip)
//   - "Begin"             (restored no-colon form from v2ToV1)
//   - "Begin:..."         (PascalCase + colon)
//   - "begin:..."         (lowercase, tolerated)
func isBeginV1Key(k string) bool {
	if k == "begin" || k == "Begin" {
		return true
	}
	return strings.HasPrefix(k, "Begin:") || strings.HasPrefix(k, "begin:")
}

// v1Emit is one entry in the v1 components map. The `upstream` field is
// intentionally omitted — see file-level comment.
type v1Emit struct {
	Downstream []string   `json:"downstream"`
	Obj        *v1ObjEmit `json:"obj"`
}

// v1ObjEmit is the `obj` sub-object of a v1 component. Mirrors v1Obj in
// converter_v1_to_v2.go with the legacy fields removed (we never re-emit
// them — v2 dropped them on the way in).
type v1ObjEmit struct {
	ComponentName string         `json:"component_name"`
	Params        map[string]any `json:"params"`
	Downstream    []string       `json:"downstream"`
}

// reverseIDToV1Key reverses a v2 ID "<name>_<UUID>" back to a v1 key
// "<Name>:<UUID>".
//
// Splitting happens on the FIRST "_" from the left. The left half is
// treated as the lowercased Name; we apply a best-effort PascalCase
// restore by upper-casing the first rune only. The right half (the UUID
// half) is preserved verbatim — it was already lowercased by v1ToV2 in
// the canonical pipeline, so the emitted UUID will be lowercase.
//
// Edge cases:
//   - Empty uuid half (trailing "_", e.g. "begin_" from a v1 key
//     "begin" that lacked a colon): emit "<Name>" with NO colon. The
//     Python v1 reader accepts no-colon keys for Begin/End, and
//     convertKey on the round-trip returns the same "begin_". This
//     matches v1ToV2's no-colon branch.
//   - No "_" in the id at all (rare; should not happen in well-formed
//     v2): emit the whole id upper-cased on the first rune, followed
//     by ":" — same shape as the populated case for tolerance.
//   - Multiple "_" (e.g. "switch_abc_def"): split on the FIRST "_" so
//     the uuid half is "abc_def".
//   - All-uppercase name (e.g. "LLM_abc"): v1ToV2 lowercases the name,
//     so v2 ID is "llm_abc"; the restore produces "Llm:abc" — lossy vs.
//     the original "LLM:abc" but structurally equivalent after v1ToV2.
func reverseIDToV1Key(v2ID string) string {
	if v2ID == "" {
		return ""
	}
	idx := strings.Index(v2ID, "_")
	if idx < 0 {
		return upperFirstRune(v2ID) + ":"
	}
	name := v2ID[:idx]
	uuid := v2ID[idx+1:]
	if uuid == "" {
		// Trailing underscore means the original v1 key had no colon.
		// Emit without ":" so v1ToV2 can re-parse it (it requires a
		// non-empty uuid on the colon path; the no-colon branch
		// handles the legacy form).
		return upperFirstRune(name)
	}
	return upperFirstRune(name) + ":" + uuid
}

// upperFirstRune returns s with its first rune converted to upper case.
// ASCII-only on purpose: v1 component names are ASCII identifiers
// (PascalCase). Operates on bytes to keep it allocation-free for the
// common case where s is non-empty and starts with a letter.
func upperFirstRune(s string) string {
	if s == "" {
		return s
	}
	// Fast ASCII path: 'a'..'z' -> 'A'..'Z'. Non-ASCII (multi-byte rune)
	// falls back to the generic Unicode upper-case.
	b := s[0]
	if b >= 'a' && b <= 'z' {
		return string(b-'a'+'A') + s[1:]
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
