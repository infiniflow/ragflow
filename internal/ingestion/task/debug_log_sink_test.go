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

package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/pipeline"

	"gorm.io/gorm"
)

// capturedStore is a fake DebugLogStore that records every write so tests can
// assert the exact JSON the sink would persist to Redis — no live Redis needed.
type capturedStore struct {
	mu   sync.Mutex
	data map[string]string
}

func (c *capturedStore) Set(ctx context.Context, key, value string, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string]string{}
	}
	c.data[key] = value
	return true
}

func (c *capturedStore) Get(ctx context.Context, key string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.data[key]
}

// clientConsidersComplete mirrors the front-end completion predicate in
// web/src/pages/agent/hooks/use-fetch-pipeline-log.ts: the run is done only
// when the LAST array element has component_id == "END" and its first trace
// message is non-empty.
func clientConsidersComplete(arr []map[string]any) bool {
	if len(arr) == 0 {
		return false
	}
	last := arr[len(arr)-1]
	if last["component_id"] != "END" {
		return false
	}
	trace, ok := last["trace"].([]any)
	if !ok || len(trace) == 0 {
		return false
	}
	first, ok := trace[0].(map[string]any)
	if !ok {
		return false
	}
	msg, _ := first["message"].(string)
	return strings.TrimSpace(msg) != ""
}

func loadArray(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var arr []map[string]any
	if err := json.Unmarshal([]byte(raw), &arr); err != nil {
		t.Fatalf("stored value is not a JSON array: %v body=%s", err, raw)
	}
	return arr
}

// TestDebugLogSink_RecordsTraceAndEndMarker covers the core shape contract:
// same-component events merge into one element's trace, a distinct component
// opens a new element, and Flush appends an END marker so the front-end can
// detect completion.
func TestDebugLogSink_RecordsTraceAndEndMarker(t *testing.T) {
	store := &capturedStore{}
	sink := NewDebugLogSink("c1", "m1", store)
	ctx := t.Context()

	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "File", Message: "File Started", Phase: phaseEnter,
	})
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "File", Message: "File Done", Phase: phaseExit,
	})
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "Chunker", Message: "Chunker Done", Phase: phaseExit,
	})

	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c1-m1-logs")
	if raw == "" {
		t.Fatalf("expected key c1-m1-logs to be written")
	}
	arr := loadArray(t, raw)
	// File (merged, 2 trace entries) + Chunker + END = 3 elements.
	if len(arr) != 3 {
		t.Fatalf("want 3 elements, got %d: %s", len(arr), raw)
	}
	if arr[0]["component_id"] != "File" {
		t.Errorf("elem0.component_id=%v want File", arr[0]["component_id"])
	}
	fileTrace, _ := arr[0]["trace"].([]any)
	if len(fileTrace) != 2 {
		t.Errorf("File trace should merge 2 entries, got %d: %s", len(fileTrace), raw)
	}
	if arr[1]["component_id"] != "Chunker" {
		t.Errorf("elem1.component_id=%v want Chunker", arr[1]["component_id"])
	}
	if arr[2]["component_id"] != "END" {
		t.Errorf("last component_id=%v want END", arr[2]["component_id"])
	}
	endTrace, _ := arr[2]["trace"].([]any)
	endFirst, _ := endTrace[0].(map[string]any)
	if strings.TrimSpace(endFirst["message"].(string)) == "" {
		t.Errorf("END message must be non-empty for completion signal: %s", raw)
	}
	if !clientConsidersComplete(arr) {
		t.Errorf("client would NOT consider run complete: %s", raw)
	}
}

// TestDebugLogSink_ErrorPrefix verifies that error-phase events get a [ERROR]
// prefix (so the front-end timeline renders them red) and that Flush with a
// non-nil error yields an END marker carrying the error, keeping the run
// detectable as finished.
func TestDebugLogSink_ErrorPrefix(t *testing.T) {
	store := &capturedStore{}
	sink := NewDebugLogSink("c1", "m1", store)
	ctx := t.Context()

	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "Tokenizer", Message: "Tokenizer: boom", Phase: phaseError,
	})
	sink.Flush(ctx, errors.New("boom"))

	raw := store.Get(ctx, "c1-m1-logs")
	arr := loadArray(t, raw)

	// First element is the errored component; its message must carry [ERROR].
	erroredTrace, _ := arr[0]["trace"].([]any)
	erroredFirst, _ := erroredTrace[0].(map[string]any)
	if !strings.HasPrefix(erroredFirst["message"].(string), "[ERROR] ") {
		t.Errorf("error trace message should be prefixed [ERROR] , got %q", erroredFirst["message"])
	}

	// Last element is END with the error surfaced, so the run still completes.
	lastTrace, _ := arr[len(arr)-1]["trace"].([]any)
	lastFirst, _ := lastTrace[0].(map[string]any)
	if !strings.HasPrefix(lastFirst["message"].(string), "[ERROR] ") {
		t.Errorf("END message on failure should be prefixed [ERROR] , got %q", lastFirst["message"])
	}
	if !clientConsidersComplete(arr) {
		t.Errorf("failed run must still be detectable as complete: %s", raw)
	}
}

// TestDebugLogSink_OnComponentTotalIsNoOp ensures the denominator callback
// (used by the DB-backed sink for progress aggregation) is a safe no-op here:
// the debug log array shape does not use it.
func TestDebugLogSink_OnComponentTotalIsNoOp(t *testing.T) {
	store := &capturedStore{}
	ctx := t.Context()

	sink := NewDebugLogSink("c1", "m1", store)
	// Must not panic and must not write.
	sink.OnComponentTotal(ctx, "task-1", 5)
	if len(store.data) != 0 {
		t.Errorf("OnComponentTotal must not write to the store, wrote %v", store.data)
	}
}

// TestPipelineExecutor_DebugRunWritesLogViaSink drives the real
// PipelineExecutor debug branch (KB.ID == "") with an injected fake pipeline
// that emits progress through the sink. It proves the end-to-end wiring inside
// the executor: a ProgressSink attached via WithProgressSink is forwarded to the
// pipeline and its events land in the persisted debug-log array (with the END
// marker), without touching the DB/index persist path.
func TestPipelineExecutor_DebugRunWritesLogViaSink(t *testing.T) {
	ctx := t.Context()
	taskCtx := NewDebugTaskContext("tenant-1", "c1", "doc.pdf", nil)
	exec, err := NewPipelineExecutor(taskCtx, "c1", 0)
	if err != nil {
		t.Fatalf("NewPipelineExecutor: %v", err)
	}

	store := &capturedStore{}
	sink := NewDebugLogSink("c1", "m1", store)
	exec.WithProgressSink(sink)

	// Fake the canvas/DSL boundary: loadDSLFunc returns a dummy DSL and
	// runPipelineFunc emits progress through the (now attached) sink instead of
	// running a real pipeline.
	exec.loadDSLFunc = func(_ context.Context, _ string) (string, string, error) {
		return "dummy-dsl", "", nil
	}
	exec.runPipelineFunc = func(ctx context.Context, _ string) (map[string]any, string, error) {
		exec.progressSink.OnComponentProgress(ctx, pipeline.ProgressEvent{
			Component: "File", Message: "File Done", Phase: phaseExit,
		})
		exec.progressSink.OnComponentProgress(ctx, pipeline.ProgressEvent{
			Component: "Chunker", Message: "Chunker Done", Phase: phaseExit,
		})
		return map[string]any{}, "", nil
	}

	if _, err := exec.Execute(ctx); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// Mirrors runCanvasPipelineDebug: flush (incl. END marker) after the run.
	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c1-m1-logs")
	if raw == "" {
		t.Fatalf("expected debug log written to c1-m1-logs")
	}
	arr := loadArray(t, raw)
	if arr[len(arr)-1]["component_id"] != "END" {
		t.Errorf("last element must be END marker, got %s", raw)
	}
	if !clientConsidersComplete(arr) {
		t.Errorf("client would not consider run complete: %s", raw)
	}
}

// traceStubComponent is a no-op runtime.Component used to exercise the REAL
// pipeline → TrackProgress → DebugLogSink wiring (the enter→exit pairing) without
// a live DB or any real ingestion work.
type traceStubComponent struct{}

func (traceStubComponent) Invoke(_ context.Context, _ *gorm.DB, _ map[string]any) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

// traceChunkComponent emits a real `chunks` output so the end-to-end wiring can
// be asserted to populate the END-marker DSL's params.outputs — the exact
// contract the front-end "View result" needs and the one the nested-state bug
// broke (empty Result tabs).
type traceChunkComponent struct{}

func (traceChunkComponent) Invoke(_ context.Context, _ *gorm.DB, _ map[string]any) (map[string]any, error) {
	// Use the real component output shape ([]map[string]any as produced by
	// ChunkDocsToMaps) so the end-to-end strip/recurse path for []map[string]any
	// is actually exercised — a []any stub hides the deepCopy type gap.
	return map[string]any{
		"chunks": []map[string]any{
			{"text": "real chunk", "vector": []float64{0.5}},
		},
	}, nil
}

// TestDebugLogSink_RealPipeline_EachComponentTraceHasStartedAndDone drives a
// REAL pipeline (not the fake runPipelineFunc used elsewhere) through the same
// wiring a dataflow debug run uses: NewPipelineFromDSL + WithProgressSink
// (DebugLogSink) + Run. The canvas framework wraps each component in
// realComponentBody → runtime.TrackProgress, which MUST emit BOTH PhaseEnter
// ("<comp> Started", progress 0) and PhaseExit ("<comp> Done", progress 1) on
// success.
//
// This locks the contract the front-end depends on: every component ends with a
// Done line, not just a Started line. It is the regression guard for the
// observation that a debug run's polled log appeared to stay at progress 0 —
// per the code, each component's trace must contain [Started, Done].
func TestDebugLogSink_RealPipeline_EachComponentTraceHasStartedAndDone(t *testing.T) {
	ctx := t.Context()
	const (
		compA = "trace.RealStubA"
		compB = "trace.RealStubB"
	)
	runtime.MustRegister(compA, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceStubComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})
	runtime.MustRegister(compB, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceStubComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})

	store := &capturedStore{}
	sink := NewDebugLogSink("c-trace", "m-trace", store)

	dsl := []byte(`{"dsl":{"components":{
		"begin":{"obj":{"component_name":"Begin","params":{}},"downstream":["a"]},
		"a":{"obj":{"component_name":"` + compA + `","params":{}},"upstream":["begin"],"downstream":["b"]},
		"b":{"obj":{"component_name":"` + compB + `","params":{}},"upstream":["a"]}
	},"path":["begin","a","b"],"graph":{"nodes":[]}}}`)

	pipe, err := pipeline.NewPipelineFromDSL(dsl, "task-trace",
		pipeline.WithProgressSink(sink), pipeline.WithDocumentID("doc-trace"))
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	if _, err = pipe.Run(ctx, map[string]any{"name": "doc-trace"}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}
	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c-trace-m-trace-logs")
	if raw == "" {
		t.Fatalf("expected debug log written to c-trace-m-trace-logs")
	}
	arr := loadArray(t, raw)

	// Completion predicate must hold: END is last with a non-empty message.
	if arr[len(arr)-1]["component_id"] != "END" {
		t.Fatalf("last element must be END marker: %s", raw)
	}
	if !clientConsidersComplete(arr) {
		t.Fatalf("client would not consider run complete: %s", raw)
	}

	// Every real component (a, b) must show BOTH Started (progress 0) and Done
	// (progress 1) within its own trace — proving TrackProgress emitted the full
	// enter→exit lifecycle, not just enter.
	for _, comp := range []string{"a", "b"} {
		var gotStart, gotDone bool
		for _, el := range arr {
			if el["component_id"] != comp {
				continue
			}
			traces, _ := el["trace"].([]any)
			for _, tr := range traces {
				tm, _ := tr.(map[string]any)
				if tm == nil {
					continue
				}
				prog, _ := tm["progress"].(float64)
				msg, _ := tm["message"].(string)
				switch {
				case prog == 0 && msg == comp+" Started":
					gotStart = true
				case prog == 1 && msg == comp+" Done":
					gotDone = true
				}
			}
		}
		if !gotStart || !gotDone {
			t.Fatalf("component %q missing Started/Done in trace: %s", comp, raw)
		}
	}
}

// TestDebugLogSink_RealPipeline_EndMarkerCarriesDSL locks the end-to-end
// contract the front-end "View result" depends on: the debug log's END marker
// trace[0] MUST carry a `dsl` (built by BuildDebugResultDSL) whose `components`
// map mirrors the canvas. The front-end `useFetchPipelineResult` picks
// `latest.trace.at(0)` where `latest.component_id == "END"` and reads
// `dsl.components[<id>].obj` to render parsed chunks (parser.tsx:41). Without
// this, "View result" shows only the file preview — the bug this change fixes.
//
// It drives a REAL pipeline (same wiring as a dataflow debug run) so the `dsl`
// is built from an actual run output map, not a hand-built one.
func TestDebugLogSink_RealPipeline_EndMarkerCarriesDSL(t *testing.T) {
	const (
		compC = "trace.RealStubC"
		compD = "trace.RealStubD"
	)
	ctx := t.Context()
	runtime.MustRegister(compC, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceStubComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})
	runtime.MustRegister(compD, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceStubComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})

	store := &capturedStore{}
	sink := NewDebugLogSink("c-dsl", "m-dsl", store)

	dsl := `{"dsl":{"components":{
		"begin":{"obj":{"component_name":"Begin","params":{}},"downstream":["c"]},
		"c":{"obj":{"component_name":"` + compC + `","params":{}},"upstream":["begin"],"downstream":["d"]},
		"d":{"obj":{"component_name":"` + compD + `","params":{}},"upstream":["c"]}
	},"path":["begin","c","d"],"graph":{"nodes":[{"id":"begin","data":{"name":"开始"}},{"id":"c","data":{"name":"解析"}},{"id":"d","data":{"name":"分词"}}]}}}`

	pipe, err := pipeline.NewPipelineFromDSL([]byte(dsl), "task-dsl",
		pipeline.WithProgressSink(sink), pipeline.WithDocumentID("doc-dsl"))
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	output, err := pipe.Run(ctx, map[string]any{"name": "doc-dsl"}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The executor would call this after Run; here we invoke it directly.
	resultDSL, err := BuildDebugResultDSL(dsl, output)
	if err != nil {
		t.Fatalf("BuildDebugResultDSL: %v", err)
	}
	sink.SetResult(resultDSL, output)
	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c-dsl-m-dsl-logs")
	if raw == "" {
		t.Fatalf("expected debug log written to c-dsl-m-dsl-logs")
	}
	arr := loadArray(t, raw)

	// END must be the last element.
	if arr[len(arr)-1]["component_id"] != "END" {
		t.Fatalf("last element must be END marker: %s", raw)
	}
	endTrace, _ := arr[len(arr)-1]["trace"].([]any)
	if len(endTrace) == 0 {
		t.Fatalf("END trace is empty: %s", raw)
	}
	endFirst, _ := endTrace[0].(map[string]any)
	dslVal := endFirst["dsl"]
	if dslVal == nil {
		t.Fatalf("END marker trace[0] must carry a non-empty dsl: %s", raw)
	}

	// json.RawMessage decodes into a generic map as map[string]any (not a
	// string), so accept either form.
	var dslDoc map[string]any
	switch v := dslVal.(type) {
	case string:
		if err := json.Unmarshal([]byte(v), &dslDoc); err != nil {
			t.Fatalf("END dsl is not valid JSON: %v body=%s", err, v)
		}
	case map[string]any:
		dslDoc = v
	default:
		t.Fatalf("END dsl unexpected type %T: %s", v, raw)
	}
	components, ok := dslDoc["components"].(map[string]any)
	if !ok || len(components) == 0 {
		t.Fatalf("END dsl.components missing or empty: %s", raw)
	}
	// The dsl must mirror every component in the canvas (begin, c, d).
	for _, id := range []string{"begin", "c", "d"} {
		if _, exists := components[id]; !exists {
			t.Errorf("END dsl.components missing component %q: %s", id, raw)
		}
	}
}

// TestDebugLogSink_RealPipeline_EndMarkerDSLShowsChunks is the strongest
// regression guard for the nested-state bug: it drives a REAL pipeline whose
// component "c" emits actual `chunks`, then asserts the END-marker DSL carries
// a non-empty params.outputs for that component. With the bug (reading
// output[id] at the top level instead of output["state"][id]), the run output
// is nested and params.outputs stays empty — the front-end "View result" shows
// blank tabs. The companion TestDebugLogSink_RealPipeline_EndMarkerCarriesDSL
// only checks component keys exist (its stubs return {"ok":true}, no chunks),
// so it would not catch this. This test closes that end-to-end gap.
//
// It also pins the inverse: component "d" (stub returning {"ok":true}) has NO
// recognized output key, so its params.outputs must be absent — the safe-empty
// contract the front-end renders as a blank step.
func TestDebugLogSink_RealPipeline_EndMarkerDSLShowsChunks(t *testing.T) {
	const (
		compC = "trace.RealStubChunks"
		compD = "trace.RealStubD2"
	)
	ctx := t.Context()
	runtime.MustRegister(compC, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceChunkComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})
	runtime.MustRegister(compD, runtime.CategoryIngestion,
		func(_ string, _ map[string]any) (runtime.Component, error) { return traceStubComponent{}, nil },
		runtime.Metadata{Version: "1.0.0"})

	store := &capturedStore{}
	sink := NewDebugLogSink("c-chunk", "m-chunk", store)

	dsl := `{"dsl":{"components":{
		"begin":{"obj":{"component_name":"Begin","params":{}},"downstream":["c"]},
		"c":{"obj":{"component_name":"` + compC + `","params":{"setups":{"pdf":{"parse_method":"general"}}}},"upstream":["begin"],"downstream":["d"]},
		"d":{"obj":{"component_name":"` + compD + `","params":{}},"upstream":["c"]}
	},"path":["begin","c","d"],"graph":{"nodes":[{"id":"begin","data":{"name":"开始"}},{"id":"c","data":{"name":"解析"}},{"id":"d","data":{"name":"分词"}}]}}}`

	pipe, err := pipeline.NewPipelineFromDSL([]byte(dsl), "task-chunk",
		pipeline.WithProgressSink(sink), pipeline.WithDocumentID("doc-chunk"))
	if err != nil {
		t.Fatalf("NewPipelineFromDSL: %v", err)
	}
	output, err := pipe.Run(ctx, map[string]any{"name": "doc-chunk"}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	resultDSL, err := BuildDebugResultDSL(dsl, output)
	if err != nil {
		t.Fatalf("BuildDebugResultDSL: %v", err)
	}
	sink.SetResult(resultDSL, output)
	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c-chunk-m-chunk-logs")
	if raw == "" {
		t.Fatalf("expected debug log written")
	}
	arr := loadArray(t, raw)
	if arr[len(arr)-1]["component_id"] != "END" {
		t.Fatalf("last element must be END marker: %s", raw)
	}
	endTrace, _ := arr[len(arr)-1]["trace"].([]any)
	endFirst, _ := endTrace[0].(map[string]any)

	var dslDoc map[string]any
	switch v := endFirst["dsl"].(type) {
	case string:
		if err = json.Unmarshal([]byte(v), &dslDoc); err != nil {
			t.Fatalf("END dsl not valid JSON: %v", err)
		}
	case map[string]any:
		dslDoc = v
	default:
		t.Fatalf("END dsl unexpected type %T", v)
	}
	components, _ := dslDoc["components"].(map[string]any)

	// Component "c" emits chunks -> params.outputs MUST be populated.
	cParams := components["c"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	cOutputs, ok := cParams["outputs"].(map[string]any)
	if !ok {
		t.Fatalf("REGRESSION: chunk-emitting component c has empty params.outputs; "+
			"the front-end 'View result' would render blank tabs. params=%#v", cParams)
	}
	if of, _ := cOutputs["output_format"].(map[string]any); of["value"] != "chunks" {
		t.Errorf("c output_format=%#v want {value:\"chunks\"}", cOutputs["output_format"])
	}
	chunksVal := cOutputs["chunks"].(map[string]any)["value"].([]any)
	if len(chunksVal) != 1 {
		t.Fatalf("c chunks.value len=%d want 1", len(chunksVal))
	}
	chunk0 := chunksVal[0].(map[string]any)
	if chunk0["text"] != "real chunk" {
		t.Errorf("chunk text=%v want 'real chunk'", chunk0["text"])
	}
	if _, exists := chunk0["vector"]; exists {
		t.Errorf("vector must be stripped from chunk payload")
	}
	if _, ok := cParams["setups"].(map[string]any); !ok {
		t.Errorf("c params.setups must be carried from DSL: %#v", cParams)
	}

	// Component "d" emits {"ok":true} -> no recognized format -> outputs absent.
	dParams := components["d"].(map[string]any)["obj"].(map[string]any)["params"].(map[string]any)
	if _, exists := dParams["outputs"]; exists {
		t.Errorf("d has no recognized output, outputs must be absent: %#v", dParams)
	}

	// The END marker MESSAGE must be the JSON dump of the LAST component's
	// ("d") raw output — the contract the front-end "Export JSON" button
	// relies on (use-download-output.ts findEndOutput JSON.parses it). A
	// plain-text sentinel leaves the button permanently disabled.
	endMsg, _ := endFirst["message"].(string)
	var endOutput map[string]any
	if err := json.Unmarshal([]byte(endMsg), &endOutput); err != nil {
		t.Fatalf("END message must be the last component's output JSON, got %q", endMsg)
	}
	if endOutput["ok"] != true {
		t.Errorf("END message output=%#v want d's raw output {ok:true}", endOutput)
	}
}

// TestDebugLogSink_EndMessageFallsBackWithoutRunOutput pins the two cases
// where the END marker message must NOT be a JSON dump: no run output was
// handed to the sink (SetResult never called, e.g. the run failed before
// producing output) and the run ended with an error. In both cases the
// front-end export button stays disabled — matching Python, whose END message
// is only the output JSON on success.
func TestDebugLogSink_EndMessageFallsBackWithoutRunOutput(t *testing.T) {
	ctx := t.Context()

	// Case 1: successful run but SetResult was never called -> plain text.
	store := &capturedStore{}
	sink := NewDebugLogSink("c-nores", "m-nores", store)
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "A", Message: "A Done", Phase: phaseExit,
	})
	sink.Flush(ctx, nil)
	arr := loadArray(t, store.Get(ctx, "c-nores-m-nores-logs"))
	endFirst, _ := arr[len(arr)-1]["trace"].([]any)[0].(map[string]any)
	if msg, _ := endFirst["message"].(string); msg != "Debug run completed" {
		t.Errorf("END message without run output=%q want %q", msg, "Debug run completed")
	}

	// Case 2: run output IS available but the run failed -> error wins.
	store2 := &capturedStore{}
	sink2 := NewDebugLogSink("c-fail", "m-fail", store2)
	sink2.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "A", Message: "A Done", Phase: phaseExit,
	})
	sink2.SetResult(map[string]any{"components": map[string]any{}},
		map[string]any{"state": map[string]any{"A": map[string]any{"ok": true}}})
	sink2.Flush(ctx, errors.New("boom"))
	arr2 := loadArray(t, store2.Get(ctx, "c-fail-m-fail-logs"))
	endFirst2, _ := arr2[len(arr2)-1]["trace"].([]any)[0].(map[string]any)
	msg2, _ := endFirst2["message"].(string)
	if !strings.HasPrefix(msg2, "[ERROR] ") {
		t.Errorf("END message on failure must stay [ERROR]-prefixed, got %q", msg2)
	}
}

// TestDebugLogSink_ElapsedTimeIsInSeconds locks the unit contract for
// `elapsed_time`: it MUST be expressed in SECONDS, not the raw UnixMicro delta.
// This is the regression guard for the dataflow timeline showing values like
// "19951s" — the sink stored microseconds while the front-end renders the number
// verbatim with an "s" suffix (dataflow-timeline.tsx) and the rest of the app
// (webhook/workflow timelines) gates on `elapsed_time < 0.000001`, i.e. seconds.
//
// The test drives two progress events ~20ms apart and asserts the recorded
// elapsed_time is close to the measured wall-clock gap in seconds (not 1e6x
// larger, which is what an unfixed microsecond delta would produce).
func TestDebugLogSink_ElapsedTimeIsInSeconds(t *testing.T) {
	store := &capturedStore{}
	sink := NewDebugLogSink("c-elapsed", "m-elapsed", store)
	ctx := t.Context()

	t0 := time.Now()
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "A", Message: "A Started", Phase: phaseEnter,
	})
	time.Sleep(20 * time.Millisecond)
	t1 := time.Now()
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "A", Message: "A Done", Phase: phaseExit,
	})
	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c-elapsed-m-elapsed-logs")
	if raw == "" {
		t.Fatalf("expected debug log written to c-elapsed-m-elapsed-logs")
	}
	arr := loadArray(t, raw)
	if len(arr) < 2 {
		t.Fatalf("expected at least A + END elements, got %d: %s", len(arr), raw)
	}
	aTrace, _ := arr[0]["trace"].([]any)
	if len(aTrace) < 2 {
		t.Fatalf("A trace should have [Started, Done], got %d entries: %s", len(aTrace), raw)
	}
	doneTrace, _ := aTrace[1].(map[string]any)
	elapsed, ok := doneTrace["elapsed_time"].(float64)
	if !ok {
		t.Fatalf("elapsed_time missing or wrong type: %s", raw)
	}

	gapSeconds := t1.Sub(t0).Seconds()
	// Tolerance absorbs call-overhead jitter; the key is that elapsed is in the
	// same SECONDS scale as gapSeconds, not 1e6x larger (microseconds).
	// With an unfixed microsecond delta, elapsed would be ~gapSeconds*1e6 and
	// this assertion fails.
	if math.Abs(elapsed-gapSeconds) > math.Max(gapSeconds*0.5, 0.01) {
		t.Fatalf("elapsed_time=%v want ~%v seconds (sink must divide the UnixMicro delta by 1e6)", elapsed, gapSeconds)
	}
}

// TestDebugLogSink_TimestampIsInSeconds locks the unit contract for the
// `timestamp` field: it MUST be a Unix epoch in SECONDS, not the raw UnixMicro
// value. Although the dataflow timeline currently renders the `datetime` string
// rather than `timestamp`, the field is part of the stable log contract the
// front-end can rely on; emitting microseconds would put it 1e6 off from any
// consumer expecting a Unix timestamp. This matches the SECONDS convention
// already established by elapsed_time and the other (webhook/workflow) timeline
// views.
func TestDebugLogSink_TimestampIsInSeconds(t *testing.T) {
	store := &capturedStore{}
	sink := NewDebugLogSink("c-ts", "m-ts", store)
	ctx := t.Context()

	before := time.Now().Unix()
	sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
		Component: "A", Message: "A Started", Phase: phaseEnter,
	})
	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c-ts-m-ts-logs")
	if raw == "" {
		t.Fatalf("expected debug log written to c-ts-m-ts-logs")
	}
	arr := loadArray(t, raw)
	aTrace, _ := arr[0]["trace"].([]any)
	if len(aTrace) == 0 {
		t.Fatalf("A trace is empty: %s", raw)
	}
	firstTrace, _ := aTrace[0].(map[string]any)
	ts, ok := firstTrace["timestamp"].(float64)
	if !ok {
		t.Fatalf("timestamp missing or wrong type: %s", raw)
	}

	// If the sink emitted UnixMicro, ts would be ~1.7e15; a Unix-seconds value
	// is ~1.7e9 and must be within a few seconds of `before`.
	diff := math.Abs(ts - float64(before))
	if diff > 5 {
		t.Fatalf("timestamp=%v want ~%d (Unix seconds, not UnixMicro)", ts, before)
	}
}

// TestDebugLogSink_RespectsCaps pins the size guards: a pathological run that
// emits far more components / trace lines / over-long messages than any
// legitimate canvas ever would must be clamped, while the END completion marker
// and the front-end's completion predicate stay intact. This is the regression
// guard for the unbounded-memory / oversized-Redis-entry DoS surface.
//
// Counts are deliberately bounded so the test stays fast: we only need to exceed
// each cap by a small margin to prove clamping, not replay millions of events.
func TestDebugLogSink_RespectsCaps(t *testing.T) {
	ctx := t.Context()

	// Component cap: more distinct components than maxLogEntries (each one trace,
	// one over-long message that must be truncated to the rune cap).
	storeA := &capturedStore{}
	sinkA := NewDebugLogSink("c1", "mA", storeA)
	for i := 0; i < maxLogEntries+200; i++ {
		sinkA.OnComponentProgress(ctx, pipeline.ProgressEvent{
			Component: fmt.Sprintf("Comp-%d", i),
			Message:   strings.Repeat("x", 5000), // truncated to maxMessageRunes
			Phase:     phaseExit,
		})
	}
	sinkA.Flush(ctx, nil)

	arrA := loadArray(t, storeA.Get(ctx, "c1-mA-logs"))
	// Flush always appends the terminal END marker, so count only real components.
	var compCountA int
	for _, el := range arrA {
		if el["component_id"] == "END" {
			continue
		}
		compCountA++
	}
	if compCountA > maxLogEntries {
		t.Fatalf("components=%d want <= %d", compCountA, maxLogEntries)
	}
	for _, el := range arrA {
		traces, _ := el["trace"].([]any)
		for _, tr := range traces {
			tm, _ := tr.(map[string]any)
			if tm == nil {
				continue
			}
			msg, _ := tm["message"].(string)
			if utf8.RuneCountInString(msg) > maxMessageRunes {
				t.Fatalf("message rune len=%d want <= %d", utf8.RuneCountInString(msg), maxMessageRunes)
			}
		}
	}
	if !clientConsidersComplete(arrA) {
		t.Errorf("clamped log must still satisfy completion predicate; raw=%s", storeA.Get(ctx, "c1-mA-logs"))
	}

	// Trace cap: a single component with more traces than maxTracePerEntry.
	storeB := &capturedStore{}
	sinkB := NewDebugLogSink("c1", "mB", storeB)
	for j := 0; j < maxTracePerEntry+50; j++ {
		sinkB.OnComponentProgress(ctx, pipeline.ProgressEvent{
			Component: "Busy",
			Message:   "ok",
			Phase:     phaseExit,
		})
	}
	sinkB.Flush(ctx, nil)

	arrB := loadArray(t, storeB.Get(ctx, "c1-mB-logs"))
	// Busy (the only real component) + the END completion marker = 2 elements.
	if len(arrB) != 2 {
		t.Fatalf("Busy + END expected, got %d elements: %s", len(arrB), storeB.Get(ctx, "c1-mB-logs"))
	}
	busyTrace, _ := arrB[0]["trace"].([]any)
	if len(busyTrace) > maxTracePerEntry {
		t.Fatalf("Busy trace len=%d want <= %d", len(busyTrace), maxTracePerEntry)
	}
	if !clientConsidersComplete(arrB) {
		t.Errorf("trace-capped log must still satisfy completion predicate; raw=%s", storeB.Get(ctx, "c1-mB-logs"))
	}
}

// TestDebugLogSink_PayloadHardCap pins the absolute byte ceiling on the JSON
// written to Redis. It fills to the per-entry caps (maxLogEntries components x
// maxTracePerEntry traces) — which pre-trim is ~16 MiB — and asserts the stored
// value never exceeds maxPayloadBytes, and still ends with the END marker.
func TestDebugLogSink_PayloadHardCap(t *testing.T) {
	store := &capturedStore{}
	sink := NewDebugLogSink("c1", "m1", store)
	ctx := t.Context()

	for i := 0; i < maxLogEntries; i++ {
		comp := fmt.Sprintf("Comp-%d", i)
		for j := 0; j < maxTracePerEntry; j++ {
			sink.OnComponentProgress(ctx, pipeline.ProgressEvent{
				Component: comp,
				Message:   "short",
				Phase:     phaseExit,
			})
		}
	}
	sink.Flush(ctx, nil)

	raw := store.Get(ctx, "c1-m1-logs")
	if raw == "" {
		t.Fatalf("expected debug log written to c1-m1-logs")
	}
	if len(raw) > maxPayloadBytes {
		t.Fatalf("stored payload bytes=%d want <= %d", len(raw), maxPayloadBytes)
	}
	arr := loadArray(t, raw)
	if !clientConsidersComplete(arr) {
		t.Errorf("hard-capped log must still satisfy completion predicate; arr=%s", raw)
	}
}
