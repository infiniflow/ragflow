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

// Package dsl contains pure-function helpers for working with the agent
// canvas DSL map structure (`map[string]any`). It is intentionally
// runtime-free: no Canvas instantiation, no component factories, no
// database access. The agent_component handlers use it to introspect
// the DSL before deciding whether to wire up a runtime component.
package dsl

import "fmt"

// ExtractComponentInputForm returns the input-form schema dict stored at
// `dsl["components"][componentID]["obj"]["input_form"]`.
//
// This is the Go equivalent of the python
// `Canvas.get_component_input_form(component_id)` method
// (api/agent/canvas.py:163) which reads the same path. The python
// version walks the live Canvas object; we walk the raw DSL map
// directly because the Go Canvas type does not expose an
// introspection API (see plan §Gap C — there is no `GetComponent` on
// the runtime Canvas type).
//
// Returns:
//   - the form-schema dict if present and well-typed
//   - ErrComponentNotFound if the componentID is missing from dsl
//   - ErrMissingInputForm if the component exists but has no input_form
//   - ErrMalformedDSL if the field is present but the wrong type
//
// Type errors (input_form is e.g. a list or a string) are NOT
// collapsed into ErrMissingInputForm — they would mask a contract
// violation in the DSL and let DebugComponent run against corrupt
// data. CodeRabbit PR review #1 on PR #16403.
func ExtractComponentInputForm(dsl map[string]any, componentID string) (map[string]any, error) {
	comp, err := navigateToComponent(dsl, componentID)
	if err != nil {
		return nil, err
	}
	obj, ok := comp["obj"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: component %q has no obj", ErrMalformedDSL, componentID)
	}
	rawForm, exists := obj["input_form"]
	if !exists || rawForm == nil {
		return nil, fmt.Errorf("%w: component %q has no input_form", ErrMissingInputForm, componentID)
	}
	form, ok := rawForm.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: component %q input_form is not a dict", ErrMalformedDSL, componentID)
	}
	return form, nil
}

// ExtractComponentParams returns the params map stored at
// `dsl["components"][componentID]["obj"]["params"]`. The debug handler
// uses this to build the inputs map for the runtime Component.Invoke
// call. Type errors collapse to ErrMalformedDSL (CodeRabbit PR
// review #1).
func ExtractComponentParams(dsl map[string]any, componentID string) (map[string]any, error) {
	comp, err := navigateToComponent(dsl, componentID)
	if err != nil {
		return nil, err
	}
	obj, ok := comp["obj"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: component %q has no obj", ErrMalformedDSL, componentID)
	}
	rawParams, exists := obj["params"]
	if !exists || rawParams == nil {
		return nil, nil
	}
	params, ok := rawParams.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: component %q params is not a dict", ErrMalformedDSL, componentID)
	}
	return params, nil
}

// ExtractComponentName returns the component's class name (e.g.
// "Begin", "LLM", "Retrieval") from `dsl["components"][componentID].
// ["obj"]["component_name"]`. The runtime factory is keyed on this
// name.
func ExtractComponentName(dsl map[string]any, componentID string) (string, error) {
	comp, err := navigateToComponent(dsl, componentID)
	if err != nil {
		return "", err
	}
	obj, ok := comp["obj"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("%w: component %q has no obj", ErrMalformedDSL, componentID)
	}
	name, _ := obj["component_name"].(string)
	if name == "" {
		return "", fmt.Errorf("%w: component %q has no component_name", ErrMalformedDSL, componentID)
	}
	return name, nil
}

// navigateToComponent walks dsl["components"][componentID] and
// returns the inner dict. Centralised so the three extractors above
// share a single traversal path. (Renamed from extractComponent to
// avoid colliding with the same-named helper in normalize.go.)
func navigateToComponent(dsl map[string]any, componentID string) (map[string]any, error) {
	if dsl == nil {
		return nil, fmt.Errorf("%w: nil dsl", ErrMalformedDSL)
	}
	comps, ok := dsl["components"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: missing components map", ErrMalformedDSL)
	}
	comp, ok := comps[componentID]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrComponentNotFound, componentID)
	}
	cm, ok := comp.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q is not a dict", ErrMalformedDSL, componentID)
	}
	return cm, nil
}

// FindBeginComponentID returns the component_id of the canvas component
// whose obj.component_name == "Begin". Returns ErrComponentNotFound if
// no such component exists. Mirrors python Canvas.begin_component_id
// (api/agent/canvas.py:180).
//
// `Begin` is a component NAME (stored at obj.component_name), not a
// component ID. The two are related but not identical; a canvas can
// have a component named "Begin" whose ID is e.g. "sally:0". Callers
// that need to read fields off the begin component must use this
// helper to resolve the name to the ID, then pass the ID to
// navigateToComponent (or any of the ExtractComponent* helpers).
func FindBeginComponentID(dsl map[string]any) (string, error) {
	if dsl == nil {
		return "", fmt.Errorf("%w: nil dsl", ErrMalformedDSL)
	}
	comps, ok := dsl["components"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("%w: missing components map", ErrMalformedDSL)
	}
	for id, raw := range comps {
		cm, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		obj, _ := cm["obj"].(map[string]any)
		name, _ := obj["component_name"].(string)
		if name == "Begin" {
			return id, nil
		}
	}
	return "", fmt.Errorf("%w: Begin component", ErrComponentNotFound)
}

// ExtractPrologue mirrors python Canvas.get_prologue
// (api/agent/canvas.py:190) — returns the "prologue" string stored at
// dsl["components"][<begin_id>]["obj"]["prologue"]. Reuses the
// shared navigateToComponent helper so the addressing rule is
// consistent with ExtractComponentInputForm.
func ExtractPrologue(dsl map[string]any) (string, error) {
	id, err := FindBeginComponentID(dsl)
	if err != nil {
		return "", err
	}
	comp, err := navigateToComponent(dsl, id)
	if err != nil {
		return "", err
	}
	obj, _ := comp["obj"].(map[string]any)
	s, _ := obj["prologue"].(string)
	return s, nil
}

// ExtractMode mirrors python Canvas.get_mode (api/agent/canvas.py:200).
// Returns the canvas mode (e.g. "Agent" / "DataFlow") stored at
// dsl["components"][<begin_id>]["obj"]["mode"].
func ExtractMode(dsl map[string]any) (string, error) {
	id, err := FindBeginComponentID(dsl)
	if err != nil {
		return "", err
	}
	comp, err := navigateToComponent(dsl, id)
	if err != nil {
		return "", err
	}
	obj, _ := comp["obj"].(map[string]any)
	s, _ := obj["mode"].(string)
	return s, nil
}
