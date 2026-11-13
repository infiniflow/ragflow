package utility

import "encoding/json"

// MustJSON marshals v to a compact JSON string. It uses json.Marshal so the
// result has no trailing newline. Callers pass statically-typed values, so a
// marshal error is unreachable in practice; on error it returns "".
func MustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
