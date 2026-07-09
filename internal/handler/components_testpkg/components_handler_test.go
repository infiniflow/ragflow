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

// Phase 4 of plan port-rag-flow-pipeline-to-go.md — integration
// tests for GET /api/v1/components.
//
// The tests import the agent and ingestion component packages via
// blank imports so their init() functions register every factory
// into runtime.DefaultRegistry. The tests then assert that the
// HTTP handler projects that registry into the expected
// JSON-shaped response.
//
// Counters are derived from runtime.DefaultRegistry.Names() /
// NamesByCategory() at test time rather than hardcoded constants —
// the registry is the source of truth, so any future registration
// change just moves the expected values with it.
//
// This file lives in its own subpackage (components_testpkg) so
// its test binary does not co-compile with the rest of the
// internal/handler tests, whose build is currently broken on the
// agent-go-port branch (pre-existing
// `interface{}` vs `string` mismatch in chat_test.go /
// searchbot_test.go). When the upstream build is fixed this
// package can be folded back into the main handler test binary.
package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/gin-gonic/gin"

	_ "ragflow/internal/agent/component" // registers agent components
	"ragflow/internal/agent/runtime"
	"ragflow/internal/handler"
	_ "ragflow/internal/ingestion/component"         // registers ingestion main components
	_ "ragflow/internal/ingestion/component/chunker" // registers 4 chunker variants
	"ragflow/internal/service"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newComponentsTestRig mounts a Gin engine with the
// /api/v1/components route registered against a fresh
// ComponentsService (which reads runtime.DefaultRegistry).
func newComponentsTestRig(t *testing.T) *gin.Engine {
	t.Helper()
	h := handler.NewComponentsHandler(service.NewComponentsService())
	eng := gin.New()
	v1 := eng.Group("/api/v1")
	v1.GET("/components", h.Get)
	return eng
}

// doRequest issues a GET against the test rig and returns the
// response recorder.
func doRequest(t *testing.T, eng *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	eng.ServeHTTP(w, req)
	return w
}

// decodeEnvelope parses the standard {code, message, data} response
// envelope used by every handler in this codebase.
func decodeEnvelope(t *testing.T, body []byte) (code int, message string, data []service.ComponentDescriptor) {
	t.Helper()
	var env struct {
		Code    int                           `json:"code"`
		Message string                        `json:"message"`
		Data    []service.ComponentDescriptor `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v; body=%s", err, string(body))
	}
	return env.Code, env.Message, env.Data
}

// TestComponentsHandler_NoFilter verifies that GET /api/v1/components
// (no filter) returns every registered component, across all
// categories. The total must equal runtime.DefaultRegistry.Names().
func TestComponentsHandler_NoFilter(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	gotCode, msg, data := decodeEnvelope(t, w.Body.Bytes())
	if gotCode != 0 {
		t.Errorf("envelope code = %d, want 0; message=%s", gotCode, msg)
	}
	if msg != "success" {
		t.Errorf("envelope message = %q, want %q", msg, "success")
	}

	wantTotal := len(runtime.DefaultRegistry.Names())
	if len(data) != wantTotal {
		t.Errorf("got %d components, want %d (== Names())", len(data), wantTotal)
	}

	// Spot-check that the default registry includes agent and
	// ingestion components when no filter is applied.
	seen := map[string]bool{}
	for _, d := range data {
		seen[d.Category] = true
	}
	for _, cat := range []string{"agent", "ingestion"} {
		if !seen[cat] {
			t.Errorf("expected category %q in unfiltered response; got categories %v", cat, seen)
		}
	}
}

// TestComponentsHandler_FilterIngestion verifies the
// ?category=ingestion filter returns the 8 ingestion components
// (Extractor, File, Parser, Tokenizer + 4 chunker variants). Names
// must be sorted ascending (plan §4 task 1 stable output).
func TestComponentsHandler_FilterIngestion(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components?category=ingestion")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	_, _, data := decodeEnvelope(t, w.Body.Bytes())

	wantNames := []string{
		"extractor", "file", "grouptitlechunker", "hierarchytitlechunker",
		"parser", "titlechunker", "tokenchunker", "tokenizer",
	}
	assertNameSet(t, "ingestion", data, wantNames)

	for _, d := range data {
		if d.Category != "ingestion" {
			t.Errorf("filtered component %q has category %q, want %q", d.Name, d.Category, "ingestion")
		}
	}
}

// TestComponentsHandler_FilterMultiple verifies comma-separated
// category values work. The shared category may currently be empty,
// so ?category=ingestion,shared must still return the ingestion set.
func TestComponentsHandler_FilterMultiple(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components?category=ingestion,shared")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	_, _, data := decodeEnvelope(t, w.Body.Bytes())

	wantNames := []string{
		"extractor", "file", "grouptitlechunker", "hierarchytitlechunker",
		"parser", "titlechunker", "tokenchunker", "tokenizer",
	}
	assertNameSet(t, "ingestion,shared", data, wantNames)
}

// TestComponentsHandler_FilterAgent verifies the
// ?category=agent filter returns every agent component. The count
// is derived from NamesByCategory(CategoryAgent) so any future
// registration change just moves the value.
func TestComponentsHandler_FilterAgent(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components?category=agent")

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	_, _, data := decodeEnvelope(t, w.Body.Bytes())

	wantNames := runtime.DefaultRegistry.NamesByCategory(runtime.CategoryAgent)
	if len(wantNames) == 0 {
		t.Fatal("no agent components registered; cannot run this test")
	}
	assertNameSet(t, "agent", data, wantNames)
	for _, d := range data {
		if d.Category != "agent" {
			t.Errorf("agent-filtered component %q has category %q", d.Name, d.Category)
		}
	}
}

// TestComponentsHandler_InvalidCategory verifies an unknown category
// yields HTTP 400 with the standard error envelope. The message
// must mention the offending token.
func TestComponentsHandler_InvalidCategory(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components?category=foo")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v; body=%s", err, w.Body.String())
	}
	if env.Code != 400 {
		t.Errorf("envelope code = %d, want 400", env.Code)
	}
	if env.Message == "" {
		t.Errorf("envelope message is empty; body=%s", w.Body.String())
	}
}

// TestComponentsHandler_ComponentShape verifies every descriptor
// has a non-empty name, a non-empty category from the known set,
// and non-nil inputs / outputs maps (the service normalises nil
// to empty maps so the JSON payload never has null fields).
func TestComponentsHandler_ComponentShape(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	_, _, data := decodeEnvelope(t, w.Body.Bytes())

	allowed := map[string]bool{"agent": true, "ingestion": true, "shared": true}
	for _, d := range data {
		if d.Name == "" {
			t.Errorf("descriptor has empty name: %+v", d)
		}
		if d.Category == "" {
			t.Errorf("descriptor %q has empty category", d.Name)
		}
		if !allowed[d.Category] {
			t.Errorf("descriptor %q has unknown category %q", d.Name, d.Category)
		}
		if d.Inputs == nil {
			t.Errorf("descriptor %q has nil inputs; service must normalise to empty map", d.Name)
		}
		if d.Outputs == nil {
			t.Errorf("descriptor %q has nil outputs; service must normalise to empty map", d.Name)
		}
	}
}

// TestComponentsHandler_CaseInsensitive verifies the
// category filter accepts mixed case (plan §4 task 1 spec:
// "case-insensitive"). The response must match the all-lowercase
// variant.
func TestComponentsHandler_CaseInsensitive(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components?category=INGESTION")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	_, _, data := decodeEnvelope(t, w.Body.Bytes())
	wantNames := []string{
		"extractor", "file", "grouptitlechunker", "hierarchytitlechunker",
		"parser", "titlechunker", "tokenchunker", "tokenizer",
	}
	assertNameSet(t, "INGESTION (case-folded)", data, wantNames)
}

// TestComponentsHandler_EmptyCategory treats the bare
// ?category= (no value) as a no-op filter — same payload as the
// unfiltered endpoint. Whitespace-only values are dropped.
func TestComponentsHandler_EmptyCategory(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components?category=")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	_, _, data := decodeEnvelope(t, w.Body.Bytes())
	if len(data) != len(runtime.DefaultRegistry.Names()) {
		t.Errorf("empty category filter returned %d, want %d (== Names())", len(data), len(runtime.DefaultRegistry.Names()))
	}
}

// TestComponentsHandler_RouteMounted verifies the route is
// actually mounted at /api/v1/components. (Catches an accidental
// router wiring regression even when no other test fires.)
func TestComponentsHandler_RouteMounted(t *testing.T) {
	eng := newComponentsTestRig(t)
	w := doRequest(t, eng, "/api/v1/components")
	if w.Code == http.StatusNotFound {
		t.Fatalf("route not mounted; got 404; body=%s", w.Body.String())
	}
}

// assertNameSet checks that the descriptors returned by the
// handler contain exactly the names in want (set comparison;
// ignores ordering — the handler also sorts by name, but the
// comparison is order-independent to make test failures easier
// to read).
func assertNameSet(t *testing.T, label string, got []service.ComponentDescriptor, want []string) {
	t.Helper()
	gotNames := make([]string, 0, len(got))
	for _, d := range got {
		gotNames = append(gotNames, d.Name)
	}
	sort.Strings(gotNames)
	wantSorted := append([]string(nil), want...)
	sort.Strings(wantSorted)

	if len(gotNames) != len(wantSorted) {
		t.Errorf("%s: got %d names (%v), want %d (%v)", label, len(gotNames), gotNames, len(wantSorted), wantSorted)
		return
	}
	for i := range wantSorted {
		if gotNames[i] != wantSorted[i] {
			t.Errorf("%s: name[%d] = %q, want %q (full got=%v want=%v)", label, i, gotNames[i], wantSorted[i], gotNames, wantSorted)
		}
	}
}
