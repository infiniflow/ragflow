package tool

import (
	"fmt"
	"strings"
)

// ValidateSearchFilters checks filter shapes while allowing unfinished draft rows.
func ValidateSearchFilters(params map[string]any, draft bool) error {
	for _, field := range []string{"include_domains", "exclude_domains", "site_include", "site_exclude", "country_include", "language_include"} {
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
				if strings.TrimSpace(value) == "" {
					return fmt.Errorf("%s does not support empty entries", field)
				}
			}
		}
	}
	return nil
}
