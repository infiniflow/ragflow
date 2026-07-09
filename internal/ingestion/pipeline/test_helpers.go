//	Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//	Licensed under the Apache License, Version 2.0 (the "License");
//	you may not use this file except in compliance with the License.
//	You may obtain a copy of the License at
//
//	    http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS,
//	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	See the License for the specific language governing permissions and
//	limitations under the License.

package pipeline

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/tokenizer"
)

func repoRootFromPipelineTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := goruntime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func RequireTokenizerPool(t *testing.T) {
	t.Helper()
	if tokenizer.IsInitialized() {
		return
	}
	cfg := &tokenizer.PoolConfig{
		DictPath:       os.Getenv("RAGFLOW_DICT_PATH"),
		MinSize:        1,
		MaxSize:        2,
		IdleTimeout:    30 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}
	if cfg.DictPath == "" {
		cfg.DictPath = "/usr/share/infinity/resource"
	}
	if err := tokenizer.Init(cfg); err != nil {
		t.Skipf("tokenizer pool init failed: %v", err)
	}
}

func terminalComponentIDsFromTemplate(t *testing.T, raw []byte) []string {
	t.Helper()
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	dsl, ok := tpl["dsl"].(map[string]any)
	if !ok {
		t.Fatalf("template dsl = %T, want map[string]any", tpl["dsl"])
	}
	components, ok := dsl["components"].(map[string]any)
	if !ok {
		t.Fatalf("template components = %T, want map[string]any", dsl["components"])
	}
	var terminals []string
	for id, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			t.Fatalf("component %q = %T, want map[string]any", id, rawComp)
		}
		switch ds := comp["downstream"].(type) {
		case nil:
			terminals = append(terminals, id)
		case []any:
			if len(ds) == 0 {
				terminals = append(terminals, id)
			}
		case []string:
			if len(ds) == 0 {
				terminals = append(terminals, id)
			}
		default:
			t.Fatalf("component %q downstream = %T, want []any/[]string/nil", id, comp["downstream"])
		}
	}
	sort.Strings(terminals)
	return terminals
}

func terminalPayloadFromRunOutput(t *testing.T, out map[string]any, terminalID string) map[string]any {
	t.Helper()
	if out == nil {
		t.Fatal("Run returned nil output")
	}
	if _, ok := out["output_format"]; ok {
		return out
	}
	if terminalID == "" {
		t.Fatal("terminalID is empty")
	}
	nested, ok := out[terminalID].(map[string]any)
	if !ok {
		t.Fatalf("run output missing terminal payload %q in %v", terminalID, out)
	}
	return nested
}

func stateFromRunOutput(t *testing.T, out map[string]any) map[string]map[string]any {
	t.Helper()
	state, ok := out["state"].(map[string]map[string]any)
	if !ok {
		t.Fatalf("state = %T, want map[string]map[string]any", out["state"])
	}
	return state
}

func floatSliceFromAny(t *testing.T, v any) []float64 {
	t.Helper()
	switch x := v.(type) {
	case []float64:
		return x
	case []any:
		out := make([]float64, 0, len(x))
		for i, item := range x {
			f, ok := item.(float64)
			if !ok {
				t.Fatalf("vector item %d = %T, want float64", i, item)
			}
			out = append(out, f)
		}
		return out
	default:
		t.Fatalf("vector = %T, want []float64 or []any", v)
		return nil
	}
}

func joinJSONItemTexts(items []map[string]any) string {
	var parts []string
	for _, item := range items {
		text, _ := item["text"].(string)
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func assertNormalizedContainsAll(t *testing.T, got string, wantSubstrings ...string) {
	t.Helper()
	normalizedGot := normalizeTestText(got)
	for _, want := range wantSubstrings {
		if !strings.Contains(normalizedGot, normalizeTestText(want)) {
			t.Fatalf("normalized text %q does not contain %q", normalizedGot, normalizeTestText(want))
		}
	}
}

func normalizeTestText(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "­", "")), " ")
}

func templateUsesComponent(t *testing.T, raw []byte, componentName string) bool {
	t.Helper()
	var tpl map[string]any
	if err := json.Unmarshal(raw, &tpl); err != nil {
		t.Fatalf("unmarshal template: %v", err)
	}
	dsl, ok := tpl["dsl"].(map[string]any)
	if !ok {
		t.Fatalf("template dsl = %T, want map[string]any", tpl["dsl"])
	}
	components, ok := dsl["components"].(map[string]any)
	if !ok {
		t.Fatalf("template components = %T, want map[string]any", dsl["components"])
	}
	for id, rawComp := range components {
		comp, ok := rawComp.(map[string]any)
		if !ok {
			t.Fatalf("component %q = %T, want map[string]any", id, rawComp)
		}
		obj, ok := comp["obj"].(map[string]any)
		if !ok {
			t.Fatalf("component %q obj = %T, want map[string]any", id, comp["obj"])
		}
		name, _ := obj["component_name"].(string)
		if name == componentName {
			return true
		}
	}
	return false
}

func expectedFixedEmbedderFirst(name, text string) float64 {
	return 0.1*float64(len(name)) + 0.9*float64(len(text))
}

func approxFloat(got, want float64) bool {
	return math.Abs(got-want) < 1e-9
}
