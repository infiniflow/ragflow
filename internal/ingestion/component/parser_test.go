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
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

// TestParserComponent_Registered asserts the factory lookup
// succeeds for the canonical "Parser" name. This is the contract
// the pipeline runner relies on (see plan §4 Phase 0, "category-
// aware registry"). A regression here would mean the component
// failed to register in init().
func TestParserComponent_Registered(t *testing.T) {
	factory, category, meta, ok := runtime.DefaultRegistry.Lookup("Parser")
	if !ok {
		t.Fatalf("Parser not registered in DefaultRegistry")
	}
	if category != runtime.CategoryIngestion {
		t.Errorf("Parser category = %q, want %q", category, runtime.CategoryIngestion)
	}
	if factory == nil {
		t.Fatalf("Parser factory is nil")
	}
	if len(meta.Inputs) == 0 {
		t.Errorf("Parser Metadata.Inputs is empty")
	}
	if len(meta.Outputs) == 0 {
		t.Errorf("Parser Metadata.Outputs is empty")
	}
}

// TestParserComponent_InputsOutputs_NonEmpty covers the static
// input/output descriptors — the API layer enumerates these to
// build the component catalog, so an empty descriptor would
// hide the component from the UI.
func TestParserComponent_InputsOutputs_NonEmpty(t *testing.T) {
	c := &ParserComponent{}
	in := c.Inputs()
	out := c.Outputs()
	if len(in) == 0 {
		t.Errorf("Inputs() returned empty map")
	}
	if len(out) == 0 {
		t.Errorf("Outputs() returned empty map")
	}
	// The contract from the file header: at least "binary" in,
	// "pages" out. Anything else is informational.
	if _, ok := in["binary"]; !ok {
		t.Errorf("Inputs() missing key %q", "binary")
	}
	if _, ok := out["pages"]; !ok {
		t.Errorf("Outputs() missing key %q", "pages")
	}
}

// TestParserComponent_Parallelism locks the fan-out degree to
// the plan §2 AD-5a value (4). Changing this would silently
// re-tune resource consumption and break the "fan-out" claim.
func TestParserComponent_Parallelism(t *testing.T) {
	c := &ParserComponent{}
	if got := c.Parallelism(); got != 4 {
		t.Errorf("Parallelism() = %d, want 4", got)
	}
}

// TestParserComponent_Invoke_TextInput covers the happy path:
// UTF-8 text input, no form-feeds, default page_size. The
// component must emit exactly one page carrying the full text
// under "text", plus the timing stamps.
func TestParserComponent_Invoke_TextInput(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	out, err := c.Invoke(context.Background(), map[string]any{
		"binary": "hello world",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok {
		t.Fatalf("pages: got %T, want []schema.Page", out["pages"])
	}
	if len(pages) != 1 {
		t.Fatalf("pages len = %d, want 1", len(pages))
	}
	if got := pages[0]["text"]; got != "hello world" {
		t.Errorf("pages[0][text] = %q, want %q", got, "hello world")
	}
	if _, ok := out["_created_time"]; !ok {
		t.Errorf("_created_time missing from output")
	}
	if _, ok := out["_elapsed_time"]; !ok {
		t.Errorf("_elapsed_time missing from output")
	}
}

// TestParserComponent_Invoke_PageRangeFilter asserts that
// form-feed boundaries are honored: "A\fB\fC" yields three
// pages, in input order, with text intact.
func TestParserComponent_Invoke_PageRangeFilter(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	out, err := c.Invoke(context.Background(), map[string]any{
		"binary": "pageA\fpageB\fpageC",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok {
		t.Fatalf("pages: got %T, want []schema.Page", out["pages"])
	}
	if len(pages) != 3 {
		t.Fatalf("pages len = %d, want 3", len(pages))
	}
	want := []string{"pageA", "pageB", "pageC"}
	for i, p := range pages {
		if got := p["text"]; got != want[i] {
			t.Errorf("pages[%d][text] = %q, want %q", i, got, want[i])
		}
	}
}

// TestParserComponent_Invoke_DeterministicMerge is the
// golden-file test for plan §8 R8 (DETERMINISTIC MERGE).
//
// We invoke the component 5 times with identical input and
// assert byte-for-byte equality of the JSON-encoded output.
// The test is expected to pass under `go test -count=10 -race`
// — that flag is run separately in the verification block.
//
// We can't tag-stamp the page text (no page_number key in
// text-page mode) so we tag the input with explicit
// page numbers and verify the SORTED output is stable across
// runs. The fan-out goroutines therefore produce pages in
// different physical orders, but the deterministic sort
// rescues byte-equality.
func TestParserComponent_Invoke_DeterministicMerge(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	// 8 form-feed-separated pages — enough to exercise the
	// ceil(8/4)=2 page_size default and force multiple
	// goroutines to interleave.
	input := "p1\fp2\fp3\fp4\fp5\fp6\fp7\fp8"

	// First call: produce the canonical bytes.
	first, err := c.Invoke(context.Background(), map[string]any{
		"binary": input,
	})
	if err != nil {
		t.Fatalf("Invoke (first): %v", err)
	}
	canonical, err := json.Marshal(first["pages"])
	if err != nil {
		t.Fatalf("Marshal canonical: %v", err)
	}

	// Subsequent calls: must produce the same bytes.
	for i := 0; i < 5; i++ {
		got, err := c.Invoke(context.Background(), map[string]any{
			"binary": input,
		})
		if err != nil {
			t.Fatalf("Invoke (run %d): %v", i, err)
		}
		encoded, err := json.Marshal(got["pages"])
		if err != nil {
			t.Fatalf("Marshal run %d: %v", i, err)
		}
		if string(encoded) != string(canonical) {
			t.Errorf("run %d output differs from canonical:\n got=%s\nwant=%s",
				i, encoded, canonical)
		}
	}
}

// TestParserComponent_Invoke_RespectsTimeout exercises the
// cancellation path. We pass a pre-cancelled parent context
// (no timeout) so the errgroup's WithContext branches see
// ctx.Done() on the first fan-out — the same code path that
// a context.WithTimeout parent takes when its deadline
// elapses. The pre-cancelled form is deterministic across
// hardware speeds; a 1ms deadline raced on slow CI.
//
// The component does NOT expose a "Parallelism" override
// hook (Parallelism() is fixed at 4). Using a pre-cancelled
// parent also verifies that a parent cancellation cascades
// through the errgroup → siblings see ctx.Done() and
// return ctx.Err().
func TestParserComponent_Invoke_RespectsTimeout(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	// Build a 400-page input so the fan-out has more than
	// one batch to dispatch.
	var b strings.Builder
	for i := 0; i < 400; i++ {
		if i > 0 {
			b.WriteByte(pageFormFeed)
		}
		b.WriteString("page")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	_, err := c.Invoke(ctx, map[string]any{
		"binary": b.String(),
	})
	if err == nil {
		t.Fatalf("Invoke returned nil error; expected a cancellation error")
	}
	// The error must reference cancellation — not an
	// unrelated "parse error" — so callers can distinguish
	// "we were cancelled" from "the input was malformed".
	if !isTimeoutish(err) {
		t.Errorf("Invoke error = %v, want a timeout/cancel error", err)
	}
}

// isTimeoutish returns true if err is a context.DeadlineExceeded
// (directly or wrapped) or otherwise carries a "deadline
// exceeded" / "context deadline" / "context canceled" substring.
func isTimeoutish(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	return strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline") ||
		strings.Contains(msg, "context canceled")
}

// TestParserComponent_New_Defaults constructs a Parser from a
// nil param map and verifies the static Param is the
// Defaults() value (i.e., NewParserComponent does not mutate
// the schema default).
func TestParserComponent_New_Defaults(t *testing.T) {
	c, err := NewParserComponent(nil)
	if err != nil {
		t.Fatalf("NewParserComponent(nil): %v", err)
	}
	pc, ok := c.(*ParserComponent)
	if !ok {
		t.Fatalf("NewParserComponent returned %T, want *ParserComponent", c)
	}
	defaults := schema.ParserParam{}.Defaults()
	if !reflect.DeepEqual(pc.Param, defaults) {
		t.Errorf("Param differs from Defaults:\n got=%+v\nwant=%+v", pc.Param, defaults)
	}
}

// TestParserComponent_New_Overrides verifies that a non-nil
// param map with a "setups" entry is layered on top of the
// defaults.
func TestParserComponent_New_Overrides(t *testing.T) {
	c, err := NewParserComponent(map[string]any{
		"setups": map[string]any{
			"text&code": map[string]any{
				"chunk_token_num": 256,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewParserComponent: %v", err)
	}
	pc, ok := c.(*ParserComponent)
	if !ok {
		t.Fatalf("NewParserComponent returned %T", c)
	}
	setup, ok := pc.Param.Setups["text&code"]
	if !ok {
		t.Fatalf("Setups[text&code] missing after override")
	}
	if got, _ := setup["chunk_token_num"].(int); got != 256 {
		t.Errorf("Setups[text&code][chunk_token_num] = %v, want 256", setup["chunk_token_num"])
	}
	// Defaults must still be present for other file types.
	if _, ok := pc.Param.Setups["pdf"]; !ok {
		t.Errorf("Setups[pdf] missing; override should not erase defaults")
	}
}

// TestParserComponent_Invoke_DocIDCarried asserts the optional
// doc_id input flows through to the "name" output.
func TestParserComponent_Invoke_DocIDCarried(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	out, err := c.Invoke(context.Background(), map[string]any{
		"binary": "x",
		"doc_id": "doc-123",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["name"].(string); got != "doc-123" {
		t.Errorf("name = %q, want %q", got, "doc-123")
	}
}

func TestParserComponent_Invoke_ResolvesBinaryFromDocID(t *testing.T) {
	ms := withMemoryStorage(t)
	db := withFileComponentTestDB(t)
	location := "docs/from-parser.txt"
	if err := ms.Put("kb-parser", location, []byte("alpha\fbeta")); err != nil {
		t.Fatalf("seed storage: %v", err)
	}
	docName := "parser.txt"
	if err := db.Create(&entity.Document{
		ID:           "doc-parser",
		KbID:         "kb-parser",
		ParserID:     "na",
		ParserConfig: entity.JSONMap{},
		Type:         "txt",
		CreatedBy:    "u1",
		Name:         &docName,
		Location:     &location,
		Suffix:       ".txt",
	}).Error; err != nil {
		t.Fatalf("seed doc: %v", err)
	}

	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	out, err := c.Invoke(context.Background(), map[string]any{"doc_id": "doc-parser"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok || len(pages) != 2 {
		t.Fatalf("pages = %T/%v, want 2 schema.Page entries", out["pages"], out["pages"])
	}
	if pages[0]["text"] != "alpha" || pages[1]["text"] != "beta" {
		t.Fatalf("pages = %+v, want [alpha beta]", pages)
	}
	if got, _ := out["name"].(string); got != "doc-parser" {
		t.Fatalf("name = %q, want %q", got, "doc-parser")
	}
}

func TestParserComponent_Invoke_ResolvesBinaryFromBucketPath(t *testing.T) {
	ms := withMemoryStorage(t)
	if err := ms.Put("bucket-1", "docs/explicit.txt", []byte("bucket content")); err != nil {
		t.Fatalf("seed storage: %v", err)
	}

	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	out, err := c.Invoke(context.Background(), map[string]any{
		"bucket": "bucket-1",
		"path":   "docs/explicit.txt",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok || len(pages) != 1 {
		t.Fatalf("pages = %T/%v, want 1 schema.Page entry", out["pages"], out["pages"])
	}
	if got := pages[0]["text"]; got != "bucket content" {
		t.Fatalf("pages[0][text] = %q, want %q", got, "bucket content")
	}
}

// TestParserComponent_Invoke_RejectsInvalidUTF8 covers the
// safety check: a "binary" string that is not valid UTF-8 is
// rejected (per the file header — base64-encoded input would
// look like this if a caller mistakenly handed a base64 string
// without decoding it).
func TestParserComponent_Invoke_RejectsInvalidUTF8(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	_, err := c.Invoke(context.Background(), map[string]any{
		// 0xFF alone is not valid UTF-8 start byte.
		"binary": string([]byte{0xFF, 0xFE, 0xFD}),
	})
	if err == nil {
		t.Fatalf("Invoke: expected an error for invalid UTF-8, got nil")
	}
	if !strings.Contains(err.Error(), "UTF-8") {
		t.Errorf("error = %v, want it to mention UTF-8", err)
	}
}

// TestParserComponent_Invoke_AcceptsBytes covers the in-process
// caller's normal form ([]byte) — the alternative to a UTF-8
// string.
func TestParserComponent_Invoke_AcceptsBytes(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	out, err := c.Invoke(context.Background(), map[string]any{
		"binary": []byte("alpha\fbeta"),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok {
		t.Fatalf("pages: got %T", out["pages"])
	}
	if len(pages) != 2 {
		t.Fatalf("pages len = %d, want 2", len(pages))
	}
	if pages[0]["text"] != "alpha" || pages[1]["text"] != "beta" {
		t.Errorf("pages = %+v, want [alpha beta]", pages)
	}
}

// TestParserComponent_Invoke_PageSizeHint covers the optional
// page_size input. A page_size of 2 with 6 pages yields 3
// batches; the deterministic merge still yields 6 pages in
// input order.
func TestParserComponent_Invoke_PageSizeHint(t *testing.T) {
	c := &ParserComponent{Param: schema.ParserParam{}.Defaults()}
	out, err := c.Invoke(context.Background(), map[string]any{
		"binary":    "p1\fp2\fp3\fp4\fp5\fp6",
		"page_size": 2,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	pages, ok := out["pages"].([]schema.Page)
	if !ok {
		t.Fatalf("pages: got %T", out["pages"])
	}
	if len(pages) != 6 {
		t.Fatalf("pages len = %d, want 6", len(pages))
	}
	for i, p := range pages {
		want := "p" + string(rune('1'+i))
		if got := p["text"]; got != want {
			t.Errorf("pages[%d] = %q, want %q", i, got, want)
		}
	}
}

// TestParseBatch_IsFormatAgnostic pins the batching contract:
// parseBatch does not resolve parsers or inspect file families. It
// only wraps already-prepared page bytes into schema.Page items.
func TestParseBatch_IsFormatAgnostic(t *testing.T) {
	got, err := parseBatch(context.Background(), [][]byte{
		[]byte("first page from dispatch"),
		[]byte("<table>second page from html dispatch</table>"),
	})
	if err != nil {
		t.Fatalf("parseBatch: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0]["text"] != "first page from dispatch" {
		t.Fatalf("got[0][text] = %v, want first page from dispatch", got[0]["text"])
	}
	if got[1]["text"] != "<table>second page from html dispatch</table>" {
		t.Fatalf("got[1][text] = %v, want HTML payload preserved verbatim", got[1]["text"])
	}
	for i := range got {
		if got[i]["doc_type_kwd"] != "text" {
			t.Fatalf("got[%d][doc_type_kwd] = %v, want text", i, got[i]["doc_type_kwd"])
		}
	}
}
