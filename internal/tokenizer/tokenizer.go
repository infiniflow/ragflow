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
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"

	rag "ragflow/internal/go_binding"
	"ragflow/internal/logger"
)

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

// analyzerPool is the elastic pool for analyzer instances
type analyzerPool struct {
	config       PoolConfig
	baseAnalyzer *rag.Analyzer      // Original analyzer used as template for copying
	instances    chan *poolInstance // Channel-based pool for available instances
	currentSize  int32              // Current number of instances (atomic)
	initialized  bool
	mu           sync.RWMutex
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

var (
	globalPool    *analyzerPool
	poolOnce      sync.Once
	poolInitError error
)

// Init initializes the elastic analyzer pool with the given configuration
// Can be called multiple times if the pool was previously closed
func Init(cfg *PoolConfig) error {
	// Check if we need to reset poolOnce (for testing or re-initialization)
	if globalPool != nil && !globalPool.initialized {
		// Pool was closed, reset poolOnce for re-initialization
		poolOnce = sync.Once{}
	}

	poolOnce.Do(func() {
		if cfg == nil {
			cfg = &PoolConfig{}
		}

		// Set default values
		if cfg.DictPath == "" {
			cfg.DictPath = "/usr/share/infinity/resource"
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

		logger.Info("Initializing analyzer pool",
			zap.String("dict_path", cfg.DictPath),
			zap.Int("min_size", cfg.MinSize),
			zap.Int("max_size", cfg.MaxSize),
			zap.Duration("idle_timeout", cfg.IdleTimeout),
			zap.Duration("acquire_timeout", cfg.AcquireTimeout))

		globalPool = &analyzerPool{
			config:    *cfg,
			instances: make(chan *poolInstance, cfg.MaxSize),
			stopCh:    make(chan struct{}),
		}

		// Create the base analyzer as template
		baseAnalyzer, err := rag.NewAnalyzer(cfg.DictPath)
		if err != nil {
			poolInitError = fmt.Errorf("failed to create base analyzer: %w", err)
			logger.Error("Failed to create base analyzer", poolInitError)
			return
		}

		if err = baseAnalyzer.Load(); err != nil {
			poolInitError = fmt.Errorf("failed to load base analyzer: %w", err)
			logger.Error("Failed to load base analyzer", poolInitError)
			baseAnalyzer.Close()
			return
		}

		globalPool.baseAnalyzer = baseAnalyzer

		// Pre-warm minSize instances
		for i := 0; i < cfg.MinSize; i++ {
			instance, err := globalPool.createInstance()
			if err != nil {
				poolInitError = fmt.Errorf("failed to create instance %d: %w", i, err)
				logger.Error("Failed to create pool instance", poolInitError)
				globalPool.Close()
				return
			}
			globalPool.instances <- instance
			atomic.AddInt32(&globalPool.currentSize, 1)
		}

		globalPool.initialized = true
		logger.Info("Analyzer pool initialized successfully",
			zap.Int("pre_warmed", cfg.MinSize),
			zap.Int32("current_size", atomic.LoadInt32(&globalPool.currentSize)))

		// Start the shrink loop for idle instance cleanup
		globalPool.wg.Add(1)
		go globalPool.shrinkLoop()
	})

	return poolInitError
}

// createInstance creates a new analyzer instance by copying the base analyzer
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
	if !p.initialized {
		return nil, fmt.Errorf("pool not initialized")
	}

	// Fast path: try to get from pool without blocking
	select {
	case instance := <-p.instances:
		instance.lastUsedAt = time.Now()
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
			logger.Info("Pool expanded dynamically",
				zap.Int32("previous_size", current),
				zap.Int32("new_size", current+1),
				zap.Int("max_size", p.config.MaxSize))
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
		return instance, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for analyzer instance (current_size=%d, max=%d)",
			atomic.LoadInt32(&p.currentSize), p.config.MaxSize)
	}
}

// release returns an analyzer instance to the pool
func (p *analyzerPool) release(instance *poolInstance) {
	if instance == nil || instance.analyzer == nil {
		return
	}

	if !p.initialized {
		instance.analyzer.Close()
		return
	}

	select {
	case p.instances <- instance:
		// Successfully returned to pool
	default:
		// Pool is full (shouldn't happen normally), close this instance
		logger.Warn("Pool full when releasing instance, destroying it",
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
		logger.Info("Pool shrunk",
			zap.Int("removed_instances", len(toRemove)),
			zap.Int32("previous_size", currentSize),
			zap.Int32("new_size", newSize),
			zap.Int("min_size", p.config.MinSize))
	}
}

// Close closes the pool and releases all resources
func (p *analyzerPool) Close() {
	if p == nil {
		return
	}

	p.mu.Lock()
	if !p.initialized {
		p.mu.Unlock()
		return
	}
	p.initialized = false
	p.mu.Unlock()

	// Signal shrink loop to stop
	close(p.stopCh)
	p.wg.Wait()

	// Close all instances in pool
	close(p.instances)
	for instance := range p.instances {
		if instance != nil && instance.analyzer != nil {
			instance.analyzer.Close()
		}
	}

	// Close base analyzer
	if p.baseAnalyzer != nil {
		p.baseAnalyzer.Close()
		p.baseAnalyzer = nil
	}

	logger.Info("Analyzer pool closed",
		zap.Int32("final_size", atomic.LoadInt32(&p.currentSize)))
}

// GetPoolStats returns current pool statistics
func GetPoolStats() map[string]interface{} {
	if globalPool == nil {
		return map[string]interface{}{
			"initialized": false,
		}
	}

	return map[string]interface{}{
		"initialized":         globalPool.initialized,
		"current_size":        atomic.LoadInt32(&globalPool.currentSize),
		"min_size":            globalPool.config.MinSize,
		"max_size":            globalPool.config.MaxSize,
		"idle_timeout":        globalPool.config.IdleTimeout.String(),
		"instances_available": len(globalPool.instances),
	}
}

// Close closes the global pool
func Close() {
	if globalPool != nil {
		globalPool.Close()
	}
}

// withAnalyzer executes the given function with an exclusive analyzer instance
func withAnalyzer(fn func(*rag.Analyzer) error) error {
	if globalPool == nil {
		return fmt.Errorf("tokenizer pool not initialized")
	}

	instance, err := globalPool.acquire()
	if err != nil {
		return err
	}
	defer globalPool.release(instance)

	return fn(instance.analyzer)
}

// withAnalyzerResult executes the given function with an exclusive analyzer instance and returns a result
func withAnalyzerResult[T any](fn func(*rag.Analyzer) (T, error)) (T, error) {
	var result T
	if globalPool == nil {
		return result, fmt.Errorf("tokenizer pool not initialized")
	}

	instance, err := globalPool.acquire()
	if err != nil {
		return result, err
	}
	defer globalPool.release(instance)

	return fn(instance.analyzer)
}

// Tokenize tokenizes the text and returns a space-separated string of tokens
// Example: "hello world" -> "hello world"
func Tokenize(text string) (string, error) {
	return withAnalyzerResult(func(a *rag.Analyzer) (string, error) {
		return a.Tokenize(text)
	})
}

// TokenizeWithPosition tokenizes the text and returns a list of tokens with position information
func TokenizeWithPosition(text string) ([]rag.TokenWithPosition, error) {
	return withAnalyzerResult(func(a *rag.Analyzer) ([]rag.TokenWithPosition, error) {
		return a.TokenizeWithPosition(text)
	})
}

// Analyze analyzes the text and returns all tokens
func Analyze(text string) ([]rag.Token, error) {
	return withAnalyzerResult(func(a *rag.Analyzer) ([]rag.Token, error) {
		return a.Analyze(text)
	})
}

// SetFineGrained sets whether to use fine-grained tokenization
// Note: This is a no-op in pool mode as each request uses its own instance
// To configure an instance, modify the base analyzer before Init() or use custom instances
func SetFineGrained(fineGrained bool) {
	// In pool mode, we don't set global state on instances
	// Each request gets a fresh instance with default settings
	logger.Debug("SetFineGrained is no-op in pool mode", zap.Bool("fine_grained", fineGrained))
}

// FineGrainedTokenize performs fine-grained tokenization on space-separated tokens
// Input: space-separated tokens (e.g., "hello world 测试")
// Output: space-separated fine-grained tokens (e.g., "hello world 测 试")
func FineGrainedTokenize(tokens string) (string, error) {
	return withAnalyzerResult(func(a *rag.Analyzer) (string, error) {
		return a.FineGrainedTokenize(tokens)
	})
}

// SetEnablePosition sets whether to enable position tracking
// Note: This is a no-op in pool mode as each request uses its own instance
func SetEnablePosition(enablePosition bool) {
	logger.Debug("SetEnablePosition is no-op in pool mode", zap.Bool("enable_position", enablePosition))
}

// IsInitialized checks whether the tokenizer pool has been initialized
func IsInitialized() bool {
	return globalPool != nil && globalPool.initialized
}

// GetTermFreq returns the frequency of a term (matching Python rag_tokenizer.freq)
// Returns: frequency value, or 0 if term not found
func GetTermFreq(term string) int32 {
	result, _ := withAnalyzerResult(func(a *rag.Analyzer) (int32, error) {
		return a.GetTermFreq(term), nil
	})
	return result
}

// GetTermTag returns the POS tag of a term (matching Python rag_tokenizer.tag)
// Returns: POS tag string (e.g., "n", "v", "ns"), or empty string if term not found or no tag
func GetTermTag(term string) string {
	result, _ := withAnalyzerResult(func(a *rag.Analyzer) (string, error) {
		return a.GetTermTag(term), nil
	})
	return result
}
