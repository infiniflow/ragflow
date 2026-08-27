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
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
)

type fakeGoogleScholarInvoker struct {
	args map[string]any
}

func (f *fakeGoogleScholarInvoker) InvokableRun(_ context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	return `{"results":[{"title":"Paper","link":"https://example.com","authors":"A Author","year":"2024","snippet":"Abstract"}]}`, nil
}

func TestGoogleScholar_InvokePassesCanvasParams(t *testing.T) {
	t.Parallel()

	fake := &fakeGoogleScholarInvoker{}
	c := newGoogleScholarComponentWithInvoker(fake, nil)

	_, err := c.Invoke(context.Background(), map[string]any{
		"query":     "  retrieval augmented generation  ",
		"top_n":     float64(7),
		"sort_by":   "date",
		"year_low":  float64(2020),
		"year_high": float64(2024),
		"patents":   false,
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}

	if got := fake.args["query"]; got != "retrieval augmented generation" {
		t.Errorf("query arg = %v, want trimmed query", got)
	}
	if got := fake.args["top_n"]; got != float64(7) {
		t.Errorf("top_n arg = %v, want 7", got)
	}
	if _, ok := fake.args["max_results"]; ok {
		t.Fatalf("max_results should not be sent for GoogleScholar args: %#v", fake.args)
	}
	if got := fake.args["sort_by"]; got != "date" {
		t.Errorf("sort_by arg = %v, want date", got)
	}
	if got := fake.args["year_low"]; got != float64(2020) {
		t.Errorf("year_low arg = %v, want 2020", got)
	}
	if got := fake.args["year_high"]; got != float64(2024) {
		t.Errorf("year_high arg = %v, want 2024", got)
	}
	if got := fake.args["patents"]; got != false {
		t.Errorf("patents arg = %v, want false", got)
	}
}

func TestGoogleScholar_InvokeMergesNodeParams(t *testing.T) {
	t.Parallel()

	fake := &fakeGoogleScholarInvoker{}
	c := newGoogleScholarComponentWithInvoker(fake, map[string]any{
		"top_n":    float64(20),
		"sort_by":  "date",
		"patents":  false,
	})

	_, err := c.Invoke(context.Background(), map[string]any{
		"query": "machine learning",
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}

	if got := fake.args["top_n"]; got != float64(20) {
		t.Errorf("top_n arg = %v, want 20 (from node params)", got)
	}
	if got := fake.args["sort_by"]; got != "date" {
		t.Errorf("sort_by arg = %v, want date (from node params)", got)
	}
	if got := fake.args["patents"]; got != false {
		t.Errorf("patents arg = %v, want false (from node params)", got)
	}
}

func TestGoogleScholar_InvokeInputsOverrideNodeParams(t *testing.T) {
	t.Parallel()

	fake := &fakeGoogleScholarInvoker{}
	c := newGoogleScholarComponentWithInvoker(fake, map[string]any{
		"top_n":   float64(20),
		"sort_by": "relevance",
	})

	_, err := c.Invoke(context.Background(), map[string]any{
		"query":   "deep learning",
		"top_n":   float64(5),
		"sort_by": "date",
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}

	if got := fake.args["top_n"]; got != float64(5) {
		t.Errorf("top_n arg = %v, want 5 (inputs override node params)", got)
	}
	if got := fake.args["sort_by"]; got != "date" {
		t.Errorf("sort_by arg = %v, want date (inputs override node params)", got)
	}
}
