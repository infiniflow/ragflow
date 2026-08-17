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
		return nil, fmt.Errorf("run output missing terminal payload %q", terminalIDs[0])
	}
	return payload, nil
}

// TerminalComponentIDs walks a raw DSL JSON and returns the sorted ids of
// components that have no downstream connections (terminals / sinks).
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
	terminals := make([]string, 0, len(components))
	for id, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("component %q has invalid type %T", id, rawComp)
		}
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
