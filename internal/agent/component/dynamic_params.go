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
	for _, field := range []string{
		"include_domains", "exclude_domains", "site_include", "site_exclude",
		"country_include", "language_include",
	} {
		if values, ok := params[field].([]any); ok && containsBlank(values) {
			return fmt.Errorf("[%s] %s does not support empty entries", component, field)
		}
	}
	// Delimiter entries are split tokens, not names: whitespace-only values
	// ("\n", "\t") are valid delimiters — the chunker form's default row is a
	// real newline — so only a zero-length string marks an incomplete row.
	for _, field := range []string{"delimiters", "children_delimiters"} {
		if values, ok := params[field].([]any); ok && containsEmpty(values) {
			return fmt.Errorf("[%s] %s does not support empty entries", component, field)
		}
	}
	switch strings.ToLower(component) {
	case "message":
		if err := validateMessageContent(params); err != nil {
			return fmt.Errorf("[%s] %w", component, err)
		}
	case "dataoperations":
		for _, field := range []string{"select_keys", "remove_keys"} {
			if values, ok := params[field].([]any); ok && containsBlank(values) {
				return fmt.Errorf("[%s] %s does not support empty entries", component, field)
			}
		}
		for field, required := range map[string][]string{
			"updates": {"key", "value"}, "rename_keys": {"old_key", "new_key"},
			"filter_values": {"key", "operator", "value"},
		} {
			if err := validateRows(component, params, field, required); err != nil {
				return err
			}
		}
	case "agent":
		if values, ok := params["tools"].([]any); ok && containsBlank(values) {
			return fmt.Errorf("[%s] tools does not support empty entries", component)
		}
	case "invoke":
		if err := validateRows(component, params, "variables", []string{"key", "ref", "value"}); err != nil {
			return err
		}
	case "loop":
		if err := validateRows(component, params, "loop_variables", []string{"variable"}); err != nil {
			return err
		}
		if err := validateRows(component, params, "loop_termination_condition", []string{"variable", "operator", "value"}); err != nil {
			return err
		}
	case "iteration":
		return validateIterationOutputs(component, params)
	case "variableaggregator":
		return validateVariableAggregatorGroups(component, params)
	case "userfillup":
		return validateInputOptions(component, params)
	}
	return nil
}

func validateMessageContent(params map[string]any) error {
	if text, ok := params["text"]; ok {
		if !isNonBlankString(text) {
			return fmt.Errorf("content does not support empty value")
		}
		return nil
	}
	content, ok := params["content"]
	if !ok {
		return fmt.Errorf("content does not support empty value")
	}
	switch values := content.(type) {
	case string:
		if !isNonBlankString(values) {
			return fmt.Errorf("content does not support empty value")
		}
	case []string:
		for _, value := range values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("content does not support empty entries")
			}
		}
	case []any:
		if len(values) == 0 {
			return fmt.Errorf("content does not support empty value")
		}
		for _, value := range values {
			if !isNonBlankString(value) {
				return fmt.Errorf("content does not support empty entries")
			}
		}
	default:
		return fmt.Errorf("content does not support empty value")
	}
	return nil
}

func validateRows(component string, params map[string]any, field string, required []string) error {
	rows, ok := params[field].([]any)
	if !ok {
		return nil
	}
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
	return nil
}

func validateVariableAggregatorGroups(component string, params map[string]any) error {
	if groups, ok := params["groups"].([]any); ok {
		for i, rawGroup := range groups {
			group, ok := rawGroup.(map[string]any)
			if !ok || group == nil {
				return fmt.Errorf("[%s] groups[%d] is incomplete", component, i)
			}
			if !isNonBlankString(group["group_name"]) {
				return fmt.Errorf("[%s] groups[%d] is incomplete", component, i)
			}
			variables, ok := group["variables"].([]any)
			if !ok {
				return fmt.Errorf("[%s] groups[%d].variables is incomplete", component, i)
			}
			for j, rawVariable := range variables {
				variable, ok := rawVariable.(map[string]any)
				if !ok || variable == nil || !isNonBlankString(variable["value"]) {
					return fmt.Errorf("[%s] groups[%d].variables[%d] is incomplete", component, i, j)
				}
			}
		}
	}
	return nil
}

func validateIterationOutputs(component string, params map[string]any) error {
	outputs, ok := params["outputs"].(map[string]any)
	if !ok {
		return nil
	}
	for name, rawOutput := range outputs {
		output, ok := rawOutput.(map[string]any)
		if isBlank(name) || !ok || output == nil || !isNonBlankString(output["ref"]) {
			return fmt.Errorf("[%s] outputs.%s is incomplete", component, name)
		}
	}
	return nil
}

func validateInputOptions(component string, params map[string]any) error {
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

func isNonBlankString(value any) bool {
	s, ok := value.(string)
	return ok && strings.TrimSpace(s) != ""
}

func containsBlank(values []any) bool {
	for _, value := range values {
		if isBlank(value) {
			return true
		}
	}
	return false
}

// containsEmpty reports whether any entry is a zero-length string. Unlike
// containsBlank it keeps whitespace-only entries, which are meaningful
// delimiter values.
func containsEmpty(values []any) bool {
	for _, value := range values {
		if s, ok := value.(string); ok && s == "" {
			return true
		}
	}
	return false
}
