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

package advanced_rag

import (
	"io"
	"log"
	"strings"
)

// Surface selected internal stage logs to the client as thinking content.
//
// Bracket-tagged lines such as "[Agentic RAG]", "[Planner]", "[Orchestrator]",
// "[Hybrid search]" and "[Composing the answer]" are forwarded to a per-request sink, so
// the front end shows live reasoning WITHOUT instrumenting every call site. The tags
// double as human-readable stage labels, so one message serves both the backend log and
// the thinking stream.
//
// The interception happens at the WRITER the run's logger is rebuilt on top of, because
// the log package has no per-record handler hook. That logger is threaded into SearchDeps
// and every graph node, so wrapping it once in Rag covers the whole pipeline.
//
// Two consequences of filtering on the tag SHAPE alone:
//   - numbered evidence markers ("[1] chunk text") have to be rejected: they are chunk
//     listings, not stage tags (see isEvidenceMarker);
//   - the tool narration ("[Function tool] Running the {name} tool with: …") reaches the
//     block by itself, because harness/tool_executor.go logs through the same wrapped
//     logger. The line around a single harness invocation comes from
//     internal/service/chat_pipeline.go (toolLoopLine), which has no model-authored loop
//     to instrument.
//
// The think block is HTML, so a stage line ends with ThinkLineBreak rather than "\n" (see
// ThinkLineBreak).

// ThinkLineBreak separates two lines inside the think block.
//
// The block is delivered to the client as inline HTML (the chat UI wraps it in
// <details class="think"> and renders the message as markdown), where a newline is just
// whitespace — so every think-line producer (thinkWriter, the outer react loop's progress
// routing, chat_pipeline's tool-loop narration) must use this marker, or consecutive
// stage lines merge into one sentence.
const ThinkLineBreak = "<br>"

// thinkWriter mirrors one log line onto the real logger while forwarding
// bracket-tagged stage lines to the per-request progress sink.
type thinkWriter struct {
	orig *log.Logger // the run's real logger; keeps backend logging unchanged
	sink func(string)
}

// Write implements io.Writer.
func (w thinkWriter) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	if line == "" {
		return len(p), nil
	}
	// The filter is the tag SHAPE: a leading "[" that is not a numbered evidence marker
	// such as "[1] ...", which is a chunk listing rather than a stage tag.
	if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "[") && !isEvidenceMarker(trimmed) {
		w.forward(trimmed)
	}
	// Re-emit through the ORIGINAL logger so its own routing (zap) and level are
	// preserved. This writer is built with no prefix and no flags, so the
	// backend output is exactly what it was before the wrap.
	if w.orig != nil {
		w.orig.Printf("%s", line)
	}
	return len(p), nil
}

// forward delivers one line to the sink.
//
// The separator is ThinkLineBreak: the thinking block reaches the client as the inline
// "<think>...<details class="think">" HTML that the markdown renderer passes through,
// where a bare "\n" collapses into a space — every stage line would then run together
// into one unreadable sentence.
//
// The recover() guards the sink: forwarding a think line must never break the request or
// the logging subsystem itself.
func (w thinkWriter) forward(line string) {
	defer func() { _ = recover() }()
	if w.sink == nil {
		return
	}
	// The break is TRAILING here: prefixing it would open the block with a blank line.
	w.sink(line + ThinkLineBreak)
}

// isEvidenceMarker reports whether line is a numbered chunk/option listing such
// as "[1] passage text" rather than a stage tag like "[Planner] ...".
func isEvidenceMarker(line string) bool {
	rest := line[1:]
	if rest == "" {
		return false
	}
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		n++
	}
	return n > 0 && n < len(rest) && rest[n] == ']'
}

// thinkLogger returns a *log.Logger that logs through orig while additionally
// forwarding bracket-tagged stage lines to sink. The sink is per REQUEST, so concurrent
// requests stay isolated.
//
// sink may be nil, in which case orig is returned unchanged.
func thinkLogger(orig *log.Logger, sink func(string)) *log.Logger {
	if orig == nil || sink == nil {
		return orig
	}
	return log.New(thinkWriter{orig: orig, sink: sink}, "", 0)
}

var _ io.Writer = thinkWriter{}
