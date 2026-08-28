package component

import (
	"fmt"
	"strings"
)

// ValidateDynamicEntries rejects blank values in repeatable Agent parameters.
func ValidateDynamicEntries(dsl map[string]any) error {
	return validateDynamicEntries(dsl)
}

func validateDynamicEntries(value any) error {
	switch node := value.(type) {
	case map[string]any:
		if name, ok := node["component_name"].(string); ok {
			if params, ok := node["params"].(map[string]any); ok {
				if err := validateDynamicParams(name, params); err != nil {
					return err
				}
			}
		}
		for _, child := range node {
			if err := validateDynamicEntries(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := validateDynamicEntries(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDynamicParams(component string, params map[string]any) error {
	for _, field := range []string{"include_domains", "exclude_domains", "site_include", "site_exclude", "country_include", "language_include", "delimiters", "children_delimiters"} {
		if values, ok := params[field].([]any); ok && containsBlank(values) {
			return fmt.Errorf("[%s] %s does not support empty entries", component, field)
		}
	}
	if values, ok := params["content"].([]any); ok && containsBlankWithContent(values) {
		return fmt.Errorf("[%s] content does not support empty entries", component)
	}
	for field, required := range map[string][]string{
		"tools": {"component_name"}, "select_keys": {"name"}, "remove_keys": {"name"},
		"updates": {"key", "value"}, "rename_keys": {"old_key", "new_key"},
		"filter_values": {"key", "value"}, "variables": {"variable"},
		"loop_termination_condition": {"variable", "value"}, "outputs": {"name"},
	} {
		if rows, ok := params[field].([]any); ok {
			for i, raw := range rows {
				row, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				for _, key := range required {
					if isBlank(row[key]) {
						return fmt.Errorf("[%s] %s[%d] is incomplete", component, field, i)
					}
				}
			}
		}
	}
	if inputs, ok := params["inputs"].(map[string]any); ok {
		for key, raw := range inputs {
			entry, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if values, ok := entry["options"].([]any); ok && containsBlank(values) {
				return fmt.Errorf("[%s] inputs.%s.options does not support empty entries", component, key)
			}
		}
	}
	return nil
}

func isBlank(value any) bool {
	s, ok := value.(string)
	return ok && strings.TrimSpace(s) == ""
}

func containsBlank(values []any) bool {
	for _, value := range values {
		if isBlank(value) {
			return true
		}
	}
	return false
}

func containsBlankWithContent(values []any) bool {
	blank := false
	content := false
	for _, value := range values {
		if isBlank(value) {
			blank = true
		} else {
			content = true
		}
	}
	return blank && content
}
