// Package canvas — param-level reference extraction (issues #19325 and follow-ups).
//
// The Python side closes this gap with `ComponentBase.param_refs()` plus
// `ComponentBase.get_dependency_ids()`; the merged upstream + param-ref
// id list is fed into the Python scheduler as additional exec-only edges
// so a node that reads another node's output via a param (for example
// `{{other@content}}` written into VariableAggregator's
// `groups[i].variables[j].value`) still waits for that source to finish
// even when the DSL draws no edge between them. The Go scheduler
// currently only walks the drawn edge list, so the same shape on the
// Go side has a hidden race.
//
// This file holds the Go-side helper. It mirrors the Python `param_refs()`
// override for the components that already extract variable references
// from their own params. The Go `Component` interface is not changed
// because the scheduler is the only caller and the raw param map is
// already on hand at the edge-wiring site (Pass 2 of BuildWorkflow).
// New components that need param-ref extraction add a case below; the
// scheduler only sees one helper.

package canvas

import "strings"

// paramRefDependencies returns the component ids this component reads
// from its own params, in the form the upstream DSL uses (e.g. "other"
// from a `{{other@content}}` selector). Empty when the component type
// has no param-refs or its params do not carry any.
//
// Component-specific rules:
//
//   - VariableAggregator: walks `params["groups"][i]["variables"][j]`
//     and collects each selector's value. Each entry may be either a
//     plain string (e.g. "other@content") or a `{"value": "..."}`
//     dict; the Python implementation accepts both, so we do too.
//   - Switch: walks `params["conditions"][i]["clauses"][j]` and
//     collects each `left` and `right` string selector. The
//     `evaluateClause` path resolves both sides through
//     `runtime.ResolveTemplate`, so a ref on either side is a real
//     dependency the drawn-edge list does not capture.
//   - Categorize: walks `params["query"]` (a string the runtime
//     resolves through `state.GetVar`) and `params["items"]` (a
//     list of strings the Invoke path iterates and adds to the
//     prompt as context). A ref on either is the same shape as
//     VariableAggregator / Switch.
//
// Other component types in the Go port do not have a Go-side
// implementation of param_refs() yet (CodeExec is not implemented on
// the Go side, and the remaining Python components read refs out of
// runtime inputs, not static params). They return an empty list here,
// which means their existing drawn edges continue to carry the
// dependency; the param-ref path is closed only for the components
// that read refs out of params.
func paramRefDependencies(name string, params map[string]any) []string {
	if params == nil {
		return nil
	}
	switch {
	case isVariableAggregator(name):
		return variableAggregatorParamRefs(params)
	case isSwitch(name):
		return switchParamRefs(params)
	case isCategorize(name):
		return categorizeParamRefs(params)
	}
	return nil
}

// isVariableAggregator matches the component name case-insensitively
// to match the registry's normalisation. The Python DSL uses
// "VariableAggregator"; the Go side registers the same canonical name.
func isVariableAggregator(name string) bool {
	return strings.EqualFold(name, "VariableAggregator")
}

// isSwitch mirrors isVariableAggregator for the Switch router node.
func isSwitch(name string) bool {
	return strings.EqualFold(name, "Switch")
}

// isCategorize mirrors isVariableAggregator for the Categorize
// classifier node. Both walk the same `{{cpnID@field}}` selector
// syntax and resolve through the canvas state, so a single
// parser covers all three.
func isCategorize(name string) bool {
	return strings.EqualFold(name, "Categorize")
}

// variableAggregatorParamRefs walks the same `params["groups"][i].variables[j]`
// shape the Python override walks, and returns the list of ref strings.
// Defensive against both the `[]any` and `[]map[string]any` JSON
// shapes the Go runtime emits (the engine decodes JSON into the first,
// hand-built params into the second; see `variableAggregatorParam.Update`).
func variableAggregatorParamRefs(params map[string]any) []string {
	rawGroups, ok := params["groups"]
	if !ok {
		return nil
	}
	groupsList, ok := rawGroups.([]any)
	if !ok {
		// Fall back to the typed slice; iterate without materialising.
		typed, ok2 := rawGroups.([]map[string]any)
		if !ok2 {
			return nil
		}
		groupsList = make([]any, 0, len(typed))
		for _, g := range typed {
			groupsList = append(groupsList, g)
		}
	}
	var refs []string
	for _, raw := range groupsList {
		gmap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		vars, ok := gmap["variables"]
		if !ok {
			continue
		}
		refs = append(refs, stringOrDictListParamRefs(vars)...)
	}
	return refs
}

// switchParamRefs walks `params["conditions"][i]["clauses"][j]` and
// returns every string selector on `left` or `right`. Switch's
// `evaluateClause` resolves both sides through `runtime.ResolveTemplate`,
// so a ref on either side is a real dependency the drawn-edge list
// does not capture. The Python fix (PR #19282) flagged this as
// future work; the Go scheduler needs it now that VariableAggregator
// is wired (cycle 68).
func switchParamRefs(params map[string]any) []string {
	raw, ok := params["conditions"]
	if !ok {
		return nil
	}
	groupsList, ok := raw.([]any)
	if !ok {
		typed, ok2 := raw.([]map[string]any)
		if !ok2 {
			return nil
		}
		groupsList = make([]any, 0, len(typed))
		for _, g := range typed {
			groupsList = append(groupsList, g)
		}
	}
	var refs []string
	for _, raw := range groupsList {
		gmap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		clauses, ok := gmap["clauses"]
		if !ok {
			continue
		}
		clauseList, ok := clauses.([]any)
		if !ok {
			if typed, ok2 := clauses.([]map[string]any); ok2 {
				clauseList = make([]any, 0, len(typed))
				for _, c := range typed {
					clauseList = append(clauseList, c)
				}
			} else {
				continue
			}
		}
		for _, raw := range clauseList {
			cmap, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if v, ok := cmap["left"].(string); ok && v != "" {
				refs = append(refs, v)
			}
			// `right` is allowed to be any JSON value: a string
			// ref, a number, a boolean, a `{"value": "..."}` dict
			// from a custom operator, etc. Only the string form
			// is a candidate param-ref; parseParamRefComponent
			// will return "" for non-ref strings.
			if v, ok := cmap["right"].(string); ok && v != "" {
				refs = append(refs, v)
			}
		}
	}
	return refs
}

// categorizeParamRefs walks `params["query"]` (a string) and
// `params["items"]` (a list of strings). `resolveCategorizeQuery`
// resolves the query through `state.GetVar`; the Invoke path
// iterates items and adds them to the prompt as context. A ref on
// either is the same shape as VariableAggregator / Switch.
func categorizeParamRefs(params map[string]any) []string {
	var refs []string
	if v, ok := params["query"].(string); ok && v != "" {
		refs = append(refs, v)
	}
	if items, ok := params["items"]; ok {
		refs = append(refs, stringListParamRefs(items)...)
	}
	return refs
}

// stringListParamRefs walks a `[]any` or `[]string` of potential
// ref selectors. The Categorize `items` field is the primary caller.
func stringListParamRefs(items any) []string {
	list, ok := items.([]any)
	if !ok {
		if typed, ok2 := items.([]string); ok2 {
			refs := make([]string, 0, len(typed))
			for _, s := range typed {
				if s != "" {
					refs = append(refs, s)
				}
			}
			return refs
		}
		return nil
	}
	var refs []string
	for _, raw := range list {
		if s, ok := raw.(string); ok && s != "" {
			refs = append(refs, s)
		}
	}
	return refs
}

// stringOrDictListParamRefs handles a list of either a `{"value": "<ref>"}`
// dict or a plain string. The Python override accepts both forms (the
// SDK sometimes drops the dict wrapper), so we mirror that here. Used
// by VariableAggregator's `groups[i].variables[j]` list.
func stringOrDictListParamRefs(vars any) []string {
	list, ok := vars.([]any)
	if !ok {
		if typed, ok2 := vars.([]map[string]any); ok2 {
			list = make([]any, 0, len(typed))
			for _, v := range typed {
				list = append(list, v)
			}
		} else {
			return nil
		}
	}
	var refs []string
	for _, raw := range list {
		switch sel := raw.(type) {
		case map[string]any:
			if v, ok := sel["value"].(string); ok && v != "" {
				refs = append(refs, v)
			}
		case string:
			if sel != "" {
				refs = append(refs, sel)
			}
		}
	}
	return refs
}

// parseParamRefComponent returns the upstream id embedded in a
// `cpnID@field` selector, or "" when the selector has no `@` and is
// therefore not a param-ref. Callers use the empty result to skip
// non-ref entries (a bare "field" string, for example, is a local
// template variable, not a cross-component dependency).
func parseParamRefComponent(ref string) string {
	at := strings.Index(ref, "@")
	if at <= 0 {
		return ""
	}
	return ref[:at]
}
