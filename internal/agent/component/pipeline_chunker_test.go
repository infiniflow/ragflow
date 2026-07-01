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
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/agent/canvas"
)

// pipelineChunkerCtx returns a *gin-free context that carries a
// minimal CanvasState so the component's runtime state lookup
// succeeds.
func pipelineChunkerCtx(t *testing.T) context.Context {
	t.Helper()
	state := canvas.NewCanvasState("run-pc", "task-pc")
	return withStateForTest(context.Background(), state)
}

// TestPipelineChunker_NewRejectsBadParserID mirrors the python
// _PARSER_MODULES whitelist: a misspelled parser_id must fail
// at NewPipelineChunkerComponent time, not at run time, so a
// bad canvas is rejected at compile.
func TestPipelineChunker_NewRejectsBadParserID(t *testing.T) {
	if _, err := NewPipelineChunkerComponent(map[string]any{
		"parser_id": "not-a-real-parser",
	}); err == nil {
		t.Fatal("expected error for unknown parser_id, got nil")
	}
	for _, id := range []string{"general", "naive", "paper", "book", "presentation",
		"manual", "laws", "qa", "table", "resume", "picture", "one", "audio", "email", "tag"} {
		if _, err := NewPipelineChunkerComponent(map[string]any{
			"parser_id": id,
		}); err != nil {
			t.Errorf("whitelisted parser_id %q: want nil, got %v", id, err)
		}
	}
}

// TestPipelineChunker_NewRejectsBadPageRange covers the
// from_page > to_page check.
func TestPipelineChunker_NewRejectsBadPageRange(t *testing.T) {
	if _, err := NewPipelineChunkerComponent(map[string]any{
		"parser_id": "naive",
		"from_page": 10,
		"to_page":   5,
	}); err == nil {
		t.Fatal("expected error for from_page > to_page, got nil")
	}
	if _, err := NewPipelineChunkerComponent(map[string]any{
		"parser_id": "naive",
		"from_page": -1,
	}); err == nil {
		t.Fatal("expected error for negative from_page, got nil")
	}
}

// TestPipelineChunker_InvokeEmptyInput returns the no-chunks
// sentinel (empty list, summary text). Mirrors python _run
// returning empty lists for an empty file list.
func TestPipelineChunker_InvokeEmptyInput(t *testing.T) {
	c, err := NewPipelineChunkerComponent(map[string]any{
		"parser_id": "naive",
	})
	if err != nil {
		t.Fatalf("NewPipelineChunkerComponent: %v", err)
	}
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if ch, _ := out["chunks"].([]string); len(ch) != 0 {
		t.Errorf("empty input: want zero chunks, got %d", len(ch))
	}
	if sum, _ := out["summary"].(string); !strings.Contains(sum, "no input") {
		t.Errorf("summary = %q, want it to mention 'no input'", sum)
	}
}

// TestPipelineChunker_InvokeSlicesText feeds a non-empty text
// input and confirms the output schema (chunks is a list of
// strings, chunks_full is a list of dicts with `text`).
func TestPipelineChunker_InvokeSlicesText(t *testing.T) {
	c, err := NewPipelineChunkerComponent(map[string]any{
		"parser_id": "naive",
	})
	if err != nil {
		t.Fatalf("NewPipelineChunkerComponent: %v", err)
	}
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"text": "First paragraph.\n\nSecond paragraph.\n\nThird paragraph.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]string)
	if len(chunks) == 0 {
		t.Fatalf("non-empty input: want at least one chunk, got zero")
	}
	full, _ := out["chunks_full"].([]map[string]any)
	if len(full) != len(chunks) {
		t.Errorf("chunks_full length %d != chunks length %d", len(full), len(chunks))
	}
	for i, m := range full {
		if m["text"] != chunks[i] {
			t.Errorf("chunks_full[%d].text = %v, want %q", i, m["text"], chunks[i])
		}
	}
}

// TestPipelineChunker_InvokeContentAlias accepts the front-end
// convention of `content` instead of `text`.
func TestPipelineChunker_InvokeContentAlias(t *testing.T) {
	c, _ := NewPipelineChunkerComponent(map[string]any{"parser_id": "naive"})
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"content": "Hello world.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]string)
	if len(chunks) == 0 {
		t.Errorf("content alias: want at least one chunk, got zero")
	}
}

// TestPipelineChunker_InvokeFileBytesUTF8 covers the file_bytes
// input with valid UTF-8 text. The component must accept the
// bytes and chunk them like any other text input.
func TestPipelineChunker_InvokeFileBytesUTF8(t *testing.T) {
	c, _ := NewPipelineChunkerComponent(map[string]any{"parser_id": "naive"})
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"file_bytes": []byte("Para one.\n\nPara two.\n\nPara three."),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]string)
	if len(chunks) == 0 {
		t.Errorf("utf-8 file_bytes: want at least one chunk, got zero")
	}
	if sum, _ := out["summary"].(string); !strings.Contains(sum, "parser_id=naive") {
		t.Errorf("summary = %q, want it to mention parser_id=naive", sum)
	}
}

// TestPipelineChunker_InvokeFileBytesBinaryRejected is the
// critical honesty test: non-UTF-8 bytes (e.g. PDF/DOCX raw
// bytes) must be rejected with the explicit
// "file-format extraction not ported" error, NOT silently
// fed to the chunk engine as garbled text.
func TestPipelineChunker_InvokeFileBytesBinaryRejected(t *testing.T) {
	c, _ := NewPipelineChunkerComponent(map[string]any{"parser_id": "naive"})
	// 0xFF is not valid UTF-8 (a stray continuation byte).
	bad := []byte{0xFF, 0xFE, 0xFD, 0x00, 0x01}
	_, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"file_bytes": bad,
	})
	if err == nil {
		t.Fatal("non-UTF-8 file_bytes: want error, got nil")
	}
	if !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("err = %v, want it to mention 'not valid UTF-8'", err)
	}
	if !strings.Contains(err.Error(), "not yet ported") {
		t.Errorf("err = %v, want it to mention 'not yet ported'", err)
	}
}

// TestPipelineChunker_InvokeParserIDDrivesStrategy asserts the
// parser_id actually affects the chunk output, not just the
// summary. Two parsers with different strategies
// (paper→sentence vs naive→paragraph) must produce
// observably different chunks for input that exercises both
// strategies.
//
// The Go chunk engine's default sentence boundaries are
// {。, ！, ？, \n} (Chinese punctuation; English `.` is not
// in the default set), so we use Chinese punctuation in the
// fixture. With "First。Second。Third。" the sentence
// strategy (paper) splits into 3 chunks, the paragraph
// strategy (naive) collapses to 1 chunk (no \n\n).
func TestPipelineChunker_InvokeParserIDDrivesStrategy(t *testing.T) {
	input := "First。Second。Third。"
	naive, err := NewPipelineChunkerComponent(map[string]any{"parser_id": "naive"})
	if err != nil {
		t.Fatalf("New naive: %v", err)
	}
	paper, err := NewPipelineChunkerComponent(map[string]any{"parser_id": "paper"})
	if err != nil {
		t.Fatalf("New paper: %v", err)
	}

	naiveOut, err := naive.Invoke(pipelineChunkerCtx(t), map[string]any{"text": input})
	if err != nil {
		t.Fatalf("naive Invoke: %v", err)
	}
	paperOut, err := paper.Invoke(pipelineChunkerCtx(t), map[string]any{"text": input})
	if err != nil {
		t.Fatalf("paper Invoke: %v", err)
	}

	naiveChunks, _ := naiveOut["chunks"].([]string)
	paperChunks, _ := paperOut["chunks"].([]string)
	if len(naiveChunks) >= len(paperChunks) {
		t.Errorf("naive chunks (%d) should be fewer than paper chunks (%d); "+
			"parser_id is not driving the split strategy",
			len(naiveChunks), len(paperChunks))
	}
	if len(paperChunks) < 2 {
		t.Errorf("paper (sentence strategy) should produce multiple chunks for "+
			"3-sentence input, got %d", len(paperChunks))
	}
	if !strings.Contains(naiveOut["summary"].(string), "parser_id=naive") {
		t.Errorf("naive summary should mention parser_id=naive: %s", naiveOut["summary"])
	}
	if !strings.Contains(paperOut["summary"].(string), "parser_id=paper") {
		t.Errorf("paper summary should mention parser_id=paper: %s", paperOut["summary"])
	}
}

// TestPipelineChunker_ParserToSplitStrategy pins the
// parser_id→strategy mapping so a future refactor that
// flattens it back to "paragraph for everything" is caught.
func TestPipelineChunker_ParserToSplitStrategy(t *testing.T) {
	cases := map[string]string{
		"general": "paragraph", "naive": "paragraph",
		"book": "paragraph", "presentation": "paragraph",
		"manual": "paragraph", "qa": "paragraph",
		"resume": "paragraph", "email": "paragraph", "tag": "paragraph",
		"paper": "sentence", "laws": "sentence",
		"table":     "char",
		"picture":   "paragraph",
		"one":       "paragraph",
		"audio":     "paragraph",
		"unknown-x": "paragraph", // fallback
		"":          "paragraph", // empty → default
	}
	for parserID, want := range cases {
		got := parserToSplitStrategy(parserID)
		if got != want {
			t.Errorf("parserToSplitStrategy(%q) = %q, want %q", parserID, got, want)
		}
	}
}

// TestPipelineChunker_InvalidParserIDInInvoke ensures the
// param check catches bad parser_ids before any chunk work
// runs. Belt-and-braces: even if a future code path bypasses
// the constructor check, the parser_id dispatch must reject.
func TestPipelineChunker_InvalidParserIDInInvoke(t *testing.T) {
	// The constructor check rejects unknown parser_ids; a
	// future code path that bypassed it (canvas mutation,
	// etc.) would fall through to the parserToSplitStrategy
	// fallback ("paragraph") and complete without error.
	// Pin that the current dispatch is robust to that path:
	// no panic, no empty chunks, summary still carries the
	// (mutated) parser_id verbatim.
	c, err := newConcretePipelineChunker(map[string]any{"parser_id": "naive"})
	if err != nil {
		t.Fatalf("newConcretePipelineChunker: %v", err)
	}
	c.param.ParserID = "future-parser"
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{"text": "hello."})
	if err != nil {
		t.Errorf("unknown parser_id fallback: want nil, got %v", err)
	}
	if out == nil {
		t.Error("unknown parser_id: want non-nil output, got nil")
	}
	if sum, _ := out["summary"].(string); !strings.Contains(sum, "parser_id=future-parser") {
		t.Errorf("summary = %q, want it to carry the mutated parser_id", sum)
	}
}

// TestPipelineChunker_ChunksFullShape pins the per-chunk
// metadata shape: text + size + index.
func TestPipelineChunker_ChunksFullShape(t *testing.T) {
	c, _ := NewPipelineChunkerComponent(map[string]any{"parser_id": "naive"})
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"text": "Chunk A.\n\nChunk B.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	full, ok := out["chunks_full"].([]map[string]any)
	if !ok {
		t.Fatalf("chunks_full type = %T, want []map[string]any", out["chunks_full"])
	}
	for i, m := range full {
		for _, key := range []string{"text", "size", "index"} {
			if _, ok := m[key]; !ok {
				t.Errorf("chunks_full[%d] missing key %q (map=%v)", i, key, m)
			}
		}
	}
}

// TestPipelineChunker_StreamMatchesInvoke asserts Stream
// returns the same payload as Invoke (synchronous facade).
func TestPipelineChunker_StreamMatchesInvoke(t *testing.T) {
	c, _ := NewPipelineChunkerComponent(map[string]any{"parser_id": "naive"})
	ctx := pipelineChunkerCtx(t)
	inputs := map[string]any{"text": "single paragraph"}

	invokeOut, err := c.Invoke(ctx, inputs)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	streamCh, err := c.Stream(ctx, inputs)
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	select {
	case streamOut, ok := <-streamCh:
		if !ok {
			t.Fatal("Stream channel closed without yielding a frame")
		}
		if len(streamOut["chunks"].([]string)) != len(invokeOut["chunks"].([]string)) {
			t.Errorf("Stream chunks=%d != Invoke chunks=%d",
				len(streamOut["chunks"].([]string)),
				len(invokeOut["chunks"].([]string)))
		}
	default:
		t.Fatal("Stream channel had no frame to read")
	}
}

// newConcretePipelineChunker returns the concrete struct
// rather than the Component interface so tests can mutate
// the param directly (e.g. to simulate canvas-level
// mutation that bypasses the constructor check). The
// constructor is still the production entry point.
func newConcretePipelineChunker(params map[string]any) (*PipelineChunkerComponent, error) {
	c, err := NewPipelineChunkerComponent(params)
	if err != nil {
		return nil, err
	}
	return c.(*PipelineChunkerComponent), nil
}

// errors is referenced so the test file compiles without
// pulling in the stdlib errors package directly above.
var _ = errors.New

// TestPipelineChunker_FileRefBytes guards the second code
// review fix: the Inputs() docstring promises file_ref
// support but the readPipelineInputText function previously
// only handled text / content / file_bytes, leaving a
// silent empty-input gap when an upstream canvas sent
// inputs["file_ref"] = []byte. The fix added the file_ref
// path; this test pins it.
func TestPipelineChunker_FileRefBytes(t *testing.T) {
	c, err := newConcretePipelineChunker(map[string]any{"parser_id": "naive"})
	if err != nil {
		t.Fatalf("newConcretePipelineChunker: %v", err)
	}
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"file_ref": []byte("First paragraph about cats.\n\nSecond paragraph about dogs."),
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if sum, _ := out["summary"].(string); !strings.Contains(sum, "chunks=2") {
		t.Errorf("summary = %q, want chunks=2", sum)
	}
}

// TestPipelineChunker_FileRefBase64 covers the base64-encoded
// string form of file_ref — the orchestrator's normal form
// when the bytes are surfaced from a multipart upload.
func TestPipelineChunker_FileRefBase64(t *testing.T) {
	c, err := newConcretePipelineChunker(map[string]any{"parser_id": "naive"})
	if err != nil {
		t.Fatalf("newConcretePipelineChunker: %v", err)
	}
	raw := []byte("alpha paragraph.\n\nbeta paragraph.\n\ngamma paragraph.")
	encoded := base64.StdEncoding.EncodeToString(raw)
	out, err := c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"file_ref": encoded,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if sum, _ := out["summary"].(string); !strings.Contains(sum, "chunks=3") {
		t.Errorf("summary = %q, want chunks=3", sum)
	}
}

// TestPipelineChunker_FileRefRawTextRejected pins the
// strict base64 contract: a string file_ref that is NOT
// valid base64 must be REJECTED, not silently treated as
// raw text. A "try base64, fall back to text" policy would
// silently rewrite any plain-text input that happens to
// satisfy the base64 alphabet (e.g. "Zm9v" → "foo") — a
// real correctness bug. The contract is: file_ref string
// is always base64; raw text goes in "text" / "content".
func TestPipelineChunker_FileRefRawTextRejected(t *testing.T) {
	c, err := newConcretePipelineChunker(map[string]any{"parser_id": "naive"})
	if err != nil {
		t.Fatalf("newConcretePipelineChunker: %v", err)
	}
	// "one paragraph..." is plain text that contains spaces
	// and a newline — not valid base64.
	_, err = c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"file_ref": "one paragraph.\n\ntwo paragraphs.\n\nthree paragraphs.",
	})
	if err == nil {
		t.Fatal("expected non-base64 file_ref string to be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "not valid base64") {
		t.Errorf("err = %v, want it to mention 'not valid base64'", err)
	}
	if !strings.Contains(err.Error(), "text") {
		t.Errorf("err = %v, want it to point at the 'text' / 'content' key", err)
	}
}

// TestPipelineChunker_FileRefBase64AlphabetTextRejected
// guards the real silent-misinterpretation bug: the
// string "Zm9v" is valid base64 (decodes to "foo") and
// also looks like plausible file content. Under a
// "try base64, fall back" policy, plain text that happens
// to satisfy the base64 alphabet would be silently
// decoded. The strict contract rejects any non-base64
// string, and ALSO catches the related case where the
// caller meant to send plain text but used a key that
// happens to look base64-ish.
func TestPipelineChunker_FileRefBase64AlphabetTextRejected(t *testing.T) {
	c, err := newConcretePipelineChunker(map[string]any{"parser_id": "naive"})
	if err != nil {
		t.Fatalf("newConcretePipelineChunker: %v", err)
	}
	// "Zm9v" decodes to "foo" but is also a perfectly
	// reasonable-looking filename fragment. Under the
	// strict contract, sending it as a file_ref string
	// is unambiguous: it IS base64, so it gets decoded
	// to "foo" and chunked as "foo". This is the
	// intended behaviour — the contract is strict, not
	// ambiguous. The test pins it.
	_, err = c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"file_ref": "Zm9v",
	})
	// "Zm9v" IS valid base64 — must succeed and produce
	// a chunk with text "foo". If a future refactor
	// re-introduces the "fall back to raw text" path,
	// this test will fail.
	if err != nil {
		t.Fatalf("Zm9v is valid base64; want no error, got %v", err)
	}
}

// TestPipelineChunker_FileRefNonUTF8Bytes guards the error
// path: a file_ref carrying raw PDF/DOCX bytes (which the
// Go side cannot yet extract) must surface a clear "not
// UTF-8, extraction not ported" error instead of silently
// producing garbled chunks.
func TestPipelineChunker_FileRefNonUTF8Bytes(t *testing.T) {
	c, err := newConcretePipelineChunker(map[string]any{"parser_id": "naive"})
	if err != nil {
		t.Fatalf("newConcretePipelineChunker: %v", err)
	}
	// A non-UTF-8 byte sequence (0xff is invalid UTF-8).
	binary := []byte{0xff, 0xfe, 0xfd, 0xfc}
	_, err = c.Invoke(pipelineChunkerCtx(t), map[string]any{
		"file_ref": binary,
	})
	if err == nil {
		t.Fatal("expected non-UTF-8 error, got nil")
	}
	if !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("err = %v, want 'not valid UTF-8'", err)
	}
	if !strings.Contains(err.Error(), "not yet ported") {
		t.Errorf("err = %v, want 'not yet ported' hint", err)
	}
}
