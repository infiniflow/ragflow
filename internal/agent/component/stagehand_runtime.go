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

// Package component — StagehandInvoker runtime (T3 sub-package).
//
// StagehandInvoker is the abstraction the Browser component uses to
// dispatch a multi-step web automation task via
// `github.com/browserbase/stagehand-go/v3` in local mode.
//
// # Multi-tenant client cache
//
// The production runtime (stagehandRuntime) maintains a cache of
// stagehand.Client instances keyed by
// `(apiKey, baseURL, modelName)`. Each cached client owns its own
// stagehand-server-v3 subprocess. The cache is bounded by:
//   - TTL (default 30 min): idle entries are evicted by a
//     background sweeper goroutine.
//   - LRU cap (default 64): when the cache exceeds `cap`, the
//     least-recently-used entry is evicted.
//
// This replaces the v1 "single-client, recycle on key change"
// model. The recycle model paid a 1–3 s subprocess rebuild on every
// tenant/model switch under mixed-tenant load; the cache model
// pays it once per tenant (cold start) and serves subsequent calls
// in O(1).
//
// # Configuration
//
// Tunable via env (read at DefaultRuntime construction time):
//   - STAGEHAND_CACHE_TTL:    entry idle-TTL (Go duration syntax, default 30m)
//   - STAGEHAND_CACHE_CAP:    max concurrent entries (default 64, 0 = unlimited)
//   - STAGEHAND_CACHE_SWEEP:  sweeper interval (default ttl/6, clamped to ≥ 30s)
//
// # Concurrency
//
// The cache itself is a sync.Map; reads are lock-free, writes use
// `LoadOrStore` to dedupe concurrent builds (the losing goroutine
// closes its orphan subprocess). The sweeper goroutine runs in the
// background; `Close()` drains it and shuts down every entry.
//
// # Dependency
//
// The runtime depends on a working `stagehand-server-v3-<platform>-<arch>`
// binary on the host (Docker: see `docker/Dockerfile`; local dev:
// `ResolveBinaryPath` in `lib/local/local.go` downloads from GitHub
// on first use).
package component

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	stagehand "github.com/browserbase/stagehand-go/v3"
	"github.com/browserbase/stagehand-go/v3/option"
)

// RunTaskRequest is the input to StagehandInvoker.RunTask.
//
// Fields are the per-call model config + task instruction. The
// runtime owns the stagehand-server process and the session
// lifecycle; the caller is responsible for resolving these from
// the tenant model config and the canvas state.
type RunTaskRequest struct {
	// TenantID is the RAGFlow tenant identifier (used for logging /
	// error context; not currently passed to stagehand).
	TenantID string
	// LLMID is the original `llm_id` from the canvas (e.g.
	// "deepseek-v4-pro@DeepSeek"). Echoed back in error context.
	LLMID string
	// ModelName is the stagehand `modelName` value (e.g.
	// "openai/gpt-4o"). Must be in `<provider>/<model>` form.
	ModelName string
	// BaseURL is the OpenAI-compatible LLM endpoint. When non-empty,
	// passed via the per-call Model.BaseURL field; when empty, the
	// stagehand server's default (api.openai.com) is used.
	BaseURL string
	// APIKey is the LLM provider key. Required by the stagehand
	// server in local mode (MODEL_API_KEY env).
	APIKey string
	// Instruction is the natural-language task description that
	// `Sessions.Execute` consumes.
	Instruction string
	// MaxSteps caps the agent's step count. Zero defaults to 30
	// (matches Python `browser.py:max_steps`).
	MaxSteps int
	// Headless controls `Sessions.Start.Browser.LaunchOptions.Headless`.
	// Nil = server default (true).
	Headless *bool

	// DownloadsPath is forwarded to
	// `Sessions.Start.Browser.LaunchOptions.DownloadsPath`. When
	// non-empty, the stagehand browser writes downloads to this
	// directory. Used by OQ-22a.
	DownloadsPath string
	// UserDataDir is forwarded to
	// `Sessions.Start.Browser.LaunchOptions.UserDataDir`. When
	// non-empty, the stagehand browser uses it as the persistent
	// profile dir. Used by OQ-22c.
	UserDataDir string
	// PreserveUserDataDir is forwarded to
	// `Sessions.Start.Browser.LaunchOptions.PreserveUserDataDir`.
	// Set when persist_session=true so cookies / local storage
	// survive across sessions. Used by OQ-22c.
	PreserveUserDataDir bool
}

// StagehandInvoker is the abstraction BrowserComponent depends on.
// Tests substitute a fake; production uses `DefaultRuntime`.
type StagehandInvoker interface {
	RunTask(ctx context.Context, req RunTaskRequest) (message string, err error)
	RunExtract(ctx context.Context, req RunExtractRequest) (rawJSON string, err error)
}

// Default cache parameters. Override via env at process start;
// DefaultRuntime reads these via envDuration / envInt at init.
const (
	defaultStagehandCacheTTL   = 30 * time.Minute
	defaultStagehandCacheCap   = 64
	defaultStagehandCacheSweep = 5 * time.Minute
	minStagehandCacheSweep     = 30 * time.Second
)

// DefaultRuntime is the package-level production invoker. Replaced
// in tests via `SetDefaultStagehandInvoker`.
var (
	DefaultRuntime StagehandInvoker = newStagehandRuntimeFromEnv()

	defaultRuntimeMu sync.RWMutex
)

// SetDefaultStagehandInvoker swaps the package-level invoker (test
// helper). Production code should not call this; the runtime is
// process-singleton.
func SetDefaultStagehandInvoker(inv StagehandInvoker) {
	defaultRuntimeMu.Lock()
	defer defaultRuntimeMu.Unlock()
	if inv == nil {
		DefaultRuntime = newStagehandRuntimeFromEnv()
		return
	}
	DefaultRuntime = inv
}

// getDefaultStagehandInvoker returns the current invoker under a
// read-lock so concurrent swaps during a test are race-free.
func getDefaultStagehandInvoker() StagehandInvoker {
	defaultRuntimeMu.RLock()
	defer defaultRuntimeMu.RUnlock()
	if DefaultRuntime == nil {
		return newStagehandRuntimeFromEnv()
	}
	return DefaultRuntime
}

// envDuration reads a Go-duration env var, falling back to def on
// missing / parse failure.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

// envInt reads a non-negative int env var, falling back to def.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}

// newStagehandRuntimeFromEnv builds a stagehandRuntime with config
// from env (or library defaults). Used as the production DefaultRuntime.
func newStagehandRuntimeFromEnv() *stagehandRuntime {
	ttl := envDuration("STAGEHAND_CACHE_TTL", defaultStagehandCacheTTL)
	cap := envInt("STAGEHAND_CACHE_CAP", defaultStagehandCacheCap)
	sweep := envDuration("STAGEHAND_CACHE_SWEEP", defaultStagehandCacheSweep)
	return newStagehandRuntime(ttl, cap, sweep)
}

// stagehandClientEntry is one cached stagehand.Client. lastUsedAt
// is touched on every cache hit; the sweeper uses it to decide
// eviction. closeOnce makes Close idempotent (called both by the
// sweeper on TTL eviction and by runtime.Close on shutdown).
type stagehandClientEntry struct {
	client     stagehand.Client
	lastUsedAt atomic.Int64 // unix nano
	closeOnce  sync.Once
}

// Close shuts down the client (kills the stagehand-server
// subprocess). Idempotent.
func (e *stagehandClientEntry) Close() {
	e.closeOnce.Do(func() {
		_ = e.client.Close()
	})
}

// stagehandRuntime is the production StagehandInvoker. It maintains
// a sync.Map cache of stagehand.Client instances keyed by
// `(apiKey, baseURL, modelName)`. See the package doc for the
// multi-tenant model rationale.
type stagehandRuntime struct {
	cache     sync.Map // map[string]*stagehandClientEntry
	ttl       time.Duration
	cap       int // 0 = unlimited
	sweepStop chan struct{}
	sweepDone chan struct{}
}

// newStagehandRuntime constructs a runtime with explicit config and
// starts the sweeper goroutine. Tests that want full control over
// timing should use this constructor (e.g., with very short TTL +
// short sweep interval) rather than newStagehandRuntimeFromEnv.
//
// sweepInterval is clamped to at least minStagehandCacheSweep to
// avoid sweeper-storm when ttl is small.
func newStagehandRuntime(ttl time.Duration, cap int, sweepInterval time.Duration) *stagehandRuntime {
	r := &stagehandRuntime{
		ttl:       ttl,
		cap:       cap,
		sweepStop: make(chan struct{}),
		sweepDone: make(chan struct{}),
	}
	if ttl > 0 && sweepInterval < minStagehandCacheSweep {
		sweepInterval = minStagehandCacheSweep
	}
	r.startSweeper(sweepInterval)
	return r
}

// startSweeper runs the TTL eviction loop in a background
// goroutine. It exits cleanly when sweepStop is closed; Close()
// drains via sweepDone.
func (r *stagehandRuntime) startSweeper(interval time.Duration) {
	if r.ttl <= 0 {
		// No TTL → no sweeper. Drain immediately so Close() doesn't
		// block.
		close(r.sweepDone)
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(r.sweepDone)
		for {
			select {
			case <-r.sweepStop:
				return
			case <-ticker.C:
				r.evictExpired()
			}
		}
	}()
}

// evictExpired removes entries whose lastUsedAt is older than ttl.
// Per-entry Close runs after the LoadAndDelete so concurrent
// readers either see the entry (and continue) or see nothing
// (and build a fresh one). The losing side of any LoadAndDelete
// race sees no entry and skips Close.
func (r *stagehandRuntime) evictExpired() {
	cutoff := time.Now().Add(-r.ttl).UnixNano()
	r.cache.Range(func(k, v any) bool {
		e := v.(*stagehandClientEntry)
		if e.lastUsedAt.Load() >= cutoff {
			return true // still fresh
		}
		if cur, loaded := r.cache.LoadAndDelete(k); loaded {
			cur.(*stagehandClientEntry).Close()
		}
		return true
	})
}

// enforceLRUCap evicts the least-recently-used entry when the cache
// exceeds `cap`. Called by clientFor after a successful new-entry
// insert. The Range + LoadAndDelete sequence is not strictly atomic
// (concurrent inserts may race), but we only need to converge to
// "size ≤ cap + a small overshoot" — not exact cardinality.
func (r *stagehandRuntime) enforceLRUCap() {
	if r.cap <= 0 {
		return
	}
	count := 0
	r.cache.Range(func(_, _ any) bool { count++; return true })
	if count <= r.cap {
		return
	}
	var (
		oldestKey string
		oldestTS  int64 = math.MaxInt64
	)
	r.cache.Range(func(k, v any) bool {
		t := v.(*stagehandClientEntry).lastUsedAt.Load()
		if t < oldestTS {
			oldestTS, oldestKey = t, k.(string)
		}
		return true
	})
	if oldestKey == "" {
		return
	}
	if cur, loaded := r.cache.LoadAndDelete(oldestKey); loaded {
		cur.(*stagehandClientEntry).Close()
	}
}

// RunTask implements StagehandInvoker.
//
// Lifecycle per call:
//  1. Look up (or build + cache) the stagehand.Client for this
//     request's key.
//  2. Sessions.Start (browser, model, launch options).
//  3. defer Sessions.End.
//  4. Sessions.Execute (instruction, max-steps).
//  5. Return the agent's final message text.
//
// Multi-tenant isolation: a request with key K hits only the
// cached client for K (or builds one). Other tenants' clients are
// unaffected.
func (r *stagehandRuntime) RunTask(ctx context.Context, req RunTaskRequest) (string, error) {
	if req.Instruction == "" {
		return "", errors.New("stagehand runtime: Instruction is required")
	}
	if req.ModelName == "" {
		return "", errors.New("stagehand runtime: ModelName is required")
	}
	if req.APIKey == "" {
		return "", errors.New("stagehand runtime: APIKey is required (stagehand local mode requires MODEL_API_KEY)")
	}

	client, err := r.clientFor(req)
	if err != nil {
		return "", fmt.Errorf("stagehand runtime: client: %w", err)
	}

	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}

	// Sessions.Start opens a local browser session. Browser.Type =
	// "local" tells the stagehand server to use its bundled CDP
	// driver (not Browserbase). The ModelName is the session-wide
	// LLM; per-call Model.BaseURL/APIKey are passed on Execute.
	//
	// LaunchOptions are populated only when the caller provided
	// non-zero values; the SDK's param.Opt[T] omits zero-valued
	// pointers from the JSON payload, so an unset DownloadsPath /
	// UserDataDir produces byte-identical wire output to v1.
	launchOpts := stagehand.SessionStartParamsBrowserLaunchOptions{
		Headless: stagehand.Bool(headless),
	}
	if req.DownloadsPath != "" {
		launchOpts.DownloadsPath = stagehand.String(req.DownloadsPath)
	}
	if req.UserDataDir != "" {
		launchOpts.UserDataDir = stagehand.String(req.UserDataDir)
		if req.PreserveUserDataDir {
			launchOpts.PreserveUserDataDir = stagehand.Bool(true)
		}
	}
	startResp, err := client.Sessions.Start(ctx, stagehand.SessionStartParams{
		ModelName: req.ModelName,
		Browser: stagehand.SessionStartParamsBrowser{
			Type:          "local",
			LaunchOptions: launchOpts,
		},
	})
	if err != nil {
		return "", fmt.Errorf("stagehand runtime: Sessions.Start: %w", err)
	}
	sessionID := startResp.Data.SessionID
	defer func() {
		// Best-effort End. The deferred call's error is intentionally
		// dropped — the agent result has already been returned; the
		// End failure is logged by the stagehand server.
		_, _ = client.Sessions.End(context.Background(), sessionID, stagehand.SessionEndParams{})
	}()

	// Sessions.Execute runs the multi-step agent. MaxSteps defaults
	// to 30 when zero (matches Python browser.py).
	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 30
	}

	execResp, err := client.Sessions.Execute(ctx, sessionID, stagehand.SessionExecuteParams{
		AgentConfig: stagehand.SessionExecuteParamsAgentConfig{
			// GenericModelConfigObject carries ModelName + APIKey +
			// BaseURL on a per-call basis; the "openai/" prefix in
			// ModelName tells the stagehand server to dispatch via
			// its OpenAI-compatible provider (CHANGELOG #1025 added
			// custom BaseURL support). The union's
			// OfSessionExecutesAgentConfigModelGenericModelConfigObject
			// variant is the only path that exposes BaseURL.
			Model: stagehand.SessionExecuteParamsAgentConfigModelUnion{
				OfSessionExecutesAgentConfigModelGenericModelConfigObject: &stagehand.SessionExecuteParamsAgentConfigModelGenericModelConfigObject{
					ModelName: req.ModelName,
					APIKey:    stagehand.String(req.APIKey),
					BaseURL:   stagehand.String(req.BaseURL),
					Provider:  "openai",
				},
			},
		},
		ExecuteOptions: stagehand.SessionExecuteParamsExecuteOptions{
			Instruction: req.Instruction,
			MaxSteps:    stagehand.Float(float64(maxSteps)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("stagehand runtime: Sessions.Execute: %w", err)
	}

	if !execResp.Success || !execResp.Data.Result.Success {
		// Surface a clear error so the canvas engine can branch on
		// the failure. The agent's own `Message` is included for
		// debugging.
		return "", fmt.Errorf("stagehand runtime: agent did not succeed (success=%v, completed=%v, message=%q)",
			execResp.Data.Result.Success, execResp.Data.Result.Completed, execResp.Data.Result.Message)
	}

	return execResp.Data.Result.Message, nil
}

// RunExtractRequest is the input to stagehandRuntime.RunExtract.
//
// RunExtract is the "structured single-shot" sibling of RunTask:
// it opens a session, optionally navigates to a URL, calls
// Sessions.Extract with a JSON schema, and returns the extracted
// data as a JSON string. The schema's structure is the result
// shape (e.g. `{"headlines": [...]}`); the response field
// `Data.Result` (any) is marshaled as JSON for the caller.
//
// RunExtract is the right API when:
//   - the URL is known up front
//   - the output shape is well-known (zod-style JSON schema)
//   - the caller wants deterministic structured data, not a
//     free-form agent summary
//
// Use RunTask (Sessions.Execute) instead when the agent must
// discover the URL, navigate multiple pages, or chain actions.
type RunExtractRequest struct {
	TenantID    string
	LLMID       string
	ModelName   string // "openai/<model>"
	BaseURL     string
	APIKey      string
	Headless    *bool
	URL         string         // optional: when set, Sessions.Navigate to this URL first
	Instruction string         // natural-language extraction prompt
	Schema      map[string]any // JSON schema; Data.Result is the structured data
}

// RunExtract implements the "structured single-shot" path against
// stagehand-go. Returns the extracted data as a JSON string.
//
// Lifecycle per call:
//  1. Look up (or build + cache) the stagehand.Client for this
//     request's key (same cache as RunTask).
//  2. Sessions.Start (browser, model, launch options).
//  3. defer Sessions.End.
//  4. If URL != "", Sessions.Navigate({URL: req.URL}).
//  5. Sessions.Extract({Instruction, Schema}). The response
//     `Data.Result` field is the structured data matching Schema;
//     we marshal it as JSON and return.
//
// RunExtract reuses the same `stagehandRuntime` cache as RunTask
// (per `(apiKey, baseURL, modelName)` key), so cold-start cost is
// amortized across Task and Extract calls for the same tenant.
func (r *stagehandRuntime) RunExtract(ctx context.Context, req RunExtractRequest) (string, error) {
	if req.Instruction == "" {
		return "", errors.New("stagehand runtime: RunExtract: Instruction is required")
	}
	if req.ModelName == "" {
		return "", errors.New("stagehand runtime: RunExtract: ModelName is required")
	}
	if req.Schema == nil {
		return "", errors.New("stagehand runtime: RunExtract: Schema is required")
	}
	if req.APIKey == "" {
		return "", errors.New("stagehand runtime: RunExtract: APIKey is required (stagehand local mode requires MODEL_API_KEY)")
	}

	client, err := r.clientFor(RunTaskRequest{
		BaseURL:   req.BaseURL,
		APIKey:    req.APIKey,
		ModelName: req.ModelName,
	})
	if err != nil {
		return "", fmt.Errorf("stagehand runtime: RunExtract: client: %w", err)
	}

	headless := true
	if req.Headless != nil {
		headless = *req.Headless
	}

	launchOpts := stagehand.SessionStartParamsBrowserLaunchOptions{
		Headless: stagehand.Bool(headless),
	}
	if req.URL != "" {
		// Note: BrowserLaunchOptions doesn't carry DownloadsPath /
		// UserDataDir here; RunExtract is single-shot extraction
		// without browser automation, so we skip those.
		_ = req.URL
	}

	startResp, err := client.Sessions.Start(ctx, stagehand.SessionStartParams{
		ModelName: req.ModelName,
		Browser: stagehand.SessionStartParamsBrowser{
			Type:          "local",
			LaunchOptions: launchOpts,
		},
	})
	if err != nil {
		return "", fmt.Errorf("stagehand runtime: RunExtract: Sessions.Start: %w", err)
	}
	sessionID := startResp.Data.SessionID
	defer func() {
		_, _ = client.Sessions.End(context.Background(), sessionID, stagehand.SessionEndParams{})
	}()

	// Optional: navigate to the target URL before extracting.
	if req.URL != "" {
		if _, err := client.Sessions.Navigate(ctx, sessionID, stagehand.SessionNavigateParams{
			URL: req.URL,
		}); err != nil {
			return "", fmt.Errorf("stagehand runtime: RunExtract: Sessions.Navigate: %w", err)
		}
	}

	extractResp, err := client.Sessions.Extract(ctx, sessionID, stagehand.SessionExtractParams{
		Instruction: stagehand.String(req.Instruction),
		Schema:      req.Schema,
	})
	if err != nil {
		return "", fmt.Errorf("stagehand runtime: RunExtract: Sessions.Extract: %w", err)
	}
	if !extractResp.Success {
		return "", fmt.Errorf("stagehand runtime: RunExtract: agent did not succeed (success=%v, data=%+v)",
			extractResp.Success, extractResp.Data)
	}

	// Data.Result is `any` (decoded from stagehand server's JSON).
	// Re-marshal as a stable JSON string for the caller.
	out, err := json.Marshal(extractResp.Data.Result)
	if err != nil {
		return "", fmt.Errorf("stagehand runtime: RunExtract: marshal result: %w", err)
	}
	return string(out), nil
}

// Close drains the sweeper goroutine and closes every cached
// client (each Close kills its stagehand-server subprocess).
// Safe to call multiple times.
func (r *stagehandRuntime) Close() error {
	if r.sweepStop != nil {
		select {
		case <-r.sweepStop:
			// already closed
		default:
			close(r.sweepStop)
		}
		if r.sweepDone != nil {
			<-r.sweepDone
		}
	}
	r.cache.Range(func(k, v any) bool {
		v.(*stagehandClientEntry).Close()
		r.cache.Delete(k)
		return true
	})
	return nil
}

// clientFor returns the cached stagehand.Client for req, building
// + caching a new one on miss. Concurrent calls for the same key
// deduplicate via sync.Map.LoadOrStore (the loser closes its
// orphan subprocess).
func (r *stagehandRuntime) clientFor(req RunTaskRequest) (stagehand.Client, error) {
	if req.APIKey == "" {
		return stagehand.Client{}, errors.New("stagehand runtime: APIKey is required")
	}
	key := req.BaseURL + "|" + req.APIKey + "|" + req.ModelName

	// Fast path: read-only.
	if v, ok := r.cache.Load(key); ok {
		e := v.(*stagehandClientEntry)
		e.lastUsedAt.Store(time.Now().UnixNano())
		return e.client, nil
	}

	// Slow path: build a fresh client, then atomically publish.
	// LoadOrStore dedupes concurrent builds for the same key.
	entry := &stagehandClientEntry{
		client: stagehand.NewClient(
			option.WithServer("local"),
			option.WithModelAPIKey(req.APIKey),
		),
	}
	entry.lastUsedAt.Store(time.Now().UnixNano())

	actual, loaded := r.cache.LoadOrStore(key, entry)
	if loaded {
		// Another goroutine won the race. Close our orphan and
		// adopt theirs. Their entry already has a fresh lastUsedAt.
		entry.Close()
		return actual.(*stagehandClientEntry).client, nil
	}

	// We won the publish. Possibly evict the LRU oldest.
	r.enforceLRUCap()
	return entry.client, nil
}
