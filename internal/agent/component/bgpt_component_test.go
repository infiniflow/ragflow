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

type fakeBGPTInvoker struct {
	args map[string]any
}

func (f *fakeBGPTInvoker) ToolMeta() tool.ToolMeta {
	return tool.ToolMeta{Name: "BGPT"}
}

func (f *fakeBGPTInvoker) InvokableRun(_ context.Context, argsJSON string) (string, error) {
	// Parse and normalize inputs matching the real BGPTTool behavior.
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
	if v, ok := f.args["top_n"]; ok {
		f.args["num_results"] = v
		// Real tool also maps max_results. Keep top_n for backward compat.
	}
	if v, ok := f.args["days_back"]; ok {
		f.args["max_days_back"] = v
	}
	return `{"formalized_content":"**Paper A** (Lee, Kim, Science, 2026)\nDOI: 10.1/a\n**Abstract:** Abstract A\n**Methods:** RCT (n=120)\n**Results:** Improved outcomes\n**Limitations:** Small cohort\n**COI:** None\n**Data:** Available\n**Blind spots:** Long term effects\n**Falsify:** Run a larger trial\n","json":[{"title":"Paper A","authors":"Lee, Kim","journal":"Science","year":"2026","doi":"10.1/a","abstract":"Abstract A","methods":"RCT","sample_size":"120","results":"Improved outcomes","limitations":"Small cohort","conflict_of_interest":"None","data_availability":"Available","blind_spots":"Long term effects","falsify":"Run a larger trial"}],"results":[{"title":"Paper A","authors":"Lee, Kim","journal":"Science","year":"2026","doi":"10.1/a","abstract":"Abstract A","methods":"RCT","sample_size":"120","results":"Improved outcomes","limitations":"Small cohort","conflict_of_interest":"None","data_availability":"Available","blind_spots":"Long term effects","falsify":"Run a larger trial"}]}`, nil
}

func TestBGPT_RegisteredFactory(t *testing.T) {
	c, err := New("BGPT", nil)
	if err != nil {
		t.Fatalf("New(BGPT) errored: %v", err)
	}
	if got := c.Name(); got != "BGPT" {
		t.Fatalf("Name() = %q, want BGPT", got)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("BGPT component does not expose GetInputForm")
	}
	form := formGetter.GetInputForm()
	query, ok := form["query"].(map[string]any)
	if !ok {
		t.Fatalf("GetInputForm()[query] has type %T, want map", form["query"])
	}
	if query["type"] != "line" {
		t.Fatalf("GetInputForm()[query][type] = %v, want line", query["type"])
	}
	if _, ok := c.Outputs()["formalized_content"]; !ok {
		t.Fatal("Outputs() missing formalized_content")
	}
	if _, ok := c.Outputs()["json"]; !ok {
		t.Fatal("Outputs() missing json")
	}
}

func TestBGPT_InvokeAdaptsCanvasInputsAndOutputs(t *testing.T) {
	fake := &fakeBGPTInvoker{}
	c := newBGPTComponentWithInvoker(fake)

	out, err := c.Invoke(context.Background(), map[string]any{
		"query":     "  cancer therapy  ",
		"api_key":   "key-1",
		"days_back": float64(30),
		"top_n":     float64(3),
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}

	if got := fake.args["query"]; got != "cancer therapy" {
		t.Errorf("query arg = %v, want trimmed query", got)
	}
	if got := fake.args["api_key"]; got != "key-1" {
		t.Errorf("api_key arg = %v, want key-1", got)
	}
	if got := fake.args["days_back"]; got != float64(30) {
		t.Errorf("days_back arg = %v, want 30", got)
	}
	if got := fake.args["num_results"]; got != float64(3) {
		t.Errorf("num_results arg = %v, want 3 from top_n", got)
	}

	formalized, _ := out["formalized_content"].(string)
	for _, want := range []string{"Paper A", "Lee, Kim", "Improved outcomes", "Run a larger trial"} {
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

func TestBGPT_InvokeUsesStoredAPIKeyWhenInputOmitsIt(t *testing.T) {
	fake := &fakeBGPTInvoker{}
	c := newBGPTComponentWithInvoker(fake, "stored-key")

	_, err := c.Invoke(context.Background(), map[string]any{
		"query": "cancer therapy",
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := fake.args["api_key"]; got != "stored-key" {
		t.Fatalf("api_key arg = %v, want stored-key", got)
	}
}

func TestBGPT_InvokeDoesNotOverrideCallerAPIKey(t *testing.T) {
	fake := &fakeBGPTInvoker{}
	c := newBGPTComponentWithInvoker(fake, "stored-key")

	_, err := c.Invoke(context.Background(), map[string]any{
		"query":   "cancer therapy",
		"api_key": "call-key",
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := fake.args["api_key"]; got != "call-key" {
		t.Fatalf("api_key arg = %v, want call-key", got)
	}
}
