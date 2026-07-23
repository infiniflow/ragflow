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
	"sort"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"ragflow/internal/common"
)

// TestCompile_LogsWhenLegacyNodesPresent exercises the
// decoder-bypass guard in Compile: a Canvas that carries
// LoopItem/IterationItem entries in `Components` (i.e. one that
// never went through dsl.NormalizeForCanvas) must produce a
// visible warning through common.Logger. The guard is
// intentionally a log, not a panic, so internal drivers / legacy
// fixtures can still drive Compile; the log makes the regression
// observable.
//
// The test swaps common.Logger for a buffer-backed encoder so
// we can assert on the structured log message. We don't fail on
// `Compile` itself failing — the legacy fixture graph is
// intentionally minimal and may not compile end-to-end without a
// Begin node; the assertion is strictly about the log surface.
func TestCompile_LogsWhenLegacyNodesPresent(t *testing.T) {
	var buf bytes.Buffer
	prev := common.Logger
	common.Logger = zap.New(
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&buf),
			zapcore.InfoLevel,
		),
	)
	t.Cleanup(func() { common.Logger = prev })

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
	prev := common.Logger
	common.Logger = zap.New(
		zapcore.NewCore(
			zapcore.NewConsoleEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(&buf),
			zapcore.InfoLevel,
		),
	)
	t.Cleanup(func() { common.Logger = prev })

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

// TestWithCheckPointID_OptionSetsField verifies the new R2 option records
// the stable id on CompileOptions. (Compile cannot apply eino's
// compose.WithCheckPointID directly — it's a run-time Option, not a
// GraphCompileOption — so the id is carried on CompiledCanvas for the
// caller to thread to Workflow.Invoke.)
func TestWithCheckPointID_OptionSetsField(t *testing.T) {
	o := CompileOptions{}
	WithCheckPointID("task-42")(&o)
	if o.CheckPointID != "task-42" {
		t.Errorf("WithCheckPointID did not set field: got %q", o.CheckPointID)
	}
}

// TestWithInterruptAfterNonTerminalCpn_OptionSetsField verifies the no-arg
// option flips the internal-compute flag.
func TestWithInterruptAfterNonTerminalCpn_OptionSetsField(t *testing.T) {
	o := CompileOptions{}
	WithInterruptAfterNonTerminalCpn()(&o)
	if !o.InterruptAfterNonTerminal {
		t.Error("WithInterruptAfterNonTerminalCpn did not set flag")
	}
}

// TestComputeNonTerminalCpnIDs covers the core selection rule for the
// no-arg WithInterruptAfterNonTerminalCpn: include every component with
// out-degree > 0, exclude terminal nodes (no downstream) and UserFillUp
// nodes (§4.2.b — they carry their own compose.Interrupt).
func TestComputeNonTerminalCpnIDs(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"n1": {Obj: CanvasComponentObj{ComponentName: "Parser"}, Downstream: []string{"n2"}},
			"n2": {Obj: CanvasComponentObj{ComponentName: "LLM"}, Downstream: []string{"n3"}},
			"n3": {Obj: CanvasComponentObj{ComponentName: "Answer"}, Downstream: nil},                // terminal
			"uf": {Obj: CanvasComponentObj{ComponentName: "UserFillUp"}, Downstream: []string{"n4"}}, // excluded
			"n4": {Obj: CanvasComponentObj{ComponentName: "Answer"}, Downstream: nil},                // terminal
		},
	}
	got := computeNonTerminalCpnIDs(c)
	sort.Strings(got)
	want := []string{"n1", "n2"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("computeNonTerminalCpnIDs = %v, want %v", got, want)
	}
}

// TestCompile_RejectsUserFillUpInResumeMode covers S3 (plan §4.2.b 方案 A):
// when WithInterruptAfterNonTerminalCpn is set (ingestion resume mode), a DSL
// containing a UserFillUp node must be rejected at compile time. A UserFillUp
// node emits its own compose.Interrupt (wait-for-user); the pipeline resume
// loop would catch it via IsInterruptError and auto-resume with nil data,
// silently skipping the human interaction. The guard fires before BuildWorkflow
// so it is independent of whether the rest of the graph compiles.
func TestCompile_RejectsUserFillUpInResumeMode(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin": {Obj: CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}}},
			"uf":    {Obj: CanvasComponentObj{ComponentName: "UserFillUp"}, Downstream: []string{"end"}},
			"end":   {Obj: CanvasComponentObj{ComponentName: "Answer", Params: map[string]any{}}},
		},
	}
	_, err := Compile(context.Background(), c, WithInterruptAfterNonTerminalCpn())
	if err == nil {
		t.Fatal("expected Compile to reject UserFillUp in resume mode, got nil error")
	}
	if !strings.Contains(err.Error(), "UserFillUp") {
		t.Errorf("expected UserFillUp reject message, got %v", err)
	}
}

// TestCompile_PropagatesCheckPointID verifies Compile stores the stable id
// on the returned CompiledCanvas. The assertion only fires when the canvas
// actually compiles end-to-end in this environment (component factories /
// DB may be unavailable in unit scope) — matching the repo convention in
// the other Compile tests that ignore compile errors. The option-side
// contract is independently covered by TestWithCheckPointID_OptionSetsField.
func TestCompile_PropagatesCheckPointID(t *testing.T) {
	c := &Canvas{
		Components: map[string]CanvasComponent{
			"begin":    {Obj: CanvasComponentObj{ComponentName: "Begin", Params: map[string]any{}}},
			"llm:0":    {Obj: CanvasComponentObj{ComponentName: "LLM", Params: map[string]any{}}, Downstream: []string{"answer:0"}},
			"answer:0": {Obj: CanvasComponentObj{ComponentName: "Answer", Params: map[string]any{}}},
		},
	}
	compiled, err := Compile(context.Background(), c, WithCheckPointID("task-9"))
	if err != nil {
		t.Skipf("skipping propagation assertion: canvas did not compile in unit scope: %v", err)
	}
	if compiled == nil {
		t.Skip("skipping propagation assertion: nil CompiledCanvas")
	}
	if compiled.CheckPointID != "task-9" {
		t.Errorf("CompiledCanvas.CheckPointID = %q, want %q", compiled.CheckPointID, "task-9")
	}
}
