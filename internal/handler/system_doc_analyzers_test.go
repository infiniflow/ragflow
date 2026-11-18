// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !ee

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

// writeDeepDocWeights materializes every file listed in
// common.DeepDocModelFiles inside dir so HasModelFiles reports true.
func writeDeepDocWeights(t *testing.T, dir string) {
	t.Helper()
	for _, name := range common.DeepDocModelFiles {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func findAnalyzer(list []DocAnalyzerInfo, id string) (DocAnalyzerInfo, bool) {
	for _, a := range list {
		if a.ID == id {
			return a, true
		}
	}
	return DocAnalyzerInfo{}, false
}

func TestDocAnalyzers_BuiltinAvailableWhenWeightsPresent(t *testing.T) {
	dir := t.TempDir()
	writeDeepDocWeights(t, dir)
	t.Setenv(common.EnvDeepDocModelDir, dir)

	list := docAnalyzers(context.Background())
	got, ok := findAnalyzer(list, "deepdoc")
	if !ok {
		t.Fatalf("expected a deepdoc analyzer entry, got %+v", list)
	}
	if got.Kind != "builtin" {
		t.Errorf("deepdoc kind = %q, want builtin", got.Kind)
	}
	if !got.Available {
		t.Errorf("deepdoc should be available when %s holds the weights", common.EnvDeepDocModelDir)
	}
}

func TestDocAnalyzers_BuiltinUnavailableWhenWeightsMissing(t *testing.T) {
	dir := t.TempDir() // empty: no model files
	t.Setenv(common.EnvDeepDocModelDir, dir)

	list := docAnalyzers(context.Background())
	got, ok := findAnalyzer(list, "deepdoc")
	if !ok {
		t.Fatalf("expected a deepdoc analyzer entry, got %+v", list)
	}
	if got.Available {
		t.Errorf("deepdoc must be unavailable when %s lacks the weights", common.EnvDeepDocModelDir)
	}
}

func TestSystemHandlerDocAnalyzers_ReturnsProbedList(t *testing.T) {
	dir := t.TempDir()
	writeDeepDocWeights(t, dir)
	t.Setenv(common.EnvDeepDocModelDir, dir)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/system/doc-analyzers", nil)
	NewSystemHandler(service.NewSystemService()).DocAnalyzers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d, body %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int               `json:"code"`
		Data []DocAnalyzerInfo `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d, want 0", resp.Code)
	}
	got, ok := findAnalyzer(resp.Data, "deepdoc")
	if !ok || !got.Available {
		t.Fatalf("expected available deepdoc in response data, got %+v", resp.Data)
	}
}

// TestDocAnalyzers_ResponseHasNoURL pins that the endpoint never leaks the
// internal backend address to clients. The probed URL is intentionally
// excluded from DocAnalyzerInfo, so no "url" key may appear in the wire
// JSON.
func TestDocAnalyzers_ResponseHasNoURL(t *testing.T) {
	dir := t.TempDir()
	writeDeepDocWeights(t, dir)
	t.Setenv(common.EnvDeepDocModelDir, dir)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/system/doc-analyzers", nil)
	NewSystemHandler(service.NewSystemService()).DocAnalyzers(c)

	if w.Code != http.StatusOK {
		t.Fatalf("HTTP %d, body %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatalf("response missing data array: %+v", resp)
	}
	for _, e := range data {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if _, has := entry["url"]; has {
			t.Fatalf("response entry exposes a url field: %+v", entry)
		}
	}
}

// TestDocAnalyzers_ProbesFreshWithoutCache pins the desired
// post-removal behavior: the availability probe must reflect the current
// DEEPDOC_MODEL_DIR on every call, with no package-global memoization. It
// is the adversarial counterpart to the old cached path -- under the cache,
// switching the env within the TTL returns a stale Available:true.
func TestDocAnalyzers_ProbesFreshWithoutCache(t *testing.T) {
	dirA := t.TempDir()
	writeDeepDocWeights(t, dirA)
	t.Setenv(common.EnvDeepDocModelDir, dirA)
	a := docAnalyzers(context.Background())
	gotA, ok := findAnalyzer(a, "deepdoc")
	if !ok || !gotA.Available {
		t.Fatalf("want deepdoc available with weights, got %+v", a)
	}

	dirB := t.TempDir()
	t.Setenv(common.EnvDeepDocModelDir, dirB)
	b := docAnalyzers(context.Background())
	gotB, ok := findAnalyzer(b, "deepdoc")
	if !ok {
		t.Fatalf("want deepdoc entry present, got %+v", b)
	}
	if gotB.Available {
		t.Fatalf("deepdoc must reflect current env immediately; got stale Available:true")
	}
}

// TestDocAnalyzers_ReportsOnlyInProcessBackend pins the provider-hook contract
// in this build: the hook contributes no backends, so the availability list
// contains exactly the in-process entry. A stale or over-eager hook would
// expose analyzers this deployment cannot serve.
func TestDocAnalyzers_ReportsOnlyInProcessBackend(t *testing.T) {
	list := docAnalyzers(context.Background())
	if len(list) != 1 {
		t.Fatalf("docAnalyzers returned %d entries, want exactly 1: %+v", len(list), list)
	}
	if list[0].ID != "deepdoc" || list[0].Kind != "builtin" {
		t.Fatalf("entry = %+v, want {ID:deepdoc Kind:builtin}", list[0])
	}
}
