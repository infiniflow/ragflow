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
	"errors"
	"testing"

	agentcanvas "ragflow/internal/agent/canvas"
)

type fakeWencaiInvoker struct {
	args  map[string]any
	calls int
	out   string
	err   error
}

func (f *fakeWencaiInvoker) InvokableRun(_ context.Context, argsJSON string) (string, error) {
	f.calls++
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	if f.out == "" && f.err == nil {
		return `{"report":""}`, nil
	}
	return f.out, f.err
}

func TestWencai_RegisteredFactoryMatchesPythonSurface(t *testing.T) {
	t.Parallel()

	c, err := New("WenCai", map[string]any{
		"top_n":      float64(20),
		"query_type": "stock",
		"outputs":    map[string]any{"report": map[string]any{}},
		"setups":     map[string]any{"query": "configured query"},
	})
	if err != nil {
		t.Fatalf("New(WenCai): %v", err)
	}
	if got := c.Name(); got != "WenCai" {
		t.Fatalf("Name() = %q, want WenCai", got)
	}
	if _, ok := c.Inputs()["query"]; !ok {
		t.Fatal("Inputs() missing query")
	}
	if _, ok := c.Outputs()["report"]; !ok {
		t.Fatal("Outputs() missing report")
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("WenCai component does not expose GetInputForm")
	}
	query, ok := formGetter.GetInputForm()["query"].(map[string]any)
	if !ok {
		t.Fatalf("query form has type %T, want map", formGetter.GetInputForm()["query"])
	}
	if query["name"] != "Query" || query["type"] != "line" {
		t.Fatalf("query form = %#v, want name=Query type=line", query)
	}
}

func TestWencai_InvokeReturnsEmptyReportWithoutError(t *testing.T) {
	t.Parallel()

	fake := &fakeWencaiInvoker{}
	c := newWencaiComponentWithInvoker(fake)
	out, err := c.Invoke(context.Background(), map[string]any{
		"query":      "商业航天",
		"top_n":      20,
		"query_type": "stock",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["report"]; got != "" {
		t.Fatalf("report = %v, want empty string", got)
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}
	if got := fake.args["query"]; got != "商业航天" {
		t.Fatalf("runtime query = %v, want 商业航天", got)
	}
	if len(fake.args) != 1 {
		t.Fatalf("runtime args = %#v, want query only", fake.args)
	}
}

func TestWencai_InvokeEmptyQuerySkipsTool(t *testing.T) {
	t.Parallel()

	fake := &fakeWencaiInvoker{}
	c := newWencaiComponentWithInvoker(fake)
	out, err := c.Invoke(context.Background(), map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["report"]; got != "" {
		t.Fatalf("report = %v, want empty string", got)
	}
	if fake.calls != 0 {
		t.Fatalf("calls = %d, want 0", fake.calls)
	}
}

func TestWencai_InvokePreservesErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := &fakeWencaiInvoker{
		out: `{"report":"","_ERROR":"upstream failed"}`,
		err: errors.New("upstream failed"),
	}
	c := newWencaiComponentWithInvoker(fake)
	out, err := c.Invoke(context.Background(), map[string]any{"query": "商业航天"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["report"]; got != "" {
		t.Fatalf("report = %v, want empty string", got)
	}
	if got := out["_ERROR"]; got != "upstream failed" {
		t.Fatalf("_ERROR = %v, want upstream failed", got)
	}
}

func TestWencai_InvokePreservesErrorEnvelopeWithoutGoError(t *testing.T) {
	t.Parallel()

	fake := &fakeWencaiInvoker{
		out: `{"report":"","_ERROR":"business error"}`,
	}
	c := newWencaiComponentWithInvoker(fake)
	out, err := c.Invoke(context.Background(), map[string]any{"query": "商业航天"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := out["report"]; got != "" {
		t.Fatalf("report = %v, want empty string", got)
	}
	if got := out["_ERROR"]; got != "business error" {
		t.Fatalf("_ERROR = %v, want business error", got)
	}
}

func TestWencai_BuildWorkflowUsesPythonComponentName(t *testing.T) {
	c := &agentcanvas.Canvas{
		Components: map[string]agentcanvas.CanvasComponent{
			"begin_0": {
				Obj:        agentcanvas.CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
				Downstream: []string{"wencai_0"},
			},
			"wencai_0": {
				Obj: agentcanvas.CanvasComponentObj{ComponentName: "WenCai", Params: map[string]any{
					"top_n":      float64(20),
					"query_type": "stock",
				}},
				Upstream:   []string{"begin_0"},
				Downstream: []string{"message_0"},
			},
			"message_0": {
				Obj:      agentcanvas.CanvasComponentObj{ComponentName: "Message", Params: map[string]any{}},
				Upstream: []string{"wencai_0"},
			},
		},
		Path: []string{"begin_0", "wencai_0", "message_0"},
	}
	if _, err := agentcanvas.BuildWorkflow(context.Background(), c); err != nil {
		t.Fatalf("BuildWorkflow with WenCai: %v", err)
	}
}
