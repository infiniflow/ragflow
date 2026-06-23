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

// GetVar resolves a variable reference to its current value.
//
// Supported forms (matches plan §2.5 + agent/canvas.py:168-239):
//
//	"cpn_id@param"        — Outputs[cpn_id][param]
//	"cpn_id@param.path"   — dot-path traversal on Outputs[cpn_id][param]
//	"sys.x"               — Sys["x"]   (also "sys.x.path")
//	"env.x"               — Env["x"]   (also "env.x.path")
//	"item"                — iteration alias (Phase 2; nil if unset)
//	"index"               — iteration alias (Phase 2; nil if unset)
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
