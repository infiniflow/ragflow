package runtime

import (
	"fmt"
	"strings"
)

// ValidateUserFillUpOptions checks the form shared by component and interrupt execution.
func ValidateUserFillUpOptions(params map[string]any, draft bool) error {
	rawInputs, exists := params["inputs"]
	if !exists {
		return nil
	}
	inputs, ok := rawInputs.(map[string]any)
	if !ok {
		return fmt.Errorf("[UserFillUp] inputs must be an object")
	}
	for key, raw := range inputs {
		entry, ok := raw.(map[string]any)
		if !ok || entry == nil {
			return fmt.Errorf("[UserFillUp] inputs.%s must be an object", key)
		}
		rawOptions, exists := entry["options"]
		if !exists {
			continue
		}
		values, ok := rawOptions.([]any)
		if !ok {
			return fmt.Errorf("[UserFillUp] inputs.%s.options must be an array", key)
		}
		if !draft && hasBlankOption(values) {
			return fmt.Errorf("[UserFillUp] inputs.%s.options does not support empty entries", key)
		}
	}
	return nil
}

func hasBlankOption(values []any) bool {
	for _, value := range values {
		if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
			return true
		}
	}
	return false
}
