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
	"sync"
	"time"

	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/utility"
)

// DebugLogTTL is the Redis expiry for a debug-run log. It mirrors the Python
// pipeline's 30-minute update TTL (rag/flow/pipeline.py callback: set_obj(...,
// 60*30)); the Go side writes the log once at the end of the synchronous run.
const DebugLogTTL = 30 * time.Minute

// DebugLogStore is the minimal Redis surface the debug log sink needs. In
// production it is satisfied by *redis.Client (which exposes Set(key, value,
// ttl)); tests pass a capturing closure. Keeping it to a single method keeps the
// sink free of any redis-package import.
type DebugLogStore interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) bool
}

// funcStore adapts a plain function to DebugLogStore so tests (and callers that
// already hold a go-redis client) can wire a sink without a named type.
type funcStore func(key, value string, ttl time.Duration) bool

// Set implements DebugLogStore.
func (f funcStore) Set(key, value string, ttl time.Duration) bool { return f(key, value, ttl) }

// component phases mirror runtime.ProgressPhase (Enter=0, Exit=1, Error=2).
// They are copied locally so the sink stays decoupled from the agent runtime
// package; the integer values are part of the pipeline's stable event contract.
const (
	phaseEnter = 0
	phaseExit  = 1
	phaseError = 2
)

// Debug-log size guards. A debug run may emit an unbounded number of component
// lifecycle events (e.g. a misbehaving component looping), so the sink caps the
// number of components, the traces per component, and the per-message length.
// This stops a single run from exhausting memory or writing an oversized entry
// into the shared Redis. The bounds are far above any legitimate canvas and only
// catch pathological runs.
const (
	maxLogEntries    = 1000    // distinct components recorded per run
	maxTracePerEntry = 200     // progress events kept per component
	maxMessageRunes  = 1024    // rune length cap for a single trace message
	maxPayloadBytes  = 1 << 20 // hard ceiling on the JSON written to Redis (1 MiB)
)

// debugLogEntry is one component's row in the front-end Log box: a component id
// plus an ordered list of trace lines. It matches the Python pipeline callback
// shape exactly (rag/flow/pipeline.py) so GetAgentLogs can return it verbatim
// and the front-end (useFetchMessageTrace) can read it unchanged.
type debugLogEntry struct {
	ComponentID string       `json:"component_id"`
	Trace       []debugTrace `json:"trace"`
}

type debugTrace struct {
	Progress    float64 `json:"progress"`
	Message     string  `json:"message"`
	Datetime    string  `json:"datetime"`
	Timestamp   float64 `json:"timestamp"`
	ElapsedTime float64 `json:"elapsed_time"`
	// Dsl carries the debug-run result (per-component outputs) on the END
	// marker's single trace entry, so the front-end "View result" page can
	// render parsed chunks. It mirrors Python's END marker `dsl`
	// (rag/flow/pipeline.py:98). Nil/empty when unset, in which case
	// encoding/json omits it (json:"omitempty").
	Dsl json.RawMessage `json:"dsl,omitempty"`
}

// DebugLogSink implements pipeline.ProgressSink by accumulating component
// lifecycle events into the [{component_id, trace}] array the front-end polls
// for. It deliberately owns no Redis I/O until Flush: all events are buffered
// in memory, so the sink is fully testable with a fake DebugLogStore and the
// executor can drive it without a live Redis. A run always ends with an END
// marker (appended in Flush) carrying a non-empty message, which is the exact
// signal the front-end uses to consider the run finished.
type DebugLogSink struct {
	canvasID  string
	messageID string
	store     DebugLogStore
	ttl       time.Duration

	mu      sync.Mutex
	entries []debugLogEntry
	lastTS  float64
	// resultDSL is the debug-run result built by BuildDebugResultDSL and handed
	// to the sink via SetResult (the ResultSink capability). It is written onto
	// the END marker's trace entry in Flush. Empty until SetResult is called.
	resultDSL json.RawMessage
	// runOutput is the raw pipeline run output map (output["state"][<id>] is
	// each component's outputs), handed over together with resultDSL. Flush
	// uses it to build the END marker's message (the last component's output
	// JSON) exactly like Python does. Nil until SetResult is called.
	runOutput map[string]any
}

// NewDebugLogSink builds a sink that writes to "{canvasID}-{messageID}-logs".
func NewDebugLogSink(canvasID, messageID string, store DebugLogStore) *DebugLogSink {
	return &DebugLogSink{
		canvasID:  canvasID,
		messageID: messageID,
		store:     store,
		ttl:       DebugLogTTL,
	}
}

// OnComponentTotal is a no-op for the debug log: the array shape does not use
// the component-count denominator (it is only consumed by the DB-backed sink
// for progress aggregation).
func (s *DebugLogSink) OnComponentTotal(_ context.Context, _ string, _ int) {}

// SetResult receives the debug-run result DSL (built by BuildDebugResultDSL)
// plus the raw pipeline run output, and stores them for the END marker. It
// implements the optional ResultSink capability the executor probes for, so
// the sink stays decoupled from how the DSL is produced. Safe to call at most
// once per run; a later call replaces the previous value.
func (s *DebugLogSink) SetResult(dsl map[string]any, output map[string]any) {
	b, err := json.Marshal(dsl)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resultDSL = b
	s.runOutput = output
}

// OnComponentProgress maps one component lifecycle event to a trace line and
// merges it into the running array. Consecutive events for the same component
// append to that component's trace (matching the Python callback's merge
// behaviour); a new component opens a fresh array element.
func (s *DebugLogSink) OnComponentProgress(_ context.Context, ev pipeline.ProgressEvent) {
	now := time.Now()
	ts := float64(now.UnixMicro())
	progress := 0.0
	switch ev.Phase {
	case phaseExit:
		progress = 1.0
	case phaseError:
		progress = -1.0
	}

	message := ev.Message
	if ev.Phase == phaseError {
		// Prefix so the front-end timeline renders the line red
		// (web/src/pages/agent/pipeline-log-sheet/dataflow-timeline.tsx).
		message = "[ERROR] " + message
	}
	// Clamp the message so a runaway component cannot blow up the stored entry.
	message = utility.TruncateRunes(message, maxMessageRunes)

	entry := debugTrace{
		Progress:    progress,
		Message:     message,
		Datetime:    now.Format("15:04:05"),
		Timestamp:   float64(now.UnixNano()) / 1e9,
		ElapsedTime: 0,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastTS != 0 {
		// ts and s.lastTS are UnixMicro(); report the delta in SECONDS so the
		// front-end's verbatim "<value>s" rendering (dataflow-timeline.tsx)
		// and the "< 0.000001" gates in the other timeline views stay correct.
		entry.ElapsedTime = (ts - s.lastTS) / 1e6
	}
	s.lastTS = ts

	if n := len(s.entries); n > 0 && s.entries[n-1].ComponentID == ev.Component {
		if len(s.entries[n-1].Trace) >= maxTracePerEntry {
			// This component already has its full quota of trace lines; drop the
			// rest so the buffer stays bounded.
			return
		}
		s.entries[n-1].Trace = append(s.entries[n-1].Trace, entry)
		return
	}
	if len(s.entries) >= maxLogEntries {
		// Component quota reached; ignore further components.
		return
	}
	s.entries = append(s.entries, debugLogEntry{
		ComponentID: ev.Component,
		Trace:       []debugTrace{entry},
	})
}

// Flush serialises the accumulated array (plus a terminal END marker) and
// persists it to the store. finalErr, when non-nil, is surfaced in the END
// message so a failed run is still reported and detectable as complete. All
// Redis I/O is best-effort: a store failure is swallowed (the run must not
// abort because the log could not be written).
func (s *DebugLogSink) Flush(ctx context.Context, finalErr error) {
	s.mu.Lock()
	entries := append([]debugLogEntry(nil), s.entries...)

	now := time.Now()
	endTS := float64(now.UnixMicro())
	elapsed := 0.0
	if s.lastTS != 0 {
		// endTS and s.lastTS are UnixMicro(); report the delta in SECONDS
		// (see OnComponentProgress for the matching conversion).
		elapsed = (endTS - s.lastTS) / 1e6
	}
	endMsg := "Debug run completed"
	if finalErr != nil {
		endMsg = "[ERROR] " + finalErr.Error()
	} else if msg := endOutputMessage(entries, s.runOutput); msg != "" {
		// Mirror Python's END marker message: a JSON dump of the last
		// component's output (rag/flow/pipeline.py:171). The front-end
		// "Export JSON" button JSON.parses this message
		// (use-download-output.ts findEndOutput), so a plain-text sentinel
		// would leave the button permanently disabled.
		endMsg = msg
	}
	entries = append(entries, debugLogEntry{
		ComponentID: "END",
		Trace: []debugTrace{{
			Progress:    1.0,
			Message:     endMsg,
			Datetime:    now.Format("15:04:05"),
			Timestamp:   float64(now.UnixNano()) / 1e9,
			ElapsedTime: elapsed,
			Dsl:         s.resultDSL,
		}},
	})
	s.mu.Unlock()

	payload, err := json.Marshal(entries)
	if err != nil {
		return
	}
	// Hard ceiling: if a buggy/abusive run still exceeds it, drop the middle
	// entries (keeping the first few and the END marker) until under budget.
	if len(payload) > maxPayloadBytes {
		entries = trimToPayloadBudget(entries, maxPayloadBytes)
		if payload, err = json.Marshal(entries); err != nil {
			return
		}
	}
	key := s.canvasID + "-" + s.messageID + "-logs"
	s.store.Set(ctx, key, string(payload), s.ttl)
}

// endOutputMessage builds the END marker's message for a successful run: the
// JSON dump of the last executed component's output, mirroring Python's
// `json.dumps(self.get_component_obj(self.path[-1]).output())`
// (rag/flow/pipeline.py:171). The last executed component is the sink's final
// entry (path[-1] always emits its exit progress last). Raw embedding vectors
// are stripped (deepCopy(out, true)) so the Redis log stays at Python-scale size.
// Returns "" when no output is available; the caller then keeps the
// plain-text fallback and the front-end export button stays disabled (empty
// output, matching Python's isEmpty check).
func endOutputMessage(entries []debugLogEntry, runOutput map[string]any) string {
	if len(entries) == 0 || len(runOutput) == 0 {
		return ""
	}
	lastID := entries[len(entries)-1].ComponentID
	out, ok := lookupComponentOutput(runOutput, lastID).(map[string]any)
	if !ok || len(out) == 0 {
		return ""
	}
	b, err := json.Marshal(deepCopy(out, true))
	if err != nil {
		return ""
	}
	return string(b)
}

// trimToPayloadBudget collapses the middle of the log (keeping the first few
// entries plus the END marker at the tail) until the marshaled size fits the
// byte budget. The END marker is always the last element, so it is preserved.
// The loop terminates because each pass shrinks the slice; once only the head
// and END remain the payload is tiny.
func trimToPayloadBudget(entries []debugLogEntry, budget int) []debugLogEntry {
	for len(entries) > 2 {
		if b, err := json.Marshal(entries); err == nil && len(b) <= budget {
			return entries
		}
		keep := len(entries) / 4
		if keep < 1 {
			keep = 1
		}
		entries = append(append([]debugLogEntry{}, entries[:keep]...), entries[len(entries)-1])
	}
	return entries
}
