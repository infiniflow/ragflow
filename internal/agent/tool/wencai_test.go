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

package tool

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestWencai_InvokeMatchesCurrentPythonResult(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		args string
	}{
		{name: "query", args: `{"query":"商业航天"}`},
		{name: "empty query", args: `{"query":""}`},
		{name: "missing query", args: `{}`},
		{name: "empty arguments", args: ``},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, err := NewWencaiTool().InvokableRun(context.Background(), tc.args)
			if err != nil {
				t.Fatalf("InvokableRun errored: %v (out=%s)", err, out)
			}
			var env wencaiEnvelope
			if err := json.Unmarshal([]byte(out), &env); err != nil {
				t.Fatalf("output is not valid JSON: %v (raw=%s)", err, out)
			}
			if env.Report != "" {
				t.Fatalf("report = %q, want empty string", env.Report)
			}
			if env.Error != "" {
				t.Fatalf("_ERROR = %q, want empty", env.Error)
			}
		})
	}
}

func TestWencai_RejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	out, err := NewWencaiTool().InvokableRun(context.Background(), `{not json`)
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	if !strings.Contains(err.Error(), "parse arguments") {
		t.Fatalf("err = %q, want parse arguments", err.Error())
	}
	var env wencaiEnvelope
	if jsonErr := json.Unmarshal([]byte(out), &env); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jsonErr, out)
	}
	if env.Error == "" {
		t.Fatal("_ERROR is empty, want parse error")
	}
}

func TestWencai_RespectsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	out, err := NewWencaiTool().InvokableRun(ctx, `{"query":"商业航天"}`)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	var env wencaiEnvelope
	if jsonErr := json.Unmarshal([]byte(out), &env); jsonErr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jsonErr, out)
	}
	if env.Error != context.Canceled.Error() {
		t.Fatalf("_ERROR = %q, want %q", env.Error, context.Canceled.Error())
	}
}

func TestWencai_InfoMatchesPythonMeta(t *testing.T) {
	t.Parallel()

	tool := NewWencaiTool()
	meta := tool.ToolMeta()
	if meta.Name != "iwencai" {
		t.Fatalf("Name = %q, want iwencai", meta.Name)
	}
	if strings.Contains(meta.Description, "STUB") || strings.Contains(meta.Description, "unsupported") {
		t.Fatalf("Desc still exposes stub behavior: %q", meta.Description)
	}
	paramsJSON, _ := json.Marshal(meta.Parameters)
	params := string(paramsJSON)
	if !strings.Contains(params, `"query"`) {
		t.Fatalf("params missing query: %s", params)
	}
}

func TestWencai_BuildByNameAcceptsNodeParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("wencai", map[string]any{
		"top_n":      float64(20),
		"query_type": "fund",
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	wencai, ok := built.(*WencaiTool)
	if !ok {
		t.Fatalf("built type = %T, want *WencaiTool", built)
	}
	if wencai.defaults.TopN != 20 {
		t.Fatalf("defaults.TopN = %d, want 20", wencai.defaults.TopN)
	}
	if wencai.defaults.QueryType != "fund" {
		t.Fatalf("defaults.QueryType = %q, want fund", wencai.defaults.QueryType)
	}
}

func TestWencai_BuildByNameUsesPythonDefaults(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("wencai", nil)
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	wencai := built.(*WencaiTool)
	if wencai.defaults.TopN != 10 || wencai.defaults.QueryType != "stock" {
		t.Fatalf("defaults = %+v, want top_n=10 query_type=stock", wencai.defaults)
	}
}

func TestWencai_BuildByNameRejectsInvalidNodeParams(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		params map[string]any
	}{
		{name: "zero top_n", params: map[string]any{"top_n": 0}},
		{name: "fractional top_n", params: map[string]any{"top_n": 1.5}},
		{name: "string top_n", params: map[string]any{"top_n": "10"}},
		{name: "invalid query_type", params: map[string]any{"query_type": "crypto"}},
		{name: "non-string query_type", params: map[string]any{"query_type": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := BuildByName("wencai", tc.params); err == nil {
				t.Fatalf("BuildByName(%v) succeeded, want validation error", tc.params)
			}
		})
	}
}

func TestWencai_BuildByNameIgnoresUnrelatedCanvasParams(t *testing.T) {
	t.Parallel()

	built, err := BuildByName("wencai", map[string]any{
		"top_n":   float64(20),
		"outputs": map[string]any{"report": map[string]any{}},
		"setups":  map[string]any{"query": "configured query"},
	})
	if err != nil {
		t.Fatalf("BuildByName: %v", err)
	}
	wencai := built.(*WencaiTool)
	if wencai.defaults.TopN != 20 {
		t.Fatalf("defaults.TopN = %d, want 20", wencai.defaults.TopN)
	}
}

func TestWencai_ComponentContract(t *testing.T) {
	t.Parallel()

	wencai := NewWencaiTool()
	spec := wencai.ComponentSpec()
	if _, ok := spec.Inputs["query"]; !ok {
		t.Fatalf("component inputs missing query: %#v", spec.Inputs)
	}
	if _, ok := spec.Outputs["report"]; !ok {
		t.Fatalf("component outputs missing report: %#v", spec.Outputs)
	}
	query, ok := spec.InputForm["query"].(map[string]any)
	if !ok || query["name"] != "Query" || query["type"] != "line" {
		t.Fatalf("query input form = %#v", spec.InputForm["query"])
	}
	outputs := wencai.BuildComponentOutputs(map[string]any{
		"report":        "market report",
		"tool_metadata": map[string]any{"request_id": "request-1"},
	})
	if outputs["report"] != "market report" {
		t.Fatalf("component outputs = %#v", outputs)
	}
}

func TestWencai_MergeDefaults(t *testing.T) {
	t.Parallel()

	got := mergeWencaiParams(
		wencaiParams{Query: "configured", TopN: 20, QueryType: "fund"},
		wencaiParams{Query: "商业航天"},
	)
	if got.Query != "商业航天" || got.TopN != 20 || got.QueryType != "fund" {
		t.Fatalf("merged params = %+v", got)
	}
}
