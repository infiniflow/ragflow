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

package service

import (
	"fmt"

	"ragflow/internal/agent/canvas"
)

// decodeCanvasFromDSL converts the DSL map (in either IMPORT shape
// or NormalizeForCanvas output shape) into a *canvas.Canvas that
// canvas.Compile accepts.
//
// Two accepted input shapes:
//
//  1. IMPORT shape (top-level "obj.component_name" + "obj.params"
//     + outer "downstream" / "upstream"). The Python-era DSL
//     convention; some legacy v1 fixtures still use it directly.
//
//  2. Normalized shape (top-level "name" + "params" + "downstream" /
//     "upstream"). The output of dslpkg.NormalizeForCanvas, which
//     is what service.normalisedDSLForRun currently feeds into
//     buildRunFunc (gap analysis §11.7.4 V2 follow-up chain).
//
// The canvas.Canvas struct itself uses IMPORT shape
// (CanvasComponentObj.ComponentName with json tag
// "component_name"). normalize.go's buildGraphFromComponents
// flattens the components map to the normalized shape so the
// React-Flow editor gets a stable byte-equal layout; the runtime
// then has to walk both shapes here.
//
// All non-sentinel failures wrap ErrAgentStorageError so the
// handler's mapAgentError classifies them as CodeServerError
// (500) with a sanitized message — the raw decoder error string
// never reaches the client.
//
// Decoder strategy: direct map walking, NOT JSON round-trip. The
// Phase 4.4 V2 plan §4.3 originally specified JSON round-trip
// for the IMPORT shape, but the production path goes through
// NormalizeForCanvas first (normalized shape), and round-tripping
// the normalized shape through JSON loses the `name` →
// `obj.component_name` mapping (json.Unmarshal into Canvas does
// not coerce flat keys into nested `obj`). Direct map walking
// handles both shapes without that hazard.
func decodeCanvasFromDSL(dsl map[string]any) (*canvas.Canvas, error) {
	if len(dsl) == 0 {
		return nil, fmt.Errorf("decode canvas: empty DSL: %w", ErrAgentStorageError)
	}
	rawComps, ok := dsl["components"].(map[string]any)
	if !ok || len(rawComps) == 0 {
		return nil, fmt.Errorf("decode canvas: no components: %w", ErrAgentStorageError)
	}
	c := &canvas.Canvas{
		Components: make(map[string]canvas.CanvasComponent, len(rawComps)),
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
	for cpnID, raw := range rawComps {
		comp, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, params, downstream, upstream := extractComponentFields(comp)
		if name == "" {
			return nil, fmt.Errorf("decode canvas: component %q has empty component_name: %w", cpnID, ErrAgentStorageError)
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
		return nil, fmt.Errorf("decode canvas: no components: %w", ErrAgentStorageError)
	}
	return c, nil
}

// extractComponentFields reads (name, params, downstream, upstream)
// from a single component map. Accepts both IMPORT shape
// (obj.component_name / obj.params / outer downstream) and
// normalized shape (flat name / flat params / flat downstream).
//
// IMPORT shape preference order: obj.* (canonical), then flat
// fallback (for normalized input). This matches the way
// dsl.extractComponent (normalize.go:269) walks both, ensuring the
// V2 decoder agrees with the normalizer on the field-resolution
// order for shared key names.
func extractComponentFields(comp map[string]any) (name string, params map[string]any, downstream []string, upstream []string) {
	if obj, ok := comp["obj"].(map[string]any); ok {
		name, _ = obj["component_name"].(string)
		if p, ok := obj["params"].(map[string]any); ok {
			params = p
		}
		if ds, ok := obj["downstream"].([]any); ok {
			downstream = toStringSlice(ds)
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
			downstream = toStringSlice(ds)
		} else if ds, ok := comp["downstream"].([]string); ok {
			downstream = ds
		}
	}
	if us, ok := comp["upstream"].([]any); ok {
		upstream = toStringSlice(us)
	} else if us, ok := comp["upstream"].([]string); ok {
		upstream = us
	}
	return
}

// toStringSlice normalises a []any of strings to []string. Empty
// for nil input.
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
