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
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/engine"
	enginetypes "ragflow/internal/engine/types"
)

// mustJSONString marshals v to a compact JSON string, failing the test on error.
func mustJSONString(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
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
	_, err := NewThinkTool().InvokableRun(context.Background(),
		`{"thought":"","next_thought_needed":false,"thought_number":1,"total_thoughts":1}`)
	if err == nil {
		t.Fatal("expected error for empty thought")
	}
}

func TestThinkTool_InvalidNumber(t *testing.T) {
	_, err := NewThinkTool().InvokableRun(context.Background(),
		`{"thought":"x","next_thought_needed":false,"thought_number":0,"total_thoughts":1}`)
	if err == nil {
		t.Fatal("expected error for thought_number < 1")
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
	_, err := NewGrepChunksTool().InvokableRun(context.Background(), `{"query":"("}`)
	if err == nil || !strings.Contains(err.Error(), "invalid regex") {
		t.Fatalf("expected invalid regex error, got %v", err)
	}
}

func TestGrepChunksTool_EmptyQuery(t *testing.T) {
	_, err := NewGrepChunksTool().InvokableRun(context.Background(), `{"query":""}`)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

// === search_chunks ===

func TestSearchChunksTool_TooManyQueries(t *testing.T) {
	_, err := NewSearchChunksTool().InvokableRun(context.Background(),
		`{"queries":["a","b","c","d","e","f"]}`)
	if err == nil || !strings.Contains(err.Error(), "at most 5") {
		t.Fatalf("expected 'at most 5' error, got %v", err)
	}
}

func TestSearchChunksTool_EmptyQueries(t *testing.T) {
	_, err := NewSearchChunksTool().InvokableRun(context.Background(), `{"queries":[]}`)
	if err == nil {
		t.Fatal("expected error for empty queries")
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
	_, err := NewRunJavascriptTool().InvokableRun(context.Background(), `{"code":""}`)
	if err == nil {
		t.Fatal("expected error for empty code")
	}
}

func TestRunJavascriptTool_RejectsES6(t *testing.T) {
	// import/export are module-system tokens the sandbox cannot honor; they are
	// rejected up-front (the module-token denylist), and ES6-only syntax goja
	// cannot parse surfaces as a compile-time syntax rejection.
	_, err := NewRunJavascriptTool().InvokableRun(context.Background(),
		`{"code":"import {x} from 'mod'; console.log(x);"}`)
	if err == nil || !strings.Contains(err.Error(), "module") {
		t.Fatalf("expected module-system rejection, got %v", err)
	}
}

func TestRunJavascriptTool_RejectsRequire(t *testing.T) {
	_, err := NewRunJavascriptTool().InvokableRun(context.Background(),
		`{"code":"var m = require('fs'); console.log(m);"}`)
	if err == nil || !strings.Contains(err.Error(), "module") {
		t.Fatalf("expected module-system rejection, got %v", err)
	}
}

// TestRunJavascriptTool_Interrupt asserts a runaway loop is interrupted when the
// request context is cancelled, so a `while(true){}` cannot hang the goroutine.
func TestRunJavascriptTool_Interrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewRunJavascriptTool().InvokableRun(ctx,
			`{"code":"while(true){}"}`)
		done <- err
	}()

	// Let the snippet start spinning, then cancel the context.
	select {
	case err := <-done:
		t.Fatalf("expected interruption, got early completion err=%v", err)
	default:
	}
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error from interrupted loop")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run_javascript did not interrupt the loop within 2s")
	}
}

// TestRunJavascriptTool_CodeSizeCap asserts oversized snippets are rejected
// up front.
func TestRunJavascriptTool_CodeSizeCap(t *testing.T) {
	big := strings.Repeat("var x=1;", runJavascriptMaxCodeBytes/8+1)
	_, err := NewRunJavascriptTool().InvokableRun(context.Background(),
		`{"code":`+mustJSONString(t, big)+`}`)
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected code-size rejection, got %v", err)
	}
}

// TestRunJavascriptTool_HardTimeout asserts the tool enforces its OWN wall-clock
// limit even when the caller's context has no deadline (a client keeping the
// connection open must not let `while(true){}` burn CPU forever).
func TestRunJavascriptTool_HardTimeout(t *testing.T) {
	orig := runJavascriptTimeout
	runJavascriptTimeout = 200 * time.Millisecond
	defer func() { runJavascriptTimeout = orig }()

	start := time.Now()
	_, err := NewRunJavascriptTool().InvokableRun(context.Background(), // no deadline
		`{"code":"while(true){}"}`)
	if err == nil {
		t.Fatal("expected hard-timeout interruption error")
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("expected interruption error, got %v", err)
	}
	// Bounded: must return well before the production 30s timeout.
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("hard timeout took %v, too slow", elapsed)
	}
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
}

func (e *grepFakeEngine) GetType() string { return string(engine.EngineElasticsearch) }
func (e *grepFakeEngine) Search(_ context.Context, req *enginetypes.SearchRequest) (*enginetypes.SearchResult, error) {
	e.searchCalls++
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

	ctx := runtime.WithScope(context.Background(), "t", []string{"kb1"})
	out, err := NewListChunksTool().InvokableRun(ctx,
		`{"doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("InvokableRun: %v", err)
	}
	if !strings.Contains(out, "马元义") || !strings.Contains(out, "自刎") {
		t.Fatalf("deep-read output missing original prose: %s", out)
	}
	if strings.Contains(out, "relation_type") {
		t.Fatalf("deep-read output leaked graph triple: %s", out)
	}
	// Graph chunk (c2, chunk_order_int=0) is excluded, so the remaining prose
	// chunks must be emitted in reading order by chunk_index: c3 (index 1,
	// "自刎") before c1 (index 2, "马元义").
	if strings.Index(out, "自刎") > strings.Index(out, "马元义") {
		t.Fatalf("deep-read chunks not in reading order: %s", out)
	}
	// Pagination metadata should be present (XML attributes on <chunks>).
	for _, want := range []string{`doc_id="d1"`, `offset="0"`, `limit="20"`, `fetched="2"`} {
		if !strings.Contains(out, want) {
			t.Errorf("deep-read output missing %q: %s", want, out)
		}
	}
}

// TestListChunks_Pagination verifies offset/limit paging over a single
// document's chunks in reading order.
func TestListChunks_Pagination(t *testing.T) {
	fe := &grepFakeEngine{
		regexpChunks: []map[string]interface{}{
			{"id": "c1", "content_with_weight": "chunk A", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(0)},
			{"id": "c2", "content_with_weight": "chunk B", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(1)},
			{"id": "c3", "content_with_weight": "chunk C", "doc_id": "d1", "docnm_kwd": "doc1", "kb_id": "kb1", "chunk_order_int": float64(2)},
		},
	}
	adapter := NewGrepAdapter(fe)
	runtime.SetGrepService(adapter)
	defer runtime.SetGrepService(nil)

	ctx := runtime.WithScope(context.Background(), "t", []string{"kb1"})

	// Page 1: offset 0, limit 2 → chunks A, B.
	out, err := NewListChunksTool().InvokableRun(ctx,
		`{"doc_id":"d1","limit":2,"offset":0}`)
	if err != nil {
		t.Fatalf("InvokableRun page1: %v", err)
	}
	if strings.Contains(out, "chunk C") {
		t.Fatalf("page1 must not contain chunk C: %s", out)
	}
	if !strings.Contains(out, `<pagination next_offset="2"`) {
		t.Fatalf("page1 must advertise next page: %s", out)
	}

	// Page 2: offset 2, limit 2 → chunk C only.
	out, err = NewListChunksTool().InvokableRun(ctx,
		`{"doc_id":"d1","limit":2,"offset":2}`)
	if err != nil {
		t.Fatalf("InvokableRun page2: %v", err)
	}
	if !strings.Contains(out, "chunk C") {
		t.Fatalf("page2 must contain chunk C: %s", out)
	}
	if strings.Contains(out, "chunk A") || strings.Contains(out, "chunk B") {
		t.Fatalf("page2 must not contain earlier chunks: %s", out)
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

	ctx := runtime.WithScope(context.Background(), "t", []string{"kb1"})
	out, err := NewListChunksTool().InvokableRun(ctx, `{"doc_id":"d1"}`)
	if err != nil {
		t.Fatalf("doc_id alone should not error: %v", err)
	}
	if !strings.Contains(out, "原文") {
		t.Fatalf("doc_id alone must return the document's chunks: %s", out)
	}
	// doc_id is mandatory.
	if _, err := NewListChunksTool().InvokableRun(ctx, `{}`); err == nil {
		t.Fatal("expected error when doc_id is missing")
	}
}
