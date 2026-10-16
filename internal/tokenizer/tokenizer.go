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

package tokenizer

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/pkoukk/tiktoken-go"
	"go.uber.org/zap"

	rag "ragflow/internal/binding"
)

var engineType string

func SetEngineType(engine string) {
	engineType = engine
}

// PoolConfig configures the elastic analyzer pool
type PoolConfig struct {
	DictPath       string        // Path to dictionary files
	MinSize        int           // Minimum number of pre-warmed instances (default: 2*CPU)
	MaxSize        int           // Maximum number of instances allowed (default: 16*CPU)
	IdleTimeout    time.Duration // Idle timeout for shrinking (default: 5 minutes)
	AcquireTimeout time.Duration // Timeout for acquiring an instance (default: 10 seconds)
}

// poolInstance wraps an analyzer instance with metadata for pool management
type poolInstance struct {
	analyzer   *rag.Analyzer
	lastUsedAt time.Time
}

// analyzerPool is the elastic pool for analyzer instances.
//
// stateMu guards the lifecycle state: initialized, baseAnalyzer, and the
// instances channel. acquire/release/shrink hold it for read across their whole
// body; Close takes it for write to close the channel and destroy the
// analyzers. currentSize is atomic and sits outside that pair so the hot paths
// keep their existing read-modify-write.
type analyzerPool struct {
	config       PoolConfig
	baseAnalyzer *rag.Analyzer      // guarded by stateMu
	instances    chan *poolInstance // send/recv and close guarded by stateMu
	currentSize  int32              // atomic
	initialized  bool               // guarded by stateMu
	stateMu      sync.RWMutex
	stopCh       chan struct{} // closed once by stopOnce to wake waiters
	stopOnce     sync.Once
	borrowers    sync.WaitGroup // checked-out instances not yet released
	wg           sync.WaitGroup
}

// defaultLanguage is applied to every analyzer instance on pool acquisition
// to clear any sticky language state left by a previous task. Ingestion
// callers should use the public API variants that accept an explicit
// language override.
const defaultLanguage = "English"

// initMu serializes Init so a single pool is built and published at a time.
// globalPool carries only fully constructed pools. The hot path loads it once
// and pins that pointer for the whole acquire/release pair, so a concurrent
// Init can never make a caller borrow from one pool and return to another.
var (
	initMu     sync.Mutex
	globalPool atomic.Pointer[analyzerPool]
)

// errPoolClosed is returned by acquire when Close is in progress and the pool
// can no longer hand out instances.
var errPoolClosed = errors.New("analyzer pool closed")

// isInitialized reports the live state under the same lock Close uses to clear
// it.
func (p *analyzerPool) isInitialized() bool {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()
	return p.initialized
}

// Init initializes the elastic analyzer pool with the given configuration. It
// is idempotent while a pool is live and builds a fresh pool after Close.
func Init(cfg *PoolConfig) error {
	initMu.Lock()
	defer initMu.Unlock()

	if current := globalPool.Load(); current != nil && current.isInitialized() {
		return nil
	}

	if cfg == nil {
		cfg = &PoolConfig{}
	}

	// Set default values
	if cfg.DictPath == "" {
		if env := common.GetEnv(common.EnvRAGFlowDictPath); env != "" {
			cfg.DictPath = env
		} else {
			cfg.DictPath = "/usr/share/infinity/resource"
		}
	}
	if cfg.MinSize <= 0 {
		cfg.MinSize = runtime.NumCPU() * 2
	}
	if cfg.MaxSize <= 0 {
		cfg.MaxSize = runtime.NumCPU() * 16
	}
	if cfg.MinSize > cfg.MaxSize {
		cfg.MinSize = cfg.MaxSize
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 5 * time.Minute
	}
	if cfg.AcquireTimeout <= 0 {
		cfg.AcquireTimeout = 10 * time.Second
	}

	// The dictionaries are reloaded below, so stale lookups must not survive.
	resetTermLookupCaches()

	common.Info("Initializing analyzer pool",
		zap.String("dict_path", cfg.DictPath),
		zap.Int("min_size", cfg.MinSize),
		zap.Int("max_size", cfg.MaxSize),
		zap.Duration("idle_timeout", cfg.IdleTimeout),
		zap.Duration("acquire_timeout", cfg.AcquireTimeout))

	p := &analyzerPool{
		config:    *cfg,
		instances: make(chan *poolInstance, cfg.MaxSize),
		stopCh:    make(chan struct{}),
	}

	// Create the base analyzer as template
	baseAnalyzer, err := rag.NewAnalyzer(cfg.DictPath)
	if err != nil {
		wrapped := fmt.Errorf("failed to create base analyzer: %w", err)
		common.Error("Failed to create base analyzer", wrapped)
		return wrapped
	}

	if err = baseAnalyzer.Load(); err != nil {
		wrapped := fmt.Errorf("failed to load base analyzer: %w", err)
		common.Error("Failed to load base analyzer", wrapped)
		baseAnalyzer.Close()
		return wrapped
	}

	p.baseAnalyzer = baseAnalyzer

	// Pre-warm minSize instances
	for i := 0; i < cfg.MinSize; i++ {
		instance, err := p.createInstance()
		if err != nil {
			wrapped := fmt.Errorf("failed to create instance %d: %w", i, err)
			common.Error("Failed to create pool instance", wrapped)
			p.discardUnpublished()
			return wrapped
		}
		p.instances <- instance
		atomic.AddInt32(&p.currentSize, 1)
	}

	p.initialized = true

	// Start the shrink loop before publishing, so no observer can see a pool
	// without its shrinker.
	p.wg.Add(1)
	go p.shrinkLoop()

	// Publish only a fully constructed pool.
	globalPool.Store(p)

	common.Info("Analyzer pool initialized successfully",
		zap.Int("pre_warmed", cfg.MinSize),
		zap.Int32("current_size", atomic.LoadInt32(&p.currentSize)))

	return nil
}

// discardUnpublished releases the analyzers of a pool whose Init failed before
// the pool was ever published. initialized is still false on this path, so
// Close's idempotency guard would return before freeing anything.
func (p *analyzerPool) discardUnpublished() {
	p.drainInstances()
	if p.baseAnalyzer != nil {
		p.baseAnalyzer.Close()
		p.baseAnalyzer = nil
	}
}

// drainInstances closes every analyzer currently resting in the channel. It
// drains without blocking, so it is safe whether or not the channel is closed.
func (p *analyzerPool) drainInstances() {
	for {
		select {
		case instance, ok := <-p.instances:
			if !ok {
				return
			}
			if instance != nil && instance.analyzer != nil {
				instance.analyzer.Close()
			}
		default:
			return
		}
	}
}

// createInstance creates a new analyzer instance by copying the base analyzer.
// Callers must hold stateMu for read, or be the sole initializer (Init).
func (p *analyzerPool) createInstance() (*poolInstance, error) {
	if p.baseAnalyzer == nil {
		return nil, fmt.Errorf("base analyzer is nil")
	}

	// Copy the base analyzer to create a new independent instance
	copied := p.baseAnalyzer.Copy()
	if copied == nil {
		return nil, fmt.Errorf("failed to copy analyzer")
	}

	return &poolInstance{
		analyzer:   copied,
		lastUsedAt: time.Now(),
	}, nil
}

// acquire gets an analyzer instance from the pool
// If pool is empty and below max size, creates a new instance dynamically
func (p *analyzerPool) acquire() (*poolInstance, error) {
	// The whole body runs under the read lock: the initialized check, the
	// channel receive, and the dynamic expansion must not interleave with
	// Close's write section, which flips initialized and closes the channel.
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()

	if !p.initialized {
		return nil, fmt.Errorf("pool not initialized")
	}

	// Fast path: try to get from pool without blocking
	select {
	case instance := <-p.instances:
		instance.lastUsedAt = time.Now()
		p.borrowers.Add(1)
		return instance, nil
	default:
	}

	// Slow path: pool is empty, try dynamic expansion or wait
	current := atomic.LoadInt32(&p.currentSize)
	if current < int32(p.config.MaxSize) {
		// Try to increment atomically and create new instance
		if atomic.CompareAndSwapInt32(&p.currentSize, current, current+1) {
			instance, err := p.createInstance()
			if err != nil {
				// Decrement counter on failure
				atomic.AddInt32(&p.currentSize, -1)
				return nil, fmt.Errorf("failed to dynamically create instance: %w", err)
			}
			common.Info("Pool expanded dynamically",
				zap.Int32("previous_size", current),
				zap.Int32("new_size", current+1),
				zap.Int("max_size", p.config.MaxSize))
			p.borrowers.Add(1)
			return instance, nil
		}
		// CAS failed, another goroutine created an instance, fall through to wait
	}

	// Wait for an instance to become available with timeout
	ctx, cancel := context.WithTimeout(context.Background(), p.config.AcquireTimeout)
	defer cancel()

	select {
	case instance := <-p.instances:
		instance.lastUsedAt = time.Now()
		p.borrowers.Add(1)
		return instance, nil
	case <-p.stopCh:
		return nil, errPoolClosed
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for analyzer instance (current_size=%d, max=%d)",
			atomic.LoadInt32(&p.currentSize), p.config.MaxSize)
	}
}

// release returns an analyzer instance to the pool.
func (p *analyzerPool) release(instance *poolInstance) {
	if instance == nil || instance.analyzer == nil {
		return
	}
	defer p.borrowers.Done()

	// The initialized check and the channel send share one critical section:
	// otherwise Close could close the channel between them.
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()

	if !p.initialized {
		instance.analyzer.Close()
		return
	}

	select {
	case p.instances <- instance:
		// Successfully returned to pool
	default:
		// Pool is full (shouldn't happen normally), close this instance
		common.Warn("Pool full when releasing instance, destroying it",
			zap.Int32("current_size", atomic.LoadInt32(&p.currentSize)))
		instance.analyzer.Close()
		atomic.AddInt32(&p.currentSize, -1)
	}
}

// shrinkLoop periodically checks and shrinks the pool by removing idle instances
func (p *analyzerPool) shrinkLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.shrink()
		case <-p.stopCh:
			return
		}
	}
}

// shrink removes idle instances that have exceeded the idle timeout
// while keeping at least MinSize instances
func (p *analyzerPool) shrink() {
	p.stateMu.RLock()
	defer p.stateMu.RUnlock()

	if !p.initialized {
		return
	}

	currentSize := atomic.LoadInt32(&p.currentSize)
	minSize := int32(p.config.MinSize)

	// Only shrink if we have more than minimum instances
	if currentSize <= minSize {
		return
	}

	now := time.Now()
	timeout := p.config.IdleTimeout
	var toRemove []*poolInstance

	// Try to collect idle instances without blocking
	for i := 0; i < int(currentSize-minSize); i++ {
		select {
		case instance := <-p.instances:
			if now.Sub(instance.lastUsedAt) > timeout {
				toRemove = append(toRemove, instance)
			} else {
				// Not idle, put back
				select {
				case p.instances <- instance:
				default:
					// Pool full, should not happen
					toRemove = append(toRemove, instance)
				}
			}
		default:
			// No more instances in pool
			break
		}
	}

	if len(toRemove) > 0 {
		// Close and destroy idle instances
		for _, instance := range toRemove {
			instance.analyzer.Close()
		}

		newSize := atomic.AddInt32(&p.currentSize, -int32(len(toRemove)))
		common.Info("Pool shrunk",
			zap.Int("removed_instances", len(toRemove)),
			zap.Int32("previous_size", currentSize),
			zap.Int32("new_size", newSize),
			zap.Int("min_size", p.config.MinSize))
	}
}

// Close closes the pool and releases all resources. It is idempotent and safe
// to call concurrently with acquire/release.
func (p *analyzerPool) Close() {
	if p == nil {
		return
	}

	// Wake blocked acquirers before requesting the write lock. Once a writer is
	// queued, RWMutex blocks new readers, so an acquire parked on <-p.instances
	// could never be fed by a release and would hold its read lock until the
	// acquire timeout. Closing stopCh makes those waiters return immediately.
	p.stopOnce.Do(func() { close(p.stopCh) })

	// Phase 1: flip initialized under the write lock. acquire/release/shrink
	// hold the read lock across their whole body, so after this no caller can
	// observe the pool as open or touch the channel.
	p.stateMu.Lock()
	if !p.initialized {
		p.stateMu.Unlock()
		return
	}
	p.initialized = false
	p.stateMu.Unlock()

	// shrinkLoop takes stateMu.RLock inside shrink, so this wait stays outside
	// the lock; stopCh is already closed, so it returns promptly.
	p.wg.Wait()

	// A copied analyzer references the base analyzer's loaded dictionaries
	// (RAGAnalyzer_Copy shares trie_/pos_table_/wordnet_lemma_ and does not own
	// them), so the base must outlive every checked-out instance. Wait for the
	// borrowers to return before destroying it.
	p.borrowers.Wait()

	// Phase 2: close the channel and release the analyzers. The read lock keeps
	// a late release from sending here; such a release sees initialized == false
	// and closes its own instance instead. drainInstances then collects whatever
	// is still buffered.
	p.stateMu.Lock()
	close(p.instances)
	p.drainInstances()
	if p.baseAnalyzer != nil {
		p.baseAnalyzer.Close()
		p.baseAnalyzer = nil
	}
	p.stateMu.Unlock()

	common.Info(fmt.Sprintf("Analyzer pool closed, final_size: %d", atomic.LoadInt32(&p.currentSize)))
}

// GetPoolStats returns current pool statistics
func GetPoolStats() map[string]interface{} {
	p := globalPool.Load()
	if p == nil {
		return map[string]interface{}{
			"initialized": false,
		}
	}

	p.stateMu.RLock()
	initialized := p.initialized
	available := len(p.instances)
	p.stateMu.RUnlock()

	return map[string]interface{}{
		"initialized":         initialized,
		"current_size":        atomic.LoadInt32(&p.currentSize),
		"min_size":            p.config.MinSize,
		"max_size":            p.config.MaxSize,
		"idle_timeout":        p.config.IdleTimeout.String(),
		"instances_available": available,
	}
}

// Close closes the global pool
func Close() {
	if p := globalPool.Load(); p != nil {
		p.Close()
	}
}

// withAnalyzer acquires an analyzer instance, applies the given language
// (with "" mapping to defaultLanguage), and executes fn.
func withAnalyzer(lang string, fn func(*rag.Analyzer) error) error {
	// Load the pool once and use that pointer for both acquire and release, so a
	// concurrent Init cannot swap the global pool between the two.
	pool := globalPool.Load()
	if pool == nil {
		return fmt.Errorf("tokenizer pool not initialized")
	}

	instance, err := pool.acquire()
	if err != nil {
		return err
	}
	defer pool.release(instance)

	if lang == "" {
		lang = defaultLanguage
	}
	instance.analyzer.SetLanguage(lang)

	return fn(instance.analyzer)
}

// withAnalyzerResult is the result-returning variant of withAnalyzer.
func withAnalyzerResult[T any](lang string, fn func(*rag.Analyzer) (T, error)) (T, error) {
	var result T
	pool := globalPool.Load()
	if pool == nil {
		return result, fmt.Errorf("tokenizer pool not initialized")
	}

	instance, err := pool.acquire()
	if err != nil {
		return result, err
	}
	defer pool.release(instance)

	if lang == "" {
		lang = defaultLanguage
	}
	instance.analyzer.SetLanguage(lang)

	return fn(instance.analyzer)
}

type Tokenizer struct {
	lang string
}

// New returns a request-scoped tokenizer. Empty language falls back to English.
func New(lang string) Tokenizer {
	return Tokenizer{lang: lang}
}

var defaultTokenizer = New("")

// Tokenize tokenizes the text and returns a space-separated string of tokens.
// Example: "hello world" -> "hello world"
//
// NOTE: For Infinity engine, returns input unchanged to match python's behavior.
func Tokenize(text string) (string, error) {
	return defaultTokenizer.Tokenize(text)
}

// Tokenize tokenizes the text using the tokenizer's request-scoped language.
func (t Tokenizer) Tokenize(text string) (string, error) {
	if engineType == "infinity" {
		return text, nil
	}
	return withAnalyzerResult(t.lang, func(a *rag.Analyzer) (string, error) {
		return a.Tokenize(text)
	})
}

// TokenizeWithPosition tokenizes the text and returns a list of tokens with position information.
func TokenizeWithPosition(text string) ([]rag.TokenWithPosition, error) {
	return defaultTokenizer.TokenizeWithPosition(text)
}

// TokenizeWithPosition tokenizes the text using the tokenizer's request-scoped language.
func (t Tokenizer) TokenizeWithPosition(text string) ([]rag.TokenWithPosition, error) {
	return withAnalyzerResult(t.lang, func(a *rag.Analyzer) ([]rag.TokenWithPosition, error) {
		return a.TokenizeWithPosition(text)
	})
}

// Analyze analyzes the text and returns all tokens.
func Analyze(text string) ([]rag.Token, error) {
	return defaultTokenizer.Analyze(text)
}

// Analyze analyzes the text using the tokenizer's request-scoped language.
func (t Tokenizer) Analyze(text string) ([]rag.Token, error) {
	return withAnalyzerResult(t.lang, func(a *rag.Analyzer) ([]rag.Token, error) {
		return a.Analyze(text)
	})
}

// SetFineGrained sets whether to use fine-grained tokenization
// Note: This is a no-op in pool mode as each request uses its own instance
// To configure an instance, modify the base analyzer before Init() or use custom instances
func SetFineGrained(fineGrained bool) {
	// In pool mode, we don't set global state on instances
	// Each request gets a fresh instance with default settings
	common.Debug("SetFineGrained is no-op in pool mode", zap.Bool("fine_grained", fineGrained))
}

// FineGrainedTokenize performs fine-grained tokenization on space-separated
// tokens.
// Input: space-separated tokens (e.g., "hello world 测试")
// Output: space-separated fine-grained tokens (e.g., "hello world 测 试")
//
// NOTE: For Infinity engine, returns input unchanged to match python's behavior.
func FineGrainedTokenize(tokens string) (string, error) {
	return defaultTokenizer.FineGrainedTokenize(tokens)
}

// FineGrainedTokenize performs fine-grained tokenization using the tokenizer's
// request-scoped language.
func (t Tokenizer) FineGrainedTokenize(tokens string) (string, error) {
	if engineType == "infinity" {
		return tokens, nil
	}
	return withAnalyzerResult(t.lang, func(a *rag.Analyzer) (string, error) {
		return a.FineGrainedTokenize(tokens)
	})
}

// SetEnablePosition sets whether to enable position tracking
// Note: This is a no-op in pool mode as each request uses its own instance
func SetEnablePosition(enablePosition bool) {
	common.Debug("SetEnablePosition is no-op in pool mode", zap.Bool("enable_position", enablePosition))
}

// IsInitialized checks whether the tokenizer pool has been initialized
func IsInitialized() bool {
	p := globalPool.Load()
	return p != nil && p.isInitialized()
}

// termLookupCacheMax caps each memo below. Reaching it rotates the memo instead
// of freezing it: freezing lets whatever happened to arrive first occupy the
// budget permanently, so a term that only became hot later would miss forever,
// with no way back. Rotating keeps the memo representative of recent traffic and
// costs one re-warm of the (small) hot set per rotation. This mirrors
// bpeModel.memoPut in bpe.go, for the same reason: the memo is an optimisation,
// not a correctness structure, so clearing it beats tracking recency.
const termLookupCacheMax = 1 << 16

// termCacheRotateMu serialises rotation so a burst of writers past the cap does
// not clear the memo repeatedly. It is only taken on the rotation path, never on
// a hit.
var termCacheRotateMu sync.Mutex

// Memoized because every lookup acquires a pooled analyzer and crosses cgo,
// while Weights issues one freq plus one tag per token per retrieved chunk. A
// result depends only on the term and on the dictionaries Init loads, so the
// term alone is a valid key. Only successful lookups are cached, so a call made
// before Init cannot pin a zero.
//
// The analyzer resolves both halves from the same packed trie entry, so they
// share one memo rather than two maps keyed by the same term: that halves the
// key storage, and a miss resolves the pair with a single analyzer checkout.
var (
	termLookupCache  sync.Map // term -> termLookup
	termLookupCached atomic.Int64
)

// termLookup is one dictionary entry: the pair the analyzer resolves together.
type termLookup struct {
	freq int32
	tag  string
}

func resetTermLookupCaches() {
	termLookupCache.Clear()
	termLookupCached.Store(0)
}

// admitTerm memoizes key -> val in cache, rotating the whole memo once it has
// reached limit. Rotation runs before the insert, so the value just looked up
// survives into the new generation instead of being recomputed.
//
// Admission is a check-then-act: concurrent writers can overshoot limit by the
// number of writers in flight, and a rotation can race an in-flight insert (that
// entry is dropped, its count lost with it). Both leave the memo bounded and
// approximately counted, which is all this policy needs.
func admitTerm[V any](cache *sync.Map, counter *atomic.Int64, limit int64, key string, val V) {
	if counter.Load() >= limit {
		termCacheRotateMu.Lock()
		if counter.Load() >= limit {
			cache.Clear()
			counter.Store(0)
			common.Info("term lookup memo rotated", zap.Int64("limit", limit))
		}
		termCacheRotateMu.Unlock()
	}
	if _, loaded := cache.LoadOrStore(key, val); !loaded {
		counter.Add(1)
	}
}

// GetTermFreqAndTag returns the frequency and the POS tag of a term, resolving
// both in one analyzer checkout. They come from the same packed trie entry, so
// asking for them separately pays for two checkouts and two cgo round trips.
// Returns: the frequency, or 0 if the term is not in the dictionary; and the POS
// tag, or "" if it has none.
func GetTermFreqAndTag(term string) (int32, string) {
	if v, ok := termLookupCache.Load(term); ok {
		entry := v.(termLookup)
		return entry.freq, entry.tag
	}
	entry, err := withAnalyzerResult("", func(a *rag.Analyzer) (termLookup, error) {
		return termLookup{freq: a.GetTermFreq(term), tag: a.GetTermTag(term)}, nil
	})
	if err != nil {
		return entry.freq, entry.tag
	}
	admitTerm(&termLookupCache, &termLookupCached, termLookupCacheMax, term, entry)
	return entry.freq, entry.tag
}

// GetTermFreq returns the frequency of a term (matching Python rag_tokenizer.freq)
// Returns: frequency value, or 0 if term not found
func GetTermFreq(term string) int32 {
	freq, _ := GetTermFreqAndTag(term)
	return freq
}

// GetTermTag returns the POS tag of a term (matching Python rag_tokenizer.tag)
// Returns: POS tag string (e.g., "n", "v", "ns"), or empty string if term not found or no tag
func GetTermTag(term string) string {
	_, tag := GetTermFreqAndTag(term)
	return tag
}

var cl100kEncoder struct {
	sync.Once
	enc *tiktoken.Tiktoken
	err error
}

func getCL100KEncoder() (*tiktoken.Tiktoken, error) {
	cl100kEncoder.Do(func() {
		cl100kEncoder.enc, cl100kEncoder.err = tiktoken.GetEncoding("cl100k_base")
	})
	return cl100kEncoder.enc, cl100kEncoder.err
}

// resetCL100KEncoderForTest clears the cl100k encoder cache so a test can force a
// fresh load under a different search-root scope. Test-only; it mutates package
// state. A plain assignment replaces the embedded sync.Once with an un-fired one
// and wipes any cached result, so the next getCL100KEncoder / InitCL100KEncoder
// re-runs the loader rather than returning a result cached by a sibling test.
func resetCL100KEncoderForTest() {
	cl100kEncoder = struct {
		sync.Once
		enc *tiktoken.Tiktoken
		err error
	}{}
}

// InitCL100KEncoder loads the cl100k_base BPE encoder once and returns an
// error if the table is unavailable. Call it during server startup (before any
// request reaches NumTokensFromString) so a missing table fails fast instead
// of silently zeroing every token count.
//
// Why fail-fast and not a retry/warning: NumTokensFromString intentionally
// returns 0 on encoder error to stay cheap on the hot path, which means a
// missing cl100k_base.tiktoken (the Go image used to omit it) degraded every
// token budget to 0 while content_ltks — a separate offline C++ tokenizer —
// kept working, and the divergence went unnoticed. Catching it at startup turns
// that silent data corruption into a hard, loud failure.
func InitCL100KEncoder() error {
	enc, err := getCL100KEncoder()
	if err != nil {
		return fmt.Errorf("cl100k_base BPE table unavailable (NumTokensFromString would silently return 0): %w", err)
	}
	if enc == nil {
		return fmt.Errorf("cl100k_base encoder unavailable: GetEncoding returned a nil encoder without error")
	}
	return nil
}

// NumTokensFromString returns the number of tokens in s using the cl100k_base
// BPE encoding.
//
// A missing BPE table PANICS instead of returning 0. Python's
// num_tokens_from_string (common/token_utils.py) resolves its encoder OUTSIDE the
// try, so an unavailable table raises there — only an encode() failure is folded
// into 0. Returning 0 here would fail OPEN (every budget would "fit"), which is
// the exact silent-corruption mode that made a Go image missing
// cl100k_base.tiktoken zero every token count. InitCL100KEncoder still runs at
// startup (cmd/ragflow_server.go) so a bad deployment dies with a clear message,
// and this function now stays loud too in case a caller bypasses that guard.
func NumTokensFromString(s string) int {
	if s == "" {
		return 0
	}
	enc, err := getCL100KEncoder()
	if err != nil {
		panic(fmt.Sprintf("tokenizer.NumTokensFromString: cl100k_base BPE table unavailable: %v", err))
	}
	if enc == nil {
		panic("tokenizer.NumTokensFromString: cl100k_base encoder is nil")
	}
	return len(enc.Encode(s, nil, nil))
}

// TrimContentToTokenLimit truncates s to at most limit tokens using the
// cl100k_base encoder. Mirrors Python's trim_content helper in
// rag/prompts/generator.py: encoder.decode(encoder.encode(content)[:limit]).
// Returns the original string if it already fits.
//
// Like Python's trim_content, an unavailable encoder PANICS: the previous
// byte-length fallback silently produced a differently-truncated string instead
// of surfacing the missing table. See NumTokensFromString.
// The decoded prefix is snapped back to a rune boundary before it is returned.
// Cutting the token list is not enough on its own: a single emoji is four UTF-8
// bytes and may span tokens, so a token-boundary cut can end mid-character and
// hand the caller - and the index - invalid UTF-8. (TestCountersSatisfyTrimProperties
// caught exactly that on the emoji corpus at limit=1.) Dropping the partial rune
// can only shrink the text, but the token count is re-checked anyway so the
// "at most limit tokens" contract holds unconditionally.
func TrimContentToTokenLimit(s string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	enc, err := getCL100KEncoder()
	if err != nil {
		panic(fmt.Sprintf("tokenizer.TrimContentToTokenLimit: cl100k_base BPE table unavailable: %v", err))
	}
	if enc == nil {
		panic("tokenizer.TrimContentToTokenLimit: cl100k_base encoder is nil")
	}
	tokens := enc.Encode(s, nil, nil)
	if len(tokens) <= limit {
		return s
	}
	trimmed := clampToValidUTF8(enc.Decode(tokens[:limit]))
	for i := 0; i < utf8.UTFMax && len(trimmed) > 0 && len(enc.Encode(trimmed, nil, nil)) > limit; i++ {
		trimmed = dropLastRune(trimmed)
	}
	return trimmed
}

// clampToValidUTF8 returns the longest prefix of s that is valid UTF-8. Only the
// tail can be broken after a token-boundary cut, so this is at most three bytes
// of backtracking rather than a scan for an interior invalid byte.
func clampToValidUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	for cut := 1; cut <= utf8.UTFMax && cut <= len(s); cut++ {
		if prefix := s[:len(s)-cut]; utf8.ValidString(prefix) {
			return prefix
		}
	}
	return ""
}

// dropLastRune removes the final rune of s, tolerating a trailing partial rune.
func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	if size <= 0 {
		return ""
	}
	return s[:len(s)-size]
}

// EncodeCL100KTokens returns the cl100k_base token ids of s, from the same
// cached encoder NumTokensFromString and TrimContentToTokenLimit use. ok is
// false when the encoder is unavailable — the same degraded world where those
// two degrade — so callers can run their own fallback instead of mistaking an
// empty result for "zero tokens".
func EncodeCL100KTokens(s string) (tokens []int, ok bool) {
	enc, err := getCL100KEncoder()
	if err != nil || enc == nil {
		return nil, false
	}
	return enc.Encode(s, nil, nil), true
}

// DecodeCL100KTokens concatenates the raw byte sequences of tokens. Decoding
// is a plain vocabulary-table concat (no re-segmentation), so
// Decode(Encode(s)) == s and Decode(tokens[a:b]) is exactly the corresponding
// byte slice of s — including a slice whose ends cut a multibyte rune, which
// comes back as raw continuation bytes rather than U+FFFD. Returns "" when
// the encoder is unavailable (callers gate on EncodeCL100KTokens first).
func DecodeCL100KTokens(tokens []int) string {
	enc, err := getCL100KEncoder()
	if err != nil || enc == nil {
		return ""
	}
	return enc.Decode(tokens)
}
