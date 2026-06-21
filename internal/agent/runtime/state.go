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

// runtime — per-run shared state for canvas components.
//
// CanvasState lives here (not in the canvas package) so that the
// builder-side (canvas) and the implementation-side (component) can
// both depend on it without forming an import cycle. The canvas
// package owns DSL types and topology building; the component package
// owns the registered component implementations; both read/write
// CanvasState through this package.
//
// Concurrency: a single sync.RWMutex guards every map in CanvasState
// (plan §2.5 — "start simple"). Helper methods (GetVar / SetVar /
// ReadVars / Snapshot / etc.) lock internally; callers should not
// acquire OutputsLock unless they have a specific reason to extend a
// critical section.
package runtime

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/cloudwego/eino/compose"
)

// CanvasState is the per-run shared state bag that all components read/write
// through eino's StatePreHandler / StatePostHandler (compose/state.go).
//
// Fields mirror Python agent/canvas.py:43-95 with these mappings:
//   - Outputs     : cpn_id -> param_name -> resolved value (variable source)
//   - Sys         : sys.* namespace (query, user_id, conversation_turns, files)
//   - Env         : env.* namespace (deployment-time constants)
//   - Path        : entry-point sequence (Begin nodes)
//   - History     : conversation history (chat-flow agents)
//   - Retrieval   : aggregate retrieval result (chunks, doc_aggs)
//   - Globals     : cross-canvas-instance globals
//   - CancelFlag  : set when cancel signal received; nodes may poll
//   - RunID       : unique per-run identifier (used by RunTracker + CheckPointStore)
type CanvasState struct {
	mu         sync.RWMutex
	Outputs    map[string]map[string]any
	Sys        map[string]any
	Env        map[string]any
	Path       []string
	History    []map[string]any
	Retrieval  map[string]any
	Globals    map[string]any
	CancelFlag *atomic.Bool
	RunID      string
	TaskID     string
}

// NewCanvasState returns a zero-valued CanvasState with all maps allocated.
// The atomic CancelFlag is allocated eagerly so nodes can safely poll it
// even before any cancel signal has been wired.
func NewCanvasState(runID, taskID string) *CanvasState {
	return &CanvasState{
		Outputs:    make(map[string]map[string]any),
		Sys:        make(map[string]any),
		Env:        make(map[string]any),
		Path:       []string{},
		History:    []map[string]any{},
		Retrieval:  make(map[string]any),
		Globals:    make(map[string]any),
		CancelFlag: &atomic.Bool{},
		RunID:      runID,
		TaskID:     taskID,
	}
}

// init registers CanvasState with eino's internal type registry so
// that eino's StatePre/Post handler chain (which uses its own
// InternalSerializer, NOT stdlib encoding/json) recognises the
// type during the deepCopyState call that fires on every interrupt
// boundary. eino's serialization registry requires the type to
// implement both json.Marshaler AND json.Unmarshaler; CanvasState
// has both (below). Without this init, the interrupt path surfaces
// "failed to marshal state: unknown type: runtime.CanvasState"
// and the resume cycle is blocked at the eino layer.
func init() {
	_ = compose.RegisterSerializableType[CanvasState]("runtime.CanvasState")
}

// canvasStateJSON is the wire shape used by MarshalJSON / UnmarshalJSON.
// Defined so the field tags and omitempty semantics are pinned in one
// place. The CancelFlag is round-tripped as a bool (atomic.Bool can't
// be marshalled directly without a wrapper).
type canvasStateJSON struct {
	Outputs    map[string]map[string]any `json:"outputs"`
	Sys        map[string]any            `json:"sys,omitempty"`
	Env        map[string]any            `json:"env,omitempty"`
	Path       []string                  `json:"path,omitempty"`
	History    []map[string]any          `json:"history,omitempty"`
	Retrieval  map[string]any            `json:"retrieval,omitempty"`
	Globals    map[string]any            `json:"globals,omitempty"`
	CancelFlag bool                      `json:"cancel_flag"`
	RunID      string                    `json:"run_id"`
	TaskID     string                    `json:"task_id"`
}

// MarshalJSON serialises the CanvasState for eino's StatePre/Post
// handler chain (which JSON-encodes the state on every node boundary
// when a StateSerializer is wired) and for Redis-backed CheckPointStore
// payloads.
//
// Eino's interrupt path hit "failed to marshal state: unknown
// type: runtime.CanvasState"
// because the struct had no MarshalJSON and contained a sync.RWMutex
// (unexported) + atomic.Bool (indirected; serialises as 8 bytes
// without explicit handling). This hook defines the stable wire shape
// (canvasStateJSON) and serialises through it.
//
// Concurrency: the lock is held briefly while we snapshot the maps;
// readers may briefly block during marshal, which is fine for the
// checkpoint/serializer hot path. The lock is read-only so concurrent
// SetVar calls also proceed.
func (s *CanvasState) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := canvasStateJSON{
		Outputs:    s.Outputs,
		Sys:        s.Sys,
		Env:        s.Env,
		Path:       s.Path,
		History:    s.History,
		Retrieval:  s.Retrieval,
		Globals:    s.Globals,
		CancelFlag: s.CancelFlag != nil && s.CancelFlag.Load(),
		RunID:      s.RunID,
		TaskID:     s.TaskID,
	}
	return json.Marshal(snap)
}

// UnmarshalJSON restores the wire shape produced by MarshalJSON.
// Cancels the read-lock contention: an unmarshal only happens during
// checkpoint restore (rare) and boot, so we accept the lock-acquire
// cost. atomic.Bool is allocated so the loaded value lands on a real
// pointer (nodes may poll it concurrently with unmarshal completion).
func (s *CanvasState) UnmarshalJSON(b []byte) error {
	var snap canvasStateJSON
	if err := json.Unmarshal(b, &snap); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if snap.Outputs != nil {
		s.Outputs = snap.Outputs
	}
	if snap.Sys != nil {
		s.Sys = snap.Sys
	}
	if snap.Env != nil {
		s.Env = snap.Env
	}
	s.Path = snap.Path
	s.History = snap.History
	if snap.Retrieval != nil {
		s.Retrieval = snap.Retrieval
	}
	if snap.Globals != nil {
		s.Globals = snap.Globals
	}
	if s.CancelFlag == nil {
		s.CancelFlag = &atomic.Bool{}
	}
	s.CancelFlag.Store(snap.CancelFlag)
	s.RunID = snap.RunID
	s.TaskID = snap.TaskID
	return nil
}

// GetVar resolves a variable reference to its current value.
//
// Supported forms (matches plan §2.5 + agent/canvas.py:168-239):
//
//	"cpn_id@param"        — Outputs[cpn_id][param]
//	"cpn_id@param.path"   — dot-path traversal on Outputs[cpn_id][param]
//	"sys.x"               — Sys["x"]   (also "sys.x.path")
//	"env.x"               — Env["x"]   (also "env.x.path")
//	"item"                — iteration alias (nil if unset)
//	"index"               — iteration alias (nil if unset)
//
// An unknown cpn_id returns (nil, nil) — mirrors Python's "treat as literal"
// fallback (canvas.py:494-495).
func (s *CanvasState) GetVar(ref string) (any, error) {
	if ref == "" {
		return nil, fmt.Errorf("canvas: empty variable reference")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return getVarLocked(s, ref)
}

// SetVar writes Outputs[cpnID][param] = v. Nested keys separated by "." are
// auto-created (mirrors Python's set_variable_param_value at
// canvas.py:261-271). The lock is held for the entire walk to keep
// "walk + assign" atomic under concurrent writers.
func (s *CanvasState) SetVar(cpnID, param string, v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	setVarLocked(s.Outputs, cpnID, param, v)
}

// ReadVars resolves a list of {{...}} references against the current state
// and returns them keyed by the original ref string. Intended for parameter
// binding: a component declares its input parameter references once, this
// resolves them in one locked pass.
//
// Empty / unresolvable refs map to nil (caller decides on nil-handling).
// The first error is returned and short-circuits the rest, but partial
// results are NOT used by callers — discard on err.
func (s *CanvasState) ReadVars(refs []string) (map[string]any, error) {
	out := make(map[string]any, len(refs))
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, ref := range refs {
		v, err := getVarLocked(s, ref)
		if err != nil {
			return nil, err
		}
		out[ref] = v
	}
	return out, nil
}

// Snapshot returns a shallow copy of every cpn's outputs map. It is the
// snapshot that StatePreHandler exposes to component bodies. Shallow is
// fine: components only re-read primitive values from this snapshot
// during one execution; a deeper copy would just cost allocations.
//
// The lock is held only for the duration of the copy; callers may pass
// the returned map around freely.
func (s *CanvasState) Snapshot() map[string]map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]map[string]any, len(s.Outputs))
	for k, v := range s.Outputs {
		cp := make(map[string]any, len(v))
		for kk, vv := range v {
			cp[kk] = vv
		}
		out[k] = cp
	}
	return out
}

// RecordOutput stores payload under Outputs[cpnID][bucket]. Used by the
// StatePostHandler to persist a node's result so downstream nodes can
// resolve {{cpnID@bucket.x}} references against it.
func (s *CanvasState) RecordOutput(cpnID, bucket string, payload any) {
	if cpnID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.Outputs[cpnID]
	if !ok {
		b = make(map[string]any)
		s.Outputs[cpnID] = b
	}
	b[bucket] = payload
}

// GetRetrievalChunks returns a snapshot of the chunks recorded in
// state.Retrieval["chunks"]. The Retrieval map is the canvas-level
// aggregate that the Retrieval tool populates during the ReAct loop;
// the post-stream citation-grounding call reads it back to
// build the prompts.CitationSource list.
//
// The function returns nil when the state has no chunks recorded
// (a non-retrieval canvas, or no tool call has populated the field
// yet). The returned slice is a fresh copy so callers can range
// over it without holding the lock.
func (s *CanvasState) GetRetrievalChunks() []map[string]any {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	raw, ok := s.Retrieval["chunks"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out
}

// SetRetrievalChunks records the supplied chunks into
// state.Retrieval["chunks"]. Existing entries are replaced
// (last-writer-wins) so a multi-tool canvas reflects the most
// recent retrieval pass when the Agent's grounding call reads the
// state.
func (s *CanvasState) SetRetrievalChunks(chunks []map[string]any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Retrieval == nil {
		s.Retrieval = make(map[string]any)
	}
	asAny := make([]any, 0, len(chunks))
	for _, c := range chunks {
		asAny = append(asAny, c)
	}
	s.Retrieval["chunks"] = asAny
}

// getVarLocked is the lock-free inner GetVar. Caller must hold s.mu (read or
// write) for the entire call.
func getVarLocked(s *CanvasState, ref string) (any, error) {
	switch {
	case ref == "item":
		return s.Globals["__item__"], nil
	case ref == "index":
		return s.Globals["__index__"], nil
	case strings.HasPrefix(ref, "sys."):
		return dotTraverse(s.Sys, strings.TrimPrefix(ref, "sys.")), nil
	case strings.HasPrefix(ref, "env."):
		return dotTraverse(s.Env, strings.TrimPrefix(ref, "env.")), nil
	case strings.Contains(ref, "@"):
		idx := strings.Index(ref, "@")
		cpnID, tail := ref[:idx], ref[idx+1:]
		outputs, ok := s.Outputs[cpnID]
		if !ok {
			return nil, nil
		}
		return dotTraverse(outputs, tail), nil
	default:
		return nil, fmt.Errorf("canvas: invalid variable reference %q", ref)
	}
}

// setVarLocked is the lock-free inner SetVar. Caller must hold s.mu.
func setVarLocked(outputs map[string]map[string]any, cpnID, param string, v any) {
	bucket, ok := outputs[cpnID]
	if !ok {
		bucket = make(map[string]any)
		outputs[cpnID] = bucket
	}
	parts := strings.Split(param, ".")
	cur := bucket
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = v
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[p] = next
		}
		cur = next
	}
}

// dotTraverse walks a dot-path inside a generic Go value. The path is split
// on "." and dispatched by intermediate type, mirroring Python's
// get_variable_param_value precedence (canvas.py:212-239):
//
//  1. nil  → return nil
//  2. string → try json.Unmarshal, then continue on the parsed value
//  3. map[string]any → index by key
//  4. []any → index by int (cast failure → nil)
//  5. else → return nil
//
// The empty path returns the root value as-is.
func dotTraverse(root any, path string) any {
	if path == "" {
		return root
	}
	parts := strings.Split(path, ".")
	cur := root
	for _, p := range parts {
		cur = step(cur, p)
		if cur == nil {
			return nil
		}
	}
	return cur
}

func step(cur any, key string) any {
	switch v := cur.(type) {
	case nil:
		return nil
	case map[string]any:
		return v[key]
	case string:
		// Strings can be JSON-encoded dicts/lists; try once.
		var parsed any
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			return step(parsed, key)
		}
		return nil
	case []any:
		var idx int
		if _, err := fmt.Sscanf(key, "%d", &idx); err != nil {
			return nil
		}
		if idx < 0 || idx >= len(v) {
			return nil
		}
		return v[idx]
	default:
		return nil
	}
}
