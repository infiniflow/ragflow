package component

import (
	"strings"
	"testing"
)

func TestValidateDynamicEntries(t *testing.T) {
	valid := dslWithComponents(
		componentDSL("DataOperations", map[string]any{
			"select_keys":   []any{"title"},
			"remove_keys":   []any{"internal"},
			"filter_values": []any{map[string]any{"key": "status", "operator": "eq", "value": "published"}},
		}),
		componentDSL("Agent", map[string]any{"tools": []any{"web_search"}}),
		componentDSL("Invoke", map[string]any{"variables": []any{map[string]any{"key": "query", "ref": "Begin@query", "value": "query"}}}),
		componentDSL("VariableAggregator", map[string]any{"groups": []any{map[string]any{
			"group_name": "answer",
			"variables":  []any{map[string]any{"value": "LLM@content"}},
		}}}),
		componentDSL("Loop", map[string]any{
			"loop_variables":             []any{map[string]any{"variable": "count", "value": 0}},
			"loop_termination_condition": []any{map[string]any{"variable": "count", "operator": "gte", "value": 3}},
		}),
		componentDSL("UserFillUp", map[string]any{"inputs": map[string]any{
			"choice": map[string]any{"options": []any{"one", "two"}},
		}}),
		componentDSL("Iteration", map[string]any{"outputs": map[string]any{
			"result": map[string]any{"type": "string", "ref": "Iteration@result"},
		}}),
		componentDSL("Message", map[string]any{"text": "hello"}),
		// Whitespace-only delimiters are legitimate split tokens: the web
		// form's default row is "\n" (a real newline) and the delimiter
		// input converts a typed "\n"/"\t" into the raw control character
		// before saving. They must not be rejected as empty entries.
		componentDSL("TokenChunker", map[string]any{
			"delimiter_mode":      "delimiter",
			"delimiters":          []any{"\n"},
			"children_delimiters": []any{"\t"},
		}),
	)

	if err := ValidateDynamicEntries(valid); err != nil {
		t.Fatalf("valid DSL rejected: %v", err)
	}

	tests := []struct {
		name      string
		component map[string]any
		want      string
	}{
		{"string array", componentDSL("DataOperations", map[string]any{"select_keys": []any{"title", " "}}), "select_keys"},
		{"tool name", componentDSL("Agent", map[string]any{"tools": []any{"web_search", ""}}), "tools"},
		{"filter operator", componentDSL("DataOperations", map[string]any{"filter_values": []any{map[string]any{"key": "status", "operator": "", "value": "published"}}}), "filter_values[0]"},
		{"invoke variable", componentDSL("Invoke", map[string]any{"variables": []any{map[string]any{"key": "", "ref": "Begin@query", "value": "query"}}}), "variables[0]"},
		{"malformed group", componentDSL("VariableAggregator", map[string]any{"groups": []any{"not a group"}}), "groups[0]"},
		{"aggregated variable", componentDSL("VariableAggregator", map[string]any{"groups": []any{map[string]any{"group_name": "answer", "variables": []any{map[string]any{"value": ""}}}}}), "groups[0].variables[0]"},
		{"malformed aggregated variable", componentDSL("VariableAggregator", map[string]any{"groups": []any{map[string]any{"group_name": "answer", "variables": []any{nil}}}}), "groups[0].variables[0]"},
		{"loop variable", componentDSL("Loop", map[string]any{"loop_variables": []any{map[string]any{"variable": ""}}}), "loop_variables[0]"},
		{"loop operator", componentDSL("Loop", map[string]any{"loop_termination_condition": []any{map[string]any{"variable": "count", "operator": "", "value": 3}}}), "loop_termination_condition[0]"},
		{"iteration output", componentDSL("Iteration", map[string]any{"outputs": map[string]any{"result": map[string]any{"ref": ""}}}), "outputs.result"},
		{"input option", componentDSL("UserFillUp", map[string]any{"inputs": map[string]any{"choice": map[string]any{"options": []any{"one", ""}}}}), "inputs.choice.options"},
		{"empty delimiter entry", componentDSL("TokenChunker", map[string]any{"delimiters": []any{"ok", ""}}), "delimiters"},
		{"empty children delimiter entry", componentDSL("TokenChunker", map[string]any{"children_delimiters": []any{""}}), "children_delimiters"},
		{"empty message text", componentDSL("Message", map[string]any{"text": " \t "}), "Message"},
		{"empty message content", componentDSL("Message", map[string]any{"content": []any{""}}), "Message"},
		{"missing message content", componentDSL("Message", map[string]any{}), "Message"},
		{"blank message row after valid row", componentDSL("Message", map[string]any{"content": []any{"hello", " "}}), "Message"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateDynamicEntries(dslWithComponents(test.component))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateDynamicEntries() error = %v, want %q", err, test.want)
			}
		})
	}
}

func dslWithComponents(components ...map[string]any) map[string]any {
	items := make(map[string]any, len(components))
	for i, component := range components {
		items[string(rune('a'+i))] = map[string]any{"obj": component}
	}
	return map[string]any{"components": items}
}

func componentDSL(name string, params map[string]any) map[string]any {
	return map[string]any{"component_name": name, "params": params}
}
