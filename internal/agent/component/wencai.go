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

package component

import (
	"context"
	"encoding/json"
	"fmt"

	agenttool "ragflow/internal/agent/tool"
)

type wencaiInvoker interface {
	InvokableRun(ctx context.Context, argsJSON string) (string, error)
}

type wencaiComponent struct {
	inner wencaiInvoker
}

func newWencaiComponent(params map[string]any) (Component, error) {
	toolParams := make(map[string]any, 2)
	for _, key := range []string{"top_n", "query_type"} {
		if value, ok := params[key]; ok {
			toolParams[key] = value
		}
	}
	inner, err := agenttool.BuildByName("wencai", toolParams)
	if err != nil {
		return nil, err
	}
	invoker, ok := inner.(wencaiInvoker)
	if !ok {
		return nil, fmt.Errorf("WenCai: tool does not implement InvokableRun")
	}
	return newWencaiComponentWithInvoker(invoker), nil
}

func newWencaiComponentWithInvoker(inner wencaiInvoker) Component {
	return &wencaiComponent{inner: inner}
}

func (c *wencaiComponent) Name() string { return "WenCai" }

func (c *wencaiComponent) Inputs() map[string]string {
	return map[string]string{
		"query": "The question/conditions to select stocks.",
	}
}

func (c *wencaiComponent) Outputs() map[string]string {
	return map[string]string{
		"report": "WenCai query report.",
	}
}

func (c *wencaiComponent) GetInputForm() map[string]any {
	return map[string]any{
		"query": map[string]any{
			"name": "Query",
			"type": "line",
		},
	}
}

func (c *wencaiComponent) Invoke(ctx context.Context, inputs map[string]any) (map[string]any, error) {
	query := stringParam(inputs["query"])
	if query == "" {
		return map[string]any{"report": ""}, nil
	}
	argsJSON, err := json.Marshal(map[string]any{"query": query})
	if err != nil {
		return nil, fmt.Errorf("canvas: WenCai: encode query: %w", err)
	}
	out, err := c.inner.InvokableRun(ctx, string(argsJSON))
	decoded := parseToolEnvelope(out)
	report, _ := decoded["report"].(string)
	if message, _ := decoded["_ERROR"].(string); message != "" {
		return map[string]any{"report": report, "_ERROR": message}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("canvas: WenCai: %w", err)
	}
	return map[string]any{"report": report}, nil
}

func (c *wencaiComponent) Stream(_ context.Context, _ map[string]any) (<-chan map[string]any, error) {
	return nil, nil
}

func init() {
	Register("WenCai", newWencaiComponent)
}
