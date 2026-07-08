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
	nodeForms := decodeGraphNodeForms(dsl)
	for cpnID, raw := range rawComps {
		comp, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, params, downstream, upstream := decodeComponentFields(comp)
		params = mergeNodeFormParams(params, nodeForms[cpnID])
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

func decodeGraphNodeForms(dsl map[string]any) map[string]map[string]any {
	graph, ok := dsl["graph"].(map[string]any)
	if !ok {
		return nil
	}
	nodes, ok := graph["nodes"].([]any)
	if !ok || len(nodes) == 0 {
		return nil
	}
	forms := make(map[string]map[string]any)
	for _, raw := range nodes {
		node, ok := raw.(map[string]any)
		if !ok || node == nil {
			continue
		}
		id, _ := node["id"].(string)
		if id == "" {
			continue
		}
		data, _ := node["data"].(map[string]any)
		form, _ := data["form"].(map[string]any)
		if len(form) == 0 {
			continue
		}
		forms[id] = form
	}
	return forms
}

func mergeNodeFormParams(params, form map[string]any) map[string]any {
	if len(form) == 0 {
		return params
	}
	merged := make(map[string]any, len(params)+1)
	for k, v := range params {
		merged[k] = v
	}
	if shouldFillParamFromForm(merged["api_key"], form["api_key"]) {
		merged["api_key"] = form["api_key"]
	}
	return merged
}

func shouldFillParamFromForm(current, form any) bool {
	if form == nil {
		return false
	}
	if current == nil {
		return true
	}
	if s, ok := current.(string); ok && s == "" {
		if fs, ok := form.(string); ok {
			return fs != ""
		}
		return true
	}
	return false
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
