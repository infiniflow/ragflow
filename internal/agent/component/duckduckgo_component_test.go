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
	"strings"
	"testing"

	"ragflow/internal/agent/tool"
)

type fakeDuckDuckGoInvoker struct {
	args map[string]any
}

func (f *fakeDuckDuckGoInvoker) ToolMeta() tool.ToolMeta {
	return tool.ToolMeta{Name: "DuckDuckGo"}
}

func (f *fakeDuckDuckGoInvoker) InvokableRun(_ context.Context, argsJSON string) (string, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &raw); err != nil {
		return "", err
	}
	f.args = make(map[string]any, len(raw))
	for k, v := range raw {
		f.args[k] = v
	}
	if q, ok := f.args["query"].(string); ok {
		f.args["query"] = strings.TrimSpace(q)
	}
	if q, ok := f.args["query"].(string); !ok || strings.TrimSpace(q) == "" {
		return `{"formalized_content":"","json":[],"results":[]}`, nil
	}
	return `{"formalized_content":"**RAGFlow** - https://ragflow.io\nOpen source RAG engine","json":[{"title":"RAGFlow","url":"https://ragflow.io","body":"Open source RAG engine"}],"results":[{"title":"RAGFlow","url":"https://ragflow.io","body":"Open source RAG engine"}]}`, nil
}

func TestDuckDuckGo_RegisteredFactory(t *testing.T) {
	t.Parallel()

	c, err := New("DuckDuckGo", nil)
	if err != nil {
		t.Fatalf("New(DuckDuckGo) errored: %v", err)
	}
	if got := c.Name(); got != "DuckDuckGo" {
		t.Fatalf("Name() = %q, want DuckDuckGo", got)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("DuckDuckGo component does not expose GetInputForm")
	}
	form := formGetter.GetInputForm()
	query, ok := form["query"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[query] has type %T, want map", form["query"])
	}
	if query["type"] != "line" {
		t.Fatalf("GetInputForm()[query][type] = %v, want line", query["type"])
	}
	channel, ok := form["channel"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[channel] has type %T, want map", form["channel"])
	}
	if channel["value"] != "general" {
		t.Fatalf("GetInputForm()[channel][value] = %v, want general", channel["value"])
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestDuckDuckGo_InvokeAdaptsCanvasInputsAndOutputs(t *testing.T) {
	t.Parallel()

	fake := &fakeDuckDuckGoInvoker{}
	c := newDuckDuckGoComponentWithInvoker(fake)

	out, err := c.Invoke(context.Background(), map[string]any{
		"query":   "  privacy search  ",
		"channel": "news",
		"top_n":   float64(3),
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}

	if got := fake.args["query"]; got != "privacy search" {
		t.Errorf("query arg = %v, want trimmed query", got)
	}
	if got := fake.args["channel"]; got != "news" {
		t.Errorf("channel arg = %v, want news", got)
	}
	if got := fake.args["top_n"]; got != float64(3) {
		t.Errorf("top_n arg = %v, want 3", got)
	}

	formalized, _ := out["formalized_content"].(string)
	for _, want := range []string{"RAGFlow", "https://ragflow.io", "Open source RAG engine"} {
		if !strings.Contains(formalized, want) {
			t.Errorf("formalized_content missing %q: %s", want, formalized)
		}
	}

	results, ok := out["json"].([]any)
	if !ok {
		t.Fatalf("json output has type %T, want []any", out["json"])
	}
	if len(results) != 1 {
		t.Fatalf("json output length = %d, want 1", len(results))
	}
}

func TestDuckDuckGo_InvokeEmptyQueryReturnsEmptyPayload(t *testing.T) {
	t.Parallel()

	c := newDuckDuckGoComponentWithInvoker(&fakeDuckDuckGoInvoker{})
	out, err := c.Invoke(context.Background(), map[string]any{"query": "   "})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := out["formalized_content"]; got != "" {
		t.Errorf("formalized_content = %v, want empty string", got)
	}
	results, ok := out["json"].([]any)
	if !ok || len(results) != 0 {
		t.Fatalf("json output = %#v, want empty []any", out["json"])
	}
}
