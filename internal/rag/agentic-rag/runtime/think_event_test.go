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

package runtime

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"ragflow/internal/common"
)

// captureLogger routes the package developer log into an observer for the duration of the
// test, so a stage line can be asserted on its message and fields.
func captureLogger(t *testing.T) *observer.ObservedLogs {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	prev := common.Logger
	common.Logger = zap.New(core)
	t.Cleanup(func() { common.Logger = prev })
	return logs
}

// stageLogText renders the developer-log half of the captured stage lines in the shape the old
// *log.Logger wrote them ("[stage] detail"), so a test that read that buffer can keep asserting on
// text.
func stageLogText(logs *observer.ObservedLogs) string {
	var b strings.Builder
	for _, e := range logs.All() {
		cm := e.ContextMap()
		detail, _ := cm["detail"].(string)
		if stage, ok := cm["stage"].(string); ok {
			b.WriteString("[" + stage + "] " + detail)
		} else {
			b.WriteString(e.Message)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// TestStepReporterStageDeliversBothProjections is the core contract of the
// explicit-step design: one call writes the developer log line AND delivers the
// same sentence to the think block and to structured clients.
func TestStepReporterStageDeliversBothProjections(t *testing.T) {
	logs := captureLogger(t)
	var text []string
	var events []ThinkEvent
	r := StepReporter{
		Text:   func(line string) { text = append(text, line) },
		Events: func(ev ThinkEvent) { events = append(events, ev) },
	}

	r.Stage("Planner", "decomposed into %d fan-out(s)", 3)

	if got := logs.FilterMessage("stage line").All(); len(got) != 1 ||
		got[0].ContextMap()["stage"] != "Planner" || got[0].ContextMap()["detail"] != "decomposed into 3 fan-out(s)" {
		t.Errorf("developer log = %v, want the stage line", logs.All())
	}
	if len(text) != 1 || text[0] != "[Planner] decomposed into 3 fan-out(s)"+thinkLineBreak {
		t.Errorf("text projection = %#v, want the sentence with a block separator", text)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v, want one stage event", events)
	}
	if ev := events[0]; ev.Kind != thinkKindStage || ev.Stage != "Planner" ||
		ev.Summary != "[Planner] decomposed into 3 fan-out(s)" {
		t.Errorf("event = %#v", ev)
	}
}

// TestStepReporterStageDetailSplitsTheTwoAudiences pins StageLineDetail, the
// escape hatch the search legs use: the log records `detail` (identifiers kept)
// while the think block and the stage event carry `summary`. Both strings are
// passed in from ONE call site, so the two audiences can never be told different
// things about what a step saw.
func TestStepReporterStageDetailSplitsTheTwoAudiences(t *testing.T) {
	const docID = "95a7aee3f11143e69dc9fa5b7bad3a14"
	logs := captureLogger(t)
	var text []string
	var events []ThinkEvent
	r := StepReporter{
		Text:   func(line string) { text = append(text, line) },
		Events: func(ev ThinkEvent) { events = append(events, ev) },
	}

	r.StageLineDetail("Hybrid search",
		`Found 5 passages in 1 document for "曹操".`,
		`"曹操" -> 5 chunk(s): `+docID+`:5chunk(4963chars)`)

	entries := logs.FilterMessage("stage line").All()
	if len(entries) != 1 || !strings.Contains(entries[0].ContextMap()["detail"].(string), docID+":5chunk(4963chars)") {
		t.Errorf("developer log = %v, want the per-document breakdown", logs.All())
	}
	want := `[Hybrid search] Found 5 passages in 1 document for "曹操".`
	if len(text) != 1 || text[0] != want+thinkLineBreak {
		t.Errorf("text projection = %#v, want %q", text, want)
	}
	if len(events) != 1 || events[0].Summary != want {
		t.Errorf("event = %#v, want the reader's sentence", events)
	}
	if strings.Contains(text[0], docID) || strings.Contains(entries[0].ContextMap()["detail"].(string), "Found 5 passages") {
		t.Error("the two projections must not swap their sentences")
	}

	// StageLine is this same function with one message: its projections cannot
	// diverge either.
	core2, logs2 := observer.New(zapcore.DebugLevel)
	common.Logger = zap.New(core2)
	text = nil
	r.StageLine("Planner", "one sentence")
	second := logs2.FilterMessage("stage line").All()
	if len(second) != 1 || second[0].ContextMap()["detail"] != "one sentence" ||
		len(text) != 1 || text[0] != "[Planner] one sentence"+thinkLineBreak {
		t.Errorf("StageLine split its audiences: log=%v text=%#v", second, text)
	}
}

// TestStepReporterIndentsNestedSteps pins the nesting the trace shows: a step
// reported beneath another one is indented in the TEXT projection, while the log
// (the greppable, Python-parity record) and the event Summary (a structured client
// gets a step tree, not entities) stay clean.
//
// The depth rides on the context, so a layer opts in by calling Nested around the
// work it launches after reporting its own step.
func TestStepReporterIndentsNestedSteps(t *testing.T) {
	logs := captureLogger(t)
	var text []string
	var events []ThinkEvent
	r := StepReporter{
		Text:   func(line string) { text = append(text, line) },
		Events: func(ev ThinkEvent) { events = append(events, ev) },
	}
	ctx := WithSteps(context.Background(), r)

	StepsFrom(ctx).StageLine("Prefetch", "The plan lists 5 sub-questions; searching 3 opening queries up front.")
	StepsFrom(Nested(ctx)).StageLine("BM25 search", `Found 5 passages in 1 document for "q".`)

	wantTop := "[Prefetch] The plan lists 5 sub-questions; searching 3 opening queries up front." + thinkLineBreak
	wantNested := StepIndentUnit + `[BM25 search] Found 5 passages in 1 document for "q".` + thinkLineBreak
	if len(text) != 2 || text[0] != wantTop || text[1] != wantNested {
		t.Errorf("text = %#v, want %q then %q", text, wantTop, wantNested)
	}
	for _, entry := range logs.FilterMessage("stage line").All() {
		detail, _ := entry.ContextMap()["detail"].(string)
		if strings.Contains(detail, StepIndentUnit) || strings.Contains(detail, "&nbsp;") {
			t.Errorf("the log must stay flush, got %q", detail)
		}
	}
	for _, ev := range events {
		if strings.Contains(ev.Summary, "&nbsp;") {
			t.Errorf("event summary carries indentation entities: %q", ev.Summary)
		}
	}

	// The cap: a five-level nesting (outer rag → graph → round → tool → leg) must not
	// walk off the right edge, so past stepMaxDepth the indent stops growing.
	deep := ctx
	for i := 0; i < stepMaxDepth+3; i++ {
		deep = Nested(deep)
	}
	StepsFrom(deep).StageLine("Deep", "x")
	if got, want := text[len(text)-1], strings.Repeat(StepIndentUnit, stepMaxDepth)+"[Deep] x"+thinkLineBreak; got != want {
		t.Errorf("deep text = %q, want %q", got, want)
	}
}

// TestStepReporterEmitLeavesTheLogToTheProducer pins the split for tools: the
// executor logs its own two sentences (Python tool_decorator parity), so Emit
// must not write them again — but it still delivers both user projections.
func TestStepReporterEmitLeavesTheLogToTheProducer(t *testing.T) {
	logs := captureLogger(t)
	var text []string
	var events []ThinkEvent
	r := StepReporter{
		Text:   func(line string) { text = append(text, line) },
		Events: func(ev ThinkEvent) { events = append(events, ev) },
	}

	r.Emit(ThinkEvent{
		Kind: ThinkKindToolResult, Stage: "Function tool", Tool: "retrieve",
		Status: StatusOK, Results: 3, Documents: 2, Sources: []string{"c1", "c2"},
		DurationMS: 12, Cause: "index not found",
		Summary: "[Function tool] The retrieve tool returned 3 results from 2 documents.",
	})

	if n := logs.Len(); n != 0 {
		t.Errorf("Emit must not write the developer log, got %v", logs.All())
	}
	if len(text) != 1 || !strings.HasSuffix(text[0], thinkLineBreak) {
		t.Errorf("text projection = %#v", text)
	}
	ev := events[0]
	if ev.Tool != "retrieve" || ev.Status != StatusOK || ev.Results != 3 || ev.Documents != 2 ||
		ev.DurationMS != 12 || ev.Cause != "index not found" || len(ev.Sources) != 2 {
		t.Errorf("event lost fields: %#v", ev)
	}
}

// TestStepReporterOptionalProjections pins that either projection may be absent:
// a run with no think block still logs, a run with no structured client still narrates, and
// the zero reporter is inert rather than a crash.
func TestStepReporterOptionalProjections(t *testing.T) {
	var events []ThinkEvent
	eventsOnly := StepReporter{Events: func(ev ThinkEvent) { events = append(events, ev) }}
	eventsOnly.Stage("SCA", "verdict=%s", "SUFFICIENT") // no logger argument any more
	eventsOnly.Emit(ThinkEvent{Summary: "x"})
	if len(events) != 2 {
		t.Errorf("events = %#v, want both steps", events)
	}

	var text []string
	logOnly := StepReporter{Text: func(line string) { text = append(text, line) }}
	logOnly.Emit(ThinkEvent{Summary: "y"})
	if len(text) != 1 {
		t.Errorf("text = %#v, want the step", text)
	}

	var zero StepReporter
	if zero.Enabled() {
		t.Error("the zero reporter must report itself disabled")
	}
	zero.Stage("Planner", "x")          // must not panic
	zero.Emit(ThinkEvent{Summary: "x"}) // must not panic
}

// TestStepReporterSurvivesPanickingText mirrors Python's bare try/except around
// the sink: a broken consumer must never break the run, and the structured
// projection must still be delivered.
func TestStepReporterSurvivesPanickingText(t *testing.T) {
	var events []ThinkEvent
	r := StepReporter{
		Text:   func(string) { panic("think block exploded") },
		Events: func(ev ThinkEvent) { events = append(events, ev) },
	}

	r.Stage("Planner", "x") // must not panic
	r.Emit(ThinkEvent{Summary: "y"})

	if len(events) != 2 {
		t.Errorf("events = %#v, want both steps despite the broken text sink", events)
	}
}

// TestStepsFromCtx pins the per-request carrier: a bound reporter comes back,
// an unbound ctx yields the inert zero value (so a caller that wired nothing
// simply gets no trace instead of a nil dereference).
func TestStepsFromCtx(t *testing.T) {
	if StepsFrom(context.Background()).Enabled() {
		t.Fatal("an unbound ctx must yield the zero reporter")
	}

	var got []string
	want := StepReporter{Text: func(line string) { got = append(got, line) }}
	ctx := WithSteps(context.Background(), want)
	StepsFrom(ctx).Stage("Planner", "x")

	if len(got) != 1 || got[0] != "[Planner] x"+thinkLineBreak {
		t.Errorf("reporter from ctx = %#v, want the bound one", got)
	}

	// A disabled reporter is not worth binding: the ctx stays untouched.
	plain := WithSteps(context.Background(), StepReporter{})
	if StepsFrom(plain).Enabled() {
		t.Error("binding a disabled reporter must be a no-op")
	}
}

// TestEmitThinkToleratesNilAndPanickingSink guards the event channel on its own.
func TestEmitThinkToleratesNilAndPanickingSink(t *testing.T) {
	emitThink(nil, ThinkEvent{Summary: "x"}) // must not panic
	emitThink(func(ThinkEvent) { panic("boom") }, ThinkEvent{Summary: "x"})
}
