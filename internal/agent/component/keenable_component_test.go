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

	einotool "github.com/cloudwego/eino/components/tool"

	"ragflow/internal/agent/runtime"
)

type fakeKeenableInvoker struct {
	args map[string]any
}

func (f *fakeKeenableInvoker) InvokableRun(_ context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	return `{"results":[{"title":"Keenable result","url":"https://example.com/item","description":"Fresh search result"}]}`, nil
}

func TestKeenableSearch_RegisteredFactory(t *testing.T) {
	t.Parallel()

	c, err := New("KeenableSearch", map[string]any{"api_key": "key-1", "mode": "realtime", "top_n": float64(3)})
	if err != nil {
		t.Fatalf("New(KeenableSearch) errored: %v", err)
	}
	kc, ok := c.(*keenableSearchComponent)
	if !ok {
		t.Fatalf("New(KeenableSearch) returned %T, want *keenableSearchComponent", c)
	}
	if kc.apiKey != "key-1" {
		t.Fatalf("apiKey = %q, want key-1", kc.apiKey)
	}
	if kc.mode != "realtime" {
		t.Fatalf("mode = %q, want realtime", kc.mode)
	}
	if kc.topN != 3 {
		t.Fatalf("topN = %d, want 3", kc.topN)
	}

	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("KeenableSearch component does not expose GetInputForm")
	}
	form := formGetter.GetInputForm()
	query, ok := form["query"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[query] has type %T, want map", form["query"])
	}
	if query["type"] != "line" {
		t.Fatalf("GetInputForm()[query][type] = %v, want line", query["type"])
	}
	site, ok := form["site"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[site] has type %T, want map", form["site"])
	}
	if site["type"] != "line" {
		t.Fatalf("GetInputForm()[site][type] = %v, want line", site["type"])
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestKeenableSearch_InvokeAdaptsCanvasInputsAndOutputs(t *testing.T) {
	t.Parallel()

	fake := &fakeKeenableInvoker{}
	c := newKeenableSearchComponentWithInvoker(fake, map[string]any{
		"mode":  "pro",
		"top_n": 5,
		"site":  "example.com",
	})

	state := runtime.NewCanvasState("run-keenable", "task-keenable")
	ctx := runtime.WithState(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"query": "  agent search  ",
		"mode":  "realtime",
		"top_n": float64(2),
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}

	if got := fake.args["query"]; got != "agent search" {
		t.Errorf("query arg = %v, want trimmed query", got)
	}
	if got := fake.args["mode"]; got != "realtime" {
		t.Errorf("mode arg = %v, want realtime", got)
	}
	if got := fake.args["top_n"]; got != float64(2) && got != 2 {
		t.Errorf("top_n arg = %v, want 2", got)
	}
	if got := fake.args["site"]; got != "example.com" {
		t.Errorf("site arg = %v, want default site", got)
	}

	formalized, _ := out["formalized_content"].(string)
	for _, want := range []string{"Keenable result", "https://example.com/item", "Fresh search result"} {
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

	reference := state.GetRetrievalReference()
	chunks, _ := reference["chunks"].([]any)
	if len(chunks) != 1 {
		t.Fatalf("reference chunks length = %d, want 1", len(chunks))
	}
	chunk, _ := chunks[0].(map[string]any)
	if chunk["document_name"] != "Keenable result" || chunk["url"] != "https://example.com/item" {
		t.Fatalf("reference chunk metadata = %#v", chunk)
	}
	docAggs, _ := reference["doc_aggs"].([]any)
	if len(docAggs) != 1 {
		t.Fatalf("reference doc_aggs length = %d, want 1", len(docAggs))
	}
}

func TestKeenableSearch_InvokeEmptyQueryReturnsEmptyPayload(t *testing.T) {
	t.Parallel()

	c := newKeenableSearchComponentWithInvoker(&fakeKeenableInvoker{}, nil)
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
