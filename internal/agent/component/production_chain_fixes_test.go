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

// production_chain_fixes_test.go — production-wiring regression tests.
//
// Each test pins one of 4 production bugs the production-chain
// review surfaced:
//
//	ExeSQL v1 DSL param mismatch    → TestExeSQL_V1DSLParamsAccepted
//	Retrieval kb_ids vs dataset_ids → TestRetrieval_KbIDsTranslatedToDatasetIDs
//	SearchMyDataset snake_case drift → TestSearchMyDataset_AllAliasesRegistered
//	LLM retry multiplicative stack  → TestLLM_RetryStackingSemantics
//
// If any of these tests starts failing, the corresponding production
// fix has regressed. Do not silently re-pin — investigate the diff.
package component

import (
	"context"
	"strings"
	"testing"

	agenttool "ragflow/internal/agent/tool"
)

// TestExeSQL_V1DSLParamsAccepted exercises the v1-DSL-compat
// translator that turns v1 DSL ExeSQL params (database/username/
// host/port/password/top_n, no db_type) into the tool's required shape
// (db_type/database/username/host/port/password/max_records).
//
// Without the translator, NewExeSQLConnParams would reject the v1
// shape with "missing required connection params (db_type/host/
// database/username)" and every legacy v1 ExeSQL canvas would fail
// at buildNodeBody time. With the translator in place, the v1 shape
// compiles cleanly (db_type defaults to "mysql"; port is coerced from
// float64; top_n is mapped to max_records).
func TestExeSQL_V1DSLParamsAccepted(t *testing.T) {
	t.Parallel()

	// v1 shape: no db_type, port as float64 (JSON-decoded),
	// top_n present, no max_records. Should compile cleanly.
	v1Params := map[string]any{
		"database": "demo",
		"username": "root",
		"host":     "127.0.0.1",
		"port":     float64(3306), // JSON-decoded numeric is float64.
		"password": "secret",
		"top_n":    float64(50),
	}
	c, err := New(componentNameExeSQL, v1Params)
	if err != nil {
		t.Fatalf("New(ExeSQL) with v1 DSL params errored: %v\n"+
			"(this is the regression — v1 shape must be translated to tool shape)", err)
	}
	if c == nil {
		t.Fatalf("New(ExeSQL) returned nil")
	}
	if got := c.Name(); got != componentNameExeSQL {
		t.Errorf("ExeSQL c.Name() = %q, want %q", got, componentNameExeSQL)
	}

	// translateExeSQLParamsToToolShape should also be directly
	// testable as a pure function: the same shape, the same result.
	got := translateExeSQLParamsToToolShape(v1Params)
	if got["db_type"] != "mysql" {
		t.Errorf("translated db_type = %v, want %q", got["db_type"], "mysql")
	}
	if v, ok := got["port"].(int); !ok || v != 3306 {
		t.Errorf("translated port = %v (%T), want int 3306", got["port"], got["port"])
	}
	if v, ok := got["max_records"].(int); !ok || v != 50 {
		t.Errorf("translated max_records = %v (%T), want int 50", got["max_records"], got["max_records"])
	}
	if _, ok := got["top_n"]; ok {
		t.Errorf("translated map should drop top_n (mapped to max_records)")
	}

	// Idempotency: a second pass must not double-default.
	got2 := translateExeSQLParamsToToolShape(got)
	if got2["db_type"] != "mysql" {
		t.Errorf("idempotent db_type = %v, want %q", got2["db_type"], "mysql")
	}
	if v, ok := got2["port"].(int); !ok || v != 3306 {
		t.Errorf("idempotent port = %v (%T), want int 3306", got2["port"], got2["port"])
	}

	// Explicit override wins: passing db_type=postgres must be
	// preserved through the translator.
	override := translateExeSQLParamsToToolShape(map[string]any{
		"db_type": "postgres",
		"host":    "10.0.0.1",
	})
	if override["db_type"] != "postgres" {
		t.Errorf("override db_type = %v, want %q", override["db_type"], "postgres")
	}
}

// TestRetrieval_KbIDsTranslatedToDatasetIDs pins the
// v1-DSL-compat translator that maps the v1 DSL name `kb_ids`
// (the Python surface) to the tool's expected name `dataset_ids`
// (the Go tool JSON schema).
//
// Without the fix, the wrapper marshals `kb_ids` into the
// argumentsInJSON sent to RetrievalTool.InvokableRun; the tool's
// retrievalArgs struct only declares `dataset_ids`, so the
// Unmarshal silently drops the v1 field and the resulting
// RetrievalRequest.DatasetIDs is empty — the search degrades to
// "no filter", which is a behavioural regression vs the v1
// Python canvas where kb_ids was honoured.
//
// The test reads the merged map directly through applyDefaults
// (same-package access). It does not exercise the wire — the
// wire-level equivalent is covered by the existence of the
// retrievalArgs.DatasetIDs field at internal/agent/tool/retrieval.go
// and the json.Marshal of `merged` in retrievalComponent.Invoke.
func TestRetrieval_KbIDsTranslatedToDatasetIDs(t *testing.T) {
	t.Parallel()

	// Case 1: kb_ids from the build-time node params (c.params.KbIDs)
	// are surfaced as dataset_ids in the merged map.
	c, err := newRetrievalComponent(map[string]any{
		"kb_ids": []any{"kb-1", "kb-2"},
	})
	if err != nil {
		t.Fatalf("newRetrievalComponent: %v", err)
	}
	rc, ok := c.(*retrievalComponent)
	if !ok {
		t.Fatalf("wrapper is %T, want *retrievalComponent", c)
	}
	merged := rc.applyDefaults(map[string]any{"query": "ragflow"})
	if _, hasKB := merged["kb_ids"]; hasKB {
		t.Errorf("kb_ids should be removed from merged map (canonical key is dataset_ids)")
	}
	ds, ok := merged["dataset_ids"].([]any)
	if !ok {
		t.Fatalf("dataset_ids missing or wrong type: %v (%T)", merged["dataset_ids"], merged["dataset_ids"])
	}
	if len(ds) != 2 || ds[0] != "kb-1" || ds[1] != "kb-2" {
		t.Errorf("dataset_ids = %v, want [kb-1 kb-2]", ds)
	}

	// Case 2: kb_ids supplied at call time (inputs map) is also
	// translated.
	merged2 := rc.applyDefaults(map[string]any{
		"query":  "ragflow",
		"kb_ids": []any{"kb-3"},
	})
	if _, hasKB := merged2["kb_ids"]; hasKB {
		t.Errorf("inputs kb_ids should be removed after translation")
	}
	if ds, ok := merged2["dataset_ids"].([]any); !ok || len(ds) != 1 || ds[0] != "kb-3" {
		t.Errorf("inputs dataset_ids = %v, want [kb-3]", merged2["dataset_ids"])
	}

	// Case 3: dataset_ids already present wins over kb_ids (no
	// silent overwrite). Per-call input is the canonical source.
	merged3 := rc.applyDefaults(map[string]any{
		"kb_ids":      []any{"kb-old"},
		"dataset_ids": []any{"kb-new"},
	})
	ds3, ok := merged3["dataset_ids"].([]any)
	if !ok || len(ds3) != 1 || ds3[0] != "kb-new" {
		t.Errorf("dataset_ids should keep call-time value %v, got %v", "kb-new", merged3["dataset_ids"])
	}
}

// TestRetrieval_KbIDsEndToEndThroughTool is the wire-level
// companion to TestRetrieval_KbIDsTranslatedToDatasetIDs: it
// installs the simple retrieval service, builds a wrapper with
// `kb_ids` in the build-time params, and confirms that Invoke
// flows through to the tool and returns non-empty content.
//
// Split from the pure-function test above so the
// SetSimpleRetrievalService / t.Cleanup dance doesn't race with
// the case-1..3 pure-function calls under t.Parallel().
//
// Not t.Parallel(): the global retrievalServiceImpl is shared
// across the package, and several other tests in this file and
// in retrieval_swap_test.go install/restore the simple service
// via t.Cleanup. Running in parallel makes the Cleanup chain
// racy — a peer's Cleanup can restore the stub between this
// test's SetSimpleRetrievalService and its Invoke. Marking this
// test serial avoids the race entirely.
func TestRetrieval_KbIDsEndToEndThroughTool(t *testing.T) {
	prev := agenttool.GetRetrievalService()
	agenttool.SetSimpleRetrievalService()
	t.Cleanup(func() { agenttool.SetRetrievalService(prev) })

	wrapper, err := New(componentNameRetrieval, map[string]any{
		"kb_ids": []any{"kb-1"},
		"top_n":  3,
	})
	if err != nil {
		t.Fatalf("New(Retrieval): %v", err)
	}
	out, err := wrapper.Invoke(context.Background(), map[string]any{"query": "ragflow"})
	if err != nil {
		t.Fatalf("Retrieval.Invoke: %v", err)
	}
	if fc, _ := out["formalized_content"].(string); fc == "" {
		t.Errorf("Retrieval.Invoke returned empty formalized_content (translation regression?)")
	}
}

// TestSearchMyDataset_AllAliasesRegistered pins the
// SearchMyDataset alias surface: the snake_case and Python-typo
// variants must also resolve to the Universe B tool. The Universe
// B tool registry has always
// accepted all three spellings (search_my_dataset /
// search_my_dateset) plus PascalCase; Universe A now mirrors that
// surface so older DSLs don't fail with "unknown component" at
// buildNodeBody time.
//
// All four names must resolve to a registered factory; the
// wrappers' Name() returns the canonical "Retrieval" but the
// alias lookup succeeds regardless of which spelling the DSL used.
func TestSearchMyDataset_AllAliasesRegistered(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"Retrieval",
		"SearchMyDataset",
		"search_my_dataset",
		"search_my_dateset",
	} {
		t.Run(name, func(t *testing.T) {
			c, err := New(name, nil)
			if err != nil {
				t.Fatalf("New(%q) errored: %v", name, err)
			}
			if c == nil {
				t.Fatalf("New(%q) returned nil", name)
			}
			// The canonical name is "Retrieval" — the aliases all
			// resolve to the same wrapper.
			if got := c.Name(); got != componentNameRetrieval {
				t.Errorf("New(%q).Name() = %q, want %q", name, got, componentNameRetrieval)
			}
		})
	}

	// Also assert the registration list (used by introspection
	// tooling) contains every alias.
	have := map[string]bool{}
	for _, n := range RegisteredNames() {
		have[strings.ToLower(n)] = true
	}
	for _, expected := range []string{"retrieval", "searchmydataset", "search_my_dataset", "search_my_dateset"} {
		if !have[expected] {
			t.Errorf("RegisteredNames() missing %q (search-mydataset alias surface regression)", expected)
		}
	}
}

// countingInvoker records every call and returns an error every
// time. Used by TestLLM_RetryStackingSemantics to assert the total
// number of invocations the LLM component makes.
type countingInvoker struct {
	calls int
}

func (c *countingInvoker) Invoke(_ context.Context, _ ChatInvokeRequest) (*ChatInvokeResponse, error) {
	c.calls++
	return nil, errLLMRetryTestAlwaysFail
}

// errLLMRetryTestAlwaysFail is a sentinel so the test can match
// without depending on a specific retryInvoker error string.
var errLLMRetryTestAlwaysFail = &fakeRetryErr{}

type fakeRetryErr struct{}

func (e *fakeRetryErr) Error() string { return "TestLLM_RetryStackingSemantics: forced failure" }

// TestLLM_RetryStackingSemantics pins the multiplicative
// retry stacking semantics (when a future change accidentally
// re-introduces the stacking bug, this test fails).
//
// Contract: when LLMParam.MaxRetries > 0, the LLM component
// re-wraps the default invoker in a fresh retryInvoker with that
// budget. Each "attempt" of the outer retryInvoker is one full
// call to the inner invoker. Because production boots wrap the
// default invoker in another retryInvoker (see
// cmd/server_main.go), the TOTAL invocations for a per-call
// MaxRetries=N setting is (N+1) × (boot_attempts+1) in production.
//
// This test runs against the test default (bare einoChatInvoker
// replaced with a counting invoker), so the test sees exactly
// MaxRetries+1 attempts. If a future change makes the LLM
// component replace the boot retry layer when MaxRetries is
// explicitly set, this test must be updated to match the new
// semantics — that change would itself warrant a new comment
// block in llm.go.
//
// Not t.Parallel(): the package-level defaultChatInvoker is
// shared across tests in this package, and several LLM tests
// (and downstream consumers like agent_test.go) swap it via
// SetDefaultChatInvoker. Serialising this test avoids races
// with peer cleanups.
func TestLLM_RetryStackingSemantics(t *testing.T) {
	counter := &countingInvoker{}
	prev := getDefaultChatInvoker()
	SetDefaultChatInvoker(counter)
	t.Cleanup(func() { SetDefaultChatInvoker(prev) })

	cases := []struct {
		name            string
		maxRetries      int
		delayAfterError int // nanoseconds; 0 → use default backoff
		wantMinAttempts int // minimum total invoker.Invokes expected
	}{
		{"no override → exactly one attempt", 0, 0, 1},
		{"MaxRetries=2 → three attempts", 2, 0, 3},
		{"MaxRetries=5 → six attempts", 5, 0, 6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			counter.calls = 0
			comp := NewLLMComponent(LLMParam{
				ModelID:         "stub-model",
				UserPrompt:      "hi",
				MaxRetries:      tc.maxRetries,
				DelayAfterError: 0, // use the retryInvoker default backoff (small)
			})
			// Use a fast backoff so the test does not block
			// on the default 2s delay.
			if tc.delayAfterError > 0 {
				comp.param.DelayAfterError = 1 // 1ns — effectively instant
			} else if tc.maxRetries > 0 {
				comp.param.DelayAfterError = 1
			}
			// The retry chain always returns errLLMRetryTestAlwaysFail
			// so we can count attempts deterministically.
			_, _ = comp.Invoke(context.Background(), nil)
			if counter.calls < tc.wantMinAttempts {
				t.Errorf("counter.calls = %d, want >= %d (MaxRetries=%d stacking regression?)",
					counter.calls, tc.wantMinAttempts, tc.maxRetries)
			}
			// Without an override, exactly 1 attempt; with an
			// override, MaxRetries+1 attempts. (In tests the
			// inner invoker has no retry chain, so the count
			// is linear, not multiplicative.)
			wantExact := tc.wantMinAttempts
			if tc.maxRetries > 0 {
				wantExact = tc.maxRetries + 1
			}
			if counter.calls != wantExact {
				t.Errorf("counter.calls = %d, want exactly %d (MaxRetries=%d stacking regression?)",
					counter.calls, wantExact, tc.maxRetries)
			}
		})
	}
}
