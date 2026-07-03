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

package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"sync"
	"time"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/storage"
)

// Pipeline is a compiled ingestion canvas plus task-scoped metadata.
type Pipeline struct {
	taskID          string
	pipelineVersion string
	dsl             map[string]any
	canvas          *canvas.Canvas
	sink            ProgressSink
	storage         storage.Storage
	dao             *dao.IngestionTaskLogDAO

	mu             sync.Mutex
	lastCheckpoint map[string]any
}

// ComputePipelineVersion returns the SHA-256 digest of the canonicalized DSL.
func ComputePipelineVersion(dsl []byte) string {
	if len(dsl) == 0 {
		return ""
	}
	var raw any
	if err := json.Unmarshal(dsl, &raw); err != nil {
		sum := sha256.Sum256(dsl)
		return hex.EncodeToString(sum[:])
	}
	canonical, err := json.Marshal(canonicaliseValue(raw))
	if err != nil {
		sum := sha256.Sum256(dsl)
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func canonicaliseValue(v any) any {
	visited := make(map[uintptr]struct{})
	return canonicaliseValueWith(v, visited)
}

func canonicaliseValueWith(v any, visited map[uintptr]struct{}) any {
	switch x := v.(type) {
	case map[string]any:
		ptr := reflect.ValueOf(x).Pointer()
		if _, seen := visited[ptr]; seen {
			return nil
		}
		visited[ptr] = struct{}{}
		defer delete(visited, ptr)
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]any, 0, 2*len(x))
		for _, k := range keys {
			out = append(out, k, canonicaliseValueWith(x[k], visited))
		}
		return out
	case []any:
		ptr := reflect.ValueOf(x).Pointer()
		if _, seen := visited[ptr]; seen {
			return nil
		}
		visited[ptr] = struct{}{}
		defer delete(visited, ptr)
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = canonicaliseValueWith(item, visited)
		}
		return out
	default:
		return v
	}
}

func ComputeParamsFingerprint(params map[string]any) string {
	return computeMapFingerprint(params)
}

func ComputeStageFingerprint(upstream, params map[string]any) string {
	wrapped := map[string]any{
		"upstream": canonicaliseValue(normaliseNil(upstream)),
		"params":   canonicaliseValue(normaliseNil(params)),
	}
	return computeMapFingerprint(wrapped)
}

func normaliseNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

func computeMapFingerprint(m map[string]any) string {
	canonical, err := json.Marshal(canonicaliseValue(m))
	if err != nil {
		sum := sha256.Sum256([]byte(fmt.Sprintf("%v", m)))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// IdempotencyKey identifies one component execution.
type IdempotencyKey struct {
	TaskID           string
	PipelineVersion  string
	ComponentName    string
	ComponentVersion string
	InputFingerprint string
}

func (k IdempotencyKey) String() string {
	return k.TaskID + "|" + k.PipelineVersion + "|" + k.ComponentName + "|" + k.ComponentVersion + "|" + k.InputFingerprint
}

// IsStageDone reports whether a matching component_done row already exists.
func IsStageDone(checkpoint map[string]any, pipelineVersion, componentName, componentVersion, inputFingerprint string) bool {
	done, ok := checkpoint["component_done"].(map[string]any)
	if !ok {
		return false
	}
	row, ok := done[componentName].(map[string]any)
	if !ok {
		return false
	}
	persistedPV, hasPV := row["pipeline_version"].(string)
	if !hasPV || persistedPV == "" || persistedPV != pipelineVersion {
		return false
	}
	if row["component_version"] != componentVersion {
		return false
	}
	if row["input_fingerprint"] != inputFingerprint {
		return false
	}
	return true
}

func recordStageDone(checkpoint map[string]any, key IdempotencyKey) {
	done, _ := checkpoint["component_done"].(map[string]any)
	if done == nil {
		done = make(map[string]any)
	}
	done[key.ComponentName] = map[string]any{
		"component_version": key.ComponentVersion,
		"input_fingerprint": key.InputFingerprint,
		"pipeline_version":  key.PipelineVersion,
		"completed_at":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	checkpoint["component_done"] = done
}

// NewPipelineFromDSL compiles the canonical ingestion canvas DSL.
// It accepts either the inner canvas DSL or the template wrapper whose
// top-level `dsl` field carries that canvas.
func NewPipelineFromDSL(dsl []byte, taskID string, sink ProgressSink, stg storage.Storage, logs *dao.IngestionTaskLogDAO) (*Pipeline, error) {
	var raw map[string]any
	if err := json.Unmarshal(dsl, &raw); err != nil {
		return nil, fmt.Errorf("pipeline: decode DSL: %w", err)
	}
	canvasDSL, err := unwrapCanvasDSL(raw)
	if err != nil {
		return nil, err
	}
	cnv, err := decodeCanvasFromDSL(canvasDSL)
	if err != nil {
		return nil, fmt.Errorf("pipeline: decode canvas DSL: %w", err)
	}
	return &Pipeline{
		taskID:          taskID,
		pipelineVersion: ComputePipelineVersion(dsl),
		dsl:             canvasDSL,
		canvas:          cnv,
		sink:            sink,
		storage:         stg,
		dao:             logs,
		lastCheckpoint:  make(map[string]any),
	}, nil
}

func unwrapCanvasDSL(raw map[string]any) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, errNilDSL
	}
	if rawDSL, ok := raw["dsl"]; ok {
		canvasDSL, ok := rawDSL.(map[string]any)
		if !ok || len(canvasDSL) == 0 {
			return nil, errNilDSL
		}
		return canvasDSL, nil
	}
	return raw, nil
}

func mergeInto(dst, src map[string]any) map[string]any {
	if src == nil {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func cloneMapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func stageTimeout() time.Duration {
	return defaultStageTimeout
}

var defaultStageTimeout = 60 * time.Second

func (p *Pipeline) recordStageDoneWithKey(key IdempotencyKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	recordStageDone(p.lastCheckpoint, key)
}

func (p *Pipeline) recordCanvasCompletion(inputs map[string]any) {
	if p == nil || p.canvas == nil {
		return
	}
	for _, id := range p.canvas.Path {
		comp, ok := p.canvas.Components[id]
		if !ok {
			continue
		}
		name := comp.Obj.ComponentName
		if name == "" || name == "Begin" {
			continue
		}
		_, _, meta, ok := runtime.DefaultRegistry.Lookup(name)
		if !ok {
			continue
		}
		key := IdempotencyKey{
			TaskID:           p.taskID,
			PipelineVersion:  p.pipelineVersion,
			ComponentName:    name,
			ComponentVersion: meta.Version,
			InputFingerprint: ComputeStageFingerprint(inputs, comp.Obj.Params),
		}
		p.recordStageDoneWithKey(key)
	}
}

func (p *Pipeline) flushCheckpoint(ctx context.Context, _ string) error {
	if p.sink == nil {
		return nil
	}
	p.mu.Lock()
	cp := make(map[string]any, len(p.lastCheckpoint))
	for k, v := range p.lastCheckpoint {
		cp[k] = v
	}
	p.mu.Unlock()
	return p.sink.Persist(ctx, p.taskID, cp)
}

func (p *Pipeline) LastCheckpoint() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]any, len(p.lastCheckpoint))
	for k, v := range p.lastCheckpoint {
		out[k] = v
	}
	return out
}

func (p *Pipeline) Sink() ProgressSink {
	return p.sink
}
