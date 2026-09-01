package common

import (
	"fmt"
	"sort"
	"strings"
)

// IndexExcludedKeys names payload keys that must never reach the embedding or
// the BM25 columns.
//
// "evidence" carries verbatim source quotes for claim/evidence compilation
// (see rag/advanced_rag/knowlege_compile/claim_evidence.md). Feeding it into
// the vector would embed a mixed "claim + raw source" unit instead of the
// claim the geometric layer is meant to index, and a []any of maps would
// stringify to Go map syntax and pollute the tokens. It is kept out of BM25 in
// the first phase too, so the payload change stays isolated from any recall
// change.
var IndexExcludedKeys = map[string]bool{"evidence": true}

// PayloadDescription is the single source of truth for the text a compiled row
// is indexed by. It mirrors Python _struct_payload_description in
// rag/advanced_rag/knowlege_compile/structure.py: the concatenated string
// values of every payload field, with slices flattened.
//
// Go maps lose JSON insertion order, so keys are sorted to keep the embedding
// deterministic across runs.
//
// excluded names additional payload keys to skip. A nil map means "use
// IndexExcludedKeys". Callers that build the embedding and the BM25 columns
// may pass different sets, which lets evidence stay out of the vector while a
// separate BM25 trial remains possible.
//
// NOTE: the tree variant used to skip "description" here while claiming to
// match Python; that dropped the primary semantic content from every tree row
// vector and diverged from the structure variant. Both variants now share this
// implementation.
func PayloadDescription(payload map[string]any, excluded map[string]bool) string {
	if excluded == nil {
		excluded = IndexExcludedKeys
	}
	keys := make([]string, 0, len(payload))
	for k := range payload {
		if !excluded[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		switch v := payload[k].(type) {
		case []any:
			for _, item := range v {
				if s := strings.TrimSpace(StringOf(item)); s != "" {
					parts = append(parts, s)
				}
			}
		case []string:
			for _, item := range v {
				if s := strings.TrimSpace(item); s != "" {
					parts = append(parts, s)
				}
			}
		default:
			if s := strings.TrimSpace(StringOf(v)); s != "" {
				parts = append(parts, s)
			}
		}
	}
	return strings.Join(parts, " ")
}

// StringOf renders a scalar payload value for index text. Maps and slices
// render as "" because they are not scalar text; callers flatten them
// themselves.
func StringOf(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strings.TrimSuffix(fmt.Sprintf("%v", x), ".0")
	case float32:
		return strings.TrimSuffix(fmt.Sprintf("%v", x), ".0")
	case int:
		return fmt.Sprintf("%v", x)
	case int64:
		return fmt.Sprintf("%v", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	}
	return ""
}
