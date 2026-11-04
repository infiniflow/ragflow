package document

import "ragflow/internal/entity"

// deepCopyJSONMap returns an independent copy of in, including all nested
// maps and slices. A nil input yields nil.
func deepCopyJSONMap(in entity.JSONMap) entity.JSONMap {
	if in == nil {
		return nil
	}
	out := make(entity.JSONMap, len(in))
	for k, v := range in {
		out[k] = deepCopyValue(v)
	}
	return out
}

// deepCopyValue recursively copies maps and slices; scalar values are shared
// (they are immutable from the caller's perspective).
func deepCopyValue(v any) any {
	switch val := v.(type) {
	case nil:
		return nil
	case entity.JSONMap:
		out := make(entity.JSONMap, len(val))
		for k, vv := range val {
			out[k] = deepCopyValue(vv)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			out[k] = deepCopyValue(vv)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			out[i] = deepCopyValue(vv)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(val))
		for i, vv := range val {
			out[i] = deepCopyValue(vv).(map[string]any)
		}
		return out
	default:
		return v
	}
}
