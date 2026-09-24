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

package pipeline

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ExtractPayload extracts the terminal component's output from a pipeline run
// result. When the output carries an "output_format" key the whole map is
// returned as-is; otherwise the payload is looked up from the run output keyed
// by the DSL's single terminal component id.
func ExtractPayload(dsl string, out map[string]any) (map[string]any, error) {
	if out == nil {
		return nil, nil
	}
	if _, ok := out["output_format"]; ok {
		return out, nil
	}
	terminalIDs, err := TerminalComponentIDs([]byte(dsl))
	if err != nil {
		return nil, err
	}
	if len(terminalIDs) != 1 {
		return nil, fmt.Errorf("pipeline requires exactly 1 terminal, got %d: %v", len(terminalIDs), terminalIDs)
	}
	payload, ok := out[terminalIDs[0]].(map[string]any)
	if !ok {
		if state, stateOK := out["state"].(map[string]any); stateOK {
			payload, ok = state[terminalIDs[0]].(map[string]any)
		} else if state, stateOK := out["state"].(map[string]map[string]any); stateOK {
			payload, ok = state[terminalIDs[0]]
		}
	}
	if !ok {
		return nil, fmt.Errorf("run output missing terminal payload %q", terminalIDs[0])
	}
	return payload, nil
}

// TerminalComponentIDs walks the execution graph from its entry component(s)
// and returns the sorted ids of reachable components that have no downstream
// connections (terminals / sinks). Disconnected canvas components are not part
// of a run and therefore must not affect terminal selection.
func TerminalComponentIDs(raw []byte) ([]string, error) {
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		return nil, fmt.Errorf("unmarshal canvas dsl: %w", err)
	}
	root := tpl
	if nested, ok := tpl["dsl"].(map[string]any); ok {
		root = nested
	}
	components, ok := root["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("canvas dsl missing components map")
	}
	roots := make([]string, 0, 2)
	for id, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("component %q has invalid type %T", id, rawComp)
		}
		if componentName(comp) == "file" || componentName(comp) == "begin" {
			roots = append(roots, id)
		}
	}
	// Minimal/raw DSLs used by older callers may omit component metadata. Keep
	// their historical sink scan rather than inventing an entry point.
	if len(roots) == 0 {
		roots = make([]string, 0, len(components))
		for id := range components {
			roots = append(roots, id)
		}
	}

	reachable := make(map[string]bool, len(components))
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if reachable[id] {
			continue
		}
		if _, ok := components[id]; !ok {
			continue
		}
		reachable[id] = true
		comp := components[id].(map[string]any)
		if downstream, ok := comp["downstream"].([]any); ok {
			for _, rawChild := range downstream {
				if child, ok := rawChild.(string); ok {
					queue = append(queue, child)
				}
			}
		}
	}

	terminals := make([]string, 0, len(reachable))
	for id := range reachable {
		comp := components[id].(map[string]any)
		switch downstream := comp["downstream"].(type) {
		case nil:
			terminals = append(terminals, id)
		case []any:
			if len(downstream) == 0 {
				terminals = append(terminals, id)
			}
		default:
			// Non-slice downstream means the component is connected; ignore it here.
		}
	}
	sort.Strings(terminals)
	return terminals, nil
}

func componentName(comp map[string]any) string {
	if obj, ok := comp["obj"].(map[string]any); ok {
		if name, ok := obj["component_name"].(string); ok {
			return strings.ToLower(name)
		}
	}
	if name, ok := comp["name"].(string); ok {
		return strings.ToLower(name)
	}
	return ""
}
