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
	"strings"
	"testing"

	einotool "github.com/cloudwego/eino/components/tool"
)

type fakePubMedInvoker struct {
	args map[string]any
	err  error
	out  string
}

func (f *fakePubMedInvoker) InvokableRun(_ context.Context, argsJSON string, _ ...einotool.Option) (string, error) {
	if err := json.Unmarshal([]byte(argsJSON), &f.args); err != nil {
		return "", err
	}
	if f.out != "" || f.err != nil {
		return f.out, f.err
	}
	return `{"results":[{"title":"Deep learning for retrieval augmented generation","url":"https://pubmed.ncbi.nlm.nih.gov/12345678","content":"Title: Deep learning for retrieval augmented generation\nAuthors: Furqan Khan, Jane Smith\nJournal: Nature Machine Intelligence\nVolume: 10\nIssue: 2\nPages: 101-110\nDOI: 10.1000/example.doi\nAbstract: A short abstract."}]}`, nil
}

func TestPubMed_RegisteredFactory(t *testing.T) {
	t.Parallel()

	c, err := New("PubMed", nil)
	if err != nil {
		t.Fatalf("New(PubMed) errored: %v", err)
	}
	if got := c.Name(); got != "PubMed" {
		t.Fatalf("Name() = %q, want PubMed", got)
	}
	formGetter, ok := c.(interface{ GetInputForm() map[string]any })
	if !ok {
		t.Fatal("PubMed component does not expose GetInputForm")
	}
	form := formGetter.GetInputForm()
	if len(form) != 1 {
		t.Fatalf("GetInputForm size = %d, want 1", len(form))
	}
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

func TestPubMed_InvokeOnlyPassesQuery(t *testing.T) {
	t.Parallel()

	fake := &fakePubMedInvoker{}
	c := newPubMedComponentWithInvoker(fake)
	out, err := c.Invoke(context.Background(), map[string]any{
		"query":  "  retrieval augmented generation  ",
		"top_n":  float64(8),
		"email":  "ignored@example.com",
		"unused": true,
	})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := fake.args["query"]; got != "retrieval augmented generation" {
		t.Fatalf("query arg = %v, want trimmed query", got)
	}
	if len(fake.args) != 1 {
		t.Fatalf("runtime args = %#v, want only query", fake.args)
	}
	formalized, _ := out["formalized_content"].(string)
	for _, want := range []string{"ID: 0", "Title: Deep learning for retrieval augmented generation", "URL: https://pubmed.ncbi.nlm.nih.gov/12345678", "Content:", "Abstract: A short abstract."} {
		if !strings.Contains(formalized, want) {
			t.Fatalf("formalized_content missing %q: %s", want, formalized)
		}
	}
	results, ok := out["json"].([]any)
	if !ok || len(results) != 1 {
		t.Fatalf("json output = %#v, want one result", out["json"])
	}
}

func TestPubMed_InvokeEmptyQueryReturnsEmptyPayload(t *testing.T) {
	t.Parallel()

	c := newPubMedComponentWithInvoker(&fakePubMedInvoker{})
	out, err := c.Invoke(context.Background(), map[string]any{"query": "   "})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := out["formalized_content"]; got != "" {
		t.Fatalf("formalized_content = %v, want empty string", got)
	}
	results, ok := out["json"].([]any)
	if !ok || len(results) != 0 {
		t.Fatalf("json output = %#v, want empty []any", out["json"])
	}
}

func TestPubMed_InvokeSurfacesToolErrorEnvelope(t *testing.T) {
	t.Parallel()

	fake := &fakePubMedInvoker{
		out: `{"results":[],"_ERROR":"upstream down"}`,
		err: errors.New("boom"),
	}
	c := newPubMedComponentWithInvoker(fake)
	out, err := c.Invoke(context.Background(), map[string]any{"query": "pubmed"})
	if err != nil {
		t.Fatalf("Invoke errored: %v", err)
	}
	if got := out["_ERROR"]; got != "upstream down" {
		t.Fatalf("_ERROR = %v, want upstream down", got)
	}
	if got := out["formalized_content"]; got != "" {
		t.Fatalf("formalized_content = %v, want empty string", got)
	}
}
