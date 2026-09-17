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

// The small helpers every stage shares live in THIS file: JSON extraction, scalar
// coercion, score extraction, string comparison, rune truncation. They live together so
// that a stage needing one of them takes the family from one place — and so that a second
// copy of any of them is not where two stages start disagreeing about the same value
// (`stringOf` and `anyString` used to be exactly that pair).
package advanced_rag

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/rag/advanced_rag/harness"
)

// similarityOrScore is the ONE reading of a chunk's relevance: its `similarity`,
// falling through to `score` when that is missing or zero — a FALSY similarity must not
// win over a present score, so this is deliberately not "first key present wins".
// It was written three times (the SCA view, the draft's ordering, the citation
// reference) before it lived here.
func similarityOrScore(c map[string]any) float64 {
	if v, ok := toFloat(c["similarity"]); ok && v != 0 {
		return v
	}
	if v, ok := toFloat(c["score"]); ok && v != 0 {
		return v
	}
	return 0.0
}

// rankByScore returns the chunks strongest-first. Stable, so equal-scored chunks keep
// their retrieval order.
func rankByScore(chunks []map[string]any) []map[string]any {
	out := append([]map[string]any(nil), chunks...)
	sort.SliceStable(out, func(i, j int) bool {
		return similarityOrScore(out[i]) > similarityOrScore(out[j])
	})
	return out
}

// queryToTerms is the harness keywords helper the SCA view reads: lowercase word
// tokens of length >= 3, de-duplicated, order preserved.
func queryToTerms(q string) []string {
	if q == "" {
		return nil
	}
	found := tokenPattern.FindAllString(strings.ToLower(q), -1)
	out := make([]string, 0, len(found))
	seen := make(map[string]bool, len(found))
	for _, t := range found {
		if len(t) < 3 || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// truncateRunes caps a string to n runes without breaking multi-byte chars.
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// dedupe preserves order and drops empty / repeated entries.
func dedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// asSliceOfAny coerces a JSON array value to []any.
//
// isPoisoned covers the values that equally must never reach a prompt (see its doc
// comment).
func asSliceOfAny(v any) []any {
	if isPoisoned(v) {
		_LOG.Printf("[StateGuard] dropping poisoned value of kind %s; treating as empty",
			reflect.ValueOf(v).Kind())
		return nil
	}
	switch x := v.(type) {
	case nil:
		return nil
	case []any:
		return x
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case map[string]any:
		out := make([]any, 0, len(x))
		for _, vv := range x {
			out = append(out, vv)
		}
		return out
	default:
		return []any{v}
	}
}

// toIntStrict parses an int-like value (JSON numbers arrive as float64).
func toIntStrict(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case string:
		var n int
		if _, err := fmt.Sscanf(x, "%d", &n); err == nil {
			return n, true
		}
	}
	return 0, false
}

// toFloat parses a numeric value.
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		var f float64
		if _, err := fmt.Sscanf(x, "%f", &f); err == nil {
			return f, true
		}
	}
	return 0, false
}

// anyString reads a string field from a chunk-like map.
func anyString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}

// truncateEach caps every entry of a string slice.
func truncateEach(in []string, n int) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = truncateRunes(s, n)
	}
	return out
}

// equalStringPtr reports whether two optional strings are equal.
func equalStringPtr(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// equalStrings reports set-equality (order-independent) of two string slices.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]int, len(a))
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		if seen[s] == 0 {
			return false
		}
		seen[s]--
	}
	return true
}

// isPoisoned reports whether a state value must be discarded before it reaches a
// prompt: channels and function values, neither of which renders as anything a model
// can read.
func isPoisoned(v any) bool {
	if v == nil {
		return false
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return true
	}
	return false
}

// extractJSONObject: return the
// first parseable JSON object in text, or nil when none parses.
//
// The fan-out model sometimes emits prose around the object, and a greedy
// brace-match would capture several objects and fail with "extra data"; each
// candidate is therefore validated before it is accepted, and an invalid one
// resumes the scan at its next "{".
func extractJSONObject(text string) any {
	for i := 0; i < len(text); {
		rel := strings.IndexByte(text[i:], '{')
		if rel < 0 {
			return nil
		}
		start := i + rel
		depth := 0
	scan:
		for j := start; j < len(text); j++ {
			switch text[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					var out map[string]any
					if json.Unmarshal([]byte(text[start:j+1]), &out) == nil {
						return out
					}
					break scan // not a valid object; try the next "{"
				}
			}
		}
		i = start + 1
	}
	return nil
}

// jsonModelAdapter adapts harness.SessionModel to orchestrator.JSONModel.
//
// The rendered prompt is the system turn and "Output:\n" the user turn (fitted ONCE
// to the model's context window), then the first JSON value is parsed out of the reply.
// Malformed JSON is retried (up to genJSONMaxRetry calls) with the model's own
// bad answer and the parse error appended to the user turn, so a single
// formatting hiccup does not abort the SCA review or the query rewrite.
type jsonModelAdapter struct {
	inner harness.SessionModel
	// maxLength is the chat model's context window. It bounds the message fit so an
	// oversized prompt is trimmed instead of rejected by the provider. <=0 falls back
	// to chat.EffectiveContextLength's 8192 default.
	maxLength int
}

// parseGenJSONReply parses a cleaned model reply with gen_json's tolerance: a
// strict decode of ANY top-level JSON value first (json_repair accepts objects,
// arrays and scalars alike), then the brace-matched object extractor for fenced
// or damaged objects — the JSON-shaped gate, NOT the prose-salvaging ExtractJSON,
// so a non-JSON reply stays a parse failure and triggers the corrective retry.
// The boolean distinguishes a successful decode from a failure, because a
// legitimate `null` reply decodes to a nil value.
func parseGenJSONReply(cleaned string) (any, bool) {
	var val any
	if err := json.Unmarshal([]byte(strings.TrimSpace(cleaned)), &val); err == nil {
		return val, true
	}
	if v := harness.ExtractJSONObject(cleaned); v != nil {
		return v, true
	}
	return nil, false
}

// stripGenJSONWrappers mirrors gen_json's answer cleanup:
//
//	ans = re.sub(r"(^.*</think>|```json\n|```\n*$)", "", ans, flags=re.DOTALL)
//
// The think term (greedy up to the LAST </think>) is common.StripThinkTrailing;
// a "```json\n" fence may occur anywhere and is removed wholesale; a trailing
// "```" (plus newlines) is cut from the end.
func stripGenJSONWrappers(s string) string {
	s = common.StripThinkTrailing(s)
	s = strings.ReplaceAll(s, "```json\n", "")
	return genJSONTailFenceRE.ReplaceAllString(s, "")
}

func toAnySlice(ss []string) []any {
	out := make([]any, 0, len(ss))
	for _, s := range ss {
		out = append(out, s)
	}
	return out
}
