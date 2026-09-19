package pipeline

import (
	"encoding/json"
	"strings"
)

// RebaseStoredParamsOnPipelineEdit drops component params that merely echo the
// pipeline's previous DSL defaults, so params the user never customized follow
// the edited DSL instead of the stale snapshot seeded at bind time. Entries for
// components absent from the new DSL are dropped; dataset-scoped keys without a
// component prefix are preserved. Any DSL that fails to yield defaults leaves
// the stored config untouched rather than guessing.
func RebaseStoredParamsOnPipelineEdit(stored map[string]any, oldDSL, newDSL []byte) map[string]any {
	if len(stored) == 0 {
		return stored
	}
	oldDefaults, err := ComponentParamsDefaults(oldDSL)
	if err != nil {
		return stored
	}
	newDefaults, err := ComponentParamsDefaults(newDSL)
	if err != nil {
		return stored
	}
	rebased := make(map[string]any, len(stored))
	for key, value := range stored {
		if !strings.Contains(key, ":") {
			rebased[key] = value
			continue
		}
		params, ok := value.(map[string]any)
		if !ok {
			rebased[key] = value
			continue
		}
		if _, exists := newDefaults[key]; !exists {
			continue
		}
		kept := make(map[string]any, len(params))
		for name, paramValue := range params {
			if oldParams, seen := oldDefaults[key]; seen {
				if defaultValue, has := oldParams[name]; has && jsonEquivalent(paramValue, defaultValue) {
					continue
				}
			}
			kept[name] = paramValue
		}
		if len(kept) > 0 {
			rebased[key] = kept
		}
	}
	return rebased
}

// jsonEquivalent compares two decoded JSON values through their canonical
// serialization, so a float64 read from the database equals a json.Number
// arriving from a request body for the same number.
func jsonEquivalent(a, b any) bool {
	ab, aerr := json.Marshal(a)
	bb, berr := json.Marshal(b)
	return aerr == nil && berr == nil && string(ab) == string(bb)
}
