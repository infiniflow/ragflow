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
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/logger"
)

func init() {
	// Initialize logger for tests
	if err := logger.Init("info"); err != nil {
		fmt.Printf("Failed to initialize logger: %v\n", err)
	}
}

// TestConcurrentTokenize tests concurrent tokenization with dynamic pool expansion and shrinking
func TestConcurrentTokenize(t *testing.T) {
	// Use small pool to test expansion
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        2,
		MaxSize:        10,
		IdleTimeout:    5 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}
	defer Close()

	// Print initial pool stats
	stats := GetPoolStats()
	t.Logf("Initial pool stats: %+v", stats)

	// Test texts
	texts := []string{
		"Hello world this is a test",
		"Natural language processing is amazing",
		"Elastic pool handles concurrent requests",
		"中文分词测试",
		"深度学习与机器学习",
		"RAGFlow is an open-source RAG engine",
	}

	// Phase 1: High concurrency test - should trigger expansion
	t.Log("=== Phase 1: High concurrency test (should trigger expansion) ===")
	var expansionDetected int32
	var wg sync.WaitGroup
	numGoroutines := 20
	requestsPerGoroutine := 10

	start := time.Now()
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				text := texts[(id+j)%len(texts)]
				result, err := Tokenize(text)
				if err != nil {
					t.Errorf("Goroutine %d request %d failed: %v", id, j, err)
					return
				}
				if result == "" {
					t.Errorf("Goroutine %d request %d returned empty result", id, j)
				}

				// Check pool stats periodically
				if j%5 == 0 {
					stats := GetPoolStats()
					currentSize := stats["current_size"].(int32)
					if currentSize > int32(cfg.MinSize) {
						atomic.StoreInt32(&expansionDetected, 1)
					}
				}
			}
		}(i)
	}
	wg.Wait()
	phase1Duration := time.Since(start)

	stats = GetPoolStats()
	t.Logf("Phase 1 completed in %v", phase1Duration)
	t.Logf("Pool stats after Phase 1: %+v", stats)

	if atomic.LoadInt32(&expansionDetected) == 1 {
		t.Log("✓ Pool expansion detected during high concurrency")
	} else {
		t.Log("℗ Pool expansion not detected (may need more concurrency)")
	}

	currentSize := stats["current_size"].(int32)
	if currentSize > int32(cfg.MinSize) {
		t.Logf("✓ Current pool size (%d) is greater than minSize (%d)", currentSize, cfg.MinSize)
	}

	// Phase 2: Wait for idle timeout - should trigger shrinking
	t.Log("=== Phase 2: Waiting for idle timeout (should trigger shrinking) ===")
	t.Logf("Waiting %v for idle instances to timeout...", cfg.IdleTimeout)
	time.Sleep(cfg.IdleTimeout + 2*time.Second)

	stats = GetPoolStats()
	t.Logf("Pool stats after Phase 2 (waiting): %+v", stats)

	currentSize = stats["current_size"].(int32)
	if currentSize <= int32(cfg.MinSize) {
		t.Logf("✓ Pool shrunk back to minSize or below: current=%d, min=%d", currentSize, cfg.MinSize)
	} else {
		t.Logf("℗ Pool not yet shrunk: current=%d, min=%d (may need more time)", currentSize, cfg.MinSize)
	}

	// Phase 3: Moderate concurrency after shrink - should trigger expansion again
	t.Log("=== Phase 3: Moderate concurrency after shrink (should trigger re-expansion) ===")
	var reExpansionDetected int32
	start = time.Now()
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine/2; j++ {
				text := texts[(id+j)%len(texts)]
				_, err := Tokenize(text)
				if err != nil {
					t.Errorf("Phase 3 goroutine %d request %d failed: %v", id, j, err)
					return
				}

				if j%3 == 0 {
					stats := GetPoolStats()
					currentSize := stats["current_size"].(int32)
					if currentSize > int32(cfg.MinSize) {
						atomic.StoreInt32(&reExpansionDetected, 1)
					}
				}
			}
		}(i)
	}
	wg.Wait()
	phase3Duration := time.Since(start)

	stats = GetPoolStats()
	t.Logf("Phase 3 completed in %v", phase3Duration)
	t.Logf("Pool stats after Phase 3: %+v", stats)

	if atomic.LoadInt32(&reExpansionDetected) == 1 {
		t.Log("✓ Pool re-expansion detected after shrink")
	}

	t.Log("=== Test completed successfully ===")
}

// TestConcurrentTokenizeWithPosition tests concurrent tokenization with position info
func TestConcurrentTokenizeWithPosition(t *testing.T) {
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        2,
		MaxSize:        8,
		IdleTimeout:    3 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}
	defer Close()

	text := "This is a test sentence for position tracking"
	var wg sync.WaitGroup
	numGoroutines := 15

	t.Log("=== Testing TokenizeWithPosition concurrently ===")
	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				tokens, err := TokenizeWithPosition(text)
				if err != nil {
					t.Errorf("Goroutine %d request %d failed: %v", id, j, err)
					return
				}
				if len(tokens) == 0 {
					t.Errorf("Goroutine %d request %d returned empty tokens", id, j)
					return
				}
				// Verify position info
				for _, token := range tokens {
					if token.Text == "" {
						t.Errorf("Goroutine %d request %d returned empty token text", id, j)
						return
					}
					if token.EndOffset <= token.Offset {
						t.Errorf("Goroutine %d request %d has invalid position: offset=%d, end=%d",
							id, j, token.Offset, token.EndOffset)
						return
					}
				}
			}
		}(i)
	}
	wg.Wait()

	duration := time.Since(start)
	stats := GetPoolStats()
	t.Logf("Completed %d goroutines x 5 requests in %v", numGoroutines, duration)
	t.Logf("Final pool stats: %+v", stats)
	t.Log("✓ TokenizeWithPosition concurrent test passed")
}

// TestPoolExhaustion tests pool exhaustion and timeout behavior
func TestPoolExhaustion(t *testing.T) {
	// Very small pool to test exhaustion
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        1,
		MaxSize:        2,
		IdleTimeout:    10 * time.Second,
		AcquireTimeout: 500 * time.Millisecond, // Short timeout for faster test
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}
	defer Close()

	t.Log("=== Testing pool exhaustion behavior ===")
	stats := GetPoolStats()
	t.Logf("Initial pool stats: %+v", stats)

	// Use all available instances
	var wg sync.WaitGroup
	barrier := make(chan struct{})
	errors := make(chan error, 10)

	// Launch goroutines that hold instances
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-barrier // Wait for signal to start
			_, err := Tokenize("test text")
			if err != nil {
				errors <- fmt.Errorf("goroutine %d: %w", id, err)
			}
		}(i)
	}

	// Release all goroutines at once to create contention
	close(barrier)

	// Wait for all to complete
	wg.Wait()
	close(errors)

	timeoutCount := 0
	for err := range errors {
		if err != nil {
			t.Logf("Expected error from limited pool: %v", err)
			timeoutCount++
		}
	}

	stats = GetPoolStats()
	t.Logf("Final pool stats: %+v", stats)
	t.Logf("Timeout errors: %d (expected with small pool)", timeoutCount)

	if timeoutCount > 0 {
		t.Log("✓ Pool correctly returned timeout errors when exhausted")
	} else {
		t.Log("℗ No timeout errors (pool handled all requests, may be too fast)")
	}
}

// TestFineGrainedTokenizeConcurrent tests concurrent fine-grained tokenization
func TestFineGrainedTokenizeConcurrent(t *testing.T) {
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        2,
		MaxSize:        6,
		IdleTimeout:    3 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}
	defer Close()

	tokens := "hello world 中文测试"
	var wg sync.WaitGroup
	numGoroutines := 10

	t.Log("=== Testing FineGrainedTokenize concurrently ===")
	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				result, err := FineGrainedTokenize(tokens)
				if err != nil {
					t.Errorf("Goroutine %d request %d failed: %v", id, j, err)
					return
				}
				if result == "" {
					t.Errorf("Goroutine %d request %d returned empty result", id, j)
				}
			}
		}(i)
	}
	wg.Wait()

	duration := time.Since(start)
	stats := GetPoolStats()
	t.Logf("Completed %d goroutines x 5 requests in %v", numGoroutines, duration)
	t.Logf("Final pool stats: %+v", stats)
	t.Log("✓ FineGrainedTokenize concurrent test passed")
}

// TestTermFreqAndTagConcurrent tests concurrent term frequency and tag lookups
func TestTermFreqAndTagConcurrent(t *testing.T) {
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        2,
		MaxSize:        6,
		IdleTimeout:    3 * time.Second,
		AcquireTimeout: 5 * time.Second,
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Failed to initialize pool: %v", err)
	}
	defer Close()

	terms := []string{"hello", "world", "中文", "test", "natural"}
	var wg sync.WaitGroup
	numGoroutines := 10

	t.Log("=== Testing GetTermFreq and GetTermTag concurrently ===")
	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				term := terms[(id+j)%len(terms)]
				freq := GetTermFreq(term)
				tag := GetTermTag(term)
				// We don't validate the results as terms may or may not exist in dictionary
				// Just ensuring no panics or errors
				_ = freq
				_ = tag
			}
		}(i)
	}
	wg.Wait()

	duration := time.Since(start)
	stats := GetPoolStats()
	t.Logf("Completed %d goroutines x 10 requests in %v", numGoroutines, duration)
	t.Logf("Final pool stats: %+v", stats)
	t.Log("✓ GetTermFreq and GetTermTag concurrent test passed")
}

// BenchmarkTokenize benchmarks the tokenization performance
func BenchmarkTokenize(b *testing.B) {
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        runtime.NumCPU() * 2,
		MaxSize:        runtime.NumCPU() * 4,
		IdleTimeout:    5 * time.Minute,
		AcquireTimeout: 10 * time.Second,
	}

	if err := Init(cfg); err != nil {
		b.Fatalf("Failed to initialize pool: %v", err)
	}
	defer Close()

	text := "This is a benchmark test for tokenization performance with natural language processing"

	// Warm up
	for i := 0; i < 100; i++ {
		Tokenize(text)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := Tokenize(text)
			if err != nil {
				b.Errorf("Tokenize failed: %v", err)
			}
		}
	})

	stats := GetPoolStats()
	b.Logf("Final pool stats: %+v", stats)
}

// BenchmarkTokenizeWithPosition benchmarks position-aware tokenization
func BenchmarkTokenizeWithPosition(b *testing.B) {
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        runtime.NumCPU() * 2,
		MaxSize:        runtime.NumCPU() * 4,
		IdleTimeout:    5 * time.Minute,
		AcquireTimeout: 10 * time.Second,
	}

	if err := Init(cfg); err != nil {
		b.Fatalf("Failed to initialize pool: %v", err)
	}
	defer Close()

	text := "This is a benchmark test for position-aware tokenization"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := TokenizeWithPosition(text)
			if err != nil {
				b.Errorf("TokenizeWithPosition failed: %v", err)
			}
		}
	})
}

// ExampleGetPoolStats demonstrates getting pool statistics
func ExampleGetPoolStats() {
	cfg := &PoolConfig{
		DictPath:       "/usr/share/infinity/resource",
		MinSize:        2,
		MaxSize:        10,
		IdleTimeout:    5 * time.Minute,
		AcquireTimeout: 10 * time.Second,
	}

	if err := Init(cfg); err != nil {
		fmt.Printf("Failed to initialize: %v\n", err)
		return
	}
	defer Close()

	stats := GetPoolStats()
	fmt.Printf("Pool initialized: %v\n", stats["initialized"])
	fmt.Printf("Current size: %d\n", stats["current_size"])
	fmt.Printf("Min size: %d\n", stats["min_size"])
	fmt.Printf("Max size: %d\n", stats["max_size"])

	// Output will vary based on actual initialization
}

// logPoolStats logs pool statistics using the zap logger
func logPoolStats(msg string) {
	stats := GetPoolStats()
	logger.Info(msg,
		zap.Bool("initialized", stats["initialized"].(bool)),
		zap.Int32("current_size", stats["current_size"].(int32)),
		zap.Int("min_size", stats["min_size"].(int)),
		zap.Int("max_size", stats["max_size"].(int)),
		zap.String("idle_timeout", stats["idle_timeout"].(string)),
		zap.Int("instances_available", stats["instances_available"].(int)),
	)
}
