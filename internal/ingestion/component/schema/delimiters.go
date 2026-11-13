package schema

import "fmt"

// ValidateDelimiterEntries permits unfinished draft rows but requires string arrays.
// Whitespace is a meaningful split token, so only zero-length strings are incomplete.
func ValidateDelimiterEntries(params map[string]any, draft bool) error {
	for _, field := range []string{"delimiters", "children_delimiters"} {
		raw, present := params[field]
		if !present || raw == nil {
			continue
		}
		var values []string
		switch entries := raw.(type) {
		case []string:
			values = entries
		case []any:
			for _, entry := range entries {
				value, ok := entry.(string)
				if !ok {
					return fmt.Errorf("%s must contain strings", field)
				}
				values = append(values, value)
			}
		default:
			return fmt.Errorf("%s must be an array", field)
		}
		if !draft {
			for _, value := range values {
				if value == "" {
					return fmt.Errorf("%s does not support empty entries", field)
				}
			}
		}
	}
	return nil
}
