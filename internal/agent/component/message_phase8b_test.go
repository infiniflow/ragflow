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
	"strings"
	"testing"

	"ragflow/internal/agent/audio"
	"ragflow/internal/agent/canvas"
)

// fakeSynthesizer is a programmable audio.Synthesizer for
// integration tests. It records the request and returns the
// canned response / error configured at construction time.
type fakeSynthesizer struct {
	resp *audio.SynthesizeResponse
	err  error
	got  *audio.SynthesizeRequest
}

func (f *fakeSynthesizer) Synthesize(_ context.Context, req audio.SynthesizeRequest) (*audio.SynthesizeResponse, error) {
	if f.got != nil {
		*f.got = req
	}
	return f.resp, f.err
}

// TestMessage_OutputFormatParam: when the DSL declares
// output_format=html, the rendered body is HTML-wrapped.
func TestMessage_OutputFormatParam(t *testing.T) {
	c, _ := NewMessageComponent(map[string]any{
		"text":          "hello",
		"output_format": "html",
	})
	state := canvas.NewCanvasState("r1", "t1")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"text": "hello", "stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["content"].(string)
	if !strings.Contains(got, `<div class="rf-message">hello`) {
		t.Errorf("expected HTML wrap, got %q", got)
	}
}

// TestMessage_OutputFormatInputOverride: the inputs["output_format"]
// key wins over the per-instance declaration.
func TestMessage_OutputFormatInputOverride(t *testing.T) {
	c, _ := NewMessageComponent(map[string]any{
		"text":          "hi",
		"output_format": "html",
	})
	state := canvas.NewCanvasState("r1", "t1")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"text":          "hi",
		"output_format": "plain",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := out["content"].(string)
	if got != "hi" {
		t.Errorf("expected plain passthrough, got %q", got)
	}
}

// TestMessage_DownloadsExtraction: a {doc_id, filename, mime_type}
// entry in inputs is surfaced under outputs["downloads"].
func TestMessage_DownloadsExtraction(t *testing.T) {
	c, _ := NewMessageComponent(map[string]any{"text": "see attachment"})
	state := canvas.NewCanvasState("r1", "t1")
	ctx := withStateForTest(context.Background(), state)

	dl := map[string]any{
		"doc_id":    "d-1",
		"filename":  "report.csv",
		"mime_type": "text/csv",
		"url":       "/dl/d-1",
	}
	out, err := c.Invoke(ctx, map[string]any{
		"text":   "see attachment",
		"stream": false,
		"attach": dl,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	dls, ok := out["downloads"].([]DownloadInfo)
	if !ok {
		t.Fatalf("downloads key missing or wrong type: %T", out["downloads"])
	}
	if len(dls) != 1 || dls[0].DocID != "d-1" || dls[0].Filename != "report.csv" {
		t.Errorf("unexpected downloads: %+v", dls)
	}
	// With default plain format, the rendered body does NOT embed
	// the download descriptor (Python's _stringify_message_value
	// returns "" for download-only inputs); we only assert the
	// downloads key is populated.
	if !strings.Contains(out["content"].(string), "see attachment") {
		t.Errorf("body not preserved: %q", out["content"])
	}
}

// TestMessage_AutoPlay_NoEngine: when auto_play is enabled but no
// real synthesizer is registered, the textual content is still
// returned and outputs["audio_error"] surfaces the deferred
// state.
func TestMessage_AutoPlay_NoEngine(t *testing.T) {
	// Force the stub (the default) so the test does not depend on
	// what previous tests installed.
	audio.SetSynthesizer(nil)
	c, _ := NewMessageComponent(map[string]any{
		"text":      "hi",
		"auto_play": true,
	})
	state := canvas.NewCanvasState("r1", "t1")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"text": "hi", "stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if _, ok := out["audio"]; ok {
		t.Errorf("expected no audio key, got %+v", out["audio"])
	}
	if _, ok := out["audio_error"]; !ok {
		t.Errorf("expected audio_error key, got %+v", out)
	}
}

// TestMessage_AutoPlay_Success: with a fake synthesizer that
// returns bytes, outputs["audio"] is populated with the right
// envelope shape.
func TestMessage_AutoPlay_Success(t *testing.T) {
	got := audio.SynthesizeRequest{}
	prev := audio.GetSynthesizer()
	audio.SetSynthesizer(&fakeSynthesizer{
		resp: &audio.SynthesizeResponse{Audio: []byte("abc"), MediaType: "audio/mpeg"},
		got:  &got,
	})
	defer audio.SetSynthesizer(prev)

	c, _ := NewMessageComponent(map[string]any{
		"text":      "hi",
		"auto_play": "gtts",
		"voice":     "en",
		"lang":      "en",
	})
	state := canvas.NewCanvasState("r1", "t1")
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{"text": "hi", "stream": false})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	env, ok := out["audio"].(map[string]any)
	if !ok {
		t.Fatalf("expected audio envelope, got %T", out["audio"])
	}
	if env["media_type"] != "audio/mpeg" {
		t.Errorf("media_type: got %v, want audio/mpeg", env["media_type"])
	}
	if got.Engine != audio.EngineGTTS {
		t.Errorf("engine forwarded: got %q, want gtts", got.Engine)
	}
	if got.Voice != "en" {
		t.Errorf("voice forwarded: got %q, want en", got.Voice)
	}
	if got.Lang != "en" {
		t.Errorf("lang forwarded: got %q, want en", got.Lang)
	}
}

// TestMessage_MemorySave_NoService: with memory_save=true and no
// MemorySaver registered, the textual content flows and
// outputs["memory_error"] surfaces the deferred state.
func TestMessage_MemorySave_NoService(t *testing.T) {
	SetMemorySaver(nil)
	c, _ := NewMessageComponent(map[string]any{"text": "hi"})
	state := canvas.NewCanvasState("run-x", "task-x")
	state.Sys["query"] = "what?"
	ctx := withStateForTest(context.Background(), state)

	out, err := c.Invoke(ctx, map[string]any{
		"text":        "hi",
		"stream":      false,
		"memory_save": true,
		"memory_ids":  []string{"m1"},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, _ := out["content"].(string); got != "hi" {
		t.Errorf("content: got %q, want 'hi'", got)
	}
	if _, ok := out["memory_error"]; !ok {
		t.Errorf("expected memory_error key, got %+v", out)
	}
}

// TestMessage_MemorySave_Success: with a custom MemorySaver,
// Save is called with the right fields.
func TestMessage_MemorySave_Success(t *testing.T) {
	var saved MemorySaveRequest
	saveFn := memSaverFunc(func(_ context.Context, req MemorySaveRequest) error {
		saved = req
		return nil
	})
	SetMemorySaver(&saveFn)
	defer SetMemorySaver(nil)

	c, _ := NewMessageComponent(map[string]any{"text": "hi"})
	state := canvas.NewCanvasState("run-y", "task-y")
	state.Sys["query"] = "what?"
	ctx := withStateForTest(context.Background(), state)

	_, err := c.Invoke(ctx, map[string]any{
		"text":        "hi",
		"stream":      false,
		"memory_save": true,
		"memory_ids":  []string{"m1", "m2"},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(saved.MemoryIDs) != 2 || saved.MemoryIDs[0] != "m1" || saved.MemoryIDs[1] != "m2" {
		t.Errorf("MemoryIDs: %+v", saved.MemoryIDs)
	}
	if saved.AgentID != "task-y" {
		t.Errorf("AgentID: got %q, want task-y", saved.AgentID)
	}
	if saved.SessionID != "run-y" {
		t.Errorf("SessionID: got %q, want run-y", saved.SessionID)
	}
	if saved.UserInput != "what?" {
		t.Errorf("UserInput: got %q, want what?", saved.UserInput)
	}
	if saved.AgentResponse != "hi" {
		t.Errorf("AgentResponse: got %q, want hi", saved.AgentResponse)
	}
}

// memSaverFunc adapts a closure to the MemorySaver interface so
// tests can record the call inline.
type memSaverFunc func(ctx context.Context, req MemorySaveRequest) error

func (f memSaverFunc) Save(ctx context.Context, req MemorySaveRequest) error {
	return f(ctx, req)
}
