package common

// DeepCopyMap returns a deep copy of a string-keyed map. Nested maps and slices
// are copied recursively; every other value (primitives, structs,
// pointers) is returned by reference, except []byte which is copied
// defensively so a caller that mutates the copy cannot corrupt the original.
// This matches the legacy per-package deepCopyMap copies used by the parser
// config, DSL reset, graph scratchpad, and DSL normalize packages, whose
// values are JSON-native (decoded from or destined for JSON), so sharing
// non-collection values by reference is safe. The graph checkpoint package
// keeps its own deep-copy helper that JSON-round-trips values (int -> float64),
// a deliberately different contract, and is not consolidated here.
//
// A nil input returns nil.
func DeepCopyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = deepCopyValue(v)
	}
	return out
}

// DeepCopySlice returns a deep copy of a slice. See DeepCopyMap for the
// recursion and value-copy semantics. A nil input returns nil.
func DeepCopySlice(src []any) []any {
	if src == nil {
		return nil
	}
	out := make([]any, len(src))
	for i, v := range src {
		out[i] = deepCopyValue(v)
	}
	return out
}

// DeepCopyValue returns a deep copy of an arbitrary value using the same
// recursion and value-copy rules as DeepCopyMap/DeepCopySlice. It is the
// value-level entry point used when a single value (rather than a container)
// must be deep-copied.
func DeepCopyValue(v any) any {
	return deepCopyValue(v)
}

func deepCopyValue(v any) any {
	if v == nil {
		return nil
	}
	switch typed := v.(type) {
	case map[string]any:
		return DeepCopyMap(typed)
	case []any:
		return DeepCopySlice(typed)
	default:
		// Non-collection values are shared by reference. The callers above deal
		// in JSON-native data, so this is safe; a []byte is copied defensively
		// so a future caller that mutates the copy cannot corrupt the original.
		if b, ok := typed.([]byte); ok {
			return append([]byte(nil), b...)
		}
		return v
	}
}
