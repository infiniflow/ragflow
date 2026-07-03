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

import "ragflow/internal/agent/canvas"

func decodeCanvasFromDSL(dsl map[string]any) (*canvas.Canvas, error) {
	if len(dsl) == 0 {
		return nil, errNilDSL
	}
	rawComps, ok := dsl["components"].(map[string]any)
	if !ok || len(rawComps) == 0 {
		return nil, errEmptyStages
	}
	c := &canvas.Canvas{
		Components:  make(map[string]canvas.CanvasComponent, len(rawComps)),
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
	for cpnID, raw := range rawComps {
		comp, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, params, downstream, upstream := extractComponentFields(comp)
		if name == "" {
			return nil, errUnknownComponent
		}
		c.Components[cpnID] = canvas.CanvasComponent{
			Obj: canvas.CanvasComponentObj{
				ComponentName: name,
				Params:        params,
			},
			Downstream: downstream,
			Upstream:   upstream,
		}
	}
	if len(c.Components) == 0 {
		return nil, errEmptyStages
	}
	return c, nil
}

func extractComponentFields(comp map[string]any) (name string, params map[string]any, downstream []string, upstream []string) {
	if obj, ok := comp["obj"].(map[string]any); ok {
		name, _ = obj["component_name"].(string)
		if p, ok := obj["params"].(map[string]any); ok {
			params = p
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
	if ds, ok := comp["downstream"].([]any); ok {
		downstream = toStringSlice(ds)
	} else if ds, ok := comp["downstream"].([]string); ok {
		downstream = ds
	}
	if us, ok := comp["upstream"].([]any); ok {
		upstream = toStringSlice(us)
	} else if us, ok := comp["upstream"].([]string); ok {
		upstream = us
	}
	return
}

func toStringSlice(in []any) []string {
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
