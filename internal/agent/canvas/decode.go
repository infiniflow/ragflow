//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package canvas

import "fmt"

// DecodeFromDSL converts a canonical canvas DSL map into a Canvas.
// It accepts both canonical IMPORT shape (`obj.component_name`) and the
// normalized flat shape (`name`/`params`) that NormalizeForCanvas emits.
func DecodeFromDSL(dsl map[string]any) (*Canvas, error) {
	if len(dsl) == 0 {
		return nil, fmt.Errorf("canvas: empty DSL")
	}
	rawComps, ok := dsl["components"].(map[string]any)
	if !ok || len(rawComps) == 0 {
		return nil, fmt.Errorf("canvas: no components")
	}
	c := &Canvas{
		Components:  make(map[string]CanvasComponent, len(rawComps)),
		NodeParents: make(map[string]string),
	}
	if p, ok := dsl["path"].([]any); ok {
		c.Path = make([]string, 0, len(p))
		for _, v := range p {
			if s, ok := v.(string); ok {
				c.Path = append(c.Path, s)
			}
		}
	}
	if p, ok := dsl["globals"].(map[string]any); ok {
		c.Globals = p
	}
	c.History = decodeHistory(dsl["history"])
	c.Memory = decodeMemory(dsl["memory"])
	if graph, ok := dsl["graph"].(map[string]any); ok {
		if nodes, ok := graph["nodes"].([]any); ok {
			for _, raw := range nodes {
				node, ok := raw.(map[string]any)
				if !ok || node == nil {
					continue
				}
				id, _ := node["id"].(string)
				parentID, _ := node["parentId"].(string)
				if id != "" && parentID != "" {
					c.NodeParents[id] = parentID
				}
			}
		}
	}
	for cpnID, raw := range rawComps {
		comp, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, params, downstream, upstream := decodeComponentFields(comp)
		if name == "" {
			return nil, fmt.Errorf("canvas: component %q has empty component_name", cpnID)
		}
		c.Components[cpnID] = CanvasComponent{
			Obj: CanvasComponentObj{
				ComponentName: name,
				Params:        params,
			},
			Downstream: downstream,
			Upstream:   upstream,
		}
	}
	if len(c.Components) == 0 {
		return nil, fmt.Errorf("canvas: no components")
	}
	return c, nil
}

func decodeHistory(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return []map[string]any{}
	}
	history := make([]map[string]any, 0, len(items))
	for _, item := range items {
		var role string
		var payload any
		switch value := item.(type) {
		case []any: // Persisted Canvas shape: [role, payload].
			if len(value) < 2 {
				continue
			}
			role, _ = value[0].(string)
			payload = value[1]
		case map[string]any: // Runtime shape: {role, content/payload}.
			role, _ = value["role"].(string)
			if preserved, exists := value["payload"]; exists {
				payload = preserved
			} else {
				payload = value["content"]
			}
		default:
			continue
		}
		if role == "" {
			continue
		}
		history = append(history, map[string]any{
			"role":    role,
			"content": decodedHistoryContent(payload),
			"payload": payload,
		})
	}
	return history
}

// decodeMemory restores [user, assistant, tool summary] entries.
func decodeMemory(raw any) []map[string]any {
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return []map[string]any{}
	}
	memory := make([]map[string]any, 0, len(items))
	for _, item := range items {
		switch value := item.(type) {
		case []any: // [[user_query, assistant_response, tool_summary], ...]
			if len(value) < 3 {
				continue
			}
			memory = append(memory, map[string]any{
				"user":      value[0],
				"assistant": value[1],
				"summary":   value[2],
			})
		case map[string]any:
			memory = append(memory, map[string]any{
				"user":      value["user"],
				"assistant": value["assistant"],
				"summary":   value["summary"],
			})
		}
	}
	return memory
}

func decodedHistoryContent(payload any) string {
	switch value := payload.(type) {
	case nil:
		return ""
	case string:
		return value
	case map[string]any:
		content, _ := value["content"].(string)
		return content
	default:
		return fmt.Sprint(value)
	}
}

// EncodeHistory converts runtime conversation entries back to the Python DSL
// list-of-pairs shape: [[role, payload], ...].
func EncodeHistory(history []map[string]any) []any {
	out := make([]any, 0, len(history))
	for _, entry := range history {
		role, _ := entry["role"].(string)
		if role == "" {
			continue
		}
		payload, exists := entry["payload"]
		if !exists {
			payload = entry["content"]
		}
		out = append(out, []any{role, payload})
	}
	return out
}

// EncodeMemory converts runtime tool-call memory back to Python's
// [[user, assistant, summary], ...] DSL shape.
func EncodeMemory(memory []map[string]any) []any {
	out := make([]any, 0, len(memory))
	for _, entry := range memory {
		out = append(out, []any{entry["user"], entry["assistant"], entry["summary"]})
	}
	return out
}

func decodeComponentFields(comp map[string]any) (name string, params map[string]any, downstream []string, upstream []string) {
	if obj, ok := comp["obj"].(map[string]any); ok {
		name, _ = obj["component_name"].(string)
		if p, ok := obj["params"].(map[string]any); ok {
			params = p
		}
		if ds, ok := obj["downstream"].([]any); ok {
			downstream = decodeStringSlice(ds)
		} else if ds, ok := obj["downstream"].([]string); ok {
			downstream = ds
		}
	}
	if name == "" {
		name, _ = comp["name"].(string)
	}
	if params == nil {
		if p, ok := comp["params"].(map[string]any); ok {
			params = p
		}
	}
	if len(downstream) == 0 {
		if ds, ok := comp["downstream"].([]any); ok {
			downstream = decodeStringSlice(ds)
		} else if ds, ok := comp["downstream"].([]string); ok {
			downstream = ds
		}
	}
	if us, ok := comp["upstream"].([]any); ok {
		upstream = decodeStringSlice(us)
	} else if us, ok := comp["upstream"].([]string); ok {
		upstream = us
	}
	return
}

func decodeStringSlice(in []any) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
