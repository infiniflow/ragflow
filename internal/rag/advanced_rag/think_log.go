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
// Mirrors Python rag/advanced_rag/think_log.py, which installs a root
// logging.Handler that forwards bracket-tagged INFO records such as
// "[Agentic RAG]", "[Planner]", "[Orchestrator]", "[Hybrid search]" and
// "[Composing the answer]" to a per-request sink, so the front end shows live
// reasoning WITHOUT instrumenting every call site. The tags double as
// human-readable stage labels, so one message serves both the backend log and
// the thinking stream.
//
// Go has no per-record handler hook on *log.Logger, so the same filtering is
// applied at the writer the run's logger is rebuilt on top of. That logger is
// threaded into SearchDeps and every graph node, so wrapping it once in Rag
// covers the whole pipeline.
//
// MECHANISM DIFFERENCE vs Python. Python installed its handler on the ROOT
// logger ("logging.getLogger().addHandler"), then narrowed by
// _SCOPED_PREFIXES = ("rag.advanced_rag", "rag.llm.chat_model",
// "rag.llm.tool_decorator"). Go's log package has no root logger, so nothing
// can be intercepted globally; only the *log.Logger explicitly threaded
// through advanced_rag (deps.Logger) is wrapped here.
//
// Both remaining namespaces are covered by other means:
//   - rag.llm.tool_decorator: "[Function tool] Running the {name} tool with:
//     {args}" (tool_decorator.py:311) is emitted directly by
//     harness/tool_executor.go, which logs through the same wrapped
//     deps.Logger and therefore reaches the think block.
//   - rag.llm.chat_model: "[Tool loop] ..." (chat_model.py:689/782/799) has no
//     Go equivalent loop to instrument — Go's chat pipeline calls Rag()
//     directly instead of an outer model choosing to call it as a tool. The
//     equivalent narration is emitted by internal/service/chat_pipeline.go
//     (toolLoopLine) around the single harness invocation.
//
// Two smaller divergences:
//   - Go additionally rejects numbered evidence markers ("[1] chunk text"),
//     which compensates for the missing namespace filter.
//   - The think block is HTML, so one stage line ends with ThinkLineBreak
//     ("<br>") rather than "\n" — Python think_log.py:69 did the same.

// ThinkLineBreak separates two lines inside the think block.
//
// The block is delivered to the client as inline HTML (the chat UI wraps it in
// <details class="think"> and renders the message as markdown), where a newline
// is just whitespace. Python think_log.py:69 emitted "<br>" for the same
// reason; every think-line producer in Go (thinkWriter, the outer react loop's
// progress routing, chat_pipeline's tool-loop narration) must use this so
// consecutive stage lines do not merge into one sentence.
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
	// Python filters on the record's logger namespace plus a leading "["; Go
	// filters on the tag shape alone, so numbered evidence markers such as
	// "[1] ..." must be rejected, since those are chunk listings, not stage tags.
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
// The separator is "<br>", exactly like Python think_log.py:69
// (`sink("<br>" + msg.strip())`): the thinking block reaches the client as the
// inline "<think>...<details class="think">" HTML that the markdown renderer
// passes through, and a bare "\n" collapses into a space there — every stage
// line then runs together into one unreadable sentence.
//
// Python wrapped the sink call in a bare try/except; the recover() here plays
// the same role, so think-log forwarding can never break the request or the
// logging subsystem itself.
func (w thinkWriter) forward(line string) {
	defer func() { _ = recover() }()
	if w.sink == nil {
		return
	}
	// Python prefixes the break (sink("<br>" + msg)); a trailing one keeps the
	// block from opening with a blank line.
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
// forwarding bracket-tagged stage lines to sink. It is the Go counterpart of
// Python's install_think_log_handler + set_think_log_sink pair: the sink is per
// request, so concurrent requests stay isolated.
//
// sink may be nil, in which case orig is returned unchanged.
func thinkLogger(orig *log.Logger, sink func(string)) *log.Logger {
	if orig == nil || sink == nil {
		return orig
	}
	return log.New(thinkWriter{orig: orig, sink: sink}, "", 0)
}

var _ io.Writer = thinkWriter{}
