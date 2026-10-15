// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package service

// NormalizeAgentInput preserves named Begin fields alongside the conversational
// query. Form descriptors contribute their submitted value, including false
// and zero, rather than the descriptor object itself.
func NormalizeAgentInput(query string, inputs map[string]any) any {
	if len(inputs) == 0 {
		if query == "" {
			return nil
		}
		return query
	}
	values := make(map[string]any, len(inputs)+1)
	for name, raw := range inputs {
		if field, ok := raw.(map[string]any); ok {
			if value, exists := field["value"]; exists {
				values[name] = value
				continue
			}
			if _, descriptor := field["type"]; descriptor {
				values[name] = nil
				continue
			}
		}
		values[name] = raw
	}
	if query != "" {
		values["query"] = query
	}
	return values
}
