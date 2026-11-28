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

	"ragflow/internal/common"
	"ragflow/internal/ingestion/pipeline"
	"ragflow/internal/utility"
)

// DebugLogTTL is the Redis expiry for a debug-run log. It mirrors the
// pipeline's 30-minute update TTL ( callback: set_obj(
// 60*30)); the Go side writes the timeline and result payload once at the end
// of the synchronous run.
const DebugLogTTL = 30 * time.Minute

// DebugLogStore is the minimal Redis surface the debug log sink needs. In
// production it is satisfied by *redis.Client (which exposes Set(key, value,
// ttl)); tests pass a capturing closure. Keeping it to a single method keeps the
// sink free of any redis-package import.
type DebugLogStore interface {
	Set(ctx context.Context, key, value string, ttl time.Duration) bool
}

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
	maxPayloadBytes  = 1 << 20 // hard ceiling on the timeline JSON written to Redis (1 MiB)
	// maxResultBytes is the separate ceiling for the run-result payload stored
	// under DebugResultKey: the complete END output plus the optional preview
	// DSL. The END output is preserved for Export JSON even when it alone
	// exceeds this budget; the budget bounds the optional DSL.
	maxResultBytes = 32 << 20
)

const (
	debugEndComponentID      = "END"
	debugRunCompletedMessage = "Debug run completed"
)

// debugLogEntry is one component's row in the front-end Log box: a component id
// plus an ordered list of trace lines. It matches the pipeline callback
// shape exactly so GetAgentLogs can return the timeline verbatim (the separate
// result payload is merged into the END marker on read).
type debugLogEntry struct {
	ComponentID string       `json:"component_id"`
	Trace       []debugTrace `json:"trace"`
}

// debugTrace is one line in a component's timeline entry.
type debugTrace struct {
	Progress    float64 `json:"progress"`
	Message     string  `json:"message"`
	Datetime    string  `json:"datetime"`
	Timestamp   float64 `json:"timestamp"`
	ElapsedTime float64 `json:"elapsed_time"`
}

// debugResultPayload is persisted under DebugResultKey, separate from the
// timeline. Message is the complete END output JSON used by the front-end
// Export JSON button; DSL is the optional result preview. Keeping the message
// out of the timeline means the size guard can never truncate export data.
type debugResultPayload struct {
	Message string          `json:"message,omitempty"`
	DSL     json.RawMessage `json:"dsl,omitempty"`
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
	// to the sink via SetResult (the ResultSink capability). Flush persists it
	// under DebugResultKey, separate from the operator timeline. Empty until
	// SetResult is called.
	resultDSL json.RawMessage
	// runOutput is the raw pipeline run output map (output["state"][<id>] is
	// each component's outputs), handed over together with resultDSL. Flush
	// uses it to build the END marker's message (the last component's output
	// JSON) exactly like does. Nil until SetResult is called
	runOutput map[string]any
}

// NewDebugLogSink builds a sink that writes the operator timeline to
// DebugLogKey and the run-result payload to DebugResultKey.
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
// plus the raw pipeline run output. Flush persists them under DebugResultKey.
// It implements the optional ResultSink capability the executor probes for, so
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
// append to that component's trace (matching the callback's merge
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
//
// The operator timeline and the run-result payload are written under separate
// keys with independent size budgets. The result carries business data (parsed
// chunks, inline images) that can exceed the timeline budget by orders of
// magnitude; keeping it in the timeline made the size guard drop middle
// component entries, so the Log box showed only the first operator and END.
func (s *DebugLogSink) Flush(ctx context.Context, finalErr error) {
	s.mu.Lock()
	entries := append([]debugLogEntry(nil), s.entries...)
	resultDSL := append(json.RawMessage(nil), s.resultDSL...)

	now := time.Now()
	endTS := float64(now.UnixMicro())
	elapsed := 0.0
	if s.lastTS != 0 {
		// endTS and s.lastTS are UnixMicro(); report the delta in SECONDS
		// (see OnComponentProgress for the matching conversion).
		elapsed = (endTS - s.lastTS) / 1e6
	}
	endMsg := debugRunCompletedMessage
	hasOutput := false
	if finalErr != nil {
		endMsg = "[ERROR] " + finalErr.Error()
	} else if msg := endOutputMessage(entries, s.runOutput); msg != "" {
		endMsg = msg
		hasOutput = true
	}
	s.mu.Unlock()

	// The complete END output is persisted separately from the timeline: the
	// timeline is polled frequently and capped at 1 MiB, while Export JSON must
	// never receive a truncated document. The timeline only carries a short
	// completion message; GetAgentLogs restores the full output on read.
	resultStored := false
	if hasOutput || len(resultDSL) > 0 {
		resultMsg := ""
		if hasOutput {
			resultMsg = endMsg
		}
		if payload, ok := buildDebugResultPayload(resultMsg, resultDSL, maxResultBytes); ok {
			resultStored = s.store.Set(ctx, DebugResultKey(s.canvasID, s.messageID), string(payload), s.ttl)
		}
	}
	if hasOutput && resultStored {
		endMsg = debugRunCompletedMessage
	}

	entries = append(entries, debugLogEntry{
		ComponentID: debugEndComponentID,
		Trace: []debugTrace{{
			Progress:    1.0,
			Message:     endMsg,
			Datetime:    now.Format("15:04:05"),
			Timestamp:   float64(now.UnixNano()) / 1e9,
			ElapsedTime: elapsed,
		}},
	})

	payload, err := json.Marshal(entries)
	if err != nil {
		return
	}
	// Hard ceiling: shrink trace detail, never component identity. Recheck the
	// encoded size after every reduction so JSON escaping (e.g. '<' -> six
	// bytes) cannot leave the stored timeline over budget.
	if len(payload) > maxPayloadBytes {
		entries = trimTracesToBudget(entries, maxPayloadBytes)
		if payload, err = json.Marshal(entries); err != nil {
			return
		}
		if len(payload) > maxPayloadBytes {
			common.Warn("debug log: timeline still exceeds the payload budget after trimming")
		}
	}
	s.store.Set(ctx, DebugLogKey(s.canvasID, s.messageID), string(payload), s.ttl)
}

// DebugLogKey returns the Redis key holding the operator timeline for a run.
func DebugLogKey(canvasID, messageID string) string {
	return canvasID + "-" + messageID + "-logs"
}

// DebugResultKey returns the Redis key holding the complete END output and the
// optional result DSL for a run. It is separate from the timeline so large
// business data cannot evict component entries or be truncated by the timeline
// size guard.
func DebugResultKey(canvasID, messageID string) string {
	return canvasID + "-" + messageID + "-result"
}

// buildDebugResultPayload serialises the result payload under the result
// budget. Priority is deliberate:
//  1. full END output + full DSL;
//  2. full END output + compact DSL (outputs removed);
//  3. full END output only — Export JSON must never receive a truncated
//     document, so the message is preserved even when it alone exceeds the
//     budget;
//  4. compact DSL only when there is no END output;
//  5. omit the result preview.
func buildDebugResultPayload(message string, dsl json.RawMessage, budget int) ([]byte, bool) {
	full := debugResultPayload{Message: message, DSL: dsl}
	if payload, err := json.Marshal(full); err == nil && len(payload) <= budget {
		return payload, true
	}

	compact := compactDebugResultDSL(dsl)
	if len(compact) > 0 {
		if payload, err := json.Marshal(debugResultPayload{Message: message, DSL: compact}); err == nil && len(payload) <= budget {
			return payload, true
		}
	}

	if message != "" {
		payload, err := json.Marshal(debugResultPayload{Message: message})
		if err == nil {
			if len(payload) > budget {
				common.Warn("debug log: END output exceeds the result budget; preserving it for export")
			}
			return payload, true
		}
	}

	if len(compact) > 0 {
		if payload, err := json.Marshal(debugResultPayload{DSL: compact}); err == nil && len(payload) <= budget {
			return payload, true
		}
	}

	common.Warn("debug log: result payload exceeds the size cap; result preview omitted")
	return nil, false
}

// compactDebugResultDSL returns the result DSL with each component's runtime
// outputs removed. It preserves the component/topology structure so the
// "View result" page can still render the steps when the full outputs are too
// large for the result budget.
func compactDebugResultDSL(dsl json.RawMessage) json.RawMessage {
	if len(dsl) == 0 {
		return nil
	}
	var doc map[string]any
	if err := json.Unmarshal(dsl, &doc); err != nil {
		return nil
	}
	components, _ := doc["components"].(map[string]any)
	for _, raw := range components {
		comp, _ := raw.(map[string]any)
		obj, _ := comp["obj"].(map[string]any)
		params, _ := obj["params"].(map[string]any)
		if params != nil {
			delete(params, "outputs")
		}
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil
	}
	return out
}

// MergeDebugResultPayload attaches the separately stored END output and result
// DSL to the END marker of a decoded debug log. It locates END by component id
// rather than by position, so the read path does not depend on where a writer
// placed it. It is a no-op when the log lacks an END marker or the payload is
// empty/corrupt, so old logs that still carry the DSL inline keep working.
func MergeDebugResultPayload(logs any, resultPayload string) {
	if resultPayload == "" {
		return
	}
	var result debugResultPayload
	if err := json.Unmarshal([]byte(resultPayload), &result); err != nil {
		return
	}
	if result.Message == "" && len(result.DSL) == 0 {
		return
	}

	entries, ok := logs.([]any)
	if !ok || len(entries) == 0 {
		return
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry, _ := entries[i].(map[string]any)
		if entry == nil || entry["component_id"] != debugEndComponentID {
			continue
		}
		traces, _ := entry["trace"].([]any)
		if len(traces) == 0 {
			return
		}
		first, _ := traces[0].(map[string]any)
		if first == nil {
			return
		}
		if result.Message != "" {
			first["message"] = result.Message
		}
		if len(result.DSL) > 0 && json.Valid(result.DSL) {
			// encoding/json embeds RawMessage verbatim, so the parsed result is
			// not decoded and re-encoded on every poll.
			first["dsl"] = result.DSL
		}
		return
	}
}

// endOutputMessage builds the END marker's message for a successful run: the
// JSON dump of the last executed component's output, mirroring
// `json.dumps(self.get_component_obj(self.path[-1]).output())`
// (the last executed component's output). The last executed component is the sink's final
// entry (path[-1] always emits its exit progress last). Raw embedding vectors
// are stripped (deepCopy(out, true)) so the stored result payload stays smaller.
// Returns "" when no output is available; the caller then keeps the
// plain-text fallback and the front-end export button stays disabled (empty
// output, matching isEmpty check)
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

// trimTracesToBudget reduces a timeline over budget without dropping any
// component entry. The operator list is the log's primary contract, so detail
// is discarded in stages and the encoded size is rechecked after each stage:
// keep start/terminal traces, then only the terminal trace, then shrink
// messages progressively. JSON escaping can make a short rune count expensive
// (e.g. '<' encodes to six bytes), so the loop must verify the marshaled bytes,
// not the rune count.
func trimTracesToBudget(entries []debugLogEntry, budget int) []debugLogEntry {
	if payloadFits(entries, budget) {
		return entries
	}
	for i := range entries {
		if n := len(entries[i].Trace); n > 2 {
			entries[i].Trace = []debugTrace{entries[i].Trace[0], entries[i].Trace[n-1]}
		}
	}
	if payloadFits(entries, budget) {
		return entries
	}
	for i := range entries {
		if n := len(entries[i].Trace); n > 1 {
			entries[i].Trace = entries[i].Trace[n-1:]
		}
	}
	if payloadFits(entries, budget) {
		return entries
	}
	for _, limit := range []int{256, 128, 64, 32, 16, 8, 4, 2, 1} {
		for i := range entries {
			for j := range entries[i].Trace {
				entries[i].Trace[j].Message = utility.TruncateRunes(entries[i].Trace[j].Message, limit)
			}
		}
		if payloadFits(entries, budget) {
			return entries
		}
	}
	return entries
}

func payloadFits(entries []debugLogEntry, budget int) bool {
	b, err := json.Marshal(entries)
	return err == nil && len(b) <= budget
}
