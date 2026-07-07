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
)

type fakeBGPTInvoker struct {
	args map[string]any
}

func (f *fakeBGPTInvoker) InvokableRun(_ context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	return `{"results":[{"title":"Paper A","authors":"Lee, Kim","journal":"Science","year":"2026","doi":"10.1/a","abstract":"Abstract A","methods":"RCT","sample_size":"120","results":"Improved outcomes","limitations":"Small cohort","conflict_of_interest":"None","data_availability":"Available","blind_spots":"Long term effects","falsify":"Run a larger trial"}]}`, nil
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
