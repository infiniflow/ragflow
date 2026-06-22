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

import (
	"context"
	"encoding/json"
	"fmt"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/agent/workflowx"

	"github.com/cloudwego/eino/compose"
)

const (
	parallelItemInputNodeKey   = "__parallel_item_input__"
	parallelItemCollectNodeKey = "__parallel_item_collect__"
)

type parallelExpansion struct {
	Graph          *compose.Workflow[map[string]any, map[string]any]
	Sub            *compose.Workflow[map[string]any, map[string]any]
	Members        map[string]bool
	ItemsRef       string
	MaxConcurrency int
	OutputRefs     map[string]string
}

func buildParallelExpansion(ctx context.Context, c *Canvas, parallelID string) (*parallelExpansion, error) {
	if c == nil {
		return nil, fmt.Errorf("canvas: nil canvas")
	}
	if parallelID == "" {
		return nil, fmt.Errorf("canvas: buildParallelExpansion: empty parallelID")
	}
	comp, ok := c.Components[parallelID]
	if !ok {
		return nil, fmt.Errorf("canvas: buildParallelExpansion: unknown cpn %q", parallelID)
	}

	members := collectGroupedMembers(c, parallelID)
	if len(members) == 0 {
		members = collectDescendants(c, parallelID)
	}

	itemsRef, maxConcurrency, outputRefs, err := readParallelParams(comp.Obj.Params)
	if err != nil {
		return nil, fmt.Errorf("canvas: parallel %q: %w", parallelID, err)
	}

	sub, err := buildParallelItemWorkflow(ctx, c, parallelID, members)
	if err != nil {
		return nil, fmt.Errorf("canvas: parallel %q: %w", parallelID, err)
	}

	graph, err := buildParallelOuterWorkflow(ctx, parallelID, itemsRef, maxConcurrency, outputRefs, sub)
	if err != nil {
		return nil, fmt.Errorf("canvas: parallel %q: %w", parallelID, err)
	}

	return &parallelExpansion{
		Graph:          graph,
		Sub:            sub,
		Members:        members,
		ItemsRef:       itemsRef,
		MaxConcurrency: maxConcurrency,
		OutputRefs:     outputRefs,
	}, nil
}

func collectGroupedMembers(c *Canvas, parentID string) map[string]bool {
	out := make(map[string]bool)
	if c == nil || len(c.NodeParents) == 0 || parentID == "" {
		return out
	}
	for childID, groupID := range c.NodeParents {
		if groupID == parentID && c.Components[childID].Obj.ComponentName != "" {
			out[childID] = true
		}
	}
	return out
}

func readParallelParams(params map[string]any) (itemsRef string, maxConcurrency int, outputRefs map[string]string, err error) {
	if params == nil {
		return "", 0, nil, fmt.Errorf("missing params")
	}
	itemsRef, _ = params["items_ref"].(string)
	if itemsRef == "" {
		return "", 0, nil, fmt.Errorf("missing items_ref")
	}
	switch v := params["max_concurrency"].(type) {
	case int:
		maxConcurrency = v
	case int64:
		maxConcurrency = int(v)
	case float64:
		maxConcurrency = int(v)
	}

	rawOutputs, _ := params["outputs"].(map[string]any)
	outputRefs = make(map[string]string, len(rawOutputs))
	for name, raw := range rawOutputs {
		spec, _ := raw.(map[string]any)
		if spec == nil {
			continue
		}
		ref, _ := spec["ref"].(string)
		if ref != "" {
			outputRefs[name] = ref
		}
	}
	return itemsRef, maxConcurrency, outputRefs, nil
}

func buildParallelItemWorkflow(
	ctx context.Context,
	c *Canvas,
	parallelID string,
	members map[string]bool,
) (*compose.Workflow[map[string]any, map[string]any], error) {
	sub, err := buildSubWorkflow(ctx, c, members, parallelID, nil)
	if err != nil {
		return nil, err
	}

	wrapper := compose.NewWorkflow[map[string]any, map[string]any]()

	inNode := wrapper.AddLambdaNode(
		parallelItemInputNodeKey,
		compose.InvokableLambda(func(_ context.Context, in map[string]any) (map[string]any, error) {
			if in == nil {
				return map[string]any{}, nil
			}
			return in, nil
		}),
		compose.WithNodeName(parallelItemInputNodeKey),
	)
	inNode.AddInput(compose.START)

	bodyNode := wrapper.AddGraphNode(parallelID+"__parallel_body__", sub)
	bodyNode.AddInput(parallelItemInputNodeKey)

	collector := wrapper.AddLambdaNode(
		parallelItemCollectNodeKey,
		compose.InvokableLambda(func(ctx context.Context, in map[string]any) (map[string]any, error) {
			localState, _, err := GetStateFromContext[*CanvasState](ctx)
			if err != nil || localState == nil {
				return nil, fmt.Errorf("canvas: parallel %q item collector: no canvas state in context", parallelID)
			}
			out := map[string]any{
				"item":  localState.Globals["__item__"],
				"index": localState.Globals["__index__"],
			}
			for cpnID, bucket := range localState.Snapshot() {
				cp := make(map[string]any, len(bucket))
				for k, v := range bucket {
					cp[k] = v
				}
				out[cpnID] = cp
			}
			return out, nil
		}),
		compose.WithNodeName(parallelItemCollectNodeKey),
	)
	collector.AddInput(parallelID + "__parallel_body__")
	wrapper.End().AddInput(parallelItemCollectNodeKey)

	return wrapper, nil
}

func buildParallelOuterWorkflow(
	ctx context.Context,
	key string,
	itemsRef string,
	maxConcurrency int,
	outputRefs map[string]string,
	sub *compose.Workflow[map[string]any, map[string]any],
) (*compose.Workflow[map[string]any, map[string]any], error) {
	outer := compose.NewWorkflow[map[string]any, map[string]any]()

	batchInputKey := key + "__parallel_batch_input__"
	batchGraphKey := key + "__parallel_batch_graph__"
	collectKey := key + "__parallel_collect__"

	toBatch := outer.AddLambdaNode(
		batchInputKey,
		compose.InvokableLambda(func(ctx context.Context, _ map[string]any) ([]map[string]any, error) {
			state, _, err := GetStateFromContext[*CanvasState](ctx)
			if err != nil || state == nil {
				return nil, fmt.Errorf("canvas: parallel %q: no canvas state in context", key)
			}
			raw, err := state.GetVar(itemsRef)
			if err != nil {
				return nil, fmt.Errorf("canvas: parallel %q items_ref %q: %w", key, itemsRef, err)
			}
			return toParallelItems(raw)
		}),
		compose.WithNodeName(batchInputKey),
	)
	toBatch.AddInput(compose.START)

	batchWF := compose.NewWorkflow[[]map[string]any, []map[string]any]()
	var parOpts []workflowx.ParallelOption
	if maxConcurrency > 0 {
		parOpts = append(parOpts, workflowx.WithParallelMaxConcurrency(maxConcurrency))
	}
	parOpts = append(parOpts, workflowx.WithParallelContextBuilder(func(
		ctx context.Context, item any, index int,
	) context.Context {
		parentState, _, err := runtime.GetStateFromContext[*runtime.CanvasState](ctx)
		if err != nil || parentState == nil {
			return ctx
		}
		itemMap, _ := item.(map[string]any)
		// cloneCanvasState can fail (e.g. unsupported value in a Sys/Env
		// field that the JSON round-trip rejects). When it does, fall
		// back to a fresh per-item state so concurrent workers never
		// share the same CanvasState — sharing would corrupt Globals
		// and Outputs across items.
		localState, cloneErr := cloneCanvasState(parentState)
		if cloneErr != nil || localState == nil {
			localState = runtime.NewCanvasState(parentState.RunID, parentState.TaskID)
			localState.Sys = shallowCopyAnyMap(parentState.Sys)
			localState.Globals = shallowCopyAnyMap(parentState.Globals)
		}
		localState.Globals["__item__"] = itemMap["item"]
		localState.Globals["__index__"] = index
		return runtime.WithState(ctx, localState)
	}))

	parNode, err := workflowx.AddParallelNode[map[string]any, map[string]any](
		ctx, batchWF, key+"__parallel_batch__", sub, parOpts...,
	)
	if err != nil {
		return nil, err
	}
	parNode.AddInput(compose.START)
	batchWF.End().AddInput(key + "__parallel_batch__")

	batchNode := outer.AddGraphNode(batchGraphKey, batchWF)
	batchNode.AddInput(batchInputKey)

	collector := outer.AddLambdaNode(
		collectKey,
		compose.InvokableLambda(func(_ context.Context, outputs []map[string]any) (map[string]any, error) {
			out := map[string]any{
				"__cpn_id__": key,
				"_result":    outputs,
			}
			for outputName, ref := range outputRefs {
				values := make([]any, 0, len(outputs))
				for _, itemOut := range outputs {
					values = append(values, resolveParallelItemRef(itemOut, ref))
				}
				out[outputName] = values
			}
			return out, nil
		}),
		compose.WithNodeName(collectKey),
	)
	collector.AddInput(batchGraphKey)
	outer.End().AddInput(collectKey)

	return outer, nil
}

func toParallelItems(raw any) ([]map[string]any, error) {
	switch items := raw.(type) {
	case nil:
		return []map[string]any{}, nil
	case []string:
		out := make([]map[string]any, 0, len(items))
		for i, item := range items {
			out = append(out, map[string]any{"item": item, "index": i})
		}
		return out, nil
	case []any:
		out := make([]map[string]any, 0, len(items))
		for i, item := range items {
			out = append(out, map[string]any{"item": item, "index": i})
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected array, got %T", raw)
	}
}

func cloneCanvasState(src *CanvasState) (*CanvasState, error) {
	if src == nil {
		return NewCanvasState("", ""), nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	dst := NewCanvasState(src.RunID, src.TaskID)
	if err := json.Unmarshal(raw, dst); err != nil {
		return nil, err
	}
	return dst, nil
}

// shallowCopyAnyMap returns a new map with the same keys/values as src.
// A nil src yields an empty (non-nil) map so callers can assign into the
// result without nil checks. Values are shared, not deep-copied.
func shallowCopyAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return map[string]any{}
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func resolveParallelItemRef(itemOut map[string]any, ref string) any {
	if ref == "" || itemOut == nil {
		return nil
	}
	if ref == "item" {
		return itemOut["item"]
	}
	if ref == "index" {
		return itemOut["index"]
	}

	if at := indexAt(ref); at > 0 {
		cpnID := ref[:at]
		param := ref[at+1:]
		if bucket, ok := itemOut[cpnID].(map[string]any); ok {
			return dotTraverseAny(bucket, param)
		}
	}
	return nil
}

func indexAt(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] == '@' {
			return i
		}
	}
	return -1
}

func dotTraverseAny(v any, path string) any {
	if path == "" {
		return v
	}
	cur := v
	segStart := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != '.' {
			continue
		}
		seg := path[segStart:i]
		segStart = i + 1
		m, ok := cur.(map[string]any)
		if !ok || seg == "" {
			return nil
		}
		cur = m[seg]
		if cur == nil {
			return nil
		}
	}
	return cur
}
