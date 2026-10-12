package component

import "ragflow/internal/parser/position"

func locationField(chunk map[string]any, key string) any {
	copy := make(map[string]any, len(chunk))
	for k, v := range chunk {
		copy[k] = v
	}
	if err := position.ProjectPublic(copy); err != nil {
		return nil
	}
	return copy[key]
}

func locationFieldPresent(chunk map[string]any, key string) (any, bool) {
	copy := make(map[string]any, len(chunk))
	for k, v := range chunk {
		copy[k] = v
	}
	if err := position.ProjectPublic(copy); err != nil {
		return nil, false
	}
	v, ok := copy[key]
	return v, ok
}
