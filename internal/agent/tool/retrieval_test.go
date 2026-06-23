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

func TestRetrieval_StubsErrorWhenServiceMissing(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	out, err := rt.InvokableRun(context.Background(), `{"query":"hello"}`)
	if err == nil {
		t.Fatal("expected stub error, got nil")
	}
	if !errors.Is(err, ErrRetrievalServiceMissing) {
		t.Fatalf("err = %v, want ErrRetrievalServiceMissing", err)
	}

	// Output is a JSON envelope with the stub error message.
	var got retrievalResult
	if jerr := json.Unmarshal([]byte(out), &got); jerr != nil {
		t.Fatalf("output is not valid JSON: %v (raw=%s)", jerr, out)
	}
	if !got.Stub {
		t.Errorf("Stub = false, want true")
	}
	if !strings.Contains(got.Error, "service not yet implemented") {
		t.Errorf("Error = %q, want to mention 'service not yet implemented'", got.Error)
	}
}

func TestRetrieval_RejectsUseKG(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	out, err := rt.InvokableRun(context.Background(), `{"query":"x","use_kg":true}`)
	if !errors.Is(err, ErrGraphRAGNotSupported) {
		t.Fatalf("err = %v, want ErrGraphRAGNotSupported", err)
	}
	if !strings.Contains(out, "GraphRAG") {
		t.Errorf("output %q should mention GraphRAG", out)
	}
}

func TestRetrieval_InfoMatchesPythonMeta(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	info, err := rt.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Name != "search_my_dateset" {
		t.Errorf("Name = %q, want search_my_dateset (typo preserved)", info.Name)
	}
	if !strings.Contains(info.Desc, "datasets") {
		t.Errorf("Desc = %q, want to mention 'datasets'", info.Desc)
	}
	// The query param must be present and required. ToJSONSchema returns
	// a *jsonschema.Schema whose Properties is an *orderedmap.Map; we use
	// MarshalJSON to assert the parameter set without depending on the
	// map's concrete Get signature.
	params, err := info.ToJSONSchema()
	if err != nil {
		t.Fatalf("ToJSONSchema: %v", err)
	}
	raw, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	if !strings.Contains(string(raw), `"query"`) {
		t.Errorf("schema JSON does not contain 'query' key: %s", raw)
	}
}

func TestRetrieval_EmptyArgsIsHandled(t *testing.T) {
	t.Parallel()

	rt := NewRetrievalTool()
	// Empty arguments should still return a stub error (not panic) — the
	// Python tool defaults to empty_response in this case. Without
	// wiring, the Go side surfaces the service-missing error.
	_, err := rt.InvokableRun(context.Background(), "")
	if !errors.Is(err, ErrRetrievalServiceMissing) {
		t.Fatalf("err = %v, want ErrRetrievalServiceMissing", err)
	}
}
