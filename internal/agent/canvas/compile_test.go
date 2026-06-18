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

package canvas

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

// TestCompile_LogsWhenLegacyNodesPresent exercises the
// decoder-bypass guard in Compile: a Canvas that carries
// LoopItem/IterationItem entries in `Components` (i.e. one that
// never went through dsl.NormalizeForCanvas) must produce a
// visible stderr warning. The guard is intentionally a log, not
// a panic, so internal drivers / legacy fixtures can still drive
// Compile; the log makes the regression observable.
//
// The test redirects log output to a buffer and asserts the
// expected substring. We don't fail on `Compile` itself failing —
// the legacy fixture graph is intentionally minimal and may not
// compile end-to-end without a Begin node; the assertion is
// strictly about the log surface.
func TestCompile_LogsWhenLegacyNodesPresent(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	c := &Canvas{
		Components: map[string]CanvasComponent{
			"Loop:abc": {
				Obj: CanvasComponentObj{ComponentName: "Loop", Params: map[string]any{}},
			},
			"LoopItem:def": {
				Obj:        CanvasComponentObj{ComponentName: "LoopItem", Params: map[string]any{}},
				Downstream: []string{"Body:1"},
			},
			"Body:1": {
				Obj: CanvasComponentObj{ComponentName: "Message", Params: map[string]any{}},
			},
		},
	}

	// Compile may return an error from downstream BuildWorkflow —
	// we ignore it; the assertion is on the log line.
	_, _ = Compile(context.Background(), c)

	got := buf.String()
	if !strings.Contains(got, "LoopItem/IterationItem") {
		t.Errorf("expected legacy-node log warning, got %q", got)
	}
	if !strings.Contains(got, "bypassed dsl.NormalizeForCanvas") {
		t.Errorf("expected bypass warning, got %q", got)
	}
}

// TestCompile_NoLogOnCleanCanvas is the negative case: a Canvas
// whose components carry only modern names must NOT trip the
// guard. This guards against an over-eager regex that fires on
// every Compile.
func TestCompile_NoLogOnCleanCanvas(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {
				Obj: CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}},
			},
			"llm:0": {
				Obj:        CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{}},
				Downstream: []string{},
			},
		},
	}

	// We don't fail on Compile's own error (it may fail for many
	// reasons unrelated to legacy names); the assertion is on the
	// absence of the legacy log line.
	_, _ = Compile(context.Background(), c)

	got := buf.String()
	if strings.Contains(got, "LoopItem/IterationItem") {
		t.Errorf("unexpected legacy-node log on clean canvas: %q", got)
	}
}
