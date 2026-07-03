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

// Package pipeline — Phase 3 orchestrator (DAG runner).
//
// SCOPE (honest):
//
//   - WHAT IS PORTED FROM rag/flow/pipeline.py:
//
//   - top-level entrypoint: Pipeline.Run(ctx, inputs) corresponds to
//     Graph.run(**kwargs) (pipeline.py:117).
//
//   - topological / sequential stage execution: Python extends
//     agent.canvas.Graph and walks self.path in order; the Go port
//     walks Pipeline.stages in declaration order (the ingestion flow
//     is linear, so Kahn's algorithm collapses to a 1-layer DAG —
//     kept in the API surface in case a future stage adds branches).
//
//   - per-component progress callback: Python's
//     self.callback(component_name, progress, message) maps to
//     runtime.ProgressCallback fed into a ProgressSink. The Go
//     pipeline routes every callback to BOTH the runtime
//     notification (for live observers) and a Persist call on the
//     sink (for resumable state).
//
//   - error propagation: first error from any stage cancels ctx
//     and aborts the run; siblings observe ctx.Err() during their
//     own work.
//
//   - cancellation: ctx.Done() is the only interrupt surface —
//     there is no OS-signal "interrupt" in the Go port. The
//     "checkpoint" is the only save-point, written after every
//     successful stage (plan §2 AD-5c).
//
//   - KNOWN GAPS / DEFERRED (per plan §4 Phase 3 task 0):
//
//   - Pipeline._doc_id + DocumentService.get_knowledgebase_id
//     resolution (python pipeline.py:36-41) — the Go port treats
//     doc_id as an opaque key; the doc-service DAO resolution is
//     a Phase 2.6+ concern (plan §1 marks "DSL schema" as not
//     implemented). The caller is expected to supply bucket+path.
//
//   - REDIS_CONN-based log streaming with key
//     {flow_id}-{task_id}-logs — replaced by per-row
//     IngestionTaskLog inserts (DB-first completion, plan §8 Q3).
//
//   - TaskService.update_progress with percentage calculation
//     (python pipeline.py:78-95) — replaced by UpdateStatus on
//     terminal state; intermediate progress is observable via
//     IngestionTaskLog rows.
//
//   - Graph.fetch_logs() Redis read — deferred; a future
//     "list task logs" API can derive the same data from the
//     IngestionTaskLog rows.
//
//   - CANVAS_DEBUG_DOC_ID debug mode (python pipeline.py:33-34) —
//     not ported; the Go port has no equivalent debug-only code
//     path.
//
//   - Pipeline._flow_id / _kb_id fields — informational only on
//     the Python side; not carried in the Go Pipeline struct.
//
//   - DELIBERATE UPGRADES (Go-only, called out in plan §4
//     Phase 3 task 0):
//
//   - layer-parallel execution: the Python pipeline runs a single
//     task per stage (asyncio.create_task(invoke)) and awaits
//     sequentially. The Go port runs each stage's component
//     Parallelism() goroutines concurrently via
//     golang.org/x/sync/errgroup. Per-component fan-out is
//     configured by the component itself (e.g., Parser=4,
//     TokenChunker=4, File/Tokenizer/Extractor=1 — see
//     internal/ingestion/component/<name>.go:Parallelism()).
//
//   - deterministic merge (plan §8 R8): components are expected
//     to merge their goroutine outputs in a deterministic order
//     (e.g., Parser sorts by page number); the pipeline runner
//     does not re-order.
//
//   - RESUME ALGORITHM (plan §2 AD-5c, locked):
//
//     1. Read the latest IngestionTaskLog.Checkpoint for the task.
//     2. Identify the LAST MATERIALIZED boundary present in
//     checkpoint["files"]. The set is {none, file_ref, chunks.jsonl}.
//     3. Resume at the component immediately following that boundary:
//     none             -> "File"
//     file_ref         -> "Parser"
//     chunks.jsonl     -> "Tokenizer"
//     4. Components between the materialized boundary and the
//     resumed point re-run from scratch (no partial-resume
//     within a component). The pipeline does NOT trust
//     informational-only checkpoints for "in-memory" stages
//     (Parser output is in memory, not materialized).
//     5. On startup, RestoreFromCheckpoint returns the next stage
//     and the materialized input map (e.g., for
//     chunks.jsonl-resume, the input to Tokenizer is the file
//     reference to chunks.jsonl, not the prior stage's
//     in-memory output).
package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/storage"
)

// Resume-boundary constants. The pipeline runner identifies the
// last materialized checkpoint entry through a structured
// `boundaries` map keyed by stage name (NOT via path-suffix
// matching — the prior implementation had a real bug where
// recordFileRef wrote "bucket/path" but resolveBoundary looked
// for HasSuffix(..., "file_ref"), so the File boundary was
// effectively undetectable on resume). The structured map is
// the single source of truth for resume decisions; the `files`
// list is kept only for downstream cleanup tracking.
const (
	// BoundaryKindFile is the key in the boundaries map for the
	// File component's materialized output (storage bucket+path
	// of the fetched source binary).
	BoundaryKindFile = "file"
	// BoundaryKindChunker is the key in the boundaries map for
	// the Chunker variants' materialized output (the chunks.jsonl
	// storage key).
	BoundaryKindChunker = "chunker"
)

// Stage-name constants. These are duplicated from the registered
// component names in internal/ingestion/component/* because the
// pipeline package cannot import the component package directly:
// the component/chunker subpackage imports the parent ingestion
// package, which transitively imports this pipeline package, so a
// direct import would close a cycle. Keeping the constants here
// preserves the static link without breaking the build; a future
// refactor that breaks the ingestion ↔ pipeline cycle can move
// these back to the component package (and have pipeline import
// the constants) without changing the wire-level behavior.
const (
	stageFile             = "File"
	stageParser           = "Parser"
	stageTokenizer        = "Tokenizer"
	stageTokenChunker     = "TokenChunker"
	stageTitleChunker     = "TitleChunker"
	stageGroupChunker     = "GroupTitleChunker"
	stageHierarchyChunker = "HierarchyTitleChunker"
)

// ComponentStep is one stage in the pipeline DAG.
type ComponentStep struct {
	Name             string
	Component        runtime.Component
	Parallelism      int
	ComponentVersion string         // contract version declared at registration; part of the idempotency key
	Params           map[string]any // per-stage params; feed into the input_fingerprint
}

// Pipeline is a compiled DAG of ingestion components. The
// pipeline is stateful across a single Run invocation: it
// carries a per-stage checkpoint map and the last successful
// component index. The struct is NOT safe for concurrent Run
// invocations on the same Pipeline value — callers construct a
// fresh Pipeline per task.
type Pipeline struct {
	taskID          string
	pipelineVersion string // SHA-256 of canonicalised DSL; part of every component_done row
	dsl             PipelineDSL
	stages          []ComponentStep
	sink            ProgressSink

	// storage is used to materialize intermediate results at the
	// two checkpoint boundaries (AD-5b). May be nil in tests
	// that don't exercise the chunker / tokenizer boundary.
	storage storage.Storage

	// bucket is the storage bucket name used for chunker
	// boundary writes. Production callers must set this via
	// SetBucket() to match the bucket used by File when it
	// fetched the source binary; otherwise the resume algorithm
	// will look in the wrong bucket for the materialized
	// chunks.jsonl. Defaults to DefaultPipelineBucket.
	bucket string

	// dao is the production checkpoint writer. Pipeline.Run
	// also calls LatestLogByTaskID + Create on it. May be nil
	// in tests that use a TestSink and don't need restore.
	dao *dao.IngestionTaskLogDAO

	mu              sync.Mutex
	lastCheckpoint  map[string]any
	completedStages []string
}

// DefaultPipelineBucket is the storage bucket used when callers
// do not explicitly set one via SetBucket(). Matches the
// historical hard-coded value so existing storage layouts continue
// to work; new deployments should pass the bucket that File used
// to fetch the source binary so the resume algorithm reads from
// the same bucket the writer wrote to.
const DefaultPipelineBucket = "ragflow"

// IdempotencyKey is the (task_id, pipeline_version, component_name,
// component_version, input_fingerprint) tuple that uniquely
// identifies one "stage run" and is the contract for stage-skip
// decisions (plan §4 Phase 3 task 0c). The fields are exported so
// tests and downstream consumers can construct / inspect the key
// without re-implementing the canonicalisation.
type IdempotencyKey struct {
	TaskID           string
	PipelineVersion  string // SHA-256 of the canonicalised DSL
	ComponentName    string
	ComponentVersion string // declared next to each registration
	InputFingerprint string // SHA-256 of upstream payload + params
}

// String returns a stable, colon-separated representation of the
// key. The format is fixed (do not change without a storage
// migration): the key is consumed both as a map key in the
// in-memory checkpoint and as a JSON field, so any
// reformatting would invalidate existing rows.
func (k IdempotencyKey) String() string {
	return k.TaskID + "|" + k.PipelineVersion + "|" + k.ComponentName + "|" + k.ComponentVersion + "|" + k.InputFingerprint
}

// ComputePipelineVersion returns the SHA-256 hex digest of the
// canonicalised DSL JSON. Two DSLs that differ only in key order
// or whitespace produce the same digest; structural changes
// (added/removed stage, changed params) produce a different one.
// The implementation is intentionally minimal — it sorts the top-
// level keys and re-marshals before hashing, so callers can pass
// pretty-printed JSON without losing idempotency.
func ComputePipelineVersion(dsl []byte) string {
	if len(dsl) == 0 {
		return ""
	}
	var raw any
	if err := json.Unmarshal(dsl, &raw); err != nil {
		// Malformed DSL: fall back to hashing the raw bytes. The
		// caller (NewPipelineFromDSL) will fail anyway; this
		// keeps the version deterministic for diagnostic logs.
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

// canonicaliseValue recursively sorts map keys for stable JSON
// encoding. Used by ComputePipelineVersion to make the digest
// independent of key order. A visited set keyed by reflect
// pointer breaks cycles — the upstream `current` payload that
// ComputeStageFingerprint hashes can legitimately contain
// self-references (e.g. test mock components that emit
// `{"received": inputs}`). Without the cycle break the
// recursion overflows the goroutine stack on the first
// non-trivial pipeline run.
func canonicaliseValue(v any) any {
	visited := make(map[uintptr]struct{})
	return canonicaliseValueWith(v, visited)
}

// canonicaliseValueWith is the recursion body. The visited set
// records every map / slice pointer we are currently inside;
// re-entering one returns the sentinel "null" instead of
// descending again, so the JSON marshal and the SHA-256 hash
// stay deterministic AND the goroutine stack stays bounded.
func canonicaliseValueWith(v any, visited map[uintptr]struct{}) any {
	switch x := v.(type) {
	case map[string]any:
		ptr := reflect.ValueOf(x).Pointer()
		if _, seen := visited[ptr]; seen {
			// Cycle: stop the descent. The sentinel "null"
			// is marshalled as the JSON null, which is
			// distinct from an empty map ("{}") and is
			// therefore observable in the digest when the
			// same cycle shape recurs.
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

// ComputeParamsFingerprint returns a stable SHA-256 hex digest
// of the per-stage parameters that affect the component's
// output. The map is JSON-encoded with sorted keys so callers
// can pass maps in any order without breaking idempotency.
// Empty / nil produce the SHA-256 of "null" — the canonical JSON
// encoding of nil — not the empty string, so a truly empty
// payload is distinguishable from "fingerprint not computed".
//
// SCOPE NOTE (Critical-fix #2): this function ONLY hashes the
// per-stage params. For stage-skip decisions that must also
// detect upstream-payload changes (a re-run of File that emits a
// different binary must invalidate Parser's done row), use
// ComputeStageFingerprint which combines `upstream` (the
// component's effective input map at run time) with `params`.
// Pipeline.Run uses ComputeStageFingerprint for the IsStageDone
// check; this single-arg helper is retained for tests and
// observers that need a params-only digest.
func ComputeParamsFingerprint(params map[string]any) string {
	return computeMapFingerprint(params)
}

// ComputeStageFingerprint returns a stable SHA-256 hex digest of
// the union of (a) the upstream payload that flows into the
// component at run time and (b) the per-stage params. A change in
// either side invalidates the stage-skip decision. This is the
// strict-deps contract the resume algorithm needs to refuse a
// stale skip when the upstream File output or prior chunker
// boundary changes between runs.
//
// `upstream` is the component's effective input map (the
// Pipeline's `current` map at the stage boundary) and may be
// nil. `params` may be nil. Either side is canonicalised under
// the JSON sorted-key contract so callers may pass any key
// order without breaking idempotency.
func ComputeStageFingerprint(upstream, params map[string]any) string {
	up := canonicaliseValue(normaliseNil(upstream))
	pr := canonicaliseValue(normaliseNil(params))
	// Wrap in a labelled structure so a params-only change cannot
	// collide with an upstream-only change that happens to share
	// the same canonicalised bytes.
	wrapped := map[string]any{
		"upstream": up,
		"params":   pr,
	}
	return computeMapFingerprint(wrapped)
}

// normaliseNil returns the empty map for a nil input so the
// downstream canonicaliseValue call doesn't produce Go's nil
// interface (encoding/json would marshal nil as "null", which
// is fine for hashing but confuses callers that pass the same
// value to other map ops).
func normaliseNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}

// computeMapFingerprint is the shared marshalling+hash step that
// ComputeParamsFingerprint and ComputeStageFingerprint build on.
// Kept private because callers must use one of the two named
// entry points (params-only vs. stage-composite) — a footgun in
// the public surface would let a caller pass a single map and
// have its upstream side silently omitted from the digest.
func computeMapFingerprint(m map[string]any) string {
	canonical, err := json.Marshal(canonicaliseValue(m))
	if err != nil {
		// Last-ditch fallback: hash the fmt.Sprintf of the map.
		// Pathological only when params contains a non-marshalable
		// value (e.g. a channel), which the pipeline runner does
		// not produce.
		sum := sha256.Sum256([]byte(fmt.Sprintf("%v", m)))
		return hex.EncodeToString(sum[:])
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

// IsStageDone reports whether a `component_done` row already
// exists for the given (pipeline_version, component_name,
// component_version, input_fingerprint) tuple. The check is a
// strict quadruple match: a change in any one field invalidates
// the prior done row and the stage re-runs. This is the
// fail-closed behaviour plan §4 Phase 3 task 0c requires.
//
// The function also fails closed when the row is missing the
// pipeline_version field (legacy row written before the field
// was added); a re-run is forced because the missing field makes
// the strict-quad invariant unverifiable.
func IsStageDone(checkpoint map[string]any, pipelineVersion, componentName, componentVersion, inputFingerprint string) bool {
	done, ok := checkpoint["component_done"].(map[string]any)
	if !ok {
		return false
	}
	row, ok := done[componentName].(map[string]any)
	if !ok {
		return false
	}
	// Fail-closed on a row that predates the pipeline_version
	// field: the invariant is unverifiable, so force a re-run.
	persistedPV, hasPV := row["pipeline_version"].(string)
	if !hasPV || persistedPV == "" {
		return false
	}
	if persistedPV != pipelineVersion {
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

// recordStageDone writes a `component_done` row for the given
// idempotency tuple. The row is the structured counterpart to
// the legacy "completed_components" list — the list tracks stage
// ordering while this row tracks "done-ness" with the full
// idempotency key. Both are kept; consumers can use either.
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

// SetBucket configures the storage bucket used by the pipeline
// runner for chunker boundary writes. Production callers should
// set this to the same bucket the File component used to fetch
// the source binary (typically derived from the upstream tenant
// config), so the resume algorithm's chunks.jsonl lookup hits
// the same bucket the boundary writer wrote to.
func (p *Pipeline) SetBucket(bucket string) {
	if bucket == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bucket = bucket
}

// NewPipelineFromDSL compiles a JSON DSL into a Pipeline. The
// DSL is validated, each stage's factory is resolved from
// runtime.DefaultRegistry, and the components are instantiated
// once at compile time (not per-invocation).
//
// taskID is the IngestionTask ID; it is stamped on every
// checkpoint row the pipeline writes. Empty is permitted for
// tests that don't care about row identity.
//
// sink may be nil for tests that don't care about checkpoints
// (the runner silently skips Persist in that case — see
// Pipeline.Run).
//
// storage may be nil for tests that don't exercise the chunker
// boundary. Production callers should pass
// storage.GetStorageFactory().GetStorage().
//
// dao may be nil in tests; production callers should pass
// dao.NewIngestionTaskLogDAO() (or use a TestSink which records
// in-memory).
func NewPipelineFromDSL(dsl []byte, taskID string, sink ProgressSink, stg storage.Storage, logs *dao.IngestionTaskLogDAO) (*Pipeline, error) {
	var d PipelineDSL
	if err := json.Unmarshal(dsl, &d); err != nil {
		return nil, fmt.Errorf("pipeline: decode DSL: %w", err)
	}
	if err := d.IsValid(); err != nil {
		return nil, err
	}
	// Plan §4 Phase 3 task 0c: pipeline_version is the SHA-256
	// of the canonicalised DSL. Computed once at compile time
	// and stamped on every component_done row the runner writes.
	pipelineVersion := ComputePipelineVersion(dsl)

	steps := make([]ComponentStep, 0, len(d.Stages))
	for i, s := range d.Stages {
		factory, _, meta, ok := runtime.DefaultRegistry.Lookup(s.Type)
		if !ok {
			return nil, fmt.Errorf("%w: %q at stage %d", errUnknownComponent, s.Type, i)
		}
		comp, err := factory(s.Type, s.Params)
		if err != nil {
			return nil, fmt.Errorf("pipeline: build %q: %w", s.Type, err)
		}
		// component_version falls back to the registration's
		// declared Version; empty means the component didn't
		// supply one (legacy ingestion components with
		// Metadata{Version:"1.0.0"} set it explicitly today).
		cv := meta.Version
		if cv == "" {
			cv = "unspecified"
		}
		steps = append(steps, ComponentStep{
			Name:             s.Type,
			Component:        comp,
			Parallelism:      parallelHint(comp),
			ComponentVersion: cv,
			Params:           s.Params,
		})
	}

	return &Pipeline{
		taskID:          taskID,
		pipelineVersion: pipelineVersion,
		dsl:             d,
		stages:          steps,
		sink:            sink,
		storage:         stg,
		bucket:          DefaultPipelineBucket,
		dao:             logs,
		lastCheckpoint:  make(map[string]any),
		completedStages: make([]string, 0, len(steps)),
	}, nil
}

// parallelHint queries the optional Parallelism() method via
// interface assertion. Components that don't implement it
// default to 1, matching the python sequential per-stage
// behaviour.
func parallelHint(c runtime.Component) int {
	type paraller interface {
		Parallelism() int
	}
	if p, ok := c.(paraller); ok {
		return p.Parallelism()
	}
	return 1
}

// Run executes the pipeline starting from `startAt`. startAt
// is the index of the first stage to run; 0 runs the full
// pipeline. A non-zero startAt is used by RestoreFromCheckpoint
// to skip stages before the resumed-from boundary.
// Pipeline.Run lives in run_canvas.go (Phase 4 cutover). The
// legacy direct-walk that this comment block replaced has been
// removed; the canvas-driven path is the only entry point.

// runStage invokes one stage with its configured parallelism.
// Components that declare Parallelism() > 1 are responsible for
// their own fan-out / fan-in (the Python component equivalents
// already do this — see e.g., parser.go:fanOutAndMerge). The
// pipeline runner does not split inputs; it hands the entire
// `current` map to the component and lets the component
// distribute. For components that do not implement their own
// fan-out, the runner still wraps the call in TrackProgress +
// TrackElapsed for consistency.
//
// statuses is the per-goroutine outcome the runner reports back
// to the checkpoint. The current implementation captures only
// a single "done" status per stage (the component's own
// fan-out merges before returning). A future enhancement could
// let components emit per-goroutine status; today the
// goroutine_status[] entry is a single row.
func (p *Pipeline) runStage(ctx context.Context, step ComponentStep, inputs map[string]any) (map[string]any, []GoroutineStatus, error) {
	out, err := runtime.TrackElapsed(step.Name, func() (map[string]any, error) {
		progressErr := runtime.TrackProgress(step.Name, runtime.ProgressCallback(nil), func() error {
			return runtime.WithTimeout(ctx, stageTimeout(), func(timeoutCtx context.Context) error {
				invokeOut, invokeErr := step.Component.Invoke(timeoutCtx, inputs)
				if invokeErr != nil {
					return invokeErr
				}
				inputs = mergeInto(inputs, invokeOut)
				return nil
			})
		})
		if progressErr != nil {
			return nil, progressErr
		}
		return inputs, nil
	})
	if err != nil {
		return nil, nil, err
	}

	// Emit one status row per Parallelism. Components handle
	// their own fan-out internally, so the runner only sees
	// one merged result. Recording N rows of "done" keeps the
	// goroutine_status[] shape consistent with plan §2 AD-5c
	// (a future enhancement can have components emit
	// per-goroutine status via an optional ComponentWithStatus
	// interface; today we don't expose that).
	n := step.Parallelism
	if n < 1 {
		n = 1
	}
	statuses := make([]GoroutineStatus, 0, n)
	for i := 0; i < n; i++ {
		statuses = append(statuses, GoroutineStatus{Goroutine: i, Status: "done"})
	}
	return out, statuses, nil
}

// mergeInto overlays src onto dst, with src winning. Returns
// the destination for chaining.
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

// isChunkerStage reports whether a stage name matches one of the
// 4 chunker variants. Used by the runner to decide when to persist
// the materialized chunks.jsonl boundary (plan §2 AD-5b).
func isChunkerStage(name string) bool {
	switch name {
	case stageTokenChunker, stageTitleChunker, stageGroupChunker, stageHierarchyChunker:
		return true
	}
	return false
}

// persistChunkerBoundary writes the chunk list to storage as JSON
// Lines under {taskID}/{stageName}/chunks.jsonl, then records the
// resulting key in two places:
//
//   - p.lastCheckpoint["files"] — propagated for downstream cleanup
//     (RemoveByAPIServerOrAdminServer walks this list to GC the
//     intermediate artifacts).
//   - p.lastCheckpoint["boundaries"][BoundaryKindChunker] — the
//     structured entry the resume algorithm reads to decide whether
//     to skip the Chunker stage on the next run.
//
// Returns nil if storage is nil or no chunks were emitted (no-op
// boundary).
func (p *Pipeline) persistChunkerBoundary(ctx context.Context, stageName string, out map[string]any) error {
	if p.storage == nil {
		return nil
	}
	chunks, ok := out["chunks"].([]string)
	if !ok {
		// Some chunkers emit "chunks" as []map[string]any. Marshal those
		// as the JSON Lines payload so downstream consumers can decode
		// either form.
		if alt, okAlt := out["chunks"].([]map[string]any); okAlt {
			return p.writeJSONLWithBoundary(ctx, stageName, chunksToLines(alt))
		}
		// No chunks emitted — no boundary to persist.
		return nil
	}
	return p.writeJSONLWithBoundary(ctx, stageName, chunksToLinesFromStrings(chunks))
}

// writeJSONL serialises the lines to JSON Lines, calls storage.Put,
// and records the key in p.lastCheckpoint["files"]. The bucket is
// p.bucket (set via SetBucket; defaults to DefaultPipelineBucket).
// The ctx parameter is unused but kept for symmetry with the rest
// of the pipeline API surface (future storage interfaces may accept
// it).
func (p *Pipeline) writeJSONL(_ context.Context, stageName string, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	payload := make([]byte, 0, len(lines)*64)
	for _, l := range lines {
		payload = append(payload, l...)
		payload = append(payload, '\n')
	}
	p.mu.Lock()
	bucket := p.bucket
	p.mu.Unlock()
	key := fmt.Sprintf("%s/%s/chunks.jsonl", p.taskID, stageName)
	if err := p.storage.Put(bucket, key, payload); err != nil {
		return err
	}
	p.mu.Lock()
	files, _ := p.lastCheckpoint["files"].([]string)
	alreadyRecorded := false
	for _, f := range files {
		if f == key {
			alreadyRecorded = true
			break
		}
	}
	if !alreadyRecorded {
		files = append(files, key)
		p.lastCheckpoint["files"] = files
	}
	p.mu.Unlock()
	return nil
}

// writeJSONLWithBoundary is writeJSONL plus the structured
// `boundaries[BoundaryKindChunker]` entry. Kept separate from
// writeJSONL so the two concerns (cleanup vs resume) can evolve
// independently — a future optimization that wants to skip the
// in-memory copy of the file list can do so without breaking the
// resume contract.
func (p *Pipeline) writeJSONLWithBoundary(ctx context.Context, stageName string, lines []string) error {
	if err := p.writeJSONL(ctx, stageName, lines); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	boundaries, _ := p.lastCheckpoint["boundaries"].(map[string]map[string]any)
	if boundaries == nil {
		boundaries = make(map[string]map[string]any)
	}
	boundaries[BoundaryKindChunker] = map[string]any{
		"chunks_jsonl": fmt.Sprintf("%s/%s/chunks.jsonl", p.taskID, stageName),
		"stage":        stageName,
	}
	p.lastCheckpoint["boundaries"] = boundaries
	return nil
}

// chunksToLines serialises []map[string]any to JSON Lines (one
// JSON object per line).
func chunksToLines(chunks []map[string]any) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		b, err := json.Marshal(c)
		if err != nil {
			continue
		}
		out = append(out, string(b))
	}
	return out
}

// chunksToLinesFromStrings emits each chunk string as a single
// JSON Lines line (i.e. a quoted string). Consumers that read
// chunks.jsonl get a list of plain-text chunks.
func chunksToLinesFromStrings(chunks []string) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		b, err := json.Marshal(c)
		if err != nil {
			continue
		}
		out = append(out, string(b))
	}
	return out
}

func cloneMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// stageTimeout returns a per-stage timeout. The Python pipeline
// uses a 60s asyncio.wait_for ceiling via ProcessBase; we
// mirror that for all stages. A future enhancement can pull a
// per-stage timeout from the DSL params.
func stageTimeout() time.Duration {
	return defaultStageTimeout
}

// defaultStageTimeout mirrors the python ProcessBase 60s
// ceiling. Declared as a var (not const) so tests can shrink
// it. The exact duration is opaque to most tests; the
// HonoursTimeout test relies on this hook.
var defaultStageTimeout = 60 * time.Second

// recordSuccess updates the in-memory checkpoint map after a
// stage completes. It is the central place where the resume
// algorithm reads its state.
//
// The persisted field is `work_unit_status` (Medium-fix #5): the
// runner is work-unit keyed, not goroutine keyed. The earlier
// `goroutine_status` name persists for read-side
// compatibility for one release (the runner writes the new
// name and the map is mirrored under the legacy key so a
// pre-rename reader on an older row still sees a recognisable
// value).
func (p *Pipeline) recordSuccess(name string, statuses []GoroutineStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.completedStages = append(p.completedStages, name)
	p.lastCheckpoint["current_component"] = name
	p.lastCheckpoint["completed_components"] = append([]string(nil), p.completedStages...)
	p.lastCheckpoint["total_components"] = len(p.stages)
	ws, _ := p.lastCheckpoint["work_unit_status"].(map[string][]GoroutineStatus)
	if ws == nil {
		ws = make(map[string][]GoroutineStatus)
	}
	ws[name] = statuses
	p.lastCheckpoint["work_unit_status"] = ws
	// Read-side compatibility shim for any consumer that still
	// looks up the legacy key. The mirror is best-effort: once
	// all readers migrate to `work_unit_status`, the mirror can
	// be retired.
	p.lastCheckpoint["goroutine_status"] = ws
}

// recordStageSkip records that a stage was skipped because its
// component_done row was already present. The skip is observable
// in the checkpoint via the `skipped_stages` list so a test or
// operator can see exactly which stages the runner decided to
// short-circuit.
func (p *Pipeline) recordStageSkip(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	skipped, _ := p.lastCheckpoint["skipped_stages"].([]string)
	for _, s := range skipped {
		if s == name {
			return
		}
	}
	skipped = append(skipped, name)
	p.lastCheckpoint["skipped_stages"] = skipped
}

// recordStageDoneWithKey writes a structured component_done row
// using the full idempotency key (plan §4 Phase 3 task 0c). The
// row is consumed by IsStageDone on the next run; a change in
// any of the four key components (pipeline_version,
// component_version, input_fingerprint, component_name) forces
// a re-run.
func (p *Pipeline) recordStageDoneWithKey(key IdempotencyKey) {
	p.mu.Lock()
	defer p.mu.Unlock()
	recordStageDone(p.lastCheckpoint, key)
}

// recordFileRef records the File component's materialized output
// (the storage bucket+path the binary was fetched from) into
// lastCheckpoint["files"] so the resume algorithm can skip File
// on the next run. The format is a synthetic key of the form
// "{bucket}/{path}" (NOT a real storage object — the binary
// already exists; we only carry a reference so resolveBoundary
// recordFileRef records the File component's materialized output
// (the storage bucket+path the binary was fetched from) into
// the structured `boundaries[BoundaryKindFile]` entry so the
// resume algorithm can skip File and start at Parser on the
// next run. The `files` list is also updated for downstream
// cleanup, but the resume decision is now driven by the
// `boundaries` map (the prior HasSuffix(path, "file_ref")
// approach silently failed because real paths never end with
// the literal string "file_ref").
func (p *Pipeline) recordFileRef(out map[string]any) {
	bucket, _ := out["bucket"].(string)
	path, _ := out["path"].(string)
	if bucket == "" && path == "" {
		return
	}
	key := bucket + "/" + path
	p.mu.Lock()
	defer p.mu.Unlock()
	// Update the structured boundaries map (resume source of truth).
	boundaries, _ := p.lastCheckpoint["boundaries"].(map[string]map[string]any)
	if boundaries == nil {
		boundaries = make(map[string]map[string]any)
	}
	boundaries[BoundaryKindFile] = map[string]any{
		"bucket": bucket,
		"path":   path,
		"key":    key,
	}
	p.lastCheckpoint["boundaries"] = boundaries
	// Keep `files` in sync for downstream cleanup.
	files, _ := p.lastCheckpoint["files"].([]string)
	alreadyRecorded := false
	for _, f := range files {
		if f == key {
			alreadyRecorded = true
			break
		}
	}
	if !alreadyRecorded {
		files = append(files, key)
		p.lastCheckpoint["files"] = files
	}
}

// recordFailure annotates the checkpoint with the failed
// component and the error message. It does NOT clear
// completed_components — partial progress is preserved so a
// resume run can skip past it.
func (p *Pipeline) recordFailure(name string, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastCheckpoint["current_component"] = name
	p.lastCheckpoint["last_error"] = err.Error()
}

// flushCheckpoint copies the in-memory checkpoint map to the
// sink. The sink is responsible for serialising. If sink is
// nil (test case), this is a no-op.
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

// LastCheckpoint returns a copy of the current in-memory
// checkpoint. Test-only helper — production callers should
// read the persisted row from the sink.
func (p *Pipeline) LastCheckpoint() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]any, len(p.lastCheckpoint))
	for k, v := range p.lastCheckpoint {
		out[k] = v
	}
	return out
}

// Sink returns the ProgressSink the Pipeline writes to, or
// nil. Test-only helper for assertions on TestSink / DB sink
// state after a run. Production code does not need this.
func (p *Pipeline) Sink() ProgressSink {
	return p.sink
}

// StageNames returns the declared stage names in execution
// order. Test-only helper.
func (p *Pipeline) StageNames() []string {
	out := make([]string, len(p.stages))
	for i, s := range p.stages {
		out[i] = s.Name
	}
	return out
}

// RestoreResult is the output of RestoreFromCheckpoint.
type RestoreResult struct {
	StartAt          int            // index of the first stage to run in Pipeline.stages
	MaterializedFile string         // storage path of the materialized boundary (or "" if none)
	Inputs           map[string]any // upstream inputs to feed into the resumed stage
	Files            []string       // all materialized file paths (so callers can GC them later)
}

// RestoreFromCheckpoint reads the latest IngestionTaskLog for
// taskID, identifies the last materialized boundary, and
// returns the index of the next stage to run along with the
// materialized input map.
//
// The algorithm follows plan §2 AD-5c:
//
//   - if files[] is empty OR missing: start at 0 (File).
//   - if boundaries[BoundaryKindFile] is present but not
//     boundaries[BoundaryKindChunker]: start at the first stage
//     after File (typically 1 = Parser). The materialized input
//     map is {binary: []byte, file_ref_bucket, file_ref_path} —
//     the binary is REHYDRATED from storage on the resume path
//     so Parser can consume it without a path-aware fork (see
//     Critical-fix #4).
//   - if boundaries[BoundaryKindChunker] is present: start at
//     the stage named "Tokenizer". The materialized input map
//     is {chunks_jsonl: path} pointing at chunks.jsonl.
//
// On every load the runner also hydrates `p.lastCheckpoint`
// with the persisted `boundaries` and `component_done` maps
// (Critical-fix #3) so the run-loop's IsStageDone check sees
// the prior `component_done` rows it needs to skip repeated
// stages.
//
// A nil dao is treated as "no prior checkpoint" — start at 0.
func (p *Pipeline) RestoreFromCheckpoint(taskID string) (*RestoreResult, error) {
	if p.dao == nil {
		return &RestoreResult{StartAt: 0, Inputs: map[string]any{}}, nil
	}
	row, err := p.dao.LatestLogByTaskID(taskID)
	if err != nil {
		// Distinguish "DB error" from "no row": a DAO query failure
		// must surface as an error so executeTask can fail the task
		// rather than silently re-running the whole pipeline.
		return nil, fmt.Errorf("pipeline: load latest log for task %q: %w", taskID, err)
	}
	if row == nil {
		// Genuinely no prior checkpoint — start from scratch.
		return &RestoreResult{StartAt: 0, Inputs: map[string]any{}}, nil
	}
	files := readFilesList(row.Checkpoint)
	boundaries := readBoundariesMap(row.Checkpoint)
	// Critical-fix #3: hydrate the in-memory checkpoint map with
	// the persisted boundaries + component_done rows so the
	// run-loop's IsStageDone check can find them at run start.
	// Without this, prior component_done rows were effectively
	// invisible: Run builds p.lastCheckpoint from empty and only
	// learns about new rows as they are written. Re-running with
	// the same idempotency key would then take the long path
	// (re-execute every stage) instead of the intended skip.
	p.hydrateCheckpointFromRow(row.Checkpoint)
	res, err := p.resolveBoundary(files, boundaries)
	if err != nil {
		return nil, err
	}
	// After resolveBoundary, if we landed on a File-boundary
	// resume, the run-loop expects the binary to be available
	// (see Critical-fix #4); resolveBoundary already populated
	// res.Inputs["binary"] when storage is non-nil, so no further
	// work is needed.
	return res, nil
}

// hydrateCheckpointFromRow copies the persisted `boundaries`,
// `component_done`, and `files` rows into p.lastCheckpoint under
// the same keys. The mutex is taken once for the whole copy so
// the run-loop sees a consistent snapshot when Run() picks up
// from there.
//
// Called exclusively from RestoreFromCheckpoint; unit tests
// cover the round-trip via TaskLogSink in
// TestPipeline_Restore_HydratesLastCheckpoint.
func (p *Pipeline) hydrateCheckpointFromRow(cp map[string]any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if bd, ok := cp["boundaries"].(map[string]any); ok {
		p.lastCheckpoint["boundaries"] = bd
	}
	if done, ok := cp["component_done"].(map[string]any); ok {
		p.lastCheckpoint["component_done"] = done
	}
	if files, ok := cp["files"]; ok && files != nil {
		p.lastCheckpoint["files"] = files
	}
}

// resolveBoundary converts the materialized boundaries and
// file list into a resume decision. The structured `boundaries`
// map (keyed by BoundaryKindFile / BoundaryKindChunker) is the
// single source of truth; the `files` list is propagated for
// downstream cleanup but does not drive the resume decision.
//
// Matching strategy (plan §2 AD-5c, refined by the boundary-
// protocol fix + Critical-fix #4 + Critical-fix #5):
//
//   - File boundary is detected via the structured `boundaries`
//     map, not path-suffix matching.
//   - When the File boundary is found AND `p.storage` is
//     non-nil, the resume algorithm REHYDRATES the binary from
//     storage and exposes it as `Inputs["binary"]` ([]byte).
//     `file_ref_bucket`/`file_ref_path` are kept alongside for
//     diagnostics and for callers that want to re-fetch (e.g.,
//     a deferred download).
//   - When storage is nil (test path without storage, or
//     configured to skip the rehydrate), the legacy
//     `file_ref_*` inputs are emitted so callers that perform
//     their own fetch can still observe the boundary.
//   - When the Chunker boundary is found AND `p.storage` is
//     non-nil, the resume algorithm REHYDRATES the parsed
//     chunks from chunks.jsonl and exposes them as
//     `Inputs["chunks"]` ([]map[string]any) — the canonical
//     shape Tokenizer's getChunks accepts (Critical-fix #5).
//     `chunks_jsonl` is kept alongside for diagnostics.
//
// Critical-fix #4/#5 rationale: each downstream component's
// `Invoke` reads a strongly-typed input key (`binary` for
// Parser, `chunks` for Tokenizer). Emitting only storage
// references forced the components to grow path-aware forks.
// Rehydrating the bytes here keeps each component's contract
// narrow and makes the resume behaviour match a real
// production run end to end.
func (p *Pipeline) resolveBoundary(files []string, boundaries map[string]map[string]any) (*RestoreResult, error) {
	res := &RestoreResult{Files: append([]string(nil), files...)}
	if len(boundaries) == 0 {
		res.StartAt = 0
		res.Inputs = map[string]any{}
		return res, nil
	}

	chunksBoundary, hasChunks := boundaries[BoundaryKindChunker]
	fileBoundary, hasFile := boundaries[BoundaryKindFile]

	switch {
	case hasChunks:
		chunksPath, _ := chunksBoundary["chunks_jsonl"].(string)
		if chunksPath == "" {
			// Malformed boundary entry — fall back to a full re-run.
			res.StartAt = 0
			res.Inputs = map[string]any{}
			return res, nil
		}
		// Resume at Tokenizer; the materialized input carries
		// either rehydrated chunks (when storage is available,
		// see Critical-fix #5) or the storage path (legacy
		// fallback that drives a path-aware future Tokenizer).
		idx := p.indexOfStage(stageTokenizer)
		if idx < 0 {
			// Plan §2 AD-5c: the materialized boundary claims we
			// already produced chunks.jsonl, but the DSL has no
			// Tokenizer stage to consume them. Silently succeeding
			// here would mark the task COMPLETED with no work done.
			// Fall back to a full re-run from File rather than
			// fabricating a successful no-op.
			return nil, fmt.Errorf("pipeline: chunks boundary present but DSL has no %s stage; refusing to silently skip work (task %q)", stageTokenizer, p.taskID)
		}
		res.StartAt = idx
		res.MaterializedFile = chunksPath
		// Critical-fix #5: rehydrate the chunks from storage when
		// available so Tokenizer (and any downstream component
		// that consumes `chunks`) sees the canonical
		// []map[string]any shape it already understands.
		if p.storage != nil && chunksPath != "" {
			data, err := p.storage.Get(p.bucket, chunksPath)
			if err != nil {
				return nil, fmt.Errorf("pipeline: rehydrate chunker boundary %q: %w", chunksPath, err)
			}
			chunks, err := parseJSONLChunks(data)
			if err != nil {
				return nil, fmt.Errorf("pipeline: parse rehydrated chunks from %q: %w", chunksPath, err)
			}
			res.Inputs = map[string]any{
				"chunks":       chunks,
				"chunks_jsonl": chunksPath,
			}
			return res, nil
		}
		res.Inputs = map[string]any{
			"chunks_jsonl": chunksPath,
		}
		return res, nil
	case hasFile:
		bucket, _ := fileBoundary["bucket"].(string)
		path, _ := fileBoundary["path"].(string)
		if bucket == "" && path == "" {
			// Malformed boundary entry — fall back to a full re-run.
			res.StartAt = 0
			res.Inputs = map[string]any{}
			return res, nil
		}
		idx := p.indexOfStage(stageParser)
		if idx < 0 {
			return nil, fmt.Errorf("pipeline: file boundary present but DSL has no %s stage; refusing to silently skip work (task %q)", stageParser, p.taskID)
		}
		res.StartAt = idx
		// Critical-fix #4: rehydrate the binary from storage when
		// available. We keep the file_ref_* keys alongside so a
		// test or operator can still inspect the boundary entries.
		if p.storage != nil && bucket != "" && path != "" {
			data, err := p.storage.Get(bucket, path)
			if err != nil {
				return nil, fmt.Errorf("pipeline: rehydrate file boundary %q/%q: %w", bucket, path, err)
			}
			res.Inputs = map[string]any{
				"binary":          data,
				"file_ref_bucket": bucket,
				"file_ref_path":   path,
			}
			return res, nil
		}
		// Storage unavailable — fall back to the file_ref_* form
		// so the caller can decide what to do (a future Parser
		// may grow a path-aware branch).
		res.Inputs = map[string]any{
			"file_ref_bucket": bucket,
			"file_ref_path":   path,
		}
		return res, nil
	default:
		// boundaries map present but no recognized key — safer to
		// re-run from File than to skip a stage.
		res.StartAt = 0
		res.Inputs = map[string]any{}
		return res, nil
	}
}

// parseJSONLChunks decodes a JSONL payload (one JSON object or
// quoted string per line, written by chunksToLines /
// chunksToLinesFromStrings) into the canonical
// []map[string]any shape the Tokenizer's `getChunks` accepts.
//
// Each line is independently unmarshalled: a JSON object
// becomes a chunk entry directly, a JSON string becomes
// `{"text": "..."}`, and unparseable lines are skipped rather
// than failing the whole resume — chunks.jsonl is a soft
// intermediate, so partial recovery beats an outright failure
// when, e.g., one line was truncated by a transient storage
// race. Lines that decode as JSON `null` are also skipped to
// align with Python's null-tolerant chunk handling.
func parseJSONLChunks(data []byte) ([]map[string]any, error) {
	if len(data) == 0 {
		return nil, nil
	}
	out := make([]map[string]any, 0)
	for _, raw := range strings.Split(string(data), "\n") {
		raw = strings.TrimRight(raw, "\r")
		if raw == "" {
			continue
		}
		var anyVal any
		if err := json.Unmarshal([]byte(raw), &anyVal); err != nil {
			// Skip malformed lines but don't abort — the
			// downstream tokenizer would otherwise refuse to
			// start on a single bad line.
			continue
		}
		if anyVal == nil {
			// JSON `null` — skip, mirroring Python's
			// null-tolerant chunk handling.
			continue
		}
		switch v := anyVal.(type) {
		case map[string]any:
			if v == nil {
				continue
			}
			out = append(out, v)
		case string:
			out = append(out, map[string]any{"text": v})
		default:
			// numbers / booleans / null / arrays — wrap as
			// best-effort single-key map so the chunk shape
			// always satisfies getChunks.
			out = append(out, map[string]any{"text": raw})
		}
	}
	return out, nil
}

// indexOfStage returns the index of the stage whose name equals `name`.
//
// Resume tests also register "MockParser" / "MockTokenizer" style stage
// names so boundary resolution can exercise the production lookup without
// shadowing the real component registrations. That fallback is intentionally
// limited to the explicit "Mock" prefix to keep production stage matching
// exact.
func (p *Pipeline) indexOfStage(name string) int {
	for i, s := range p.stages {
		if s.Name == name {
			return i
		}
		if strings.HasPrefix(s.Name, "Mock") && strings.TrimPrefix(s.Name, "Mock") == name {
			return i
		}
	}
	return -1
}

// readFilesList extracts the `files` key from a JSONMap. The
// DAO decodes JSONList values as `[]any`, not `[]string`, so
// we coerce. Returns nil if the key is missing or empty.
func readFilesList(cp map[string]any) []string {
	v, ok := cp["files"]
	if !ok {
		return nil
	}
	switch raw := v.(type) {
	case []string:
		return append([]string(nil), raw...)
	case []any:
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		sort.Strings(out) // canonical for stable resume decisions
		return out
	}
	return nil
}

// readBoundariesMap extracts the `boundaries` map from a JSONMap
// checkpoint. Returns nil if the key is missing or the value is
// not the expected map type. The DAO round-trips nested maps as
// `map[string]any`, so each value is itself a `map[string]any`
// (the per-boundary data).
func readBoundariesMap(cp map[string]any) map[string]map[string]any {
	v, ok := cp["boundaries"]
	if !ok {
		return nil
	}
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]map[string]any, len(raw))
	for k, val := range raw {
		if inner, ok := val.(map[string]any); ok {
			out[k] = inner
		}
	}
	return out
}
