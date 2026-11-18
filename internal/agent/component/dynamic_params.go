package component

import (
	"errors"
	"fmt"
	"ragflow/internal/agent/runtime"
	"strings"

	"ragflow/internal/agent/tool"
	"ragflow/internal/ingestion/component/schema"
)

// ErrCompilationTemplateRequired identifies a Compiler component without a
// selected compilation template or template group.
var ErrCompilationTemplateRequired = errors.New("Compilation Template Group does not support empty value")

// ValidateDynamicEntries rejects blank values in repeatable Agent parameters.
func ValidateDynamicEntries(dsl map[string]any) error {
	return validateDynamicEntries(dsl, false)
}

// ValidateDraftDynamicEntries allows unfinished Message content, DataOperations
// entries, Invoke mappings, Compiler configuration, input options, Loop settings,
// Iteration outputs, SQL connections, search filters, chunker delimiters, and Agent
// tool entries and aggregation groups in saved drafts.
// All other checks remain unchanged.
func ValidateDraftDynamicEntries(dsl map[string]any) error {
	return validateDynamicEntries(dsl, true)
}

func validateDynamicEntries(value any, draft bool) error {
	switch node := value.(type) {
	case map[string]any:
		if name, ok := node["component_name"].(string); ok {
			if params, ok := node["params"].(map[string]any); ok {
				if err := validateDynamicParams(name, params, draft); err != nil {
					return err
				}
			}
		}
		for _, child := range node {
			if err := validateDynamicEntries(child, draft); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := validateDynamicEntries(child, draft); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDynamicParams(component string, params map[string]any, draft bool) error {
	if err := tool.ValidateSearchFilters(params, draft); err != nil {
		return fmt.Errorf("[%s] %w", component, err)
	}
	if err := schema.ValidateDelimiterEntries(params, draft); err != nil {
		return fmt.Errorf("[%s] %w", component, err)
	}
	switch strings.ToLower(component) {
	case "message":
		if draft {
			return nil
		}
		if err := validateMessageContent(params); err != nil {
			return fmt.Errorf("[%s] %w", component, err)
		}
	case "dataoperations":
		return validateDataOperationsParams(component, params, draft)
	case "agent":
		if err := validateAgentToolEntries(params["tools"], draft); err != nil {
			return fmt.Errorf("[%s] %w", component, err)
		}
	case "invoke":
		if err := validateInvokeVariables(component, params, draft); err != nil {
			return err
		}
	case "loop":
		for field, required := range map[string][]string{
			"loop_variables": {"variable"}, "loop_termination_condition": {"variable", "operator", "value"},
		} {
			raw := params[field]
			if raw == nil {
				continue
			}
			rows, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("[%s] %s must be an array", component, field)
			}
			for i, raw := range rows {
				row, ok := raw.(map[string]any)
				if !ok || row == nil {
					return fmt.Errorf("[%s] %s[%d] must be an object", component, field, i)
				}
				for _, key := range required {
					if !draft && (row[key] == nil || isBlank(row[key])) {
						return fmt.Errorf("[%s] %s[%d] is incomplete", component, field, i)
					}
				}
			}
		}
	case "iteration", "parallel":
		return validateIterationOutputs(component, params, draft)
	case "variableaggregator":
		return validateVariableAggregatorGroups(component, params, draft)
	case "userfillup":
		return runtime.ValidateUserFillUpOptions(params, draft)
	case "exesql":
		if err := validateExeSQLParams(params, draft); err != nil {
			return fmt.Errorf("[%s] %w", component, err)
		}
	case "compiler":
		if !draft && !isNonBlankString(params["compilation_template_id"]) && !isNonBlankString(params["compilation_template_group_id"]) {
			return fmt.Errorf("[%s] %w", component, ErrCompilationTemplateRequired)
		}
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
		if len(values) == 0 {
			return fmt.Errorf("content does not support empty value")
		}
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

// validateInvokeVariables requires every Invoke variables row to carry a
// nonblank key plus one value source: an upstream reference (ref) or a
// literal (value). The two sources are alternatives — a reference row has
// no literal and a literal row has no reference — so a row with neither
// is still rejected. Omitted and null variables carry no rows to check;
// any other non-array value is malformed and rejected. Drafts retain unfinished
// mappings while still rejecting malformed containers, rows, and field types.
func validateInvokeVariables(component string, params map[string]any, draft bool) error {
	raw, present := params["variables"]
	if !present || raw == nil {
		return nil
	}
	rows, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("[%s] variables must be an array", component)
	}
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok || row == nil {
			return fmt.Errorf("[%s] variables[%d] is incomplete", component, i)
		}
		for _, key := range []string{"key", "ref"} {
			if value := row[key]; value != nil {
				if _, ok := value.(string); !ok {
					return fmt.Errorf("[%s] variables[%d] is incomplete: %s must be a string", component, i, key)
				}
			}
		}
		if !draft && (!isNonBlankString(row["key"]) || !invokeRowHasSource(row)) {
			return fmt.Errorf("[%s] variables[%d] is incomplete", component, i)
		}
	}
	return nil
}

// invokeRowHasSource reports whether an Invoke variables row supplies a
// reference or a literal. Non-string literals (numbers, booleans) count;
// nil does not.
func invokeRowHasSource(row map[string]any) bool {
	if isNonBlankString(row["ref"]) {
		return true
	}
	value := row["value"]
	return value != nil && !isBlank(value)
}

func validateVariableAggregatorGroups(component string, params map[string]any, draft bool) error {
	var p variableAggregatorParam
	if err := p.Update(params); err != nil {
		return fmt.Errorf("[%s] %w", component, err)
	}
	if !draft && len(p.Groups) == 0 {
		return fmt.Errorf("[%s] groups is incomplete", component)
	}
	for i, group := range p.Groups {
		if !draft && !isNonBlankString(group["group_name"]) {
			return fmt.Errorf("[%s] groups[%d] is incomplete", component, i)
		}
		raw, exists := group["variables"]
		if !exists {
			if draft {
				continue
			}
			return fmt.Errorf("[%s] groups[%d].variables is incomplete", component, i)
		}
		var variables []any
		switch v := raw.(type) {
		case []any:
			variables = v
		case []map[string]any:
			for _, entry := range v {
				variables = append(variables, entry)
			}
		default:
			return fmt.Errorf("[%s] groups[%d].variables must be a list", component, i)
		}
		if !draft && len(variables) == 0 {
			return fmt.Errorf("[%s] groups[%d].variables is incomplete", component, i)
		}
		for j, raw := range variables {
			switch v := raw.(type) {
			case string:
			case map[string]any:
				if v == nil {
					return fmt.Errorf("[%s] groups[%d].variables[%d] must be a selector", component, i, j)
				}
				if value, exists := v["value"]; exists {
					if _, ok := value.(string); !ok {
						return fmt.Errorf("[%s] groups[%d].variables[%d].value must be a string", component, i, j)
					}
				}
			default:
				return fmt.Errorf("[%s] groups[%d].variables[%d] must be a selector", component, i, j)
			}
			if !draft && normalizeSelectorRef(raw) == "" {
				return fmt.Errorf("[%s] groups[%d].variables[%d] is incomplete", component, i, j)
			}
		}
	}
	return nil
}

func validateIterationOutputs(component string, params map[string]any, draft bool) error {
	if params["outputs"] == nil {
		return nil
	}
	outputs, ok := params["outputs"].(map[string]any)
	if !ok {
		return fmt.Errorf("[%s] outputs must be an object", component)
	}
	for name, rawOutput := range outputs {
		output, ok := rawOutput.(map[string]any)
		if !ok || output == nil {
			return fmt.Errorf("[%s] outputs.%s is incomplete: must be an object", component, name)
		}
		if !draft && (isBlank(name) || !isNonBlankString(output["ref"])) {
			return fmt.Errorf("[%s] outputs.%s is incomplete", component, name)
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

func validateAgentToolEntries(raw any, draft bool) error {
	if raw == nil {
		return nil
	}
	var values []any
	switch tools := raw.(type) {
	case []any:
		values = tools
	case []string:
		for _, tool := range tools {
			values = append(values, tool)
		}
	default:
		return errors.New("tools must be an array")
	}
	for _, raw := range values {
		incomplete := false
		switch tool := raw.(type) {
		case string:
			incomplete = strings.TrimSpace(tool) == ""
		case map[string]any:
			_, _, ok := agentToolObject(tool)
			incomplete = !ok
		default:
			return errors.New("tools entries must be strings or objects")
		}
		if incomplete && !draft {
			return errors.New("tools does not support empty entries")
		}
	}
	return nil
}

func validateDataOperationsParams(component string, params map[string]any, draft bool) error {
	for _, field := range []string{"select_keys", "remove_keys", "updates", "rename_keys", "filter_values"} {
		raw := params[field]
		if raw == nil {
			continue
		}
		var rows []any
		switch values := raw.(type) {
		case []any:
			rows = values
		case []string:
			for _, value := range values {
				rows = append(rows, value)
			}
		case []map[string]any:
			for _, value := range values {
				rows = append(rows, value)
			}
		default:
			return fmt.Errorf("[%s] %s must be an array", component, field)
		}
		for index, rawRow := range rows {
			if field == "select_keys" || field == "remove_keys" {
				value, ok := rawRow.(string)
				if !ok {
					return fmt.Errorf("[%s] %s[%d] must be a string", component, field, index)
				}
				if !draft && strings.TrimSpace(value) == "" {
					return fmt.Errorf("[%s] %s does not support empty entries", component, field)
				}
				continue
			}
			row, ok := rawRow.(map[string]any)
			if !ok || row == nil {
				return fmt.Errorf("[%s] %s[%d] must be an object", component, field, index)
			}
			required := []string{"key", "value"}
			if field == "rename_keys" {
				required = []string{"old_key", "new_key"}
			}
			if field == "filter_values" {
				required = []string{"key", "operator", "value"}
			}
			for _, key := range required {
				value := row[key]
				if key != "value" && value != nil {
					if _, ok := value.(string); !ok {
						return fmt.Errorf("[%s] %s[%d].%s must be a string", component, field, index, key)
					}
				}
				if !draft && (value == nil || isBlank(value)) {
					return fmt.Errorf("[%s] %s[%d] is incomplete", component, field, index)
				}
			}
		}
	}
	return nil
}
