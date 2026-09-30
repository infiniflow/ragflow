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

package agentic_rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"

	"gorm.io/gorm"
)

// failingRetrievalService makes every search_chunks query fail the way a dead
// embedding backend does (e.g. a drained SiliconFlow balance).
type failingRetrievalService struct {
	calls           int
	chunksPerOKCall []runtime.RetrievalChunk
	failOnCall      map[int]bool // 1-based; nil = fail every call
	err             error
}

func (f *failingRetrievalService) Search(_ context.Context, _ *gorm.DB, _ runtime.RetrievalRequest) ([]runtime.RetrievalChunk, error) {
	f.calls++
	if f.failOnCall == nil || f.failOnCall[f.calls] {
		return nil, errors.New("SILICONFLOW API error: 402 Payment Required")
	}
	return f.chunksPerOKCall, nil
}

// TestSearchChunksTool_AllQueriesFailIsCanonicalToolError pins the unified
// failure contract: every query died at the backend, so the result is a
// severity="error" <tool_error> carrying the root cause — not a Go error
// (which would abort the ReAct loop) and not a bare empty result (which the
// model would misread as "the corpus has nothing").
func TestSearchChunksTool_AllQueriesFailIsCanonicalToolError(t *testing.T) {
	fake := &failingRetrievalService{}
	runtime.SetRetrievalService(fake)
	defer runtime.SetRetrievalService(nil)

	out, err := NewSearchChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"queries":["a","b"]}`)
	if err != nil {
		t.Fatalf("all-queries-fail must become a result, got error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="search_chunks" severity="error" failed_queries="2">`) ||
		!strings.Contains(out, "SEMANTIC RETRIEVAL UNAVAILABLE") ||
		!strings.Contains(out, "402 Payment Required") {
		t.Errorf("expected canonical error tool_error with root cause, got %.300q", out)
	}
	if fake.calls != 2 {
		t.Errorf("every query must reach the backend, got %d calls", fake.calls)
	}
}

// TestSearchChunksTool_PartialFailIsWarnToolError asserts a partial failure
// keeps the surviving hits AND carries the failure as severity="warn" — the
// model learns the dead queries errored (not empty) while the hits stay usable.
func TestSearchChunksTool_PartialFailIsWarnToolError(t *testing.T) {
	fake := &failingRetrievalService{
		failOnCall:      map[int]bool{1: true},
		chunksPerOKCall: []runtime.RetrievalChunk{{ID: "c1", Content: "hello world", DocumentID: "d1"}},
	}
	runtime.SetRetrievalService(fake)
	defer runtime.SetRetrievalService(nil)

	out, err := NewSearchChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"queries":["broken","fine"]}`)
	if err != nil {
		t.Fatalf("partial failure must not error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="search_chunks" severity="warn" failed_queries="1" total_queries="2">`) {
		t.Errorf("expected canonical warn tool_error, got %.300q", out)
	}
	if !strings.Contains(out, `chunk_id="c1"`) {
		t.Errorf("surviving hit must stay in the result, got %.300q", out)
	}
}

// mustJSONString marshals v to a compact JSON string, failing the test on error.
func mustJSONString(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// stubOwner overrides the document-owner lookup for one test so deep-read flows
// don't need a database. kbID is what ResolveDocDatasetID will report.
func stubOwner(t *testing.T, kbID string) {
	t.Helper()
	orig := resolveDocumentOwner
	resolveDocumentOwner = func(_ context.Context, _ string) (string, error) { return kbID, nil }
	t.Cleanup(func() { resolveDocumentOwner = orig })
}

// === think ===

func TestThinkTool_Valid(t *testing.T) {
	out, err := NewThinkTool().InvokableRun(context.Background(),
		`{"thought":"explore","next_thought_needed":false,"thought_number":1,"total_thoughts":1}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "recorded") {
		t.Errorf("unexpected output: %q", out)
	}
}

func TestThinkTool_EmptyThought(t *testing.T) {
	out, err := NewThinkTool().InvokableRun(context.Background(),
		`{"thought":"","next_thought_needed":false,"thought_number":1,"total_thoughts":1}`)
	if err != nil {
		t.Fatalf("failure must become a result, got error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="think"`) {
		t.Errorf("expected a think <tool_error> result, got %.200q", out)
	}
}

func TestThinkTool_InvalidNumber(t *testing.T) {
	out, err := NewThinkTool().InvokableRun(context.Background(),
		`{"thought":"x","next_thought_needed":false,"thought_number":0,"total_thoughts":1}`)
	if err != nil {
		t.Fatalf("failure must become a result, got error: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("expected a <tool_error> result, got %.200q", out)
	}
}

// === todo_write ===

func TestTodoWriteTool_Echo(t *testing.T) {
	out, err := NewTodoWriteTool().InvokableRun(context.Background(),
		`{"task":"find X","steps":[{"id":"1","description":"search","status":"completed"},{"id":"2","description":"read","status":"pending"}]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "Completed: 1") || !strings.Contains(out, "Pending: 1") {
		t.Errorf("unexpected output: %q", out)
	}
	if strings.Contains(out, "✅") || strings.Contains(out, "🔍") {
		t.Errorf("emoji must not appear: %q", out)
	}
}

func TestTodoWriteTool_EmptySteps(t *testing.T) {
	out, err := NewTodoWriteTool().InvokableRun(context.Background(), `{"task":"find X","steps":[]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "grep_chunks") || !strings.Contains(out, "search_chunks") {
		t.Errorf("suggested workflow missing: %q", out)
	}
}

// === grep_chunks scoring / snippet ===

func TestScoreGrepChunks_ScoreAndDedupe(t *testing.T) {
	re := regexp.MustCompile("(?i)stardust")
	chunks := []runtime.RetrievalChunk{
		{ID: "a", Content: "stardust engine stardust engine"},
		{ID: "b", Content: "nothing here"},
		{ID: "a", Content: "duplicate id"},
	}
	scored := scoreGrepChunks(chunks, re)
	if len(scored) != 2 {
		t.Fatalf("len=%d, want 2 (dedupe)", len(scored))
	}
	if scored[0].chunk.ID != "a" || scored[0].score <= 0 {
		t.Errorf("chunk a should rank first with score>0, got id=%s score=%f", scored[0].chunk.ID, scored[0].score)
	}
	if scored[1].chunk.ID != "b" || scored[1].score != 0 {
		t.Errorf("chunk b should score 0, got id=%s score=%f", scored[1].chunk.ID, scored[1].score)
	}
}

func TestGrepChunksTool_InvalidRegex(t *testing.T) {
	out, err := NewGrepChunksTool("", nil).InvokableRun(context.Background(), `{"query":"("}`)
	if err != nil {
		t.Fatalf("failure must become a result, got error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="grep_chunks"`) || !strings.Contains(out, "invalid regex") {
		t.Errorf("expected a grep_chunks <tool_error> naming the bad regex, got %.200q", out)
	}
}

func TestGrepChunksTool_EmptyQuery(t *testing.T) {
	out, err := NewGrepChunksTool("", nil).InvokableRun(context.Background(), `{"query":""}`)
	if err != nil {
		t.Fatalf("failure must become a result, got error: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("expected a <tool_error> result, got %.200q", out)
	}
}

// === search_chunks ===

func TestSearchChunksTool_TooManyQueries(t *testing.T) {
	out, err := NewSearchChunksTool("", nil).InvokableRun(context.Background(),
		`{"queries":["a","b","c","d","e","f"]}`)
	if err != nil {
		t.Fatalf("failure must become a result, got error: %v", err)
	}
	if !strings.Contains(out, `<tool_error tool="search_chunks"`) || !strings.Contains(out, "at most 5") {
		t.Errorf("expected a search_chunks <tool_error> naming the limit, got %.200q", out)
	}
}

func TestSearchChunksTool_EmptyQueries(t *testing.T) {
	out, err := NewSearchChunksTool("", nil).InvokableRun(context.Background(), `{"queries":[]}`)
	if err != nil {
		t.Fatalf("failure must become a result, got error: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("expected a <tool_error> result, got %.200q", out)
	}
}

// === run_javascript ===

func TestRunJavascriptTool_Stdout(t *testing.T) {
	out, err := NewRunJavascriptTool().InvokableRun(context.Background(),
		`{"code":"var nums=[1,2,3,4];var s=0;for(var i=0;i<nums.length;i++){s+=nums[i];}console.log(\"sum=\"+s);"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "sum=10") {
		t.Errorf("unexpected stdout: %q", out)
	}
}

func TestRunJavascriptTool_EmptyCode(t *testing.T) {
	out, err := NewRunJavascriptTool().InvokableRun(context.Background(), `{"code":""}`)
	if err != nil {
		t.Fatalf("failure must become a result, got error: %v", err)
	}
	if !strings.Contains(out, toolErrorMarker) {
		t.Errorf("expected a <tool_error> result, got %.200q", out)
	}
}

func TestRunJavascriptTool_RejectsES6(t *testing.T) {
	// import/export are module-system tokens the sandbox cannot honor; they are
	// rejected up-front (the module-token denylist), and ES6-only syntax goja
	// cannot parse surfaces as a compile-time syntax rejection. The rejection
	// travels as a <tool_error> result so the model can rewrite the snippet.
	out, err := NewRunJavascriptTool().InvokableRun(context.Background(),
		`{"code":"import {x} from 'mod'; console.log(x);"}`)
	if err != nil || !strings.Contains(out, "module") || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("expected module-system rejection as tool_error result, got err=%v out=%.200q", err, out)
	}
}

func TestRunJavascriptTool_RejectsRequire(t *testing.T) {
	out, err := NewRunJavascriptTool().InvokableRun(context.Background(),
		`{"code":"var m = require('fs'); console.log(m);"}`)
	if err != nil || !strings.Contains(out, "module") || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("expected module-system rejection as tool_error result, got err=%v out=%.200q", err, out)
	}
}

// TestRunJavascriptTool_Interrupt asserts a runaway loop is interrupted when the
// request context is cancelled, so a `while(true){}` cannot hang the goroutine.
func TestRunJavascriptTool_Interrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan string, 1)
	go func() {
		out, _ := NewRunJavascriptTool().InvokableRun(ctx,
			`{"code":"while(true){}"}`)
		done <- out
	}()

	// Let the snippet start spinning, then cancel the context.
	select {
	case out := <-done:
		t.Fatalf("expected interruption, got early completion out=%.200q", out)
	default:
	}
	cancel()

	select {
	case out := <-done:
		if !strings.Contains(out, toolErrorMarker) {
			t.Fatalf("interrupted loop must produce a tool_error result, got %.200q", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run_javascript did not interrupt the loop within 2s")
	}
}

// TestRunJavascriptTool_CodeSizeCap asserts oversized snippets are rejected
// up front.
func TestRunJavascriptTool_CodeSizeCap(t *testing.T) {
	big := strings.Repeat("var x=1;", runJavascriptMaxCodeBytes/8+1)
	out, err := NewRunJavascriptTool().InvokableRun(context.Background(),
		`{"code":`+mustJSONString(t, big)+`}`)
	if err != nil || !strings.Contains(out, "too large") || !strings.Contains(out, toolErrorMarker) {
		t.Fatalf("expected code-size rejection as tool_error result, got err=%v out=%.200q", err, out)
	}
}

// TestRunJavascriptTool_ContextCancelInterrupts asserts the tool honors the
// caller's context: canceling it interrupts the interpreter promptly (there is
// intentionally no per-tool wall-clock timeout; the run-level budget cancels
// the agent context when exceeded).
func TestRunJavascriptTool_ContextCancelInterrupts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	start := time.Now()
	outCh := make(chan string, 1)
	go func() {
		cancel()
		out, _ := NewRunJavascriptTool().InvokableRun(ctx, // canceled before/during run
			`{"code":"while(true){}"}`)
		outCh <- out
	}()

	select {
	case out := <-outCh:
		if !strings.Contains(out, toolErrorMarker) || !strings.Contains(out, "interrupted") {
			t.Fatalf("expected interrupted tool_error result, got %.200q", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("infinite loop was not interrupted by canceled context")
	}
	_ = start
}

// TestBoundedBuffer_CapsOutput asserts the stdout buffer stops growing once it
// hits its cap, so an unbounded console.log cannot exhaust host memory.
func TestBoundedBuffer_CapsOutput(t *testing.T) {
	b := newBoundedBuffer(10)
	b.writeString("abcdefghij") // exactly cap
	if b.String() != "abcdefghij" {
		t.Fatalf("buf = %q, want first 10 bytes", b.String())
	}
	b.writeString("KLMN") // beyond cap → dropped
	if b.String() != "abcdefghij" {
		t.Errorf("buf grew past cap: %q", b.String())
	}
	b.writeByte('X') // beyond cap → dropped
	if len(b.String()) != 10 {
		t.Errorf("writeByte past cap grew buffer: %q", b.String())
	}

	// Partial write is truncated to the remaining budget.
	b2 := newBoundedBuffer(5)
	b2.writeString("hello world")
	if b2.String() != "hello" {
		t.Errorf("partial truncation = %q, want \"hello\"", b2.String())
	}
	if !b2.exceeded {
		t.Error("expected exceeded flag after truncation")
	}
}

// === GrepAdapter ES-failure → RE2 fallback ===

// grepFakeEngine stubs only the engine methods GrepAdapter touches. It embeds
// the full DocEngine interface as a nil value; the explicitly-defined promoted
// methods (GetType / Search / SearchByRegexp) take precedence.
type grepFakeEngine struct {
	engine.DocEngine
	searchByRegexpErr error
	regexpChunks      []map[string]interface{}
	searchChunks      []map[string]interface{}
	searchErr         error
	searchCalls       int
	lastReq           *enginetypes.SearchRequest
}

func (e *grepFakeEngine) GetType() string { return string(engine.EngineElasticsearch) }
func (e *grepFakeEngine) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	e.searchCalls++
	e.lastReq = req
	if e.searchErr != nil {
		return nil, e.searchErr
	}
	return &enginetypes.SearchResult{Chunks: e.searchChunks}, nil
}
func (e *grepFakeEngine) SearchByRegexp(_ context.Context, req *enginetypes.RegexpSearchRequest) (*enginetypes.SearchResult, error) {
	if e.searchByRegexpErr != nil {
		return nil, e.searchByRegexpErr
	}
	chunks := e.regexpChunks
	// Term-filter on chunk ids (FetchChunksByID / meta lookups), mirroring the
	// bool filter the real engine applies before sort/offset/limit.
	if ids, ok := req.Filter["id"].([]string); ok && len(ids) > 0 {
		want := make(map[string]bool, len(ids))
		for _, id := range ids {
			want[id] = true
		}
		filtered := chunks[:0:0]
		for _, c := range chunks {
			if want[runtime.FirstStringFromMap(c, "id", "_id")] {
				filtered = append(filtered, c)
			}
		}
		chunks = filtered
	}
	// When a sort (e.g. reading order) is requested, order the result set by
	// chunk_order_int ascending before applying offset/limit, mirroring what the
	// real ES engine does with a pushed-down sort clause.
	if req.Sort != nil && len(req.Sort.Fields) > 0 {
		ordered := append([]map[string]interface{}(nil), chunks...)
		sort.SliceStable(ordered, func(i, j int) bool {
			return runtime.IntFromMap(ordered[i], "chunk_order_int") < runtime.IntFromMap(ordered[j], "chunk_order_int")
		})
		chunks = ordered
	}
	offset := max(req.Offset, 0)
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}
	if offset < len(chunks) {
		chunks = chunks[offset:]
	} else {
		chunks = nil
	}
	if len(chunks) > limit {
		chunks = chunks[:limit]
	}
	return &enginetypes.SearchResult{Chunks: chunks}, nil
}

// TestGrepAdapter_NoFallbackOnRegexpError asserts that with content_with_weight
// mapped as a searchable keyword field, a regexp pushdown failure is surfaced as
// an error (there is intentionally no in-memory RE2 fallback anymore).
func TestGrepAdapter_NoFallbackOnRegexpError(t *testing.T) {
	fe := &grepFakeEngine{
		searchByRegexpErr: fmt.Errorf("elasticsearch regexp error on index \"ragflow_t\": \\b is not supported"),
	}
	adapter := NewGrepAdapter(fe)

	_, err := adapter.Grep(context.Background(), runtime.GrepRequest{
		TenantID:   "t",
		Pattern:    `\bbeta\b`,
		DatasetIDs: []string{"kb1"},
		Limit:      30,
	})
	if err == nil {
		t.Fatal("expected Grep to surface the regexp pushdown error (no in-memory fallback)")
	}
	if fe.searchCalls != 0 {
		t.Fatalf("expected no in-memory recall Search after fallback removal, got %d", fe.searchCalls)
	}
}

// TestGrepAdapter_RegexpPushdownSucceeds asserts the ES pushdown result is used
// directly when it does not error.
func TestGrepAdapter_RegexpPushdownSucceeds(t *testing.T) {
	fe := &grepFakeEngine{
		regexpChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "alpha beta", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1"},
		},
	}
	adapter := NewGrepAdapter(fe)

	chunks, err := adapter.Grep(context.Background(), runtime.GrepRequest{
		TenantID: "t", Pattern: "beta", DatasetIDs: []string{"kb1"}, Limit: 30,
	})
	if err != nil {
		t.Fatalf("Grep: %v", err)
	}
	if fe.searchCalls != 0 {
		t.Fatalf("expected pushdown to succeed without in-memory recall; Search called %d times", fe.searchCalls)
	}
	if len(chunks) != 1 || chunks[0].ID != "c1" {
		t.Errorf("chunks = %v, want [c1]", chunks)
	}
}

// TestEngineCallContext_CanceledParent asserts a canceled incoming context (the
// streaming-ReAct artifact where eino cancels the tool context after the model
// emits tool_calls) does not propagate to the engine query: the returned context
// must still be usable.
func TestEngineCallContext_CanceledParent(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // parent already canceled

	ectx, ecancel := engineCallContext(cancelledCtx)
	defer ecancel()
	if err := ectx.Err(); err != nil {
		t.Fatalf("engineCallContext from canceled parent still canceled: %v", err)
	}

	// A live parent keeps its values/deadline semantics.
	liveCtx := context.Background()
	ectx2, ecancel2 := engineCallContext(liveCtx)
	defer ecancel2()
	if err := ectx2.Err(); err != nil {
		t.Fatalf("engineCallContext from live parent canceled: %v", err)
	}
}

// TestIsGraphChunkContent asserts graph relation/entity/location payloads are
// detected so deep-read and search_chunks skip them, while original prose is
// kept.
func TestIsGraphChunkContent(t *testing.T) {
	graphCases := []string{
		`{"head":"何进","relation_type":"鸩杀","tail":"董太后","type":"relation"}`,
		`{"name":"渑池","type":"location"}`,
		`{"object":"何进","predicate":"谋害","subject":"蹇硕","type":"relation"}`,
		`{"head":"A","tail":"B"}`,
	}
	for _, c := range graphCases {
		if !isGraphChunkContent(c) {
			t.Errorf("expected graph chunk detected: %s", c)
		}
	}
	proseCases := []string{
		`帝召大将军何进调兵擒马元义，斩之。`,
		`董太后被何进鸩杀，董重自刎于后堂。`,
		`{"quoted":"not a graph payload but valid json"}`,
		`何进犹豫不决，听信袁绍之言。`,
	}
	for _, c := range proseCases {
		if isGraphChunkContent(c) {
			t.Errorf("expected prose not treated as graph chunk: %s", c)
		}
	}
}

// TestListChunks_DeepRead verifies list_chunks reads the full original
// prose of ONE document (single doc_id) in reading order
// (chunk_index), excludes graph triple chunks, and respects offset/limit.
func TestListChunks_DeepRead(t *testing.T) {
	fe := &grepFakeEngine{
		regexpChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "帝召大将军何进调兵擒马元义，斩之。", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(2)},
			{"id": "c2", "content_with_weight": `{"head":"何进","relation_type":"鸩杀","tail":"董太后","type":"relation"}`, "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(0)},
			{"id": "c3", "content_with_weight": "董重知事急，自刎于后堂。", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(1)},
		},
	}
	adapter := NewGrepAdapter(fe)

	runtime.SetGrepService(adapter)
	defer runtime.SetGrepService(nil)
	stubOwner(t, "kb1")

	out, err := NewListChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"doc_id":"d1","anchor_chunk_ids":["c1","c3"]}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "马元义") || !strings.Contains(out, "自刎") {
		t.Fatalf("deep-read output missing original prose: %s", out)
	}
	if strings.Contains(out, "relation_type") {
		t.Fatalf("deep-read output leaked graph triple: %s", out)
	}
	// Graph chunk (c2, chunk_order_int=0) stays in the ordered index but its
	// payload is excluded from output, so the remaining prose chunks must be
	// emitted in reading order by chunk_index: c3 (index 1, "自刎") before
	// c1 (index 2, "马元义").
	if strings.Index(out, "自刎") > strings.Index(out, "马元义") {
		t.Fatalf("deep-read chunks not in reading order: %s", out)
	}
	// Whole-document read header should carry doc_id and fetched counts.
	for _, want := range []string{`doc_id="d1"`, `fetched="2"`} {
		if !strings.Contains(out, want) {
			t.Errorf("deep-read output missing %q: %s", want, out)
		}
	}
}

// TestListChunks_AnchorNeighbors verifies the anchored-window contract:
// required array anchors, symmetric merged windows with doc_chunks_total,
// single-chunk direct reads at number_neighbors=0, multi-anchor union reads,
// and loud failures for missing anchors / contract violations.
func TestListChunks_AnchorNeighbors(t *testing.T) {
	fe := &grepFakeEngine{
		regexpChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk A", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(0)},
			{"id": "c2", "content_with_weight": "chunk B", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(1)},
			{"id": "c3", "content_with_weight": "chunk C", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(2)},
			{"id": "c4", "content_with_weight": "chunk D", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(3)},
		},
	}
	runtime.SetGrepService(NewGrepAdapter(fe))
	defer runtime.SetGrepService(nil)
	stubOwner(t, "kb1")

	tool := NewListChunksTool("t", []string{"kb1"})

	// Single anchor + 1 neighbor → window c2..c4, header carries doc_chunks_total.
	out, err := tool.InvokableRun(context.Background(),
		`{"doc_id":"d1","anchor_chunk_ids":["c3"],"number_neighbors":1}`)
	if err != nil {
		t.Fatalf("InvokableRun anchored: %v", err)
	}
	for _, forbidden := range []string{"chunk A"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("window leaked below the -1 neighbor (%s present): %.200q", forbidden, out)
		}
	}
	for _, want := range []string{"chunk B", "chunk C", `anchored_chunk_ids="c3"`,
		`doc_chunks_total="4"`, `number_neighbors="1"`} {
		if !strings.Contains(out, want) {
			t.Errorf("anchored output missing %q: %.200q", want, out)
		}
	}

	// Two overlapping anchors merge into one reading-order window (no dupes).
	out, err = tool.InvokableRun(context.Background(),
		`{"doc_id":"d1","anchor_chunk_ids":["c2","c3"],"number_neighbors":1}`)
	if err != nil {
		t.Fatalf("InvokableRun two anchors: %v", err)
	}
	if got := strings.Count(out, "<chunk "); got != 4 {
		t.Errorf("merged window chunk count = %d, want 4 (A..D)", got)
	}
	if idx := strings.Index(out, "chunk A"); idx > strings.Index(out, "chunk D") {
		t.Errorf("merged window not in reading order: %.200q", out)
	}

	// number_neighbors=0 → ONLY the anchored chunks themselves in one direct fetch.
	out, err = tool.InvokableRun(context.Background(),
		`{"doc_id":"d1","anchor_chunk_ids":["c1","c3"],"number_neighbors":0}`)
	if err != nil {
		t.Fatalf("InvokableRun direct anchors: %v", err)
	}
	for _, forbidden := range []string{"chunk B", "chunk D"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("direct read must exclude non-anchors (%s): %.200q", forbidden, out)
		}
	}
	for _, want := range []string{"chunk A", "chunk C", `anchored_chunk_ids="c1,c3"`, `number_neighbors="0"`} {
		if !strings.Contains(out, want) {
			t.Errorf("direct output missing %q: %.200q", want, out)
		}
	}
	if strings.Contains(out, "doc_chunks_total") {
		t.Error("doc_chunks_total must be absent for nb=0 (no index pass)")
	}

	// Unknown anchors with nb=0 are reported IN BAND, not as an error: "this
	// citation does not resolve" is a FINDING the caller must be able to see
	// and judge (the auditor records it as `field integrity: chunk_id does not
	// resolve`), whereas an error aborts the whole ReAct turn - one fabricated
	// chunk_id used to kill an entire audit pass.
	out, err = tool.InvokableRun(context.Background(),
		`{"doc_id":"d1","anchor_chunk_ids":["zz"],"number_neighbors":0}`)
	if err != nil {
		t.Fatalf("unknown anchor (nb=0) must not abort the turn, got %v", err)
	}
	for _, want := range []string{`fetched="0"`, "notice", "UNVERIFIED"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty-result output missing %q: %.300q", want, out)
		}
	}
	// Unknown anchors with nb>0 are likewise reported IN BAND, never as an
	// error: one fabricated id used to kill an entire repair turn, and the
	// gate then paid for a retry that failed the same way. Every unresolved
	// id must be NAMED, and no substitute content is invented for them.
	out, err = tool.InvokableRun(context.Background(),
		`{"doc_id":"d1","anchor_chunk_ids":["zz"],"number_neighbors":2}`)
	if err != nil {
		t.Fatalf("unknown anchor (nb>0) must not abort the turn, got %v", err)
	}
	for _, want := range []string{"notice", "missing: zz", "UNVERIFIED", "NO anchor resolved"} {
		if !strings.Contains(out, want) {
			t.Errorf("nb>0 unresolved-anchor output missing %q: %.400q", want, out)
		}
	}
	// Fail fast: no Pass-2 fetch and no partial window, so nothing is returned.
	if !strings.Contains(out, `fetched="0"`) {
		t.Errorf("unresolved anchors must return no chunks: %.400q", out)
	}
	// A PARTIAL failure is not degraded into partial content either: the whole
	// call reports the unresolved ids and returns no text.
	out, err = tool.InvokableRun(context.Background(),
		`{"doc_id":"d1","anchor_chunk_ids":["c1","zz"],"number_neighbors":1}`)
	if err != nil {
		t.Fatalf("partial unresolved anchors must not abort the turn, got %v", err)
	}
	for _, want := range []string{"missing: zz", "NO chunk text is returned", `fetched="0"`} {
		if !strings.Contains(out, want) {
			t.Errorf("partial unresolved-anchor output missing %q: %.400q", want, out)
		}
	}

	// Contract violations surface as <tool_error> results (the loop keeps
	// running; the model reads the reason and can fix its arguments).
	if out, err := tool.InvokableRun(context.Background(), `{"doc_id":"d1","number_neighbors":1}`); err != nil ||
		!strings.Contains(out, "anchor_chunk_ids is required") {
		t.Fatalf("missing anchors must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
}

// TestListChunks_DocIDRequired verifies doc_id is the only identifier the tool
// needs (no dataset_id parameter) and that it is mandatory.
func TestListChunks_DocIDRequired(t *testing.T) {
	fe := &grepFakeEngine{
		regexpChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "原文", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(0)},
		},
	}
	adapter := NewGrepAdapter(fe)
	runtime.SetGrepService(adapter)
	defer runtime.SetGrepService(nil)
	stubOwner(t, "kb1")

	// Both required args must be present: doc_id and at least one anchor.
	if out, err := NewListChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"anchor_chunk_ids":["c1"]}`); err != nil || !strings.Contains(out, "doc_id is required") {
		t.Fatalf("missing doc_id must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
	if out, err := NewListChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(),
		`{"doc_id":"d1"}`); err != nil || !strings.Contains(out, "anchor_chunk_ids is required") {
		t.Fatalf("missing anchors must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
	if out, err := NewListChunksTool("t", []string{"kb1"}).InvokableRun(context.Background(), `{}`); err != nil ||
		!strings.Contains(out, toolErrorMarker) {
		t.Fatalf("everything missing must produce a tool_error result, got err=%v out=%.200q", err, out)
	}
}
